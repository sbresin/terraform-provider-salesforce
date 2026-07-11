package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &SelfSignedCertResource{}
var _ resource.ResourceWithConfigure = &SelfSignedCertResource{}
var _ resource.ResourceWithModifyPlan = &SelfSignedCertResource{}

type SelfSignedCertResource struct {
	clients *ProviderClients
}

type selfSignedCertResourceModel struct {
	ID                   types.String `tfsdk:"id"`
	DeveloperName        types.String `tfsdk:"developer_name"`
	MasterLabel          types.String `tfsdk:"master_label"`
	KeySize              types.Int64  `tfsdk:"key_size"`
	PrivateKeyExportable types.Bool   `tfsdk:"private_key_exportable"`
	EarlyRenewalHours    types.Int64  `tfsdk:"early_renewal_hours"`
	CertPEM              types.String `tfsdk:"cert_pem"`
	CertSHA256           types.String `tfsdk:"cert_sha256"`
	ExpirationDate       types.String `tfsdk:"expiration_date"`
	ReadyForRenewal      types.Bool   `tfsdk:"ready_for_renewal"`
	Platform             types.Bool   `tfsdk:"platform"`
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
			},
			"cert_pem":          schema.StringAttribute{Computed: true},
			"cert_sha256":       schema.StringAttribute{Computed: true},
			"expiration_date":   schema.StringAttribute{Computed: true},
			"ready_for_renewal": schema.BoolAttribute{Computed: true},
			"platform":          schema.BoolAttribute{Computed: true},
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
	r.clients = clients
}

func (r *SelfSignedCertResource) Create(_ context.Context, _ resource.CreateRequest, resp *resource.CreateResponse) {
	resp.Diagnostics.AddError("Not Implemented", "Create for salesforce_self_signed_cert will be implemented in a later task.")
}

func (r *SelfSignedCertResource) Read(_ context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	resp.Diagnostics.AddError("Not Implemented", "Read for salesforce_self_signed_cert will be implemented in a later task.")
}

func (r *SelfSignedCertResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Not Implemented", "Update for salesforce_self_signed_cert will be implemented in a later task.")
}

func (r *SelfSignedCertResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddError("Not Implemented", "Delete for salesforce_self_signed_cert will be implemented in a later task.")
}

func (r *SelfSignedCertResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	var state selfSignedCertResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || state.ID.IsNull() || state.ID.IsUnknown() {
		return
	}

	if state.ExpirationDate.IsNull() || state.ExpirationDate.IsUnknown() {
		return
	}

	early := int64(0)
	if !state.EarlyRenewalHours.IsNull() && !state.EarlyRenewalHours.IsUnknown() {
		early = state.EarlyRenewalHours.ValueInt64()
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
