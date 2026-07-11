# Retrieve information about a Salesforce certificate
data "salesforce_cert" "integration" {
  developer_name = "IntegrationCertificate"
}

output "certificate_id" {
  description = "The Salesforce ID of the certificate"
  value       = data.salesforce_cert.integration.id
}

output "certificate_expiration" {
  description = "When the certificate expires"
  value       = data.salesforce_cert.integration.expiration_date
}

output "certificate_key_size" {
  description = "The key size in bits"
  value       = data.salesforce_cert.integration.key_size
}
