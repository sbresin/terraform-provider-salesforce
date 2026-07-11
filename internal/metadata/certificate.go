package metadata

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

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
		t, err := time.Parse("2006-01-02", record.ExpirationDate)
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
