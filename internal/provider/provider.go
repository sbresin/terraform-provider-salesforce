package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/sbresin/terraform-provider-salesforce/internal/client"
	"github.com/sbresin/terraform-provider-salesforce/internal/metadata"
	"github.com/sbresin/terraform-provider-salesforce/internal/uploader"
)

var _ provider.Provider = &SalesforceProvider{}

type SalesforceProvider struct {
	version string
}

type SalesforceProviderModel struct {
	InstanceURL  types.String `tfsdk:"instance_url"`
	ClientID     types.String `tfsdk:"client_id"`
	ClientSecret types.String `tfsdk:"client_secret"`
	AuthURL      types.String `tfsdk:"auth_url"`
	APIVersion   types.String `tfsdk:"api_version"`
}

// ProviderClients holds all API clients for resources and data sources.
type ProviderClients struct {
	REST     *client.Client
	Metadata *metadata.Client
	Uploader *uploader.Uploader
	Session  *client.SessionCredentials
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
		Attributes: map[string]schema.Attribute{
			"instance_url": schema.StringAttribute{
				Description: "The Salesforce instance URL (e.g., https://example.my.salesforce.com). " +
					"Can also be set via SALESFORCE_INSTANCE_URL environment variable. " +
					"Required for client_credentials authentication.",
				Optional: true,
			},
			"client_id": schema.StringAttribute{
				Description: "The OAuth client ID (Consumer Key) from the Connected App. " +
					"Can also be set via SALESFORCE_CLIENT_ID environment variable. " +
					"Required for client_credentials authentication.",
				Optional: true,
			},
			"client_secret": schema.StringAttribute{
				Description: "The OAuth client secret (Consumer Secret) from the Connected App. " +
					"Can also be set via SALESFORCE_CLIENT_SECRET environment variable. " +
					"Required for client_credentials authentication.",
				Optional:  true,
				Sensitive: true,
			},
			"auth_url": schema.StringAttribute{
				Description: "The sfdx auth URL (force://...) containing credentials. " +
					"Can also be set via SALESFORCE_AUTH_URL environment variable. " +
					"Use this OR the instance_url/client_id/client_secret combination.",
				Optional:  true,
				Sensitive: true,
			},
			"api_version": schema.StringAttribute{
				Description: "The Salesforce API version to use (e.g., v61.0). " +
					"Can also be set via SALESFORCE_API_VERSION environment variable. " +
					"Defaults to v61.0.",
				Optional: true,
			},
		},
	}
}

func (p *SalesforceProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config SalesforceProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Resolve values from environment variables if not set in config
	instanceURL := stringValueOrEnv(config.InstanceURL, "SALESFORCE_INSTANCE_URL")
	clientID := stringValueOrEnv(config.ClientID, "SALESFORCE_CLIENT_ID")
	clientSecret := stringValueOrEnv(config.ClientSecret, "SALESFORCE_CLIENT_SECRET")
	authURL := stringValueOrEnv(config.AuthURL, "SALESFORCE_AUTH_URL")
	apiVersion := stringValueOrEnv(config.APIVersion, "SALESFORCE_API_VERSION")

	if apiVersion == "" {
		apiVersion = "v61.0"
	}

	// Determine auth mode
	hasClientCreds := instanceURL != "" || clientID != "" || clientSecret != ""
	hasAuthURL := authURL != ""

	if hasClientCreds && hasAuthURL {
		resp.Diagnostics.AddAttributeError(
			path.Root("auth_url"),
			"Conflicting Authentication Configuration",
			"Cannot use both auth_url and instance_url/client_id/client_secret. Choose one authentication method.",
		)
		return
	}

	if !hasClientCreds && !hasAuthURL {
		resp.Diagnostics.AddError(
			"Missing Authentication Configuration",
			"Either auth_url or all of instance_url, client_id, and client_secret must be configured. "+
				"Set them in the provider configuration or via environment variables "+
				"(SALESFORCE_AUTH_URL or SALESFORCE_INSTANCE_URL/SALESFORCE_CLIENT_ID/SALESFORCE_CLIENT_SECRET).",
		)
		return
	}

	var tokenSource client.TokenSource
	var resolvedInstanceURL string

	if hasAuthURL {
		// Parse auth URL and create refresh token source
		creds, err := client.ParseAuthURL(authURL)
		if err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("auth_url"),
				"Invalid Auth URL",
				"Failed to parse auth_url: "+err.Error(),
			)
			return
		}
		resolvedInstanceURL = creds.InstanceURL
		tokenSource = client.NewRefreshTokenSource(
			creds.InstanceURL,
			creds.ClientID,
			creds.ClientSecret,
			creds.RefreshToken,
		)
	} else {
		// Validate all client_credentials fields are present
		if instanceURL == "" {
			resp.Diagnostics.AddAttributeError(
				path.Root("instance_url"),
				"Missing Instance URL",
				"instance_url is required when using client_credentials authentication.",
			)
		}
		if clientID == "" {
			resp.Diagnostics.AddAttributeError(
				path.Root("client_id"),
				"Missing Client ID",
				"client_id is required when using client_credentials authentication.",
			)
		}
		if clientSecret == "" {
			resp.Diagnostics.AddAttributeError(
				path.Root("client_secret"),
				"Missing Client Secret",
				"client_secret is required when using client_credentials authentication.",
			)
		}
		if resp.Diagnostics.HasError() {
			return
		}

		resolvedInstanceURL = instanceURL
		tokenSource = client.NewClientCredentialsTokenSource(instanceURL, clientID, clientSecret)
	}

	// Create the REST client
	restClient := client.NewClient(client.Config{
		InstanceURL: resolvedInstanceURL,
		APIVersion:  apiVersion,
		TokenSource: tokenSource,
	})

	// Create session source for UI/SOAP operations
	sessionSource := client.NewSessionSource(tokenSource, resolvedInstanceURL)

	// Get session credentials
	session, err := sessionSource.Session(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Session Establishment Failed",
			"Unable to establish Salesforce session: "+err.Error(),
		)
		return
	}

	// Create metadata client
	// Use "67.0" for metadata API (no "v" prefix, different from REST API)
	metadataAPIVersion := "67.0"
	metaClient := metadata.NewClient(
		session.InstanceURL,
		session.SessionID,
		metadata.WithAPIVersion(metadataAPIVersion),
	)

	// Create uploader for certificate uploads
	certUploader := uploader.NewUploader(session.InstanceURL, session.Cookies)

	// Make clients available to data sources and resources
	clients := &ProviderClients{
		REST:     restClient,
		Metadata: metaClient,
		Uploader: certUploader,
		Session:  session,
	}
	resp.DataSourceData = clients
	resp.ResourceData = clients
}

func (p *SalesforceProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewCertificateDataSource,
	}
}

func (p *SalesforceProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{}
}

// stringValueOrEnv returns the string value if not null/unknown, otherwise the env var value.
func stringValueOrEnv(v types.String, envVar string) string {
	if !v.IsNull() && !v.IsUnknown() {
		return v.ValueString()
	}
	return os.Getenv(envVar)
}
