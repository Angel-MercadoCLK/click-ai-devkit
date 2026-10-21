//go:build windows

package installer

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const (
	dotRunKeyPath             = `Software\Microsoft\Windows\CurrentVersion\Run`
	dotStartupApprovedKeyPath = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`

	detachedProcess       = 0x00000008
	createNewProcessGroup = 0x00000200
)

// startDotDetached launches Dot so that it outlives `click` and never shares its console.
func startDotDetached(exe string) error {
	cmd := exec.Command(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedProcess | createNewProcessGroup}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

type registryDotLoginStore struct{}

func newDotLoginStore() DotLoginStore { return registryDotLoginStore{} }

func (registryDotLoginStore) Entries() (map[string]string, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, dotRunKeyPath, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("abrir HKCU\\%s: %w", dotRunKeyPath, err)
	}
	defer key.Close()
	names, err := key.ReadValueNames(-1)
	if err != nil {
		return nil, fmt.Errorf("listar HKCU\\%s: %w", dotRunKeyPath, err)
	}
	entries := make(map[string]string, len(names))
	for _, name := range names {
		value, _, err := key.GetStringValue(name)
		if err != nil {
			continue // non-string values cannot be a Dot command line
		}
		entries[name] = value
	}
	return entries, nil
}

func (registryDotLoginStore) Delete(name string) error {
	run, err := registry.OpenKey(registry.CURRENT_USER, dotRunKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("abrir HKCU\\%s: %w", dotRunKeyPath, err)
	}
	defer run.Close()
	if err := run.DeleteValue(name); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("quitar %q de HKCU\\%s: %w", name, dotRunKeyPath, err)
	}
	if approved, err := registry.OpenKey(registry.CURRENT_USER, dotStartupApprovedKeyPath, registry.SET_VALUE); err == nil {
		defer approved.Close()
		_ = approved.DeleteValue(name)
	}
	return nil
}
