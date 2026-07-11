# Salesforce Provider

The Salesforce provider allows Terraform to interact with Salesforce APIs.

## Authentication

The provider supports two mutually exclusive authentication methods:

### Client Credentials Flow

Use a Connected App with client credentials enabled:

```hcl
provider "salesforce" {
  instance_url  = "https://example.my.salesforce.com"
  client_id     = "your-consumer-key"
  client_secret = "your-consumer-secret"
  api_version   = "v61.0"  # optional
}
```

### SFDX Auth URL

Use an auth URL from the Salesforce CLI:

```hcl
provider "salesforce" {
  auth_url = "force://PlatformCLI::refresh-token@example.my.salesforce.com"
}
```

Generate with: `sf org auth show-sfdx-auth-url -o <alias> --no-prompt`

## Environment Variables

| Variable | Description |
|----------|-------------|
| `SALESFORCE_INSTANCE_URL` | Salesforce instance URL |
| `SALESFORCE_CLIENT_ID` | OAuth client ID |
| `SALESFORCE_CLIENT_SECRET` | OAuth client secret |
| `SALESFORCE_AUTH_URL` | SFDX auth URL |
| `SALESFORCE_API_VERSION` | API version (default: v61.0) |

## Data Sources

### salesforce_cert

Retrieves information about a Salesforce Certificate.

**Arguments:**
- `developer_name` (Required) - The developer name (API name) of the certificate

**Attributes:**
- `id` - The Salesforce ID of the certificate
- `master_label` - The human-readable label
- `expiration_date` - The expiration date (YYYY-MM-DD format)
- `is_private_key_exportable` - Whether the private key can be exported
- `key_size` - The key size in bits (e.g., 2048, 4096)

**Example:**

```hcl
data "salesforce_cert" "integration" {
  developer_name = "IntegrationCertificate"
}

output "cert_expiration" {
  value = data.salesforce_cert.integration.expiration_date
}
```

## Provider Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `instance_url` | String | Salesforce instance URL |
| `client_id` | String | OAuth client ID |
| `client_secret` | String | OAuth client secret (sensitive) |
| `auth_url` | String | SFDX auth URL (sensitive) |
| `api_version` | String | API version (default: v61.0) |
