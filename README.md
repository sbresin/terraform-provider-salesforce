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

resource "salesforce_self_signed_cert" "integration" {
  developer_name = "IntegrationCertificate"
  master_label   = "Integration Certificate"

  key_size               = 2048
  private_key_exportable = false
  early_renewal_hours    = 720
}

output "cert_pem" {
  value = salesforce_self_signed_cert.integration.cert_pem
}
```

`salesforce_self_signed_cert` creates a Salesforce-generated key pair and certificate. Salesforce owns the private key. The `cert_pem` output contains only the public certificate and can be shared with services that verify signatures created by Salesforce.

`jwk` is the public key as an RS256 JSON Web Key with `kid` = `developer_name`. Publish one or more as a JWKS:

```hcl
locals {
  jwks = jsonencode({ keys = [jsondecode(salesforce_self_signed_cert.integration.jwk)] })
}
```

Once `early_renewal_hours` before expiry, the next apply replaces the certificate under the same `developer_name` (same `kid`, new key) and everything derived from `jwk` follows in the same apply.

Without credentials the provider still configures; only using a resource or data source fails. That lets a module use it in some environments only (`count = 0` elsewhere).

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
