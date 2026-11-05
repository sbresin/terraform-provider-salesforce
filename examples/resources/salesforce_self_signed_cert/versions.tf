terraform {
  required_providers {
    salesforce = {
      source = "sbresin/salesforce"
    }
  }
}

# Credentials from SALESFORCE_AUTH_URL or
# SALESFORCE_INSTANCE_URL / SALESFORCE_CLIENT_ID / SALESFORCE_CLIENT_SECRET.
provider "salesforce" {}
