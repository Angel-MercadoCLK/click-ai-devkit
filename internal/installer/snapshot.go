package installer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// snapshotManifestName is the fixed filename SnapshotRun writes inside BackupDir()/latest/,
// recording what each entry's original path was, where its copy landed, and whether the source
// existed at snapshot time.
const snapshotManifestName = "manifest.json"

// DriftPolicy specifies how drift (hand-edits) is detected and handled for a snapshotted file.
type DriftPolicy string

const (
	// DriftPolicyWholeFileVeto refuses rollback if the entire file has been edited since snapshot.
	DriftPolicyWholeFileVeto DriftPolicy = "whole-file-veto"
	// DriftPolicyManagedContentVeto refuses rollback if only the managed content block has been edited.
	DriftPolicyManagedContentVeto DriftPolicy = "managed-content-veto"
	// DriftPolicyNonVeto allows rollback regardless of edits (e.g., for config files we don't manage).
	DriftPolicyNonVeto DriftPolicy = "non-veto"
)

// SnapshotDecl is a typed snapshot declaration that pairs a file path with its drift policy.
// Both Path and Policy are required — bare string literals in Snapshot: fields no longer compile.
type SnapshotDecl struct {
	Path   string
	Policy DriftPolicy
}

// snapshot creates a SnapshotDecl with the given path and policy.
// This is the only valid way to construct a SnapshotDecl, ensuring every snapshot has an explicit policy.
func snapshot(path string, policy DriftPolicy) SnapshotDecl {
	return SnapshotDecl{Path: path, Policy: policy}
}

// manifestEntry is one snapshotted file's record inside manifest.json. When Existed is false, the
// source file did not exist at snapshot time (spec Decision 1's "no-prior-state" case): BackupFile
// is then deliberately left empty — there is nothing to copy — and this explicit, structured
// Existed=false marker (never an empty/missing file, never an error) IS the no-prior-state marker
// the spec requires.
//
// The three optional fields (ExpectedPostRunHash, DriftPolicy, ExpectedPostRunManagedHash) are
// for A-record-2 and later: they record post-run hashes and drift policies for drift detection
// and rollback decisions. They round-trip through JSON and default to safe values when absent.
type manifestEntry struct {
	OriginalPath               string      `json:"originalPath"`
	BackupFile                 string      `json:"backupFile"`
	Existed                    bool        `json:"existed"`
	ExpectedPostRunHash        string      `json:"expectedPostRunHash,omitempty"`
	DriftPolicy                DriftPolicy `json:"driftPolicy,omitempty"`
	ExpectedPostRunManagedHash string      `json:"expectedPostRunManagedHash,omitempty"`
}

// runManifest is manifest.json's on-disk shape: one entry per file SnapshotRun/RestoreRun manage.
type runManifest struct {
	Entries []manifestEntry `json:"entries"`
}

// snapshotSource pairs a Config-resolved original path with the fixed filename its copy uses
// inside backups/latest/, plus the drift policy to apply.
type snapshotSource struct {
	originalPath string
	backupFile   string
	policy       DriftPolicy
}

// snapshotSources returns the per-target set of files a run-start snapshot covers: CLAUDE.md and
// settings.json ALWAYS (design's Data Flow — the two root-level files `click install`/`click
// update` write to, ahead of any external `claude` subprocess invocation), PLUS OpenClaw's
// AGENTS.md/SOUL.md/openclaw.json AND the click-memory-guard plugin's files (hooks.js, plugin.json —
// PR-C task 3.9's "add file(s) to PR-B's per-target snapshot list") AND the click-owned OpenClaw
// skill manifests (clickhola, clickdev, and the portable click-sdd phase skills) when
// cfg.OpenClawHome is populated (openclaw-target-support spec's install-snapshot-preview capability
// — generalizing this from a fixed 2-file list to a per-target list, so install-reliability-
// foundation's backup/preview/rollback protection extends to every OpenClaw file). Order is fixed
// so manifest.json's entry order is deterministic across runs.
//
// The plugin's own file list (openClawPluginRelPaths, openclawplugin.go) and the skill's own file
// list (openClawSkillRelPaths, openclawskills.go) are the single sources of truth iterated here —
// SyncOpenClawPlugin/SyncOpenClawSkills and snapshotSources can never drift out of sync about which
// files exist.
//
// ZERO behavior change for a Claude-only host: when cfg.OpenClawHome == "" (the zero value, exactly
// what every pre-existing caller that never sets it produces), this returns the identical 2-entry
// slice it always did.
//
// Drift policies are assigned per-file based on the classification table from PR A-record-1:
// - managed-content-veto: Claude CLAUDE.md, Claude settings.json, Codex AGENTS.md, OpenClaw AGENTS.md
// - non-veto: .claude.json (Context7ConfigPath), known_marketplaces.json, installed_plugins.json, Codex config.toml
// - whole-file-veto: everything else (safe default)
func snapshotSources(cfg Config) []snapshotSource {
	sources := []snapshotSource{}
	if cfg.ClaudeHome != "" {
		sources = append(sources,
			snapshotSource{originalPath: cfg.ClaudeMDPath(), backupFile: "CLAUDE.md", policy: DriftPolicyManagedContentVeto},
			snapshotSource{originalPath: cfg.SettingsPath(), backupFile: "settings.json", policy: DriftPolicyManagedContentVeto},
		)
	}
	if _, err := os.Stat(cfg.TargetSelectionPath()); err == nil {
		sources = append(sources,
			snapshotSource{originalPath: cfg.TargetSelectionPath(), backupFile: "targets.json", policy: DriftPolicyWholeFileVeto},
		)
	}
	if cfg.CodexHome != "" {
		sources = append(sources,
			snapshotSource{originalPath: cfg.CodexAgentsMDPath(), backupFile: "codex-AGENTS.md", policy: DriftPolicyManagedContentVeto},
			snapshotSource{originalPath: cfg.CodexModelProfilePath(), backupFile: "codex-model-profile.json", policy: DriftPolicyWholeFileVeto},
		)
		// Codex config.toml is non-veto (user-owned)
		if cfg.CodexConfigPath() != "" {
			sources = append(sources,
				snapshotSource{originalPath: cfg.CodexConfigPath(), backupFile: "config.toml", policy: DriftPolicyNonVeto},
			)
		}
	}
	if cfg.OpenClawHome == "" {
		return sources
	}
	sources = append(sources,
		snapshotSource{originalPath: cfg.OpenClawAgentsMDPath(), backupFile: "AGENTS.md", policy: DriftPolicyManagedContentVeto},
		snapshotSource{originalPath: cfg.OpenClawSoulMDPath(), backupFile: "SOUL.md", policy: DriftPolicyWholeFileVeto},
		snapshotSource{originalPath: cfg.OpenClawMCPConfigPath(), backupFile: "openclaw.json", policy: DriftPolicyWholeFileVeto},
		snapshotSource{originalPath: cfg.OpenClawModelProfilePath(), backupFile: "openclaw-model-profile.json", policy: DriftPolicyWholeFileVeto},
	)
	for _, rel := range openClawPluginRelPaths {
		sources = append(sources, snapshotSource{
			originalPath: filepath.Join(cfg.OpenClawPluginDir(), filepath.FromSlash(rel)),
			backupFile:   openClawPluginBackupFileName(rel),
			policy:       DriftPolicyWholeFileVeto,
		})
	}
	for _, rel := range openClawSkillRelPaths {
		sources = append(sources, snapshotSource{
			originalPath: filepath.Join(cfg.OpenClawSkillsDir(), filepath.FromSlash(rel)),
			backupFile:   openClawSkillBackupFileName(rel),
			policy:       DriftPolicyWholeFileVeto,
		})
	}
	return sources
}

// snapshotLatestDir is the single-latest-retention snapshot directory (design's "Retention"
// decision: fixed backups/latest/, overwritten each run — no ring, no history).
func snapshotLatestDir(cfg Config) string {
	backupDir := cfg.BackupDir()
	if backupDir == "" {
		return ""
	}
	return filepath.Join(backupDir, "latest")
}

// snapshotReadDir resolves which snapshot directory to READ from. It is the current location
// whenever that holds a completed snapshot, and otherwise the legacy ClaudeHome-rooted location
// from before BackupDir() moved to ClickStateHome — so an install upgraded mid-cycle can still roll
// back the run it took before upgrading. When neither holds a snapshot it returns the current
// location, so "nothing to restore" is reported against the place snapshots are actually written.
//
// Writes never go through here: snapshotLatestDir is always the current location, which is what
// makes the legacy fallback read-only and lets the migration actually complete on the next run.
func snapshotReadDir(cfg Config) string {
	current := snapshotLatestDir(cfg)
	if current != "" {
		if _, err := os.Stat(filepath.Join(current, snapshotManifestName)); err == nil {
			return current
		}
	}
	legacy := cfg.LegacyBackupDir()
	if legacy == "" {
		return current
	}
	legacyLatest := filepath.Join(legacy, "latest")
	if _, err := os.Stat(filepath.Join(legacyLatest, snapshotManifestName)); err == nil {
		return legacyLatest
	}
	return current
}

// snapshotManifestPath is where manifest.json lives inside the snapshot directory being read.
// Empty when no snapshot location resolves at all, which os.Stat/os.ReadFile then report as
// "does not exist" — the correct answer for an installer with no state home configured.
func snapshotManifestPath(cfg Config) string {
	readDir := snapshotReadDir(cfg)
	if readDir == "" {
		return ""
	}
	return filepath.Join(readDir, snapshotManifestName)
}

// SnapshotRun takes exactly one run-start snapshot of CLAUDE.md and settings.json, writing it to
// BackupDir()/latest/ plus a manifest.json describing each entry. It MUST be called before step 1
// of install/update and before any external `claude` CLI subprocess invocation (spec Requirement:
// Single Run-Start Snapshot Before Any Write) — that ordering is enforced by callers (PR2), not by
// this function itself.
//
// A missing source file is NOT an error: SnapshotRun records an explicit no-prior-state marker for
// it (Existed=false, no backup file) and continues (spec Decision 1).
//
// Last-known-good safety (spec Decision 2 / design's "Retention" decision): the new snapshot is
// built ENTIRELY in a temporary sibling directory under BackupDir() first. Only after every file
// copy and the manifest itself have been written successfully does SnapshotRun remove the previous
// backups/latest/ and rename the temporary directory into its place. Any failure before that final
// swap (e.g. a disk/write error injected via the createTempFile seam) leaves the prior completed
// snapshot in backups/latest/ completely untouched and unambiguously last-known-good — and never
// touches the original source files, which SnapshotRun only ever reads.
func SnapshotRun(cfg Config) error {
	return snapshotRunWithSources(cfg, snapshotSources(cfg))
}

func SnapshotTargetPlan(cfg Config, plan TargetPlan) error {
	_, err := SnapshotTargetPlanWithWarnings(cfg, plan)
	return err
}

// SnapshotTargetPlanWithWarnings snapshots a target plan and reports entries skipped because their
// malformed settings document cannot be safely redacted. The rest of the plan remains protected.
func SnapshotTargetPlanWithWarnings(cfg Config, plan TargetPlan) ([]string, error) {
	sources := make([]snapshotSource, 0, len(plan.SnapshotPaths()))
	for i, decl := range plan.SnapshotSpecs() {
		sources = append(sources, snapshotSource{originalPath: decl.Path, backupFile: fmt.Sprintf("plan-%03d%s", i+1, filepath.Ext(decl.Path)), policy: decl.Policy})
	}
	return snapshotRunWithSourcesWithWarnings(cfg, sources)
}

// redactEngramCloudToken removes the value of env.ENGRAM_CLOUD_TOKEN from settings.json
// backups, replacing it with a redacted placeholder.
//
// IMPORTANT: Rollback cannot restore a prior token because this redaction permanently
// removes the token value from the backup. The next install/update must re-consent
// or receive the token externally (NFR-6, DD-7 consequence acknowledgement).
//
// This ensures that if a developer syncs their ~/.claude directory into a dotfiles repo
// or backup, they won't leak the token.
//
// Byte-fidelity contract: when ENGRAM_CLOUD_TOKEN IS present, this function performs
// byte-level redaction of only its value, preserving every other byte exactly as-is (no
// reformatting, no reordering, no added or removed whitespace) — this preserves the snapshot
// subsystem's byte-for-byte backup fidelity contract. When no token is present, the input is
// returned unchanged. Returns an error when the token is present but its extent cannot be
// determined with confidence (genuinely malformed input), allowing the caller to fail closed.
func redactEngramCloudToken(data []byte) ([]byte, error) {
	if !json.Valid(data) {
		return nil, fmt.Errorf("malformed JSON settings document")
	}
	ranges, err := findEngramCloudTokenValueRanges(data)
	if err != nil || len(ranges) == 0 {
		return data, err
	}
	result := make([]byte, 0, len(data))
	last := 0
	for _, r := range ranges {
		result = append(result, data[last:r.start]...)
		result = append(result, `"[REDACTED]"`...)
		last = r.end
	}
	return append(result, data[last:]...), nil
}

type jsonByteRange struct{ start, end int }

func findEngramCloudTokenValueRanges(data []byte) ([]jsonByteRange, error) {
	p := jsonTokenRangeParser{data: data}
	if _, err := p.value(); err != nil {
		return nil, err
	}
	return p.ranges, nil
}

type jsonTokenRangeParser struct {
	data   []byte
	pos    int
	ranges []jsonByteRange
	// path tracks the exact JSON keys at each level (its length IS the depth — there is no
	// separate depth counter). For example:
	// {"env":{"ENGRAM_CLOUD_TOKEN":"x"}} would have path=["env","ENGRAM_CLOUD_TOKEN"] (len 2)
	// {"plugin":{"env":{"ENGRAM_CLOUD_TOKEN":"x"}}} would have path=["plugin","env","ENGRAM_CLOUD_TOKEN"] (len 3)
	path []string
}

func (p *jsonTokenRangeParser) value() (jsonByteRange, error) {
	p.ws()
	start := p.pos
	if start >= len(p.data) {
		return jsonByteRange{}, fmt.Errorf("missing JSON value")
	}
	switch p.data[p.pos] {
	case '{':
		return p.object()
	case '[':
		return p.array()
	case '"':
		return p.string()
	default:
		for p.pos < len(p.data) && !bytes.ContainsRune([]byte(" \t\r\n,]}"), rune(p.data[p.pos])) {
			p.pos++
		}
		if p.pos == start {
			return jsonByteRange{}, fmt.Errorf("invalid JSON value")
		}
		return jsonByteRange{start, p.pos}, nil
	}
}

func (p *jsonTokenRangeParser) object() (jsonByteRange, error) {
	start := p.pos
	p.pos++
	p.ws()
	if p.pos < len(p.data) && p.data[p.pos] == '}' {
		p.pos++
		return jsonByteRange{start, p.pos}, nil
	}
	for {
		p.ws()
		kr, err := p.string()
		if err != nil {
			return jsonByteRange{}, err
		}
		var key string
		if err := json.Unmarshal(p.data[kr.start:kr.end], &key); err != nil {
			return jsonByteRange{}, err
		}

		// Push this key onto the path stack
		p.path = append(p.path, key)

		p.ws()
		if p.pos >= len(p.data) || p.data[p.pos] != ':' {
			return jsonByteRange{}, fmt.Errorf("missing colon after object key")
		}
		p.pos++
		vr, err := p.value()
		if err != nil {
			return jsonByteRange{}, err
		}

		// REDACTION: Only redact when the exact path is ["env", "ENGRAM_CLOUD_TOKEN"]
		// This matches the DD-7 design specification: redact ONLY the top-level env.ENGRAM_CLOUD_TOKEN
		// path, not any nested occurrence of ENGRAM_CLOUD_TOKEN inside foreign objects.
		//
		// The path length check ensures we're at depth 2 (top level is depth 1, env is depth 2)
		// Path[0] == "env" ensures we're inside the top-level env object
		// Path[1] == "ENGRAM_CLOUD_TOKEN" ensures we're processing the exact key to redact
		if len(p.path) == 2 && p.path[0] == "env" && p.path[1] == "ENGRAM_CLOUD_TOKEN" {
			if p.data[vr.start] != '"' {
				return jsonByteRange{}, fmt.Errorf("ENGRAM_CLOUD_TOKEN value is not a string")
			}
			p.ranges = append(p.ranges, vr)
		}

		// Pop the key from the path after processing this key-value pair
		if len(p.path) > 0 {
			p.path = p.path[:len(p.path)-1]
		}

		p.ws()
		if p.pos >= len(p.data) {
			return jsonByteRange{}, fmt.Errorf("unterminated object")
		}
		if p.data[p.pos] == '}' {
			p.pos++
			return jsonByteRange{start, p.pos}, nil
		}
		if p.data[p.pos] != ',' {
			return jsonByteRange{}, fmt.Errorf("missing object delimiter")
		}
		p.pos++
	}
}

func (p *jsonTokenRangeParser) array() (jsonByteRange, error) {
	start := p.pos
	p.pos++
	p.ws()
	if p.pos < len(p.data) && p.data[p.pos] == ']' {
		p.pos++
		return jsonByteRange{start, p.pos}, nil
	}
	for {
		if _, err := p.value(); err != nil {
			return jsonByteRange{}, err
		}
		p.ws()
		if p.pos >= len(p.data) {
			return jsonByteRange{}, fmt.Errorf("unterminated array")
		}
		if p.data[p.pos] == ']' {
			p.pos++
			return jsonByteRange{start, p.pos}, nil
		}
		if p.data[p.pos] != ',' {
			return jsonByteRange{}, fmt.Errorf("missing array delimiter")
		}
		p.pos++
	}
}

func (p *jsonTokenRangeParser) string() (jsonByteRange, error) {
	start := p.pos
	if p.pos >= len(p.data) || p.data[p.pos] != '"' {
		return jsonByteRange{}, fmt.Errorf("expected JSON string")
	}
	p.pos++
	escaped := false
	for p.pos < len(p.data) {
		c := p.data[p.pos]
		p.pos++
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == '"' {
			return jsonByteRange{start, p.pos}, nil
		}
	}
	return jsonByteRange{}, fmt.Errorf("unterminated JSON string")
}

func (p *jsonTokenRangeParser) ws() {
	for p.pos < len(p.data) && bytes.ContainsRune([]byte(" \t\r\n"), rune(p.data[p.pos])) {
		p.pos++
	}
}

func snapshotRunWithSources(cfg Config, sources []snapshotSource) error {
	_, err := snapshotRunWithSourcesWithWarnings(cfg, sources)
	return err
}

func snapshotRunWithSourcesWithWarnings(cfg Config, sources []snapshotSource) ([]string, error) {
	backupDir := cfg.BackupDir()
	if backupDir == "" {
		return nil, fmt.Errorf("installer: cannot snapshot: no click state home configured")
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return nil, fmt.Errorf("installer: create backup dir %s: %w", backupDir, err)
	}

	tmpDir, err := os.MkdirTemp(backupDir, ".latest-tmp-*")
	if err != nil {
		return nil, fmt.Errorf("installer: create temporary snapshot dir: %w", err)
	}
	swapped := false
	defer func() {
		// Best-effort cleanup: once the rename below succeeds, tmpDir no longer exists under this
		// name, so this is a harmless no-op. On any earlier failure it removes the partially built
		// temp snapshot so it never accumulates and never gets mistaken for a real snapshot.
		if !swapped {
			_ = os.RemoveAll(tmpDir)
		}
	}()

	manifest := runManifest{}
	warnings := []string{}
	for _, src := range sources {
		data, readErr := os.ReadFile(src.originalPath)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				manifest.Entries = append(manifest.Entries, manifestEntry{
					OriginalPath: src.originalPath,
					BackupFile:   "",
					Existed:      false,
					DriftPolicy:  src.policy,
				})
				continue
			}
			return nil, fmt.Errorf("installer: read %s for snapshot: %w", src.originalPath, readErr)
		}

		// Redact env.ENGRAM_CLOUD_TOKEN from settings.json backups (NFR-6)
		if filepath.Base(src.originalPath) == "settings.json" {
			redacted, err := redactEngramCloudToken(data)
			if err != nil {
				// Fail closed: if we can't redact safely, don't write the backup
				// A missing backup is recoverable; a leaked credential is not
				if !json.Valid(data) {
					warnings = append(warnings, src.originalPath)
					continue
				}
				return nil, fmt.Errorf("installer: redact token from %s: %w", src.originalPath, err)
			}
			data = redacted
		}

		backupPath := filepath.Join(tmpDir, src.backupFile)
		if writeErr := atomicWriteFile(backupPath, data, 0o600); writeErr != nil {
			return nil, fmt.Errorf("installer: write snapshot backup for %s: %w", src.originalPath, writeErr)
		}
		manifest.Entries = append(manifest.Entries, manifestEntry{
			OriginalPath: src.originalPath,
			BackupFile:   src.backupFile,
			Existed:      true,
			DriftPolicy:  src.policy,
		})
	}

	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("installer: marshal snapshot manifest: %w", err)
	}
	manifestData = append(manifestData, '\n')
	if err := atomicWriteFile(filepath.Join(tmpDir, snapshotManifestName), manifestData, 0o600); err != nil {
		return nil, fmt.Errorf("installer: write snapshot manifest: %w", err)
	}

	// Every file copy and the manifest itself are now safely on disk under tmpDir. Only now do we
	// touch backups/latest/ at all — this is the sole point where the prior snapshot could be
	// affected, and it only runs after full success above.
	latestDir := snapshotLatestDir(cfg)
	if err := os.RemoveAll(latestDir); err != nil {
		return nil, fmt.Errorf("installer: remove previous snapshot %s: %w", latestDir, err)
	}
	if err := os.Rename(tmpDir, latestDir); err != nil {
		return nil, fmt.Errorf("installer: activate new snapshot at %s: %w", latestDir, err)
	}
	swapped = true
	return warnings, nil
}

// RestoreRun restores every snapshotted file to its last run-start snapshot (spec Requirement:
// Restore Last Run Snapshot). It is a thin composition of PrepareRestore + ApplyPreparedRestore
// with NO prompting and NO warning output of its own: it is used by install/update's automatic
// failure-recovery paths, which must never prompt a user mid-command. The ownership-scoped
// drift/consent policy lives in the rollback command, not here.
func RestoreRun(cfg Config) error {
	prepared, err := PrepareRestore(cfg)
	if err != nil {
		return err
	}
	return ApplyPreparedRestore(prepared)
}

// HasRunSnapshot reports whether a completed run-start snapshot exists: specifically, whether
// manifest.json is present under BackupDir()/latest/. This answers "did a snapshot run ever
// complete" — NOT "is there content to restore": a manifest whose entries are ALL no-prior-state
// markers (every source was absent at snapshot time) still means a real run completed, so
// HasRunSnapshot reports true for it too. Callers that need the finer "nothing to restore"
// distinction (spec's install-rollback "No snapshot exists" scenario) must inspect each manifest
// entry's Existed flag themselves — that is the future rollback command's (PR3) concern, not this
// function's.
func HasRunSnapshot(cfg Config) (bool, error) {
	_, err := os.Stat(snapshotManifestPath(cfg))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("installer: check snapshot manifest: %w", err)
	}
	return true, nil
}

// HasRestorableSnapshot reports whether a completed run-start snapshot exists AND has at least one
// entry with real backed-up content to restore (Existed=true). It is false both when no snapshot
// has ever completed (see HasRunSnapshot) and when a snapshot completed but recorded only
// no-prior-state markers for every entry (both CLAUDE.md and settings.json were absent at snapshot
// time) — the rollback command's (PR3) "No snapshot exists" scenario (spec install-rollback).
// HasRunSnapshot's own doc comment deliberately defers this finer distinction to a future caller;
// this is that caller.
func HasRestorableSnapshot(cfg Config) (bool, error) {
	has, err := HasRunSnapshot(cfg)
	if err != nil || !has {
		return false, err
	}
	manifest, err := loadSnapshotManifest(cfg)
	if err != nil {
		return false, err
	}
	for _, entry := range manifest.Entries {
		if entry.Existed {
			return true, nil
		}
	}
	return false, nil
}

// SnapshotDrift reports the ownership-scoped drift of the snapshot's backed-up files (see
// DriftReport): which paths veto a rollback because click-owned content changed, and which
// present non-veto paths merely warrant a warning because they match neither baseline. It shares
// PrepareRestore's exact classification logic so the drift check and the restore plan can never
// diverge, and — like RestoreRun — assumes a manifest already exists (callers should check
// HasRestorableSnapshot first). PR4's future doctor drift check must reuse this same report; that
// doctor-side check itself is explicitly out of scope for this change and is NOT implemented here.
func SnapshotDrift(cfg Config) (DriftReport, error) {
	prepared, err := PrepareRestore(cfg)
	if err != nil {
		return DriftReport{}, err
	}
	return prepared.Drift, nil
}

// loadSnapshotManifest reads and parses BackupDir()/latest/manifest.json.
// It validates the manifest content: unknown policies are rejected, duplicate paths with
// conflicting policies are rejected, and absent/empty policies are normalized to
// DriftPolicyWholeFileVeto for backward compatibility with legacy v0.5.11 manifests.
func loadSnapshotManifest(cfg Config) (runManifest, error) {
	data, err := os.ReadFile(snapshotManifestPath(cfg))
	if err != nil {
		return runManifest{}, fmt.Errorf("installer: read snapshot manifest: %w", err)
	}
	var manifest runManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return runManifest{}, fmt.Errorf("installer: parse snapshot manifest: %w", err)
	}

	// Normalize and validate manifest entries.
	knownPolicies := map[DriftPolicy]bool{
		DriftPolicyWholeFileVeto:      true,
		DriftPolicyManagedContentVeto: true,
		DriftPolicyNonVeto:            true,
	}

	// Track policies for duplicate detection: same path must have same policy every time.
	pathPolicy := make(map[string]DriftPolicy)

	for i := range manifest.Entries {
		entry := &manifest.Entries[i]

		// Normalize empty/absent policy to whole-file-veto (legacy manifest compatibility).
		if entry.DriftPolicy == "" {
			entry.DriftPolicy = DriftPolicyWholeFileVeto
		}

		// Reject unknown policies.
		if !knownPolicies[entry.DriftPolicy] {
			return runManifest{}, fmt.Errorf("installer: manifest entry %s has unknown policy %q", entry.OriginalPath, entry.DriftPolicy)
		}

		// Reject duplicate paths with conflicting policies.
		if priorPolicy, exists := pathPolicy[entry.OriginalPath]; exists {
			if priorPolicy != entry.DriftPolicy {
				return runManifest{}, fmt.Errorf("installer: manifest has duplicate path %s with conflicting policies %q and %q",
					entry.OriginalPath, priorPolicy, entry.DriftPolicy)
			}
		} else {
			pathPolicy[entry.OriginalPath] = entry.DriftPolicy
		}
	}

	return manifest, nil
}

// CanonicalContentHash returns the sha256 hex digest of content after canonicalizing line endings
// to LF via crlfAwareSplitLines/joinWithLineEnding (claudemd.go) — so a CRLF-saved file and an
// LF-saved file with the same logical content always hash identically. Exported for CLI tests
// that need to verify post-run hashes in manifests. Extracted here (rather than duplicated)
// because BOTH PR3's rollback hand-edit drift check (spec install-rollback Decision 3,
// "refuse-by-default" when current content drifts from the snapshot's recorded hash) and PR4's
// doctor managed-block drift check (spec managed-block-integrity, design's "Drift hash" decision)
// need the exact same LF-canonicalization + hash algorithm, and must never be allowed to silently
// diverge from each other.
func CanonicalContentHash(content string) string {
	canonical := joinWithLineEnding(crlfAwareSplitLines(content), "\n")
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}
