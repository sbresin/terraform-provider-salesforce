package metadata

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

func (c *Client) CreateCertificate(ctx context.Context, cert CertificateCreate) (string, error) {
	zipData, err := buildCertificateDeployZip(cert, c.apiVersion)
	if err != nil {
		return "", err
	}

	deployID, err := c.Deploy(ctx, zipData, DeployOptions{
		SinglePackage:   true,
		RollbackOnError: true,
	})
	if err != nil {
		return "", fmt.Errorf("deploying certificate metadata: %w", err)
	}

	return deployID, nil
}

// ReadCertificate reads a single certificate by its fullName (developer name).
// Returns ErrNotFound if the certificate does not exist.
func (c *Client) ReadCertificate(ctx context.Context, fullName string) (*Certificate, error) {
	req := &ReadMetadataRequest{
		Type:      "Certificate",
		FullNames: []string{fullName},
	}

	respBody, err := c.doRequest(ctx, "readMetadata", req)
	if err != nil {
		return nil, err
	}

	var resp ReadMetadataResponse
	if err := xml.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parsing readMetadata response: %w", err)
	}

	if len(resp.Result.Records) == 0 {
		return nil, ErrNotFound
	}

	record := resp.Result.Records[0]
	// An empty <records/> element parses to a record with empty fullName
	if record.FullName == "" {
		return nil, ErrNotFound
	}
	cert := &Certificate{
		FullName:             record.FullName,
		MasterLabel:          record.MasterLabel,
		Certificate:          record.Certificate,
		KeySize:              record.KeySize,
		Platform:             record.Platform,
		PrivateKeyExportable: record.PrivateKeyExportable,
	}

	// Parse expiration date if present
	if record.ExpirationDate != "" {
		// Try ISO 8601 with time first, then date-only format
		t, err := time.Parse(time.RFC3339, record.ExpirationDate)
		if err != nil {
			t, err = time.Parse("2006-01-02T15:04:05.000Z", record.ExpirationDate)
		}
		if err != nil {
			t, err = time.Parse("2006-01-02", record.ExpirationDate)
		}
		if err == nil {
			cert.ExpirationDate = &t
		} else {
			tflog.Warn(ctx, "failed to parse certificate expiration date",
				map[string]interface{}{
					"fullName":       record.FullName,
					"expirationDate": record.ExpirationDate,
					"error":          err.Error(),
				})
		}
	}

	return cert, nil
}

// ListCertificates returns the fullNames of all certificates in the org.
func (c *Client) ListCertificates(ctx context.Context) ([]string, error) {
	req := &ListMetadataRequest{
		Queries: []ListMetadataQuery{
			{Type: "Certificate"},
		},
		AsOfVersion: c.apiVersion,
	}

	respBody, err := c.doRequest(ctx, "listMetadata", req)
	if err != nil {
		return nil, err
	}

	var resp ListMetadataResponse
	if err := xml.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parsing listMetadata response: %w", err)
	}

	names := make([]string, 0, len(resp.Result))
	for _, fp := range resp.Result {
		names = append(names, fp.FullName)
	}

	return names, nil
}

// DeleteCertificate deletes a certificate by deploying destructive changes.
// Returns the deployment ID. The caller can use CheckDeployStatus to poll for completion.
func (c *Client) DeleteCertificate(ctx context.Context, fullName string) (string, error) {
	// Build destructive changes zip
	zipData, err := buildDestructiveChangesZip("Certificate", fullName, c.apiVersion)
	if err != nil {
		return "", fmt.Errorf("building destructive changes: %w", err)
	}

	// Deploy the destructive changes
	deployID, err := c.Deploy(ctx, zipData, DeployOptions{
		SinglePackage:   true,
		RollbackOnError: true,
	})
	if err != nil {
		return "", fmt.Errorf("deploying destructive changes: %w", err)
	}

	return deployID, nil
}

func buildCertificateDeployZip(cert CertificateCreate, apiVersion string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	packageXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Package xmlns="http://soap.sforce.com/2006/04/metadata">
    <types>
        <members>%s</members>
        <name>Certificate</name>
    </types>
    <version>%s</version>
</Package>`, cert.FullName, apiVersion)

	if err := writeZipFile(zw, "package.xml", []byte(packageXML)); err != nil {
		return nil, err
	}

	masterLabel := cert.MasterLabel
	if masterLabel == "" {
		masterLabel = cert.FullName
	}

	certXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Certificate xmlns="http://soap.sforce.com/2006/04/metadata">
    <keySize>%d</keySize>
    <masterLabel>%s</masterLabel>
    <privateKeyExportable>%t</privateKeyExportable>
</Certificate>`, cert.KeySize, xmlEscape(masterLabel), cert.PrivateKeyExportable)

	if err := writeZipFile(zw, fmt.Sprintf("certs/%s.crt-meta.xml", cert.FullName), []byte(certXML)); err != nil {
		return nil, err
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("closing zip: %w", err)
	}

	return buf.Bytes(), nil
}

func writeZipFile(zw *zip.Writer, name string, content []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("creating %s: %w", name, err)
	}
	if _, err := w.Write(content); err != nil {
		return fmt.Errorf("writing %s: %w", name, err)
	}
	return nil
}

func xmlEscape(s string) string {
	var buf bytes.Buffer
	if err := xml.EscapeText(&buf, []byte(s)); err != nil {
		return s
	}
	return buf.String()
}
