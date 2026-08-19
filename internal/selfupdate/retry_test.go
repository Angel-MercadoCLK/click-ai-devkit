package selfupdate

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestRetryFileOperation_SevenAttemptsExactBackoff(t *testing.T) {
	original := fileRetrySleep
	var waits []time.Duration
	fileRetrySleep = func(d time.Duration) { waits = append(waits, d) }
	t.Cleanup(func() { fileRetrySleep = original })

	attempts := 0
	err := retryFileOperation(func() error { attempts++; return os.ErrPermission })
	if !errors.Is(err, os.ErrPermission) || attempts != 7 {
		t.Fatalf("err=%v attempts=%d, want permission error and 7 attempts", err, attempts)
	}
	want := []time.Duration{50, 100, 200, 400, 800, 1600}
	for i, ms := range want {
		if waits[i] != ms*time.Millisecond {
			t.Fatalf("wait %d = %v, want %v", i, waits[i], ms*time.Millisecond)
		}
	}
}

func TestRetryFileOperation_PermanentErrorShortCircuits(t *testing.T) {
	attempts := 0
	err := retryFileOperation(func() error { attempts++; return errors.New("permanent") })
	if err == nil || attempts != 1 {
		t.Fatalf("err=%v attempts=%d", err, attempts)
	}
}

func TestIsTransientFileError_Classification(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{os.ErrPermission, true},
		{&os.PathError{Op: "rename", Path: "x", Err: os.ErrPermission}, true},
		{os.ErrNotExist, false},
		{errors.New("permanent"), false},
	}
	for _, tt := range tests {
		if got := isTransientFileError(tt.err); got != tt.want {
			t.Errorf("%v: got %t want %t", tt.err, got, tt.want)
		}
	}
}
