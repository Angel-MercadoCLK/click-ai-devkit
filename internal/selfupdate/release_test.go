package selfupdate

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFetchLatest_ReturnsTagName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/releases/latest" {
			t.Errorf("expected path /releases/latest, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"tag_name":"v0.5.0","ignored":"fields"}`))
	}))
	defer server.Close()

	client := newHTTPClient()
	tag, err := fetchLatest(client, server.URL+"/releases/latest", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag != "v0.5.0" {
		t.Errorf("expected tag v0.5.0, got %s", tag)
	}
}

func TestFetchLatest_AuthorizationHeader(t *testing.T) {
	tests := []struct {
		name        string
		setTokenEnv func()
		wantHeader  string
	}{
		{
			name: "GITHUB_TOKEN only",
			setTokenEnv: func() {
				os.Setenv("GITHUB_TOKEN", "ghp_test123")
				os.Unsetenv("GH_TOKEN")
			},
			wantHeader: "Bearer ghp_test123",
		},
		{
			name: "GH_TOKEN only",
			setTokenEnv: func() {
				os.Unsetenv("GITHUB_TOKEN")
				os.Setenv("GH_TOKEN", "gho_test456")
			},
			wantHeader: "Bearer gho_test456",
		},
		{
			name: "both tokens (GITHUB_TOKEN wins)",
			setTokenEnv: func() {
				os.Setenv("GITHUB_TOKEN", "ghp_primary")
				os.Setenv("GH_TOKEN", "gho_secondary")
			},
			wantHeader: "Bearer ghp_primary",
		},
		{
			name: "no tokens",
			setTokenEnv: func() {
				os.Unsetenv("GITHUB_TOKEN")
				os.Unsetenv("GH_TOKEN")
			},
			wantHeader: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotHeader string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotHeader = r.Header.Get("Authorization")
				w.WriteHeader(200)
				w.Write([]byte(`{"tag_name":"v0.5.0"}`))
			}))
			defer server.Close()

			tt.setTokenEnv()
			defer func() {
				os.Unsetenv("GITHUB_TOKEN")
				os.Unsetenv("GH_TOKEN")
			}()

			token := githubToken()
			_, err := fetchLatest(newHTTPClient(), server.URL, token)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if gotHeader != tt.wantHeader {
				t.Errorf("expected Authorization header %q, got %q", tt.wantHeader, gotHeader)
			}
		})
	}
}

func TestFetchLatest_UsesBearerScheme(t *testing.T) {
	// This test proves the bug: the code currently uses "token" but should use "Bearer"
	os.Setenv("GITHUB_TOKEN", "test_token_123")
	defer os.Unsetenv("GITHUB_TOKEN")

	var gotHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Authorization")
		w.WriteHeader(200)
		w.Write([]byte(`{"tag_name":"v0.5.0"}`))
	}))
	defer server.Close()

	token := githubToken()
	_, err := fetchLatest(newHTTPClient(), server.URL, token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// RED: this should fail with current code (which sends "token test_token_123")
	wantHeader := "Bearer test_token_123"
	if gotHeader != wantHeader {
		t.Errorf("expected Authorization header %q, got %q (bug: code uses 'token' instead of 'Bearer')", wantHeader, gotHeader)
	}
}

func TestFetchLatest_StatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    bool
	}{
		{"200 success", 200, false},
		{"201 success", 201, false},
		{"299 success", 299, false},
		{"300 redirect error", 300, true},
		{"404 not found", 404, true},
		{"500 server error", 500, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(`{"tag_name":"v0.5.0"}`))
			}))
			defer server.Close()

			_, err := fetchLatest(newHTTPClient(), server.URL, "")
			if (err != nil) != tt.wantErr {
				t.Errorf("fetchLatest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFetchLatest_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"tag_name":invalid`))
	}))
	defer server.Close()

	_, err := fetchLatest(newHTTPClient(), server.URL, "")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestNewHTTPClient_HasTwoSecondTimeout(t *testing.T) {
	client := newHTTPClient()
	if client.Timeout != 2*time.Second {
		t.Errorf("expected 2s timeout, got %v", client.Timeout)
	}
}

func TestFetchLatest_ClientTimeoutBoundsLatency(t *testing.T) {
	// Server that blocks for longer than timeout
	blockDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blockDone
		w.WriteHeader(200)
		w.Write([]byte(`{"tag_name":"v0.5.0"}`))
	}))
	defer server.Close()
	defer close(blockDone)

	// Client with very short timeout for test
	client := &http.Client{Timeout: 10 * time.Millisecond}

	start := time.Now()
	_, err := fetchLatest(client, server.URL, "")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("timeout did not bound latency; got %v, wanted <500ms", elapsed)
	}
}

// fakeRoundTripper implements http.RoundTripper for testing error conditions.
type fakeRoundTripper struct {
	mu           sync.Mutex
	responseErr  error
	closeCalled  bool
	bodyToRead   io.ReadCloser
	responseCode int
	responseBody []byte
}

func (f *fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.responseErr != nil {
		return nil, f.responseErr
	}

	resp := &http.Response{
		StatusCode: f.responseCode,
		Body:       f.bodyToRead,
	}

	if f.responseBody != nil {
		resp.Body = io.NopCloser(strings.NewReader(string(f.responseBody)))
	}

	return resp, nil
}

func TestFetchLatest_TransportErrorPropagates(t *testing.T) {
	fake := &fakeRoundTripper{
		responseErr:  io.EOF,
		responseCode: 200,
	}

	client := &http.Client{
		Transport: fake,
		Timeout:   2 * time.Second,
	}

	_, err := fetchLatest(client, "http://fake.example", "")
	if err == nil {
		t.Fatal("expected transport error")
	}
	if !strings.Contains(err.Error(), "request failed") {
		t.Errorf("expected 'request failed' in error, got %v", err)
	}
}

func TestFetchLatest_BodyReadErrorPropagates(t *testing.T) {
	// Create a reader that will fail on Read
	reader, writer := io.Pipe()

	fake := &fakeRoundTripper{
		responseCode: 200,
		bodyToRead:   reader,
	}

	client := &http.Client{
		Transport: fake,
		Timeout:   2 * time.Second,
	}

	// Start a goroutine that will write to the pipe (but we'll close it to cause read error)
	go func() {
		writer.Close()
	}()

	_, err := fetchLatest(client, "http://fake.example", "")
	if err == nil {
		t.Fatal("expected body read error")
	}
	// The error should be about reading (either "read body" or a pipe error)
}

func TestFetchLatest_AlwaysClosesBody(t *testing.T) {
	// Success case - body should be closed
	{
		closeCalled := false
		fake := &fakeRoundTripper{
			responseCode: 200,
		}

		// Override the body to track Close() calls
		closeTracker := &closeTrackingReader{
			ReadCloser: io.NopCloser(strings.NewReader(`{"tag_name":"v0.5.0"}`)),
			onClose: func() {
				closeCalled = true
			},
		}
		fake.bodyToRead = closeTracker

		client := &http.Client{
			Transport: fake,
			Timeout:   2 * time.Second,
		}

		_, err := fetchLatest(client, "http://fake.example", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Assert the body was closed
		if !closeCalled {
			t.Error("body was not closed in success case")
		}
	}

	// Error case - non-2xx status code (body should still be closed)
	{
		closeCalled := false
		fake := &fakeRoundTripper{
			responseCode: 404, // Non-2xx status
		}

		// Override the body to track Close() calls
		closeTracker := &closeTrackingReader{
			ReadCloser: io.NopCloser(strings.NewReader(`{"tag_name":"v0.5.0"}`)),
			onClose: func() {
				closeCalled = true
			},
		}
		fake.bodyToRead = closeTracker

		client := &http.Client{
			Transport: fake,
			Timeout:   2 * time.Second,
		}

		_, err := fetchLatest(client, "http://fake.example", "")
		if err == nil {
			t.Fatal("expected error for 404 status")
		}

		// Assert the body was closed even when status code is an error
		if !closeCalled {
			t.Error("body was not closed in non-2xx status error case")
		}
	}
}

type closeTrackingReader struct {
	io.ReadCloser
	onClose func()
}

func (c *closeTrackingReader) Close() error {
	c.onClose()
	return c.ReadCloser.Close()
}

func TestFetchLatest_DoesNotFollowRedirects(t *testing.T) {
	// Track whether the redirect target was called
	redirectTargetCalled := false

	// Create the redirect target server (the endpoint we should NOT reach)
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectTargetCalled = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"tag_name":"v9.9.9"}`))
	}))
	defer targetServer.Close()

	// Create the initial server that returns a redirect
	initialServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a 302 redirect to the target server
		w.Header().Set("Location", targetServer.URL)
		w.WriteHeader(http.StatusFound)
	}))
	defer initialServer.Close()

	// Use the default HTTP client (which should NOT follow redirects after fix)
	client := newHTTPClient()

	// Attempt to fetch - this should fail because redirect should be treated as non-2xx
	tag, err := fetchLatest(client, initialServer.URL, "")

	// After the fix, we expect an error (redirect treated as failure)
	// Before the fix, this would succeed and return "v9.9.9"
	if err == nil {
		t.Errorf("expected error when encountering redirect, got nil error and tag=%s", tag)
	}
	if redirectTargetCalled {
		t.Error("redirect target was called - client followed redirect (should NOT happen)")
	}
}
