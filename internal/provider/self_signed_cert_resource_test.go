package provider

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const testCertPEM = `-----BEGIN CERTIFICATE-----
MIIDBTCCAe2gAwIBAgIUH1zJiy0bKoQXd2nZubWokLGpf5kwDQYJKoZIhvcNAQEL
BQAwEjEQMA4GA1UEAwwHVGVzdCBDQTAeFw0yNjA3MTExODMyMDBaFw0yNzA3MTEx
ODMyMDBaMBIxEDAOBgNVBAMMB1Rlc3QgQ0EwggEiMA0GCSqGSIb3DQEBAQUAA4IB
DwAwggEKAoIBAQCwdqOrF9Hd77u9TEfvvaq/7KuV3SqesWoIE1RgYLEP3Egu0BYn
ZxbBMR0ZT+f6aklWZscqYEJE+beO0fQptB3tSzUVsFaGcllzNUW13LJ31/dRcCCW
4jCBiPV6CO2bYYqoQU3zfCfSAAR1aQyg1HQNyZh+AzV0xvljA5I/tNFTx56W9qlc
6JPYVjAhg8v51AUkCYiB26Oo7sXokQ65S4LVlJJd6yxLY/vk71RPp/a3OT7PX0QK
dzl3OV6aiZPKzklKg3utvNXsj+v+6I6g7nuTcdPJfHLWvUpZihen61dXm5QV5DDv
rua7JXlGB4OpmER7fRWwX+OsP7DM1jLQ+byBAgMBAAGjUzBRMB0GA1UdDgQWBBS4
c5qu/KijkkY+nx1ofpMaOGcaajAfBgNVHSMEGDAWgBS4c5qu/KijkkY+nx1ofpMa
OGcaajAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQBdMTHuJzUY
mIhOH03Wm52KZU5xe6PL6FPHho52/tOIyl5VZwsEaKtjOiYj4ZqXnW6DugNpV9yK
s2H6jolg8ZoH5licn+dEjDuWTOn8A44TIm1UhpJTe1QCxM/cjU6VrEY92lkmR6qg
HXgUQfstPCc9GOY7QtIO43ubL4spBm+s5U6ERRhP5KLC4KGPQQIVq36IXaR9EFl/
YQJMg13DrFHljoabFc96n3V/5mBlrGSwrrHbSxtloStIBwvux6rN53NKA/p/nZQS
0uqxDvC4dW5q+G0ZT/4bHlBz7x0M/hcd8uGda1ChN9V6EV8jwDMlBWtQLHExfmWs
Awi6554msCQ1
-----END CERTIFICATE-----
`

func TestReadyForRenewal(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		expires   string
		early     int64
		wantReady bool
	}{
		{name: "before window", expires: "2026-08-11", early: 24, wantReady: false},
		{name: "inside window", expires: "2026-07-12", early: 24, wantReady: true},
		{name: "expired", expires: "2026-07-10", early: 0, wantReady: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readyForRenewal(now, tt.expires, tt.early)
			if err != nil {
				t.Fatalf("readyForRenewal returned error: %v", err)
			}
			if got != tt.wantReady {
				t.Fatalf("expected %v, got %v", tt.wantReady, got)
			}
		})
	}
}

func TestReadyForRenewal_InvalidDate(t *testing.T) {
	_, err := readyForRenewal(time.Now(), "bad-date", 0)
	if err == nil {
		t.Fatal("expected error for invalid date")
	}
}

func TestSelfSignedCertResource_CreateReadDelete(t *testing.T) {
	var serverURL string
	deployCalls := 0
	deleteCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/services/oauth2/token":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"access_token": "test-token"})
		case r.URL.Path == "/services/oauth2/singleaccess":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"frontdoor_uri": serverURL + "/secur/frontdoor.jsp?otp=abc"})
		case r.URL.Path == "/secur/frontdoor.jsp":
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "test-session-id"})
			w.WriteHeader(http.StatusOK)
		case strings.HasPrefix(r.URL.Path, "/services/Soap/m/"):
			bodyBytes, _ := io.ReadAll(r.Body)
			body := string(bodyBytes)
			w.Header().Set("Content-Type", "text/xml")
			switch {
			case strings.Contains(body, "checkDeployStatus"):
				w.Write([]byte(`<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"><soapenv:Body><checkDeployStatusResponse><result><id>0AfCreate</id><done>true</done><success>true</success><status>Succeeded</status></result></checkDeployStatusResponse></soapenv:Body></soapenv:Envelope>`))
			case strings.Contains(body, "readMetadata"):
				cert := base64.StdEncoding.EncodeToString([]byte(testCertPEM))
				w.Write([]byte(`<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"><soapenv:Body><readMetadataResponse><result><records><fullName>TerraformE2ETestCert</fullName><masterLabel>Terraform E2E Test Cert</masterLabel><certificate>` + cert + `</certificate><expirationDate>2027-07-11</expirationDate><keySize>2048</keySize><platform>false</platform><privateKeyExportable>false</privateKeyExportable></records></result></readMetadataResponse></soapenv:Body></soapenv:Envelope>`))
			case strings.Contains(body, "deploy"):
				deployZip, err := readDeployZip(body)
				if err != nil {
					t.Fatalf("failed to read deploy zip: %v", err)
				}
				if strings.Contains(deployZip, "destructiveChanges.xml") {
					deleteCalls++
					w.Write([]byte(`<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"><soapenv:Body><deployResponse><result><id>0AfDelete</id><done>false</done><state>Queued</state></result></deployResponse></soapenv:Body></soapenv:Envelope>`))
					return
				}
				deployCalls++
				w.Write([]byte(`<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"><soapenv:Body><deployResponse><result><id>0AfCreate</id><done>false</done><state>Queued</state></result></deployResponse></soapenv:Body></soapenv:Envelope>`))
			default:
				t.Fatalf("unexpected SOAP request: %s", body)
			}
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	t.Setenv("SALESFORCE_INSTANCE_URL", server.URL)
	t.Setenv("SALESFORCE_CLIENT_ID", "test-client-id")
	t.Setenv("SALESFORCE_CLIENT_SECRET", "test-client-secret")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "salesforce_self_signed_cert" "test" {
  developer_name = "TerraformE2ETestCert"
  master_label   = "Terraform E2E Test Cert"

  key_size               = 2048
  private_key_exportable = false
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("salesforce_self_signed_cert.test", "id", "TerraformE2ETestCert"),
					resource.TestCheckResourceAttrSet("salesforce_self_signed_cert.test", "cert_pem"),
					resource.TestCheckResourceAttrSet("salesforce_self_signed_cert.test", "cert_sha256"),
					resource.TestCheckResourceAttr("salesforce_self_signed_cert.test", "expiration_date", "2027-07-11"),
					resource.TestCheckResourceAttr("salesforce_self_signed_cert.test", "ready_for_renewal", "false"),
				),
			},
		},
	})

	if deployCalls == 0 {
		t.Fatal("expected create deploy call")
	}
	if deleteCalls == 0 {
		t.Fatal("expected delete deploy call during test cleanup")
	}
}

func TestAccSelfSignedCertResource(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set, skipping acceptance test")
	}

	name := fmt.Sprintf("TerraformAccCert%d", time.Now().Unix())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "salesforce_self_signed_cert" "test" {
  developer_name = %q
  master_label   = "Terraform Acceptance Cert"

  key_size               = 2048
  private_key_exportable = false
  early_renewal_hours    = 720
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("salesforce_self_signed_cert.test", "id", name),
					resource.TestCheckResourceAttrSet("salesforce_self_signed_cert.test", "cert_pem"),
					resource.TestCheckResourceAttrSet("salesforce_self_signed_cert.test", "cert_sha256"),
					resource.TestCheckResourceAttrSet("salesforce_self_signed_cert.test", "expiration_date"),
					resource.TestCheckResourceAttr("salesforce_self_signed_cert.test", "key_size", "2048"),
					resource.TestCheckResourceAttr("salesforce_self_signed_cert.test", "private_key_exportable", "false"),
				),
			},
		},
	})
}

func readDeployZip(body string) (string, error) {
	var req struct {
		ZipFile string `xml:"Body>deploy>ZipFile"`
	}
	if err := xml.Unmarshal([]byte(body), &req); err != nil {
		return "", err
	}
	zipBytes, err := base64.StdEncoding.DecodeString(req.ZipFile)
	if err != nil {
		return "", err
	}
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return "", err
	}
	var contents strings.Builder
	for _, f := range zr.File {
		contents.WriteString(f.Name)
		contents.WriteByte('\n')
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return "", err
		}
		contents.Write(data)
		contents.WriteByte('\n')
	}
	return contents.String(), nil
}
