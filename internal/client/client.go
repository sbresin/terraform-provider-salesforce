package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Config holds the configuration for creating a Salesforce client.
type Config struct {
	InstanceURL string
	APIVersion  string
	TokenSource TokenSource
}

// Client is a Salesforce API client.
type Client struct {
	instanceURL string
	apiVersion  string
	tokenSource TokenSource
	httpClient  *http.Client
}

// NewClient creates a new Salesforce client with the given configuration.
func NewClient(cfg Config) *Client {
	apiVersion := cfg.APIVersion
	if apiVersion == "" {
		apiVersion = "v61.0"
	}

	return &Client{
		instanceURL: cfg.InstanceURL,
		apiVersion:  apiVersion,
		tokenSource: cfg.TokenSource,
		httpClient:  http.DefaultClient,
	}
}

// ToolingQuery executes a SOQL query against the Tooling API.
func (c *Client) ToolingQuery(ctx context.Context, soql string, result any) error {
	token, err := c.tokenSource.Token(ctx)
	if err != nil {
		return fmt.Errorf("getting access token: %w", err)
	}

	queryURL := fmt.Sprintf("%s/services/data/%s/tooling/query?q=%s",
		c.instanceURL, c.apiVersion, url.QueryEscape(soql))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, queryURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return parseAPIError(resp.StatusCode, body)
	}

	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	return nil
}

// apiError represents a Salesforce API error response.
type apiError struct {
	Message   string `json:"message"`
	ErrorCode string `json:"errorCode"`
}

func parseAPIError(statusCode int, body []byte) error {
	// Salesforce returns errors as an array
	var errors []apiError
	if json.Unmarshal(body, &errors) == nil && len(errors) > 0 {
		return fmt.Errorf("salesforce API error (HTTP %d): %s - %s",
			statusCode, errors[0].ErrorCode, errors[0].Message)
	}

	// Fallback if not standard error format
	return fmt.Errorf("salesforce API error (HTTP %d): %s", statusCode, string(body))
}
