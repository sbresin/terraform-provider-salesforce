terraform {
  required_providers {
    salesforce = {
      source = "sbresin/salesforce"
    }
  }
}

# Option A: Client credentials authentication
provider "salesforce" {
  instance_url  = "https://example.my.salesforce.com"
  client_id     = var.salesforce_client_id
  client_secret = var.salesforce_client_secret
}

# Option B: SFDX auth URL authentication (uncomment to use)
# provider "salesforce" {
#   auth_url = var.salesforce_auth_url
# }

variable "salesforce_client_id" {
  description = "Salesforce OAuth client ID"
  type        = string
  sensitive   = true
  default     = ""
}

variable "salesforce_client_secret" {
  description = "Salesforce OAuth client secret"
  type        = string
  sensitive   = true
  default     = ""
}

variable "salesforce_auth_url" {
  description = "Salesforce SFDX auth URL"
  type        = string
  sensitive   = true
  default     = ""
}
