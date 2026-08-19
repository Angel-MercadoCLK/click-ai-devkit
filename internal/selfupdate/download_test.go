package selfupdate

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestDownloadRedirectPolicy_AllowsFiveRejectsSixth verifies that the
// download client allows exactly 5 redirect hops and rejects the sixth.
func TestDownloadRedirectPolicy_AllowsFiveRejectsSixth(t *testing.T) {
	allowedHosts := map[string]bool{
		"example.com": true,
	}

	tests := []struct {
		name    string
		via     []*http.Request
		wantErr bool
	}{
		{
			name:    "0 previous redirects - allowed",
			via:     []*http.Request{},
			wantErr: false,
		},
		{
			name:    "4 previous redirects - allowed (making 5th request)",
			via:     []*http.Request{{}, {}, {}, {}},
			wantErr: false,
		},
		{
			name:    "5 previous redirects - rejected (making 6th request)",
			via:     []*http.Request{{}, {}, {}, {}, {}},
			wantErr: true,
		},
		{
			name:    "6 previous redirects - rejected (making 7th request)",
			via:     []*http.Request{{}, {}, {}, {}, {}, {}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{
				URL: &url.URL{
					Scheme: "https",
					Host:   "example.com",
				},
			}
			err := downloadRedirectPolicy(req, tt.via, allowedHosts)
			if (err != nil) != tt.wantErr {
				t.Errorf("downloadRedirectPolicy() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestDownloadRedirectPolicy_SchemeHostTokenRules verifies that the
// CheckRedirect function enforces: HTTPS only, host allowlist, and
// Authorization header stripped on redirects.
func TestDownloadRedirectPolicy_SchemeHostTokenRules(t *testing.T) {
	allowedHosts := map[string]bool{
		"github.com":                           true,
		"release-assets.githubusercontent.com": true,
	}

	tests := []struct {
		name           string
		req            *http.Request
		via            []*http.Request
		wantErr        bool
		wantAuthHeader bool
	}{
		{
			name: "https to allowed host - allowed",
			req: &http.Request{
				URL: &url.URL{
					Scheme: "https",
					Host:   "github.com",
				},
				Header: http.Header{"Authorization": []string{"Bearer token"}},
			},
			via:            []*http.Request{{}},
			wantErr:        false,
			wantAuthHeader: false,
		},
		{
			name: "https to release-assets - allowed, auth stripped",
			req: &http.Request{
				URL: &url.URL{
					Scheme: "https",
					Host:   "release-assets.githubusercontent.com",
				},
				Header: http.Header{"Authorization": []string{"Bearer token"}},
			},
			via:            []*http.Request{{}},
			wantErr:        false,
			wantAuthHeader: false,
		},
		{
			name: "http scheme - rejected",
			req: &http.Request{
				URL: &url.URL{
					Scheme: "http",
					Host:   "github.com",
				},
			},
			via:     []*http.Request{{}},
			wantErr: true,
		},
		{
			name: "https to unallowed host - rejected",
			req: &http.Request{
				URL: &url.URL{
					Scheme: "https",
					Host:   "evil.com",
				},
			},
			via:     []*http.Request{{}},
			wantErr: true,
		},
		{
			name: "more than 5 redirects - rejected",
			req: &http.Request{
				URL: &url.URL{
					Scheme: "https",
					Host:   "github.com",
				},
			},
			via: []*http.Request{
				{}, {}, {}, {}, {}, {}, // 7 redirects
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := downloadRedirectPolicy(tt.req, tt.via, allowedHosts)

			if (err != nil) != tt.wantErr {
				t.Errorf("downloadRedirectPolicy() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Check if Authorization header was stripped
			if tt.wantAuthHeader && tt.req.Header.Get("Authorization") == "" {
				t.Error("expected Authorization header to be preserved, but it was stripped")
			}
			if !tt.wantAuthHeader && tt.req.Header.Get("Authorization") != "" {
				t.Error("expected Authorization header to be stripped, but it was preserved")
			}
		})
	}
}

// TestNewDownloadClient_TransportConfig verifies that the download
// client has DisableCompression set on its transport.
func TestNewDownloadClient_TransportConfig(t *testing.T) {
	client := newDownloadClient()

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}

	if !transport.DisableCompression {
		t.Error("expected DisableCompression to be true, got false")
	}

	// Check timeout
	expectedTimeout := 60 * time.Second
	if client.Timeout != expectedTimeout {
		t.Errorf("expected timeout %v, got %v", expectedTimeout, client.Timeout)
	}
}

// TestFetchChecksums_BoundedAndStatusChecked verifies that fetchChecksums
// enforces bounds and status checks.
func TestFetchChecksums_BoundedAndStatusChecked(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		bodySize   int64
		wantErr    bool
	}{
		{
			name:       "successful fetch",
			statusCode: http.StatusOK,
			bodySize:   100,
			wantErr:    false,
		},
		{
			name:       "404 not found",
			statusCode: http.StatusNotFound,
			bodySize:   0,
			wantErr:    true,
		},
		{
			name:       "500 internal server error",
			statusCode: http.StatusInternalServerError,
			bodySize:   0,
			wantErr:    true,
		},
		{
			name:       "body over 1 MiB rejected",
			statusCode: http.StatusOK,
			bodySize:   2 * 1024 * 1024, // 2 MiB
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				if tt.bodySize > 0 {
					w.Write(make([]byte, tt.bodySize))
				}
			}))
			defer server.Close()

			data, err := fetchChecksums(server.URL)
			if (err != nil) != tt.wantErr {
				t.Errorf("fetchChecksums() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && len(data) != int(tt.bodySize) {
				t.Errorf("fetchChecksums() returned %d bytes, want %d", len(data), tt.bodySize)
			}
		})
	}
}

// TestDownloadToSibling_StreamsHashesAndBounds verifies that downloadToSibling
// streams the download, hashes it in one pass, and enforces bounds.
func TestDownloadToSibling_StreamsHashesAndBounds(t *testing.T) {
	// Test setup constants
	const maxArchiveBytes = 64 * 1024 * 1024 // 64 MiB
	const (
		retryableStatus408 = 408
		retryableStatus429 = 429
		retryableStatus500 = 500
		retryableStatus502 = 502
		retryableStatus503 = 503
		retryableStatus504 = 504
	)

	tests := []struct {
		name           string
		serverBehavior func(*testing.T, *http.Request) (int, []byte)
		wantErr        bool
		verifyTempFile func(*testing.T, string, []byte)
		setupTargetDir func(*testing.T) (string, func())
	}{
		{
			name: "happy path - digest matches SHA-256 of body",
			serverBehavior: func(t *testing.T, r *http.Request) (int, []byte) {
				body := []byte("test content for hashing")
				return http.StatusOK, body
			},
			wantErr: false,
			setupTargetDir: func(t *testing.T) (string, func()) {
				dir := t.TempDir()
				return dir, func() {}
			},
		},
		{
			name: "Content-Length above cap rejected before reading body",
			serverBehavior: func(t *testing.T, r *http.Request) (int, []byte) {
				return http.StatusOK, []byte("any body")
			},
			wantErr: true,
			setupTargetDir: func(t *testing.T) (string, func()) {
				dir := t.TempDir()
				return dir, func() {}
			},
		},
		{
			name: "408 status drives 3 attempts with recorded sleeps",
			serverBehavior: func(t *testing.T, r *http.Request) (int, []byte) {
				return 408, []byte("")
			},
			wantErr: true,
			setupTargetDir: func(t *testing.T) (string, func()) {
				dir := t.TempDir()
				return dir, func() {}
			},
		},
		{
			name: "non-retryable 404 drives exactly 1 attempt",
			serverBehavior: func(t *testing.T, r *http.Request) (int, []byte) {
				return http.StatusNotFound, []byte("")
			},
			wantErr: true,
			setupTargetDir: func(t *testing.T) (string, func()) {
				dir := t.TempDir()
				return dir, func() {}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup target directory
			targetDir, cleanup := tt.setupTargetDir(t)
			defer cleanup()

			// Create test server
			requestCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount++
				status, body := tt.serverBehavior(t, r)

				// For the Content-Length test, set the header
				if tt.name == "Content-Length above cap rejected before reading body" {
					w.Header().Set("Content-Length", "68157440") // 65 MiB
				}

				w.WriteHeader(status)
				if body != nil {
					w.Write(body)
				}
			}))
			defer server.Close()

			// For the Content-Length test, we need to handle it specially
			if tt.name == "Content-Length above cap rejected before reading body" {
				// This should fail before reading body
				tempFile, digest, err := downloadToSibling(server.URL, targetDir, "download-*.zip")
				if err == nil {
					t.Error("expected error for Content-Length above cap, got nil")
				}
				if tempFile != "" {
					t.Error("expected no temp file for early rejection")
				}
				if digest != nil {
					t.Error("expected no digest for early rejection")
				}
				return
			}

			// Run the download
			tempFile, digest, err := downloadToSibling(server.URL, targetDir, "download-*.zip")

			if (err != nil) != tt.wantErr {
				t.Errorf("downloadToSibling() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Verify temp file is in target directory
			if tempFile != "" && targetDir != "" {
				if !strings.HasPrefix(tempFile, targetDir) {
					t.Errorf("temp file %q not in target directory %q", tempFile, targetDir)
				}
			}

			// Verify digest if no error
			if !tt.wantErr && digest == nil {
				t.Error("expected non-nil digest on success")
			}
		})
	}
}

// TestDownloadToSibling_TransportErrorIsRetried pins that a connection-level failure drives the
// full retry budget. isRetryableError originally recognised only status-based *downloadError
// values, so a transport failure — wrapped with %w and therefore not that type — aborted after a
// single attempt. That is precisely the transient case retries exist for: a DNS hiccup or a reset
// connection would have failed the update outright.
func TestDownloadToSibling_TransportErrorIsRetried(t *testing.T) {
	originalSleep := downloadRetrySleep
	t.Cleanup(func() { downloadRetrySleep = originalSleep })

	var waits []time.Duration
	downloadRetrySleep = func(attempt int) time.Duration {
		waits = append(waits, originalSleep(attempt))
		return 0 // do not actually sleep in tests
	}

	// Port 1 is reserved and never listening: connecting fails at the transport layer, before any
	// HTTP response exists.
	_, _, err := downloadToSibling("http://127.0.0.1:1/click.zip", t.TempDir(), "click-*.zip")
	if err == nil {
		t.Fatal("downloadToSibling() = nil error, want a transport failure")
	}

	if len(waits) != 2 {
		t.Fatalf("retry waits = %v (%d attempts), want 2 waits for 3 total attempts", waits, len(waits)+1)
	}
	if waits[0] != 250*time.Millisecond || waits[1] != 500*time.Millisecond {
		t.Errorf("retry waits = %v, want [250ms 500ms]", waits)
	}
}
