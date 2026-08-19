package selfupdate

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// newDownloadClient creates an HTTP client for downloading release assets.
// It follows a strict security policy:
// - 60 second timeout per attempt
// - Allows exactly 5 redirect hops
// - Only follows HTTPS redirects
// - Only allows github.com and release-assets.githubusercontent.com hosts
// - Strips Authorization header on redirected hosts
// - Disables compression so the hash covers the downloaded archive bytes
func newDownloadClient() *http.Client {
	allowedHosts := map[string]bool{
		"github.com":                           true,
		"release-assets.githubusercontent.com": true,
	}

	return &http.Client{
		Timeout: 60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return downloadRedirectPolicy(req, via, allowedHosts)
		},
		Transport: &http.Transport{
			DisableCompression: true,
		},
	}
}

// downloadRedirectPolicy implements the redirect policy for downloads.
// Returns nil to allow the redirect, or an error to reject it.
func downloadRedirectPolicy(req *http.Request, via []*http.Request, allowedHosts map[string]bool) error {
	// Enforce maximum 5 redirects
	if len(via) >= 5 {
		return http.ErrUseLastResponse
	}

	// Only allow HTTPS
	if req.URL.Scheme != "https" {
		return http.ErrUseLastResponse
	}

	// Only allow whitelisted hosts
	if !allowedHosts[req.URL.Host] {
		return http.ErrUseLastResponse
	}

	// Strip Authorization header on redirect (don't send token to asset hosts)
	req.Header.Del("Authorization")

	return nil
}

const maxChecksumsBytes = 1 << 20 // 1 MiB

// fetchChecksums fetches the checksums.txt file from the given URL.
// Returns the file contents or an error if the status is not 2xx or if
// the file exceeds maxChecksumsBytes.
func fetchChecksums(url string) ([]byte, error) {
	client := newDownloadClient()
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch checksums: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	// Check Content-Length if available
	if resp.ContentLength > maxChecksumsBytes {
		return nil, fmt.Errorf("checksums file too large: %d bytes", resp.ContentLength)
	}

	// Read with limit to prevent unbounded reads
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxChecksumsBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read checksums: %w", err)
	}

	// Check if we hit the limit (means file was larger than allowed)
	if int64(len(data)) > maxChecksumsBytes {
		return nil, fmt.Errorf("checksums file exceeds %d byte limit", maxChecksumsBytes)
	}

	return data, nil
}

const (
	maxArchiveBytes     = 64 * 1024 * 1024 // 64 MiB
	maxDownloadAttempts = 3
)

var (
	// downloadRetrySleep is a seam for testing retry timing
	downloadRetrySleep = func(attempt int) time.Duration {
		// 250ms for first retry, 500ms for second
		if attempt == 1 {
			return 250 * time.Millisecond
		}
		return 500 * time.Millisecond
	}
	// createDownloadTemp is a seam for testing temp file creation failures
	createDownloadTemp = func(dir, pattern string) (*os.File, error) {
		return os.CreateTemp(dir, pattern)
	}
)

// downloadToSibling downloads a file from url to a temporary file in targetDir.
// It streams the download while computing SHA-256, enforces size bounds,
// and retries on retryable errors. Returns the temp file path, computed digest,
// or an error.
func downloadToSibling(url, targetDir, pattern string) (string, []byte, error) {
	var lastErr error

	for attempt := 1; attempt <= maxDownloadAttempts; attempt++ {
		if attempt > 1 {
			// Sleep before retry
			time.Sleep(downloadRetrySleep(attempt - 1))
		}

		tempFile, digest, err := downloadAttempt(url, targetDir, pattern)
		if err == nil {
			// Success
			return tempFile, digest, nil
		}

		lastErr = err

		// Don't retry on certain errors
		if !isRetryableError(err) {
			break
		}
	}

	return "", nil, lastErr
}

// downloadAttempt performs a single download attempt.
func downloadAttempt(url, targetDir, pattern string) (string, []byte, error) {
	client := newDownloadClient()
	resp, err := client.Get(url)
	if err != nil {
		// Tagged so isRetryableError can tell a transport failure apart from the deterministic
		// failures below, which must never be retried.
		return "", nil, fmt.Errorf("download request: %w: %w", errTransportFailure, err)
	}
	defer resp.Body.Close()

	// Check status code - only retry on specific statuses
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", nil, &downloadError{statusCode: resp.StatusCode, retryable: isRetryableStatus(resp.StatusCode)}
	}

	// Check Content-Length if available
	if resp.ContentLength > maxArchiveBytes {
		return "", nil, fmt.Errorf("archive too large: %d bytes exceeds %d byte limit", resp.ContentLength, maxArchiveBytes)
	}

	// Create temp file in target directory
	tempFile, err := createDownloadTemp(targetDir, pattern)
	if err != nil {
		return "", nil, fmt.Errorf("create temp file: %w", err)
	}
	tempPath := tempFile.Name()

	// Ensure temp file is cleaned up on error
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tempPath)
		}
	}()

	// Stream download to file while computing SHA-256
	hasher := sha256.New()
	multiWriter := io.MultiWriter(tempFile, hasher)

	bytesWritten, err := io.Copy(multiWriter, io.LimitReader(resp.Body, maxArchiveBytes+1))
	if err != nil {
		_ = tempFile.Close()
		return "", nil, fmt.Errorf("download write: %w", err)
	}

	// Check if we hit the size limit
	if bytesWritten > maxArchiveBytes {
		_ = tempFile.Close()
		return "", nil, fmt.Errorf("archive exceeded %d byte limit", maxArchiveBytes)
	}

	// Verify Content-Length if provided
	if resp.ContentLength > 0 && bytesWritten != resp.ContentLength {
		_ = tempFile.Close()
		return "", nil, fmt.Errorf("size mismatch: expected %d bytes, got %d", resp.ContentLength, bytesWritten)
	}

	// Sync and close the file
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return "", nil, fmt.Errorf("sync temp file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return "", nil, fmt.Errorf("close temp file: %w", err)
	}

	success = true
	digest := hasher.Sum(nil)
	return tempPath, digest, nil
}

// isRetryableStatus returns true if the HTTP status code is retryable.
func isRetryableStatus(statusCode int) bool {
	switch statusCode {
	case 408, 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}

// isRetryableError reports whether err should trigger another attempt.
//
// Two classes are retryable, per the approved bounds:
//
//   - A status-based *downloadError whose code is one of 408/429/500/502/503/504.
//   - A transport failure that occurred BEFORE any usable response — a refused or reset
//     connection, a DNS hiccup, a dial timeout. These are the transient faults retries exist for.
//
// Everything else fails immediately: a non-retryable status, a checksum mismatch, malformed
// metadata, invalid archive contents, and any disk-write error. Retrying those would only repeat a
// deterministic failure.
func isRetryableError(err error) bool {
	var de *downloadError
	if errors.As(err, &de) {
		return de.retryable
	}
	// Not a status-based failure, so the request never produced a usable response: transport-level.
	return errors.Is(err, errTransportFailure)
}

// errTransportFailure marks a failure that happened before any HTTP response was obtained.
var errTransportFailure = errors.New("transport failure")

// downloadError represents a download error with retry information.
type downloadError struct {
	statusCode int
	retryable  bool
}

func (e *downloadError) Error() string {
	return fmt.Sprintf("download failed with status %d", e.statusCode)
}
