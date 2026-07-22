'use strict';

/**
 * hooks.test.js — RED-first coverage for hooks.js's threat-matrix requirements (design #1666,
 * tasks 3.1-3.5). Uses Node's BUILT-IN test runner (node:test/node:assert) deliberately — this is a
 * static, dependency-free plugin (OCG-4), so it ships with zero npm dependencies and zero
 * package.json; adding a test framework dependency here would contradict that decision. Run with:
 *
 *   node --test internal/installer/assets/openclaw/click-memory-guard/plugins/hooks.test.js
 *
 * EXECUTION STATUS (this apply session): written test-first against hooks.js's actual exports and
 * manually traced line-by-line against hooks.js's source, but NEVER ACTUALLY RUN — no Bash tool was
 * available this session (matching PR-A/PR-B's original apply sessions before their own follow-up
 * Bash-capable verification passes). Needs a `node --test` execution pass before this can be called
 * verified. Every test below only exercises hooks.js's exported pure/injectable-seam functions
 * (isEngramMemSaveTool, buildPreToolUsePayload, runClickMemoryGuard with a fake spawnFn) — NONE of
 * them spawn a real child process, matching this codebase's "small mocks/interfaces around system or
 * command execution boundaries" convention (go-testing skill, applied here to its JS counterpart).
 */

const test = require('node:test');
const assert = require('node:assert/strict');
const { EventEmitter } = require('node:events');

const hooks = require('./hooks.js');

// fakeChildProcess builds a minimal stand-in for node:child_process's ChildProcess: an EventEmitter
// with .stdout (another EventEmitter), .stdin (a recording writable-like object), and .kill(). Tests
// drive its behavior by calling the returned emit* helpers AFTER runClickMemoryGuard has attached its
// listeners (mirrors real spawn()'s async event timing).
function fakeChildProcess() {
  const proc = new EventEmitter();
  proc.stdout = new EventEmitter();
  proc.killed = false;
  proc.kill = () => {
    proc.killed = true;
  };
  const stdinWrites = [];
  proc.stdin = {
    write(data) {
      stdinWrites.push(data);
    },
    end() {
      stdinWrites.push('__END__');
    },
  };
  proc.__stdinWrites = stdinWrites;
  return proc;
}

// --- Task 3.4: tool-name bypass prevention ---

test('isEngramMemSaveTool: recognizes every documented candidate name', () => {
  for (const name of hooks.ENGRAM_TOOL_NAME_CANDIDATES) {
    assert.equal(hooks.isEngramMemSaveTool(name), true, `expected ${name} to be recognized`);
  }
});

test('isEngramMemSaveTool: a genuinely different tool is NOT recognized (no false-positive scanning)', () => {
  assert.equal(hooks.isEngramMemSaveTool('mcp__filesystem__read_file'), false);
  assert.equal(hooks.isEngramMemSaveTool(undefined), false);
  assert.equal(hooks.isEngramMemSaveTool(''), false);
});

test('beforeToolCall: a non-engram tool is allowed through WITHOUT spawning anything', async () => {
  let spawnCalled = false;
  const spawnFn = () => {
    spawnCalled = true;
    return fakeChildProcess();
  };
  const result = await hooks.beforeToolCall(
    { tool_name: 'mcp__filesystem__read_file', tool_input: { path: '/etc/passwd' } },
    { spawnFn }
  );
  assert.deepEqual(result, { block: false });
  assert.equal(spawnCalled, false, 'a non-engram tool must never trigger a spawn — it is not scanned at all');
});

// --- Task 3.5: payload/bytes parity with internal/cli/memoryguard.go's preToolUsePayload ---

test('buildPreToolUsePayload: field names/order/values match Go\'s preToolUsePayload struct exactly', () => {
  const event = {
    session_id: 'sess-123',
    cwd: '/home/dev/project',
    tool_name: 'mcp__engram__mem_save',
    tool_input: { title: 'note', content: 'AKIAABCDEFGHIJKLMNOP' },
  };
  const payload = hooks.buildPreToolUsePayload(event);
  // Go's json.Marshal on an equivalent preToolUsePayload{...} struct value marshals fields in
  // DECLARATION order (session_id, cwd, hook_event_name, tool_name, tool_input) — this object
  // literal is declared in that same order, and JSON.stringify preserves string-key insertion
  // order, so the two produce byte-identical JSON for equivalent values. This exact string is what
  // Go's encoding/json would produce for the equivalent Go struct value.
  const wantJSON =
    '{"session_id":"sess-123","cwd":"/home/dev/project","hook_event_name":"PreToolUse",' +
    '"tool_name":"mcp__plugin_engram_engram__mem_save",' +
    '"tool_input":{"title":"note","content":"AKIAABCDEFGHIJKLMNOP"}}';
  assert.equal(JSON.stringify(payload), wantJSON);
});

test('buildPreToolUsePayload: ALWAYS rewrites tool_name to the canonical Go matcher, regardless of which candidate matched', () => {
  for (const candidate of hooks.ENGRAM_TOOL_NAME_CANDIDATES) {
    const payload = hooks.buildPreToolUsePayload({ tool_name: candidate, tool_input: {} });
    assert.equal(payload.tool_name, hooks.CANONICAL_TOOL_NAME);
  }
});

// --- Task 3.1: subprocess spawn safety (stdin only, argv array, no shell) ---

test('runClickMemoryGuard: spawns with an argument array and shell:false, payload reaches the child ONLY via stdin', async () => {
  let capturedArgv;
  let capturedOpts;
  const proc = fakeChildProcess();
  const spawnFn = (cmd, args, opts) => {
    capturedArgv = [cmd, args];
    capturedOpts = opts;
    return proc;
  };

  const dangerousPayload = JSON.stringify({ tool_input: { content: '$(rm -rf /); `whoami`; secret && evil' } });
  const resultPromise = hooks.runClickMemoryGuard('/opt/click/bin/click', dangerousPayload, spawnFn);

  // Confirm the shell-metacharacter-laden payload was written to stdin, NEVER placed in argv.
  assert.deepEqual(capturedArgv, ['/opt/click/bin/click', ['memory-guard']]);
  assert.equal(capturedOpts.shell, false);
  assert.deepEqual(proc.__stdinWrites, [dangerousPayload, '__END__']);
  for (const argvPart of capturedArgv) {
    assert.equal(JSON.stringify(argvPart).indexOf('rm -rf'), -1, 'argv must never contain the payload content');
  }

  // Resolve the pending promise so the test doesn't leak an unresolved handler.
  proc.stdout.emit('data', Buffer.from('{"hookSpecificOutput":{"permissionDecision":"allow"}}'));
  proc.emit('close', 0);
  const result = await resultPromise;
  assert.deepEqual(result, { allow: true });
});

// --- Task 3.2: fail-closed on missing binary / spawn error ---

test('runClickMemoryGuard: a spawn "error" event (e.g. missing click binary) resolves allow:false', async () => {
  const proc = fakeChildProcess();
  const spawnFn = () => proc;
  const resultPromise = hooks.runClickMemoryGuard('/nonexistent/click', '{}', spawnFn);

  proc.emit('error', Object.assign(new Error('spawn /nonexistent/click ENOENT'), { code: 'ENOENT' }));

  const result = await resultPromise;
  assert.equal(result.allow, false);
  assert.match(result.reason, /spawn failed/);
});

test('beforeToolCall: missing binary end-to-end BLOCKS the engram tool call', async () => {
  const proc = fakeChildProcess();
  const spawnFn = () => proc;
  const resultPromise = hooks.beforeToolCall(
    { tool_name: 'mcp__engram__mem_save', tool_input: { content: 'irrelevant' } },
    { spawnFn }
  );
  proc.emit('error', new Error('ENOENT'));
  const result = await resultPromise;
  assert.equal(result.block, true);
});

// --- Task 3.3: fail-closed on non-zero exit ---

test('runClickMemoryGuard: a non-zero exit code resolves allow:false', async () => {
  const proc = fakeChildProcess();
  const spawnFn = () => proc;
  const resultPromise = hooks.runClickMemoryGuard('/opt/click/bin/click', '{}', spawnFn);

  proc.stdout.emit('data', Buffer.from('memory-guard panic'));
  proc.emit('close', 2);

  const result = await resultPromise;
  assert.equal(result.allow, false);
  assert.match(result.reason, /exited with code 2/);
});

test('runClickMemoryGuard: exit 0 with a "deny" decision resolves allow:false with the reason surfaced', async () => {
  const proc = fakeChildProcess();
  const spawnFn = () => proc;
  const resultPromise = hooks.runClickMemoryGuard('/opt/click/bin/click', '{}', spawnFn);

  const denyResponse = JSON.stringify({
    hookSpecificOutput: {
      hookEventName: 'PreToolUse',
      permissionDecision: 'deny',
      permissionDecisionReason: 'blocked: aws_secret_key pattern matched',
    },
  });
  proc.stdout.emit('data', Buffer.from(denyResponse));
  proc.emit('close', 0);

  const result = await resultPromise;
  assert.equal(result.allow, false);
  assert.equal(result.reason, 'blocked: aws_secret_key pattern matched');
});

test('runClickMemoryGuard: exit 0 with unparsable stdout resolves allow:false (fail-closed, never throws)', async () => {
  const proc = fakeChildProcess();
  const spawnFn = () => proc;
  const resultPromise = hooks.runClickMemoryGuard('/opt/click/bin/click', '{}', spawnFn);

  proc.stdout.emit('data', Buffer.from('not json at all'));
  proc.emit('close', 0);

  const result = await resultPromise;
  assert.equal(result.allow, false);
});

test('runClickMemoryGuard: exit 0 with allow decision resolves allow:true', async () => {
  const proc = fakeChildProcess();
  const spawnFn = () => proc;
  const resultPromise = hooks.runClickMemoryGuard('/opt/click/bin/click', '{}', spawnFn);

  proc.stdout.emit('data', Buffer.from('{"hookSpecificOutput":{"permissionDecision":"allow"}}'));
  proc.emit('close', 0);

  const result = await resultPromise;
  assert.deepEqual(result, { allow: true });
});

// --- CLICK_BIN templating sanity (paired with the Go-side tests in openclawplugin_test.go) ---

test('CLICK_BIN placeholder is present in the SOURCE module before install-time templating', () => {
  // This test file loads hooks.js DIRECTLY from the repo (untemplated) — SyncOpenClawPlugin
  // (Go side) is what replaces {{CLICK_BIN}} at install time, never this file itself. Asserts the
  // placeholder survives exactly as openclawplugin.go expects to find it.
  assert.equal(hooks.CLICK_BIN, '{{CLICK_BIN}}');
});
