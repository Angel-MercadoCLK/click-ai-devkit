package installer

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// testDotLimits are small enough to keep the suite fast but large enough to be stable: every
// "should fail" case fails at idle/header/total, and the steady case keeps well inside idle.
func testDotLimits() dotTransferLimits {
	return dotTransferLimits{Connect: time.Second, Header: 300 * time.Millisecond, Idle: 300 * time.Millisecond, Total: 5 * time.Second}
}

func openTestStream(t *testing.T, handler http.HandlerFunc, limits dotTransferLimits) (io.ReadCloser, error) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(func() { server.CloseClientConnections(); server.Close() })
	return openDotStream(context.Background(), newDotHTTPClient(limits), server.URL, limits)
}

func TestDotDownloadSlowButProgressingSucceeds(t *testing.T) {
	limits := testDotLimits()
	const chunks = 12
	body, err := openTestStream(t, func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < chunks; i++ {
			_, _ = w.Write([]byte("x"))
			w.(http.Flusher).Flush()
			time.Sleep(60 * time.Millisecond)
		}
	}, limits)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	start := time.Now()
	data, err := io.ReadAll(body)
	if err != nil || len(data) != chunks {
		t.Fatalf("slow but steady download must succeed: n=%d err=%v", len(data), err)
	}
	if time.Since(start) < limits.Idle {
		t.Fatalf("test must outlast the idle window to prove progress resets it (took %s)", time.Since(start))
	}
}

func TestDotDownloadStalledBodyFailsWithinIdleWindow(t *testing.T) {
	limits := testDotLimits()
	body, err := openTestStream(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("partial"))
		w.(http.Flusher).Flush()
		<-r.Context().Done() // never sends another byte
	}, limits)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	start := time.Now()
	_, err = io.ReadAll(body)
	if err == nil {
		t.Fatal("a stalled download must fail")
	}
	if elapsed := time.Since(start); elapsed > 3*limits.Idle+time.Second {
		t.Fatalf("stall detected after %s, want about %s", elapsed, limits.Idle)
	}
	if !errors.Is(err, errDotDownloadStalled) || !strings.Contains(err.Error(), "sin recibir datos") {
		t.Fatalf("err = %v, want the Spanish stall error", err)
	}
}

func TestDotDownloadSilentHeadersFailWithinHeaderTimeout(t *testing.T) {
	limits := testDotLimits()
	start := time.Now()
	body, err := openTestStream(t, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}, limits)
	if err == nil {
		body.Close()
		t.Fatal("a server that never answers must fail")
	}
	if elapsed := time.Since(start); elapsed > 3*limits.Header+time.Second {
		t.Fatalf("header wait ended after %s, want about %s", elapsed, limits.Header)
	}
}

func TestDotDownloadTotalBoundStopsEndlessTrickle(t *testing.T) {
	limits := testDotLimits()
	limits.Total = 400 * time.Millisecond
	body, err := openTestStream(t, func(w http.ResponseWriter, r *http.Request) {
		for r.Context().Err() == nil {
			_, _ = w.Write([]byte("x"))
			w.(http.Flusher).Flush()
			time.Sleep(50 * time.Millisecond)
		}
	}, limits)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	if _, err := io.ReadAll(body); err == nil {
		t.Fatal("the overall bound must end a trickle that never stalls")
	}
}

func TestDotDownloadNon200IsAnError(t *testing.T) {
	limits := testDotLimits()
	body, err := openTestStream(t, func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}, limits)
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		if body != nil {
			body.Close()
		}
		t.Fatalf("err = %v", err)
	}
}

func TestDotProductionLimitsAreIdleBasedNotATotalClientTimeout(t *testing.T) {
	if dotDefaultLimits.Idle < 30*time.Second || dotDefaultLimits.Idle > 5*time.Minute {
		t.Fatalf("idle = %s, want a window that tolerates brief stalls but not hangs", dotDefaultLimits.Idle)
	}
	if dotDefaultLimits.Total < time.Hour {
		t.Fatalf("total = %s, must stay generous for ~200 MB on slow links", dotDefaultLimits.Total)
	}
	if newDotHTTPClient(dotDefaultLimits).Timeout != 0 {
		t.Fatal("http.Client.Timeout covers the whole body and would fail slow-but-progressing downloads")
	}
}
