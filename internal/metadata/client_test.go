package metadata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_doRequest(t *testing.T) {
	t.Run("successful request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify request
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if !strings.HasPrefix(r.URL.Path, "/services/Soap/m/") {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			if ct := r.Header.Get("Content-Type"); ct != "text/xml; charset=UTF-8" {
				t.Errorf("unexpected Content-Type: %s", ct)
			}
			if action := r.Header.Get("SOAPAction"); action != "testAction" {
				t.Errorf("unexpected SOAPAction: %s", action)
			}

			// Return valid SOAP response
			w.Header().Set("Content-Type", "text/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Body>
    <testResponse>success</testResponse>
  </soapenv:Body>
</soapenv:Envelope>`))
		}))
		defer server.Close()

		client := NewClient(server.URL, "test-session-id")
		body, err := client.doRequest(context.Background(), "testAction", &ReadMetadataRequest{
			Type:      "Certificate",
			FullNames: []string{"TestCert"},
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(string(body), "testResponse") {
			t.Errorf("unexpected response body: %s", string(body))
		}
	})

	t.Run("SOAP fault", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/xml")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Body>
    <soapenv:Fault>
      <faultcode>sf:INVALID_TYPE</faultcode>
      <faultstring>Invalid type: Unknown</faultstring>
    </soapenv:Fault>
  </soapenv:Body>
</soapenv:Envelope>`))
		}))
		defer server.Close()

		client := NewClient(server.URL, "test-session-id")
		_, err := client.doRequest(context.Background(), "testAction", &ReadMetadataRequest{
			Type:      "Unknown",
			FullNames: []string{"Test"},
		})

		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var soapErr *SOAPFaultError
		if !errors.As(err, &soapErr) {
			t.Errorf("expected SOAPFaultError, got %T: %v", err, err)
		}
	})

	t.Run("custom API version", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.URL.Path, "/62.0") {
				t.Errorf("expected API version 62.0 in path, got %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "text/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Body><ok/></soapenv:Body>
</soapenv:Envelope>`))
		}))
		defer server.Close()

		client := NewClient(server.URL, "test-session-id", WithAPIVersion("62.0"))
		_, err := client.doRequest(context.Background(), "test", &ReadMetadataRequest{})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
