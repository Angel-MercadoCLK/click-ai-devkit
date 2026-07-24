## Exploration: Repository Production Quality

### Current State
The checked-out baseline is `v0.5.3` (`fb10640`) with a substantial uncommitted remediation attempt: 454 additions and 154 deletions in tracked files, plus eight untracked Go source/test files. The working tree passes `git diff --check`, `go test ./... -count=1`, `go test -race ./... -count=1`, `go vet ./...`, `go build ./...`, and `gofmt -l .`.

The attempted implementation materially improves the reported Windows failure. `click install` now detects and presents targets before model screens; Claude-only preflight is delayed until Claude is selected; its command runner gives Claude a repository cwd; and the doctor differentiates a stale registered cache path from an absent source asset. A real Windows smoke run in an isolated `CLICK_CLAUDE_HOME` completed marketplace add/update, all four Claude plugins, Engram, Context7, memory guard, and the selected Codex path without the prior unsafe-cwd Git error.

However, this is not releasable as-is. The displayed installation plan, actual mutation ordering, persisted target state, runtime ownership boundaries, update path, documentation, and release gates are not yet coherent.

### Affected Areas
- `internal/cli/install.go` — new target-first wizard flow; target state is persisted only when Claude is selected, while the final plan does not represent the actual write order.
- `internal/cli/installplan.go` — plan asserts all target branches precede common integrations, but execution performs Claude integrations before Codex/OpenClaw native mutations.
- `internal/cli/preview.go` — preview says native model configuration is part of the plan, but the matching mutations are not consistently represented as spinner-backed steps.
- `internal/cli/update.go`, `internal/cli/configuretargets.go` — existing update/configure-target orchestration still follows legacy persisted-selection behavior and must be reconciled with install.
- `internal/installer/plugins.go` — fixes Claude/Git cwd by selecting a repository root or initializing a stable temp repository; needs real failure/recovery validation on the exact Scoop/Git environment.
- `internal/installer/codexmodel.go` — directly rewrites `CODEX_HOME/config.toml` using line matching; it cannot distinguish root `model` from a TOML table key and has no parser/round-trip coverage.
- `internal/installer/openclawmodel.go` — invokes assumed `openclaw config set` keys/flags; the project’s own runbook explicitly says this command shape is pending confirmation against a real OpenClaw CLI.
- `internal/installer/targets.go` — target-selection state is stored under Claude home, which makes Claude-free installs operationally coupled to Claude’s config location.
- `internal/doctor/checks.go` — stale-cache wording is better, but diagnosis remains static and only treats repository source assets as authoritative when the running cwd has a `.git` directory.
- `internal/menu/menu.go` — reorders flat rows but has no visual grouping, hierarchy, or target-aware availability; the menu still exposes an OpenClaw-native action without a corresponding Codex-native action.
- `README.md`, `documentacion/codex-target.md`, `documentacion/portability-runbook.md`, `documentacion/{vision,prd,requirements}.md`, `AGENTS.md`, `CLAUDE.md` — contradict current native-model behavior, current target-first flow, or current release version.
- `.github/workflows/ci.yml`, `.github/workflows/release.yml`, `.goreleaser.yaml` — unit tests run on Windows CI, but release is built/tested only on Ubuntu and no release job validates Windows-installed binaries, Scoop upgrade, Claude marketplace refresh, or target CLIs.

### Approaches
1. **Finish the current remediation as one coherent release hardening change** — retain the target-first design, correct the execution/state/documentation contradictions, and add a release qualification matrix.
   - Pros: Preserves the validated Windows marketplace cwd fix; aligns with the requested target-first experience; least rework.
   - Cons: Cross-target state and rollback semantics need deliberate design, not incremental patches.
   - Effort: High.

2. **Revert native Codex/OpenClaw model writes and release only the marketplace/doctor/menu fixes** — keep model configuration as documented guidance until each native contract is proven.
   - Pros: Smaller immediate risk surface; respects the previously documented boundaries.
   - Cons: Does not meet the requested wizard sequence or native-model goal; creates another release soon after.
   - Effort: Medium.

### Recommendation
Use Approach 1, but gate the release on explicit target capability contracts. The installer should be a data-driven state machine with these user-visible stages: target selection; Claude per-phase configuration only when Claude is selected; Codex native configuration only after an explicit confirmation that describes the exact file/key; OpenClaw native configuration only after validating the installed CLI contract; one complete final summary; then ordered writes. The displayed plan, snapshot/rollback set, execution order, and doctor checks must derive from the same plan object.

Do not claim cross-target “common integrations” where an integration is actually Claude-specific. Make ownership explicit: Claude gets marketplace/plugins/Context7/Claude memory guard; OpenClaw gets its own MCP/skills/guard only after its executable contract is verified; Codex gets only explicitly supported native config plus managed guidance. Store selection state in Click-owned neutral state rather than under a required Claude home, or document and test that dependency.

### Risks
- **Release blocker — plan/execution mismatch:** `BuildInstallPlan` says Claude, Codex, and OpenClaw setup occur before Engram/Context7/plugins/guard/assets, but `runInstall` mutates Claude integrations first and configures Codex/OpenClaw later. A final confirmation cannot reliably describe side effects or rollback scope.
- **Release blocker — noninteractive implicit model mutation:** `--yes` chooses `gpt-5.6` for Codex and calls `ConfigureCodexModel`; this contradicts the UI/README claim that native config changes require explicit selection and can overwrite a user model without `--codex-model`.
- **High — Codex TOML safety:** `ConfigureCodexModel` uses a prefix match (`model...=`) rather than TOML syntax/section awareness. It can modify a `model` key inside a table and does not have comment/format/atomic-write/rollback tests.
- **High — OpenClaw contract remains unverified:** code and tests assert an “official” command, while `documentacion/portability-runbook.md` says the keys, subcommand, and `--strict-json` flag are pending real-CLI confirmation. No real OpenClaw execution was performed.
- **High — target persistence remains Claude-coupled:** Claude-free selection is allowed by `SaveTargetSelection`, but install persists it only under Claude home and only when Claude is selected. Subsequent `update`, `configure-targets`, snapshots, and uninstall need Claude-free lifecycle tests.
- **High — marketplace remediation has only one live profile:** the isolated Windows smoke run proves the current installed Claude/Git combination succeeds, but no test exercises Scoop’s Git shim/PATH ordering, a non-repository launch directory, a failed `git init`, cache corruption, or recovery/cleanup of the stable temporary repository.
- **Medium — doctor diagnosis is not an end-to-end repair proof:** it reads installed registries and files, does not verify a fresh Claude session’s nested-agent propagation, and source-package attribution depends on a Git checkout cwd. The stale `click-apply` report needs a real stale-cache → `click update` → restarted-Claude recovery test.
- **Medium — menu IA is still flat:** row reordering is not an information architecture. It lacks grouped headings, target availability/status, and consistent configure actions for all selected runtimes.
- **High — documentation drift:** README says Claude is always primary, Codex never changes `config.toml`, and the latest Scoop release is v0.4.7; Codex/OpenClaw/PRD/vision docs retain the old boundaries. No documentation files are in the current remediation diff.
- **High — release qualification is incomplete:** CI builds/tests on Windows, but release executes GoReleaser only on Ubuntu and runs no packaged Windows binary smoke test. The manifest comment still says there is no marketplace manifest despite the repository shipping one. Release metadata tests compare local manifest and Scoop metadata but not tag/version, release assets, or update behavior.
- **Medium — test design gaps:** Bubble Tea component tests cover isolated transitions, but `runInstall`/the target selector have no full workflow tests with injected selectors. External command tests use fakes; no skippable Windows integration suite exists for Claude/Codex/OpenClaw/Scoop. Current full tests passing therefore prove regression coverage, not the user’s real environment matrix.

### Ready for Proposal
Yes — propose a single production-hardening change with release blockers first: unify wizard plan and side-effect order; establish explicit native configuration/rollback contracts; reconcile target persistence and update/uninstall; add Windows real-environment qualification; then update documentation and release automation. Do not create a release from the present working tree.
