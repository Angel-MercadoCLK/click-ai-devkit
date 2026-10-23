import assert from "node:assert/strict";
import { test } from "node:test";

const mod = await import("../desktop/codex-app-server.mts");

const tick = () => new Promise((resolve) => setImmediate(resolve));
const settle = async (times = 3) => { for (let i = 0; i < times; i += 1) await tick(); };

function manualTimers() {
  const pending = new Map();
  let next = 1;
  return {
    setTimeout: (fn, ms) => { const id = next++; pending.set(id, { fn, ms }); return id; },
    clearTimeout: (id) => { pending.delete(id); },
    fire: (ms) => { for (const [id, timer] of [...pending]) if (timer.ms === ms) { pending.delete(id); timer.fn(); } },
    msList: () => [...pending.values()].map((timer) => timer.ms),
    pending,
  };
}

/** A fake `codex`: `--version` answers from `version`, `app-server` is a child the test drives line by line. */
function fakeCodex({ version = "codex-cli 0.163.0\n", versionExit = 0, autoInitialize = true, startFails = false } = {}) {
  const control = { autoInitialize, version, versionExit, startFails };
  const spawns = [];
  const servers = [];
  function child(kind) {
    const listeners = {};
    const state = { stdin: [], ended: false, killed: false };
    const api = {
      pid: 1000 + spawns.length,
      stdin: { write: (chunk) => { state.stdin.push(String(chunk)); api.onWrite?.(String(chunk)); return true; }, end: () => { state.ended = true; if (kind === "server" && api.exitOnEnd) setImmediate(() => listeners.close?.(0)); }, on: () => {} },
      stdout: { on: (event, listener) => { listeners.stdout = listener; } },
      stderr: { on: (event, listener) => { listeners.stderr = listener; } },
      on: (event, listener) => { listeners[event] = listener; },
      kill: () => { state.killed = true; },
    };
    return {
      api,
      state,
      listeners,
      lines: () => state.stdin.join("").split("\n").filter(Boolean).map((line) => JSON.parse(line)),
      send: (value) => listeners.stdout?.(Buffer.from(typeof value === "string" ? value : `${JSON.stringify(value)}\n`)),
      exit: (code = 1) => listeners.close?.(code),
    };
  }
  const spawn = (executable, args, options) => {
    if (control.startFails) throw new Error("ENOENT");
    const isVersion = args.some((arg) => String(arg).includes("--version"));
    const proc = child(isVersion ? "version" : "server");
    const record = { executable, args, options, proc };
    spawns.push(record);
    if (isVersion) {
      setImmediate(() => { proc.listeners.stdout?.(Buffer.from(control.version)); proc.listeners.close?.(control.versionExit); });
    } else {
      servers.push(record);
      {
        proc.api.onWrite = (chunk) => {
          if (!control.autoInitialize) return;
          for (const line of chunk.split("\n").filter(Boolean)) {
            const message = JSON.parse(line);
            if (message.method === "initialize") setImmediate(() => proc.send({ id: message.id, result: { userAgent: "click-dot/0.163.0", codexHome: "/home/u/.codex", platformFamily: "unix", platformOs: "linux" } }));
          }
        };
      }
    }
    return proc.api;
  };
  return { spawn, spawns, servers, control, last: () => servers[servers.length - 1].proc };
}

function setup(options = {}, codexOptions = {}) {
  const fake = fakeCodex(codexOptions);
  const timers = manualTimers();
  const server = mod.createCodexAppServer({
    spawn: fake.spawn,
    platform: "linux",
    env: { PATH: "/bin", KEEP: "1" },
    timers,
    killTree: () => {},
    which: async () => ["/usr/bin/codex"],
    ...options,
  });
  return { server, fake, timers };
}

// Version ---------------------------------------------------------------------------------------------------------

test("codex 0.163.0 and anything newer is supported, from one pinned range constant", () => {
  assert.equal(mod.CODEX_SUPPORTED_RANGE, ">=0.163.0");
  for (const ok of ["codex-cli 0.163.0", "codex-cli 0.163.7\n", "0.163.0", "codex-cli 0.170.0", "codex-cli 0.164.2", "codex-cli 1.0.0", "codex-cli 0.164.0-alpha.1"]) assert.equal(mod.isSupportedCodexVersion(ok), true, ok);
  for (const bad of ["codex-cli 0.162.9", "codex-cli 0.162.0", "codex-cli 0.145.0", "codex-cli 0.163.0-alpha.1", "", "no version here"]) assert.equal(mod.isSupportedCodexVersion(bad), false, bad);
});

test("an unsupported codex stops before app-server starts and says so in Spanish", async () => {
  const { server, fake } = setup({}, { version: "codex-cli 0.145.0\n" });
  const result = await server.start();
  assert.equal(result.ok, false);
  assert.equal(result.reason, "unsupported");
  assert.equal(result.version, "0.145.0");
  assert.match(result.message, /Codex/);
  assert.match(result.message, /0\.145\.0/);
  assert.match(result.message, />=0\.163\.0/);
  assert.equal(fake.servers.length, 0);
  assert.equal(server.status().state, "unavailable");
});

test("a missing codex (no launcher, a failing --version or no spawn) is reported as missing", async () => {
  for (const [options, codexOptions] of [
    [{ which: async () => [] }, {}],
    [{ which: async () => null, platform: "win32" }, {}],
    [{}, { versionExit: 127, version: "" }],
    [{}, { startFails: true }],
  ]) {
    const { server, fake } = setup(options, codexOptions);
    const result = await server.start();
    assert.equal(result.ok, false);
    assert.equal(result.reason, "missing");
    assert.match(result.message, /Codex/);
    assert.equal(fake.servers.length, 0);
  }
});

// Handshake and arguments -----------------------------------------------------------------------------------------

test("the handshake is initialize, then its response, then the initialized notification, and nothing else first", async () => {
  const { server, fake } = setup();
  const started = server.start();
  await settle();
  const sent = fake.last().lines();
  const result = await started;
  assert.equal(sent.length >= 1, true);
  assert.equal(sent[0].method, "initialize");
  assert.deepEqual(sent[0].params.clientInfo.name, "click-dot");
  assert.equal(sent[0].params.capabilities.experimentalApi, true);
  assert.equal(sent[0].params.capabilities.requestAttestation, false);
  assert.equal(typeof sent[0].id, "number");
  assert.equal("jsonrpc" in sent[0], false, "codex speaks JSON-RPC lite: no jsonrpc field");
  const after = fake.last().lines();
  assert.deepEqual(after.map((message) => message.method), ["initialize", "initialized"]);
  assert.equal("id" in after[1], false);
  assert.deepEqual(result, { ok: true, version: "0.163.0", userAgent: "click-dot/0.163.0", codexHome: "/home/u/.codex" });
  assert.equal(server.status().state, "ready");
  assert.equal(fake.spawns[0].args.includes("--version"), true, "the version is checked before app-server starts");
});

test("app-server runs as `codex app-server` with every config override as -c, hidden, without a shell, with the extra environment", async () => {
  const overrides = ['mcp_servers.dot.url="http://127.0.0.1:5000/mcp"', "mcp_servers.dot.bearer_token_env_var=DOT_MCP_TOKEN"];
  const { server, fake } = setup({ configOverrides: overrides, env: { PATH: "/bin", KEEP: "1", DOT_MCP_TOKEN: "secret" } });
  await server.start();
  const { executable, args, options } = fake.servers[0];
  assert.equal(executable, "/usr/bin/codex");
  assert.deepEqual(args, ["app-server", "-c", overrides[0], "-c", overrides[1]]);
  assert.equal(options.windowsHide, true);
  assert.equal(options.shell, false);
  assert.deepEqual(options.stdio, ["pipe", "pipe", "pipe"]);
  assert.equal(options.env.DOT_MCP_TOKEN, "secret");
  assert.equal(options.env.KEEP, "1");
  assert.equal("CODEX_HOME" in options.env, false, "the user's default CODEX_HOME is used");
});

test("a failing handshake (no answer in time, or the child dies) is a failed start", async () => {
  const hung = setup({}, { autoInitialize: false });
  const startedHung = hung.server.start();
  await settle();
  hung.timers.fire(20_000);
  const resultHung = await startedHung;
  assert.deepEqual([resultHung.ok, resultHung.reason], [false, "failed"]);
  assert.equal(hung.fake.last().state.killed, true);

  const dies = setup({}, { autoInitialize: false });
  const startedDies = dies.server.start();
  await settle();
  dies.fake.last().exit(1);
  assert.deepEqual([(await startedDies).ok, dies.server.status().state], [false, "unavailable"]);
  assert.equal(dies.fake.servers.length, 1, "a failed first start does not loop");
});

// Windows launchers -----------------------------------------------------------------------------------------------

test("on Windows a real codex.exe is preferred and a .cmd shim runs through cmd.exe /d /s /c with constant arguments only", async () => {
  const exe = setup({ platform: "win32", which: async () => "C:\\npm\\codex\nC:\\npm\\codex.cmd\nC:\\Tools\\codex.exe\n" });
  await exe.server.start();
  assert.equal(exe.fake.servers[0].executable, "C:\\Tools\\codex.exe");
  assert.deepEqual(exe.fake.servers[0].args, ["app-server"]);

  const shim = setup({ platform: "win32", env: { ComSpec: "C:\\Windows\\System32\\cmd.exe", PATH: "C:\\" }, which: async () => ["C:\\Users\\a b\\AppData\\Roaming\\npm\\codex", "C:\\Users\\a b\\AppData\\Roaming\\npm\\codex.cmd"], configOverrides: ["mcp_servers.dot.url='http://127.0.0.1:5000/mcp'", "features.x=true"] });
  const started = await shim.server.start();
  assert.equal(started.ok, true);
  const { executable, args, options } = shim.fake.servers[0];
  assert.equal(executable, "C:\\Windows\\System32\\cmd.exe");
  assert.deepEqual(args, ["/d", "/s", "/c", `""C:\\Users\\a b\\AppData\\Roaming\\npm\\codex.cmd" app-server -c mcp_servers.dot.url='http://127.0.0.1:5000/mcp' -c features.x=true"`]);
  assert.equal(options.windowsVerbatimArguments, true);
  assert.equal(options.shell, false);
});

test("through a .cmd shim an override or path that cmd.exe could interpret is refused before anything runs", async () => {
  for (const override of ['mcp_servers.dot.url="http://x"', "a=b&calc", "a=%PATH%", "a=b c", "a=b|c", "a=b^c", "a=b>c"]) {
    const { server, fake } = setup({ platform: "win32", which: async () => ["C:\\npm\\codex.cmd"], configOverrides: [override] });
    const result = await server.start();
    assert.deepEqual([result.ok, result.reason], [false, "failed"], override);
    assert.equal(fake.servers.length, 0, override);
  }
  const badPath = setup({ platform: "win32", which: async () => ['C:\\a"b\\codex.cmd'] });
  assert.equal((await badPath.server.start()).ok, false);
  assert.equal(badPath.fake.servers.length, 0);
});

// JSON-RPC --------------------------------------------------------------------------------------------------------

test("responses are matched to their requests by id, in any order, and an error response rejects with its code", async () => {
  const { server, fake } = setup();
  await server.start();
  const proc = fake.last();
  const first = server.request("thread/list", { limit: 1 });
  const second = server.request("account/read", {});
  const third = server.request("thread/read", { threadId: "x" });
  const [a, b, c] = proc.lines().slice(-3);
  assert.deepEqual([a.method, b.method, c.method], ["thread/list", "account/read", "thread/read"]);
  assert.deepEqual(a.params, { limit: 1 });
  proc.send({ id: c.id, error: { code: -32600, message: "no rollout found" } });
  proc.send({ id: b.id, result: { account: null } });
  proc.send({ id: a.id, result: { data: [] } });
  assert.deepEqual(await first, { data: [] });
  assert.deepEqual(await second, { account: null });
  await assert.rejects(third, (error) => error instanceof mod.CodexRpcError && error.code === -32600 && error.message === "no rollout found");
});

test("a request that gets no answer times out, frees its slot, and a late answer is ignored", async () => {
  const { server, fake, timers } = setup({ requestTimeoutMs: 7_000 });
  await server.start();
  const slow = server.request("thread/list", {});
  const override = server.request("thread/list", {}, { timeoutMs: 1_500 });
  assert.deepEqual(timers.msList().sort((x, y) => x - y), [1_500, 7_000]);
  timers.fire(1_500);
  await assert.rejects(override, (error) => error instanceof mod.CodexTimeoutError);
  timers.fire(7_000);
  await assert.rejects(slow, (error) => error instanceof mod.CodexTimeoutError && /thread\/list/.test(error.message));
  const lateId = fake.last().lines().slice(-2)[0].id;
  fake.last().send({ id: lateId, result: {} });
  await settle();
  assert.equal(timers.pending.size, 0);
});

test("an aborted request signal rejects that request only", async () => {
  const { server } = setup();
  await server.start();
  const abort = new AbortController();
  const waiting = server.request("thread/list", {}, { signal: abort.signal });
  abort.abort();
  await assert.rejects(waiting, (error) => error instanceof mod.CodexAbortError);
  await assert.rejects(server.request("thread/list", {}, { signal: abort.signal }), mod.CodexAbortError);
});

test("notifications reach every listener until it unsubscribes, however the output is chunked, and junk lines are ignored", async () => {
  const { server, fake } = setup();
  await server.start();
  const seen = [];
  const off = server.onNotification((n) => seen.push(n));
  server.onNotification((n) => { throw new Error("a listener failure never breaks the others"); });
  const proc = fake.last();
  const one = JSON.stringify({ method: "item/agentMessage/delta", params: { delta: "Hol" } });
  const two = JSON.stringify({ method: "item/agentMessage/delta", params: { delta: "a — ñ" } });
  proc.send(`${one}\nnot json\n${two.slice(0, 20)}`);
  proc.send(`${two.slice(20)}\n\n`);
  assert.deepEqual(seen.map((n) => n.params.delta), ["Hol", "a — ñ"]);
  off();
  proc.send({ method: "turn/completed", params: {} });
  assert.equal(seen.length, 2);
});

test("a UTF-8 character split across two chunks is not corrupted", async () => {
  const { server, fake } = setup();
  await server.start();
  const seen = [];
  server.onNotification((n) => seen.push(n.params.delta));
  const bytes = Buffer.from(`${JSON.stringify({ method: "item/agentMessage/delta", params: { delta: "año" } })}\n`);
  const cut = bytes.indexOf(0xc3) + 1;
  fake.last().listeners.stdout(bytes.subarray(0, cut));
  fake.last().listeners.stdout(bytes.subarray(cut));
  assert.deepEqual(seen, ["año"]);
});

test("a server request is answered with its handler's result by the same id, or an error when the handler fails or there is none", async () => {
  const { server, fake } = setup();
  await server.start();
  const proc = fake.last();
  proc.send({ id: 41, method: "item/commandExecution/requestApproval", params: { command: "ls" } });
  await settle();
  assert.deepEqual(proc.lines().at(-1), { id: 41, error: { code: -32601, message: "Method not found: item/commandExecution/requestApproval" } });

  const calls = [];
  server.setRequestHandler(async (request) => { calls.push(request); if (request.params.fail) throw new Error("boom"); return { decision: "accept" }; });
  proc.send({ id: "abc", method: "item/commandExecution/requestApproval", params: { command: "ls" } });
  proc.send({ id: 43, method: "item/fileChange/requestApproval", params: { fail: true } });
  await settle();
  assert.deepEqual(calls.map((call) => [call.id, call.method]), [["abc", "item/commandExecution/requestApproval"], [43, "item/fileChange/requestApproval"]]);
  const replies = proc.lines().slice(-2);
  assert.deepEqual(replies.find((r) => r.id === "abc"), { id: "abc", result: { decision: "accept" } });
  assert.equal(replies.find((r) => r.id === 43).error.code, -32603);
});

test("an answer to a request from a process that has since crashed is dropped, never sent to the new process", async () => {
  const { server, fake, timers } = setup();
  await server.start();
  let release;
  server.setRequestHandler(() => new Promise((resolve) => { release = resolve; }));
  fake.last().send({ id: 0, method: "item/commandExecution/requestApproval", params: { command: "harmless" } });
  await settle();
  fake.last().exit(1);
  await settle();
  timers.fire(1_000);
  await settle(5);
  const fresh = fake.last();
  assert.equal(fake.servers.length, 2);
  release({ decision: "accept" });
  await settle();
  assert.equal(fresh.lines().some((message) => message.id === 0 && "result" in message), false, "the stale answer is dropped");
});

// Re-detection (S1) -----------------------------------------------------------------------------------------------

const versionSpawns = (fake) => fake.spawns.filter((record) => record.args.some((arg) => String(arg).includes("--version"))).length;

test("an update is noticed without restarting Dot: a recheck after an unsupported start finds the new version and starts", async () => {
  const { server, fake } = setup({}, { version: "codex-cli 0.145.0\n" });
  const first = await server.start();
  assert.equal(first.reason, "unsupported");
  assert.equal(server.status().state, "unavailable");

  fake.control.version = "codex-cli 0.163.0\n";
  const status = await server.recheck();
  assert.equal(status.state, "ready");
  assert.equal(status.version, "0.163.0");
  assert.equal(server.status().state, "ready");
  assert.equal(fake.servers.length, 1);
  assert.deepEqual(fake.last().lines().map((m) => m.method), ["initialize", "initialized"]);
});

test("a recheck also recovers a codex that was missing, and one that gave up restarting", async () => {
  const missing = setup({}, { startFails: true });
  assert.equal((await missing.server.start()).reason, "missing");
  missing.fake.control.startFails = false;
  assert.equal((await missing.server.recheck()).state, "ready");

  const crashed = setup();
  await crashed.server.start();
  crashed.fake.control.autoInitialize = false;
  crashed.fake.last().exit(1);
  for (let attempt = 0; attempt < 6; attempt += 1) {
    const [delay] = crashed.timers.msList();
    if (delay === undefined) break;
    crashed.timers.fire(delay);
    await settle();
    crashed.fake.last().exit(1);
    await settle();
  }
  assert.equal(crashed.server.status().state, "unavailable");
  crashed.fake.control.autoInitialize = true;
  assert.equal((await crashed.server.recheck()).state, "ready");
  assert.equal(crashed.server.status().restarts, 0);
});

test("a recheck that still finds an unsupported codex keeps the Spanish note and does not flicker through starting", async () => {
  const { server, fake } = setup({}, { version: "codex-cli 0.145.0\n" });
  await server.start();
  const states = [];
  server.onStatus((status) => states.push(status.state));
  fake.control.version = "codex-cli 0.150.0\n";
  const status = await server.recheck();
  assert.equal(status.state, "unavailable");
  assert.match(status.message, /0\.150\.0/);
  assert.ok(!states.includes("starting"), `states: ${states.join(",")}`);
});

test("concurrent rechecks are coalesced into one detection", async () => {
  const { server, fake } = setup({}, { version: "codex-cli 0.145.0\n" });
  await server.start();
  const before = versionSpawns(fake);
  fake.control.version = "codex-cli 0.163.0\n";
  const [a, b, c] = await Promise.all([server.recheck(), server.recheck(), server.recheck()]);
  assert.equal(versionSpawns(fake) - before, 1);
  assert.equal(fake.servers.length, 1);
  assert.deepEqual([a.state, b.state, c.state], ["ready", "ready", "ready"]);
});

test("a recheck on a running codex does nothing, and after stop it never restarts the process", async () => {
  const { server, fake } = setup();
  await server.start();
  const before = fake.spawns.length;
  assert.equal((await server.recheck()).state, "ready");
  assert.equal(fake.spawns.length, before);
  fake.last().api.exitOnEnd = true;
  await server.stop();
  assert.equal((await server.recheck()).state, "stopped");
  assert.equal(fake.spawns.length, before);
});

test("a codex newer than the generated protocol is tried; when the handshake fails the note says so in Spanish", async () => {
  assert.equal(mod.CODEX_PROTOCOL_VERSION, "0.163.0");
  const ok = setup({}, { version: "codex-cli 0.170.0\n" });
  const worked = await ok.server.start();
  assert.equal(worked.ok, true);
  assert.equal(ok.server.status().version, "0.170.0");

  const bad = setup({}, { version: "codex-cli 0.170.0\n", autoInitialize: false });
  const failed = bad.server.start();
  await settle();
  bad.timers.fire(20_000);
  const result = await failed;
  assert.equal(result.ok, false);
  assert.equal(result.reason, "failed");
  assert.equal(result.message, "Tu versión de Codex (0.170.0) es más nueva que la que Dot conoce; no pudo conectarse. Actualiza Dot.");
  assert.equal(bad.server.status().message, result.message);

  const same = setup({}, { version: "codex-cli 0.163.0\n", autoInitialize: false });
  const sameStart = same.server.start();
  await settle();
  same.timers.fire(20_000);
  const sameResult = await sameStart;
  assert.equal(sameResult.message, "Codex no pudo iniciarse. Dot usa su asistente integrado mientras tanto.");
});

// Crash and shutdown ----------------------------------------------------------------------------------------------

test("a crash rejects what was waiting, restarts with growing backoff, handshakes again and tells the restart listeners", async () => {
  const { server, fake, timers } = setup();
  await server.start();
  const statuses = [];
  server.onStatus((status) => statuses.push(status.state));
  let restarts = 0;
  server.onRestart(() => { restarts += 1; });
  const waiting = server.request("thread/list", {});
  fake.last().exit(1);
  await assert.rejects(waiting, (error) => error instanceof mod.CodexClosedError);
  assert.equal(server.status().state, "restarting");
  assert.deepEqual(timers.msList(), [1_000]);
  await assert.rejects(server.request("thread/list", {}), mod.CodexClosedError, "nothing is sent while down");

  timers.fire(1_000);
  await settle(5);
  assert.equal(fake.servers.length, 2);
  assert.deepEqual(fake.last().lines().map((m) => m.method), ["initialize", "initialized"]);
  assert.equal(server.status().state, "ready");
  assert.equal(server.status().restarts, 1);
  assert.equal(restarts, 1);
  assert.deepEqual(statuses, ["restarting", "ready"]);

  fake.last().exit(1);
  await settle();
  assert.deepEqual(timers.msList(), [1_000], "a healthy restart resets the backoff");
});

test("repeated failed restarts back off 1 s, 2 s, 5 s, 10 s, 30 s and then give up as unavailable", async () => {
  const { server, fake, timers } = setup();
  await server.start();
  fake.control.autoInitialize = false;
  fake.last().exit(1);
  const delays = [];
  for (let attempt = 0; attempt < 6; attempt += 1) {
    const [delay] = timers.msList();
    if (delay === undefined) break;
    delays.push(delay);
    // Make the next child die before it answers initialize.
    timers.fire(delay);
    await settle();
    fake.last().exit(1);
    await settle();
  }
  assert.deepEqual(delays, [1_000, 2_000, 5_000, 10_000, 30_000]);
  assert.equal(server.status().state, "unavailable");
  assert.equal(timers.pending.size, 0);
});

test("a clean stop ends stdin, does not restart, and kills a child that will not leave", async () => {
  const { server, fake, timers } = setup();
  await server.start();
  fake.last().api.exitOnEnd = true;
  await server.stop();
  assert.equal(fake.last().state.ended, true);
  assert.equal(server.status().state, "stopped");
  await settle();
  assert.equal(timers.pending.size, 0);
  assert.equal(fake.servers.length, 1);
  await assert.rejects(server.request("thread/list", {}), mod.CodexClosedError);

  const stuck = setup();
  await stuck.server.start();
  const stopping = stuck.server.stop();
  await settle();
  assert.equal(stuck.fake.last().state.killed, false);
  stuck.timers.fire(3_000);
  await stopping;
  assert.equal(stuck.fake.last().state.killed, true);
});

// Bridge ----------------------------------------------------------------------------------------------------------

test("the bridge publishes the server under click-dot.codex-engine and refuses anything that is not a transport", async () => {
  const { server } = setup();
  const target = {};
  mod.installCodexEngine(target, server);
  assert.equal(target[Symbol.for("click-dot.codex-engine")], server);
  assert.equal(mod.CODEX_ENGINE_KEY, "click-dot.codex-engine");
  assert.throws(() => mod.installCodexEngine({}, { request() {} }), /malformed/);
  assert.throws(() => mod.installCodexEngine({}, null), /malformed/);
});

// System browser for the Codex sign-in ----------------------------------------------------------------------------

test("the system browser opens only an https page on auth.openai.com, without credentials, and says whether it did", async () => {
  const opened = [];
  const target = {};
  mod.installSystemBrowser(target, (url) => opened.push(url));
  const open = Reflect.get(target, Symbol.for(mod.SYSTEM_BROWSER_KEY));
  const good = "https://auth.openai.com/oauth/authorize?response_type=code&redirect_uri=http%3A%2F%2F127.0.0.1%3A1455%2Fauth%2Fcallback";
  assert.equal(open(good), true);
  assert.deepEqual(opened, [good], "the URL is opened as Codex gave it, not replaced by Dot's own page");
  for (const bad of ["http://auth.openai.com/x", "https://evil.example/x", "https://auth.openai.com.evil.example/x", "https://user:pw@auth.openai.com/x", "file:///etc/passwd", "javascript:alert(1)", "not a url", ""]) {
    assert.equal(open(bad), false, bad);
  }
  assert.equal(opened.length, 1);
  assert.equal(open(42), false);
});
