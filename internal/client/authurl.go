package client

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// AuthURLCredentials holds credentials parsed from a sfdx auth URL.
type AuthURLCredentials struct {
	ClientID     string
	ClientSecret string
	RefreshToken string
	InstanceURL  string
}

// ParseAuthURL parses a sfdx auth URL in the format:
// force://<clientId>:<clientSecret>:<refreshToken>@<host>
//
// The clientSecret is often empty for CLI-generated URLs.
func ParseAuthURL(s string) (*AuthURLCredentials, error) {
	if s == "" {
		return nil, errors.New("auth URL is empty")
	}

	parsed, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("invalid auth URL: %w", err)
	}

	if parsed.Scheme != "force" {
		return nil, fmt.Errorf("invalid auth URL scheme: expected 'force', got %q", parsed.Scheme)
	}

	if parsed.Host == "" {
		return nil, errors.New("auth URL missing host")
	}

	// The "user info" portion contains clientId:clientSecret:refreshToken
	// Go's url.Parse interprets this as username:password where password is clientSecret:refreshToken
	if parsed.User == nil {
		return nil, errors.New("auth URL has invalid format: missing credentials")
	}

	clientID := parsed.User.Username()
	password, hasPassword := parsed.User.Password()
	if !hasPassword {
		return nil, errors.New("auth URL has invalid format: expected clientId:clientSecret:refreshToken")
	}

	// Password contains clientSecret:refreshToken - split on first colon
	parts := strings.SplitN(password, ":", 2)
	if len(parts) < 2 {
		return nil, errors.New("auth URL has invalid format: expected clientId:clientSecret:refreshToken")
	}

	clientSecret := parts[0]
	refreshToken := parts[1]

	if clientID == "" {
		return nil, errors.New("auth URL missing client ID")
	}
	if refreshToken == "" {
		return nil, errors.New("auth URL missing refresh token")
	}

	// Note: url.Parse already handles URL decoding for Username() and Password()

	return &AuthURLCredentials{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RefreshToken: refreshToken,
		InstanceURL:  "https://" + parsed.Host,
	}, nil
}
