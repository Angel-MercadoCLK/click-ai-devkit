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
 * logic. It only (1) recognizes an OpenClaw call to Engram's mem_save tool, (2) rewrites it into the
 * exact `preToolUsePayload` JSON shape internal/cli/memoryguard.go's runMemoryGuard already expects,
 * (3) spawns the existing, already-tested `click memory-guard` binary with that JSON on stdin, and
 * (4) maps its exit-code/stdout back to OpenClaw's block/allow semantics. Fail-closed throughout
 * (threat-matrix row "Fail-open on spawn error"): any non-allow signal — missing binary, non-zero
 * exit, spawn failure, invalid stdout, or timeout — BLOCKS the tool call, identical to Claude Code's
 * own PreToolUse exit-2 contract.
 */

const { spawn } = require('node:child_process');

// ENGRAM_TOOL_NAME_CANDIDATES — VERIFY-AT-APPLY (design #1666, "Verify-at-apply-time" item 1): the
// exact tool name OpenClaw's before_tool_call hook sees for the Engram MCP server's mem_save tool is
// UNCONFIRMED (no live OpenClaw runtime was available to this apply session). The primary candidate,
// `mcp__engram__mem_save`, is a DOCUMENTED BEST GUESS assuming OpenClaw follows the common
// `mcp__<server>__<tool>` MCP tool-naming convention — NOT Claude Code's plugin-wrapped
// `mcp__plugin_<plugin>_<server>__<tool>` scheme, because OpenClaw registers Engram as a raw
// mcpServers entry keyed "engram" (see internal/installer/openclaw.go's SyncOpenClawMCPConfig,
// which writes servers["engram"] directly — there is no plugin-marketplace wrapping layer on the
// OpenClaw side the way there is for Claude Code's engram@engram plugin). The other two candidates
// from the design are also checked, so a small naming variance doesn't silently disable scanning.
//
// The explicit list documents known variants; isEngramMemSaveTool also has a conservative fallback
// for names that still identify both Engram and mem_save.
const ENGRAM_TOOL_NAME_CANDIDATES = [
  'mcp__engram__mem_save',
  'engram__mem_save',
  'engram.mem_save',
];

// CANONICAL_TOOL_NAME is the exact string internal/installer/hooksettings.go's
// MemoryGuardToolMatcher expects — Claude Code's own plugin-scoped tool name. Rewriting every
// matched OpenClaw call to this value (OCG-3) means click memory-guard's Go matcher never has to
// learn OpenClaw's own naming — the redundant Go-side check in evaluateMemoryGuard stays a harmless
// safety net, never the actual gate.
const CANONICAL_TOOL_NAME = 'mcp__plugin_engram_engram__mem_save';

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
// mem_save call forever, while still failing CLOSED (never open) when the timeout fires.
const SPAWN_TIMEOUT_MS = 10000;

// isEngramMemSaveTool reports whether toolName matches an explicit candidate or its lowercase form
// contains both "engram" and "mem_save". This is the ONLY function that decides whether a call gets
// scanned at all (threat-matrix row "Tool-name bypass"). The fallback fails safe toward scanning
// novel Engram mem_save naming variants while requiring both markers to avoid scanning unrelated
// tools.
function isEngramMemSaveTool(toolName) {
  if (typeof toolName !== 'string') return false;
  if (ENGRAM_TOOL_NAME_CANDIDATES.indexOf(toolName) !== -1) return true;
  const lowerToolName = toolName.toLowerCase();
  return lowerToolName.indexOf('engram') !== -1 && lowerToolName.indexOf('mem_save') !== -1;
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
  return {
    session_id: (event && event.session_id) || '',
    cwd: (event && event.cwd) || '',
    hook_event_name: 'PreToolUse',
    tool_name: CANONICAL_TOOL_NAME,
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

  // Threat-matrix row "Tool-name bypass": a tool that is NOT the engram mem_save call is allowed
  // through untouched — this adapter must never scan unrelated tool calls. The engram tool itself,
  // by contrast, is ALWAYS scanned below; there is no code path that lets it through unchecked.
  if (!isEngramMemSaveTool(toolName)) {
    return { block: false };
  }

  const payload = buildPreToolUsePayload(event || {});
  let payloadJSON;
  try {
    payloadJSON = JSON.stringify(payload);
  } catch (marshalErr) {
    // Fail-closed even when the payload itself cannot be serialized — an engram mem_save call must
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
  ENGRAM_TOOL_NAME_CANDIDATES: ENGRAM_TOOL_NAME_CANDIDATES,
  CANONICAL_TOOL_NAME: CANONICAL_TOOL_NAME,
  CLICK_BIN: CLICK_BIN,
  isEngramMemSaveTool: isEngramMemSaveTool,
  buildPreToolUsePayload: buildPreToolUsePayload,
  runClickMemoryGuard: runClickMemoryGuard,
};
