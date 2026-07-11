terraform {
  required_providers {
    salesforce = {
      source = "sbresin/salesforce"
    }
  }
}

provider "salesforce" {
  # Uses SALESFORCE_AUTH_URL or:
  # SALESFORCE_INSTANCE_URL
  # SALESFORCE_CLIENT_ID
  # SALESFORCE_CLIENT_SECRET
}

resource "salesforce_self_signed_cert" "signing" {
  developer_name = "TerraformE2ESigningCert"
  master_label   = "Terraform E2E Signing Cert"

  key_size               = 2048
  private_key_exportable = false

  early_renewal_hours = 720
}

output "cert_pem" {
  description = "Public certificate PEM for verifying signatures made by Salesforce."
  value       = salesforce_self_signed_cert.signing.cert_pem
}

output "cert_expiration_date" {
  value = salesforce_self_signed_cert.signing.expiration_date
}

output "ready_for_renewal" {
  value = salesforce_self_signed_cert.signing.ready_for_renewal
}
