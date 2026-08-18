// Package selfupdate provides silent self-update notification for the click CLI.
//
// The Check() function determines if an update is available by:
//   - Comparing versions using a numeric semantic version comparator
//   - Fetching the latest release from GitHub with a 2s timeout
//   - Caching results for 24 hours with atomic file writes
//   - Gracefully degrading on all failures (network, cache, parsing)
//
// All failures are silent: no errors are returned to the caller, no diagnostics
// are printed, and the CLI continues to operate normally. This ensures that
// update checks never block or interfere with the primary CLI functionality.
//
// The cache is updated with the checked_at timestamp on every attempt (success
// or failure) to prevent retry loops, while the latest version is only updated
// on successful fetches with valid comparable tags.
package selfupdate

import (
	"os"
	"time"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

// Notice holds information about an available update.
type Notice struct {
	Latest  string
	Current string
}

// checkForUpdate is a seam for testing.
var checkForUpdate = Check

// resolveStateHome is a seam for testing.
var resolveStateHome = installer.ResolveClickStateHome

// now is a seam for testing.
var now = time.Now

const cacheFile = "update-check.json"
const cacheTTL = 24 * time.Hour

// Check performs an update check for the current version.
// Returns a Notice and a boolean indicating whether an update is available.
// The check is silent: all failures result in no error and no notice.
// - Skips if CLICK_NO_SELF_UPDATE="1"
// - Skips if current is non-comparable (e.g., "dev", malformed)
// - Uses cached result if within TTL
// - Fetches fresh result after TTL
// - Any failure (network, decode, etc.) is silent and preserves cache
func Check(current string) (Notice, bool) {
	// Opt-out env var
	if os.Getenv("CLICK_NO_SELF_UPDATE") == "1" {
		return Notice{}, false
	}

	// Skip non-comparable versions
	_, comparable := compareVersions(current, current)
	if !comparable {
		return Notice{}, false
	}

	attemptTime := now()

	// Resolve cache file path
	stateHome, err := resolveStateHome()
	if err != nil {
		// State home resolution failure: still attempt network check
		return attemptNetworkCheck(current, attemptTime)
	}

	cachePath := stateHome + "/" + cacheFile

	// Try to read existing cache
	entry, err := readCache(cachePath)
	if err == nil && !entry.CheckedAt.IsZero() {
		// Within TTL? Use cache without network call
		if attemptTime.Sub(entry.CheckedAt) < cacheTTL {
			if entry.Latest != "" {
				order, comp := compareVersions(entry.Latest, current)
				if comp && order > 0 {
					return Notice{
						Latest:  entry.Latest,
						Current: current,
					}, true
				}
			}
			return Notice{}, false
		}
	}

	// Cache miss or expired: fetch from network
	return attemptNetworkCheck(current, attemptTime)
}

// attemptNetworkCheck tries to fetch the latest release from GitHub.
// Returns a Notice if an update is available, otherwise no notice.
// All failures are silent and will update the cache checked_at time.
func attemptNetworkCheck(current string, attemptTime time.Time) (Notice, bool) {
	// Resolve cache file path
	stateHome, err := resolveStateHome()
	cachePath := stateHome + "/" + cacheFile

	// Read existing cache to potentially preserve Latest on failure
	var cachedLatest string
	if err == nil {
		if entry, err := readCache(cachePath); err == nil {
			cachedLatest = entry.Latest
		}
	}

	// Fetch latest release
	client := newHTTPClient()
	token := githubToken()
	latest, err := fetchLatest(client, latestReleaseURL, token)

	entry := cacheEntry{
		CheckedAt: attemptTime,
		Latest:    cachedLatest, // Preserve old Latest on failure
	}

	if err != nil {
		// Network or decode failure: update checked_at but keep old Latest
		writeCacheSilent(cachePath, entry)
		return Notice{}, false
	}

	// Validate fetched tag is comparable
	_, comp := compareVersions(latest, current)
	if !comp {
		// Fetched tag is non-comparable: don't update Latest, but update checked_at
		writeCacheSilent(cachePath, entry)
		return Notice{}, false
	}

	// Success: update Latest
	entry.Latest = latest
	writeCacheSilent(cachePath, entry)

	// Check if latest > current
	order, _ := compareVersions(latest, current)
	if order > 0 {
		return Notice{
			Latest:  latest,
			Current: current,
		}, true
	}

	return Notice{}, false
}

// writeCacheSilent writes the cache, ignoring any errors.
func writeCacheSilent(path string, entry cacheEntry) {
	_ = writeCache(path, entry)
}
