# Design: Presets de instalación (menu item)

## Technical Approach

Turn `internal/menu/menu.go:51` ("Presets de instalación", currently `Active: false`) into a real
menu action that launches a new preset-select TUI, then runs the SAME install pipeline
`click install --yes --profile <profile>` already runs — with zero installer.go changes. A
preset is a friendly Spanish label for one of the three existing built-in orchestration profiles
(`modelconfig.ProfileBalanced/CostSaver/Quality`); its only real effect is skipping the per-phase
model-editor screen (`ui.ModelSelectModel`) that `click install`'s interactive path always shows
after profile-select. Component selection (which plugins/services install) is explicitly OUT of
v1 scope — see Decision 2.

## Architecture Decisions

### Decision 1: New `internal/preset` package, not folded into `modelconfig` or `installer`

| Option | Tradeoff | Decision |
|---|---|---|
| Add preset list directly in `modelconfig/profiles.go` | Couples UI-facing labels/descriptions to the model-resolution package | Rejected |
| New `internal/preset` package: `Preset{Name, Label, Profile, Description}` + `BuiltinPresets()` + `Resolve(name)` | Mirrors `modelconfig/profiles.go`'s exact shape (fixed-order slice + resolve helper); gives v2 (real component bundles) a clean seam without touching `modelconfig` | **Chosen** |

### Decision 2: v1 presets vary ONLY the profile, not the component set

`installer.SyncMarketplacePlugins` always installs all of `managedPlugins` (click-sdd,
click-memory, click-review) plus Engram plus Context7 — there is no existing partial-install
path. Making "Minimal"/"Memory only" bundles real would require a component filter threaded
through `SyncMarketplacePlugins`, `SyncEngram`, `SyncContext7`, and `install.go` — out of scope
for this change. **v1 built-in presets = named shortcuts for balanced / cost-saver / quality,
all installing the full component set.** Flagged as an Open Question for v2.

### Decision 3: New `internal/ui/presetselect.go`, mirrors `profileselect.go` exactly

Same `Cursor/Selected/Confirmed/Cancelled` shape, same j/k/arrow/enter/esc/q keymap, same
`styleRenderer` usage. `PresetSelectModel.Selected` holds a `preset.Preset` (not just a name) so
the caller reads `.Profile` directly. No "custom" row — presets are a single-keystroke shortcut;
full customization still goes through `click install`'s existing two-screen flow.

### Decision 4: Extract `performInstall` from `install.go`, reuse from a new `install-preset` command

| Option | Tradeoff | Decision |
|---|---|---|
| Duplicate `runInstall`'s post-resolution steps (SyncMarketplacePlugins → SyncEngram → SyncContext7 → WriteManagedBlock → RegisterMemoryGuardHook → SaveModelsWithProfile) in a new command | Drift risk: any future install-step change must be updated in two places | Rejected |
| Extract `performInstall(cmd, out, r, cfg, profile, models) error` from `runInstall`; both `runInstall` and the new `install-preset` command call it | Single source of truth for "what an install actually does" | **Chosen** |

### Decision 5: New hidden cobra command `click install-preset`, not a special menu-only path

Matches the `configure-models` precedent (`internal/cli/configuremodels.go`): `Hidden: true`,
directly runnable, and gated by the same `isTerminalWriter` non-TTY guard (prints a fallback
message pointing at `click install --profile <name> --yes` and returns nil, never hangs).

## Data Flow

    menu.go (Active item) ──Enter──▶ ActionInstallPreset ──▶ dispatch(["install-preset"])
         │
         ▼
    cli.runInstallPreset ──TTY check──▶ ui.PresetSelectModel (bubbletea)
         │ Confirmed
         ▼
    preset.Preset{Profile} ──▶ modelconfig.ResolveProfile(Profile) ──▶ performInstall(...)
         │
         ▼
    installer.SyncMarketplacePlugins / SyncEngram / SyncContext7 / WriteManagedBlock /
    RegisterMemoryGuardHook / SaveModelsWithProfile   (identical to `click install`)

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/preset/preset.go` | Create | `Preset` type, `BuiltinPresets()`, `Resolve(name string) (Preset, bool)` |
| `internal/preset/preset_test.go` | Create | Table tests: fixed order, `Resolve` hit/miss |
| `internal/ui/presetselect.go` | Create | `PresetSelectModel`, `NewPresetSelectModel()`, keymap Update/View |
| `internal/ui/presetselect_test.go` | Create | Cursor wrap, enter-confirms, esc/q-cancels (mirrors `profileselect_test.go`) |
| `internal/cli/install.go` | Modify | Extract `performInstall(cmd, out, r, cfg, profile, models) error` from `runInstall`'s post-resolution body |
| `internal/cli/installpreset.go` | Create | `newInstallPresetCommand`, `runInstallPreset`, `resolveAndRunInstallPreset`, `runPresetSelectTUI` |
| `internal/cli/installpreset_test.go` | Create | Fake `presetSelector`; asserts cancel path, confirm path calls `performInstall` inputs correctly |
| `internal/cli/root.go` | Modify | Register `newInstallPresetCommand()` |
| `internal/menu/menu.go` | Modify | Flip item `Active: true`, add `ActionInstallPreset` const, `ActionArgs` case → `{"install-preset"}` |
| `internal/menu/menu_test.go` | Modify | Extend `ActionArgs` table with the new case |

## Interfaces / Contracts

```go
// internal/preset/preset.go
type Preset struct {
    Name        string                     // stable id: "full-stack" | "economico" | "calidad"
    Label       string                     // Spanish display label
    Profile     modelconfig.ProfileName
    Description string                     // Spanish one-line description
}
func BuiltinPresets() []Preset
func Resolve(name string) (Preset, bool)

// internal/cli/install.go
func performInstall(cmd *cobra.Command, out io.Writer, r *ui.Renderer, cfg installer.Config,
    profile modelconfig.ProfileName, models map[modelconfig.Phase]string) error
```

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit | `preset.BuiltinPresets`/`Resolve` | Table tests, fixed order assertion |
| Unit | `PresetSelectModel` Update/View | Headless `keyMsg()` helper, same pattern as `profileselect_test.go` |
| Unit | `resolveAndRunInstallPreset` | Fake `presetSelector` injected (no real bubbletea); assert `performInstall` receives the resolved profile/models; assert cancel path never calls it |
| Integration | `runInstall` still passes after extraction | Existing `install_test.go`/`commands_test.go` suite must stay green unchanged |

## Migration / Rollout

No migration required. Purely additive except the `performInstall` extraction, which is a
behavior-preserving refactor (existing `install` tests must pass unchanged).

## Open Questions

- [ ] v2: should presets ever gate which components install (true bundles), and if so does
      `SyncMarketplacePlugins` need a components filter parameter?
- [ ] Should `install-preset`'s non-TTY fallback message list all three preset profile names, or
      just point at `--profile`'s own `--help` text?
