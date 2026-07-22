# Engram Setup

## Purpose

click-ai-devkit bundles Engram as the persistent-memory backend for Claude Code sessions.

## Packaging model

Engram is treated as:

- a Go MCP binary
- plus a Claude Code plugin/runtime integration layer

For reproducibility, click-ai-devkit does **not** rely on a floating marketplace install.

## Pinning rule

`click` pins the Engram **binary version** per click-ai-devkit release.

That pin is applied by:

- downloading the release binary, or
- installing a tagged version with `go install ...@tag`

The exact version is controlled by click-ai-devkit release metadata.

## Claude Code wiring

`click` writes its own MCP entry with an **absolute binary path**.

This makes the installed machine deterministic:

- every developer gets the same Engram version for the same click-ai-devkit release
- `click update` can move the pin in a controlled way
- no hidden dependency on `latest`

## Operational expectation

- install should be reproducible
- update should be idempotent
- doctor should verify the expected wiring
- uninstall should remove Click-managed configuration cleanly

## Engram Cloud (opt-in)

By default Engram stays local-only. A developer can opt into a shared Engram Cloud project so the same memory is available across machines.

### Quick path

1. Ask the server operator for the Engram Cloud server URL, project name, and a personal token.
2. Export the token in your shell (never commit it):

   ```bash
   export ENGRAM_CLOUD_TOKEN="<your-token>"
   ```

3. Make cloud config available to `click` — either in the release manifest (`engram_cloud.server` / `engram_cloud.project`) or via environment overrides:

   ```bash
   export CLICK_ENGRAM_CLOUD_SERVER="https://engram.example.com"
   export CLICK_ENGRAM_CLOUD_PROJECT="click-ai-devkit"
   ```

4. Run `click install` or `click update`. On first enrollment Click runs:
   - `engram cloud config --server <server>`
   - `engram cloud enroll <project>`
   - `engram cloud upgrade doctor`
   - `engram cloud upgrade repair`
   - `engram cloud upgrade bootstrap`
   - `engram sync --cloud --project <project>`
5. Repeat runs are idempotent: they re-apply config and sync only.

### Details

| Topic | Decision |
|---|---|
| Opt-in | Cloud enrollment runs only when server, project, and `ENGRAM_CLOUD_TOKEN` are all present. Missing any value keeps Engram local-only. |
| Token handling | `ENGRAM_CLOUD_TOKEN` is read from the environment only. Click never writes it to disk, to logs, or to any committed file. |
| Config source | `engram_cloud.server` / `engram_cloud.project` in the release manifest are defaults; `CLICK_ENGRAM_CLOUD_SERVER` / `CLICK_ENGRAM_CLOUD_PROJECT` override them. |
| First-time migration | Existing local observations are pushed to the shared project by the upgrade sequence on the first enrollment. |
| Repeat runs | After the first enrollment, `click install`/`update` runs only `engram cloud config` and `engram sync --cloud --project <project>`. |

### Out of scope

- Standing up the Engram Cloud server, its docker-compose, or its ops are the server operator's manual tasks.
- Issuing tokens via `engram cloud bootstrap admin` is also manual and external to `click`.

### Checklist

- [ ] `ENGRAM_CLOUD_TOKEN` is set in the shell, never in a tracked file.
- [ ] `CLICK_ENGRAM_CLOUD_SERVER` and `CLICK_ENGRAM_CLOUD_PROJECT` (or manifest values) are set.
- [ ] `click doctor` reports the fourth Engram check after enrollment.
