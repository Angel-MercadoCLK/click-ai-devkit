// Package selfupdate provides silent self-update notification and installation
// method classification for the click CLI.
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
//
// The package also provides installation method detection through DetectInstallation(),
// which classifies the current CLI installation as Scoop, standalone, or unknown
// based on metadata files and shim indirection patterns.
package selfupdate

import (
	"os"
	"path/filepath"
	"time"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

// Notice holds information about an available update.
type Notice struct {
	Latest  string
	Current string
}

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

	// Skip non-comparable versions (e.g. the "dev" default of a local build).
	// Comparing current against itself is the cheapest way to ask the
	// comparator whether it can parse this version at all.
	if _, isComparable := compareVersions(current, current); !isComparable {
		return Notice{}, false
	}

	attemptTime := now()

	// Resolve the cache file path. An empty path means persistence is
	// impossible (no resolvable state home): the check still runs, it simply
	// cannot be cached. It must never fall back to a path built from an empty
	// root, which would resolve to the filesystem root.
	var cachePath string
	if stateHome, err := resolveStateHome(); err == nil {
		cachePath = filepath.Join(stateHome, cacheFile)
	}

	// Read the cache once and reuse it both for the TTL decision and, on a
	// miss, to preserve the last known tag across a failed refresh.
	var cachedLatest string
	if cachePath != "" {
		if entry, err := readCache(cachePath); err == nil && !entry.CheckedAt.IsZero() {
			cachedLatest = entry.Latest
			if attemptTime.Sub(entry.CheckedAt) < cacheTTL {
				return noticeIfNewer(entry.Latest, current)
			}
		}
	}

	// Cache miss or expired: fetch from network
	return attemptNetworkCheck(current, attemptTime, cachePath, cachedLatest)
}

// noticeIfNewer reports an update notice when latest is a comparable version
// strictly greater than current. An empty or non-comparable latest is silently
// treated as "nothing to report".
func noticeIfNewer(latest, current string) (Notice, bool) {
	if latest == "" {
		return Notice{}, false
	}
	order, comp := compareVersions(latest, current)
	if comp && order > 0 {
		return Notice{Latest: latest, Current: current}, true
	}
	return Notice{}, false
}

// attemptNetworkCheck tries to fetch the latest release from GitHub.
// Returns a Notice if an update is available, otherwise no notice.
// All failures are silent. Every attempt — success or failure — records
// checked_at so an unreachable network is retried at most once per TTL rather
// than on every launch. cachePath may be empty, in which case nothing is
// persisted at all. cachedLatest is the last known tag, preserved across a
// failed refresh.
func attemptNetworkCheck(current string, attemptTime time.Time, cachePath, cachedLatest string) (Notice, bool) {
	latest, err := fetchLatest(newHTTPClient(), latestReleaseURL, githubToken())

	entry := cacheEntry{
		CheckedAt: attemptTime,
		Latest:    cachedLatest, // Preserve old Latest on failure
	}

	if err != nil {
		// Network or decode failure: update checked_at but keep old Latest
		persist(cachePath, entry)
		return Notice{}, false
	}

	// Validate fetched tag is comparable
	if _, comp := compareVersions(latest, current); !comp {
		// Fetched tag is non-comparable: don't update Latest, but update checked_at
		persist(cachePath, entry)
		return Notice{}, false
	}

	// Success: update Latest
	entry.Latest = latest
	persist(cachePath, entry)

	return noticeIfNewer(latest, current)
}

// persist writes the cache entry unless no cache path could be resolved.
func persist(cachePath string, entry cacheEntry) {
	if cachePath == "" {
		return
	}
	writeCacheSilent(cachePath, entry)
}

// writeCacheSilent writes the cache, ignoring any errors. It is a package var
// (like now, resolveStateHome and newHTTPClient) so tests can observe whether a
// write was attempted at all, and with which path.
var writeCacheSilent = func(path string, entry cacheEntry) {
	_ = writeCache(path, entry)
}
