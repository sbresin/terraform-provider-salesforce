# Record of MySigningKey__mdt with a Text and a DateTime field.
resource "salesforce_custom_metadata_record" "signing_key" {
  type           = "MySigningKey"
  developer_name = "SigningKey_2026T3"
  label          = "SigningKey_2026T3"

  values = {
    Certificate__c = "SigningKey_2026T3"
    ValidFrom__c   = "2026-11-06T09:00:00Z"
  }
}
