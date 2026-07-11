package metadata

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClient_ReadCertificate(t *testing.T) {
	t.Run("successful read", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:sf="http://soap.sforce.com/2006/04/metadata">
  <soapenv:Body>
    <readMetadataResponse>
      <result>
        <records>
          <fullName>TestCert</fullName>
          <masterLabel>Test Certificate</masterLabel>
          <content>LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t</content>
          <expirationDate>2025-12-31</expirationDate>
          <keySize>2048</keySize>
          <platform>false</platform>
          <privateKeyExportable>true</privateKeyExportable>
        </records>
      </result>
    </readMetadataResponse>
  </soapenv:Body>
</soapenv:Envelope>`))
		}))
		defer server.Close()

		client := NewClient(server.URL, "test-session-id")
		cert, err := client.ReadCertificate(context.Background(), "TestCert")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cert.FullName != "TestCert" {
			t.Errorf("expected fullName 'TestCert', got %q", cert.FullName)
		}
		if cert.MasterLabel != "Test Certificate" {
			t.Errorf("expected masterLabel 'Test Certificate', got %q", cert.MasterLabel)
		}
		if cert.Certificate != "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t" {
			t.Errorf("unexpected certificate value: %q", cert.Certificate)
		}
		if cert.KeySize != 2048 {
			t.Errorf("expected keySize 2048, got %d", cert.KeySize)
		}
		if cert.PrivateKeyExportable != true {
			t.Error("expected privateKeyExportable true")
		}

		expectedDate := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
		if cert.ExpirationDate == nil || !cert.ExpirationDate.Equal(expectedDate) {
			t.Errorf("expected expiration date %v, got %v", expectedDate, cert.ExpirationDate)
		}
	})

	t.Run("certificate not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		}))
		defer server.Close()

		client := NewClient(server.URL, "test-session-id")
		_, err := client.ReadCertificate(context.Background(), "NonExistent")

		if err != ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestClient_ListCertificates(t *testing.T) {
	t.Run("successful list", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Body>
    <listMetadataResponse>
      <result>
        <fullName>Cert1</fullName>
        <type>Certificate</type>
      </result>
      <result>
        <fullName>Cert2</fullName>
        <type>Certificate</type>
      </result>
    </listMetadataResponse>
  </soapenv:Body>
</soapenv:Envelope>`))
		}))
		defer server.Close()

		client := NewClient(server.URL, "test-session-id")
		names, err := client.ListCertificates(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(names) != 2 {
			t.Fatalf("expected 2 certificates, got %d", len(names))
		}
		if names[0] != "Cert1" || names[1] != "Cert2" {
			t.Errorf("unexpected certificate names: %v", names)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Body>
    <listMetadataResponse/>
  </soapenv:Body>
</soapenv:Envelope>`))
		}))
		defer server.Close()

		client := NewClient(server.URL, "test-session-id")
		names, err := client.ListCertificates(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(names) != 0 {
			t.Errorf("expected empty list, got %v", names)
		}
	})
}

func TestClient_DeleteCertificate(t *testing.T) {
	t.Run("successful delete", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/xml")

			body, _ := io.ReadAll(r.Body)
			bodyStr := string(body)

			if strings.Contains(bodyStr, "deploy") && !strings.Contains(bodyStr, "checkDeployStatus") {
				// Deploy request
				w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Body>
    <deployResponse>
      <result>
        <id>0Af000000000001</id>
        <done>false</done>
        <state>Queued</state>
      </result>
    </deployResponse>
  </soapenv:Body>
</soapenv:Envelope>`))
			} else {
				// CheckDeployStatus request - return done
				w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Body>
    <checkDeployStatusResponse>
      <result>
        <id>0Af000000000001</id>
        <done>true</done>
        <success>true</success>
        <status>Succeeded</status>
      </result>
    </checkDeployStatusResponse>
  </soapenv:Body>
</soapenv:Envelope>`))
			}
		}))
		defer server.Close()

		client := NewClient(server.URL, "test-session-id")
		deployID, err := client.DeleteCertificate(context.Background(), "TestCert")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if deployID != "0Af000000000001" {
			t.Errorf("expected deployID '0Af000000000001', got %q", deployID)
		}
	})
}

func TestBuildCertificateDeployZip(t *testing.T) {
	zipData, err := buildCertificateDeployZip(CertificateCreate{
		FullName:             "TerraformE2ESigningCert",
		MasterLabel:          "Terraform E2E Signing Cert",
		KeySize:              2048,
		PrivateKeyExportable: false,
	}, "67.0")
	if err != nil {
		t.Fatalf("buildCertificateDeployZip returned error: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("zip did not parse: %v", err)
	}

	files := map[string]string{}
	for _, f := range zr.File {
		r, err := f.Open()
		if err != nil {
			t.Fatalf("opening %s: %v", f.Name, err)
		}
		content, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			t.Fatalf("reading %s: %v", f.Name, err)
		}
		files[f.Name] = string(content)
	}

	packageXML := files["package.xml"]
	if !strings.Contains(packageXML, "<members>TerraformE2ESigningCert</members>") {
		t.Fatalf("package.xml missing certificate member: %s", packageXML)
	}
	if !strings.Contains(packageXML, "<name>Certificate</name>") {
		t.Fatalf("package.xml missing Certificate type: %s", packageXML)
	}

	certXML := files["certs/TerraformE2ESigningCert.crt-meta.xml"]
	for _, want := range []string{
		"<keySize>2048</keySize>",
		"<masterLabel>Terraform E2E Signing Cert</masterLabel>",
		"<privateKeyExportable>false</privateKeyExportable>",
	} {
		if !strings.Contains(certXML, want) {
			t.Fatalf("certificate XML missing %q: %s", want, certXML)
		}
	}
}
