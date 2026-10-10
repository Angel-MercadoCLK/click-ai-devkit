// Package installer owns the logic that installs and reverses click-ai-devkit's managed Claude Code
// plugins into a developer's Claude Code setup (Config.ClaudeHome), including safe retirement of
// the legacy Click SDD plugin.
//
// v0.2 foundation installs Click's active plugins through the
// native `claude plugin` marketplace flow while keeping the CLI thin: it patches the managed
// CLAUDE.md block and wires the memory-guard hook.
package installer

import (
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

// Install writes Click-owned Claude surfaces only: marketplace plugins, CLAUDE.md, and the
// memory-guard hook. The CLI provisions gentle-ai and its Engram/Context7 components separately;
// this compatibility composition never mutates shared MCP registrations.
//
// models is the per-phase click-sdd model selection (D25); pass nil to install with defaults
// (modelconfig.Resolve fills every phase). Install does not persist models to disk — that's
// internal/cli's job (installer.SaveModels), matching how RegisterMemoryGuardHook et al. are also
// orchestrated from the cli layer rather than bundled here.
func Install(cfg Config, models map[modelconfig.Phase]string) error {
	if err := SyncMarketplacePlugins(cfg, models); err != nil {
		return err
	}
	if err := WriteManagedBlock(cfg.ClaudeMDPath(), DefaultManagedContent); err != nil {
		return err
	}
	if err := RegisterMemoryGuardHook(cfg); err != nil {
		return err
	}
	return nil
}

// EngramCloudConfigured reports whether the local Engram client should enroll into a shared cloud
// project on this run: server URL, project name, and ENGRAM_CLOUD_TOKEN must all be present. It is
// exposed so the CLI preview plan can decide whether to list the cloud step before taking the
// run-start snapshot.
func EngramCloudConfigured(cfg Config, m *manifest.Manifest) bool {
	server, project, tokenPresent := resolveEngramCloudConfig(cfg, m)
	return server != "" && project != "" && tokenPresent
}

// EngramCloudPartiallyConfigured reports whether server and project are resolved but the cloud
// token is missing. This is the UI trigger for the Spanish "skipped because token is absent" report.
func EngramCloudPartiallyConfigured(cfg Config, m *manifest.Manifest) bool {
	server, project, tokenPresent := resolveEngramCloudConfig(cfg, m)
	return server != "" && project != "" && !tokenPresent
}

// Uninstall reverses Click-owned Claude surfaces only. It intentionally leaves Engram and Context7
// registrations and binaries untouched because the pinned gentle-ai component owns their lifecycle.
// The Click-local Engram Cloud enrollment record is still removed offline without unenrolling the
// shared project.
func Uninstall(cfg Config) error {
	if err := RemoveMarketplacePlugins(cfg); err != nil {
		return err
	}
	if err := StripManagedBlock(cfg.ClaudeMDPath()); err != nil {
		return err
	}
	if err := UnregisterMemoryGuardHook(cfg); err != nil {
		return err
	}
	// engram-cloud-wiring: reverse the local enrollment record SyncEngramCloud may have written.
	// This is offline and non-destructive — it removes only click's own bookkeeping file and never
	// un-enrolls the shared cloud project (see RemoveEngramCloudState).
	if err := RemoveEngramCloudState(cfg); err != nil {
		return err
	}
	return nil
}
