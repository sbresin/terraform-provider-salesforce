package provider

import (
	"context"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func testAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"salesforce": providerserver.NewProtocol6WithError(New("test")()),
	}
}

func TestProvider_Schema(t *testing.T) {
	p := New("test")()
	resp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics)
	}

	schema := resp.Schema

	// Check that expected attributes exist
	attrs := schema.Attributes
	expectedAttrs := []string{"instance_url", "client_id", "client_secret", "auth_url", "api_version"}
	for _, name := range expectedAttrs {
		if _, ok := attrs[name]; !ok {
			t.Errorf("expected attribute %q not found in schema", name)
		}
	}
}

func TestProvider_ConfigureValidation(t *testing.T) {
	// This test validates that the provider requires auth configuration
	// It will be expanded when we have a mock server for testing
	t.Run("schema has description", func(t *testing.T) {
		p := New("test")()
		resp := &provider.SchemaResponse{}
		p.Schema(context.Background(), provider.SchemaRequest{}, resp)

		if resp.Schema.Description == "" {
			t.Error("expected provider schema to have a description")
		}
	})
}

func TestProvider_Resources(t *testing.T) {
	p := New("test")()
	resources := p.Resources(context.Background())
	if len(resources) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(resources))
	}
}

// TestAccProvider_Configure is an acceptance test that requires real credentials.
// Run with: TF_ACC=1 go test -v ./internal/provider/...
func TestAccProvider_Configure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping acceptance test")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
provider "salesforce" {
  # Uses environment variables:
  # SALESFORCE_AUTH_URL or
  # SALESFORCE_INSTANCE_URL + SALESFORCE_CLIENT_ID + SALESFORCE_CLIENT_SECRET
}
`,
				// Just verify the provider configures without error
				// when proper env vars are set
			},
		},
	})
}

func unsetSalesforceEnv(t *testing.T) {
	for _, name := range []string{"SALESFORCE_AUTH_URL", "SALESFORCE_INSTANCE_URL", "SALESFORCE_CLIENT_ID", "SALESFORCE_CLIENT_SECRET"} {
		t.Setenv(name, "")
	}
}

// A root module that only uses the provider in some environments (count = 0
// elsewhere) must plan without credentials there.
func TestProvider_UnusedWithoutCredentials(t *testing.T) {
	unsetSalesforceEnv(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "salesforce_self_signed_cert" "test" {
  count          = 0
  developer_name = "Unused"
}
`,
			},
		},
	})
}

func TestProvider_UsedWithoutCredentials(t *testing.T) {
	unsetSalesforceEnv(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "salesforce_self_signed_cert" "test" {
  developer_name = "Used"
}
`,
				ExpectError: regexp.MustCompile("Missing Authentication Configuration"),
			},
			{
				Config: `
data "salesforce_cert" "test" {
  developer_name = "Used"
}
`,
				ExpectError: regexp.MustCompile("Missing Authentication Configuration"),
			},
		},
	})
}
