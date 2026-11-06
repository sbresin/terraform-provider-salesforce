package metadata

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func soapResponse(body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns="http://soap.sforce.com/2006/04/metadata" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema">
<soapenv:Body>` + body + `</soapenv:Body></soapenv:Envelope>`
}

func TestClient_ReadCustomMetadata(t *testing.T) {
	t.Run("parses values and skips nil", func(t *testing.T) {
		var gotBody string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			w.Write([]byte(soapResponse(`<readMetadataResponse><result><records xsi:type="CustomMetadata">
<fullName>Probe.One</fullName><label>One &amp; Co</label><protected>true</protected>
<values><field>Checkbox__c</field><value xsi:type="xsd:boolean">false</value></values>
<values><field>DateTime__c</field><value xsi:type="xsd:dateTime">2026-11-06T09:00:00.000Z</value></values>
<values><field>Number__c</field><value xsi:type="xsd:double">7.0</value></values>
<values><field>Text__c</field><value xsi:type="xsd:string">a &amp; b</value></values>
<values><field>Date__c</field><value xsi:nil="true"/></values>
</records></result></readMetadataResponse>`)))
		}))
		defer server.Close()

		rec, err := NewClient(server.URL, "sid").ReadCustomMetadata(context.Background(), "Probe.One")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(gotBody, "<met:type>CustomMetadata</met:type>") || !strings.Contains(gotBody, "<met:fullNames>Probe.One</met:fullNames>") {
			t.Errorf("unexpected request: %s", gotBody)
		}
		if rec.FullName != "Probe.One" || rec.Label != "One & Co" || !rec.Protected {
			t.Errorf("unexpected record header: %+v", rec)
		}
		want := map[string]FieldValue{
			"Checkbox__c": {Type: "xsd:boolean", Value: "false"},
			"DateTime__c": {Type: "xsd:dateTime", Value: "2026-11-06T09:00:00.000Z"},
			"Number__c":   {Type: "xsd:double", Value: "7.0"},
			"Text__c":     {Type: "xsd:string", Value: "a & b"},
		}
		if len(rec.Values) != len(want) {
			t.Fatalf("expected %d values, got %+v", len(want), rec.Values)
		}
		for k, v := range want {
			if rec.Values[k] != v {
				t.Errorf("%s: expected %+v, got %+v", k, v, rec.Values[k])
			}
		}
	})

	t.Run("nil record is not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(soapResponse(`<readMetadataResponse><result><records xsi:nil="true"/></result></readMetadataResponse>`)))
		}))
		defer server.Close()

		_, err := NewClient(server.URL, "sid").ReadCustomMetadata(context.Background(), "Probe.Nope")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestClient_DeployCustomMetadata(t *testing.T) {
	var zipData []byte
	var actions []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.Header.Get("SOAPAction")
		actions = append(actions, action)
		b, _ := io.ReadAll(r.Body)
		switch action {
		case "readMetadata":
			if !strings.Contains(string(b), "<met:type>CustomObject</met:type>") || !strings.Contains(string(b), "<met:fullNames>Probe__mdt</met:fullNames>") {
				t.Errorf("unexpected readMetadata request: %s", b)
			}
			w.Write([]byte(soapResponse(`<readMetadataResponse><result><records xsi:type="CustomObject">
<fullName>Probe__mdt</fullName>
<fields><fullName>Checkbox__c</fullName><type>Checkbox</type></fields>
<fields><fullName>Date__c</fullName><type>Date</type></fields>
<fields><fullName>DateTime__c</fullName><type>DateTime</type></fields>
<fields><fullName>Number__c</fullName><type>Number</type></fields>
<fields><fullName>Text__c</fullName><type>Text</type></fields>
</records></result></readMetadataResponse>`)))
		case "deploy":
			var req struct {
				ZipFile string `xml:"Body>deploy>ZipFile"`
			}
			if err := xml.Unmarshal(b, &req); err != nil {
				t.Fatalf("parsing deploy request: %v", err)
			}
			zipData, _ = base64.StdEncoding.DecodeString(req.ZipFile)
			if !strings.Contains(string(b), "<met:singlePackage>true</met:singlePackage>") || !strings.Contains(string(b), "<met:rollbackOnError>true</met:rollbackOnError>") {
				t.Errorf("unexpected deploy options: %s", b)
			}
			w.Write([]byte(soapResponse(`<deployResponse><result><id>0Af1</id><done>false</done></result></deployResponse>`)))
		default:
			t.Errorf("unexpected action %q", action)
		}
	}))
	defer server.Close()

	id, err := NewClient(server.URL, "sid").DeployCustomMetadata(context.Background(), CustomMetadataDeploy{
		Type:          "Probe",
		DeveloperName: "One",
		Label:         "One & <Co>",
		Protected:     true,
		Values: map[string]string{
			"Checkbox__c": "true",
			"Date__c":     "2026-11-06",
			"DateTime__c": "2026-11-06T09:00:00Z",
			"Number__c":   "42.5",
			"Text__c":     "a & b",
			"Unknown__c":  "x",
		},
		Nulls: []string{"Gone__c"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "0Af1" {
		t.Errorf("expected deploy id 0Af1, got %q", id)
	}
	if strings.Join(actions, ",") != "readMetadata,deploy" {
		t.Errorf("unexpected actions: %v", actions)
	}

	files := unzip(t, zipData)
	if !strings.Contains(files["package.xml"], "<members>Probe.One</members>") || !strings.Contains(files["package.xml"], "<name>CustomMetadata</name>") {
		t.Errorf("unexpected package.xml: %s", files["package.xml"])
	}
	rec, ok := files["customMetadata/Probe.One.md"]
	if !ok {
		t.Fatalf("record file missing, got %v", keys(files))
	}
	for _, want := range []string{
		`xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"`,
		`xmlns:xsd="http://www.w3.org/2001/XMLSchema"`,
		`<label>One &amp; &lt;Co&gt;</label>`,
		`<protected>true</protected>`,
		`<field>Checkbox__c</field><value xsi:type="xsd:boolean">true</value>`,
		`<field>Date__c</field><value xsi:type="xsd:date">2026-11-06</value>`,
		`<field>DateTime__c</field><value xsi:type="xsd:dateTime">2026-11-06T09:00:00Z</value>`,
		`<field>Number__c</field><value xsi:type="xsd:string">42.5</value>`,
		`<field>Text__c</field><value xsi:type="xsd:string">a &amp; b</value>`,
		`<field>Unknown__c</field><value xsi:type="xsd:string">x</value>`,
		`<field>Gone__c</field><value xsi:nil="true"/>`,
	} {
		if !strings.Contains(rec, want) {
			t.Errorf("record XML missing %s:\n%s", want, rec)
		}
	}
	if err := xml.Unmarshal([]byte(rec), new(struct{})); err != nil {
		t.Errorf("record XML not well-formed: %v", err)
	}
}

func TestClient_DeleteCustomMetadata(t *testing.T) {
	var zipData []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var req struct {
			ZipFile string `xml:"Body>deploy>ZipFile"`
		}
		_ = xml.Unmarshal(b, &req)
		zipData, _ = base64.StdEncoding.DecodeString(req.ZipFile)
		w.Write([]byte(soapResponse(`<deployResponse><result><id>0Af2</id></result></deployResponse>`)))
	}))
	defer server.Close()

	id, err := NewClient(server.URL, "sid").DeleteCustomMetadata(context.Background(), "Probe.One")
	if err != nil || id != "0Af2" {
		t.Fatalf("unexpected result %q, %v", id, err)
	}
	d := unzip(t, zipData)["destructiveChanges.xml"]
	if !strings.Contains(d, "<members>Probe.One</members>") || !strings.Contains(d, "<name>CustomMetadata</name>") {
		t.Errorf("unexpected destructiveChanges.xml: %s", d)
	}
}

func TestSameValue(t *testing.T) {
	tests := []struct {
		typ, configured, remote string
		want                    bool
	}{
		{"xsd:dateTime", "2026-11-06T09:00:00Z", "2026-11-06T09:00:00.000Z", true},
		{"xsd:dateTime", "2026-11-06T10:00:00+01:00", "2026-11-06T09:00:00.000Z", true},
		{"xsd:dateTime", "2026-11-06T09:00:01Z", "2026-11-06T09:00:00.000Z", false},
		{"xsd:double", "7", "7.0", true},
		{"xsd:double", "42.50", "42.5", true},
		{"xsd:double", "7", "7.5", false},
		{"xsd:boolean", "TRUE", "true", true},
		{"xsd:boolean", "false", "true", false},
		{"xsd:date", "2026-11-06", "2026-11-06", true},
		{"xsd:string", "7", "7.0", false},
		{"xsd:string", "a", "a", true},
		{"xsd:dateTime", "not a date", "2026-11-06T09:00:00.000Z", false},
	}
	for _, tt := range tests {
		if got := SameValue(tt.typ, tt.configured, tt.remote); got != tt.want {
			t.Errorf("SameValue(%q, %q, %q) = %v, want %v", tt.typ, tt.configured, tt.remote, got, tt.want)
		}
	}
}

func unzip(t *testing.T, data []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("reading zip: %v", err)
	}
	files := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = string(b)
	}
	return files
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
