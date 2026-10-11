# Click v0.8.0 release

## Objective and status

Ship the PR #71 Gentle AI distribution and ownership integration to installed Click users as v0.8.0, with release metadata that never points to an asset before GoReleaser has published it. **R01–R03 are pending.** This task list is the recovery record for the release; it does not itself merge, tag, publish, or approve native review.

## Problem and rationale

The embedded manifest currently carries `click_version`, but the running CLI version already comes from the release tag through GoReleaser ldflags. Keeping a second Click version tied to `bucket/click.json` creates a mismatch during tag preparation: the manifest is patched before GoReleaser runs its Go test hook, while the public Scoop manifest must remain at the last published release until the new archive exists. The tag workflow also runs a snapshot Windows package smoke while expecting the stable version in the existing bucket. Decouple source dependency pins, binary release identity, and published Scoop artifact metadata without weakening their respective checks.

## Current authorization and boundaries

- The user authorizes merging PR #71 and publishing a new release to `Angel-MercadoCLK/click-ai-devkit`, using the work identity `github-trabajo` and `gh` identity `Angel-MercadoCLK`. The v0.8.0 target is orchestrator-selected, not a version specified by the user. This supersedes only the earlier time-scoped no-merge/no-release statement in `odd/tasks/gentle-ai-distribution.md`; all other safety constraints remain.
- PR #71 is the existing oversized integration tracker and has an accepted `size:exception`; this release task adds an estimated 150–250 authored changed lines. Do not game the estimate or split this small coherent release fix into another PR.
- Preserve backup branches and existing files unless an authorized task requires a change. Do not run installs or provisioning against any real user profile, use credentials for setup, change authentication, or touch production data.
- No live Gentle AI installation/enrollment or real Cloud-token use is part of release qualification. The previously successful disposable Windows smoke used offline external-CLI fixtures; report that limitation honestly.
- Keep existing nonblocking findings in `odd/tasks/gentle-ai-distribution.md` deferred. No unrelated product changes or new pin qualification.
- Native review is not approved by this task document. Preserve pending review base `c68d733` and its prior ~254-line candidate. Before a new commit boundary, assess the actual normalized candidate; if native review is due, follow the provider-issued lifecycle and consent. Do not treat prior PR CI or this task list as review approval.

## Implementation configuration

- **TDD:** strict, required by `AGENTS.md` / D13.
- **Test runner:** `go test ./... -count=1`.
- **Delivery:** existing PR #71 `feature-branch-chain` tracker; no new PR unless a genuine blocker changes scope. The existing size exception covers the integration PR only.
- **Expected new authored lines:** approximately 150–250 across R01; estimate only, not a cap.

## Tasks

### R01 — Decouple release metadata and repair release qualification

- [x] Add regression coverage first and observe RED. Remove the unused embedded `click_version` field and `Manifest.ClickVersion` contract; keep the tag-injected CLI version path, plugin-version metadata consistency, and `ENGRAM_VERSION`/manifest Engram pin consistency checks intact.
- [x] Remove tag-time patching of `click_version`; retain only required upstream pin preparation. Keep `bucket/click.json` as the last published artifact metadata until GoReleaser has created the new release assets. Do not pre-bump its version, URL, or checksum.
- [x] Update the release snapshot smoke to validate that the packaged CLI prints a version line (the CI contract), not to compare a snapshot version against the previous stable bucket version.
- [x] Keep Scoop metadata independently meaningful and internally valid: version, archive URL, and SHA-256 must have the expected format and remain consistent with one another. Do not require source dependency pins to equal Scoop artifact metadata.
- [x] Update README/release qualification assertions to document the independent sources: CLI version is tag-injected, the embedded manifest contains runtime pins, and Scoop metadata describes published assets.
- [ ] Run focused regression tests, then `go test ./... -count=1`, `go build ./...`, `go vet ./...`, and `git diff --check`. Obtain green hosted Windows+Ubuntu PR CI before R02. No live profile execution.
- **Route:** delegated direct writer; multi-file behavior/test/docs change. Keep implementation, regression tests, and user-facing release documentation in one work unit.
- **Rollback boundary:** the release metadata contract, tag workflow smoke expectation, associated tests, and corresponding README wording revert together; do not roll back the already-published release.
- **Commit:** `db07cc5` (247 authored lines). Luna high implementation, parent readback/focused rerun and independent Sol full tests/build/vet/diff checks passed. Native high review of `c68d733..db07cc5` completed with four empty finding sets; exact acknowledgement burned lineage `review-86e8a3c6afdb7ee4`, revision `sha256:f60f7116d7a7b3217e14b5b2d232cb33614940d139b850483bd7735fe5bfac6c`. New reviewed source boundary: `db07cc5`. Hosted exact-head CI remains pending.

### R02 — Merge the qualified integration PR

- [ ] Confirm PR #71 is the intended work-repository PR, its head contains the completed R01 commit, and all required Ubuntu and Windows checks are green on the exact head.
- [ ] Merge PR #71 to `main` under the explicit authorization above; record the merge commit and resulting main HEAD. Preserve the existing backup branches.
- [ ] Confirm main CI passes for the merged commit. Do not tag if the merge/head identity or required checks are uncertain or failing.
- **Route:** parent-owned GitHub state and merge operation; no parallel writer. Evidence is the exact PR head, check run identities/results, merge commit, and main CI result.
- **Rollback:** no source rollback is automatic; stop and report if merge state is ambiguous. Do not force-reset or rewrite remote history.

### R03 — Publish v0.8.0 and verify installed-user delivery

- [ ] Create/push tag `v0.8.0` only at the verified merged main commit, then monitor the tag workflow: disposable Windows release qualification must pass before the Ubuntu GoReleaser publication job proceeds.
- [ ] Verify the GitHub release is for `v0.8.0` and contains `checksums.txt` plus the five supported archives: Windows amd64, Darwin amd64/arm64, and Linux amd64/arm64. Validate every published archive against `checksums.txt`.
- [ ] After assets exist, verify the GoReleaser-published `bucket/click.json` has version `0.8.0`, its Windows amd64 URL resolves to the v0.8.0 archive, and its SHA-256 matches that archive. Never manually publish Scoop metadata that references absent assets.
- [ ] Verify the packaged CLI reports v0.8.0 using only a disposable artifact check; do not run `click install`, `click update`, or live provisioning against a real profile. Confirm the embedded upstream pin values remain the intended qualified values.
- [ ] Report installed-user instructions clearly: Scoop users run `scoop update; scoop update click` (Scoop needs Git to refresh the bucket); non-Scoop users download the release binary/archive manually. Then run `click update` separately to sync Click/Gentle AI components; it does not update the Click binary.
- [ ] Record the actual workflow/run IDs, release URL, asset names and checksum results, bucket metadata verification, skipped/live limitations, and final main/tag identities. Failed, missing, or ambiguous publication evidence leaves R03 pending; do not retry a tag or publish a replacement blindly.
- **Route:** parent owns tag/release operations; delegated artifact verification may inspect only published release assets and metadata, not provision any profile.
- **Rollback:** release publication is not automatically reversible. If the tag workflow or release is partial, preserve state and report exact evidence; do not delete/recreate tags or silently replace assets.

## Acceptance criteria

- R01 regression demonstrates the previous release-version coupling/snapshot expectation fails before the fix and passes after it; unrelated tests remain active and failures are not skipped.
- Go tests, build, vet, `git diff --check`, and hosted Windows/Ubuntu PR CI pass for the exact R01 candidate.
- Plugin metadata and Engram pin checks remain effective; no source pre-bump causes Scoop to reference absent assets.
- PR #71 is merged only at the verified green head; v0.8.0 tag points to that merged main commit.
- The published release assets, checksums, CLI version, and Scoop URL/hash are verified against one another. User update guidance distinguishes binary update from `click update` component synchronization.
- No live profile provisioning, real Cloud-token use, or unsupported claim about upstream integration is made.

## Progress and next step

- R01: implementation complete; no commit created (parent owns commit/review boundary). Strict TDD evidence: `go test ./internal/manifest -run 'Test(ScoopMetadataIsIndependentOfRuntimePins|ValidateScoopMetadata)$' -count=1` failed before the fix with `manifest.yaml still contains click_version`; after the fix, `go test ./internal/manifest -count=1` passed. `go test ./... -count=1`, `go build ./...`, `go vet ./...`, and `git diff --check` all passed. The disposable hosted Windows package smoke is reported as passed with offline external-CLI fixtures; no live upstream provisioning was performed. Green hosted Ubuntu+Windows CI for this exact candidate remains pending before R02.
- R02: pending; wait for R01 and exact green PR CI.
- R03: pending; wait for R02 and the successful tag workflow.

**Next:** parent mirrors this updated task document to Engram and reads both copies back, then owns the review/commit boundary and exact-candidate hosted CI gate.
