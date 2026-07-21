// Package doctor owns click's read-only environment/health checks: verifying that the installed
// plugins are actually registered in Claude Code, that the managed CLAUDE.md block exists, that
// the memory-guard hook is registered, and that Context7 is registered as a user-scope MCP server
// (tech-spec.md §2.1 "click doctor"). Checks in this package never mutate state — `click doctor`
// is read-only by design (NFR-012).
package doctor

import (
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

// EngramChecksCount is the number of doctor checks contributed by Engram (plugin + binary + PATH
// persistence), kept as an exported constant so other packages/tests documenting Run()'s total
// check count don't have to hardcode a magic number that silently drifts if a check is added or
// removed here.
const EngramChecksCount = 3

// Context7ChecksCount is the number of doctor checks contributed by Context7, kept as an exported
// constant for the same reason as EngramChecksCount.
const Context7ChecksCount = 1

// CheckResult is the outcome of a single doctor check.
type CheckResult struct {
	Name    string
	Healthy bool
	Detail  string
}

// Report is the full set of check results from one Run.
type Report struct {
	Checks []CheckResult
}

// Healthy reports whether every check in the report passed.
func (r Report) Healthy() bool {
	for _, c := range r.Checks {
		if !c.Healthy {
			return false
		}
	}
	return true
}

// Run executes every current health check against cfg.ClaudeHome. It never mutates the
// filesystem.
func Run(cfg installer.Config) Report {
	return Report{Checks: []CheckResult{
		checkGit(cfg),
		checkClaude(cfg),
		checkPlugin(cfg),
		checkMemoryPlugin(cfg),
		checkReviewPlugin(cfg),
		checkSkillsPlugin(cfg),
		checkClaudeMD(cfg),
		checkMemoryGuardHook(cfg),
		checkClickBinary(cfg),
		checkModelsConfig(cfg),
		checkAppliedPluginConfig(cfg),
		checkEngramPlugin(cfg),
		checkEngramBinary(cfg),
		checkEngramPath(cfg),
		checkContext7(cfg),
	}}
}

// checkGit reports whether git is resolvable on PATH. It is foundational and read first (NFR-012:
// read-only — it only resolves PATH, never installs anything): click's plugin marketplace
// registration (`claude plugin marketplace add <source>`, run by both `click install` and `click
// update`) shells out to `git clone` under the hood, so a missing git breaks install/update deep
// inside plugin registration with a cryptic error — reproduced live on a fresh Windows VM with no
// git installed. When missing, Detail carries the exact same actionable message
// installer.GitMissingMessage that `click install`/`click update`'s own PreflightGit uses, so
// doctor and install/update never give a developer conflicting instructions (mirroring
// checkEngramBinary's shared-message contract with EngramBinaryRemediationMessage).
func checkGit(cfg installer.Config) CheckResult {
	const name = "git"

	path, ok := installer.GitPath()
	if ok {
		return CheckResult{Name: name, Healthy: true, Detail: "resuelto en " + path}
	}
	return CheckResult{Name: name, Healthy: false, Detail: installer.GitMissingMessage}
}

// checkClaude reports whether the claude CLI is resolvable on PATH. It is foundational and read
// right after checkGit (NFR-012: read-only — it only resolves PATH, never installs anything): click
// registers every plugin by shelling out to the claude CLI itself (SyncMarketplacePlugins →
// `claude plugin marketplace add`/`claude plugin install`, plugins.go's pluginCLIBinary), so a
// missing claude breaks install/update deep inside plugin registration with a cryptic "exec:
// \"claude\": executable file not found" — the exact PATH-lookup failure PreflightClaude guards
// against up front. When missing, Detail carries the exact same actionable message
// installer.ClaudeMissingMessage that `click install`/`click update`'s own PreflightClaude uses, so
// doctor and install/update never give a developer conflicting instructions (the same shared-message
// contract checkGit holds via GitMissingMessage, and checkEngramBinary via
// EngramBinaryRemediationMessage). This closes the observability asymmetry where a claude that fell
// off PATH after a working install would show doctor healthy while install/update immediately fail.
func checkClaude(cfg installer.Config) CheckResult {
	const name = "claude"

	path, ok := installer.ClaudePath()
	if ok {
		return CheckResult{Name: name, Healthy: true, Detail: "resuelto en " + path}
	}
	return CheckResult{Name: name, Healthy: false, Detail: installer.ClaudeMissingMessage}
}

func checkPlugin(cfg installer.Config) CheckResult {
	const name = "plugin click-sdd"

	ok, err := installer.HasInstalledPlugin(cfg, "click-sdd")
	if err != nil {
		return CheckResult{Name: name, Healthy: false, Detail: err.Error()}
	}
	if !ok {
		return CheckResult{Name: name, Healthy: false, Detail: "no registrado en Claude Code"}
	}
	return CheckResult{Name: name, Healthy: true, Detail: "registrado y habilitado"}
}

func checkMemoryPlugin(cfg installer.Config) CheckResult {
	const name = "plugin click-memory"

	ok, err := installer.HasInstalledPlugin(cfg, "click-memory")
	if err != nil {
		return CheckResult{Name: name, Healthy: false, Detail: err.Error()}
	}
	if !ok {
		return CheckResult{Name: name, Healthy: false, Detail: "no registrado en Claude Code"}
	}
	return CheckResult{Name: name, Healthy: true, Detail: "registrado y habilitado"}
}

func checkReviewPlugin(cfg installer.Config) CheckResult {
	const name = "plugin click-review"

	ok, err := installer.HasInstalledPlugin(cfg, "click-review")
	if err != nil {
		return CheckResult{Name: name, Healthy: false, Detail: err.Error()}
	}
	if !ok {
		return CheckResult{Name: name, Healthy: false, Detail: "no registrado en Claude Code"}
	}
	return CheckResult{Name: name, Healthy: true, Detail: "registrado y habilitado"}
}

func checkSkillsPlugin(cfg installer.Config) CheckResult {
	const name = "plugin click-skills"

	ok, err := installer.HasInstalledPlugin(cfg, "click-skills")
	if err != nil {
		return CheckResult{Name: name, Healthy: false, Detail: err.Error()}
	}
	if !ok {
		return CheckResult{Name: name, Healthy: false, Detail: "no registrado en Claude Code"}
	}
	return CheckResult{Name: name, Healthy: true, Detail: "registrado y habilitado"}
}

func checkClaudeMD(cfg installer.Config) CheckResult {
	const name = "CLAUDE.md managed block"

	ok, err := installer.HasManagedBlock(cfg.ClaudeMDPath())
	if err != nil {
		return CheckResult{Name: name, Healthy: false, Detail: err.Error()}
	}
	if !ok {
		return CheckResult{Name: name, Healthy: false, Detail: "bloque gestionado ausente en " + cfg.ClaudeMDPath()}
	}
	return CheckResult{Name: name, Healthy: true, Detail: "bloque gestionado presente"}
}

// checkEngramPlugin reports whether the engram@engram plugin is registered and enabled — the
// mechanism confirmed in Step 0 (spike-e-engram-install.md) that actually wires Engram's tools
// into a Claude Code session. It does not care whether click or the developer installed it.
func checkEngramPlugin(cfg installer.Config) CheckResult {
	const name = "plugin engram"

	ok, err := installer.HasInstalledPluginID(cfg, installer.EngramPluginID)
	if err != nil {
		return CheckResult{Name: name, Healthy: false, Detail: err.Error()}
	}
	if !ok {
		return CheckResult{Name: name, Healthy: false, Detail: "no registrado en Claude Code"}
	}
	return CheckResult{Name: name, Healthy: true, Detail: "registrado y habilitado"}
}

// checkEngramBinary reports whether the Engram binary the plugin's bundled MCP server needs
// (bare, PATH-resolved `command: "engram"` — confirmed in Step 0) actually resolves to a file on
// disk. The plugin can be registered and enabled yet still fail to connect if this binary is
// missing, so this is a separate check from checkEngramPlugin. This check is read-only (NFR-012:
// `click doctor` never mutates state) — it never attempts to provision the binary itself, unlike
// `click install`'s EnsureEngramBinary. When missing, the Detail includes the exact same
// remediation `go install` command that `click install`'s own non-fatal provisioning fallback shows
// (installer.EngramBinaryRemediationMessage), so doctor and install never give a developer
// conflicting instructions.
func checkEngramBinary(cfg installer.Config) CheckResult {
	const name = "engram binary"

	path, ok, err := installer.EngramBinaryResolvable(cfg)
	if err != nil {
		return CheckResult{Name: name, Healthy: false, Detail: err.Error()}
	}
	if ok {
		return CheckResult{Name: name, Healthy: true, Detail: "resuelto en " + path}
	}

	version := ""
	if m, mErr := manifest.Load(); mErr == nil {
		version = m.Engram.Version
	}
	return CheckResult{
		Name:    name,
		Healthy: false,
		Detail:  "no encontrado en " + path + " (el MCP de engram no podrá conectar). " + installer.EngramBinaryRemediationMessage(version),
	}
}

// checkEngramPath diagnoses PERSISTED-vs-LIVE PATH drift for the resolved Go bin dir the Engram
// binary needs to be on (installer.GoBinDir) — distinct from checkEngramBinary above, which asks
// "does the binary resolve/exist at all", regardless of why. This check answers a narrower,
// different question: "is the directory click's own PATH-persistence step
// (EnsureEngramBinary/persistPathToBinaryDir, sdd/engram-mcp-resolution obs #1436 D-5) writes to
// the user's PERSISTED PATH also visible in THIS live process's PATH right now". It is
// deliberately diagnose-only (NFR-012, design D-6): no `claude mcp list` connectivity probe
// (explicitly deferred scope per the design), no mutation, and no attempt to fix drift itself —
// `click install`/`click update`'s own persistPathToBinaryDir already does that.
//
// Four states:
//   - persisted=true,  live=true  -> healthy: everything lines up right now.
//   - persisted=true,  live=false -> the exact bug class this change targets, now CAUGHT instead
//     of silent: a PATH fix was already persisted (a prior install/update ran successfully) but
//     THIS doctor process (or any already-running Claude Code session) started before that and
//     still has a stale live PATH. Reported non-fatal (Healthy: true) with an actionable restart
//     message — the persisted state is actually correct going forward, so this must never fail
//     `click doctor`.
//   - persisted=false, live=true  -> an edge case: dir resolves in THIS session's live PATH
//     without click having persisted it (e.g. a developer's own manual `export PATH=...`) —
//     healthy, click just didn't put it there.
//   - persisted=false, live=false -> genuinely not configured at all: unhealthy, matching
//     checkEngramBinary's existing "not on PATH" severity.
func checkEngramPath(cfg installer.Config) CheckResult {
	const name = "engram PATH persistence"

	gobin, err := installer.GoBinDir(cfg)
	if err != nil {
		return CheckResult{
			Name:    name,
			Healthy: false,
			Detail:  "no se pudo resolver el directorio de binarios de Go (go env GOBIN/GOPATH): " + err.Error(),
		}
	}

	persisted, err := installer.PathPersisted(gobin)
	if err != nil {
		return CheckResult{Name: name, Healthy: false, Detail: err.Error()}
	}
	live := installer.LivePathContains(gobin)

	switch {
	case persisted && live:
		return CheckResult{Name: name, Healthy: true, Detail: gobin + " está persistido en el PATH y activo en esta sesión"}
	case persisted && !live:
		return CheckResult{
			Name:    name,
			Healthy: true,
			Detail: gobin + " ya está persistido en el PATH, pero esta sesión (o cualquier Claude Code ya en ejecución) " +
				"todavía no lo ve — reinicie la terminal o Claude Code para que tome efecto",
		}
	case !persisted && live:
		return CheckResult{Name: name, Healthy: true, Detail: gobin + " está en el PATH de esta sesión, aunque no fue persistido por click"}
	default:
		return CheckResult{
			Name:    name,
			Healthy: false,
			Detail: gobin + " no está en el PATH persistido ni en el de esta sesión — el MCP de engram podría no conectar " +
				"en una sesión nueva. Ejecute `click install` o `click update` para reintentar la persistencia.",
		}
	}
}

// checkContext7 reports whether Context7 is registered as a user-scope MCP server, read directly
// from Claude Code's own user config file (installer.HasContext7) — matching checkEngramPlugin's
// pure-file-read approach so `click doctor` never shells out to the real `claude` CLI (NFR-012:
// read-only by design).
func checkContext7(cfg installer.Config) CheckResult {
	const name = "context7 MCP"

	ok, err := installer.HasContext7(cfg)
	if err != nil {
		return CheckResult{Name: name, Healthy: false, Detail: err.Error()}
	}
	if !ok {
		return CheckResult{Name: name, Healthy: false, Detail: "no registrado como servidor MCP de usuario"}
	}
	return CheckResult{Name: name, Healthy: true, Detail: "registrado (scope user, https://mcp.context7.com/mcp)"}
}

// checkModelsConfig reports whether cfg.ModelsPath() holds a stale (pre-taxonomy-realignment or
// otherwise outdated schema_version) models.json. It only READS the file via installer.IsStale —
// never installer.MigrateIfStale — so `click doctor` stays read-only (NFR-012). An absent file is
// healthy: it just means defaults will be generated on the next `click install`/`click update`.
func checkModelsConfig(cfg installer.Config) CheckResult {
	const name = "models.json schema"

	stale, err := installer.IsStale(cfg)
	if err != nil {
		return CheckResult{Name: name, Healthy: false, Detail: err.Error()}
	}
	if stale {
		return CheckResult{
			Name:    name,
			Healthy: false,
			Detail:  "models.json usa una taxonomía o schema desactualizado — se regenerará (con backup en models.json.bak) en el próximo `click update`",
		}
	}
	return CheckResult{Name: name, Healthy: true, Detail: "actualizado (o aún no generado)"}
}

// checkAppliedPluginConfig reports whether every click-sdd `--config` key click computed it SHOULD
// configure (expectedClickSDDConfigKeys — modelconfig.ProfileConfigKey plus one ConfigKey() per
// modelconfig.Phases entry) is ACTUALLY present in Claude Code's own settings.json, under
// pluginConfigs[installer.ClickSDDPluginID].options — i.e. what Claude Code itself accepted and
// applied, not merely what click intended to send.
//
// This closes a real blind spot checkModelsConfig cannot see: checkModelsConfig only validates
// click's own models.json (what SHOULD be configured, computed from modelconfig). During a live
// incident, click-sdd's plugin.json grew from 13 to 18 model-config phases (adding the 5
// review_*_model userConfig keys), but because the plugin version never bumped, Claude Code cached
// a stale schema and silently DROPPED those 5 --config keys during `claude plugin install` — so
// the developer's real settings.json ended up with only 14 of the 19 expected keys, while `click
// doctor` kept reporting healthy the whole time, because checkModelsConfig never looks at the
// applied side at all. checkAppliedPluginConfig is a SEPARATE check from checkModelsConfig on
// purpose: the two answer different questions (should-be-configured vs. actually-applied) and
// either can go unhealthy independently of the other.
//
// It is intentionally read-only (NFR-012): it only reads settings.json via
// installer.AppliedClickSDDPluginConfig, never writes it.
func checkAppliedPluginConfig(cfg installer.Config) CheckResult {
	const name = "click-sdd applied plugin config"

	expected := expectedClickSDDConfigKeys()

	applied, found, err := installer.AppliedClickSDDPluginConfig(cfg)
	if err != nil {
		return CheckResult{Name: name, Healthy: false, Detail: err.Error()}
	}
	if !found {
		return CheckResult{
			Name:    name,
			Healthy: false,
			Detail:  "pluginConfigs[\"" + installer.ClickSDDPluginID + "\"] ausente en " + cfg.SettingsPath() + " — ejecute `click update` para aplicarla",
		}
	}

	var missing []string
	for _, key := range expected {
		if _, ok := applied[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return CheckResult{
			Name:    name,
			Healthy: false,
			Detail: "Claude Code no aplicó " + strconv.Itoa(len(missing)) + " de " + strconv.Itoa(len(expected)) +
				" claves esperadas (posible schema en caché desactualizado): " + strings.Join(missing, ", ") +
				" — ejecute `click update` para forzar un refresh del marketplace y reaplicarlas",
		}
	}

	return CheckResult{
		Name:    name,
		Healthy: true,
		Detail:  strconv.Itoa(len(expected)) + "/" + strconv.Itoa(len(expected)) + " claves aplicadas correctamente en " + cfg.SettingsPath(),
	}
}

// expectedClickSDDConfigKeys returns every plugin.json userConfig key click-sdd's --config flags
// SHOULD apply: modelconfig.ProfileConfigKey plus one Phase.ConfigKey() per modelconfig.Phases
// entry. It deliberately derives this from modelconfig — never a hardcoded literal count or key
// list — so checkAppliedPluginConfig auto-tracks any future taxonomy change (a phase added or
// removed) without a second manual edit here.
func expectedClickSDDConfigKeys() []string {
	keys := make([]string, 0, len(modelconfig.Phases)+1)
	keys = append(keys, modelconfig.ProfileConfigKey)
	for _, phase := range modelconfig.Phases {
		keys = append(keys, phase.ConfigKey())
	}
	return keys
}

func checkMemoryGuardHook(cfg installer.Config) CheckResult {
	const name = "memory-guard hook"

	ok, err := installer.HasMemoryGuardHook(cfg)
	if err != nil {
		return CheckResult{Name: name, Healthy: false, Detail: err.Error()}
	}
	if !ok {
		return CheckResult{Name: name, Healthy: false, Detail: "hook PreToolUse ausente en " + cfg.SettingsPath()}
	}
	return CheckResult{Name: name, Healthy: true, Detail: "hook PreToolUse registrado"}
}

// ClickBinaryMissingMessage is the actionable Spanish message shown when checkClickBinary cannot
// resolve the click binary on the live PATH (D10 convention: neutral register, no voseo). This is a
// security-relevant check, not a convenience one: the memory-guard PreToolUse hook is registered as
// the bare, PATH-resolved command "click memory-guard" (installer.MemoryGuardCommand,
// installer/hooksettings.go) — Claude Code itself resolves that command on PATH when it spawns the
// hook process. checkMemoryGuardHook above only confirms the hook ENTRY exists in settings.json; it
// says nothing about whether the command it names can actually run. On a machine where click is not
// on the PATH Claude Code's own process sees, the hook silently never fires: every mem_save payload
// flows straight into persistent memory with zero secrets/PII scanning, and nothing else in this
// package would ever surface it.
const ClickBinaryMissingMessage = "el binario click no está en el PATH visible para Claude Code. El hook memory-guard está registrado como el comando \"click memory-guard\" (resuelto por PATH cuando Claude Code lo ejecuta), así que si click no resuelve ahí el hook nunca se dispara y mem_save queda sin el escaneo de secretos/PII, sin ningún aviso. Verifique la instalación de click y asegúrese de que el comando click esté disponible en el PATH; luego reinicie Claude Code para que la sesión lo vea."

// clickBinaryLookup resolves the click binary on the live PATH. It is a package-level var — not a
// direct exec.LookPath call — so checkClickBinary's tests can override it deterministically,
// mirroring the injectable BinaryLookup seam installer.GitPath/ClaudePath use for the same reason:
// doctor tests must not depend on whether click happens to be on the real test machine's PATH.
var clickBinaryLookup = exec.LookPath

// SetClickBinaryLookupForTests overrides checkClickBinary's PATH-lookup dependency and returns a
// restore function, mirroring installer.SetEngramMarketplaceSourceForTests/
// SetBinaryLookupFactoryForTests's exported-seam-plus-restore-func shape. clickBinaryLookup itself
// stays package-private (this package's own checks_test.go pokes it directly), but downstream
// packages need a way in too: internal/cli's end-to-end command tests (TestDoctorCommand_AfterInstall_
// Succeeds and friends) run `click install` then `click doctor` and assert the install reports
// healthy — a real CI runner never has the click binary it just built anywhere on PATH, so without
// this seam those assertions are only true by accident of whichever machine happens to run them (a
// developer machine with click on PATH via scoop passes; CI, which never installed click to PATH,
// fails checkClickBinary and therefore the whole doctor report). Exported so those tests can force a
// deterministic outcome instead of depending on the real test machine's PATH.
func SetClickBinaryLookupForTests(lookup func(name string) (string, error)) func() {
	old := clickBinaryLookup
	clickBinaryLookup = lookup
	return func() { clickBinaryLookup = old }
}

// checkClickBinary reports whether the click binary itself is resolvable on the live PATH — the
// blind spot checkMemoryGuardHook cannot see (see ClickBinaryMissingMessage for the full rationale).
// This check is read-only (NFR-012: `click doctor` never mutates state) — it only resolves PATH, it
// never installs or repairs anything, mirroring checkGit/checkClaude's shape for the same class of
// "is this executable resolvable" question.
func checkClickBinary(cfg installer.Config) CheckResult {
	const name = "click binary"

	path, err := clickBinaryLookup("click")
	if err != nil {
		return CheckResult{Name: name, Healthy: false, Detail: ClickBinaryMissingMessage}
	}
	return CheckResult{Name: name, Healthy: true, Detail: "resuelto en " + path}
}
