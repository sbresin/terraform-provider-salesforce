package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// SessionSource provides session credentials for UI and SOAP API operations.
type SessionSource interface {
	// Session returns session credentials. Each call fetches fresh credentials.
	Session(ctx context.Context) (*SessionCredentials, error)
}

// SessionCredentials holds session information for Salesforce UI/SOAP operations.
type SessionCredentials struct {
	SessionID   string         // sid cookie value (used for SOAP Metadata API)
	Cookies     []*http.Cookie // all session cookies (used for UI form operations)
	InstanceURL string         // Salesforce instance URL
}

// sessionSource implements SessionSource using the UI Bridge API.
type sessionSource struct {
	tokenSource TokenSource
	instanceURL string
}

// NewSessionSource creates a SessionSource that exchanges OAuth tokens for session cookies.
// It uses the /services/oauth2/singleaccess UI Bridge API to get a frontdoor URL,
// then visits the frontdoor URL to establish session cookies.
func NewSessionSource(tokenSource TokenSource, instanceURL string) SessionSource {
	return &sessionSource{
		tokenSource: tokenSource,
		instanceURL: instanceURL,
	}
}

func (s *sessionSource) Session(ctx context.Context) (*SessionCredentials, error) {
	// Step 1: Get access token
	accessToken, err := s.tokenSource.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting access token: %w", err)
	}

	// Step 2: Exchange access token for frontdoor URL
	frontdoorURL, err := s.getFrontdoorURL(ctx, accessToken)
	if err != nil {
		return nil, fmt.Errorf("getting frontdoor URL: %w", err)
	}

	// Step 3: Visit frontdoor URL to establish session cookies
	cookies, err := s.establishSession(ctx, frontdoorURL)
	if err != nil {
		return nil, fmt.Errorf("establishing session: %w", err)
	}

	// Extract session ID from sid cookie
	var sessionID string
	for _, c := range cookies {
		if c.Name == "sid" {
			sessionID = c.Value
			break
		}
	}

	if sessionID == "" {
		return nil, fmt.Errorf("session establishment failed: 'sid' cookie not found")
	}

	return &SessionCredentials{
		SessionID:   sessionID,
		Cookies:     cookies,
		InstanceURL: s.instanceURL,
	}, nil
}

// singleAccessResponse is the response from /services/oauth2/singleaccess
type singleAccessResponse struct {
	FrontdoorURI string `json:"frontdoor_uri"`
}

func (s *sessionSource) getFrontdoorURL(ctx context.Context, accessToken string) (string, error) {
	url := s.instanceURL + "/services/oauth2/singleaccess"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("singleaccess request failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result singleAccessResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}

	if result.FrontdoorURI == "" {
		return "", fmt.Errorf("response missing frontdoor_uri")
	}

	return result.FrontdoorURI, nil
}

// cookieCapturingTransport wraps an http.RoundTripper to capture cookies from all responses
// in the redirect chain. This is necessary because Go's http.Client with CheckRedirect
// does not provide access to response cookies from intermediate redirects.
type cookieCapturingTransport struct {
	transport http.RoundTripper
	cookies   map[string]*http.Cookie
}

func (t *cookieCapturingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	for _, c := range resp.Cookies() {
		t.cookies[c.Name] = c
	}
	return resp, nil
}

func (s *sessionSource) establishSession(ctx context.Context, frontdoorURL string) ([]*http.Cookie, error) {
	// Use a custom transport to capture cookies from all responses in the redirect chain
	transport := &cookieCapturingTransport{
		transport: http.DefaultTransport,
		cookies:   make(map[string]*http.Cookie),
	}

	client := &http.Client{
		Transport: transport,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, frontdoorURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	// Read body to complete the request
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("frontdoor request failed (HTTP %d)", resp.StatusCode)
	}

	// Convert map to slice
	var cookies []*http.Cookie
	for _, c := range transport.cookies {
		cookies = append(cookies, c)
	}

	return cookies, nil
}
