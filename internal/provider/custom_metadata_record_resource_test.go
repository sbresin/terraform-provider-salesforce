package provider

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// fakeCMDTOrg is an in-memory org holding records of Probe__mdt. Like a real org (see
// internal/metadata/custom_metadata.go) it merges deployed values, clears xsi:nil ones,
// reads datetimes back as .000Z, numbers as doubles and an unset Checkbox as false.
type fakeCMDTOrg struct {
	mu        sync.Mutex
	records   map[string]map[string]string // fullName -> field -> value
	labels    map[string]string
	lastXML   string
	deletes   int
	fieldType map[string]string
	// vanishOnDelete removes the record just before a destructive deploy, as if deleted
	// out of band after refresh; the deploy then fails like a real org's does.
	vanishOnDelete bool
	deployFailed   bool
}

func newFakeCMDTOrg() *fakeCMDTOrg {
	return &fakeCMDTOrg{
		records: map[string]map[string]string{},
		labels:  map[string]string{},
		fieldType: map[string]string{
			"Text__c": "Text", "When__c": "DateTime", "Amount__c": "Number", "Flag__c": "Checkbox",
		},
	}
}

func (o *fakeCMDTOrg) set(fullName, field, value string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.records[fullName][field] = value
}

func (o *fakeCMDTOrg) server(t *testing.T) *httptest.Server {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/services/oauth2/token":
			json.NewEncoder(w).Encode(map[string]string{"access_token": "test-token"})
		case r.URL.Path == "/services/oauth2/singleaccess":
			json.NewEncoder(w).Encode(map[string]string{"frontdoor_uri": serverURL + "/secur/frontdoor.jsp?otp=abc"})
		case r.URL.Path == "/secur/frontdoor.jsp":
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "test-session-id"})
		case strings.HasPrefix(r.URL.Path, "/services/Soap/m/"):
			b, _ := io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "text/xml")
			w.Write([]byte(soapEnvelope(o.handle(t, string(b)))))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	serverURL = server.URL
	return server
}

func soapEnvelope(body string) string {
	return `<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns="http://soap.sforce.com/2006/04/metadata" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema"><soapenv:Body>` + body + `</soapenv:Body></soapenv:Envelope>`
}

func (o *fakeCMDTOrg) handle(t *testing.T, body string) string {
	o.mu.Lock()
	defer o.mu.Unlock()
	var req struct {
		Type      string `xml:"Body>readMetadata>type"`
		FullNames string `xml:"Body>readMetadata>fullNames"`
	}
	_ = xml.Unmarshal([]byte(body), &req)

	switch {
	case strings.Contains(body, "checkDeployStatus") && o.deployFailed:
		o.deployFailed = false
		return `<checkDeployStatusResponse><result><id>0Af</id><done>true</done><success>false</success><status>Failed</status><details><componentFailures><fullName>Probe.One</fullName><problem>No CustomMetadata named: Probe.One found</problem><problemType>Warning</problemType><componentType>CustomMetadata</componentType></componentFailures></details></result></checkDeployStatusResponse>`
	case strings.Contains(body, "checkDeployStatus"):
		return `<checkDeployStatusResponse><result><id>0Af</id><done>true</done><success>true</success><status>Succeeded</status></result></checkDeployStatusResponse>`
	case req.Type == "CustomObject":
		var fields strings.Builder
		for f, ft := range o.fieldType {
			fmt.Fprintf(&fields, "<fields><fullName>%s</fullName><type>%s</type></fields>", f, ft)
		}
		return `<readMetadataResponse><result><records xsi:type="CustomObject"><fullName>Probe__mdt</fullName>` + fields.String() + `</records></result></readMetadataResponse>`
	case req.Type == "CustomMetadata":
		rec, ok := o.records[req.FullNames]
		if !ok {
			return `<readMetadataResponse><result><records xsi:nil="true"/></result></readMetadataResponse>`
		}
		var values strings.Builder
		for f, ft := range o.fieldType {
			v, ok := rec[f]
			switch {
			case ft == "Checkbox":
				if !ok {
					v = "false"
				}
				fmt.Fprintf(&values, `<values><field>%s</field><value xsi:type="xsd:boolean">%s</value></values>`, f, v)
			case !ok:
				fmt.Fprintf(&values, `<values><field>%s</field><value xsi:nil="true"/></values>`, f)
			case ft == "DateTime":
				ts, _ := time.Parse(time.RFC3339, v)
				fmt.Fprintf(&values, `<values><field>%s</field><value xsi:type="xsd:dateTime">%s</value></values>`, f, ts.UTC().Format("2006-01-02T15:04:05.000Z"))
			case ft == "Number":
				fmt.Fprintf(&values, `<values><field>%s</field><value xsi:type="xsd:double">%s.0</value></values>`, f, v)
			default:
				fmt.Fprintf(&values, `<values><field>%s</field><value xsi:type="xsd:string">%s</value></values>`, f, xmlText(v))
			}
		}
		return fmt.Sprintf(`<readMetadataResponse><result><records xsi:type="CustomMetadata"><fullName>%s</fullName><label>%s</label><protected>false</protected>%s</records></result></readMetadataResponse>`,
			req.FullNames, xmlText(o.labels[req.FullNames]), values.String())
	case strings.Contains(body, "<met:deploy>"):
		files, err := readDeployZipFiles(body)
		if err != nil {
			t.Errorf("reading deploy zip: %v", err)
		}
		if d, ok := files["destructiveChanges.xml"]; ok {
			o.deletes++
			if o.vanishOnDelete {
				o.records = map[string]map[string]string{}
			}
			o.deployFailed = true
			for name := range o.records {
				if strings.Contains(d, "<members>"+name+"</members>") {
					delete(o.records, name)
					o.deployFailed = false
				}
			}
			return `<deployResponse><result><id>0Af</id></result></deployResponse>`
		}
		for name, content := range files {
			if !strings.HasPrefix(name, "customMetadata/") {
				continue
			}
			o.lastXML = content
			fullName := strings.TrimSuffix(strings.TrimPrefix(name, "customMetadata/"), ".md")
			var rec struct {
				Label  string `xml:"label"`
				Values []struct {
					Field string `xml:"field"`
					Value struct {
						Type string `xml:"type,attr"`
						Nil  bool   `xml:"nil,attr"`
						Text string `xml:",chardata"`
					} `xml:"value"`
				} `xml:"values"`
			}
			if err := xml.Unmarshal([]byte(content), &rec); err != nil {
				t.Errorf("parsing record: %v", err)
			}
			if o.records[fullName] == nil {
				o.records[fullName] = map[string]string{}
			}
			o.labels[fullName] = rec.Label
			for _, v := range rec.Values {
				if v.Value.Nil {
					delete(o.records[fullName], v.Field)
				} else {
					o.records[fullName][v.Field] = v.Value.Text
				}
			}
		}
		return `<deployResponse><result><id>0Af</id></result></deployResponse>`
	}
	t.Errorf("unexpected SOAP request: %s", body)
	return ""
}

func xmlText(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func readDeployZipFiles(body string) (map[string]string, error) {
	var req struct {
		ZipFile string `xml:"Body>deploy>ZipFile"`
	}
	if err := xml.Unmarshal([]byte(body), &req); err != nil {
		return nil, err
	}
	zipBytes, err := base64.StdEncoding.DecodeString(req.ZipFile)
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, err
	}
	files := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		files[f.Name] = string(data)
	}
	return files, nil
}

func useFakeOrg(t *testing.T) *fakeCMDTOrg {
	org := newFakeCMDTOrg()
	server := org.server(t)
	t.Cleanup(server.Close)
	t.Setenv("SALESFORCE_INSTANCE_URL", server.URL)
	t.Setenv("SALESFORCE_CLIENT_ID", "test-client-id")
	t.Setenv("SALESFORCE_CLIENT_SECRET", "test-client-secret")
	return org
}

func TestCustomMetadataRecordResource_Lifecycle(t *testing.T) {
	org := useFakeOrg(t)
	const addr = "salesforce_custom_metadata_record.test"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "salesforce_custom_metadata_record" "test" {
  type           = "Probe"
  developer_name = "One"
  label          = "One & Co"
  values = {
    Text__c   = "a & b"
    When__c   = "2026-11-06T10:00:00+01:00"
    Amount__c = "7"
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "id", "Probe.One"),
					resource.TestCheckResourceAttr(addr, "label", "One & Co"),
					resource.TestCheckResourceAttr(addr, "protected", "false"),
					resource.TestCheckResourceAttr(addr, "values.%", "3"),
					resource.TestCheckResourceAttr(addr, "values.Text__c", "a & b"),
					resource.TestCheckResourceAttr(addr, "values.When__c", "2026-11-06T10:00:00+01:00"),
					resource.TestCheckResourceAttr(addr, "values.Amount__c", "7"),
					func(*terraform.State) error {
						if !strings.Contains(org.lastXML, `<field>When__c</field><value xsi:type="xsd:dateTime">2026-11-06T10:00:00+01:00</value>`) {
							return fmt.Errorf("DateTime not sent as xsd:dateTime: %s", org.lastXML)
						}
						return nil
					},
				),
			},
			{
				// Removing a key must clear it in the org: a deploy merges values.
				Config: `
resource "salesforce_custom_metadata_record" "test" {
  type           = "Probe"
  developer_name = "One"
  label          = "One renamed"
  values = {
    Text__c = "changed"
    When__c = "2026-11-06T09:00:00Z"
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "label", "One renamed"),
					resource.TestCheckResourceAttr(addr, "values.%", "2"),
					resource.TestCheckNoResourceAttr(addr, "values.Amount__c"),
					func(*terraform.State) error {
						if !strings.Contains(org.lastXML, `<field>Amount__c</field><value xsi:nil="true"/>`) {
							return fmt.Errorf("removed field not cleared: %s", org.lastXML)
						}
						if _, ok := org.records["Probe.One"]["Amount__c"]; ok {
							return fmt.Errorf("Amount__c still set in org")
						}
						return nil
					},
				),
			},
			{
				// The record is owned as a whole: a field set outside Terraform shows as drift.
				PreConfig: func() { org.set("Probe.One", "Amount__c", "5") },
				Config: `
resource "salesforce_custom_metadata_record" "test" {
  type           = "Probe"
  developer_name = "One"
  label          = "One renamed"
  values = {
    Text__c = "changed"
    When__c = "2026-11-06T09:00:00Z"
  }
}
`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				ResourceName:  addr,
				ImportState:   true,
				ImportStateId: "Probe.One",
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 state, got %d", len(states))
					}
					a := states[0].Attributes
					// Unchecked Flag__c reads back as false and is left out like an unset field.
					want := map[string]string{
						"id":               "Probe.One",
						"type":             "Probe",
						"developer_name":   "One",
						"label":            "One renamed",
						"protected":        "false",
						"values.%":         "3",
						"values.Text__c":   "changed",
						"values.When__c":   "2026-11-06T09:00:00.000Z",
						"values.Amount__c": "5.0",
					}
					for k, v := range want {
						if a[k] != v {
							return fmt.Errorf("%s: expected %q, got %q", k, v, a[k])
						}
					}
					return nil
				},
			},
			{
				// Applying the config clears the field set outside Terraform.
				Config: `
resource "salesforce_custom_metadata_record" "test" {
  type           = "Probe"
  developer_name = "One"
  label          = "One renamed"
  values = {
    Text__c = "changed"
    When__c = "2026-11-06T09:00:00Z"
  }
}
`,
				Check: func(*terraform.State) error {
					if _, ok := org.records["Probe.One"]["Amount__c"]; ok {
						return fmt.Errorf("Amount__c set outside Terraform was not cleared")
					}
					return nil
				},
			},
		},
	})

	if org.deletes == 0 {
		t.Fatal("expected delete deploy during destroy")
	}
	if len(org.records) != 0 {
		t.Fatalf("expected record to be deleted, got %v", org.records)
	}
}

func TestCustomMetadataRecordResource_RemovedOutOfBand(t *testing.T) {
	org := useFakeOrg(t)
	config := `
resource "salesforce_custom_metadata_record" "test" {
  type           = "Probe"
  developer_name = "Gone"
  label          = "Gone"
}
`
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: config, Check: resource.TestCheckNoResourceAttr("salesforce_custom_metadata_record.test", "values.%")},
			{
				PreConfig: func() {
					org.mu.Lock()
					delete(org.records, "Probe.Gone")
					org.mu.Unlock()
				},
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestCustomMetadataRecordResource_CheckboxFalse(t *testing.T) {
	useFakeOrg(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{{
			Config: `
resource "salesforce_custom_metadata_record" "test" {
  type           = "Probe"
  developer_name = "One"
  label          = "One"
  values         = { Flag__c = "false" }
}
`,
			Check: resource.TestCheckResourceAttr("salesforce_custom_metadata_record.test", "values.Flag__c", "false"),
		}},
	})
}

func TestCustomMetadataRecordResource_DeleteAlreadyGone(t *testing.T) {
	org := useFakeOrg(t)
	org.vanishOnDelete = true
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{{
			Config: `
resource "salesforce_custom_metadata_record" "test" {
  type           = "Probe"
  developer_name = "One"
  label          = "One"
}
`,
		}},
	})
	if org.deletes == 0 {
		t.Fatal("expected delete deploy during destroy")
	}
}

func TestCustomMetadataRecordResource_Validation(t *testing.T) {
	useFakeOrg(t)
	tests := []struct {
		name, typ, devName, label, wantErr string
	}{
		{"type with __mdt", "Probe__mdt", "One", "One", `without the __mdt suffix`},
		{"type invalid chars", "Pro-be", "One", "One", `must be a Salesforce API name`},
		{"developer_name double underscore", "Probe", "One__Two", "One", `developer_name`},
		{"developer_name trailing underscore", "Probe", "One_", "One", `developer_name`},
		{"developer_name leading digit", "Probe", "1One", "One", `developer_name`},
		{"developer_name too long", "Probe", strings.Repeat("a", 41), "One", `developer_name`},
		{"label too long", "Probe", "One", strings.Repeat("a", 41), `label`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
				Steps: []resource.TestStep{{
					Config: fmt.Sprintf(`
resource "salesforce_custom_metadata_record" "test" {
  type           = %q
  developer_name = %q
  label          = %q
}
`, tt.typ, tt.devName, tt.label),
					ExpectError: regexp.MustCompile(tt.wantErr),
				}},
			})
		})
	}

	t.Run("empty value", func(t *testing.T) {
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
			Steps: []resource.TestStep{{
				Config: `
resource "salesforce_custom_metadata_record" "test" {
  type           = "Probe"
  developer_name = "One"
  label          = "One"
  values         = { Text__c = "" }
}
`,
				ExpectError: regexp.MustCompile(`at least 1`),
			}},
		})
	})

	t.Run("null value", func(t *testing.T) {
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
			Steps: []resource.TestStep{{
				Config: `
resource "salesforce_custom_metadata_record" "test" {
  type           = "Probe"
  developer_name = "One"
  label          = "One"
  values         = { Text__c = "a", Flag__c = null }
}
`,
				ExpectError: regexp.MustCompile(`Null Map Value`),
			}},
		})
	})

	t.Run("bad import id", func(t *testing.T) {
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
			Steps: []resource.TestStep{{
				Config:        `resource "salesforce_custom_metadata_record" "test" {}`,
				ResourceName:  "salesforce_custom_metadata_record.test",
				ImportState:   true,
				ImportStateId: "NoDot",
				ExpectError:   regexp.MustCompile(`<type>.<developer_name>`),
			}},
		})
	})
}

// TestAccCustomMetadataRecordResource needs the CMDT TfProviderProbe__mdt in the target org with
// fields Text__c (Text 255), DateTime__c (DateTime), Date__c (Date), Checkbox__c (Checkbox) and
// Number__c (Number 18,2). Run locally only: TF_ACC=1 SALESFORCE_AUTH_URL=... make testacc.
func TestAccCustomMetadataRecordResource(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set, skipping acceptance test")
	}

	name := fmt.Sprintf("TfAcc_%d", time.Now().Unix())
	const addr = "salesforce_custom_metadata_record.test"
	config := func(label, values string) string {
		return fmt.Sprintf(`
resource "salesforce_custom_metadata_record" "test" {
  type           = "TfProviderProbe"
  developer_name = %q
  label          = %q
  values = {
%s
  }
}
`, name, label, values)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config("Acc & <Test>", `
    Text__c     = "a & b"
    DateTime__c = "2026-11-06T09:00:00Z"
    Date__c     = "2026-11-06"
    Checkbox__c = "true"
    Number__c   = "7"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "id", "TfProviderProbe."+name),
					resource.TestCheckResourceAttr(addr, "values.%", "5"),
					resource.TestCheckResourceAttr(addr, "values.DateTime__c", "2026-11-06T09:00:00Z"),
					resource.TestCheckResourceAttr(addr, "values.Number__c", "7"),
				),
			},
			{
				Config: config("Acc updated", `
    Text__c     = "changed"
    DateTime__c = "2026-11-07T09:00:00Z"
    Checkbox__c = "false"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "label", "Acc updated"),
					resource.TestCheckResourceAttr(addr, "values.%", "3"),
					resource.TestCheckResourceAttr(addr, "values.Checkbox__c", "false"),
				),
			},
			{
				ResourceName:  addr,
				ImportState:   true,
				ImportStateId: "TfProviderProbe." + name,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					a := states[0].Attributes
					// Date__c and Number__c were cleared by the update; the unchecked Checkbox__c is
					// left out like an unset field.
					want := map[string]string{
						"values.%":           "2",
						"values.Text__c":     "changed",
						"values.DateTime__c": "2026-11-07T09:00:00.000Z",
						"label":              "Acc updated",
					}
					for k, v := range want {
						if a[k] != v {
							return fmt.Errorf("%s: expected %q, got %q", k, v, a[k])
						}
					}
					return nil
				},
			},
		},
	})
}
