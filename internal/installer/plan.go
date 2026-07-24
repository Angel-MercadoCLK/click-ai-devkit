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
	Snapshot         []string
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
	CloudConfigured bool
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
	StepActionSyncMarketplacePlugins       StepActionKind = "sync-marketplace-plugins"
	StepActionSyncEngram                   StepActionKind = "sync-engram"
	StepActionSyncEngramCloud              StepActionKind = "sync-engram-cloud"
	StepActionSyncContext7                 StepActionKind = "sync-context7"
	StepActionWriteClaudeManagedBlock      StepActionKind = "write-claude-managed-block"
	StepActionRegisterMemoryGuard          StepActionKind = "register-memory-guard"
	StepActionSaveModels                   StepActionKind = "save-models"
	StepActionSyncCodexGuidance            StepActionKind = "sync-codex-guidance"
	StepActionSyncCodexMCP                 StepActionKind = "sync-codex-mcp"
	StepActionConfigureCodexNativeModel    StepActionKind = "configure-codex-native-model"
	StepActionSyncOpenClawWorkspace        StepActionKind = "sync-openclaw-workspace"
	StepActionSyncOpenClawMCP              StepActionKind = "sync-openclaw-mcp"
	StepActionSyncOpenClawPlugin           StepActionKind = "sync-openclaw-plugin"
	StepActionSyncOpenClawSkills           StepActionKind = "sync-openclaw-skills"
	StepActionSyncOpenClawModelProfile     StepActionKind = "sync-openclaw-model-profile"
	StepActionConfigureOpenClawNativeModel StepActionKind = "configure-openclaw-native-model"
	StepActionRemoveMarketplacePlugins     StepActionKind = "remove-marketplace-plugins"
	StepActionStripClaudeManagedBlock      StepActionKind = "strip-claude-managed-block"
	StepActionUnregisterMemoryGuard        StepActionKind = "unregister-memory-guard"
	StepActionRemoveEngram                 StepActionKind = "remove-engram"
	StepActionRemoveContext7               StepActionKind = "remove-context7"
	StepActionRemoveOpenClawPlugin         StepActionKind = "remove-openclaw-plugin"
	StepActionRemoveOpenClawSkills         StepActionKind = "remove-openclaw-skills"
	StepActionStripOpenClawWorkspace       StepActionKind = "strip-openclaw-workspace"
	StepActionRemoveOpenClawModelProfile   StepActionKind = "remove-openclaw-model-profile"
	StepActionStripCodexGuidance           StepActionKind = "strip-codex-guidance"
	StepActionRemoveCodexMCP               StepActionKind = "remove-codex-mcp"
	StepActionRemoveTargetSelection        StepActionKind = "remove-target-selection"
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
	paths := make([]string, 0, len(p.Steps)*2)
	seen := map[string]struct{}{}
	for _, step := range p.Steps {
		for _, path := range step.Snapshot {
			if path == "" {
				continue
			}
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			paths = append(paths, path)
		}
	}
	return paths
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
	StepActionSyncOpenClawPlugin,
	StepActionSyncOpenClawSkills,
	StepActionSyncOpenClawModelProfile,
	StepActionConfigureOpenClawNativeModel,
	StepActionSyncCodexGuidance,
	StepActionSyncCodexMCP,
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
	StepActionSyncOpenClawPlugin,
	StepActionSyncOpenClawSkills,
	StepActionSyncOpenClawModelProfile,
	StepActionConfigureOpenClawNativeModel,
	StepActionSyncCodexGuidance,
	StepActionSyncCodexMCP,
	StepActionConfigureCodexNativeModel,
}

func BuildTargetPlan(cfg Config, selection TargetSelection, options PlanOptions) TargetPlan {
	steps := make([]Step, 0, 12)
	capabilities := make([]string, 0, 3)
	if selection.Claude {
		steps = append(steps,
			Step{ID: "claude-runtime", Target: PlanTargetClaude, Label: "Claude Code", Snapshot: []string{cfg.ClaudeMDPath(), cfg.SettingsPath()}, InstallActions: []StepActionKind{StepActionSyncMarketplacePlugins}, UpdateActions: []StepActionKind{StepActionSyncMarketplacePlugins}, UninstallActions: []StepActionKind{StepActionRemoveMarketplacePlugins}, DoctorChecks: []DoctorCheckKind{DoctorCheckClaude, DoctorCheckClickPluginRegistries, DoctorCheckClickSDDPlugin, DoctorCheckClickMemoryPlugin, DoctorCheckClickReviewPlugin, DoctorCheckClickSkillsPlugin, DoctorCheckClaudeManagedBlock}},
			Step{ID: "claude-models", Target: PlanTargetClaude, Label: "Claude model/profile", Snapshot: []string{cfg.ModelsPath()}, InstallActions: []StepActionKind{StepActionSaveModels}, UpdateActions: []StepActionKind{StepActionSaveModels}, DoctorChecks: []DoctorCheckKind{DoctorCheckModelsConfig, DoctorCheckAppliedPluginConfig}},
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
		codexModelStep := Step{ID: "codex-model", Target: PlanTargetCodex, Label: "Codex native model", Snapshot: []string{cfg.CodexConfigPath()}}
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
		steps = append(steps,
			Step{ID: "codex-runtime", Target: PlanTargetCodex, Label: "Codex CLI", Snapshot: []string{cfg.CodexAgentsMDPath()}, InstallActions: []StepActionKind{StepActionSyncCodexGuidance}, UpdateActions: []StepActionKind{StepActionSyncCodexGuidance}, UninstallActions: []StepActionKind{StepActionStripCodexGuidance}, DoctorChecks: []DoctorCheckKind{DoctorCheckCodexGuidance}},
			codexMCPStep,
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
		steps = append(steps,
			Step{ID: "openclaw-runtime", Target: PlanTargetOpenClaw, Label: "OpenClaw", Snapshot: []string{cfg.OpenClawAgentsMDPath(), cfg.OpenClawSoulMDPath(), cfg.OpenClawMCPConfigPath(), cfg.OpenClawModelProfilePath()}, InstallActions: []StepActionKind{StepActionSyncOpenClawWorkspace, StepActionSyncOpenClawMCP, StepActionSyncOpenClawPlugin, StepActionSyncOpenClawSkills, StepActionSyncOpenClawModelProfile}, UpdateActions: []StepActionKind{StepActionSyncOpenClawWorkspace, StepActionSyncOpenClawMCP, StepActionSyncOpenClawPlugin, StepActionSyncOpenClawSkills, StepActionSyncOpenClawModelProfile}, UninstallActions: []StepActionKind{StepActionStripOpenClawWorkspace, StepActionRemoveOpenClawPlugin, StepActionRemoveOpenClawSkills}, DoctorChecks: []DoctorCheckKind{DoctorCheckOpenClaw}},
			openClawModelStep,
		)
		capabilities = append(capabilities, "OpenClaw: workspace, skills y modelo provider/model mediante su CLI")
	}
	if selection.Claude || selection.OpenClaw {
		steps = append(steps, Step{ID: "engram", Target: PlanTargetShared, Label: "Engram", Snapshot: []string{cfg.EngramStatePath()}, InstallActions: []StepActionKind{StepActionSyncEngram}, UpdateActions: []StepActionKind{StepActionSyncEngram}, UninstallActions: []StepActionKind{StepActionRemoveEngram}, DoctorChecks: []DoctorCheckKind{DoctorCheckEngramPlugin, DoctorCheckEngramSubagentVisibility, DoctorCheckEngramBinary, DoctorCheckEngramPath}})
	}
	if options.CloudConfigured && selection.Claude {
		steps = append(steps, Step{ID: "engram-cloud", Target: PlanTargetShared, Label: "Engram Cloud", Snapshot: []string{cfg.EngramCloudStatePath()}, InstallActions: []StepActionKind{StepActionSyncEngramCloud}, UpdateActions: []StepActionKind{StepActionSyncEngramCloud}, DoctorChecks: []DoctorCheckKind{DoctorCheckEngramCloud}})
	}
	if selection.Claude {
		steps = append(steps,
			Step{ID: "context7", Target: PlanTargetShared, Label: "Context7", Snapshot: []string{cfg.Context7StatePath(), cfg.Context7ConfigPath()}, InstallActions: []StepActionKind{StepActionSyncContext7}, UpdateActions: []StepActionKind{StepActionSyncContext7}, UninstallActions: []StepActionKind{StepActionRemoveContext7}, DoctorChecks: []DoctorCheckKind{DoctorCheckContext7}},
			Step{ID: "plugins", Target: PlanTargetShared, Label: "plugins", Snapshot: []string{cfg.KnownMarketplacesPath(), cfg.InstalledPluginsPath()}},
			Step{ID: "memory-guard", Target: PlanTargetShared, Label: "memory guard", Snapshot: []string{cfg.SettingsPath()}, InstallActions: []StepActionKind{StepActionWriteClaudeManagedBlock, StepActionRegisterMemoryGuard}, UpdateActions: []StepActionKind{StepActionWriteClaudeManagedBlock, StepActionRegisterMemoryGuard}, UninstallActions: []StepActionKind{StepActionStripClaudeManagedBlock, StepActionUnregisterMemoryGuard}, DoctorChecks: []DoctorCheckKind{DoctorCheckMemoryGuard}},
		)
	}
	steps = append(steps, Step{ID: "sdd-assets", Target: PlanTargetShared, Label: "SDD assets", Snapshot: []string{cfg.TargetSelectionPath()}, UninstallActions: []StepActionKind{StepActionRemoveTargetSelection}})
	return TargetPlan{Selection: selection, Capabilities: capabilities, Steps: steps}
}
