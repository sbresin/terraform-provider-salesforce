# E2E Lifecycle Test for Salesforce Provider

This document describes how to manually verify the full CRUD lifecycle of the Terraform Salesforce provider against a real Salesforce org.

## Prerequisites

1. **Salesforce CLI** (`sf`) installed and authenticated to the target org
2. **Go** installed (to build the provider)
3. **Terraform** installed

## Setup

### 1. Get the Auth URL

```bash
sf org auth show-sfdx-auth-url -o <org-alias> --no-prompt
```

Extract the `sfdxAuthUrl` value from the JSON response. It looks like:
```
force://PlatformCLI::<refresh-token>@<instance>.my.salesforce.com
```

### 2. Build the Provider

```bash
cd ~/workspace/terraform-provider-salesforce
go build -o terraform-provider-salesforce
```

### 3. Set Environment Variables

```bash
export SALESFORCE_AUTH_URL="force://PlatformCLI::...<your-auth-url>..."
```

### 4. Create Terraform Config Override

Create a temporary config file to use the local provider build:

```bash
cat > /tmp/tf-dev-override.tfrc <<'EOF'
provider_installation {
  dev_overrides {
    "sbresin/salesforce" = "/home/sebe/workspace/terraform-provider-salesforce"
  }
  direct {}
}
EOF
```

## Test Procedure

All commands should be run from:
```bash
cd ~/workspace/terraform-provider-salesforce/examples/resources/salesforce_self_signed_cert
```

### 1. Plan (Verify Provider Connects)

```bash
TF_CLI_CONFIG_FILE=/tmp/tf-dev-override.tfrc terraform plan
```

**Expected**: Shows plan to create `salesforce_self_signed_cert.signing`

### 2. Apply (Create)

```bash
TF_CLI_CONFIG_FILE=/tmp/tf-dev-override.tfrc terraform apply -auto-approve
```

**Expected**:
- Certificate created in ~2-10 seconds
- Outputs show `cert_pem`, `cert_expiration_date`, `ready_for_renewal`
- State file created

### 3. Plan Again (Verify Idempotency)

```bash
TF_CLI_CONFIG_FILE=/tmp/tf-dev-override.tfrc terraform plan
```

**Expected**: "No changes. Your infrastructure matches the configuration."

### 4. Show State (Verify Read)

```bash
TF_CLI_CONFIG_FILE=/tmp/tf-dev-override.tfrc terraform show
```

**Expected**: Shows all attributes including:
- `cert_pem` with full PEM certificate
- `cert_sha256` with fingerprint
- `expiration_date` (1 year from creation)
- `ready_for_renewal = false`

### 5. Destroy (Delete)

```bash
TF_CLI_CONFIG_FILE=/tmp/tf-dev-override.tfrc terraform destroy -auto-approve
```

**Expected**: Certificate deleted, "Destroy complete! Resources: 1 destroyed."

### 6. Apply Again (Re-create for Import Test)

```bash
TF_CLI_CONFIG_FILE=/tmp/tf-dev-override.tfrc terraform apply -auto-approve
```

### 7. Remove from State (Prepare for Import)

```bash
TF_CLI_CONFIG_FILE=/tmp/tf-dev-override.tfrc terraform state rm salesforce_self_signed_cert.signing
```

### 8. Import

```bash
TF_CLI_CONFIG_FILE=/tmp/tf-dev-override.tfrc terraform import salesforce_self_signed_cert.signing TerraformE2ESigningCert
```

**Expected**: "Import successful!"

### 9. Plan After Import

```bash
TF_CLI_CONFIG_FILE=/tmp/tf-dev-override.tfrc terraform plan
```

**Expected**: Only change should be `early_renewal_hours: 0 -> 720` (Terraform-only attribute defaults to 0 on import)

### 10. Final Cleanup

```bash
TF_CLI_CONFIG_FILE=/tmp/tf-dev-override.tfrc terraform destroy -auto-approve
```

## Quick One-Liner Test

For a fast smoke test, run this from the provider repo root:

```bash
cd ~/workspace/terraform-provider-salesforce && \
go build -o terraform-provider-salesforce && \
cd examples/resources/salesforce_self_signed_cert && \
SALESFORCE_AUTH_URL="$(sf org auth show-sfdx-auth-url -o crm-sebastian --no-prompt --json | jq -r .result.sfdxAuthUrl)" \
TF_CLI_CONFIG_FILE=/dev/stdin terraform apply -auto-approve <<'EOF'
provider_installation {
  dev_overrides {
    "sbresin/salesforce" = "/home/sebe/workspace/terraform-provider-salesforce"
  }
  direct {}
}
EOF
```

## Checklist

| Step | Operation | Expected Result |
|------|-----------|-----------------|
| 1 | Plan | Shows 1 to add |
| 2 | Apply | Creates cert, shows outputs |
| 3 | Plan | No changes |
| 4 | Show | All attributes populated |
| 5 | Destroy | Deletes cert |
| 6 | Apply | Re-creates cert |
| 7 | State rm | Removed from state |
| 8 | Import | Import successful |
| 9 | Plan | Only early_renewal_hours diff |
| 10 | Destroy | Final cleanup |

## Troubleshooting

### "certificate content is empty"
The Salesforce API returns certificate content in a field called `content`, not `certificate`. Ensure the provider code uses the correct XML tag.

### "was not found in zipped directory"
The metadata file extension should be `.crt-meta.xml`, not `.cert-meta.xml`.

### Auth failures
Verify the auth URL is valid by running:
```bash
sf org display -o <org-alias>
```
