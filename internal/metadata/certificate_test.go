package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
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
          <certificate>LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t</certificate>
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
