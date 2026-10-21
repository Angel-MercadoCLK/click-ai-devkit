//go:build !windows

package installer

import "errors"

// Dot is a Windows-only application (A1): these stubs keep the package building elsewhere and are
// never reached because DotManager.Supported is false off Windows.
func startDotDetached(string) error { return errors.New("Click Dot solo se puede iniciar en Windows") }

func newDotLoginStore() DotLoginStore { return nil }
