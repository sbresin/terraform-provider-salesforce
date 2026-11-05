package uploader

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUploader_Upload(t *testing.T) {
	t.Run("successful upload", func(t *testing.T) {
		requestCount := 0
		var csrfToken string

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount++

			switch {
			case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "KeyStoreImportUi"):
				// Return form page with CSRF token
				csrfToken = "test-csrf-token-12345"
				w.Header().Set("Content-Type", "text/html")
				w.Write([]byte(`<!DOCTYPE html>
<html>
<body>
<form method="POST">
  <input type="hidden" name="_CONFIRMATIONTOKEN" value="` + csrfToken + `"/>
  <input type="hidden" name="retURL" value="/0P1"/>
  <input type="file" name="ksFile"/>
  <input type="password" name="Password"/>
  <input type="submit" name="save" value="Save"/>
</form>
</body>
</html>`))

			case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "KeyStoreImportUi"):
				// Verify form submission
				if err := r.ParseMultipartForm(10 << 20); err != nil {
					t.Errorf("failed to parse multipart form: %v", err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}

				// Check CSRF token
				if token := r.FormValue("_CONFIRMATIONTOKEN"); token != csrfToken {
					t.Errorf("expected CSRF token %q, got %q", csrfToken, token)
				}

				// Check password
				if pwd := r.FormValue("Password"); pwd != "test-password" {
					t.Errorf("expected password 'test-password', got %q", pwd)
				}

				// Check file was uploaded
				file, header, err := r.FormFile("ksFile")
				if err != nil {
					t.Errorf("failed to get uploaded file: %v", err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				defer file.Close()

				if header.Filename != "test.jks" {
					t.Errorf("expected filename 'test.jks', got %q", header.Filename)
				}

				content, _ := io.ReadAll(file)
				if string(content) != "fake-jks-content" {
					t.Errorf("unexpected file content: %q", string(content))
				}

				// Redirect to success page (simulating successful upload)
				// Use relative URL since server.URL isn't available in closure
				w.Header().Set("Location", "/0P1/certificate-list")
				w.WriteHeader(http.StatusFound)

			case r.URL.Path == "/0P1/certificate-list":
				// Success page
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("Certificate List"))

			default:
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		cookies := []*http.Cookie{
			{Name: "sid", Value: "test-session-id"},
			{Name: "oid", Value: "test-org-id"},
		}

		uploader := NewUploader(server.URL, cookies)
		err := uploader.Upload(context.Background(), "test.jks", []byte("fake-jks-content"), "test-password")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if requestCount != 2 {
			t.Errorf("expected 2 requests (get form, post form), got %d", requestCount)
		}
	})

	t.Run("upload error - form error message", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				w.Write([]byte(`<form><input name="_CONFIRMATIONTOKEN" value="token"/></form>`))
			case http.MethodPost:
				// Return error page (still on form URL)
				w.Header().Set("Content-Type", "text/html")
				w.Write([]byte(`<!DOCTYPE html>
<html>
<body>
<div class="errorMsg">Invalid keystore format</div>
</body>
</html>`))
			}
		}))
		defer server.Close()

		cookies := []*http.Cookie{{Name: "sid", Value: "test-session-id"}}
		uploader := NewUploader(server.URL, cookies)
		err := uploader.Upload(context.Background(), "bad.jks", []byte("invalid"), "password")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Invalid keystore format") {
			t.Errorf("expected error to contain 'Invalid keystore format', got: %v", err)
		}
	})

	t.Run("missing CSRF token", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`<form><input name="other" value="value"/></form>`))
		}))
		defer server.Close()

		cookies := []*http.Cookie{{Name: "sid", Value: "test-session-id"}}
		uploader := NewUploader(server.URL, cookies)
		err := uploader.Upload(context.Background(), "test.jks", []byte("content"), "password")

		if err == nil {
			t.Fatal("expected error for missing CSRF token, got nil")
		}
	})
}
