package selfupdate

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

var (
	// newHTTPClient is a seam for testing. Returns a client with 2s timeout.
	newHTTPClient = func() *http.Client {
		return &http.Client{Timeout: 2 * time.Second}
	}
	// latestReleaseURL is a seam for testing.
	latestReleaseURL = "https://api.github.com/repos/Angel-MercadoCLK/click-ai-devkit/releases/latest"
)

// githubToken returns a GitHub token for authentication, if available.
// Checks GITHUB_TOKEN first, then GH_TOKEN. Returns empty string if neither is set.
func githubToken() string {
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return token
	}
	return os.Getenv("GH_TOKEN")
}

// releaseResponse is the minimal JSON structure we care about.
type releaseResponse struct {
	TagName string `json:"tag_name"`
}

// fetchLatest fetches the latest release tag from GitHub.
// Returns the tag name (with or without 'v' prefix as provided by GitHub) or an error.
// Only 2xx status codes are accepted.
func fetchLatest(client *http.Client, endpoint, token string) (string, error) {
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	var release releaseResponse
	if err := json.Unmarshal(body, &release); err != nil {
		return "", fmt.Errorf("decode JSON: %w", err)
	}

	if release.TagName == "" {
		return "", fmt.Errorf("missing tag_name in response")
	}

	return release.TagName, nil
}
