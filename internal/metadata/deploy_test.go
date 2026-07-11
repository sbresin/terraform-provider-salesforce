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
)

func TestClient_Deploy(t *testing.T) {
	t.Run("successful deploy", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Read request body to verify it contains base64 zip
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "ZipFile") {
				t.Error("expected ZipFile in request")
			}

			w.Header().Set("Content-Type", "text/xml")
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
		}))
		defer server.Close()

		client := NewClient(server.URL, "test-session-id")

		// Create a minimal test zip
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		w, _ := zw.Create("package.xml")
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Package/>`))
		zw.Close()

		deployID, err := client.Deploy(context.Background(), buf.Bytes(), DeployOptions{
			SinglePackage: true,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if deployID != "0Af000000000001" {
			t.Errorf("expected deployID '0Af000000000001', got %q", deployID)
		}
	})
}

func TestClient_CheckDeployStatus(t *testing.T) {
	t.Run("deploy in progress", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Body>
    <checkDeployStatusResponse>
      <result>
        <id>0Af000000000001</id>
        <done>false</done>
        <status>InProgress</status>
        <numberComponentsTotal>1</numberComponentsTotal>
        <numberComponentsDeployed>0</numberComponentsDeployed>
      </result>
    </checkDeployStatusResponse>
  </soapenv:Body>
</soapenv:Envelope>`))
		}))
		defer server.Close()

		client := NewClient(server.URL, "test-session-id")
		result, err := client.CheckDeployStatus(context.Background(), "0Af000000000001")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Done {
			t.Error("expected Done=false")
		}
		if result.Status != "InProgress" {
			t.Errorf("expected status 'InProgress', got %q", result.Status)
		}
	})

	t.Run("deploy succeeded", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/xml")
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
		}))
		defer server.Close()

		client := NewClient(server.URL, "test-session-id")
		result, err := client.CheckDeployStatus(context.Background(), "0Af000000000001")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !result.Done {
			t.Error("expected Done=true")
		}
		if !result.Success {
			t.Error("expected Success=true")
		}
	})

	t.Run("deploy failed with component errors", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Body>
    <checkDeployStatusResponse>
      <result>
        <id>0Af000000000001</id>
        <done>true</done>
        <success>false</success>
        <status>Failed</status>
        <details>
          <componentFailures>
            <fullName>TestCert</fullName>
            <problem>Certificate in use</problem>
            <problemType>Error</problemType>
            <componentType>Certificate</componentType>
          </componentFailures>
        </details>
      </result>
    </checkDeployStatusResponse>
  </soapenv:Body>
</soapenv:Envelope>`))
		}))
		defer server.Close()

		client := NewClient(server.URL, "test-session-id")
		result, err := client.CheckDeployStatus(context.Background(), "0Af000000000001")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Success {
			t.Error("expected Success=false")
		}
		if len(result.ComponentFailures) != 1 {
			t.Fatalf("expected 1 component failure, got %d", len(result.ComponentFailures))
		}
		if result.ComponentFailures[0].Problem != "Certificate in use" {
			t.Errorf("unexpected problem: %q", result.ComponentFailures[0].Problem)
		}
	})
}

func TestBuildDestructiveChangesZip(t *testing.T) {
	zipData, err := buildDestructiveChangesZip("Certificate", "TestCert", "67.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it's a valid zip
	reader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("invalid zip: %v", err)
	}

	// Check expected files
	expectedFiles := map[string]bool{
		"package.xml":            false,
		"destructiveChanges.xml": false,
	}

	for _, f := range reader.File {
		if _, ok := expectedFiles[f.Name]; ok {
			expectedFiles[f.Name] = true

			// Read and verify content
			rc, _ := f.Open()
			content, _ := io.ReadAll(rc)
			rc.Close()

			if f.Name == "destructiveChanges.xml" {
				if !strings.Contains(string(content), "TestCert") {
					t.Errorf("destructiveChanges.xml missing certificate name")
				}
				if !strings.Contains(string(content), "Certificate") {
					t.Errorf("destructiveChanges.xml missing type")
				}
			}
			if f.Name == "package.xml" {
				if !strings.Contains(string(content), "67.0") {
					t.Errorf("package.xml missing version")
				}
			}
		}
	}

	for name, found := range expectedFiles {
		if !found {
			t.Errorf("missing expected file: %s", name)
		}
	}
}

func TestWaitForDeploy_Success(t *testing.T) {
	call := 0
	client, server := newTestMetadataClient(t, func(w http.ResponseWriter, r *http.Request) {
		call++
		w.Header().Set("Content-Type", "text/xml")
		if call == 1 {
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Body><checkDeployStatusResponse><result><id>0AfTest</id><done>false</done><status>InProgress</status></result></checkDeployStatusResponse></soapenv:Body>
</soapenv:Envelope>`))
			return
		}
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Body><checkDeployStatusResponse><result><id>0AfTest</id><done>true</done><success>true</success><status>Succeeded</status></result></checkDeployStatusResponse></soapenv:Body>
</soapenv:Envelope>`))
	})
	defer server.Close()

	result, err := client.WaitForDeploy(context.Background(), "0AfTest")
	if err != nil {
		t.Fatalf("WaitForDeploy returned error: %v", err)
	}
	if !result.Success || result.Status != "Succeeded" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestWaitForDeploy_Failure(t *testing.T) {
	client, server := newTestMetadataClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
  <soapenv:Body><checkDeployStatusResponse><result><id>0AfTest</id><done>true</done><success>false</success><status>Failed</status><errorMessage>bad cert</errorMessage></result></checkDeployStatusResponse></soapenv:Body>
</soapenv:Envelope>`))
	})
	defer server.Close()

	_, err := client.WaitForDeploy(context.Background(), "0AfTest")
	if err == nil || !strings.Contains(err.Error(), "bad cert") {
		t.Fatalf("expected failure containing bad cert, got %v", err)
	}
}

func newTestMetadataClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()

	server := httptest.NewServer(handler)
	client := NewClient(server.URL, "test-session-id", WithAPIVersion("67.0"))
	return client, server
}
