package provider

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/sbresin/terraform-provider-salesforce/internal/certutil"
	"github.com/sbresin/terraform-provider-salesforce/internal/metadata"
)

var _ resource.Resource = &SelfSignedCertResource{}
var _ resource.ResourceWithConfigure = &SelfSignedCertResource{}
var _ resource.ResourceWithModifyPlan = &SelfSignedCertResource{}
var _ resource.ResourceWithImportState = &SelfSignedCertResource{}

type SelfSignedCertResource struct {
	clients *ProviderClients
}

type selfSignedCertResourceModel struct {
	ID                              types.String `tfsdk:"id"`
	DeveloperName                   types.String `tfsdk:"developer_name"`
	MasterLabel                     types.String `tfsdk:"master_label"`
	KeySize                         types.Int64  `tfsdk:"key_size"`
	PrivateKeyExportable            types.Bool   `tfsdk:"private_key_exportable"`
	EarlyRenewalHours               types.Int64  `tfsdk:"early_renewal_hours"`
	CertPEM                         types.String `tfsdk:"cert_pem"`
	JWK                             types.String `tfsdk:"jwk"`
	CertSHA256                      types.String `tfsdk:"cert_sha256"`
	ExpirationDate                  types.String `tfsdk:"expiration_date"`
	ReadyForRenewal                 types.Bool   `tfsdk:"ready_for_renewal"`
	EncryptedWithPlatformEncryption types.Bool   `tfsdk:"encrypted_with_platform_encryption"`
}

func NewSelfSignedCertResource() resource.Resource {
	return &SelfSignedCertResource{}
}

func (r *SelfSignedCertResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_self_signed_cert"
}

func (r *SelfSignedCertResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Salesforce-generated self-signed certificate. Salesforce owns the private key; Terraform exposes only public certificate metadata.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true},
			"developer_name": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"master_label": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"key_size": schema.Int64Attribute{
				Optional: true,
				Computed: true,
				Default:  int64default.StaticInt64(2048),
				Validators: []validator.Int64{
					int64validator.OneOf(2048, 4096),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"private_key_exportable": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"early_renewal_hours": schema.Int64Attribute{
				Optional: true,
				Computed: true,
				Default:  int64default.StaticInt64(0),
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"cert_pem":    schema.StringAttribute{Computed: true},
			"cert_sha256": schema.StringAttribute{Computed: true},
			"jwk": schema.StringAttribute{
				Computed:    true,
				Description: "The public key as an RS256 JSON Web Key (JSON string), kid = developer_name. Collect several into a JWKS to publish them side by side.",
			},
			"expiration_date":                    schema.StringAttribute{Computed: true},
			"ready_for_renewal":                  schema.BoolAttribute{Computed: true},
			"encrypted_with_platform_encryption": schema.BoolAttribute{Computed: true},
		},
	}
}

func (r *SelfSignedCertResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	clients, ok := req.ProviderData.(*ProviderClients)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *ProviderClients, got: %T", req.ProviderData))
		return
	}
	if clients.configured(&resp.Diagnostics) {
		r.clients = clients
	}
}

func (r *SelfSignedCertResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan selfSignedCertResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	masterLabel := plan.MasterLabel.ValueString()
	if masterLabel == "" {
		masterLabel = plan.DeveloperName.ValueString()
	}

	deployID, err := r.clients.Metadata.CreateCertificate(ctx, metadata.CertificateCreate{
		FullName:             plan.DeveloperName.ValueString(),
		MasterLabel:          masterLabel,
		KeySize:              int(plan.KeySize.ValueInt64()),
		PrivateKeyExportable: plan.PrivateKeyExportable.ValueBool(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Certificate", err.Error())
		return
	}
	if _, err := r.clients.Metadata.WaitForDeploy(ctx, deployID); err != nil {
		resp.Diagnostics.AddError("Certificate Deployment Failed", err.Error())
		return
	}

	// Salesforce generates the certificate asynchronously after deploy.
	// Retry reading until the certificate content is available.
	var state *selfSignedCertResourceModel
	for attempt := 0; attempt < 10; attempt++ {
		state, err = r.readState(ctx, plan.DeveloperName.ValueString(), plan.EarlyRenewalHours.ValueInt64())
		if err == nil {
			break
		}
		// Wait before retrying (500ms, 1s, 1.5s, ...)
		select {
		case <-ctx.Done():
			resp.Diagnostics.AddError("Error Reading Created Certificate", ctx.Err().Error())
			return
		case <-time.After(time.Duration(500*(attempt+1)) * time.Millisecond):
		}
	}
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Created Certificate", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *SelfSignedCertResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state selfSignedCertResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	newState, err := r.readState(ctx, state.DeveloperName.ValueString(), state.EarlyRenewalHours.ValueInt64())
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Certificate", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, newState)...)
}

func (r *SelfSignedCertResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// The only mutable attribute without RequiresReplace is early_renewal_hours.
	// This attribute is purely Terraform-side state; changing it does not require
	// any Salesforce API call. We simply read the planned value and refresh state.
	var plan selfSignedCertResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	state, err := r.readState(ctx, plan.DeveloperName.ValueString(), plan.EarlyRenewalHours.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Certificate During Update", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *SelfSignedCertResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state selfSignedCertResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deployID, err := r.clients.Metadata.DeleteCertificate(ctx, state.DeveloperName.ValueString())
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError("Error Deleting Certificate", err.Error())
		return
	}
	if _, err := r.clients.Metadata.WaitForDeploy(ctx, deployID); err != nil {
		resp.Diagnostics.AddError("Certificate Delete Deployment Failed", err.Error())
		return
	}
}

func (r *SelfSignedCertResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("developer_name"), req, resp)
}

func (r *SelfSignedCertResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Skip if creating (no prior state) or destroying (no plan)
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var state selfSignedCertResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || state.ID.IsNull() || state.ID.IsUnknown() {
		return
	}

	if state.ExpirationDate.IsNull() || state.ExpirationDate.IsUnknown() {
		return
	}

	// Read planned early_renewal_hours to check against the renewal window.
	// This ensures that increasing early_renewal_hours into the window triggers replacement.
	var plan selfSignedCertResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	early := int64(0)
	if !plan.EarlyRenewalHours.IsNull() && !plan.EarlyRenewalHours.IsUnknown() {
		early = plan.EarlyRenewalHours.ValueInt64()
	}

	ready, err := readyForRenewal(time.Now().UTC(), state.ExpirationDate.ValueString(), early)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Certificate Expiration Date", err.Error())
		return
	}
	if ready {
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root("expiration_date"))
	}
}

func readyForRenewal(now time.Time, expirationDate string, earlyRenewalHours int64) (bool, error) {
	expires, err := time.Parse("2006-01-02", expirationDate)
	if err != nil {
		return false, fmt.Errorf("parsing expiration date %q: %w", expirationDate, err)
	}
	renewalTime := expires.UTC().Add(-time.Duration(earlyRenewalHours) * time.Hour)
	return !now.Before(renewalTime), nil
}

func (r *SelfSignedCertResource) readState(ctx context.Context, developerName string, earlyRenewalHours int64) (*selfSignedCertResourceModel, error) {
	cert, err := r.clients.Metadata.ReadCertificate(ctx, developerName)
	if err != nil {
		return nil, err
	}

	certPEM, err := certutil.NormalizePEMFromSalesforce(cert.Certificate)
	if err != nil {
		return nil, err
	}
	sha256, err := certutil.LeafSHA256(certPEM)
	if err != nil {
		return nil, err
	}
	jwk, err := certutil.JWK(certPEM, cert.FullName)
	if err != nil {
		return nil, err
	}

	expirationDate := ""
	if cert.ExpirationDate != nil {
		expirationDate = cert.ExpirationDate.Format("2006-01-02")
	}
	ready := false
	if expirationDate != "" {
		ready, err = readyForRenewal(time.Now().UTC(), expirationDate, earlyRenewalHours)
		if err != nil {
			return nil, err
		}
	}

	return &selfSignedCertResourceModel{
		ID:                              types.StringValue(cert.FullName),
		DeveloperName:                   types.StringValue(cert.FullName),
		MasterLabel:                     types.StringValue(cert.MasterLabel),
		KeySize:                         types.Int64Value(int64(cert.KeySize)),
		PrivateKeyExportable:            types.BoolValue(cert.PrivateKeyExportable),
		EarlyRenewalHours:               types.Int64Value(earlyRenewalHours),
		CertPEM:                         types.StringValue(certPEM),
		CertSHA256:                      types.StringValue(sha256),
		JWK:                             types.StringValue(jwk),
		ExpirationDate:                  types.StringValue(expirationDate),
		ReadyForRenewal:                 types.BoolValue(ready),
		EncryptedWithPlatformEncryption: types.BoolValue(cert.EncryptedWithPlatformEncryption),
	}, nil
}
