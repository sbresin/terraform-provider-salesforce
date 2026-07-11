package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccCertificateDataSource(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set, skipping acceptance test")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
data "salesforce_certificate" "test" {
  developer_name = "TestCertificate"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.salesforce_certificate.test", "id"),
					resource.TestCheckResourceAttr("data.salesforce_certificate.test", "developer_name", "TestCertificate"),
				),
			},
		},
	})
}

func TestCertificateDataSource_Read(t *testing.T) {
	// Unit test with mock server
	var serverURL string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/services/oauth2/token":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"access_token": "test-token",
			})

		case r.URL.Path == "/services/oauth2/singleaccess":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"frontdoor_uri": serverURL + "/secur/frontdoor.jsp?otp=abc",
			})

		case r.URL.Path == "/secur/frontdoor.jsp":
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "test-session-id"})
			w.WriteHeader(http.StatusOK)

		case strings.HasPrefix(r.URL.Path, "/services/Soap/m/"):
			// Metadata API request
			w.Header().Set("Content-Type", "text/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Body>
    <readMetadataResponse>
      <result>
        <records>
          <fullName>TestCert</fullName>
          <masterLabel>Test Certificate</masterLabel>
          <certificate>LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t</certificate>
          <expirationDate>2025-12-31</expirationDate>
          <keySize>2048</keySize>
          <platform>false</platform>
          <privateKeyExportable>false</privateKeyExportable>
        </records>
      </result>
    </readMetadataResponse>
  </soapenv:Body>
</soapenv:Envelope>`))

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	// Set environment variables for provider
	t.Setenv("SALESFORCE_INSTANCE_URL", server.URL)
	t.Setenv("SALESFORCE_CLIENT_ID", "test-client-id")
	t.Setenv("SALESFORCE_CLIENT_SECRET", "test-client-secret")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
data "salesforce_certificate" "test" {
  developer_name = "TestCert"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.salesforce_certificate.test", "id", "TestCert"),
					resource.TestCheckResourceAttr("data.salesforce_certificate.test", "developer_name", "TestCert"),
					resource.TestCheckResourceAttr("data.salesforce_certificate.test", "master_label", "Test Certificate"),
					resource.TestCheckResourceAttr("data.salesforce_certificate.test", "expiration_date", "2025-12-31"),
					resource.TestCheckResourceAttr("data.salesforce_certificate.test", "is_private_key_exportable", "false"),
					resource.TestCheckResourceAttr("data.salesforce_certificate.test", "key_size", "2048"),
					resource.TestCheckResourceAttr("data.salesforce_certificate.test", "certificate", "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t"),
					resource.TestCheckResourceAttr("data.salesforce_certificate.test", "platform", "false"),
				),
			},
		},
	})
}

func TestCertificateDataSource_NotFound(t *testing.T) {
	var serverURL string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/services/oauth2/token":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"access_token": "test-token",
			})

		case r.URL.Path == "/services/oauth2/singleaccess":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"frontdoor_uri": serverURL + "/secur/frontdoor.jsp?otp=abc",
			})

		case r.URL.Path == "/secur/frontdoor.jsp":
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "test-session-id"})
			w.WriteHeader(http.StatusOK)

		case strings.HasPrefix(r.URL.Path, "/services/Soap/m/"):
			// Return empty result (not found)
			w.Header().Set("Content-Type", "text/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Body>
    <readMetadataResponse>
      <result>
        <records/>
      </result>
    </readMetadataResponse>
  </soapenv:Body>
</soapenv:Envelope>`))

		default:
			w.WriteHeader(http.StatusNotFound)
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
data "salesforce_certificate" "test" {
  developer_name = "NonExistent"
}
`,
				ExpectError: regexMustCompile(".*not found.*"),
			},
		},
	})
}

func regexMustCompile(s string) *regexp.Regexp {
	return regexp.MustCompile(s)
}
