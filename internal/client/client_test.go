package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockTokenSource is a test helper that returns a fixed token.
type mockTokenSource struct {
	token string
	err   error
}

func (m *mockTokenSource) Token(_ context.Context) (string, error) {
	return m.token, m.err
}

func TestClient_ToolingQuery(t *testing.T) {
	t.Run("successful query", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("expected GET, got %s", r.Method)
			}
			if !strings.HasPrefix(r.URL.Path, "/services/data/v61.0/tooling/query") {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
				t.Errorf("unexpected auth header: %s", auth)
			}

			query := r.URL.Query().Get("q")
			if !strings.Contains(query, "SELECT Id FROM Certificate") {
				t.Errorf("unexpected query: %s", query)
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"size":         1,
				"totalSize":    1,
				"done":         true,
				"queryLocator": nil,
				"records": []map[string]any{
					{"Id": "0P1xx0000000001", "DeveloperName": "TestCert"},
				},
			})
		}))
		defer server.Close()

		client := NewClient(Config{
			InstanceURL: server.URL,
			APIVersion:  "v61.0",
			TokenSource: &mockTokenSource{token: "test-token"},
		})

		var result struct {
			Records []struct {
				ID            string `json:"Id"`
				DeveloperName string `json:"DeveloperName"`
			} `json:"records"`
		}

		err := client.ToolingQuery(context.Background(), "SELECT Id FROM Certificate", &result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Records) != 1 {
			t.Fatalf("expected 1 record, got %d", len(result.Records))
		}
		if result.Records[0].ID != "0P1xx0000000001" {
			t.Errorf("unexpected ID: %s", result.Records[0].ID)
		}
	})

	t.Run("query error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode([]map[string]any{
				{
					"message":   "Invalid query",
					"errorCode": "MALFORMED_QUERY",
				},
			})
		}))
		defer server.Close()

		client := NewClient(Config{
			InstanceURL: server.URL,
			APIVersion:  "v61.0",
			TokenSource: &mockTokenSource{token: "test-token"},
		})

		var result any
		err := client.ToolingQuery(context.Background(), "INVALID QUERY", &result)

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "MALFORMED_QUERY") {
			t.Errorf("expected error containing 'MALFORMED_QUERY', got: %v", err)
		}
	})

	t.Run("401 unauthorized", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode([]map[string]any{
				{
					"message":   "Session expired",
					"errorCode": "INVALID_SESSION_ID",
				},
			})
		}))
		defer server.Close()

		client := NewClient(Config{
			InstanceURL: server.URL,
			APIVersion:  "v61.0",
			TokenSource: &mockTokenSource{token: "expired-token"},
		})

		var result any
		err := client.ToolingQuery(context.Background(), "SELECT Id FROM Certificate", &result)

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "INVALID_SESSION_ID") && !strings.Contains(err.Error(), "401") {
			t.Errorf("expected auth error, got: %v", err)
		}
	})
}
