package selfupdate

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireTargetLock_SerializesOnOnePath(t *testing.T) {
	target := filepath.Join(t.TempDir(), "click")
	first, err := acquireTargetLock(target)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	result := make(chan error, 1)
	go func() {
		second, err := acquireTargetLock(target)
		if second != nil {
			_ = second.Close()
		}
		result <- err
	}()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("second lock acquired while first held")
		}
	case <-time.After(time.Second):
		t.Fatal("lock acquisition was unbounded")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := acquireTargetLock(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target + ".lock"); err != nil {
		t.Fatalf("lock path removed: %v", err)
	}
}
