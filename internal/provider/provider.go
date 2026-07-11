package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

var _ provider.Provider = &SalesforceProvider{}

type SalesforceProvider struct {
	version string
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &SalesforceProvider{
			version: version,
		}
	}
}

func (p *SalesforceProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "salesforce"
	resp.Version = p.version
}

func (p *SalesforceProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Interact with Salesforce APIs.",
	}
}

func (p *SalesforceProvider) Configure(_ context.Context, _ provider.ConfigureRequest, _ *provider.ConfigureResponse) {
	// TODO: implement in Task 5
}

func (p *SalesforceProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

func (p *SalesforceProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{}
}
