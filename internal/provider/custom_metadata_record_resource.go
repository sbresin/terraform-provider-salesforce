package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/sbresin/terraform-provider-salesforce/internal/metadata"
)

var _ resource.Resource = &CustomMetadataRecordResource{}
var _ resource.ResourceWithConfigure = &CustomMetadataRecordResource{}
var _ resource.ResourceWithImportState = &CustomMetadataRecordResource{}
var _ resource.ResourceWithModifyPlan = &CustomMetadataRecordResource{}

var (
	apiNameRegexp       = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
	developerNameRegexp = regexp.MustCompile(`^[A-Za-z](?:[A-Za-z0-9]|_[A-Za-z0-9])*$`)
)

type CustomMetadataRecordResource struct {
	clients *ProviderClients
}

type customMetadataRecordResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Type          types.String `tfsdk:"type"`
	DeveloperName types.String `tfsdk:"developer_name"`
	Label         types.String `tfsdk:"label"`
	Protected     types.Bool   `tfsdk:"protected"`
	Values        types.Map    `tfsdk:"values"`
}

func NewCustomMetadataRecordResource() resource.Resource {
	return &CustomMetadataRecordResource{}
}

func (r *CustomMetadataRecordResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_custom_metadata_record"
}

func (r *CustomMetadataRecordResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Custom Metadata record (any Custom Metadata Type) through the Metadata API.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Metadata fullName: <type>.<developer_name>.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"type": schema.StringAttribute{
				Description: "API name of the Custom Metadata Type without the __mdt suffix, e.g. MyType for MyType__mdt. Changing it forces a new record.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(apiNameRegexp, "must be a Salesforce API name"),
					noMdtSuffix{},
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"developer_name": schema.StringAttribute{
				Description: "Record name (DeveloperName): starts with a letter, only letters, digits and single underscores, no trailing underscore, at most 40 characters. Changing it forces a new record.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(40),
					stringvalidator.RegexMatches(developerNameRegexp,
						"must start with a letter and contain only letters, digits and single underscores, not ending with an underscore"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"label": schema.StringAttribute{
				Description: "Record label, at most 40 characters.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 40),
				},
			},
			"protected": schema.BoolAttribute{
				Description: "Whether the record is protected. Defaults to false.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"values": schema.MapAttribute{
				Description: "Field API name -> value, e.g. { Certificate__c = \"MyCert\", ValidFrom__c = \"2026-11-06T09:00:00Z\" }. " +
					"Write Checkbox as true/false, Date as YYYY-MM-DD and DateTime as RFC 3339. " +
					"The record is owned as a whole: fields not listed are cleared, also when set outside Terraform.",
				Optional:    true,
				ElementType: types.StringType,
				Validators: []validator.Map{
					mapvalidator.NoNullValues(),
					mapvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
				},
			},
		},
	}
}

func (r *CustomMetadataRecordResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *CustomMetadataRecordResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan customMetadataRecordResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.deployAndRead(ctx, plan, &resp.State)...)
}

func (r *CustomMetadataRecordResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state customMetadataRecordResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	newState, err := r.readState(ctx, state.Type.ValueString(), state.DeveloperName.ValueString(), state.Values)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Custom Metadata Record", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, newState)...)
}

func (r *CustomMetadataRecordResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan customMetadataRecordResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.deployAndRead(ctx, plan, &resp.State)...)
}

func (r *CustomMetadataRecordResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state customMetadataRecordResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deployID, err := r.clients.Metadata.DeleteCustomMetadata(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error Deleting Custom Metadata Record", err.Error())
		return
	}
	if _, err := r.clients.Metadata.WaitForDeploy(ctx, deployID); err != nil {
		// A destructive deploy of a record that is already gone fails ("No CustomMetadata named ... found").
		if _, readErr := r.clients.Metadata.ReadCustomMetadata(ctx, state.ID.ValueString()); errors.Is(readErr, metadata.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError("Custom Metadata Record Delete Deployment Failed", err.Error())
	}
}

// ImportState takes <type>.<developer_name>; Read then fills in the record.
func (r *CustomMetadataRecordResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	cmdtType, devName, ok := strings.Cut(req.ID, ".")
	if !ok || cmdtType == "" || devName == "" {
		resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("Expected <type>.<developer_name>, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("type"), cmdtType)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("developer_name"), devName)...)
}

// ModifyPlan fails early on an unknown type or field. It looks up the field types only when the
// type or the set of keys in values changes; the client caches the lookup per type.
func (r *CustomMetadataRecordResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || r.clients == nil {
		return
	}
	var plan customMetadataRecordResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || plan.Type.IsUnknown() || plan.Values.IsUnknown() {
		return
	}

	keys := map[string]bool{}
	for k := range plan.Values.Elements() {
		keys[k] = true
	}
	if !req.State.Raw.IsNull() {
		var state customMetadataRecordResourceModel
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
		same := state.Type.Equal(plan.Type) && len(state.Values.Elements()) == len(keys)
		for k := range state.Values.Elements() {
			same = same && keys[k]
		}
		if same {
			return
		}
	}

	fieldTypes, err := r.clients.Metadata.CustomMetadataFieldTypes(ctx, plan.Type.ValueString())
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			resp.Diagnostics.AddAttributeError(path.Root("type"), "Unknown Custom Metadata Type", err.Error())
			return
		}
		resp.Diagnostics.AddError("Error Reading Custom Metadata Type", err.Error())
		return
	}
	var unknown, known []string
	for k := range keys {
		if _, ok := fieldTypes[k]; !ok {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 0 {
		return
	}
	for f := range fieldTypes {
		known = append(known, f)
	}
	sort.Strings(unknown)
	sort.Strings(known)
	resp.Diagnostics.AddAttributeError(path.Root("values"), "Unknown Custom Metadata Field",
		fmt.Sprintf("%s__mdt has no field %s. Fields: %s.",
			plan.Type.ValueString(), strings.Join(unknown, ", "), strings.Join(known, ", ")))
}

func (r *CustomMetadataRecordResource) deployAndRead(ctx context.Context, plan customMetadataRecordResourceModel, out interface {
	Set(context.Context, interface{}) diag.Diagnostics
}) diag.Diagnostics {
	var diags diag.Diagnostics
	values, d := stringMap(ctx, plan.Values)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}

	deployID, err := r.clients.Metadata.DeployCustomMetadata(ctx, metadata.CustomMetadataDeploy{
		Type:          plan.Type.ValueString(),
		DeveloperName: plan.DeveloperName.ValueString(),
		Label:         plan.Label.ValueString(),
		Protected:     plan.Protected.ValueBool(),
		Values:        values,
	})
	if err != nil {
		diags.AddError("Error Deploying Custom Metadata Record", err.Error())
		return diags
	}
	if _, err := r.clients.Metadata.WaitForDeploy(ctx, deployID); err != nil {
		diags.AddError("Custom Metadata Record Deployment Failed", err.Error())
		return diags
	}

	state, err := r.readState(ctx, plan.Type.ValueString(), plan.DeveloperName.ValueString(), plan.Values)
	if err != nil {
		diags.AddError("Error Reading Custom Metadata Record", err.Error())
		return diags
	}
	diags.Append(out.Set(ctx, state)...)
	return diags
}

// readState reads the record. The record is owned as a whole, so every non-null field goes
// into values, keeping the prior form when the org returns an equivalent one
// (2026-11-06T09:00:00Z vs 2026-11-06T09:00:00.000Z, 7 vs 7.0). An unset Checkbox reads back
// as false, so a false Checkbox only appears when the prior state has it.
func (r *CustomMetadataRecordResource) readState(ctx context.Context, cmdtType, devName string, prior types.Map) (*customMetadataRecordResourceModel, error) {
	fullName := cmdtType + "." + devName
	rec, err := r.clients.Metadata.ReadCustomMetadata(ctx, fullName)
	if err != nil {
		return nil, err
	}

	priorValues, diags := stringMap(ctx, prior)
	out := map[string]string{}
	for f, remote := range rec.Values {
		want, inPrior := priorValues[f]
		switch {
		case inPrior && metadata.SameValue(remote.Type, want, remote.Value):
			out[f] = want
		case remote.Type == "xsd:boolean" && remote.Value == "false":
		default:
			out[f] = remote.Value
		}
	}
	values := types.MapNull(types.StringType)
	if !prior.IsNull() || len(out) > 0 {
		var d diag.Diagnostics
		values, d = types.MapValueFrom(ctx, types.StringType, out)
		diags.Append(d...)
	}
	if diags.HasError() {
		return nil, fmt.Errorf("converting values: %v", diags)
	}

	return &customMetadataRecordResourceModel{
		ID:            types.StringValue(fullName),
		Type:          types.StringValue(cmdtType),
		DeveloperName: types.StringValue(devName),
		Label:         types.StringValue(rec.Label),
		Protected:     types.BoolValue(rec.Protected),
		Values:        values,
	}, nil
}

func stringMap(ctx context.Context, m types.Map) (map[string]string, diag.Diagnostics) {
	out := map[string]string{}
	if m.IsNull() || m.IsUnknown() {
		return out, nil
	}
	diags := m.ElementsAs(ctx, &out, false)
	return out, diags
}

type noMdtSuffix struct{}

func (noMdtSuffix) Description(context.Context) string {
	return "must be given without the __mdt suffix"
}

func (v noMdtSuffix) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (noMdtSuffix) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if v := req.ConfigValue.ValueString(); strings.HasSuffix(strings.ToLower(v), "__mdt") {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid Custom Metadata Type",
			fmt.Sprintf("type must be the API name without the __mdt suffix: use %q instead of %q", v[:len(v)-5], v))
	}
}
