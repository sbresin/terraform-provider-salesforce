package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientCredentialsTokenSource(t *testing.T) {
	t.Run("successful token fetch", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if r.URL.Path != "/services/oauth2/token" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}

			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("grant_type") != "client_credentials" {
				t.Errorf("expected grant_type=client_credentials, got %s", r.Form.Get("grant_type"))
			}
			if r.Form.Get("client_id") != "test-client-id" {
				t.Errorf("unexpected client_id: %s", r.Form.Get("client_id"))
			}
			if r.Form.Get("client_secret") != "test-client-secret" {
				t.Errorf("unexpected client_secret: %s", r.Form.Get("client_secret"))
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"access_token": "test-access-token",
				"token_type":   "Bearer",
			})
		}))
		defer server.Close()

		ts := NewClientCredentialsTokenSource(server.URL, "test-client-id", "test-client-secret")
		token, err := ts.Token(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if token != "test-access-token" {
			t.Errorf("expected token 'test-access-token', got %q", token)
		}
	})

	t.Run("invalid credentials", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{
				"error":             "invalid_client",
				"error_description": "Invalid client credentials",
			})
		}))
		defer server.Close()

		ts := NewClientCredentialsTokenSource(server.URL, "bad-id", "bad-secret")
		_, err := ts.Token(context.Background())

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "invalid_client") {
			t.Errorf("expected error containing 'invalid_client', got: %v", err)
		}
	})
}

func TestRefreshTokenSource(t *testing.T) {
	t.Run("successful token refresh", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("grant_type") != "refresh_token" {
				t.Errorf("expected grant_type=refresh_token, got %s", r.Form.Get("grant_type"))
			}
			if r.Form.Get("refresh_token") != "test-refresh-token" {
				t.Errorf("unexpected refresh_token: %s", r.Form.Get("refresh_token"))
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"access_token": "refreshed-access-token",
				"token_type":   "Bearer",
			})
		}))
		defer server.Close()

		ts := NewRefreshTokenSource(server.URL, "test-client-id", "", "test-refresh-token")
		token, err := ts.Token(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if token != "refreshed-access-token" {
			t.Errorf("expected token 'refreshed-access-token', got %q", token)
		}
	})

	t.Run("with client secret", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("client_secret") != "the-secret" {
				t.Errorf("expected client_secret='the-secret', got %q", r.Form.Get("client_secret"))
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"access_token": "token-with-secret",
			})
		}))
		defer server.Close()

		ts := NewRefreshTokenSource(server.URL, "client-id", "the-secret", "refresh-token")
		token, err := ts.Token(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if token != "token-with-secret" {
			t.Errorf("unexpected token: %s", token)
		}
	})
}
