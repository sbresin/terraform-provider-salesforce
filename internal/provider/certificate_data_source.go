package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/sbresin/terraform-provider-salesforce/internal/client"
)

var _ datasource.DataSource = &CertificateDataSource{}
var _ datasource.DataSourceWithConfigure = &CertificateDataSource{}

type CertificateDataSource struct {
	client *client.Client
}

type CertificateDataSourceModel struct {
	ID                     types.String `tfsdk:"id"`
	DeveloperName          types.String `tfsdk:"developer_name"`
	MasterLabel            types.String `tfsdk:"master_label"`
	ExpirationDate         types.String `tfsdk:"expiration_date"`
	IsPrivateKeyExportable types.Bool   `tfsdk:"is_private_key_exportable"`
	KeySize                types.Int64  `tfsdk:"key_size"`
}

func NewCertificateDataSource() datasource.DataSource {
	return &CertificateDataSource{}
}

func (d *CertificateDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_certificate"
}

func (d *CertificateDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Retrieves information about a Salesforce Certificate.",
		Attributes: map[string]schema.Attribute{
			"developer_name": schema.StringAttribute{
				Description: "The developer name (API name) of the certificate.",
				Required:    true,
			},
			"id": schema.StringAttribute{
				Description: "The Salesforce ID of the certificate.",
				Computed:    true,
			},
			"master_label": schema.StringAttribute{
				Description: "The human-readable label of the certificate.",
				Computed:    true,
			},
			"expiration_date": schema.StringAttribute{
				Description: "The expiration date of the certificate (YYYY-MM-DD format).",
				Computed:    true,
			},
			"is_private_key_exportable": schema.BoolAttribute{
				Description: "Whether the private key can be exported.",
				Computed:    true,
			},
			"key_size": schema.Int64Attribute{
				Description: "The key size in bits (e.g., 2048, 4096).",
				Computed:    true,
			},
		},
	}
}

func (d *CertificateDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData),
		)
		return
	}

	d.client = client
}

func (d *CertificateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data CertificateDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	developerName := data.DeveloperName.ValueString()

	// Query the Tooling API for the certificate
	soql := fmt.Sprintf(
		"SELECT Id, DeveloperName, MasterLabel, ExpirationDate, OptionsIsPrivateKeyExportable, KeySize "+
			"FROM Certificate WHERE DeveloperName = '%s'",
		escapeSoqlString(developerName),
	)

	var result struct {
		Records []struct {
			ID                            string `json:"Id"`
			DeveloperName                 string `json:"DeveloperName"`
			MasterLabel                   string `json:"MasterLabel"`
			ExpirationDate                string `json:"ExpirationDate"`
			OptionsIsPrivateKeyExportable bool   `json:"OptionsIsPrivateKeyExportable"`
			KeySize                       int64  `json:"KeySize"`
		} `json:"records"`
		TotalSize int `json:"totalSize"`
	}

	err := d.client.ToolingQuery(ctx, soql, &result)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Certificate",
			fmt.Sprintf("Unable to read certificate %q: %s", developerName, err),
		)
		return
	}

	if result.TotalSize == 0 {
		resp.Diagnostics.AddError(
			"Certificate Not Found",
			fmt.Sprintf("Certificate with DeveloperName %q not found.", developerName),
		)
		return
	}

	if result.TotalSize > 1 {
		resp.Diagnostics.AddError(
			"Multiple Certificates Found",
			fmt.Sprintf("Expected 1 certificate with DeveloperName %q, found %d.", developerName, result.TotalSize),
		)
		return
	}

	cert := result.Records[0]
	data.ID = types.StringValue(cert.ID)
	data.DeveloperName = types.StringValue(cert.DeveloperName)
	data.MasterLabel = types.StringValue(cert.MasterLabel)
	data.ExpirationDate = types.StringValue(cert.ExpirationDate)
	data.IsPrivateKeyExportable = types.BoolValue(cert.OptionsIsPrivateKeyExportable)
	data.KeySize = types.Int64Value(cert.KeySize)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// escapeSoqlString escapes special characters in a string for use in SOQL.
func escapeSoqlString(s string) string {
	// Escape single quotes by doubling them
	result := ""
	for _, r := range s {
		if r == '\'' {
			result += "''"
		} else if r == '\\' {
			result += "\\\\"
		} else {
			result += string(r)
		}
	}
	return result
}
