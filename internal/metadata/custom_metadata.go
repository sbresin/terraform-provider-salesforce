package metadata

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Findings against a real org (API 67.0), which this file depends on:
//   - Deploy rejects xsi:type="xsd:string" for Checkbox, Date and DateTime fields and rejects a
//     missing xsi:type for every field, so the type is looked up from the CMDT's field definitions.
//   - Deploy merges: an omitted field keeps its value. Only <value xsi:nil="true"/> clears it.
//   - readMetadata returns datetimes as 2026-11-06T09:00:00.000Z, numbers as xsd:double (7.0),
//     null fields as xsi:nil and a missing record as <records xsi:nil="true"/>.

// ReadCustomMetadata reads a Custom Metadata record by fullName (<type>.<developer_name>).
// Null fields are omitted from Values. Returns ErrNotFound if the record does not exist.
func (c *Client) ReadCustomMetadata(ctx context.Context, fullName string) (*CustomMetadataRecord, error) {
	respBody, err := c.doRequest(ctx, "readMetadata", &ReadMetadataRequest{
		Type:      "CustomMetadata",
		FullNames: []string{fullName},
	})
	if err != nil {
		return nil, err
	}

	var resp readCustomMetadataResponse
	if err := xml.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parsing readMetadata response: %w", err)
	}
	if len(resp.Records) == 0 || resp.Records[0].FullName == "" {
		return nil, ErrNotFound
	}

	raw := resp.Records[0]
	rec := &CustomMetadataRecord{
		FullName:  raw.FullName,
		Label:     raw.Label,
		Protected: raw.Protected,
		Values:    map[string]FieldValue{},
	}
	for _, v := range raw.Values {
		if v.Value.Nil {
			continue
		}
		rec.Values[v.Field] = FieldValue{Type: v.Value.Type, Value: v.Value.Text}
	}
	return rec, nil
}

// DeployCustomMetadata creates or updates a Custom Metadata record and returns the deployment ID.
// The record is replaced as a whole: fields of the type that are not in Values are cleared.
func (c *Client) DeployCustomMetadata(ctx context.Context, rec CustomMetadataDeploy) (string, error) {
	fieldTypes, err := c.customMetadataFieldTypes(ctx, rec.Type)
	if err != nil {
		return "", err
	}

	zipData, err := buildCustomMetadataDeployZip(rec, fieldTypes, c.apiVersion)
	if err != nil {
		return "", err
	}

	deployID, err := c.Deploy(ctx, zipData, DeployOptions{
		SinglePackage:   true,
		RollbackOnError: true,
	})
	if err != nil {
		return "", fmt.Errorf("deploying custom metadata: %w", err)
	}
	return deployID, nil
}

// DeleteCustomMetadata deletes a Custom Metadata record by deploying destructive changes.
func (c *Client) DeleteCustomMetadata(ctx context.Context, fullName string) (string, error) {
	zipData, err := buildDestructiveChangesZip("CustomMetadata", fullName, c.apiVersion)
	if err != nil {
		return "", fmt.Errorf("building destructive changes: %w", err)
	}

	deployID, err := c.Deploy(ctx, zipData, DeployOptions{
		SinglePackage:   true,
		RollbackOnError: true,
	})
	if err != nil {
		return "", fmt.Errorf("deploying destructive changes: %w", err)
	}
	return deployID, nil
}

// customMetadataFieldTypes returns field API name -> field type (e.g. "Checkbox") of <cmdtType>__mdt.
func (c *Client) customMetadataFieldTypes(ctx context.Context, cmdtType string) (map[string]string, error) {
	respBody, err := c.doRequest(ctx, "readMetadata", &ReadMetadataRequest{
		Type:      "CustomObject",
		FullNames: []string{cmdtType + "__mdt"},
	})
	if err != nil {
		return nil, err
	}

	var resp readCustomObjectResponse
	if err := xml.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parsing readMetadata response: %w", err)
	}

	types := map[string]string{}
	for _, r := range resp.Records {
		for _, f := range r.Fields {
			types[f.FullName] = f.Type
		}
	}
	return types, nil
}

func xsiTypeFor(fieldType string) string {
	switch fieldType {
	case "Checkbox":
		return "xsd:boolean"
	case "Date":
		return "xsd:date"
	case "DateTime":
		return "xsd:dateTime"
	default:
		// Text, Number and the other field types accept xsd:string.
		return "xsd:string"
	}
}

func buildCustomMetadataDeployZip(rec CustomMetadataDeploy, fieldTypes map[string]string, apiVersion string) ([]byte, error) {
	fullName := rec.Type + "." + rec.DeveloperName

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	packageXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Package xmlns="http://soap.sforce.com/2006/04/metadata">
    <types>
        <members>%s</members>
        <name>CustomMetadata</name>
    </types>
    <version>%s</version>
</Package>`, xmlEscape(fullName), apiVersion)

	if err := writeZipFile(zw, "package.xml", []byte(packageXML)); err != nil {
		return nil, err
	}

	var values strings.Builder
	fields := make([]string, 0, len(rec.Values))
	for f := range rec.Values {
		fields = append(fields, f)
	}
	sort.Strings(fields)
	for _, f := range fields {
		fmt.Fprintf(&values, "\n    <values><field>%s</field><value xsi:type=\"%s\">%s</value></values>",
			xmlEscape(f), xsiTypeFor(fieldTypes[f]), xmlEscape(rec.Values[f]))
	}
	var nulls []string
	for f := range fieldTypes {
		if _, ok := rec.Values[f]; !ok {
			nulls = append(nulls, f)
		}
	}
	sort.Strings(nulls)
	for _, f := range nulls {
		fmt.Fprintf(&values, "\n    <values><field>%s</field><value xsi:nil=\"true\"/></values>", xmlEscape(f))
	}

	recordXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<CustomMetadata xmlns="http://soap.sforce.com/2006/04/metadata" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema">
    <label>%s</label>
    <protected>%t</protected>%s
</CustomMetadata>`, xmlEscape(rec.Label), rec.Protected, values.String())

	if err := writeZipFile(zw, "customMetadata/"+fullName+".md", []byte(recordXML)); err != nil {
		return nil, err
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("closing zip: %w", err)
	}
	return buf.Bytes(), nil
}

// SameValue reports whether a configured value and a value read back with the given xsi:type
// mean the same thing (e.g. "7" and "7.0" for xsd:double).
func SameValue(xsiType, configured, remote string) bool {
	if configured == remote {
		return true
	}
	switch xsiType {
	case "xsd:dateTime":
		a, errA := time.Parse(time.RFC3339, configured)
		b, errB := time.Parse(time.RFC3339, remote)
		return errA == nil && errB == nil && a.Equal(b)
	case "xsd:double":
		a, errA := strconv.ParseFloat(configured, 64)
		b, errB := strconv.ParseFloat(remote, 64)
		return errA == nil && errB == nil && a == b
	case "xsd:boolean":
		a, errA := strconv.ParseBool(configured)
		b, errB := strconv.ParseBool(remote)
		return errA == nil && errB == nil && a == b
	}
	return false
}
