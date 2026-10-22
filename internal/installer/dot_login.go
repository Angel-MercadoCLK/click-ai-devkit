package installer

import (
	"errors"
	"path/filepath"
	"strings"
)

// DotLoginStore is the minimal surface needed to remove Click Dot's start-at-login entry. The Windows
// implementation edits HKCU\Software\Microsoft\Windows\CurrentVersion\Run (and the matching
// Explorer\StartupApproved\Run value); tests use an in-memory fake.
//
// Electron 44's app.setLoginItemSettings writes the Run value under
// settings.name || GetAppUserModelID(), and the default AppUserModelID is
// "electron.app.<exe ProductName>" unless the app sets one explicitly (shell/browser/browser_win.cc and
// shell/common/application_info_win.cc, v44.9.3). Dot sets neither, and the ProductName depends on
// the packaging pipeline's resource editing, so the value name is not stable enough to hard-code.
// Instead the entry is identified by what it launches: only values whose command starts Dot's own
// installed executable are removed.
type DotLoginStore interface {
	// Entries lists every value in the Run key as name -> command line.
	Entries() (map[string]string, error)
	// Delete removes one Run value, and the StartupApproved value with the same name.
	Delete(name string) error
}

func removeDotLoginItems(store DotLoginStore, exe string) error {
	entries, err := store.Entries()
	if err != nil {
		return err
	}
	var errs []error
	for name, command := range entries {
		if dotCommandLaunches(command, exe) {
			if err := store.Delete(name); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// dotCommandLaunches reports whether a Run command line starts exe (quoted or bare, any case).
func dotCommandLaunches(command, exe string) bool {
	command = strings.TrimSpace(command)
	target := normalizeDotPath(exe)
	if strings.HasPrefix(command, `"`) {
		end := strings.Index(command[1:], `"`)
		if end < 0 {
			return false
		}
		return normalizeDotPath(command[1:1+end]) == target
	}
	lower := strings.ToLower(command)
	if !strings.HasPrefix(lower, target) {
		return false
	}
	rest := lower[len(target):]
	return rest == "" || rest[0] == ' '
}

func normalizeDotPath(path string) string {
	return strings.ToLower(strings.ReplaceAll(filepath.Clean(strings.ReplaceAll(path, `\`, "/")), "/", `\`))
}
