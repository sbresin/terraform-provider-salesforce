# Terraform Provider Salesforce

A lean Terraform provider for interacting with Salesforce APIs.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.22

## Building the Provider

```bash
go build -o terraform-provider-salesforce
```

## Using the Provider

### Authentication

The provider supports two authentication methods:

#### Option A: Client Credentials Flow

```hcl
provider "salesforce" {
  instance_url  = "https://example.my.salesforce.com"
  client_id     = var.salesforce_client_id
  client_secret = var.salesforce_client_secret
}
```

Or via environment variables:
- `SALESFORCE_INSTANCE_URL`
- `SALESFORCE_CLIENT_ID`
- `SALESFORCE_CLIENT_SECRET`

#### Option B: SFDX Auth URL

```hcl
provider "salesforce" {
  auth_url = var.salesforce_auth_url
}
```

Or via environment variable:
- `SALESFORCE_AUTH_URL`

Generate an auth URL using the Salesforce CLI:
```bash
sf org auth show-sfdx-auth-url -o <org-alias> --no-prompt
```

### Example Usage

```hcl
terraform {
  required_providers {
    salesforce = {
      source = "sbresin/salesforce"
    }
  }
}

provider "salesforce" {
  # Uses environment variables for authentication
}

data "salesforce_cert" "integration" {
  developer_name = "IntegrationCertificate"
}

output "cert_expiration" {
  value = data.salesforce_cert.integration.expiration_date
}
```

## Development

### Running Tests

```bash
# Unit tests
make test

# Acceptance tests (requires Salesforce credentials)
TF_ACC=1 make testacc
```

### Linting

```bash
make lint
```

## License

[MPL-2.0](LICENSE)
