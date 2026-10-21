# Click Dot distribution

Objective: installing Click AI Devkit from this repository installs Click Dot automatically; Dot lives in `dot/` and ships in the same release as `click`.
Branch: `feat/click-dot` (from `origin/main`). Delivery strategy: ask-on-risk. Test runners: `go test ./...` (strict TDD, D13) and `npm test` in `dot/`.

## Specs

- S1 — User: "lo que quiero es que instalando click ia dev kit desde el repositorio se instale solito el click dots". `click install` on Windows installs Click Dot without extra steps; `click update`, `click uninstall`, `click doctor` and `click rollback` manage it like other Click-owned content.
- S2 — User chose to publish Dot inside the devkit: answered "Dentro del devkit" to "¿Dónde publicamos Click Dot para que el devkit lo descargue al instalar?", whose option read: "Dot pasa a una carpeta del repo click-ai-devkit (también público) y el mismo release publica el binario de click y el paquete de Dot, siempre con versiones alineadas."
- S3 — Dot's own specs continue in `dot/odd/tasks/dot-desktop-agent.md` (S1–S6 there); T4 (call from Claude Code) and the remaining hardening stay tracked there.

Assumptions (confirm or correct):
- A1 — Windows only for Dot installation; on other platforms `click install` skips Dot with a Spanish notice.
- A2 — Dot installs per user under `%LOCALAPPDATA%\Programs\Click Dot` with a Start Menu shortcut, then starts; the app itself manages start-at-login.
- A3 — The Dot package is downloaded from the GitHub release whose tag matches the running `click` version and verified against that release's checksums before extraction; development builds without a release skip Dot with a notice.
- A4 — A failed Dot download or install warns in Spanish and does not fail `click install`/`update` (precedent D45 for supplementary integrations).
- A5 — Single source of truth for personal-data patterns: `dot/lib/guard-patterns.json` is generated from `internal/guard/patterns.yaml`, and both Go and Dot tests run the shared corpus `dot/test/fixtures/guard-corpus.json`.
- A6 — `click rollback` leaves the Dot installation as it is and says so; uninstall stops Dot, removes the install folder, the Start Menu shortcut and its start-at-login entry, and preserves Dot user data (`%APPDATA%\Click Dot`: credentials and history).

## Tasks

| ID | Specs | Task | Route | Status | Commit |
|----|-------|------|-------|--------|--------|
| D1 | S2 | Import Dot with history into `dot/` | inline | done | 7f2dfea |
| D2 | S2, A5 | In-repo pattern source: Dot sync/parity reads `internal/guard/patterns.yaml`; Go test runs the shared corpus; synthetic-PII annotations | inline | done | see L4 |
| D3 | S1, S2 | CI and release: Dot job in `ci.yml`; release builds the Windows Dot zip, adds it to `checksums.txt` and the GitHub release | delegated (parallel with D4) | done (Windows CI and real publish unexercised) | ce51a58 |
| D4 | S1, A1–A4 | Go installer: install/update/uninstall/doctor/rollback/snapshot of Dot, Spanish messages, isolated tests with a fake release server | delegated (parallel with D3) | done (real Windows unexercised) | 737ba75, 425fa2d |
| D5 | S1 | Dot release hardening: asar and Electron fuses, trim unused production deps, per-user writable install path | pending | pending | |
| D6 | S1 | Disposable Windows qualification: package smoke installs Dot from the local artifact | pending | pending | |

## Log

- L1 (2026-10-21) — Original request: "lo que quiero es que instalando click ia dev kit desde el repositorio se instale solito el click dots"
- L2 (2026-10-21) — Verified: `click-dot` had no remote; the devkit is public (`Angel-MercadoCLK/click-ai-devkit`) and installs through Scoop from its releases. User chose "Dentro del devkit".
- L3 (2026-10-21) — D1: `git subtree add --prefix=dot ../click-dot feat/dot-desktop` keeps Dot's history (reviewed boundary there: `d41d0f7`; `cd982d9` pending slice). Push, PR and release remain the user's decisions.
- L4 (2026-10-21) — D2: `internal/guard/dot_corpus_test.go` runs `dot/test/fixtures/guard-corpus.json` against the embedded patterns (RED with the corpus moved away, GREEN with it); Dot's drift test reads `../../internal/guard/patterns.yaml` by default (no skip): Dot 181/181, Go guard ok. `click scan-diff` went from 58 findings to 1 after `click:allow-secret` annotations; the remaining one is npm's deprecation text for `glob@7.2.3` inside `dot/package-lock.json` (JSON cannot carry the marker; candidate for removal in D5).
- L5 (2026-10-21) — D3↔D4 contract. Release asset `click-dot_<version>_windows_amd64.zip` (version = tag without `v`), listed in the same release's `checksums.txt` as `<sha256>  <name>`. Zip root holds the unpacked app directly, with `Click Dot.exe` at the root. Download base: `https://github.com/Angel-MercadoCLK/click-ai-devkit/releases/download/v<version>/` (injectable in tests).
- L6 (2026-10-21) — D3 (delegated Sonnet): Dot is built on the Windows release runner (`scripts/package-dot.mjs`, deterministic, Node-only zip) and handed to GoReleaser as an artifact; `checksum.extra_files` and `release.extra_files` glob `./dot/release/click-dot_{{ .Version }}_windows_amd64.zip`, so a missing zip aborts before publishing. `goreleaser check` (v2.18.3, scratch install) passed; snapshot release listed the zip in `checksums.txt`; local package 200,075,934 bytes, 11,776 files, `Click Dot.exe` at the root, identical sha256 on rebuild. Not run: Windows CI legs, real upload.
- L7 (2026-10-21) — D4 (delegated Sonnet): Dot sync after existing install/update steps on Windows x64; marker `.click-dot-version` doubles as ownership proof; restorable directory swap; shortcut via PowerShell 5.1 WScript.Shell; detached start; warnings never `[FAIL]` (A4); uninstall removes only Run/StartupApproved values that launch the installed exe (Electron 44 names the value from AppUserModelID, so name-based removal was rejected); doctor gains a non-failing `Warning`; rollback untouched. TDD: the CLI layer had true RED (11 failing tests); the installer and doctor cores were written before their tests — a strict-TDD deviation (D13) — compensated with guard mutations (checksum, size, marker, duplicates, already-current each made a test fail); `doctor.CheckDot` has no mutation evidence. Parent added the Spanish README rows. Checks: `go build`, `go vet`, `go test ./... -count=1`, Windows cross-build, `gofmt -l`, `git diff --check` clean; Dot 181/181. `click scan-diff`: 1 remaining (npm lockfile text).

