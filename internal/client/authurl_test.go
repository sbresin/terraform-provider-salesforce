package client

import (
	"testing"
)

func TestParseAuthURL(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantCreds   *AuthURLCredentials
		wantErr     bool
		errContains string
	}{
		{
			name:  "CLI-generated URL with empty secret",
			input: "force://PlatformCLI::00Dxx0000000000!refreshtoken@example.my.salesforce.com",
			wantCreds: &AuthURLCredentials{
				ClientID:     "PlatformCLI",
				ClientSecret: "",
				RefreshToken: "00Dxx0000000000!refreshtoken",
				InstanceURL:  "https://example.my.salesforce.com",
			},
		},
		{
			name:  "Connected App with secret",
			input: "force://3MVG9abc123::myrefreshtoken@test.salesforce.com",
			wantCreds: &AuthURLCredentials{
				ClientID:     "3MVG9abc123",
				ClientSecret: "",
				RefreshToken: "myrefreshtoken",
				InstanceURL:  "https://test.salesforce.com",
			},
		},
		{
			name:  "URL with client secret",
			input: "force://clientid:clientsecret:refreshtoken@org.my.salesforce.com",
			wantCreds: &AuthURLCredentials{
				ClientID:     "clientid",
				ClientSecret: "clientsecret",
				RefreshToken: "refreshtoken",
				InstanceURL:  "https://org.my.salesforce.com",
			},
		},
		{
			name:        "invalid scheme",
			input:       "https://example.com",
			wantErr:     true,
			errContains: "scheme",
		},
		{
			name:        "missing host",
			input:       "force://clientid::token@",
			wantErr:     true,
			errContains: "host",
		},
		{
			name:        "empty string",
			input:       "",
			wantErr:     true,
			errContains: "empty",
		},
		{
			name:        "malformed - missing refresh token separator",
			input:       "force://clientid@host.com",
			wantErr:     true,
			errContains: "format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseAuthURL(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseAuthURL() expected error containing %q, got nil", tt.errContains)
					return
				}
				if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("ParseAuthURL() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("ParseAuthURL() unexpected error: %v", err)
				return
			}

			if got.ClientID != tt.wantCreds.ClientID {
				t.Errorf("ClientID = %q, want %q", got.ClientID, tt.wantCreds.ClientID)
			}
			if got.ClientSecret != tt.wantCreds.ClientSecret {
				t.Errorf("ClientSecret = %q, want %q", got.ClientSecret, tt.wantCreds.ClientSecret)
			}
			if got.RefreshToken != tt.wantCreds.RefreshToken {
				t.Errorf("RefreshToken = %q, want %q", got.RefreshToken, tt.wantCreds.RefreshToken)
			}
			if got.InstanceURL != tt.wantCreds.InstanceURL {
				t.Errorf("InstanceURL = %q, want %q", got.InstanceURL, tt.wantCreds.InstanceURL)
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
