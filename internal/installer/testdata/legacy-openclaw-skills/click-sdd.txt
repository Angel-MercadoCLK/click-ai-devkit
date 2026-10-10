---
name: click-sdd
description: Start and route the portable Click SDD workflow in OpenClaw for a new code change.
---

# Click SDD for OpenClaw

Use this skill as the entry point for a new change. This is an OpenClaw-native workflow built from
skills and workspace instructions. It does not use external agent delegation, plugin registries,
or runtime-specific model configuration. Click may also write a portable profile recommendation to
`<OpenClawHome>/click-ai-devkit/model-profile.json`; treat it as reference data for choosing models,
not as an active OpenClaw model/provider configuration.

## Flow

Run the phases in this order:

`click-sdd-explore` -> `click-sdd-propose` -> `click-sdd-spec` and `click-sdd-design` ->
`click-sdd-tasks` -> `click-sdd-apply` -> `click-sdd-verify` -> `click-sdd-archive`

Use `click-sdd-onboard` instead when the developer wants to learn the workflow rather than start a
real change. Pause between phases unless the developer explicitly asks for an automatic run.

## Portable contract

- Keep artifacts in the repository's agreed SDD location or the available Engram memory store.
- Write technical artifacts in English.
- Read the previous phase's artifact before producing the next one.
- Do not invent agent, plugin, registry, model, or tool metadata for OpenClaw.
- If a phase cannot run in the current OpenClaw session, explain the limitation and stop at that
  phase instead of pretending to delegate it.
