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
}

type StepActionKind string

const (
	StepActionSyncGentleAI                    StepActionKind = "sync-gentle-ai"
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
	StepActionRemoveEngramCloudSessionSync    StepActionKind = "remove-engram-cloud-session-sync"
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
	DoctorCheckClaude                 DoctorCheckKind = "claude"
	DoctorCheckOpenClaw               DoctorCheckKind = "openclaw"
	DoctorCheckOpenClawNativeModel    DoctorCheckKind = "openclaw-native-model"
	DoctorCheckClickPluginRegistries  DoctorCheckKind = "click-plugin-registries"
	DoctorCheckClickMemoryPlugin      DoctorCheckKind = "click-memory-plugin"
	DoctorCheckClickReviewPlugin      DoctorCheckKind = "click-review-plugin"
	DoctorCheckClickSkillsPlugin      DoctorCheckKind = "click-skills-plugin"
	DoctorCheckClaudeManagedBlock     DoctorCheckKind = "claude-managed-block"
	DoctorCheckMemoryGuard            DoctorCheckKind = "memory-guard"
	DoctorCheckModelsConfig           DoctorCheckKind = "models-config"
	DoctorCheckAppliedPluginConfig    DoctorCheckKind = "applied-plugin-config"
	DoctorCheckEngramPlugin           DoctorCheckKind = "engram-plugin"
	DoctorCheckEngramBinary           DoctorCheckKind = "engram-binary"
	DoctorCheckEngramPath             DoctorCheckKind = "engram-path"
	DoctorCheckEngramCloud            DoctorCheckKind = "engram-cloud"
	DoctorCheckEngramCloudSessionSync DoctorCheckKind = "engram-cloud-session-sync"
	DoctorCheckContext7               DoctorCheckKind = "context7"
	DoctorCheckCodexGuidance          DoctorCheckKind = "codex-guidance"
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
	StepActionSyncGentleAI,
	StepActionSyncMarketplacePlugins,
	StepActionSyncEngram,
	StepActionConfigureEngramCloudSessionSync,
	StepActionSyncEngramCloud,
	StepActionSyncContext7,
	StepActionWriteClaudeManagedBlock,
	StepActionRegisterMemoryGuard,
	StepActionSyncOpenClawWorkspace,
	StepActionRegisterOpenClawMCP,
	StepActionSyncOpenClawPlugin,
	StepActionSyncOpenClawSkills,
	StepActionSyncCodexGuidance,
	StepActionSyncCodexMCP,
}

var updateActionOrder = []StepActionKind{
	StepActionSyncGentleAI,
	StepActionSyncMarketplacePlugins,
	StepActionWriteClaudeManagedBlock,
	StepActionRegisterMemoryGuard,
	StepActionSyncEngram,
	StepActionConfigureEngramCloudSessionSync,
	StepActionSyncEngramCloud,
	StepActionSyncContext7,
	StepActionSyncOpenClawWorkspace,
	StepActionRegisterOpenClawMCP,
	StepActionSyncOpenClawPlugin,
	StepActionSyncOpenClawSkills,
	StepActionSyncCodexGuidance,
	StepActionSyncCodexMCP,
}

func BuildTargetPlan(cfg Config, selection TargetSelection, options PlanOptions) TargetPlan {
	steps := make([]Step, 0, 12)
	capabilities := make([]string, 0, 3)
	if selection.Claude || selection.Codex || selection.OpenClaw {
		steps = append(steps, Step{ID: "gentle-ai", Target: PlanTargetShared, Label: "gentle-ai", InstallActions: []StepActionKind{StepActionSyncGentleAI}, UpdateActions: []StepActionKind{StepActionSyncGentleAI}})
	}
	if selection.Claude {
		claudeRuntime := Step{ID: "claude-runtime", Target: PlanTargetClaude, Label: "Claude Code", Snapshot: []SnapshotDecl{snapshot(cfg.ClaudeMDPath(), DriftPolicyManagedContentVeto), snapshot(cfg.SettingsPath(), DriftPolicyManagedContentVeto)}, InstallActions: []StepActionKind{StepActionSyncMarketplacePlugins}, UpdateActions: []StepActionKind{StepActionSyncMarketplacePlugins}, UninstallActions: []StepActionKind{StepActionRemoveMarketplacePlugins}, DoctorChecks: []DoctorCheckKind{DoctorCheckClaude, DoctorCheckClickPluginRegistries, DoctorCheckClickMemoryPlugin, DoctorCheckClickReviewPlugin, DoctorCheckClickSkillsPlugin, DoctorCheckClaudeManagedBlock, DoctorCheckEngramPlugin, DoctorCheckEngramBinary, DoctorCheckEngramPath, DoctorCheckContext7}}
		if !options.CloudResolvable {
			claudeRuntime.UninstallActions = append(claudeRuntime.UninstallActions, StepActionRemoveEngramCloudSessionSync, StepActionRemoveEngramCloudState)
		}
		steps = append(steps,
			claudeRuntime,
		)
		capabilities = append(capabilities, "Claude Code: plugins nativos y assets portables")
	}
	if selection.Codex {
		steps = append(steps,
			Step{ID: "codex-runtime", Target: PlanTargetCodex, Label: "Codex CLI", Snapshot: []SnapshotDecl{snapshot(cfg.CodexAgentsMDPath(), DriftPolicyManagedContentVeto), snapshot(cfg.CodexConfigPath(), DriftPolicyNonVeto)}, InstallActions: []StepActionKind{StepActionSyncCodexGuidance}, UpdateActions: []StepActionKind{StepActionSyncCodexGuidance}, UninstallActions: []StepActionKind{StepActionStripCodexGuidance}, DoctorChecks: []DoctorCheckKind{DoctorCheckCodexGuidance}},
		)
		capabilities = append(capabilities, "Codex CLI: AGENTS.md gestionado y componentes MCP configurados por gentle-ai")
	}
	if selection.OpenClaw {
		steps = append(steps,
			Step{ID: "openclaw-runtime", Target: PlanTargetOpenClaw, Label: "OpenClaw", Snapshot: []SnapshotDecl{snapshot(cfg.OpenClawAgentsMDPath(), DriftPolicyManagedContentVeto), snapshot(cfg.OpenClawSoulMDPath(), DriftPolicyWholeFileVeto), snapshot(cfg.OpenClawMCPConfigPath(), DriftPolicyWholeFileVeto)}, InstallActions: []StepActionKind{StepActionSyncOpenClawWorkspace, StepActionSyncOpenClawPlugin, StepActionSyncOpenClawSkills}, UpdateActions: []StepActionKind{StepActionSyncOpenClawWorkspace, StepActionSyncOpenClawPlugin, StepActionSyncOpenClawSkills}, UninstallActions: []StepActionKind{StepActionStripOpenClawWorkspace, StepActionRemoveOpenClawPlugin, StepActionRemoveOpenClawSkills}, DoctorChecks: []DoctorCheckKind{DoctorCheckOpenClaw}},
		)
		capabilities = append(capabilities, "OpenClaw: workspace, skills y MCP mediante gentle-ai y su CLI")
	}
	if options.CloudResolvable && selection.Claude {
		// StepActionRemoveEngramCloudState is StepActionSyncEngramCloud's reversal. RemoveEngramCloudState
		// already existed but was reachable only from the zero-caller legacy installer.Uninstall, so
		// engram-cloud.json survived `click uninstall`. It stays offline: it deletes click's own local
		// enrollment record and never un-enrolls the shared cloud project other machines still use.
		//
		cloudStep := Step{ID: "engram-cloud", Target: PlanTargetShared, Label: "Engram Cloud", Snapshot: []SnapshotDecl{snapshot(cfg.EngramCloudStatePath(), DriftPolicyWholeFileVeto)}, InstallActions: []StepActionKind{StepActionConfigureEngramCloudSessionSync, StepActionSyncEngramCloud}, UpdateActions: []StepActionKind{StepActionConfigureEngramCloudSessionSync, StepActionSyncEngramCloud}, UninstallActions: []StepActionKind{StepActionRemoveEngramCloudSessionSync, StepActionRemoveEngramCloudState}, DoctorChecks: []DoctorCheckKind{DoctorCheckEngramCloud, DoctorCheckEngramCloudSessionSync}}
		steps = append(steps, cloudStep)
	}
	if selection.Claude {
		steps = append(steps,
			Step{ID: "plugins", Target: PlanTargetShared, Label: "plugins", Snapshot: []SnapshotDecl{snapshot(cfg.KnownMarketplacesPath(), DriftPolicyNonVeto), snapshot(cfg.InstalledPluginsPath(), DriftPolicyNonVeto)}},
			Step{ID: "memory-guard", Target: PlanTargetShared, Label: "memory guard", Snapshot: []SnapshotDecl{snapshot(cfg.SettingsPath(), DriftPolicyManagedContentVeto)}, InstallActions: []StepActionKind{StepActionWriteClaudeManagedBlock, StepActionRegisterMemoryGuard}, UpdateActions: []StepActionKind{StepActionWriteClaudeManagedBlock, StepActionRegisterMemoryGuard}, UninstallActions: []StepActionKind{StepActionStripClaudeManagedBlock, StepActionUnregisterMemoryGuard}, DoctorChecks: []DoctorCheckKind{DoctorCheckMemoryGuard}},
		)
	}
	steps = append(steps, Step{ID: "sdd-assets", Target: PlanTargetShared, Label: "SDD assets", Snapshot: []SnapshotDecl{snapshot(cfg.TargetSelectionPath(), DriftPolicyWholeFileVeto)}, UninstallActions: []StepActionKind{StepActionRemoveTargetSelection}})
	return TargetPlan{Selection: selection, Capabilities: capabilities, Steps: steps}
}
