package installer

import "strings"

type PlanTarget string

const (
	PlanTargetClaude   PlanTarget = "claude"
	PlanTargetCodex    PlanTarget = "codex"
	PlanTargetOpenClaw PlanTarget = "openclaw"
	PlanTargetShared   PlanTarget = "shared"
)

type Step struct {
	ID               string
	Target           PlanTarget
	Label            string
	Snapshot         []SnapshotDecl
	InstallActions   []StepActionKind
	UpdateActions    []StepActionKind
	UninstallActions []StepActionKind
	DoctorChecks     []DoctorCheckKind
}

type TargetPlan struct {
	Selection    TargetSelection
	Capabilities []string
	Steps        []Step
}

type PlanOptions struct {
	CloudResolvable bool
	// CodexNativeModel / OpenClawNativeModel are set ONLY when the developer explicitly opted into a
	// native-model mutation via --codex-model / --openclaw-model. When false (the normal case), the
	// corresponding native-model write action is omitted from the plan entirely, so a plain install
	// neither lists nor runs any native-model mutation — Codex and OpenClaw keep running the portable
	// SDD against the model each already has configured in its own installation.
	CodexNativeModel    bool
	OpenClawNativeModel bool
}

type StepActionKind string

const (
	StepActionSyncMarketplacePlugins          StepActionKind = "sync-marketplace-plugins"
	StepActionSyncEngram                      StepActionKind = "sync-engram"
	StepActionConfigureEngramCloudSessionSync StepActionKind = "configure-engram-cloud-session-sync"
	StepActionSyncEngramCloud                 StepActionKind = "sync-engram-cloud"
	StepActionSyncContext7                    StepActionKind = "sync-context7"
	StepActionWriteClaudeManagedBlock         StepActionKind = "write-claude-managed-block"
	StepActionRegisterMemoryGuard             StepActionKind = "register-memory-guard"
	StepActionSaveModels                      StepActionKind = "save-models"
	StepActionSyncCodexGuidance               StepActionKind = "sync-codex-guidance"
	StepActionSyncCodexMCP                    StepActionKind = "sync-codex-mcp"
	StepActionConfigureCodexNativeModel       StepActionKind = "configure-codex-native-model"
	StepActionSyncOpenClawWorkspace           StepActionKind = "sync-openclaw-workspace"
	StepActionSyncOpenClawMCP                 StepActionKind = "sync-openclaw-mcp"
	StepActionRegisterOpenClawMCP             StepActionKind = "register-openclaw-mcp"
	StepActionUnregisterOpenClawMCP           StepActionKind = "unregister-openclaw-mcp"
	StepActionSyncOpenClawPlugin              StepActionKind = "sync-openclaw-plugin"
	StepActionSyncOpenClawSkills              StepActionKind = "sync-openclaw-skills"
	StepActionSyncOpenClawModelProfile        StepActionKind = "sync-openclaw-model-profile"
	StepActionConfigureOpenClawNativeModel    StepActionKind = "configure-openclaw-native-model"
	StepActionRemoveModels                    StepActionKind = "remove-models"
	StepActionRemoveEngramCloudState          StepActionKind = "remove-engram-cloud-state"
	StepActionRemoveMarketplacePlugins        StepActionKind = "remove-marketplace-plugins"
	StepActionStripClaudeManagedBlock         StepActionKind = "strip-claude-managed-block"
	StepActionUnregisterMemoryGuard           StepActionKind = "unregister-memory-guard"
	StepActionRemoveEngram                    StepActionKind = "remove-engram"
	StepActionRemoveContext7                  StepActionKind = "remove-context7"
	StepActionRemoveOpenClawPlugin            StepActionKind = "remove-openclaw-plugin"
	StepActionRemoveOpenClawSkills            StepActionKind = "remove-openclaw-skills"
	StepActionStripOpenClawWorkspace          StepActionKind = "strip-openclaw-workspace"
	StepActionRemoveOpenClawModelProfile      StepActionKind = "remove-openclaw-model-profile"
	StepActionSaveCodexModelProfile           StepActionKind = "save-codex-model-profile"
	StepActionRemoveCodexModelProfile         StepActionKind = "remove-codex-model-profile"
	StepActionStripCodexGuidance              StepActionKind = "strip-codex-guidance"
	StepActionRemoveCodexMCP                  StepActionKind = "remove-codex-mcp"
	StepActionRemoveTargetSelection           StepActionKind = "remove-target-selection"
)

type DoctorCheckKind string

const (
	DoctorCheckClaude                   DoctorCheckKind = "claude"
	DoctorCheckOpenClaw                 DoctorCheckKind = "openclaw"
	DoctorCheckOpenClawNativeModel      DoctorCheckKind = "openclaw-native-model"
	DoctorCheckClickPluginRegistries    DoctorCheckKind = "click-plugin-registries"
	DoctorCheckClickSDDPlugin           DoctorCheckKind = "click-sdd-plugin"
	DoctorCheckClickMemoryPlugin        DoctorCheckKind = "click-memory-plugin"
	DoctorCheckClickReviewPlugin        DoctorCheckKind = "click-review-plugin"
	DoctorCheckClickSkillsPlugin        DoctorCheckKind = "click-skills-plugin"
	DoctorCheckClaudeManagedBlock       DoctorCheckKind = "claude-managed-block"
	DoctorCheckMemoryGuard              DoctorCheckKind = "memory-guard"
	DoctorCheckModelsConfig             DoctorCheckKind = "models-config"
	DoctorCheckAppliedPluginConfig      DoctorCheckKind = "applied-plugin-config"
	DoctorCheckEngramPlugin             DoctorCheckKind = "engram-plugin"
	DoctorCheckEngramSubagentVisibility DoctorCheckKind = "engram-subagent-visibility"
	DoctorCheckEngramBinary             DoctorCheckKind = "engram-binary"
	DoctorCheckEngramPath               DoctorCheckKind = "engram-path"
	DoctorCheckEngramCloud              DoctorCheckKind = "engram-cloud"
	DoctorCheckContext7                 DoctorCheckKind = "context7"
	DoctorCheckCodexGuidance            DoctorCheckKind = "codex-guidance"
)

func (p TargetPlan) StepLabels() []string {
	labels := make([]string, len(p.Steps))
	for i, step := range p.Steps {
		labels[i] = step.Label
	}
	return labels
}

func (p TargetPlan) CapabilitiesSummary() string {
	return strings.Join(p.Capabilities, "; ")
}

func (p TargetPlan) SnapshotPaths() []string {
	// Derive from typed declarations for backward compatibility with preview.go
	paths := make([]string, 0, len(p.Steps)*2)
	seen := map[string]struct{}{}
	for _, step := range p.Steps {
		for _, decl := range step.Snapshot {
			if decl.Path == "" {
				continue
			}
			if _, ok := seen[decl.Path]; ok {
				continue
			}
			seen[decl.Path] = struct{}{}
			paths = append(paths, decl.Path)
		}
	}
	return paths
}

// SnapshotSpecs returns the typed snapshot declarations from all steps, preserving order
// and deduplicating paths. Each path appears at most once in the result.
func (p TargetPlan) SnapshotSpecs() []SnapshotDecl {
	specs := make([]SnapshotDecl, 0, len(p.Steps)*2)
	seen := map[string]struct{}{}
	for _, step := range p.Steps {
		for _, decl := range step.Snapshot {
			if decl.Path == "" {
				continue
			}
			if _, ok := seen[decl.Path]; ok {
				continue
			}
			seen[decl.Path] = struct{}{}
			specs = append(specs, decl)
		}
	}
	return specs
}

func (p TargetPlan) InstallActionKinds() []StepActionKind {
	return collectOrderedActionKinds(p.Steps, installActionOrder, func(step Step) []StepActionKind { return step.InstallActions })
}

func (p TargetPlan) UpdateActionKinds() []StepActionKind {
	return collectOrderedActionKinds(p.Steps, updateActionOrder, func(step Step) []StepActionKind { return step.UpdateActions })
}

func (p TargetPlan) UninstallActionKinds() []StepActionKind {
	return collectActionKinds(p.Steps, func(step Step) []StepActionKind { return step.UninstallActions })
}

func (p TargetPlan) DoctorCheckKinds() []DoctorCheckKind {
	checks := make([]DoctorCheckKind, 0, len(p.Steps)*2)
	seen := map[DoctorCheckKind]struct{}{}
	for _, step := range p.Steps {
		for _, check := range step.DoctorChecks {
			if _, ok := seen[check]; ok {
				continue
			}
			seen[check] = struct{}{}
			checks = append(checks, check)
		}
	}
	return checks
}

func collectActionKinds(steps []Step, actionsFor func(Step) []StepActionKind) []StepActionKind {
	actions := make([]StepActionKind, 0, len(steps)*2)
	for _, step := range steps {
		actions = append(actions, actionsFor(step)...)
	}
	return actions
}

func collectOrderedActionKinds(steps []Step, order []StepActionKind, actionsFor func(Step) []StepActionKind) []StepActionKind {
	present := map[StepActionKind]struct{}{}
	for _, step := range steps {
		for _, action := range actionsFor(step) {
			present[action] = struct{}{}
		}
	}
	actions := make([]StepActionKind, 0, len(present))
	for _, action := range order {
		if _, ok := present[action]; ok {
			actions = append(actions, action)
		}
	}
	return actions
}

var installActionOrder = []StepActionKind{
	StepActionSyncMarketplacePlugins,
	StepActionSyncEngram,
	StepActionSyncEngramCloud,
	StepActionSyncContext7,
	StepActionWriteClaudeManagedBlock,
	StepActionRegisterMemoryGuard,
	StepActionSaveModels,
	StepActionSyncOpenClawWorkspace,
	StepActionSyncOpenClawMCP,
	StepActionRegisterOpenClawMCP,
	StepActionSyncOpenClawPlugin,
	StepActionSyncOpenClawSkills,
	StepActionSyncOpenClawModelProfile,
	StepActionConfigureOpenClawNativeModel,
	StepActionSyncCodexGuidance,
	StepActionSyncCodexMCP,
	StepActionSaveCodexModelProfile,
	StepActionConfigureCodexNativeModel,
}

var updateActionOrder = []StepActionKind{
	StepActionSyncMarketplacePlugins,
	StepActionSaveModels,
	StepActionWriteClaudeManagedBlock,
	StepActionRegisterMemoryGuard,
	StepActionSyncEngram,
	StepActionSyncEngramCloud,
	StepActionSyncContext7,
	StepActionSyncOpenClawWorkspace,
	StepActionSyncOpenClawMCP,
	StepActionRegisterOpenClawMCP,
	StepActionSyncOpenClawPlugin,
	StepActionSyncOpenClawSkills,
	StepActionSyncOpenClawModelProfile,
	StepActionConfigureOpenClawNativeModel,
	StepActionSyncCodexGuidance,
	StepActionSyncCodexMCP,
	StepActionSaveCodexModelProfile,
	StepActionConfigureCodexNativeModel,
}

func BuildTargetPlan(cfg Config, selection TargetSelection, options PlanOptions) TargetPlan {
	steps := make([]Step, 0, 12)
	capabilities := make([]string, 0, 3)
	if selection.Claude {
		steps = append(steps,
			Step{ID: "claude-runtime", Target: PlanTargetClaude, Label: "Claude Code", Snapshot: []SnapshotDecl{snapshot(cfg.ClaudeMDPath(), DriftPolicyManagedContentVeto), snapshot(cfg.SettingsPath(), DriftPolicyManagedContentVeto)}, InstallActions: []StepActionKind{StepActionSyncMarketplacePlugins}, UpdateActions: []StepActionKind{StepActionSyncMarketplacePlugins}, UninstallActions: []StepActionKind{StepActionRemoveMarketplacePlugins}, DoctorChecks: []DoctorCheckKind{DoctorCheckClaude, DoctorCheckClickPluginRegistries, DoctorCheckClickSDDPlugin, DoctorCheckClickMemoryPlugin, DoctorCheckClickReviewPlugin, DoctorCheckClickSkillsPlugin, DoctorCheckClaudeManagedBlock}},
			// StepActionRemoveModels is StepActionSaveModels' reversal: without it, click's own
			// models.json (the active profile + per-phase model selection) survived `click uninstall`
			// forever, so a later reinstall silently inherited the previous run's choices.
			Step{ID: "claude-models", Target: PlanTargetClaude, Label: "Claude model/profile", Snapshot: []SnapshotDecl{snapshot(cfg.ModelsPath(), DriftPolicyWholeFileVeto)}, InstallActions: []StepActionKind{StepActionSaveModels}, UpdateActions: []StepActionKind{StepActionSaveModels}, UninstallActions: []StepActionKind{StepActionRemoveModels}, DoctorChecks: []DoctorCheckKind{DoctorCheckModelsConfig, DoctorCheckAppliedPluginConfig}},
		)
		capabilities = append(capabilities, "Claude Code: plugins nativos, SDD, perfiles y modelos por fase")
	}
	if selection.Codex {
		// The codex-model step keeps its config.toml snapshot unconditionally (so an explicit
		// --codex-model mutation can always roll back), but only carries the native-model write
		// ACTION when the developer opted in via PlanOptions.CodexNativeModel. Without the flag, the
		// step is inert: config.toml is never touched and the write is never listed or run.
		//
		// DECISION: it has NO UninstallActions. `click uninstall` deliberately does NOT revert the
		// config.toml `model =` value: it is a user-owned setting, and reverting it is ambiguous (there
		// is no unambiguous "previous" value to restore to). Uninstall only reverses click-owned
		// managed blocks, click-owned state files, and MCP registrations — never native model values.
		codexModelStep := Step{ID: "codex-model", Target: PlanTargetCodex, Label: "Codex native model", Snapshot: []SnapshotDecl{snapshot(cfg.CodexConfigPath(), DriftPolicyNonVeto)}}
		if options.CodexNativeModel {
			codexModelStep.InstallActions = []StepActionKind{StepActionConfigureCodexNativeModel}
			codexModelStep.UpdateActions = []StepActionKind{StepActionConfigureCodexNativeModel}
		}
		// codex-mcp always runs when Codex is a target — independent of the --codex-model native-model
		// opt-in (unlike codexModelStep above): registering Engram's MCP server is a supplementary,
		// non-fatal integration (D45 pattern), always attempted, never gated behind a flag. No Snapshot
		// paths: SyncCodexMCP makes zero file writes, so there is nothing for click's own backup/rollback
		// to capture — it is pure CLI state delegated to `codex mcp`.
		codexMCPStep := Step{ID: "codex-mcp", Target: PlanTargetCodex, Label: "Codex Engram MCP", InstallActions: []StepActionKind{StepActionSyncCodexMCP}, UpdateActions: []StepActionKind{StepActionSyncCodexMCP}, UninstallActions: []StepActionKind{StepActionRemoveCodexMCP}}
		// codex-model-profile persists Click's own portable tier recommendation. It is reference data
		// only — it never writes Codex's native config.toml or any ~/.codex/sdd-*.config.toml profile.
		codexModelProfileStep := Step{ID: "codex-model-profile", Target: PlanTargetCodex, Label: "Codex model profile", Snapshot: []SnapshotDecl{snapshot(cfg.CodexModelProfilePath(), DriftPolicyWholeFileVeto)}, InstallActions: []StepActionKind{StepActionSaveCodexModelProfile}, UpdateActions: []StepActionKind{StepActionSaveCodexModelProfile}, UninstallActions: []StepActionKind{StepActionRemoveCodexModelProfile}}
		steps = append(steps,
			Step{ID: "codex-runtime", Target: PlanTargetCodex, Label: "Codex CLI", Snapshot: []SnapshotDecl{snapshot(cfg.CodexAgentsMDPath(), DriftPolicyManagedContentVeto)}, InstallActions: []StepActionKind{StepActionSyncCodexGuidance}, UpdateActions: []StepActionKind{StepActionSyncCodexGuidance}, UninstallActions: []StepActionKind{StepActionStripCodexGuidance}, DoctorChecks: []DoctorCheckKind{DoctorCheckCodexGuidance}},
			codexMCPStep,
			codexModelProfileStep,
			codexModelStep,
		)
		capabilities = append(capabilities, "Codex CLI: AGENTS.md gestionado, Engram MCP registrado y modelo nativo de config.toml")
	}
	if selection.OpenClaw {
		// The openclaw-model step always keeps its native-model doctor check, but only carries the
		// native `openclaw config set` write ACTION when the developer opted in via
		// PlanOptions.OpenClawNativeModel. Without --openclaw-model the step runs no CLI mutation:
		// OpenClaw keeps the provider/model the developer already connected. The portable model
		// metadata (StepActionSyncOpenClawModelProfile) still runs regardless as part of the runtime
		// step below.
		//
		// DECISION: its UninstallAction removes ONLY click's own model-profile.json
		// (StepActionRemoveOpenClawModelProfile). `click uninstall` deliberately does NOT revert the
		// native provider/model set via `openclaw config set`: like Codex's config.toml model above,
		// that is a user-owned setting and reverting it is ambiguous.
		openClawModelStep := Step{ID: "openclaw-model", Target: PlanTargetOpenClaw, Label: "OpenClaw native model", UninstallActions: []StepActionKind{StepActionRemoveOpenClawModelProfile}, DoctorChecks: []DoctorCheckKind{DoctorCheckOpenClawNativeModel}}
		if options.OpenClawNativeModel {
			openClawModelStep.InstallActions = []StepActionKind{StepActionConfigureOpenClawNativeModel}
			openClawModelStep.UpdateActions = []StepActionKind{StepActionConfigureOpenClawNativeModel}
		}
		openClawMCPStep := Step{ID: "openclaw-mcp", Target: PlanTargetOpenClaw, Label: "OpenClaw Engram MCP", InstallActions: []StepActionKind{StepActionRegisterOpenClawMCP}, UpdateActions: []StepActionKind{StepActionRegisterOpenClawMCP}, UninstallActions: []StepActionKind{StepActionUnregisterOpenClawMCP}}
		steps = append(steps,
			Step{ID: "openclaw-runtime", Target: PlanTargetOpenClaw, Label: "OpenClaw", Snapshot: []SnapshotDecl{snapshot(cfg.OpenClawAgentsMDPath(), DriftPolicyManagedContentVeto), snapshot(cfg.OpenClawSoulMDPath(), DriftPolicyWholeFileVeto), snapshot(cfg.OpenClawMCPConfigPath(), DriftPolicyWholeFileVeto), snapshot(cfg.OpenClawModelProfilePath(), DriftPolicyWholeFileVeto)}, InstallActions: []StepActionKind{StepActionSyncOpenClawWorkspace, StepActionSyncOpenClawMCP, StepActionSyncOpenClawPlugin, StepActionSyncOpenClawSkills, StepActionSyncOpenClawModelProfile}, UpdateActions: []StepActionKind{StepActionSyncOpenClawWorkspace, StepActionSyncOpenClawMCP, StepActionSyncOpenClawPlugin, StepActionSyncOpenClawSkills, StepActionSyncOpenClawModelProfile}, UninstallActions: []StepActionKind{StepActionStripOpenClawWorkspace, StepActionRemoveOpenClawPlugin, StepActionRemoveOpenClawSkills}, DoctorChecks: []DoctorCheckKind{DoctorCheckOpenClaw}},
			openClawMCPStep,
			openClawModelStep,
		)
		capabilities = append(capabilities, "OpenClaw: workspace, skills y modelo provider/model mediante su CLI")
	}
	if selection.Claude || selection.OpenClaw {
		steps = append(steps, Step{ID: "engram", Target: PlanTargetShared, Label: "Engram", Snapshot: []SnapshotDecl{snapshot(cfg.EngramStatePath(), DriftPolicyWholeFileVeto)}, InstallActions: []StepActionKind{StepActionSyncEngram}, UpdateActions: []StepActionKind{StepActionSyncEngram}, UninstallActions: []StepActionKind{StepActionRemoveEngram}, DoctorChecks: []DoctorCheckKind{DoctorCheckEngramPlugin, DoctorCheckEngramSubagentVisibility, DoctorCheckEngramBinary, DoctorCheckEngramPath}})
	}
	if options.CloudResolvable && selection.Claude {
		// StepActionRemoveEngramCloudState is StepActionSyncEngramCloud's reversal. RemoveEngramCloudState
		// already existed but was reachable only from the zero-caller legacy installer.Uninstall, so
		// engram-cloud.json survived `click uninstall`. It stays offline: it deletes click's own local
		// enrollment record and never un-enrolls the shared cloud project other machines still use.
		//
		// With CloudResolvable (server and project known, token optional), we always include
		// StepActionConfigureEngramCloudSessionSync to write env block and hook, while
		// StepActionSyncEngramCloud remains exclusively for D40 enrollment when token is present.
		steps = append(steps, Step{ID: "engram-cloud", Target: PlanTargetShared, Label: "Engram Cloud", Snapshot: []SnapshotDecl{snapshot(cfg.EngramCloudStatePath(), DriftPolicyWholeFileVeto)}, InstallActions: []StepActionKind{StepActionConfigureEngramCloudSessionSync, StepActionSyncEngramCloud}, UpdateActions: []StepActionKind{StepActionConfigureEngramCloudSessionSync, StepActionSyncEngramCloud}, UninstallActions: []StepActionKind{StepActionRemoveEngramCloudState}, DoctorChecks: []DoctorCheckKind{DoctorCheckEngramCloud}})
	}
	if selection.Claude {
		steps = append(steps,
			Step{ID: "context7", Target: PlanTargetShared, Label: "Context7", Snapshot: []SnapshotDecl{snapshot(cfg.Context7StatePath(), DriftPolicyWholeFileVeto), snapshot(cfg.Context7ConfigPath(), DriftPolicyNonVeto)}, InstallActions: []StepActionKind{StepActionSyncContext7}, UpdateActions: []StepActionKind{StepActionSyncContext7}, UninstallActions: []StepActionKind{StepActionRemoveContext7}, DoctorChecks: []DoctorCheckKind{DoctorCheckContext7}},
			Step{ID: "plugins", Target: PlanTargetShared, Label: "plugins", Snapshot: []SnapshotDecl{snapshot(cfg.KnownMarketplacesPath(), DriftPolicyNonVeto), snapshot(cfg.InstalledPluginsPath(), DriftPolicyNonVeto)}},
			Step{ID: "memory-guard", Target: PlanTargetShared, Label: "memory guard", Snapshot: []SnapshotDecl{snapshot(cfg.SettingsPath(), DriftPolicyManagedContentVeto)}, InstallActions: []StepActionKind{StepActionWriteClaudeManagedBlock, StepActionRegisterMemoryGuard}, UpdateActions: []StepActionKind{StepActionWriteClaudeManagedBlock, StepActionRegisterMemoryGuard}, UninstallActions: []StepActionKind{StepActionStripClaudeManagedBlock, StepActionUnregisterMemoryGuard}, DoctorChecks: []DoctorCheckKind{DoctorCheckMemoryGuard}},
		)
	}
	steps = append(steps, Step{ID: "sdd-assets", Target: PlanTargetShared, Label: "SDD assets", Snapshot: []SnapshotDecl{snapshot(cfg.TargetSelectionPath(), DriftPolicyWholeFileVeto)}, UninstallActions: []StepActionKind{StepActionRemoveTargetSelection}})
	return TargetPlan{Selection: selection, Capabilities: capabilities, Steps: steps}
}
