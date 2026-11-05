package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionSource_Session(t *testing.T) {
	t.Run("successful session establishment", func(t *testing.T) {
		// Track request sequence
		requestCount := 0
		var serverURL string

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/services/oauth2/singleaccess":
				// Step 1: Return frontdoor URL
				if auth := r.Header.Get("Authorization"); auth != "Bearer test-access-token" {
					t.Errorf("expected Bearer token, got %q", auth)
				}
				requestCount++
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{
					"frontdoor_uri": serverURL + "/secur/frontdoor.jsp?otp=abc123",
				})

			case "/secur/frontdoor.jsp":
				// Step 2: Set session cookies and redirect
				requestCount++
				http.SetCookie(w, &http.Cookie{Name: "sid", Value: "session-id-12345"})
				http.SetCookie(w, &http.Cookie{Name: "oid", Value: "org-id-67890"})
				w.Header().Set("Location", serverURL+"/home/home.jsp")
				w.WriteHeader(http.StatusFound)

			case "/home/home.jsp":
				// Step 3: Final redirect target
				requestCount++
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("Home Page"))

			default:
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()
		serverURL = server.URL

		tokenSource := &mockTokenSource{token: "test-access-token"}
		sessionSource := NewSessionSource(tokenSource, server.URL)

		creds, err := sessionSource.Session(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if creds.SessionID != "session-id-12345" {
			t.Errorf("expected session ID 'session-id-12345', got %q", creds.SessionID)
		}

		if creds.InstanceURL != server.URL {
			t.Errorf("expected instance URL %q, got %q", server.URL, creds.InstanceURL)
		}

		// Check cookies were captured
		var hasSid, hasOid bool
		for _, c := range creds.Cookies {
			if c.Name == "sid" {
				hasSid = true
			}
			if c.Name == "oid" {
				hasOid = true
			}
		}
		if !hasSid {
			t.Error("expected 'sid' cookie in credentials")
		}
		if !hasOid {
			t.Error("expected 'oid' cookie in credentials")
		}

		if requestCount != 3 {
			t.Errorf("expected 3 requests, got %d", requestCount)
		}
	})

	t.Run("singleaccess error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("Session expired"))
		}))
		defer server.Close()

		tokenSource := &mockTokenSource{token: "expired-token"}
		sessionSource := NewSessionSource(tokenSource, server.URL)

		_, err := sessionSource.Session(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("missing sid cookie", func(t *testing.T) {
		var serverURL string

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/services/oauth2/singleaccess":
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{
					"frontdoor_uri": serverURL + "/secur/frontdoor.jsp?otp=abc123",
				})
			case "/secur/frontdoor.jsp":
				// No cookies set!
				w.WriteHeader(http.StatusOK)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()
		serverURL = server.URL

		tokenSource := &mockTokenSource{token: "test-token"}
		sessionSource := NewSessionSource(tokenSource, server.URL)

		_, err := sessionSource.Session(context.Background())
		if err == nil {
			t.Fatal("expected error for missing sid cookie, got nil")
		}
	})
}
