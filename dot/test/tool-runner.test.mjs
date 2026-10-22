import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { test } from "node:test";

const tools = await import("../desktop/tool-runner.mts");

const KEY = Symbol.for("click-dot.tool-runner");
const HOME = "C:\\Users\\angel";

// Fakes ---------------------------------------------------------------------------------------------------------

function controlled() {
  const listeners = {};
  const state = { killed: false, stdin: [], ended: false };
  const child = {
    pid: 777,
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

function setup(extra = {}) {
  const procs = [];
  const calls = [];
  const timers = manualTimers();
  const directories = new Set(["C:\\Users\\angel", "C:\\work\\repo", "\\\\server\\share\\dir"]);
  const files = new Set(["C:\\work\\file.txt"]);
  const runner = tools.createToolRunner({
    platform: "win32",
    env: { SystemRoot: "C:\\Windows", PATH: "C:\\Windows", KEEP: "1" },
    homedir: () => HOME,
    stat: async (path) => {
      if (directories.has(path)) return { isDirectory: () => true };
      if (files.has(path)) return { isDirectory: () => false };
      throw Object.assign(new Error("ENOENT"), { code: "ENOENT" });
    },
    spawn: (executable, args, options) => {
      const proc = controlled();
      procs.push(proc);
      calls.push({ executable, args, options });
      extra.onSpawn?.(proc, executable, args);
      return proc.child;
    },
    timers,
    ...extra.options,
  });
  return { runner, procs, calls, timers };
}

const scriptOf = (call) => Buffer.from(call.args[call.args.indexOf("-EncodedCommand") + 1], "base64").toString("utf16le");
const request = (overrides = {}) => ({ command: "Get-Date", cwd: null, signal: new AbortController().signal, ...overrides });

// runPowerShell: process shape ------------------------------------------------------------------------------------

test("runPowerShell runs powershell.exe with the exact S3 flags and a UTF-16LE base64 command", async () => {
  const { runner, procs, calls } = setup();
  const pending = runner.runPowerShell(request({ command: "Write-Output 'héllo ✓'" }));
  await tick();
  procs[0].out("ok");
  procs[0].close(0);
  await pending;
  const [{ executable, args, options }] = calls;
  assert.equal(executable, "C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe");
  assert.deepEqual(args.slice(0, -1), ["-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand"]);
  assert.match(args.at(-1), /^[A-Za-z0-9+/]+=*$/);
  assert.match(scriptOf(calls[0]), /Write-Output 'héllo ✓'/);
  assert.equal(options.windowsHide, true);
  assert.equal(options.shell, false);
  assert.equal(options.cwd, HOME);
});

test("the script sets UTF-8 output and ends with the real exit code", async () => {
  const { runner, procs, calls } = setup();
  const pending = runner.runPowerShell(request({ command: "git status" }));
  await tick();
  procs[0].close(0);
  await pending;
  const script = scriptOf(calls[0]);
  assert.match(script, /\[Console\]::OutputEncoding\s*=\s*New-Object System\.Text\.UTF8Encoding\(\$false\)/);
  assert.match(script, /\$OutputEncoding/);
  assert.match(script, /exit \$LASTEXITCODE/);
  assert.ok(script.indexOf("git status") > script.indexOf("UTF8Encoding"), "the command comes after the encoding setup");
  assert.ok(script.indexOf("git status") < script.indexOf("exit $LASTEXITCODE"), "the exit handling comes after the command");
});

test("the command is never an argument and stdin is closed so nothing waits for input", async () => {
  const { runner, procs, calls } = setup();
  const pending = runner.runPowerShell(request({ command: "SECRET-MARKER" }));
  await tick();
  procs[0].close(0);
  await pending;
  assert.ok(!calls[0].args.some((arg) => arg.includes("SECRET-MARKER")), "only the encoded form is in argv");
  assert.equal(procs[0].state.ended, true);
});

test("the environment adds GIT_PAGER=cat and GIT_TERMINAL_PROMPT=0 and keeps the rest", async () => {
  const { runner, procs, calls } = setup();
  const pending = runner.runPowerShell(request());
  await tick();
  procs[0].close(0);
  await pending;
  assert.equal(calls[0].options.env.GIT_PAGER, "cat");
  assert.equal(calls[0].options.env.GIT_TERMINAL_PROMPT, "0");
  assert.equal(calls[0].options.env.KEEP, "1");
});

// runPowerShell: results ----------------------------------------------------------------------------------------

test("runPowerShell returns the exit code and both streams", async () => {
  const { runner, procs } = setup();
  const pending = runner.runPowerShell(request());
  await tick();
  procs[0].out("line one\n");
  procs[0].out("línea dos\n");
  procs[0].err("warning");
  procs[0].close(3);
  assert.deepEqual(await pending, { exitCode: 3, stdout: "line one\nlínea dos\n", stderr: "warning", truncated: false, timedOut: false });
});

test("a non-zero exit code is a result, never a throw", async () => {
  const { runner, procs } = setup();
  const pending = runner.runPowerShell(request());
  await tick();
  procs[0].close(1);
  assert.equal((await pending).exitCode, 1);
});

test("a PowerShell that cannot start is reported, not thrown", async () => {
  const { runner, procs } = setup();
  const pending = runner.runPowerShell(request());
  await tick();
  procs[0].fail();
  const result = await pending;
  assert.equal(result.exitCode, null);
  assert.match(result.stderr, /could not be started/i);
  const throwing = tools.createToolRunner({ platform: "win32", env: {}, homedir: () => HOME, stat: async () => ({ isDirectory: () => true }), spawn: () => { throw new Error("EACCES"); } });
  assert.match((await throwing.runPowerShell(request())).stderr, /could not be started/i);
});

test("output beyond 64 KiB per stream is cut and flagged truncated, and the process keeps being drained", async () => {
  const { runner, procs } = setup();
  const pending = runner.runPowerShell(request());
  await tick();
  procs[0].out("a".repeat(40 * 1024));
  procs[0].out("b".repeat(40 * 1024));
  procs[0].err("e".repeat(70 * 1024));
  procs[0].out("c".repeat(10));
  procs[0].close(0);
  const result = await pending;
  assert.equal(result.stdout.length, 64 * 1024);
  assert.equal(result.stderr.length, 64 * 1024);
  assert.equal(result.truncated, true);
  assert.equal(procs[0].state.killed, false, "truncation does not kill the process");
});

test("a truncated stream keeps its end, where a failing command prints its error", async () => {
  const { runner, procs } = setup();
  const pending = runner.runPowerShell(request());
  await tick();
  procs[0].out("a".repeat(40 * 1024));
  procs[0].out("b".repeat(40 * 1024));
  procs[0].out("ERROR at the end");
  procs[0].close(1);
  const result = await pending;
  assert.ok(result.stdout.endsWith("bERROR at the end"), "the last bytes are kept");
  assert.ok(!result.stdout.startsWith("a".repeat(30 * 1024)), "the first bytes are the ones dropped");
  assert.equal(result.truncated, true);
});

test("the cap never leaves half of a multi-byte character", async () => {
  const { runner, procs } = setup({ options: { limits: { streamBytes: 10 } } });
  const pending = runner.runPowerShell(request());
  await tick();
  procs[0].out(Buffer.from("✓✓aaaaaaaa")); // two 3-byte characters, then 8 bytes; the cut lands inside the second.
  procs[0].close(0);
  const result = await pending;
  assert.equal(result.stdout, "aaaaaaaa");
  assert.equal(result.truncated, true);
});

test("a stream within the cap is not truncated", async () => {
  const { runner, procs } = setup();
  const pending = runner.runPowerShell(request());
  await tick();
  procs[0].out("x".repeat(64 * 1024));
  procs[0].close(0);
  const result = await pending;
  assert.equal(result.stdout.length, 64 * 1024);
  assert.equal(result.truncated, false);
});

// runPowerShell: cwd ----------------------------------------------------------------------------------------------

test("cwd null runs in the home directory", async () => {
  const { runner, procs, calls } = setup();
  const pending = runner.runPowerShell(request({ cwd: null }));
  await tick();
  procs[0].close(0);
  await pending;
  assert.equal(calls[0].options.cwd, HOME);
});

test("an existing absolute directory is used as the working directory, UNC shares included", async () => {
  for (const cwd of ["C:\\work\\repo", "\\\\server\\share\\dir"]) {
    const { runner, procs, calls } = setup();
    const pending = runner.runPowerShell(request({ cwd }));
    await tick();
    procs[0].close(0);
    await pending;
    assert.equal(calls[0].options.cwd, cwd);
  }
});

test("a relative, missing, file, rooted-only or empty cwd is refused without starting a process", async () => {
  for (const cwd of ["repo", ".\\repo", "..\\x", "C:repo", "\\work", "C:\\missing", "C:\\work\\file.txt", "", "C:\\work\\repo\u0000"]) {
    const { runner, calls } = setup();
    const result = await runner.runPowerShell(request({ cwd }));
    assert.equal(result.exitCode, null, cwd);
    assert.match(result.stderr, /folder/i, cwd);
    assert.equal(calls.length, 0, cwd);
  }
});

test("an empty, oversized or NUL-bearing command is refused without starting a process", async () => {
  for (const command of ["", "  \n", "x".repeat(8_001), "ls\u0000", 5, undefined]) {
    const { runner, calls } = setup();
    const result = await runner.runPowerShell(request({ command }));
    assert.equal(result.exitCode, null);
    assert.ok(result.stderr.length > 0);
    assert.equal(calls.length, 0);
  }
  const { runner, procs } = setup();
  const pending = runner.runPowerShell(request({ command: "x".repeat(8_000) }));
  await tick();
  procs[0].close(0);
  assert.equal((await pending).exitCode, 0);
});

// runPowerShell: timeout and abort ------------------------------------------------------------------------------

test("the 120 s timeout kills the whole process tree with taskkill /PID /T /F and reports timedOut", async () => {
  const { runner, procs, calls, timers } = setup();
  const pending = runner.runPowerShell(request());
  await tick();
  procs[0].out("partial");
  assert.ok([...timers.pending.values()].some((timer) => timer.ms === 120_000));
  timers.fire(120_000);
  const result = await pending;
  assert.equal(result.timedOut, true);
  assert.equal(result.exitCode, null);
  assert.equal(result.stdout, "partial");
  assert.equal(calls.length, 2);
  assert.equal(calls[1].executable, "C:\\Windows\\System32\\taskkill.exe");
  assert.deepEqual(calls[1].args, ["/PID", "777", "/T", "/F"]);
  assert.equal(procs[0].state.killed, true);
});

test("an injected killTree replaces taskkill", async () => {
  const killed = [];
  const { runner, procs, calls, timers } = setup({ options: { killTree: (pid) => killed.push(pid) } });
  const pending = runner.runPowerShell(request());
  await tick();
  timers.fire(120_000);
  await pending;
  assert.deepEqual(killed, [777]);
  assert.equal(calls.length, 1);
  assert.equal(procs[0].state.killed, true);
});

test("a command that finishes in time clears its timeout", async () => {
  const { runner, procs, timers } = setup();
  const pending = runner.runPowerShell(request());
  await tick();
  procs[0].close(0);
  await pending;
  assert.equal(timers.pending.size, 0);
});

test("aborting the signal kills the tree and returns what was read, without timedOut", async () => {
  const controller = new AbortController();
  const { runner, procs, calls } = setup();
  const pending = runner.runPowerShell(request({ signal: controller.signal }));
  await tick();
  procs[0].out("so far");
  controller.abort();
  const result = await pending;
  assert.equal(result.timedOut, false);
  assert.equal(result.exitCode, null);
  assert.equal(result.stdout, "so far");
  assert.deepEqual(calls[1].args, ["/PID", "777", "/T", "/F"]);
  assert.equal(procs[0].state.killed, true);
});

test("a signal that is already aborted starts nothing", async () => {
  const controller = new AbortController();
  controller.abort();
  const { runner, calls } = setup();
  const result = await runner.runPowerShell(request({ signal: controller.signal }));
  assert.equal(result.exitCode, null);
  assert.equal(calls.length, 0);
});

test("a late close after the timeout or abort is ignored", async () => {
  const { runner, procs, timers } = setup();
  const pending = runner.runPowerShell(request());
  await tick();
  timers.fire(120_000);
  const result = await pending;
  procs[0].out("late");
  procs[0].close(0);
  assert.equal(result.timedOut, true);
  assert.equal(result.stdout, "");
});

// Platform and wiring ---------------------------------------------------------------------------------------------

test("off Windows the runner reports unavailable and never spawns", async () => {
  const calls = [];
  const runner = tools.createToolRunner({ platform: "linux", spawn: () => { calls.push(1); throw new Error("no"); } });
  const ps = await runner.runPowerShell(request());
  assert.equal(ps.exitCode, null);
  assert.match(ps.stderr, /Windows/);
  assert.equal((await runner.classifyPowerShell("ls")).readOnly, false);
  const claude = await runner.runClaudeCode({ task: "x", cwd: "/tmp", signal: new AbortController().signal, onProgress() {} });
  assert.equal(claude.ok, false);
  assert.match(claude.error, /Windows/);
  assert.equal(calls.length, 0);
});

test("classifyPowerShell on the runner goes through the helper and fails closed", async () => {
  const { runner, procs, calls } = setup();
  const pending = runner.classifyPowerShell("git status");
  await tick();
  procs[0].out(JSON.stringify({ ok: true, statements: [[{ c: Buffer.from("git").toString("base64"), a: [Buffer.from("status").toString("base64")] }]] }));
  procs[0].close(0);
  assert.equal((await pending).readOnly, true);
  assert.ok(calls[0].args.includes("-InputFormat"));
  const second = runner.classifyPowerShell("git status");
  await tick();
  procs[1].close(1);
  assert.equal((await second).readOnly, false);
});

test("installToolRunner publishes the runner under Symbol.for(\"click-dot.tool-runner\")", () => {
  const target = {};
  const runner = tools.createToolRunner({ platform: "win32" });
  tools.installToolRunner(target, runner);
  assert.equal(target[KEY], runner);
  for (const method of ["classifyPowerShell", "runPowerShell", "runClaudeCode"]) assert.equal(typeof runner[method], "function");
  assert.equal(tools.TOOL_RUNNER_KEY, "click-dot.tool-runner");
});

test("installToolRunner refuses something that is not a runner", () => {
  assert.throws(() => tools.installToolRunner({}, {}), /tool runner/i);
  assert.throws(() => tools.installToolRunner({}, null), /tool runner/i);
});

test("main installs the runner only on Windows, before the server starts", async () => {
  const source = await readFile(new URL("../desktop/main.mts", import.meta.url), "utf8");
  const install = source.indexOf("installToolRunner(globalThis");
  const start = source.indexOf("await startDotServer(");
  assert.ok(install > 0 && install < start, "the runner is published before the server exists");
  assert.match(source, /if \(process\.platform === "win32"\)\s*(\{\s*)?installToolRunner\(globalThis, createToolRunner\(\)\)/);
  // Existing startup order is untouched.
  assert.ok(source.indexOf("installVoiceController(globalThis") < install);
  assert.ok(source.indexOf("installAgentHook(globalThis") > 0);
});

test("the desktop modules keep to the desktop root: no imports from lib or app", async () => {
  for (const name of ["tool-runner", "tool-process", "tool-classify", "tool-claude"]) {
    const source = await readFile(new URL(`../desktop/${name}.mts`, import.meta.url), "utf8");
    assert.ok(!/from\s+["'](\.\.\/|\.\/\.\.)/.test(source), `${name} imports only from desktop/`);
  }
});

// CLIXML on stderr -------------------------------------------------------------------------------------------------

test("decodeClixml turns PowerShell's serialized stderr back into plain text", async () => {
  const { decodeClixml } = await import("../desktop/tool-process.mts");
  const clixml = '#< CLIXML\r\n<Objs Version="1.1.0.1" xmlns="http://schemas.microsoft.com/powershell/2004/04"><S S="Error">Get-Item : No se encuentra &apos;C:\\x&apos; &lt;ahora&gt; &amp; _x005F_x_x000D__x000A_</S><S S="Error">+ Get-Item C:\\x_x000D__x000A_</S><S S="Warning">careful_x000D__x000A_</S><S S="Output">ignored</S></Objs>';
  assert.equal(decodeClixml(clixml), "Get-Item : No se encuentra 'C:\\x' <ahora> & _x\r\n+ Get-Item C:\\x\r\ncareful\r\n");
});

test("decodeClixml leaves plain text, empty text and CLIXML without items alone", async () => {
  const { decodeClixml } = await import("../desktop/tool-process.mts");
  assert.equal(decodeClixml("boom\r\n"), "boom\r\n");
  assert.equal(decodeClixml(""), "");
  const noItems = '#< CLIXML\r\n<Objs Version="1.1.0.1"><PR N="p">x</PR></Objs>';
  assert.equal(decodeClixml(noItems), noItems);
  assert.equal(decodeClixml("text #< CLIXML <S S=\"Error\">x</S>"), "text #< CLIXML <S S=\"Error\">x</S>");
});

test("runPowerShell returns stderr decoded from CLIXML", async () => {
  const { runner, procs } = setup();
  const pending = runner.runPowerShell(request());
  await tick();
  procs[0].err('#< CLIXML\r\n<Objs Version="1.1.0.1"><S S="Error">The term is not recognized_x000D__x000A_</S></Objs>');
  procs[0].close(1);
  assert.equal((await pending).stderr, "The term is not recognized\r\n");
});
