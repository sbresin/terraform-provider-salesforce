package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// TokenSource provides access tokens for Salesforce API authentication.
type TokenSource interface {
	// Token returns a valid access token. Implementations may cache tokens.
	Token(ctx context.Context) (string, error)
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	IssuedAt    string `json:"issued_at"`
	Scope       string `json:"scope"`
}

type tokenError struct {
	Error       string `json:"error"`
	Description string `json:"error_description"`
}

// clientCredentialsTokenSource implements TokenSource using the client_credentials grant.
type clientCredentialsTokenSource struct {
	instanceURL  string
	clientID     string
	clientSecret string
	httpClient   *http.Client
}

// NewClientCredentialsTokenSource creates a TokenSource using OAuth 2.0 client_credentials grant.
func NewClientCredentialsTokenSource(instanceURL, clientID, clientSecret string) TokenSource {
	return &clientCredentialsTokenSource{
		instanceURL:  instanceURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   http.DefaultClient,
	}
}

func (ts *clientCredentialsTokenSource) Token(ctx context.Context) (string, error) {
	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {ts.clientID},
		"client_secret": {ts.clientSecret},
	}

	return fetchToken(ctx, ts.httpClient, ts.instanceURL+"/services/oauth2/token", data)
}

// refreshTokenSource implements TokenSource using the refresh_token grant.
type refreshTokenSource struct {
	instanceURL  string
	clientID     string
	clientSecret string
	refreshToken string
	httpClient   *http.Client
}

// NewRefreshTokenSource creates a TokenSource using OAuth 2.0 refresh_token grant.
// clientSecret may be empty for CLI-generated auth URLs.
func NewRefreshTokenSource(instanceURL, clientID, clientSecret, refreshToken string) TokenSource {
	return &refreshTokenSource{
		instanceURL:  instanceURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		refreshToken: refreshToken,
		httpClient:   http.DefaultClient,
	}
}

func (ts *refreshTokenSource) Token(ctx context.Context) (string, error) {
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {ts.clientID},
		"refresh_token": {ts.refreshToken},
	}
	if ts.clientSecret != "" {
		data.Set("client_secret", ts.clientSecret)
	}

	return fetchToken(ctx, ts.httpClient, ts.instanceURL+"/services/oauth2/token", data)
}

func fetchToken(ctx context.Context, client *http.Client, tokenURL string, data url.Values) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var tokenErr tokenError
		if json.Unmarshal(body, &tokenErr) == nil && tokenErr.Error != "" {
			return "", fmt.Errorf("token request failed: %s - %s", tokenErr.Error, tokenErr.Description)
		}
		return "", fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("parsing token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("token response missing access_token")
	}

	return tokenResp.AccessToken, nil
}
