package installer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// dotTransferLimits bound a download without failing a slow link that is still making progress.
// http.Client.Timeout would cover the whole body, so a healthy 200 MB transfer on a slow connection
// would be cut off at the limit; instead each phase gets its own bound:
//   - Connect covers the TCP dial and the TLS handshake;
//   - Header covers the wait for response headers after the request is sent;
//   - Idle resets on every received chunk and ends a transfer that stops delivering bytes;
//   - Total is only a safety net so a trickle that never quite stalls cannot run forever.
type dotTransferLimits struct {
	Connect, Header, Idle, Total time.Duration
}

// dotDefaultLimits: Total allows roughly 200 MB at ~30 KB/s, far below any link worth supporting.
var dotDefaultLimits = dotTransferLimits{
	Connect: 30 * time.Second,
	Header:  60 * time.Second,
	Idle:    60 * time.Second,
	Total:   2 * time.Hour,
}

var errDotDownloadStalled = errors.New("la descarga se detuvo: sin recibir datos durante el tiempo máximo de espera")

var dotAllowedDownloadHosts = []string{"github.com", "githubusercontent.com"}

func dotHostAllowed(host string) bool {
	host = strings.ToLower(host)
	for _, allowed := range dotAllowedDownloadHosts {
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

// newDotHTTPClient builds the client for Dot downloads: no automatic decompression (the digest covers
// the exact bytes), proxy from the environment, per-phase timeouts, and a redirect chain that is
// bounded, HTTPS-only, restricted to GitHub hosts and stripped of credentials. It deliberately sets
// no Client.Timeout; see dotTransferLimits.
func newDotHTTPClient(limits dotTransferLimits) *http.Client {
	dialer := &net.Dialer{Timeout: limits.Connect}
	return &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DisableCompression:    true,
			DialContext:           dialer.DialContext,
			TLSHandshakeTimeout:   limits.Connect,
			ResponseHeaderTimeout: limits.Header,
		},
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if len(via) > dotMaxRedirects {
				return errors.New("demasiadas redirecciones")
			}
			if next.URL.Scheme != "https" || !dotHostAllowed(next.URL.Hostname()) {
				return errors.New("la redirección sale de GitHub o no usa HTTPS")
			}
			next.Header.Del("Authorization")
			return nil
		},
	}
}

// fetchDotAsset is the production download: HTTPS only, from GitHub hosts, with the transfer limits above.
func fetchDotAsset(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if req.URL.Scheme != "https" || !dotHostAllowed(req.URL.Hostname()) {
		return nil, errors.New("la descarga requiere HTTPS desde GitHub")
	}
	return openDotStream(ctx, newDotHTTPClient(dotDefaultLimits), rawURL, dotDefaultLimits)
}

// openDotStream issues the GET and returns a body that enforces the idle and total limits. Closing
// the body releases every timer and the request context.
func openDotStream(ctx context.Context, client *http.Client, rawURL string, limits dotTransferLimits) (io.ReadCloser, error) {
	ctx, cancel := context.WithTimeout(ctx, limits.Total)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	response, err := client.Do(req)
	if err != nil {
		cancel()
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		cancel()
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	body := &dotIdleBody{rc: response.Body, idle: limits.Idle, cancel: cancel}
	body.timer = time.AfterFunc(limits.Idle, func() {
		body.stalled.Store(true)
		cancel()
	})
	return body, nil
}

// dotIdleBody cancels the request when no bytes arrive for the idle window. Any chunk resets the clock.
type dotIdleBody struct {
	rc      io.ReadCloser
	idle    time.Duration
	timer   *time.Timer
	cancel  context.CancelFunc
	stalled atomic.Bool
}

func (b *dotIdleBody) Read(p []byte) (int, error) {
	n, err := b.rc.Read(p)
	if n > 0 {
		b.timer.Reset(b.idle)
	}
	if err != nil && err != io.EOF && b.stalled.Load() {
		err = errDotDownloadStalled
	}
	return n, err
}

func (b *dotIdleBody) Close() error {
	b.timer.Stop()
	b.cancel()
	return b.rc.Close()
}
