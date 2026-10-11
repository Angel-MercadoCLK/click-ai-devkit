'use strict';

/**
 * click-memory-guard — OpenClaw before_tool_call adapter (design #1666, decisions OCG-1..6).
 *
 * This file is entirely click-owned (installed wholesale by
 * internal/installer/openclawplugin.go's SyncOpenClawPlugin, no managed-block markers) and is
 * re-written on every `click install`/`click update` run — do not hand-edit it, changes are
 * discarded on the next sync.
 *
 * Anti-corruption adapter (OCG-2/OCG-3): this file NEVER reimplements internal/guard's scanning
 * logic. It only (1) recognizes an OpenClaw call to any Engram tool that persists content (mem_save, mem_update, ...), (2) rewrites it into the
 * exact `preToolUsePayload` JSON shape internal/cli/memoryguard.go's runMemoryGuard already expects,
 * (3) spawns the existing, already-tested `click memory-guard` binary with that JSON on stdin, and
 * (4) maps its exit-code/stdout back to OpenClaw's block/allow semantics. Fail-closed throughout
 * (threat-matrix row "Fail-open on spawn error"): any non-allow signal — missing binary, non-zero
 * exit, spawn failure, invalid stdout, or timeout — BLOCKS the tool call, identical to Claude Code's
 * own PreToolUse exit-2 contract.
 */

const { spawn } = require('node:child_process');

// PERSISTING_ENGRAM_TOOLS is this adapter's copy of internal/guard's persisting-tool set
// (guard.PersistingEngramTools in tools.go — the single source of truth). hooks.js cannot import Go,
// so internal/guard/tools_openclaw_test.go fails the build if the two lists ever drift. Read-only
// Engram tools (mem_search, mem_context, mem_get_observation, ...) are deliberately absent.
const PERSISTING_ENGRAM_TOOLS = [
  'mem_capture_passive',
  'mem_compare',
  'mem_judge',
  'mem_save',
  'mem_save_prompt',
  'mem_session_end',
  'mem_session_summary',
  'mem_update',
];

// ENGRAM_TOOL_PREFIXES — VERIFY-AT-APPLY (design #1666, "Verify-at-apply-time" item 1): the exact
// tool name OpenClaw's before_tool_call hook sees for the Engram MCP server's tools is UNCONFIRMED
// (no live OpenClaw runtime was available, and OpenClaw enforcement has NOT been verified at
// runtime). `mcp__engram__<tool>` is a DOCUMENTED BEST GUESS assuming OpenClaw follows the common
// `mcp__<server>__<tool>` MCP naming convention; Claude Code's plugin-wrapped
// `mcp__plugin_engram_engram__<tool>` is accepted too, along with the `engram__<tool>` and
// `engram.<tool>` variants from the original design. NOTE: the premise that OpenClaw registers
// Engram as a raw top-level "mcpServers" entry was WRONG — real `validate config` evidence proved
// OpenClaw's schema has no such key, and internal/installer/openclaw.go's SyncOpenClawMCPConfig no
// longer writes it (cleanup-only now). The prefix list is a best guess until that mechanism is
// confirmed; resolvePersistingTool also has a conservative fallback for names that still identify
// both Engram and a persisting tool.
const ENGRAM_TOOL_PREFIXES = [
  'mcp__plugin_engram_engram__',
  'mcp__engram__',
  'engram__',
  'engram.',
];

// CANONICAL_TOOL_PREFIX is the plugin-scoped prefix Claude Code uses
// (internal/installer/hooksettings.go's MemoryGuardToolMatcher accepts it). Rewriting every matched
// OpenClaw call to CANONICAL_TOOL_PREFIX + <the same tool> (OCG-3) means click memory-guard's Go
// side never has to learn OpenClaw's own naming — the Go-side tool check stays a harmless safety
// net, never the actual gate.
const CANONICAL_TOOL_PREFIX = 'mcp__plugin_engram_engram__';

// CLICK_BIN is templated at install/update time by openclawplugin.go's SyncOpenClawPlugin: the
// absolute path from os.Executable() (OCG-5), with backslashes doubled and single quotes escaped so
// a Windows path (e.g. C:\Users\...\click.exe) remains a VALID JavaScript single-quoted string
// literal — an unescaped backslash before a non-special character (e.g. \U) is silently dropped by
// JS string parsing, which would corrupt the path. Falls back to the bare command name "click" when
// os.Executable() itself fails to resolve (rare, e.g. some stripped/unusual build environments) —
// OpenClaw's node runtime must then resolve "click" from its own PATH.
const CLICK_BIN = '{{CLICK_BIN}}';

// SPAWN_TIMEOUT_MS bounds how long this adapter waits for `click memory-guard` before treating the
// call as failed (fail-closed) — protects against a hung child process silently blocking every
// persisting Engram call forever, while still failing CLOSED (never open) when the timeout fires.
const SPAWN_TIMEOUT_MS = 10000;

// resolvePersistingTool returns the bare persisting Engram tool name (e.g. 'mem_update') that
// toolName refers to, or null when the call must not be scanned. An exact `<prefix><tool>` match
// wins; otherwise a conservative fallback applies: a lowercase name containing "engram" AND a
// persisting tool name resolves to the LONGEST such tool (so mem_save_prompt is never collapsed into
// mem_save). The fallback fails safe toward scanning novel Engram naming variants while requiring
// both markers to avoid scanning unrelated tools.
function resolvePersistingTool(toolName) {
  if (typeof toolName !== 'string') return null;
  for (const prefix of ENGRAM_TOOL_PREFIXES) {
    if (toolName.indexOf(prefix) === 0) {
      const bare = toolName.slice(prefix.length);
      if (PERSISTING_ENGRAM_TOOLS.indexOf(bare) !== -1) return bare;
    }
  }
  const lowerToolName = toolName.toLowerCase();
  if (lowerToolName.indexOf('engram') === -1) return null;
  let best = null;
  for (const tool of PERSISTING_ENGRAM_TOOLS) {
    if (lowerToolName.indexOf(tool) !== -1 && (best === null || tool.length > best.length)) {
      best = tool;
    }
  }
  return best;
}

// isPersistingEngramTool is the ONLY predicate that decides whether a call gets scanned at all
// (threat-matrix row "Tool-name bypass").
function isPersistingEngramTool(toolName) {
  return resolvePersistingTool(toolName) !== null;
}

// buildPreToolUsePayload maps an OpenClaw before_tool_call event to the EXACT JSON shape
// internal/cli/memoryguard.go's preToolUsePayload struct expects — same field names, same field
// order (session_id, cwd, hook_event_name, tool_name, tool_input) — so JSON.stringify here and
// encoding/json's json.Marshal on an equivalent Go struct value produce IDENTICAL bytes for
// equivalent field values (Go struct fields marshal in declaration order; this object literal is
// declared in that same order, and JSON.stringify preserves string-key insertion order). That
// byte-for-byte parity is threat-matrix row "Payload/bytes mismatch"'s whole point: the same secret
// string scanned through Claude Code's own PreToolUse hook and through this adapter must be scanned
// as the same bytes.
//
// VERIFY-AT-APPLY (design item 3): assumes event.tool_input carries the MCP tool's argument object
// (title/content/type/...) exactly as OpenClaw received it from the calling agent, mirroring how
// Claude Code's own PreToolUse hook receives tool_input.
function buildPreToolUsePayload(event) {
  const tool = resolvePersistingTool(event && event.tool_name);
  return {
    session_id: (event && event.session_id) || '',
    cwd: (event && event.cwd) || '',
    hook_event_name: 'PreToolUse',
    // Unreachable fallback (callers only build payloads for recognized tools): keep the raw name so
    // the Go side, which only scans persisting tools, never silently treats it as a scanned tool.
    tool_name: tool === null ? (event && event.tool_name) : CANONICAL_TOOL_PREFIX + tool,
    tool_input: event ? event.tool_input : undefined,
  };
}

// runClickMemoryGuard spawns `<clickBin> memory-guard` and resolves to { allow, reason }. It NEVER
// throws — every failure mode resolves allow:false instead, so a caller that forgets to wrap this in
// try/catch can never accidentally fail OPEN.
//
// Threat-matrix row "Subprocess spawn" (arg/shell injection via tool_input): payloadJSON reaches the
// child EXCLUSIVELY via its stdin stream (child.stdin.write below) — it is NEVER placed in the argv
// array and NEVER interpolated into a shell command string. doSpawn is called with a fixed argument
// ARRAY (`[clickBin, ['memory-guard']]`) and `{ shell: false }` explicitly, so even a tool_input
// containing shell metacharacters (`$(...)`, `` ` ``, `;`, `|`, ...) is only ever treated as
// arbitrary DATA inside a JSON string value on stdin — it can never reach a shell for interpretation.
//
// spawnFn is an injectable seam (mirrors this codebase's Go-side commandRunnerFactory/
// binaryLookupFactory pattern) defaulting to node:child_process's spawn, letting tests supply a fake
// child process without spawning anything real.
function runClickMemoryGuard(clickBin, payloadJSON, spawnFn) {
  const doSpawn = spawnFn || spawn;
  return new Promise((resolve) => {
    let settled = false;
    const finish = (result) => {
      if (settled) return;
      settled = true;
      resolve(result);
    };

    let child;
    try {
      child = doSpawn(clickBin, ['memory-guard'], { shell: false });
    } catch (spawnErr) {
      // Threat-matrix row "Fail-open on spawn error": a spawn() call that throws synchronously
      // (e.g. some platforms surface a missing binary this way instead of an async 'error' event)
      // still resolves to a BLOCK, never a silent allow.
      finish({ allow: false, reason: 'click memory-guard: spawn threw: ' + spawnErr.message });
      return;
    }

    const timer = setTimeout(() => {
      try {
        child.kill();
      } catch (_killErr) {
        // best-effort — the process may already be gone.
      }
      finish({ allow: false, reason: 'click memory-guard: timed out after ' + SPAWN_TIMEOUT_MS + 'ms' });
    }, SPAWN_TIMEOUT_MS);

    // 'error' fires for the async spawn-failure case (ENOENT for a missing binary, EACCES, ...) —
    // threat-matrix row "Fail-open on spawn error"'s primary case: missing `click` binary.
    child.on('error', (err) => {
      clearTimeout(timer);
      finish({ allow: false, reason: 'click memory-guard: spawn failed: ' + err.message });
    });

    let stdout = '';
    if (child.stdout) {
      child.stdout.on('data', (chunk) => {
        stdout += chunk.toString('utf8');
      });
    }

    if (child.stderr && typeof child.stderr.on === 'function') {
      child.stderr.on('data', () => {});
      child.stderr.on('error', () => {});
    }

    child.on('close', (code) => {
      clearTimeout(timer);
      if (code !== 0) {
        // Threat-matrix row "Fail-open on spawn error": any non-zero exit denies, mirroring
        // runMemoryGuard's own fail-closed exit-2 contract for every one of its internal failure
        // branches (read failure, invalid payload, scan failure, audit failure, write failure).
        finish({ allow: false, reason: 'click memory-guard: exited with code ' + code });
        return;
      }
      let parsed;
      try {
        parsed = JSON.parse(stdout);
      } catch (parseErr) {
        finish({ allow: false, reason: 'click memory-guard: invalid stdout JSON' });
        return;
      }
      const hookOutput = parsed && parsed.hookSpecificOutput;
      const decision = hookOutput && hookOutput.permissionDecision;
      if (decision === 'allow') {
        finish({ allow: true });
        return;
      }
      const reason = (hookOutput && hookOutput.permissionDecisionReason) || 'blocked by click memory-guard';
      finish({ allow: false, reason: reason });
    });

    if (child.stdin && typeof child.stdin.on === 'function') {
      child.stdin.on('error', (err) => {
        clearTimeout(timer);
        finish({ allow: false, reason: 'click memory-guard: stdin error: ' + err.message });
      });
    }

    try {
      if (child.stdin) {
        child.stdin.write(payloadJSON);
        child.stdin.end();
      } else {
        clearTimeout(timer);
        finish({ allow: false, reason: 'click memory-guard: spawned process has no stdin' });
      }
    } catch (writeErr) {
      clearTimeout(timer);
      finish({ allow: false, reason: 'click memory-guard: stdin write failed: ' + writeErr.message });
    }
  });
}

// beforeToolCall is this plugin's OpenClaw Plugin SDK hook entry point.
//
// VERIFY-AT-APPLY (design item 2): the exact export name/signature the OpenClaw Plugin SDK's
// createHookRunner()/before_tool_call registration resolves is UNCONFIRMED. This function is
// exported under several plausible bindings below (beforeToolCall, before_tool_call, and as the
// module's default export) so plugin.json's manifest can bind whichever key OpenClaw actually
// expects; correcting this later only means adjusting plugin.json's own hook-registration field,
// never this function's logic.
//
// deps is an OPTIONAL second parameter (absent in real OpenClaw invocations) used only by this
// plugin's own test suite to inject a fake spawnFn — see runClickMemoryGuard's doc comment.
async function beforeToolCall(event, deps) {
  const spawnFn = deps && deps.spawnFn;
  const toolName = event && event.tool_name;

  // Threat-matrix row "Tool-name bypass": a tool that is NOT a persisting Engram call is allowed
  // through untouched — this adapter must never scan unrelated tool calls. A persisting Engram
  // tool, by contrast, is ALWAYS scanned below; there is no code path that lets it through
  // unchecked.
  if (!isPersistingEngramTool(toolName)) {
    return { block: false };
  }

  const payload = buildPreToolUsePayload(event || {});
  let payloadJSON;
  try {
    payloadJSON = JSON.stringify(payload);
  } catch (marshalErr) {
    // Fail-closed even when the payload itself cannot be serialized — a persisting engram call must
    // never slip through just because its tool_input is unusual (e.g. contains a circular
    // reference or a BigInt).
    return { block: true, reason: 'click memory-guard: unable to build scan payload' };
  }

  const result = await runClickMemoryGuard(CLICK_BIN, payloadJSON, spawnFn);
  if (result.allow) {
    return { block: false };
  }
  return { block: true, reason: result.reason };
}

module.exports = {
  beforeToolCall: beforeToolCall,
  before_tool_call: beforeToolCall,
  default: beforeToolCall,
  PERSISTING_ENGRAM_TOOLS: PERSISTING_ENGRAM_TOOLS,
  ENGRAM_TOOL_PREFIXES: ENGRAM_TOOL_PREFIXES,
  CANONICAL_TOOL_PREFIX: CANONICAL_TOOL_PREFIX,
  CLICK_BIN: CLICK_BIN,
  resolvePersistingTool: resolvePersistingTool,
  isPersistingEngramTool: isPersistingEngramTool,
  buildPreToolUsePayload: buildPreToolUsePayload,
  runClickMemoryGuard: runClickMemoryGuard,
};
