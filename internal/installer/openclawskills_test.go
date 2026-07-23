package installer

import (
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
	cfg := Config{ClaudeHome: t.TempDir()}
	if err := SyncOpenClawSkills(cfg); err != nil {
		t.Fatalf("SyncOpenClawSkills() error = %v, want nil no-op when OpenClawHome is empty", err)
	}
}

// TestSyncOpenClawSkills_WritesBothSkills verifies the happy path: both clickhola and clickdev
// SKILL.md files are written under cfg.OpenClawSkillsDir() with the embedded asset bytes.
func TestSyncOpenClawSkills_WritesBothSkills(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir()}

	if err := SyncOpenClawSkills(cfg); err != nil {
		t.Fatalf("SyncOpenClawSkills() error = %v", err)
	}

	for _, name := range []string{"clickhola", "clickdev"} {
		path := filepath.Join(cfg.OpenClawSkillsDir(), name, "SKILL.md")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v, want it written", path, err)
		}
		content := string(data)
		if !strings.Contains(content, "---") {
			t.Errorf("%s content = %q, want YAML frontmatter", path, content)
		}
		if !strings.Contains(content, "name: "+name) {
			t.Errorf("%s content = %q, want frontmatter name %q", path, content, name)
		}
	}
}

func TestSyncOpenClawSkills_WritesPortableSDDWorkflow(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir()}
	if err := SyncOpenClawSkills(cfg); err != nil {
		t.Fatalf("SyncOpenClawSkills() error = %v", err)
	}

	for _, rel := range []string{
		"click-sdd/SKILL.md", "click-sdd-explore/SKILL.md", "click-sdd-propose/SKILL.md",
		"click-sdd-spec/SKILL.md", "click-sdd-design/SKILL.md", "click-sdd-tasks/SKILL.md",
		"click-sdd-apply/SKILL.md", "click-sdd-verify/SKILL.md", "click-sdd-archive/SKILL.md",
		"click-sdd-onboard/SKILL.md",
	} {
		data, err := os.ReadFile(filepath.Join(cfg.OpenClawSkillsDir(), filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v, want embedded asset installed", rel, err)
		}
		if !strings.Contains(string(data), "name: ") {
			t.Errorf("%s has no skill frontmatter", rel)
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
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir()}

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
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir()}

	if err := SyncOpenClawSkills(cfg); err != nil {
		t.Fatalf("SyncOpenClawSkills() 1st run error = %v", err)
	}
	path := filepath.Join(cfg.OpenClawSkillsDir(), "clickdev", "SKILL.md")
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
		t.Fatalf("clickdev SKILL.md still contains tampered content = %q, want restored embedded bytes", got)
	}
	if !strings.Contains(string(got), "name: clickdev") {
		t.Fatalf("clickdev SKILL.md = %q, want restored embedded bytes with frontmatter name", got)
	}
}

// TestSyncOpenClawSkills_InjectedWriteError_PreservesOldBytes exercises the atomic-write failure
// path: when atomicWriteFile fails, the existing target file must remain byte-for-byte intact. We
// inject the failure via the package-level createTempFile seam used by pathenv.go's own tests.
func TestSyncOpenClawSkills_InjectedWriteError_PreservesOldBytes(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir()}

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
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir()}

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
	cfg := Config{ClaudeHome: t.TempDir()}
	if err := RemoveOpenClawSkills(cfg); err != nil {
		t.Fatalf("RemoveOpenClawSkills() error = %v, want nil no-op when OpenClawHome is empty", err)
	}
}

// TestRemoveOpenClawSkills_Absent_NoOp verifies that removing never-installed skills is harmless.
func TestRemoveOpenClawSkills_Absent_NoOp(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir()}
	if err := RemoveOpenClawSkills(cfg); err != nil {
		t.Fatalf("RemoveOpenClawSkills() error = %v, want nil when skills dir is absent", err)
	}
}

// TestRemoveOpenClawSkills_RemovesOnlyOwnedDirs verifies that RemoveOpenClawSkills deletes
// clickhola and clickdev but leaves any user-created sibling skill directories untouched.
func TestRemoveOpenClawSkills_RemovesOnlyOwnedDirs(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir()}

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

	for _, owned := range append([]string{"clickhola", "clickdev"}, portableSDDSkillDirs()...) {
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
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir()}
	if err := SyncOpenClawSkills(cfg); err != nil {
		t.Fatalf("SyncOpenClawSkills() error = %v", err)
	}
	for _, owned := range append([]string{"clickhola", "clickdev"}, portableSDDSkillDirs()...) {
		path := filepath.Join(cfg.OpenClawSkillsDir(), owned, "user-notes.txt")
		if err := os.WriteFile(path, []byte("keep me\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}

	if err := RemoveOpenClawSkills(cfg); err != nil {
		t.Fatalf("RemoveOpenClawSkills() error = %v", err)
	}
	for _, owned := range []string{"clickhola", "clickdev"} {
		dir := filepath.Join(cfg.OpenClawSkillsDir(), owned)
		if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); !os.IsNotExist(err) {
			t.Fatalf("Stat(%s) after removal error = %v, want owned file removed", dir, err)
		}
		if _, err := os.Stat(filepath.Join(dir, "user-notes.txt")); err != nil {
			t.Fatalf("user file in %s after removal error = %v, want preserved", dir, err)
		}
	}
}

func portableSDDSkillDirs() []string {
	return []string{"click-sdd", "click-sdd-explore", "click-sdd-propose", "click-sdd-spec", "click-sdd-design", "click-sdd-tasks", "click-sdd-apply", "click-sdd-verify", "click-sdd-archive", "click-sdd-onboard"}
}

// TestRemoveOpenClawSkills_Failure_WrapsContextualError verifies that an underlying RemoveAll
// failure is surfaced as a contextual error from RemoveOpenClawSkills.
func TestRemoveOpenClawSkills_Failure_WrapsContextualError(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir()}

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
