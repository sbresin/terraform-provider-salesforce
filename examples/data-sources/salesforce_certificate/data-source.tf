# Retrieve information about a Salesforce certificate
data "salesforce_certificate" "integration" {
  developer_name = "IntegrationCertificate"
}

output "certificate_id" {
  description = "The Salesforce ID of the certificate"
  value       = data.salesforce_certificate.integration.id
}

output "certificate_expiration" {
  description = "When the certificate expires"
  value       = data.salesforce_certificate.integration.expiration_date
}

output "certificate_key_size" {
  description = "The key size in bits"
  value       = data.salesforce_certificate.integration.key_size
}
