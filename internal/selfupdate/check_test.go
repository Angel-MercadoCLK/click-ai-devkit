package selfupdate

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

type fakeRoundTripperWithCallback struct {
	responseCode int
	responseBody []byte
	responseErr  error
	onRoundTrip  func()
}

func (f *fakeRoundTripperWithCallback) RoundTrip(req *http.Request) (*http.Response, error) {
	if f.onRoundTrip != nil {
		f.onRoundTrip()
	}
	if f.responseErr != nil {
		return nil, f.responseErr
	}
	resp := &http.Response{
		StatusCode: f.responseCode,
		Body:       io.NopCloser(strings.NewReader(string(f.responseBody))),
	}
	return resp, nil
}

func TestCheck_SkipsWithoutTouchingCacheOrNetwork(t *testing.T) {
	tests := []struct {
		name         string
		current      string
		setEnv       func()
		httpCalled   *bool
		expectNotice bool
	}{
		{
			name:    "opt-out env var",
			current: "0.4.0",
			setEnv: func() {
				os.Setenv("CLICK_NO_SELF_UPDATE", "1")
			},
			expectNotice: false,
		},
		{
			name:    "dev version",
			current: "dev",
			setEnv: func() {
				os.Unsetenv("CLICK_NO_SELF_UPDATE")
			},
			expectNotice: false,
		},
		{
			name:    "non-comparable version",
			current: "v1.x.3",
			setEnv: func() {
				os.Unsetenv("CLICK_NO_SELF_UPDATE")
			},
			expectNotice: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup temp state home
			stateHome := t.TempDir()
			t.Setenv("CLICK_STATE_HOME", stateHome)

			tt.setEnv()
			defer func() {
				os.Unsetenv("CLICK_NO_SELF_UPDATE")
			}()

			httpCalled := false
			originalNewClient := newHTTPClient
			newHTTPClient = func() *http.Client {
				return &http.Client{
					Transport: &fakeRoundTripperWithCallback{
						responseCode: 200,
						responseBody: []byte(`{"tag_name":"v0.5.0"}`),
						onRoundTrip: func() {
							httpCalled = true
						},
					},
					Timeout: 2 * time.Second,
				}
			}
			defer func() { newHTTPClient = originalNewClient }()

			_, available := Check(tt.current)

			if available != tt.expectNotice {
				t.Errorf("Check() available = %v, want %v", available, tt.expectNotice)
			}

			if httpCalled {
				t.Error("network should not have been called")
			}

			// Verify cache file was not created
			cachePath := stateHome + "/" + cacheFile
			if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
				t.Error("cache file should not have been created")
			}
		})
	}
}

func TestCheck_ColdStartFetchSuccessWritesCacheAndNotices(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("CLICK_STATE_HOME", stateHome)

	fixedTime := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	originalNow := now
	now = func() time.Time { return fixedTime }
	defer func() { now = originalNow }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"tag_name":"v0.5.0"}`))
	}))
	defer server.Close()

	originalURL := latestReleaseURL
	latestReleaseURL = server.URL
	defer func() { latestReleaseURL = originalURL }()

	notice, available := Check("0.4.0")

	if !available {
		t.Fatal("expected update available")
	}

	if notice.Latest != "v0.5.0" {
		t.Errorf("expected Latest v0.5.0, got %s", notice.Latest)
	}

	if notice.Current != "0.4.0" {
		t.Errorf("expected Current 0.4.0, got %s", notice.Current)
	}

	// Verify cache was written
	cachePath := stateHome + "/" + cacheFile
	entry, err := readCache(cachePath)
	if err != nil {
		t.Fatalf("read cache failed: %v", err)
	}

	if !entry.CheckedAt.Equal(fixedTime) {
		t.Errorf("expected CheckedAt %v, got %v", fixedTime, entry.CheckedAt)
	}

	if entry.Latest != "v0.5.0" {
		t.Errorf("expected Latest v0.5.0 in cache, got %s", entry.Latest)
	}
}

func TestCheck_WithinTTLUsesCacheWithoutFetching(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("CLICK_STATE_HOME", stateHome)

	// Seed cache with 23h old entry
	oldTime := time.Now().Add(-23 * time.Hour)
	cachePath := stateHome + "/" + cacheFile
	writeCache(cachePath, cacheEntry{
		CheckedAt: oldTime,
		Latest:    "v0.5.0",
	})

	httpCalled := false
	originalNewClient := newHTTPClient
	newHTTPClient = func() *http.Client {
		return &http.Client{
			Transport: &fakeRoundTripperWithCallback{
				responseCode: 200,
				responseBody: []byte(`{"tag_name":"v0.6.0"}`),
				onRoundTrip: func() {
					httpCalled = true
				},
			},
			Timeout: 2 * time.Second,
		}
	}
	defer func() { newHTTPClient = originalNewClient }()

	// Current is older: should notice
	notice, available := Check("0.4.0")

	if httpCalled {
		t.Error("network should not have been called within TTL")
	}

	if !available {
		t.Fatal("expected update available from cache")
	}

	if notice.Latest != "v0.5.0" {
		t.Errorf("expected Latest v0.5.0 from cache, got %s", notice.Latest)
	}

	// Current is newer: should not notice
	_, available = Check("v0.6.0")

	if available {
		t.Fatal("expected no notice when current >= latest from cache")
	}
}

func TestCheck_AtTTLBoundaryFetches(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("CLICK_STATE_HOME", stateHome)

	// Seed cache with exactly 24h old entry
	oldTime := time.Now().Add(-24 * time.Hour)
	cachePath := stateHome + "/" + cacheFile
	writeCache(cachePath, cacheEntry{
		CheckedAt: oldTime,
		Latest:    "v0.5.0",
	})

	httpCalled := false
	originalNewClient := newHTTPClient
	newHTTPClient = func() *http.Client {
		return &http.Client{
			Transport: &fakeRoundTripperWithCallback{
				responseCode: 200,
				responseBody: []byte(`{"tag_name":"v0.6.0"}`),
				onRoundTrip: func() {
					httpCalled = true
				},
			},
			Timeout: 2 * time.Second,
		}
	}
	defer func() { newHTTPClient = originalNewClient }()

	notice, available := Check("0.4.0")

	if !httpCalled {
		t.Error("network should have been called at TTL boundary")
	}

	if !available {
		t.Fatal("expected update available")
	}

	if notice.Latest != "v0.6.0" {
		t.Errorf("expected Latest v0.6.0 from fresh fetch, got %s", notice.Latest)
	}
}

func TestCheck_FailureUpdatesCheckedAtPreservesLatest(t *testing.T) {
	tests := []struct {
		name       string
		setupFake  func() *fakeRoundTripperWithCallback
		expectDiff bool
	}{
		{
			name: "transport error",
			setupFake: func() *fakeRoundTripperWithCallback {
				return &fakeRoundTripperWithCallback{
					responseErr: errors.New("network error"),
				}
			},
			expectDiff: false,
		},
		{
			name: "non-2xx status",
			setupFake: func() *fakeRoundTripperWithCallback {
				return &fakeRoundTripperWithCallback{
					responseCode: 404,
				}
			},
			expectDiff: false,
		},
		{
			name: "malformed JSON",
			setupFake: func() *fakeRoundTripperWithCallback {
				return &fakeRoundTripperWithCallback{
					responseCode: 200,
					responseBody: []byte(`{invalid`),
				}
			},
			expectDiff: false,
		},
		{
			name: "non-comparable tag",
			setupFake: func() *fakeRoundTripperWithCallback {
				return &fakeRoundTripperWithCallback{
					responseCode: 200,
					responseBody: []byte(`{"tag_name":"v1.x.3"}`),
				}
			},
			expectDiff: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stateHome := t.TempDir()
			t.Setenv("CLICK_STATE_HOME", stateHome)

			// Seed cache with old entry
			oldTime := time.Now().Add(-25 * time.Hour)
			cachePath := stateHome + "/" + cacheFile
			writeCache(cachePath, cacheEntry{
				CheckedAt: oldTime,
				Latest:    "v0.5.0",
			})

			attemptTime := time.Now()
			originalNow := now
			now = func() time.Time { return attemptTime }
			defer func() { now = originalNow }()

			originalNewClient := newHTTPClient
			newHTTPClient = func() *http.Client {
				return &http.Client{
					Transport: tt.setupFake(),
					Timeout:   2 * time.Second,
				}
			}
			defer func() { newHTTPClient = originalNewClient }()

			_, available := Check("0.4.0")

			if available {
				t.Fatal("expected no notice on failure")
			}

			// Verify checked_at was updated but Latest preserved
			entry, err := readCache(cachePath)
			if err != nil {
				t.Fatalf("read cache failed: %v", err)
			}

			if !entry.CheckedAt.Equal(attemptTime) {
				t.Errorf("expected CheckedAt %v, got %v", attemptTime, entry.CheckedAt)
			}

			if entry.Latest != "v0.5.0" {
				t.Errorf("expected Latest preserved as v0.5.0, got %s", entry.Latest)
			}
		})
	}
}

func TestCheck_PreservedTagNoticesOnNextWithinTTLLaunch(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("CLICK_STATE_HOME", stateHome)

	// Seed cache with old entry
	oldTime := time.Now().Add(-25 * time.Hour)
	cachePath := stateHome + "/" + cacheFile
	writeCache(cachePath, cacheEntry{
		CheckedAt: oldTime,
		Latest:    "v0.5.0",
	})

	// First call: fail (transport error)
	originalNewClient := newHTTPClient
	newHTTPClient = func() *http.Client {
		return &http.Client{
			Transport: &fakeRoundTripperWithCallback{
				responseErr: errors.New("network error"),
			},
			Timeout: 2 * time.Second,
		}
	}

	_, available := Check("0.4.0")
	if available {
		t.Fatal("expected no notice on failure")
	}

	// Second call: within TTL, should use preserved tag
	newHTTPClient = func() *http.Client {
		return &http.Client{
			Transport: &fakeRoundTripperWithCallback{
				responseCode: 200,
				responseBody: []byte(`{"tag_name":"v0.6.0"}`),
			},
			Timeout: 2 * time.Second,
		}
	}

	notice, available := Check("0.4.0")
	if !available {
		t.Fatal("expected notice from preserved tag")
	}

	if notice.Latest != "v0.5.0" {
		t.Errorf("expected Latest v0.5.0 from preserved tag, got %s", notice.Latest)
	}

	newHTTPClient = originalNewClient
}

func TestCheck_StateHomeResolutionFailureStillChecksNetwork(t *testing.T) {
	// Override resolveStateHome to fail
	originalResolve := resolveStateHome
	resolveStateHome = func() (string, error) {
		return "", errors.New("state home error")
	}
	defer func() { resolveStateHome = originalResolve }()

	httpCalled := false
	originalNewClient := newHTTPClient
	newHTTPClient = func() *http.Client {
		return &http.Client{
			Transport: &fakeRoundTripperWithCallback{
				responseCode: 200,
				responseBody: []byte(`{"tag_name":"v0.5.0"}`),
				onRoundTrip: func() {
					httpCalled = true
				},
			},
			Timeout: 2 * time.Second,
		}
	}
	defer func() { newHTTPClient = originalNewClient }()

	notice, available := Check("0.4.0")

	if !httpCalled {
		t.Fatal("network should have been called despite state home error")
	}

	if !available {
		t.Fatal("expected notice")
	}

	if notice.Latest != "v0.5.0" {
		t.Errorf("expected Latest v0.5.0, got %s", notice.Latest)
	}
}

func TestCheck_CacheWriteFailureIsSilent(t *testing.T) {
	stateHome, err := os.CreateTemp("", "statehome")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(stateHome.Name())

	// State home is a file, not a directory - cache write will fail
	t.Setenv("CLICK_STATE_HOME", stateHome.Name())

	originalNewClient := newHTTPClient
	newHTTPClient = func() *http.Client {
		return &http.Client{
			Transport: &fakeRoundTripperWithCallback{
				responseCode: 200,
				responseBody: []byte(`{"tag_name":"v0.5.0"}`),
			},
			Timeout: 2 * time.Second,
		}
	}
	defer func() { newHTTPClient = originalNewClient }()

	// Should not panic
	notice, available := Check("0.4.0")

	// Notice should still be returned
	if !available {
		t.Fatal("expected notice despite cache write failure")
	}

	if notice.Latest != "v0.5.0" {
		t.Errorf("expected Latest v0.5.0, got %s", notice.Latest)
	}
}

func TestCheck_MalformedCacheIsMissNotFatal(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("CLICK_STATE_HOME", stateHome)

	// Write garbage to cache
	cachePath := stateHome + "/" + cacheFile
	os.WriteFile(cachePath, []byte("garbage bytes"), 0o600)

	originalNewClient := newHTTPClient
	newHTTPClient = func() *http.Client {
		return &http.Client{
			Transport: &fakeRoundTripperWithCallback{
				responseCode: 200,
				responseBody: []byte(`{"tag_name":"v0.5.0"}`),
			},
			Timeout: 2 * time.Second,
		}
	}
	defer func() { newHTTPClient = originalNewClient }()

	// Should not panic, should fetch fresh
	notice, available := Check("0.4.0")

	if !available {
		t.Fatal("expected notice despite malformed cache")
	}

	if notice.Latest != "v0.5.0" {
		t.Errorf("expected Latest v0.5.0, got %s", notice.Latest)
	}
}
