# Spike B — PreToolUse Hook Contract (mem_save memory-guard)

> Status: spike result. Grounded in 00-decisions-and-open-questions.md (D1–D22).
**Doc version:** code.claude.com/docs/en/hooks.md, "Last updated: 2026-07-06" (per docs map)
**Sources:**
- https://code.claude.com/docs/en/hooks.md
- https://code.claude.com/docs/en/hooks-guide.md

## Verdict

**PreToolUse hooks CAN mutate the tool input.** They are not allow/deny-only. A hook returns `hookSpecificOutput.updatedInput` (an object) to replace the tool's arguments before it runs — a documented, first-class field, separate from `permissionDecision`. This means **D7 (redact) is technically supported**, not just D21 (block-only). D21 becomes a deliberate policy choice, not a platform-forced fallback.

## 1. Input contract — what a PreToolUse hook receives (stdin, command hooks)

```json
{
  "session_id": "abc123",
  "prompt_id": "550e8400-e29b-41d4-a716-446655440000",
  "transcript_path": "/home/user/.claude/projects/.../transcript.jsonl",
  "cwd": "/home/user/my-project",
  "permission_mode": "default",
  "hook_event_name": "PreToolUse",
  "tool_name": "mcp__engram__mem_save",
  "tool_input": { "...": "the mem_save arguments (title, content, scope, etc.)" }
}
```

Relevant for the guard: `tool_name` (confirm it's `mem_save`), `tool_input` (payload to scan/redact), `cwd`, `session_id` (audit logging), `permission_mode`.

## 2. Output contract — PreToolUse decision control

Return `hookSpecificOutput` with `hookEventName: "PreToolUse"`:

```json
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "allow",
    "permissionDecisionReason": "Command validated"
  }
}
```

`permissionDecision` values:

| Value | Behavior |
|---|---|
| `allow` | Approve the tool call, bypassing the permission flow |
| `deny` | Block the tool call and show the user the `permissionDecisionReason` |
| `ask` | Show the user a permission dialog, even in `auto` mode |
| `defer` | Skip this hook's decision and continue through the normal permission flow |

### Mutation field — `updatedInput`

> "You can also modify the tool's input before it runs by returning `updatedInput` directly under `hookSpecificOutput`"

> "The `updatedInput` object replaces the tool's input fields. For Bash, you can modify the `command` field. For other tools, replace the fields that tool accepts."

Combine allow + redact in one response — the exact shape the memory-guard would use:

```json
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "allow",
    "permissionDecisionReason": "Redacted PII/policy fields before persisting",
    "updatedInput": {
      "content": "<sanitized mem_save payload, PII/policy numbers/claims data stripped>"
    }
  }
}
```

Docs confirm the pattern across events:
> "A few events can also rewrite content rather than only allow or block it:
> - `PreToolUse`: `updatedInput` directly under `hookSpecificOutput` replaces a tool's arguments before it runs.
> - `PostToolUse`: `updatedToolOutput` replaces the tool's result.
> For redaction or transformation use cases, intercept at `PreToolUse` for outbound tool inputs and `PostToolUse` for inbound tool results."

**Universal fields available alongside `hookSpecificOutput`:** `continue`, `stopReason`, `suppressOutput`, `systemMessage`.

## 3. Exit codes (PreToolUse)

| Exit code | Behavior |
|---|---|
| `0` | Success. stdout parsed as JSON for decision fields. No JSON on stdout → normal permission flow. |
| `2` | Blocking error. stdout ignored; stderr fed back to Claude as the denial reason; **tool call blocked**. |
| any other (1, 3, ...) | Non-blocking error (**fail open**) — tool call proceeds. |

> "If your hook is meant to enforce a policy, use `exit 2`." (exit 1 is treated as non-blocking / fail-open.)

## 4. Matcher — targeting the Engram `mem_save` MCP tool only

MCP tool naming: `mcp__<server>__<tool>`. But a plugin-bundled MCP server is scoped:
> "Tools from a plugin-bundled MCP server use a scoped server segment that includes the plugin name: `mcp__plugin_<plugin-name>_<server-name>__<tool>`. A matcher written against the bare server key never fires for these tools."

Since Engram is installed as a plugin here, the real matcher is almost certainly:
```json
"matcher": "mcp__plugin_engram_engram__mem_save"
```
**Verify the exact scoped name at runtime** (via `/hooks` or a log-only hook first) before shipping — a wrong matcher fails **silently open**.

## 5. Fail-closed guidance

Fail-**open** cases (docs-confirmed): exit codes other than 2, exit 0 with no JSON, invalid JSON on stdout, HTTP hook non-2xx, and hook timeout — all let the call proceed.

To make the guard fail **closed**:
- On any internal error/exception, explicitly `exit 2` (never a bare crash / exit 1).
- Do not rely on "no JSON output" as a safe default — that path is fail-open.
- Timeout is fail-open per docs; if true fail-closed-on-timeout is required, the guard must guarantee its own fast exit-2 path (e.g., a watchdog wrapper).
- Always set `permissionDecisionReason` on deny.

## Impact on memory-guard design (D7 redact vs D21 block-only)

`updatedInput` is a real, supported PreToolUse field, explicitly recommended for redaction of outbound tool inputs. **D7 (field-level redaction before the sanitized `mem_save` proceeds) is implementable as designed.** D21 (block-only) is therefore a stricter *policy* choice, not a technical fallback.

Suggested middle ground: **redact-when-certain, block-when-uncertain, never allow-unredacted** — use `updatedInput` for deterministic known-safe redactions, `permissionDecision: "deny"` for anything the guard can't confidently redact.

Two risks to close before committing to D7 in the design:
1. Confirm the exact plugin-scoped tool name at runtime — wrong matcher = silent fail-open.
2. Docs don't specify whether `updatedInput` is validated against the MCP tool's input schema — verify empirically that a redacted payload round-trips cleanly through Engram's `mem_save`.
