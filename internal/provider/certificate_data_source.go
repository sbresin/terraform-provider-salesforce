package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/sbresin/terraform-provider-salesforce/internal/metadata"
)

var _ datasource.DataSource = &CertificateDataSource{}
var _ datasource.DataSourceWithConfigure = &CertificateDataSource{}

type CertificateDataSource struct {
	clients *ProviderClients
}

type CertificateDataSourceModel struct {
	ID                     types.String `tfsdk:"id"`
	DeveloperName          types.String `tfsdk:"developer_name"`
	MasterLabel            types.String `tfsdk:"master_label"`
	ExpirationDate         types.String `tfsdk:"expiration_date"`
	IsPrivateKeyExportable types.Bool   `tfsdk:"is_private_key_exportable"`
	KeySize                types.Int64  `tfsdk:"key_size"`
	Certificate            types.String `tfsdk:"certificate"`
	Platform               types.Bool   `tfsdk:"platform"`
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
			"certificate": schema.StringAttribute{
				Description: "The certificate chain in base64-encoded PEM format.",
				Computed:    true,
			},
			"platform": schema.BoolAttribute{
				Description: "Whether this is a platform certificate.",
				Computed:    true,
			},
		},
	}
}

func (d *CertificateDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	clients, ok := req.ProviderData.(*ProviderClients)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *ProviderClients, got: %T", req.ProviderData),
		)
		return
	}

	d.clients = clients
}

func (d *CertificateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data CertificateDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	developerName := data.DeveloperName.ValueString()

	// Use metadata client to read certificate
	cert, err := d.clients.Metadata.ReadCertificate(ctx, developerName)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			resp.Diagnostics.AddError(
				"Certificate Not Found",
				fmt.Sprintf("Certificate with DeveloperName %q not found.", developerName),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading Certificate",
			fmt.Sprintf("Unable to read certificate %q: %s", developerName, err),
		)
		return
	}

	// Map certificate to model
	data.ID = types.StringValue(cert.FullName)
	data.DeveloperName = types.StringValue(cert.FullName)
	data.MasterLabel = types.StringValue(cert.MasterLabel)
	data.Certificate = types.StringValue(cert.Certificate)
	data.KeySize = types.Int64Value(int64(cert.KeySize))
	data.IsPrivateKeyExportable = types.BoolValue(cert.PrivateKeyExportable)
	data.Platform = types.BoolValue(cert.Platform)

	if cert.ExpirationDate != nil {
		data.ExpirationDate = types.StringValue(cert.ExpirationDate.Format("2006-01-02"))
	} else {
		data.ExpirationDate = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
