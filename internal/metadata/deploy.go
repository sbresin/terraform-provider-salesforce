package metadata

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"time"
)

// Deploy starts a metadata deployment and returns the deployment ID.
// Use CheckDeployStatus to poll for completion.
func (c *Client) Deploy(ctx context.Context, zipData []byte, opts DeployOptions) (string, error) {
	req := &DeployRequest{
		ZipFile:       base64.StdEncoding.EncodeToString(zipData),
		DeployOptions: opts,
	}

	respBody, err := c.doRequest(ctx, "deploy", req)
	if err != nil {
		return "", err
	}

	var resp DeployResponse
	if err := xml.Unmarshal(respBody, &resp); err != nil {
		return "", fmt.Errorf("parsing deploy response: %w", err)
	}

	return resp.Result.ID, nil
}

// CheckDeployStatus checks the status of a deployment.
// Poll this until result.Done is true.
func (c *Client) CheckDeployStatus(ctx context.Context, deployID string) (*DeployResult, error) {
	req := &CheckDeployStatusRequest{
		AsyncProcessID: deployID,
		IncludeDetails: true,
	}

	respBody, err := c.doRequest(ctx, "checkDeployStatus", req)
	if err != nil {
		return nil, err
	}

	var resp CheckDeployStatusResponse
	if err := xml.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parsing checkDeployStatus response: %w", err)
	}

	return &resp.Result, nil
}

func (c *Client) WaitForDeploy(ctx context.Context, deployID string) (*DeployResult, error) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		result, err := c.CheckDeployStatus(ctx, deployID)
		if err != nil {
			return nil, err
		}

		if result.Done {
			if result.Success {
				return result, nil
			}
			// Return a DeployError that includes component failures for diagnostics
			return result, &DeployError{
				ID:              deployID,
				Status:          result.Status,
				ErrorMessage:    result.ErrorMessage,
				ErrorStatusCode: result.ErrorStatusCode,
				ComponentErrors: result.ComponentFailures,
			}
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// buildDestructiveChangesZip creates a zip package for destructive changes deployment.
// This is used to delete metadata components.
func buildDestructiveChangesZip(metadataType, fullName, apiVersion string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// Create package.xml (empty manifest)
	packageXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Package xmlns="http://soap.sforce.com/2006/04/metadata">
    <version>%s</version>
</Package>`, apiVersion)

	pw, err := zw.Create("package.xml")
	if err != nil {
		return nil, fmt.Errorf("creating package.xml: %w", err)
	}
	if _, err := pw.Write([]byte(packageXML)); err != nil {
		return nil, fmt.Errorf("writing package.xml: %w", err)
	}

	// Create destructiveChanges.xml
	destructiveXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Package xmlns="http://soap.sforce.com/2006/04/metadata">
    <types>
        <members>%s</members>
        <name>%s</name>
    </types>
    <version>%s</version>
</Package>`, fullName, metadataType, apiVersion)

	dw, err := zw.Create("destructiveChanges.xml")
	if err != nil {
		return nil, fmt.Errorf("creating destructiveChanges.xml: %w", err)
	}
	if _, err := dw.Write([]byte(destructiveXML)); err != nil {
		return nil, fmt.Errorf("writing destructiveChanges.xml: %w", err)
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("closing zip: %w", err)
	}

	return buf.Bytes(), nil
}
