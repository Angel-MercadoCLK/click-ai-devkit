import assert from "node:assert/strict";
import { test } from "node:test";

const tools = await import("../desktop/tool-runner.mts");
const claudeModule = await import("../desktop/tool-claude.mts");

const HOME = "C:\\Users\\angel";
const REPO = "C:\\work\\repo";
const EXE = "C:\\Users\\angel\\.local\\bin\\claude.exe";
const SHIM = "C:\\Users\\a b\\AppData\\Roaming\\npm\\claude.cmd";

// Fakes ---------------------------------------------------------------------------------------------------------

function controlled() {
  const listeners = {};
  const state = { killed: false, stdin: [], ended: false };
  const child = {
    pid: 888,
    stdin: { write: (chunk) => { state.stdin.push(String(chunk)); return true; }, end: () => { state.ended = true; }, on: () => {} },
    stdout: { on: (event, listener) => { listeners.stdout = listener; } },
    stderr: { on: (event, listener) => { listeners.stderr = listener; } },
    on: (event, listener) => { listeners[event] = listener; },
    kill: () => { state.killed = true; },
  };
  return {
    child,
    state,
    out: (text) => listeners.stdout?.(Buffer.isBuffer(text) ? text : Buffer.from(text)),
    err: (text) => listeners.stderr?.(Buffer.from(text)),
    close: (code) => listeners.close?.(code),
    fail: () => listeners.error?.(new Error("ENOENT")),
  };
}

function manualTimers() {
  const pending = new Map();
  let next = 1;
  return {
    setTimeout: (fn, ms) => { const id = next++; pending.set(id, { fn, ms }); return id; },
    clearTimeout: (id) => { pending.delete(id); },
    fire: (ms) => { for (const [id, timer] of [...pending]) if (timer.ms === ms) { pending.delete(id); timer.fn(); } },
    pending,
  };
}

const tick = () => new Promise((resolve) => setImmediate(resolve));
const line = (value) => `${JSON.stringify(value)}\n`;

function setup({ which = EXE, options = {} } = {}) {
  const procs = [];
  const calls = [];
  const timers = manualTimers();
  const runner = tools.createToolRunner({
    platform: "win32",
    env: { SystemRoot: "C:\\Windows", ComSpec: "C:\\Windows\\System32\\cmd.exe", KEEP: "1" },
    homedir: () => HOME,
    stat: async (path) => {
      if (path === REPO || path === HOME) return { isDirectory: () => true };
      throw Object.assign(new Error("ENOENT"), { code: "ENOENT" });
    },
    spawn: (executable, args, spawnOptions) => {
      const proc = controlled();
      procs.push(proc);
      calls.push({ executable, args, options: spawnOptions });
      return proc.child;
    },
    whichClaude: async () => which,
    timers,
    ...options,
  });
  const progress = [];
  const run = (overrides = {}) => runner.runClaudeCode({
    task: "Fix the failing test in foo.ts",
    cwd: REPO,
    signal: new AbortController().signal,
    onProgress: (text) => progress.push(text),
    ...overrides,
  });
  return { runner, run, procs, calls, timers, progress };
}

const INIT = { type: "system", subtype: "init", session_id: "sess-1", cwd: REPO, tools: ["Edit", "Bash"] };
const RESULT = { type: "result", subtype: "success", is_error: false, result: "Done: fixed foo.ts", session_id: "sess-1", total_cost_usd: 0.0123, permission_denials: [] };
const tool = (name, input) => ({ type: "assistant", message: { role: "assistant", content: [{ type: "tool_use", id: "t1", name, input }] } });

// Starting Claude Code --------------------------------------------------------------------------------------------

test("claude.exe runs directly with the exact S4 arguments, cwd and no shell", async () => {
  const { run, procs, calls } = setup();
  const pending = run();
  await tick();
  procs[0].out(line(RESULT));
  procs[0].close(0);
  await pending;
  assert.equal(calls[0].executable, EXE);
  assert.deepEqual(calls[0].args, ["-p", "--output-format", "stream-json", "--verbose", "--permission-mode", "acceptEdits"]);
  assert.equal(calls[0].options.cwd, REPO);
  assert.equal(calls[0].options.shell, false);
  assert.equal(calls[0].options.windowsHide, true);
  assert.equal(calls[0].options.env.KEEP, "1");
});

test("the task goes to stdin and stdin is closed; it is never in argv", async () => {
  const { run, procs, calls } = setup();
  const task = "Refactor \"x\" & delete %TEMP% ^ \n then `whoami` $(evil)";
  const pending = run({ task });
  await tick();
  procs[0].out(line(RESULT));
  procs[0].close(0);
  await pending;
  assert.equal(procs[0].state.stdin.join(""), task);
  assert.equal(procs[0].state.ended, true);
  assert.ok(!JSON.stringify(calls[0]).includes("whoami"), "nothing of the task is in the spawn call");
});

test("a .cmd shim runs through cmd.exe /d /s /c with constant arguments only", async () => {
  const { run, procs, calls } = setup({ which: SHIM });
  const task = "do things & more > file";
  const pending = run({ task });
  await tick();
  procs[0].out(line(RESULT));
  procs[0].close(0);
  await pending;
  assert.equal(calls[0].executable, "C:\\Windows\\System32\\cmd.exe");
  assert.deepEqual(calls[0].args, ["/d", "/s", "/c", `""${SHIM}" -p --output-format stream-json --verbose --permission-mode acceptEdits"`]);
  assert.equal(calls[0].options.windowsVerbatimArguments, true);
  assert.equal(calls[0].options.shell, false);
  assert.equal(procs[0].state.stdin.join(""), task);
  assert.ok(!JSON.stringify(calls[0]).includes("do things"));
});

test("a shim path that could break out of the quoting is refused", async () => {
  for (const path of ["C:\\x\\cl\"aude.cmd", "C:\\%TEMP%\\claude.cmd", "C:\\x\\claude.cmd\r\n& calc"]) {
    const { run, calls } = setup({ which: [path] });
    const result = await run();
    assert.equal(result.ok, false, path);
    assert.equal(calls.length, 0, path);
  }
});

test("claude.exe is preferred over a .cmd shim when where.exe lists both", async () => {
  const { run, procs, calls } = setup({ which: [SHIM, EXE, "C:\\x\\claude"] });
  const pending = run();
  await tick();
  procs[0].close(0);
  await pending;
  assert.equal(calls[0].executable, EXE);
});

test("a .bat shim is treated like a .cmd shim, and entries that are not executables are ignored", async () => {
  const { run, procs, calls } = setup({ which: ["C:\\x\\claude", "C:\\x\\claude.bat"] });
  const pending = run();
  await tick();
  procs[0].close(0);
  await pending;
  assert.equal(calls[0].executable, "C:\\Windows\\System32\\cmd.exe");
});

test("the default lookup runs where.exe claude and parses its output lines", async () => {
  const procs = [];
  const calls = [];
  const runner = tools.createToolRunner({
    platform: "win32",
    env: { SystemRoot: "C:\\Windows" },
    homedir: () => HOME,
    stat: async () => ({ isDirectory: () => true }),
    spawn: (executable, args, options) => {
      const proc = controlled();
      procs.push(proc);
      calls.push({ executable, args, options });
      if (executable.endsWith("where.exe")) queueMicrotask(() => { proc.out(`${SHIM}\r\n${EXE}\r\n`); proc.close(0); });
      return proc.child;
    },
    timers: manualTimers(),
  });
  const pending = runner.runClaudeCode({ task: "t", cwd: REPO, signal: new AbortController().signal, onProgress() {} });
  await tick();
  await tick();
  assert.equal(calls[0].executable, "C:\\Windows\\System32\\where.exe");
  assert.deepEqual(calls[0].args, ["claude"]);
  assert.equal(calls[1].executable, EXE);
  procs[1].close(0);
  await pending;
});

test("a missing claude returns the clear error and starts nothing", async () => {
  for (const which of [null, [], "", ["C:\\x\\claude"]]) {
    const { run, calls } = setup({ which });
    const result = await run();
    assert.deepEqual(result, { ok: false, result: "", sessionId: null, costUsd: null, permissionDenials: 0, truncated: false, timedOut: false, error: "Claude Code is not installed" });
    assert.equal(calls.length, 0);
  }
  const { run } = setup({ options: { whichClaude: async () => { throw new Error("where failed"); } } });
  assert.equal((await run()).error, "Claude Code is not installed");
});

test("a missing, relative or file project directory is refused before anything starts", async () => {
  for (const cwd of ["repo", "C:\\missing", "", undefined]) {
    const { run, calls } = setup();
    const result = await run({ cwd });
    assert.equal(result.ok, false, String(cwd));
    assert.match(result.error, /folder/i);
    assert.equal(calls.length, 0);
  }
});

test("an empty task is refused", async () => {
  for (const task of ["", "   ", undefined, 5]) {
    const { run, calls } = setup();
    const result = await run({ task });
    assert.equal(result.ok, false);
    assert.match(result.error, /task/i);
    assert.equal(calls.length, 0);
  }
});

test("a claude that cannot start is reported, not thrown", async () => {
  const { run, procs } = setup();
  const pending = run();
  await tick();
  procs[0].fail();
  const result = await pending;
  assert.equal(result.ok, false);
  assert.match(result.error, /could not be started/i);
});

// NDJSON parsing and progress -------------------------------------------------------------------------------------

test("tool_use blocks become progress lines with a short target", async () => {
  const { run, procs, progress } = setup();
  const pending = run();
  await tick();
  procs[0].out(line(INIT));
  procs[0].out(line(tool("Edit", { file_path: "C:\\work\\repo\\src\\foo.ts", old_string: "a", new_string: "b" })));
  procs[0].out(line(tool("Write", { file_path: "/home/a/bar.md", content: "x" })));
  const long = "npm test -- --runInBand --coverage --reporter=spec --bail --verbose --silent";
  procs[0].out(line(tool("Bash", { command: long })));
  procs[0].out(line(tool("Read", { file_path: "C:\\work\\repo\\README.md" })));
  procs[0].out(line(tool("Grep", { pattern: "TODO", path: "src" })));
  procs[0].out(line(tool("TodoWrite", { todos: [] })));
  procs[0].out(line(RESULT));
  procs[0].close(0);
  await pending;
  assert.deepEqual(progress, [
    "Claude Code: Edit foo.ts",
    "Claude Code: Write bar.md",
    `Claude Code: Bash ${long.slice(0, 60)}`,
    "Claude Code: Read README.md",
    "Claude Code: Grep TODO",
    "Claude Code: TodoWrite",
  ]);
  assert.equal(long.slice(0, 60).length, 60);
  assert.ok(long.length > 60);
});

test("several tool_use blocks in one message each report, and text blocks are ignored", async () => {
  const { run, procs, progress } = setup();
  const pending = run();
  await tick();
  procs[0].out(line({ type: "assistant", message: { content: [{ type: "text", text: "thinking" }, { type: "tool_use", name: "Read", input: { file_path: "a.ts" } }, { type: "tool_use", name: "Read", input: { file_path: "b.ts" } }] } }));
  procs[0].out(line(RESULT));
  procs[0].close(0);
  await pending;
  assert.deepEqual(progress, ["Claude Code: Read a.ts", "Claude Code: Read b.ts"]);
});

test("progress targets are single-line and free of control characters", async () => {
  const { run, procs, progress } = setup();
  const pending = run();
  await tick();
  procs[0].out(line(tool("Bash", { command: "echo one\n\techo \u0007two" })));
  procs[0].out(line(RESULT));
  procs[0].close(0);
  await pending;
  assert.deepEqual(progress, ["Claude Code: Bash echo one echo two"]);
});

test("lines split across chunks, multi-byte characters split across chunks, CRLF and blank lines parse", async () => {
  const { run, procs, progress } = setup();
  const pending = run();
  await tick();
  const bytes = Buffer.from(line(tool("Edit", { file_path: "C:\\work\\ñandú-✓.ts" })) + "\r\n\n" + line(RESULT));
  for (let index = 0; index < bytes.length; index += 7) procs[0].out(bytes.subarray(index, index + 7));
  procs[0].close(0);
  const result = await pending;
  assert.deepEqual(progress, ["Claude Code: Edit ñandú-✓.ts"]);
  assert.equal(result.ok, true);
});

test("a malformed or unknown line is skipped and never breaks the run", async () => {
  const { run, procs, progress } = setup();
  const pending = run();
  await tick();
  procs[0].out("not json\n");
  procs[0].out("{\"type\":\"assistant\"}\n");
  procs[0].out("[1,2]\n");
  procs[0].out(line({ type: "assistant", message: { content: "text only" } }));
  procs[0].out(line({ type: "assistant", message: { content: [{ type: "tool_use", name: 5, input: null }, null, "x"] } }));
  procs[0].out(line({ type: "stream_event", event: {} }));
  procs[0].out(line(RESULT));
  procs[0].close(0);
  const result = await pending;
  assert.equal(result.ok, true);
  assert.deepEqual(progress, ["Claude Code: tool"]);
});

test("a throwing onProgress does not break parsing", async () => {
  const { run, procs } = setup();
  const pending = run({ onProgress: () => { throw new Error("ui gone"); } });
  await tick();
  procs[0].out(line(tool("Edit", { file_path: "a.ts" })));
  procs[0].out(line(RESULT));
  procs[0].close(0);
  assert.equal((await pending).ok, true);
});

// Result mapping --------------------------------------------------------------------------------------------------

test("the final result line maps to the contract", async () => {
  const { run, procs } = setup();
  const pending = run();
  await tick();
  procs[0].out(line(INIT));
  procs[0].out(line({ ...RESULT, permission_denials: [{ tool_name: "Bash" }, { tool_name: "Edit" }] }));
  procs[0].close(0);
  assert.deepEqual(await pending, { ok: true, result: "Done: fixed foo.ts", sessionId: "sess-1", costUsd: 0.0123, permissionDenials: 2, truncated: false, timedOut: false });
});

test("the session id comes from init when the result line lacks it, and the result's wins when both exist", async () => {
  const { run, procs } = setup();
  const pending = run();
  await tick();
  procs[0].out(line(INIT));
  const { session_id: _omit, ...withoutSession } = RESULT;
  procs[0].out(line(withoutSession));
  procs[0].close(0);
  assert.equal((await pending).sessionId, "sess-1");
  const second = setup();
  const again = second.run();
  await tick();
  second.procs[0].out(line(INIT));
  second.procs[0].out(line({ ...RESULT, session_id: "sess-2" }));
  second.procs[0].close(0);
  assert.equal((await again).sessionId, "sess-2");
});

test("the result text is capped at 16 KiB and flagged truncated", async () => {
  const { run, procs } = setup();
  const pending = run();
  await tick();
  procs[0].out(line({ ...RESULT, result: "é".repeat(20 * 1024) }));
  procs[0].close(0);
  const result = await pending;
  assert.ok(Buffer.byteLength(result.result) <= 16 * 1024);
  assert.ok(Buffer.byteLength(result.result) >= 16 * 1024 - 1);
  assert.equal(result.truncated, true);
  assert.ok(!result.result.includes("\uFFFD"));
  assert.equal(result.ok, true);
});

test("an is_error result is not ok but carries its text", async () => {
  const { run, procs } = setup();
  const pending = run();
  await tick();
  procs[0].out(line({ ...RESULT, is_error: true, subtype: "success", result: "API Error: overloaded" }));
  procs[0].close(1);
  const result = await pending;
  assert.equal(result.ok, false);
  assert.equal(result.result, "API Error: overloaded");
  assert.ok(result.error);
});

test("an error subtype without result text is not ok", async () => {
  const { run, procs } = setup();
  const pending = run();
  await tick();
  procs[0].out(line({ type: "result", subtype: "error_max_turns", is_error: true, session_id: "s", total_cost_usd: 0.5, permission_denials: [] }));
  procs[0].close(1);
  const result = await pending;
  assert.equal(result.ok, false);
  assert.match(result.error, /error_max_turns/);
  assert.equal(result.costUsd, 0.5);
});

test("missing optional fields map to null and zero", async () => {
  const { run, procs } = setup();
  const pending = run();
  await tick();
  procs[0].out(line({ type: "result", result: "ok" }));
  procs[0].close(0);
  assert.deepEqual(await pending, { ok: true, result: "ok", sessionId: null, costUsd: null, permissionDenials: 0, truncated: false, timedOut: false });
});

test("an exit without a result line is a failure that quotes stderr briefly", async () => {
  const { run, procs } = setup();
  const pending = run();
  await tick();
  procs[0].out(line(INIT));
  procs[0].err("Error: not logged in. Run claude login.\n" + "x".repeat(2000));
  procs[0].close(1);
  const result = await pending;
  assert.equal(result.ok, false);
  assert.match(result.error, /code 1/);
  assert.match(result.error, /not logged in/);
  assert.ok(result.error.length < 500);
  assert.equal(result.sessionId, "sess-1");
});

test("a line is parsed even when the process closes without a trailing newline", async () => {
  const { run, procs } = setup();
  const pending = run();
  await tick();
  procs[0].out(JSON.stringify(RESULT));
  procs[0].close(0);
  assert.equal((await pending).ok, true);
});

// Timeout and abort -----------------------------------------------------------------------------------------------

test("the 15 minute timeout kills the tree and reports timedOut", async () => {
  const { run, procs, calls, timers } = setup();
  const pending = run();
  await tick();
  procs[0].out(line(INIT));
  assert.ok([...timers.pending.values()].some((timer) => timer.ms === 15 * 60_000));
  timers.fire(15 * 60_000);
  const result = await pending;
  assert.equal(result.ok, false);
  assert.equal(result.timedOut, true);
  assert.match(result.error, /timed out/i);
  assert.equal(result.sessionId, "sess-1");
  assert.deepEqual(calls[1].args, ["/PID", "888", "/T", "/F"]);
  assert.equal(procs[0].state.killed, true);
});

test("aborting kills the tree and reports a stop, not a timeout", async () => {
  const controller = new AbortController();
  const { run, procs, calls } = setup();
  const pending = run({ signal: controller.signal });
  await tick();
  controller.abort();
  const result = await pending;
  assert.equal(result.ok, false);
  assert.equal(result.timedOut, false);
  assert.match(result.error, /stopped/i);
  assert.deepEqual(calls[1].args, ["/PID", "888", "/T", "/F"]);
  assert.equal(procs[0].state.killed, true);
});

test("an already-aborted signal starts nothing", async () => {
  const controller = new AbortController();
  controller.abort();
  const { run, calls } = setup();
  const result = await run({ signal: controller.signal });
  assert.equal(result.ok, false);
  assert.match(result.error, /stopped/i);
  assert.equal(calls.length, 0);
});

test("no progress reaches the UI after the run was stopped", async () => {
  const controller = new AbortController();
  const { run, procs, progress } = setup();
  const pending = run({ signal: controller.signal });
  await tick();
  controller.abort();
  await pending;
  procs[0].out(line(tool("Edit", { file_path: "late.ts" })));
  assert.deepEqual(progress, []);
});

// Stream parser, directly -----------------------------------------------------------------------------------------

test("describeToolUse shortens targets", () => {
  const { describeToolUse } = claudeModule;
  assert.equal(describeToolUse("Edit", { file_path: "C:\\a\\b\\c.ts" }), "Claude Code: Edit c.ts");
  assert.equal(describeToolUse("Bash", { command: "ls" }), "Claude Code: Bash ls");
  assert.equal(describeToolUse("WebFetch", { url: "https://example.com/" + "a".repeat(100) }).length, "Claude Code: WebFetch ".length + 60);
  assert.equal(describeToolUse("Mystery", {}), "Claude Code: Mystery");
  assert.equal(describeToolUse("Evil\nName", {}), "Claude Code: EvilName");
});
