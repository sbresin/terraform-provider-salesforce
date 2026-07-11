package metadata

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
)

const defaultAPIVersion = "67.0"

// Client is a SOAP Metadata API client.
type Client struct {
	instanceURL string
	sessionID   string
	apiVersion  string
	httpClient  *http.Client
}

// Option configures a Client.
type Option func(*Client)

// NewClient creates a new Metadata API client.
// The sessionID should be obtained from a SessionSource.
func NewClient(instanceURL, sessionID string, opts ...Option) *Client {
	c := &Client{
		instanceURL: instanceURL,
		sessionID:   sessionID,
		apiVersion:  defaultAPIVersion,
		httpClient:  http.DefaultClient,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// WithAPIVersion sets the Metadata API version.
func WithAPIVersion(version string) Option {
	return func(c *Client) {
		c.apiVersion = version
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		c.httpClient = client
	}
}

// endpoint returns the SOAP Metadata API endpoint URL.
func (c *Client) endpoint() string {
	return fmt.Sprintf("%s/services/Soap/m/%s", c.instanceURL, c.apiVersion)
}

// doRequest sends a SOAP request and returns the response body content.
func (c *Client) doRequest(ctx context.Context, action string, body any) ([]byte, error) {
	// Build SOAP envelope
	envelope := Envelope{
		XMLNS:  nsSOAPEnv,
		XMLNSm: nsMetadata,
		Header: Header{
			SessionHeader: SessionHeader{
				SessionID: c.sessionID,
			},
		},
		Body: Body{
			Content: body,
		},
	}

	// Marshal to XML
	reqBody, err := xml.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	// Add XML declaration
	reqBody = append([]byte(xml.Header), reqBody...)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(), bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "text/xml; charset=UTF-8")
	req.Header.Set("SOAPAction", action)

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	// Parse response envelope
	var respEnvelope ResponseEnvelope
	if err := xml.Unmarshal(respBody, &respEnvelope); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	// Check for SOAP fault
	if respEnvelope.Body.Fault != nil {
		fault := respEnvelope.Body.Fault
		return nil, &SOAPFaultError{
			FaultCode:   fault.FaultCode,
			FaultString: fault.FaultString,
		}
	}

	return respEnvelope.Body.Content, nil
}

// APIVersion returns the configured API version.
func (c *Client) APIVersion() string {
	return c.apiVersion
}
