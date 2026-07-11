package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/services/oauth2/token":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"access_token": "test-token",
			})
		case r.URL.Path == "/services/data/v61.0/tooling/query":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"size":      1,
				"totalSize": 1,
				"done":      true,
				"records": []map[string]any{
					{
						"Id":                            "0P1000000000001AAA",
						"DeveloperName":                 "TestCert",
						"MasterLabel":                   "Test Certificate",
						"ExpirationDate":                "2025-12-31",
						"OptionsIsPrivateKeyExportable": false,
						"KeySize":                       2048,
					},
				},
			})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

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
					resource.TestCheckResourceAttr("data.salesforce_certificate.test", "id", "0P1000000000001AAA"),
					resource.TestCheckResourceAttr("data.salesforce_certificate.test", "developer_name", "TestCert"),
					resource.TestCheckResourceAttr("data.salesforce_certificate.test", "master_label", "Test Certificate"),
					resource.TestCheckResourceAttr("data.salesforce_certificate.test", "expiration_date", "2025-12-31"),
					resource.TestCheckResourceAttr("data.salesforce_certificate.test", "is_private_key_exportable", "false"),
					resource.TestCheckResourceAttr("data.salesforce_certificate.test", "key_size", "2048"),
				),
			},
		},
	})
}

func TestCertificateDataSource_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/services/oauth2/token":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"access_token": "test-token",
			})
		case r.URL.Path == "/services/data/v61.0/tooling/query":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"size":      0,
				"totalSize": 0,
				"done":      true,
				"records":   []map[string]any{},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

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
