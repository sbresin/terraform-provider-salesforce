resource "salesforce_self_signed_cert" "signing" {
  developer_name = "IntegrationSigningCert"
  master_label   = "Integration Signing Cert"

  key_size               = 2048
  private_key_exportable = false

  # Replace the certificate 30 days before it expires.
  early_renewal_hours = 720
}

# Public certificate for services that verify signatures made by Salesforce.
output "cert_pem" {
  value = salesforce_self_signed_cert.signing.cert_pem
}

# JWKS document, e.g. to serve at a jwks_uri.
output "jwks" {
  value = jsonencode({
    keys = [jsondecode(salesforce_self_signed_cert.signing.jwk)]
  })
}
