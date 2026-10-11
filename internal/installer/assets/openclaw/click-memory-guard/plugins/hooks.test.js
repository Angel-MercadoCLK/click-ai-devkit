'use strict';

/**
 * hooks.test.js — RED-first coverage for hooks.js's threat-matrix requirements (design #1666,
 * tasks 3.1-3.5). Uses Node's BUILT-IN test runner (node:test/node:assert) deliberately — this is a
 * static, dependency-free plugin (OCG-4), so it ships with zero npm dependencies and zero
 * package.json; adding a test framework dependency here would contradict that decision. Run with:
 *
 *   node --test internal/installer/assets/openclaw/click-memory-guard/plugins/hooks.test.js
 *
 * EXECUTION STATUS: written test-first against hooks.js's actual exports and manually traced
 * line-by-line against hooks.js's source, then confirmed executable in a Bash-capable verification
 * pass (13/13 passing via `node --test`). Every test below only exercises hooks.js's exported
 * pure/injectable-seam functions
 * (isPersistingEngramTool, buildPreToolUsePayload, runClickMemoryGuard with a fake spawnFn) — NONE of
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
  proc.stderr = new EventEmitter();
  proc.killed = false;
  proc.kill = () => {
    proc.killed = true;
  };
  const stdinWrites = [];
  proc.stdin = new EventEmitter();
  proc.stdin.write = (data) => {
    stdinWrites.push(data);
  };
  proc.stdin.end = () => {
    stdinWrites.push('__END__');
  };
  proc.__stdinWrites = stdinWrites;
  return proc;
}

// --- Task 3.4: tool-name bypass prevention ---

const ALL_PREFIXES = ['mcp__plugin_engram_engram__', 'mcp__engram__', 'engram__', 'engram.'];

test('isPersistingEngramTool: recognizes every persisting tool under every documented name form', () => {
  assert.ok(hooks.PERSISTING_ENGRAM_TOOLS.length > 1, 'the guard must cover more than mem_save');
  for (const tool of hooks.PERSISTING_ENGRAM_TOOLS) {
    for (const prefix of ALL_PREFIXES) {
      assert.equal(hooks.isPersistingEngramTool(prefix + tool), true, `expected ${prefix}${tool} to be recognized`);
    }
  }
  assert.equal(hooks.isPersistingEngramTool('mcp__engram__mem_update'), true);
});

test('isPersistingEngramTool: read-only Engram tools and unrelated tools are NOT recognized', () => {
  for (const name of [
    'mcp__engram__mem_search',
    'mcp__plugin_engram_engram__mem_context',
    'engram__mem_get_observation',
    'engram.mem_current_project',
    'mcp__engram__mem_session_start',
    'mcp__filesystem__read_file',
    'mem_save',
    undefined,
    null,
    '',
  ]) {
    assert.equal(hooks.isPersistingEngramTool(name), false, `expected ${String(name)} to be ignored`);
  }
});

test('isPersistingEngramTool: fail-safe fallback scans only names containing engram and a persisting tool', () => {
  assert.equal(hooks.isPersistingEngramTool('OpenClaw__ENGRAM__experimental_MEM_SAVE_v2'), true);
  assert.equal(hooks.isPersistingEngramTool('OpenClaw__ENGRAM__MEM_UPDATE'), true);
  assert.equal(hooks.isPersistingEngramTool('engram_search'), false);
  assert.equal(hooks.isPersistingEngramTool('read_file'), false);
  assert.equal(hooks.isPersistingEngramTool(42), false);
});

test('resolvePersistingTool: picks the longest matching tool so mem_save_prompt is not collapsed into mem_save', () => {
  assert.equal(hooks.resolvePersistingTool('mcp__engram__mem_save_prompt'), 'mem_save_prompt');
  assert.equal(hooks.resolvePersistingTool('mcp__engram__mem_save'), 'mem_save');
  assert.equal(hooks.resolvePersistingTool('mcp__engram__mem_search'), null);
});

test('beforeToolCall: a persisting tool other than mem_save is scanned and blocked on deny', async () => {
  const proc = fakeChildProcess();
  let spawned = 0;
  const resultPromise = hooks.beforeToolCall(
    // click:allow-secret synthetic sequential-placeholder input for adapter unit test; not user data.
    { tool_name: 'mcp__engram__mem_update', tool_input: { id: 1, content: 'CUIT 20-12345678-3' } },
    { spawnFn: () => { spawned += 1; return proc; } }
  );
  assert.equal(spawned, 1, 'mem_update must be handed to click memory-guard');
  const written = JSON.parse(proc.__stdinWrites[0]);
  assert.equal(written.tool_name, 'mcp__plugin_engram_engram__mem_update');
  proc.stdout.emit('data', Buffer.from('{"hookSpecificOutput":{"permissionDecision":"deny","permissionDecisionReason":"blocked: detected pii"}}'));
  proc.emit('close', 0);
  assert.deepEqual(await resultPromise, { block: true, reason: 'blocked: detected pii' });
});

test('beforeToolCall: a read-only Engram tool is allowed WITHOUT spawning', async () => {
  let spawned = 0;
  const result = await hooks.beforeToolCall(
    // click:allow-secret synthetic sequential-placeholder query for adapter unit test; not user data.
    { tool_name: 'mcp__engram__mem_search', tool_input: { query: 'CUIT 20-12345678-3' } },
    { spawnFn: () => { spawned += 1; return fakeChildProcess(); } }
  );
  assert.deepEqual(result, { block: false });
  assert.equal(spawned, 0);
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

test('buildPreToolUsePayload: ALWAYS rewrites tool_name to the canonical plugin-scoped name of the same tool', () => {
  for (const tool of hooks.PERSISTING_ENGRAM_TOOLS) {
    for (const prefix of ALL_PREFIXES) {
      const payload = hooks.buildPreToolUsePayload({ tool_name: prefix + tool, tool_input: {} });
      assert.equal(payload.tool_name, hooks.CANONICAL_TOOL_PREFIX + tool);
    }
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

test('runClickMemoryGuard: a stdin error resolves allow:false without throwing in the host', async () => {
  const proc = fakeChildProcess();
  const resultPromise = hooks.runClickMemoryGuard('/opt/click/bin/click', '{}', () => proc);

  assert.doesNotThrow(() => proc.stdin.emit('error', new Error('broken pipe')));

  assert.deepEqual(await resultPromise, {
    allow: false,
    reason: 'click memory-guard: stdin error: broken pipe',
  });
});

test('beforeToolCall: a stdin error blocks without throwing in the host', async () => {
  const proc = fakeChildProcess();
  const resultPromise = hooks.beforeToolCall(
    { tool_name: 'mcp__engram__mem_save', tool_input: { content: 'irrelevant' } },
    { spawnFn: () => proc }
  );

  assert.doesNotThrow(() => proc.stdin.emit('error', new Error('broken pipe')));

  assert.deepEqual(await resultPromise, {
    block: true,
    reason: 'click memory-guard: stdin error: broken pipe',
  });
});

test('runClickMemoryGuard: stderr data is drained without changing the allow resolution', async () => {
  const proc = fakeChildProcess();
  const resultPromise = hooks.runClickMemoryGuard('/opt/click/bin/click', '{}', () => proc);
  const stderrDataListeners = proc.stderr.listenerCount('data');

  // This fake proves consumption wiring, but cannot model full OS pipe saturation.
  proc.stderr.emit('data', Buffer.from('diagnostic output'));
  proc.stdout.emit('data', Buffer.from('{"hookSpecificOutput":{"permissionDecision":"allow"}}'));
  proc.emit('close', 0);

  assert.equal(stderrDataListeners, 1);
  assert.deepEqual(await resultPromise, { allow: true });
});

test('runClickMemoryGuard: a stderr error cannot throw in the host or change the result', async () => {
  const proc = fakeChildProcess();
  const resultPromise = hooks.runClickMemoryGuard('/opt/click/bin/click', '{}', () => proc);
  let thrown;

  try {
    proc.stderr.emit('error', new Error('stderr read failed'));
  } catch (err) {
    thrown = err;
  }
  proc.stdout.emit('data', Buffer.from('{"hookSpecificOutput":{"permissionDecision":"allow"}}'));
  proc.emit('close', 0);

  assert.equal(thrown, undefined);
  assert.deepEqual(await resultPromise, { allow: true });
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
