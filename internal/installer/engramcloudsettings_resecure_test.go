package installer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
)

// TestConfigureEngramCloudSessionSync_ResecuresWhenPermissionsDrifted pins the repair for a real
// production defect observed on a live machine: click is NOT settings.json's only writer. Claude
// Code rewrites that same file whenever the developer changes one of ITS settings (/model,
// /effort, ...), and its writer does not preserve the owner-only protection click applies — so the
// file holding the consented ENGRAM_CLOUD_TOKEN silently becomes broadly readable.
//
// Before this repair, that state was permanent: the content stays canonical, so
// ConfigureEngramCloudSessionSync's idempotency short-circuit kept returning early without ever
// rewriting (and therefore without ever re-applying security), and `click doctor` is read-only by
// design so it can only report the problem, never fix it. The developer had no way back.
//
// This test seeds byte-identical canonical content through a PLAIN write (no security applied,
// exactly like a foreign writer would leave it) and asserts the next configure run rewrites it —
// observed through the injectable security factory, which only runs on an actual write.
func TestConfigureEngramCloudSessionSync_ResecuresWhenPermissionsDrifted(t *testing.T) {
	t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())

	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	cfg := Config{ClaudeHome: dir}
	m := &manifest.Manifest{EngramCloud: manifest.EngramCloud{
		Server:  "https://engram.example.com",
		Project: "team-hive",
	}}
	token := "consented-token-value"
	mode := CloudTokenPersistencePersist

	// First run writes the canonical document through the secured writer.
	if err := ConfigureEngramCloudSessionSync(cfg, m, mode, token); err != nil {
		t.Fatalf("first ConfigureEngramCloudSessionSync() error = %v", err)
	}
	canonical, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile() after first run error = %v", err)
	}

	// Simulate the foreign writer (Claude Code): identical bytes, but written plainly so the
	// owner-only protection is gone. 0o644 makes OwnerOnly false on POSIX; on Windows a plainly
	// created file inherits its parent's DACL instead of carrying a protected one, which is
	// likewise not owner-only.
	if err := os.Remove(settingsPath); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if err := os.WriteFile(settingsPath, canonical, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if ownerOnly, err := OwnerOnly(settingsPath); err == nil && ownerOnly {
		t.Skip("this platform reports the plainly-written file as owner-only; drift cannot be simulated here")
	}

	// The security factory only runs on a real write, so it is the observable proof of a rewrite.
	secured := 0
	restore := SetSettingsSecurityFactoryForTests(func(path string) error {
		secured++
		return Apply(path)
	})
	defer restore()

	if err := ConfigureEngramCloudSessionSync(cfg, m, mode, token); err != nil {
		t.Fatalf("second ConfigureEngramCloudSessionSync() error = %v", err)
	}

	if secured == 0 {
		t.Fatal("ConfigureEngramCloudSessionSync() short-circuited on canonical content and never re-applied owner-only security; the token file stays broadly readable forever")
	}

	after, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile() after repair error = %v", err)
	}
	if string(after) != string(canonical) {
		t.Fatalf("re-securing rewrite changed the document content;\nwant:\n%s\ngot:\n%s", canonical, after)
	}
	if ownerOnly, err := OwnerOnly(settingsPath); err != nil || !ownerOnly {
		t.Fatalf("OwnerOnly() = %v, err = %v after repair; want true", ownerOnly, err)
	}
}
