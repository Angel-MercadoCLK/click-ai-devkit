package selfupdate

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newerReleaseServer returns a test server that always reports a release newer
// than the versions used by the tests in this file.
func newerReleaseServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// captureCacheWrites swaps writeCacheSilent for a recorder and restores it after
// the test. It returns a pointer to the recorded calls.
func captureCacheWrites(t *testing.T) *[]string {
	t.Helper()
	original := writeCacheSilent
	t.Cleanup(func() { writeCacheSilent = original })

	var paths []string
	writeCacheSilent = func(path string, entry cacheEntry) {
		paths = append(paths, path)
	}
	return &paths
}

// TestCheck_UnresolvableStateHomeNeverWritesCache pins that a failure to resolve
// the click state home disables cache persistence entirely, instead of falling
// back to a path derived from an empty root (which resolves to the filesystem
// root, e.g. "/update-check.json").
func TestCheck_UnresolvableStateHomeNeverWritesCache(t *testing.T) {
	srv := newerReleaseServer(t)

	originalResolve := resolveStateHome
	originalURL := latestReleaseURL
	t.Cleanup(func() {
		resolveStateHome = originalResolve
		latestReleaseURL = originalURL
	})
	resolveStateHome = func() (string, error) {
		return "", errors.New("selfupdate: no user config dir")
	}
	latestReleaseURL = srv.URL

	writes := captureCacheWrites(t)

	notice, ok := Check("1.0.0")

	// The network check must still run and still report the newer release.
	if !ok {
		t.Fatalf("Check() = %+v, %v; want a notice even when the cache is unusable", notice, ok)
	}
	if notice.Latest != "v9.9.9" {
		t.Errorf("notice.Latest = %q, want %q", notice.Latest, "v9.9.9")
	}

	// But nothing may be persisted, at any path.
	if len(*writes) != 0 {
		t.Errorf("expected zero cache writes when the state home is unresolvable, got writes to %q", *writes)
	}
}

// TestCheck_CachePathUsesPlatformSeparator pins that the cache path is built
// with filepath.Join rather than string concatenation, so it does not mix
// separators on Windows.
func TestCheck_CachePathUsesPlatformSeparator(t *testing.T) {
	srv := newerReleaseServer(t)
	stateHome := t.TempDir()

	originalResolve := resolveStateHome
	originalURL := latestReleaseURL
	t.Cleanup(func() {
		resolveStateHome = originalResolve
		latestReleaseURL = originalURL
	})
	resolveStateHome = func() (string, error) { return stateHome, nil }
	latestReleaseURL = srv.URL

	writes := captureCacheWrites(t)

	if _, ok := Check("1.0.0"); !ok {
		t.Fatal("Check() reported no notice; want one so a cache write is attempted")
	}

	if len(*writes) != 1 {
		t.Fatalf("expected exactly one cache write, got %d: %q", len(*writes), *writes)
	}

	want := filepath.Join(stateHome, cacheFile)
	if got := (*writes)[0]; got != want {
		t.Errorf("cache path = %q, want %q", got, want)
	}
}

// TestFetchLatest_LimitsResponseBodySize pins that an oversized response body is
// truncated rather than read into memory in full. A body past the limit is cut
// mid-JSON, so decoding fails and the check degrades silently like any other
// malformed response.
func TestFetchLatest_LimitsResponseBodySize(t *testing.T) {
	oversized := `{"tag_name":"v9.9.9","padding":"` + strings.Repeat("x", 2*maxResponseBytes) + `"}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(oversized))
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 10 * time.Second}

	tag, err := fetchLatest(client, srv.URL, "")
	if err == nil {
		t.Fatalf("fetchLatest() = %q, nil; want an error from the truncated body", tag)
	}
}
