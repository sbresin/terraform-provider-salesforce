package uploader

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
)

// Uploader handles certificate uploads via Salesforce UI form POST.
type Uploader struct {
	instanceURL string
	cookies     []*http.Cookie
}

// NewUploader creates a new certificate uploader.
// The cookies should be obtained from a SessionSource.
func NewUploader(instanceURL string, cookies []*http.Cookie) *Uploader {
	return &Uploader{
		instanceURL: instanceURL,
		cookies:     cookies,
	}
}

// Upload uploads a JKS keystore to Salesforce Certificate and Key Management.
// filename is used for the uploaded file name.
// keystoreData is the raw JKS file content.
// password is the keystore password.
func (u *Uploader) Upload(ctx context.Context, filename string, keystoreData []byte, password string) error {
	// Step 1: Fetch the form page to get CSRF token
	csrfToken, formFields, err := u.fetchFormPage(ctx)
	if err != nil {
		return fmt.Errorf("fetching form page: %w", err)
	}

	// Step 2: Submit the form with the keystore
	if err := u.submitForm(ctx, filename, keystoreData, password, csrfToken, formFields); err != nil {
		return fmt.Errorf("submitting form: %w", err)
	}

	return nil
}

func (u *Uploader) createClient() *http.Client {
	// Create a client with cookie jar
	jar, _ := cookiejar.New(nil)

	// Add our session cookies to the jar
	instanceURL, _ := url.Parse(u.instanceURL)
	jar.SetCookies(instanceURL, u.cookies)

	return &http.Client{
		Jar: jar,
		// Don't follow redirects automatically for form submission
		// so we can check where we end up
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (u *Uploader) fetchFormPage(ctx context.Context) (string, map[string]string, error) {
	formURL := u.instanceURL + "/_ui/security/certificate/KeyStoreImportUi/e"

	client := u.createClient()
	// For GET request, follow redirects
	client.CheckRedirect = nil

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, formURL, nil)
	if err != nil {
		return "", nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body[:min(500, len(body))]))
	}

	// Parse CSRF token and hidden fields from HTML
	csrfToken, formFields := parseFormFields(string(body))

	if csrfToken == "" {
		return "", nil, fmt.Errorf("CSRF token (_CONFIRMATIONTOKEN) not found in form")
	}

	return csrfToken, formFields, nil
}

func (u *Uploader) submitForm(ctx context.Context, filename string, keystoreData []byte, password, csrfToken string, formFields map[string]string) error {
	formURL := u.instanceURL + "/_ui/security/certificate/KeyStoreImportUi/e"

	// Build multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add CSRF token
	if err := writer.WriteField("_CONFIRMATIONTOKEN", csrfToken); err != nil {
		return fmt.Errorf("writing CSRF token: %w", err)
	}

	// Add password
	if err := writer.WriteField("Password", password); err != nil {
		return fmt.Errorf("writing password: %w", err)
	}

	// Add other hidden fields
	for _, key := range []string{"retURL", "cancelURL", "save_new_url", "setupid"} {
		if val, ok := formFields[key]; ok {
			if err := writer.WriteField(key, val); err != nil {
				return fmt.Errorf("writing field %s: %w", key, err)
			}
		}
	}

	// Add save button
	if err := writer.WriteField("save", "Save"); err != nil {
		return fmt.Errorf("writing save field: %w", err)
	}

	// Add keystore file
	fileWriter, err := writer.CreateFormFile("ksFile", filename)
	if err != nil {
		return fmt.Errorf("creating form file: %w", err)
	}
	if _, err := fileWriter.Write(keystoreData); err != nil {
		return fmt.Errorf("writing file content: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("closing multipart writer: %w", err)
	}

	// Create request
	client := u.createClient()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, formURL, &buf)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	// Check for redirect (success) vs staying on form page (error)
	if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusSeeOther {
		location := resp.Header.Get("Location")
		if !strings.Contains(location, "KeyStoreImportUi") {
			// Redirected away from form = success
			return nil
		}
	}

	// Check for error messages in response
	if err := checkForErrors(string(body)); err != nil {
		return err
	}

	// If status is 200 and no errors found, check if we're still on the form
	if resp.StatusCode == http.StatusOK {
		if strings.Contains(string(body), "KeyStoreImportUi") || strings.Contains(string(body), "_CONFIRMATIONTOKEN") {
			// Still on form page without explicit error - might be an issue
			return fmt.Errorf("upload may have failed: still on form page")
		}
	}

	return nil
}

// parseFormFields extracts the CSRF token and hidden fields from HTML.
func parseFormFields(html string) (string, map[string]string) {
	fields := make(map[string]string)
	var csrfToken string

	// Simple regex-based parsing for hidden input fields
	// Pattern: <input type="hidden" name="..." value="..."/>
	inputPattern := regexp.MustCompile(`<input[^>]*name=["']([^"']+)["'][^>]*value=["']([^"']*)["'][^>]*>`)
	inputPatternAlt := regexp.MustCompile(`<input[^>]*value=["']([^"']*)["'][^>]*name=["']([^"']+)["'][^>]*>`)

	for _, match := range inputPattern.FindAllStringSubmatch(html, -1) {
		name, value := match[1], match[2]
		fields[name] = value
		if name == "_CONFIRMATIONTOKEN" {
			csrfToken = value
		}
	}

	for _, match := range inputPatternAlt.FindAllStringSubmatch(html, -1) {
		value, name := match[1], match[2]
		if _, exists := fields[name]; !exists {
			fields[name] = value
			if name == "_CONFIRMATIONTOKEN" {
				csrfToken = value
			}
		}
	}

	return csrfToken, fields
}

// checkForErrors looks for error messages in the HTML response.
func checkForErrors(html string) error {
	// Common Salesforce error message patterns
	errorPatterns := []string{
		`<div[^>]*class="[^"]*errorMsg[^"]*"[^>]*>([^<]+)</div>`,
		`<span[^>]*class="[^"]*errorMsg[^"]*"[^>]*>([^<]+)</span>`,
		`<div[^>]*class="[^"]*message errorM3[^"]*"[^>]*>([^<]+)</div>`,
	}

	for _, pattern := range errorPatterns {
		re := regexp.MustCompile(pattern)
		if match := re.FindStringSubmatch(html); len(match) > 1 {
			errorMsg := strings.TrimSpace(match[1])
			if errorMsg != "" {
				return fmt.Errorf("upload failed: %s", errorMsg)
			}
		}
	}

	return nil
}
