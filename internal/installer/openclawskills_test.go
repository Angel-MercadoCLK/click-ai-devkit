package installer

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- PR3 task 3.2/3.3 RED tests: SyncOpenClawSkills and RemoveOpenClawSkills do not exist until
// openclawskills.go's GREEN change. ---

// TestSyncOpenClawSkills_EmptyHome_NoOp guards the OpenClaw absent/skip scenario: when
// cfg.OpenClawHome is empty, SyncOpenClawSkills must write nothing and return nil.
func TestSyncOpenClawSkills_EmptyHome_NoOp(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	if err := SyncOpenClawSkills(cfg); err != nil {
		t.Fatalf("SyncOpenClawSkills() error = %v, want nil no-op when OpenClawHome is empty", err)
	}
}

// TestSyncOpenClawSkills_WritesOnlyRetainedBusinessSkill verifies that only clickhola remains an
// actively synchronized Click skill; clickdev and SDD workflow skills are retired.
func TestSyncOpenClawSkills_WritesOnlyRetainedBusinessSkill(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir(), ClickStateHome: t.TempDir()}

	if err := SyncOpenClawSkills(cfg); err != nil {
		t.Fatalf("SyncOpenClawSkills() error = %v", err)
	}

	path := filepath.Join(cfg.OpenClawSkillsDir(), "clickhola", "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v, want retained skill written", path, err)
	}
	if !strings.Contains(string(data), "name: clickhola") {
		t.Errorf("%s content = %q, want clickhola frontmatter", path, data)
	}
	for _, rel := range retiredOpenClawSkillPaths() {
		if _, err := os.Lstat(filepath.Join(cfg.OpenClawSkillsDir(), filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Errorf("retired skill %s exists after sync, want it excluded from active synchronization", rel)
		}
	}
}

func TestSyncOpenClawSkills_DoesNotInstallRetiredSDDWorkflow(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir(), ClickStateHome: t.TempDir()}
	if err := SyncOpenClawSkills(cfg); err != nil {
		t.Fatalf("SyncOpenClawSkills() error = %v", err)
	}

	for _, rel := range retiredOpenClawSkillPaths() {
		if _, err := os.Lstat(filepath.Join(cfg.OpenClawSkillsDir(), filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Errorf("retired skill %s exists after sync, want no SDD workflow copied", rel)
		}
	}
}

func TestOpenClawSDDEntry_DoesNotExposeClaudeDelegation(t *testing.T) {
	data, err := openClawSkillsAssets.ReadFile(openClawSkillsAssetsRoot + "/click-sdd/SKILL.md")
	if err != nil {
		t.Fatalf("ReadFile(embedded click-sdd entry) error = %v", err)
	}
	content := strings.ToLower(string(data))
	for _, forbidden := range []string{"agent tool", "skill tool", "claude plugin", "plugin registry", "model routing"} {
		if strings.Contains(content, forbidden) {
			t.Errorf("entry skill contains Claude-specific invocation text %q", forbidden)
		}
	}
}

// TestSyncOpenClawSkills_Rerun_ByteIdenticalOutput is PR3's idempotency case: re-running with the
// same cfg must produce byte-identical output (no timestamp or nonce is templated).
func TestSyncOpenClawSkills_Rerun_ByteIdenticalOutput(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir(), ClickStateHome: t.TempDir()}

	if err := SyncOpenClawSkills(cfg); err != nil {
		t.Fatalf("SyncOpenClawSkills() 1st run error = %v", err)
	}
	first, err := os.ReadFile(filepath.Join(cfg.OpenClawSkillsDir(), "clickhola", "SKILL.md"))
	if err != nil {
		t.Fatalf("ReadFile(clickhola SKILL.md) after 1st run error = %v", err)
	}

	if err := SyncOpenClawSkills(cfg); err != nil {
		t.Fatalf("SyncOpenClawSkills() 2nd run error = %v", err)
	}
	second, err := os.ReadFile(filepath.Join(cfg.OpenClawSkillsDir(), "clickhola", "SKILL.md"))
	if err != nil {
		t.Fatalf("ReadFile(clickhola SKILL.md) after 2nd run error = %v", err)
	}

	if string(first) != string(second) {
		t.Fatalf("clickhola SKILL.md changed across re-run:\n1st=%q\n2nd=%q\nwant byte-identical", first, second)
	}
}

// TestSyncOpenClawSkills_ChangedContent_Overwrites verifies that when the on-disk SKILL.md has
// drifted from the embedded asset, SyncOpenClawSkills restores the owned bytes.
func TestSyncOpenClawSkills_ChangedContent_Overwrites(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir(), ClickStateHome: t.TempDir()}

	if err := SyncOpenClawSkills(cfg); err != nil {
		t.Fatalf("SyncOpenClawSkills() 1st run error = %v", err)
	}
	path := filepath.Join(cfg.OpenClawSkillsDir(), "clickhola", "SKILL.md")
	if err := os.WriteFile(path, []byte("tampered content"), 0o644); err != nil {
		t.Fatalf("WriteFile(tamper) error = %v", err)
	}

	if err := SyncOpenClawSkills(cfg); err != nil {
		t.Fatalf("SyncOpenClawSkills() 2nd run error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(clickdev SKILL.md) error = %v", err)
	}
	if strings.Contains(string(got), "tampered content") {
		t.Fatalf("clickhola SKILL.md still contains tampered content = %q, want restored embedded bytes", got)
	}
	if !strings.Contains(string(got), "name: clickhola") {
		t.Fatalf("clickhola SKILL.md = %q, want restored embedded bytes with frontmatter name", got)
	}
}

func TestRetireLegacyOpenClawSkills_DeletesOnlyPinnedRegularFiles(t *testing.T) {
	root := t.TempDir()
	cfg := Config{OpenClawHome: root}
	good := []byte("pinned legacy bytes\n")
	digest := sha256.Sum256(good)
	entries := []retiredOpenClawSkill{
		{rel: "clickdev/SKILL.md", sha256: hex.EncodeToString(digest[:])},
		{rel: "click-sdd/SKILL.md", sha256: hex.EncodeToString(digest[:])},
		{rel: "click-sdd-propose/SKILL.md", sha256: hex.EncodeToString(digest[:])},
		{rel: "click-sdd-archive/SKILL.md", sha256: hex.EncodeToString(digest[:])},
		{rel: "sdd-upstream/SKILL.md", sha256: hex.EncodeToString(digest[:])},
	}
	paths := map[string]string{
		"clickdev/SKILL.md":          "pinned legacy bytes\n",
		"click-sdd/SKILL.md":         "edited legacy bytes\n",
		"click-sdd-propose/SKILL.md": "pinned legacy bytes\n",
		"clickhola/SKILL.md":         "business skill\n",
		"sdd-upstream/SKILL.md":      "upstream skill\n",
		"click-sdd-extra/SKILL.md":   "sibling skill\n",
	}
	for rel, content := range paths {
		writeTestFile(t, filepath.Join(cfg.OpenClawSkillsDir(), filepath.FromSlash(rel)), content)
	}
	symlinkPath := filepath.Join(cfg.OpenClawSkillsDir(), "click-sdd-propose", "link")
	if err := os.Symlink(filepath.Join(cfg.OpenClawSkillsDir(), "sdd-upstream"), symlinkPath); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	// The retired leaf itself is a symlink, so even matching target bytes are not eligible.
	linkLeaf := filepath.Join(cfg.OpenClawSkillsDir(), "click-sdd-verify", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(linkLeaf), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(cfg.OpenClawSkillsDir(), "sdd-upstream", "SKILL.md"), linkLeaf); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(cfg.OpenClawSkillsDir(), "click-sdd-archive", "SKILL.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	entries = append(entries, retiredOpenClawSkill{rel: "click-sdd-verify/SKILL.md", sha256: hex.EncodeToString(digest[:])})

	warnings, err := retireLegacyOpenClawSkills(cfg, entries)
	if err != nil {
		t.Fatalf("retireLegacyOpenClawSkills() error = %v", err)
	}
	if len(warnings) < 3 {
		t.Fatalf("warnings = %#v, want modified, symlink, and redirected-directory warnings", warnings)
	}
	for _, rel := range []string{"clickdev/SKILL.md"} {
		if _, err := os.Lstat(filepath.Join(cfg.OpenClawSkillsDir(), filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Errorf("matching pinned file %s was not removed: %v", rel, err)
		}
	}
	for _, rel := range []string{"click-sdd/SKILL.md", "clickhola/SKILL.md", "sdd-upstream/SKILL.md", "click-sdd-extra/SKILL.md", "click-sdd-propose/link"} {
		if _, err := os.Lstat(filepath.Join(cfg.OpenClawSkillsDir(), filepath.FromSlash(rel))); err != nil {
			t.Errorf("preserved file %s missing: %v", rel, err)
		}
	}
	if err := os.RemoveAll(filepath.Join(cfg.OpenClawSkillsDir(), "click-sdd-propose")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(cfg.OpenClawSkillsDir(), "sdd-upstream"), filepath.Join(cfg.OpenClawSkillsDir(), "click-sdd-propose")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	warnings, err = retireLegacyOpenClawSkills(cfg, entries)
	if err != nil {
		t.Fatalf("retireLegacyOpenClawSkills() with redirected parent error = %v", err)
	}
	if len(warnings) < 4 {
		t.Fatalf("warnings = %#v, want redirected ancestor warning", warnings)
	}
	if got, err := os.ReadFile(filepath.Join(cfg.OpenClawSkillsDir(), "sdd-upstream", "SKILL.md")); err != nil || string(got) != "upstream skill\n" {
		t.Fatalf("upstream target = %q, %v, want unchanged", got, err)
	}
}

func TestRetiredOpenClawSkillPinsMatchShippedAssetsAndRetireAll(t *testing.T) {
	cfg := Config{OpenClawHome: t.TempDir()}
	for _, entry := range retiredOpenClawSkills {
		asset, err := openClawSkillsAssets.ReadFile(openClawSkillsAssetsRoot + "/" + entry.rel)
		if err != nil {
			t.Fatalf("read shipped retired skill %s: %v", entry.rel, err)
		}
		digest := sha256.Sum256(asset)
		if got := hex.EncodeToString(digest[:]); got != entry.sha256 {
			t.Errorf("pinned digest for %s = %s, want shipped asset digest %s", entry.rel, entry.sha256, got)
		}
		writeTestFile(t, filepath.Join(cfg.OpenClawSkillsDir(), filepath.FromSlash(entry.rel)), string(asset))
	}

	warnings, err := RetireLegacyOpenClawSkills(cfg)
	if err != nil {
		t.Fatalf("RetireLegacyOpenClawSkills() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("RetireLegacyOpenClawSkills() warnings = %#v, want none for shipped assets", warnings)
	}
	for _, entry := range retiredOpenClawSkills {
		path := filepath.Join(cfg.OpenClawSkillsDir(), filepath.FromSlash(entry.rel))
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Errorf("shipped retired skill %s remains after retirement: %v", entry.rel, err)
		}
	}
}

func TestRetireLegacyOpenClawSkills_ValidationErrorsAreSpanish(t *testing.T) {
	cfg := Config{OpenClawHome: t.TempDir()}
	tests := []struct {
		name     string
		entry    retiredOpenClawSkill
		want     string
		unwanted string
	}{
		{name: "unsafe path", entry: retiredOpenClawSkill{rel: "../outside", sha256: strings.Repeat("0", sha256.Size*2)}, want: "ruta heredada", unwanted: "invalid retired OpenClaw skill path"},
		{name: "invalid digest", entry: retiredOpenClawSkill{rel: "click-sdd/SKILL.md", sha256: "not-a-digest"}, want: "digest fijado", unwanted: "invalid pinned digest"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := retireLegacyOpenClawSkills(cfg, []retiredOpenClawSkill{tt.entry})
			if err == nil {
				t.Fatal("retireLegacyOpenClawSkills() error = nil, want validation error")
			}
			if !strings.Contains(err.Error(), tt.want) || strings.Contains(err.Error(), tt.unwanted) {
				t.Fatalf("error = %q, want Spanish context containing %q", err, tt.want)
			}
		})
	}
}

// TestSyncOpenClawSkills_InjectedWriteError_PreservesOldBytes exercises the atomic-write failure
// path: when atomicWriteFile fails, the existing target file must remain byte-for-byte intact. We
// inject the failure via the package-level createTempFile seam used by pathenv.go's own tests.
func TestSyncOpenClawSkills_InjectedWriteError_PreservesOldBytes(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir(), ClickStateHome: t.TempDir()}

	if err := SyncOpenClawSkills(cfg); err != nil {
		t.Fatalf("SyncOpenClawSkills() baseline run error = %v", err)
	}
	path := filepath.Join(cfg.OpenClawSkillsDir(), "clickhola", "SKILL.md")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(clickhola SKILL.md) error = %v", err)
	}

	injectedErr := errors.New("injected write failure")
	old := createTempFile
	createTempFile = func(dir, pattern string) (tempFileWriter, error) {
		return &fakeFailingTempFile{name: filepath.Join(dir, ".click-injected-fake"), writeErr: injectedErr}, nil
	}
	defer func() { createTempFile = old }()

	err = SyncOpenClawSkills(cfg)
	if err == nil {
		t.Fatal("SyncOpenClawSkills() error = nil, want the injected write error to propagate")
	}
	if !errors.Is(err, injectedErr) {
		t.Fatalf("SyncOpenClawSkills() error = %v, want it to wrap %v", err, injectedErr)
	}
	if !strings.Contains(err.Error(), "write openclaw skill file") {
		t.Fatalf("SyncOpenClawSkills() error = %v, want a contextual wrapped error", err)
	}

	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile(clickhola SKILL.md) after failed write error = %v", readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("clickhola SKILL.md = %q after failed write, want untouched original", got)
	}
}

// TestSyncOpenClawSkills_MkdirAllFailure_WrapsContextualError verifies that a filesystem failure
// during destination directory creation is surfaced as a contextual error from SyncOpenClawSkills.
func TestSyncOpenClawSkills_MkdirAllFailure_WrapsContextualError(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir(), ClickStateHome: t.TempDir()}

	// Make the skills directory path itself a regular file, so MkdirAll(".../skills/clickhola")
	// cannot create a directory named "skills".
	if err := os.WriteFile(cfg.OpenClawSkillsDir(), []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile(skills file) error = %v", err)
	}

	err := SyncOpenClawSkills(cfg)
	if err == nil {
		t.Fatal("SyncOpenClawSkills() error = nil, want MkdirAll failure to propagate")
	}
	if !strings.Contains(err.Error(), "create dir") {
		t.Fatalf("SyncOpenClawSkills() error = %v, want a contextual 'create dir' error", err)
	}
}

// --- PR3 task 3.3's supporting RED coverage (RemoveOpenClawSkills) ---

// TestRemoveOpenClawSkills_EmptyHome_NoOp guards the OpenClaw absent/skip scenario for removal.
func TestRemoveOpenClawSkills_EmptyHome_NoOp(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	if err := RemoveOpenClawSkills(cfg); err != nil {
		t.Fatalf("RemoveOpenClawSkills() error = %v, want nil no-op when OpenClawHome is empty", err)
	}
}

// TestRemoveOpenClawSkills_Absent_NoOp verifies that removing never-installed skills is harmless.
func TestRemoveOpenClawSkills_Absent_NoOp(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir(), ClickStateHome: t.TempDir()}
	if err := RemoveOpenClawSkills(cfg); err != nil {
		t.Fatalf("RemoveOpenClawSkills() error = %v, want nil when skills dir is absent", err)
	}
}

// TestRemoveOpenClawSkills_RemovesOnlyOwnedDirs verifies that RemoveOpenClawSkills deletes only
// the retained clickhola skill and leaves sibling directories untouched.
func TestRemoveOpenClawSkills_RemovesOnlyOwnedDirs(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir(), ClickStateHome: t.TempDir()}

	if err := SyncOpenClawSkills(cfg); err != nil {
		t.Fatalf("SyncOpenClawSkills() error = %v", err)
	}

	// Simulate a user-created sibling skill.
	sibling := filepath.Join(cfg.OpenClawSkillsDir(), "user-skill")
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatalf("MkdirAll(sibling) error = %v", err)
	}
	if err := RemoveOpenClawSkills(cfg); err != nil {
		t.Fatalf("RemoveOpenClawSkills() error = %v", err)
	}

	for _, owned := range []string{"clickhola"} {
		path := filepath.Join(cfg.OpenClawSkillsDir(), owned)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("Stat(%s) after removal error = %v, want os.IsNotExist", owned, err)
		}
	}
	if _, err := os.Stat(sibling); err != nil {
		t.Fatalf("Stat(user-skill) after removal error = %v, want sibling preserved", err)
	}
}

func TestRemoveOpenClawSkills_PreservesUserFilesInsideOwnedDirs(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir(), ClickStateHome: t.TempDir()}
	if err := SyncOpenClawSkills(cfg); err != nil {
		t.Fatalf("SyncOpenClawSkills() error = %v", err)
	}
	for _, owned := range []string{"clickhola"} {
		path := filepath.Join(cfg.OpenClawSkillsDir(), owned, "user-notes.txt")
		if err := os.WriteFile(path, []byte("keep me\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}

	if err := RemoveOpenClawSkills(cfg); err != nil {
		t.Fatalf("RemoveOpenClawSkills() error = %v", err)
	}
	for _, owned := range []string{"clickhola"} {
		dir := filepath.Join(cfg.OpenClawSkillsDir(), owned)
		if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); !os.IsNotExist(err) {
			t.Fatalf("Stat(%s) after removal error = %v, want owned file removed", dir, err)
		}
		if _, err := os.Stat(filepath.Join(dir, "user-notes.txt")); err != nil {
			t.Fatalf("user file in %s after removal error = %v, want preserved", dir, err)
		}
	}
}

// TestRemoveOpenClawSkills_Failure_WrapsContextualError verifies that an underlying RemoveAll
// failure is surfaced as a contextual error from RemoveOpenClawSkills.
func TestRemoveOpenClawSkills_Failure_WrapsContextualError(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir(), ClickStateHome: t.TempDir()}

	injectedErr := errors.New("injected remove failure")
	old := removeAll
	removeAll = func(path string) error {
		return injectedErr
	}
	defer func() { removeAll = old }()

	err := RemoveOpenClawSkills(cfg)
	if err == nil {
		t.Fatal("RemoveOpenClawSkills() error = nil, want the injected remove error to propagate")
	}
	if !errors.Is(err, injectedErr) {
		t.Fatalf("RemoveOpenClawSkills() error = %v, want it to wrap %v", err, injectedErr)
	}
	if !strings.Contains(err.Error(), "remove openclaw skill dir") {
		t.Fatalf("RemoveOpenClawSkills() error = %v, want a contextual wrapped error", err)
	}
}
