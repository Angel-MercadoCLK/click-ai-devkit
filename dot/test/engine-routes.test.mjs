import assert from "node:assert/strict";
import { test } from "node:test";

process.env.PORT = "3000";
const http = await import("../lib/engine-http.ts");
const engineLib = await import("../lib/engine.ts");

const CONTROL_KEY = Symbol.for("click-dot.control-token");
const TOKEN = "b".repeat(64);
const THREAD = "01a165bf-926d-79f0-9b12-3ce7cd822aea";

const get = (path, headers = {}) => new Request(`http://127.0.0.1:3000${path}`, { headers: { host: "127.0.0.1:3000", ...headers } });
const write = (path, init = {}) => new Request(`http://127.0.0.1:3000${path}`, { method: "POST", ...init, headers: { host: "127.0.0.1:3000", origin: "http://127.0.0.1:3000", ...(init.headers ?? {}) } });

function fakeEngine(extra = {}) {
  const calls = [];
  return {
    calls,
    status: () => ({ state: "ready", restarts: 0, version: "0.163.0" }),
    accountStatus: async () => ({ state: "signedIn", method: "chatgpt", email: "a@b.c", plan: "plus" }),
    listThreads: async (options) => { calls.push(["list", options]); return { threads: [{ id: THREAD, title: "Mi chat", updatedAt: 1, cwd: "/p" }], nextCursor: null }; },
    readThread: async (id, options) => { calls.push(["read", id, options]); return { id, title: "Mi chat", updatedAt: 1, cwd: "/p", messages: [{ role: "user", text: "hola" }] }; },
    archiveThread: async (id) => { calls.push(["archive", id]); },
    stopComputerUse: () => { calls.push(["stop"]); return 1; },
    activity: () => ({ computerUse: { active: true, servers: ["computer"], threadIds: [THREAD] } }),
    startLogin: async () => ({ loginId: "L", authUrl: "https://auth.openai.com/x" }),
    waitForLogin: () => new Promise(() => {}),
    ...extra,
  };
}

function handlers(engine, extra = {}) {
  const flow = engineLib.createLoginFlow();
  return { flow, h: http.createEngineHandlers({ getEngine: () => engine, readTools: () => null, loginFlow: flow, getSystemBrowser: () => () => true, readTasks: () => [], ...extra }) };
}

async function withToken(run) {
  const saved = globalThis[CONTROL_KEY];
  globalThis[CONTROL_KEY] = TOKEN;
  try { await run(); } finally { if (saved === undefined) delete globalThis[CONTROL_KEY]; else globalThis[CONTROL_KEY] = saved; }
}

test("every engine route refuses a foreign host or origin and, once the shell published a token, a request without the cookie", async () => {
  const { h } = handlers(fakeEngine());
  assert.equal((await h.status(get("/api/codex/status", { host: "evil.example" }))).status, 403);
  assert.equal((await h.threads(get("/api/codex/threads", { origin: "http://evil.example" }))).status, 403);
  assert.equal((await h.login(write("/api/codex/login", { headers: { origin: "http://evil.example" } }))).status, 403);
  assert.equal((await h.login(new Request("http://127.0.0.1:3000/api/codex/login", { method: "POST", headers: { host: "127.0.0.1:3000" } }))).status, 403, "a write must send the exact origin");
  await withToken(async () => {
    for (const response of [await h.status(get("/api/codex/status")), await h.activity(get("/api/agent/activity")), await h.stop(write("/api/codex/stop")), await h.thread(get(`/api/codex/threads/${THREAD}`), THREAD)]) {
      assert.equal(response.status, 403);
    }
    assert.equal((await h.status(get("/api/codex/status", { cookie: `click_dot_control=${TOKEN}` }))).status, 200);
  });
});

test("the status route answers the engine status the chat card needs", async () => {
  const { h } = handlers(fakeEngine());
  const body = await (await h.status(get("/api/codex/status"))).json();
  assert.equal(body.engine, "codex");
  assert.equal(body.codex.account.email, "a@b.c");
  assert.equal((await h.status(get("/api/codex/status"))).headers.get("cache-control").includes("no-store"), true);
});

test("signing in opens the page and answers pending; cancelling and a Codex that is not ready are handled", async () => {
  const { h, flow } = handlers(fakeEngine());
  const body = await (await h.login(write("/api/codex/login"))).json();
  assert.deepEqual(body, { authUrl: "https://auth.openai.com/x", opened: true });
  assert.equal(flow.view().state, "pending");
  const cancelled = await h.login(write("/api/codex/login", { method: "DELETE" }));
  assert.equal(cancelled.status, 200);
  assert.equal(flow.view().state, "idle");
  const notReady = handlers(fakeEngine({ status: () => ({ state: "unavailable", restarts: 0, message: "Dot no encontró Codex en este equipo." }) })).h;
  const refused = await notReady.login(write("/api/codex/login"));
  assert.equal(refused.status, 409);
  assert.equal((await refused.json()).error, "Dot no encontró Codex en este equipo.");
  assert.equal((await handlers(null).h.login(write("/api/codex/login"))).status, 409);
});

test("the thread list is searchable and paged, with bounded query values", async () => {
  const engine = fakeEngine();
  const { h } = handlers(engine);
  const body = await (await h.threads(get("/api/codex/threads?q=chat&cursor=c0"))).json();
  assert.deepEqual(body, { threads: [{ id: THREAD, title: "Mi chat", updatedAt: 1, cwd: "/p" }], nextCursor: null });
  assert.deepEqual(engine.calls[0], ["list", { searchTerm: "chat", cursor: "c0" }]);
  await h.threads(get("/api/codex/threads"));
  assert.deepEqual(engine.calls[1], ["list", {}]);
  assert.equal((await h.threads(get(`/api/codex/threads?q=${"x".repeat(201)}`))).status, 400);
  assert.equal((await h.threads(get(`/api/codex/threads?cursor=${"x".repeat(501)}`))).status, 400);
  assert.equal((await handlers(null).h.threads(get("/api/codex/threads"))).status, 409);
});

test("one thread opens with its messages, deleting archives it, and a malformed id never reaches Codex", async () => {
  const engine = fakeEngine();
  const { h } = handlers(engine);
  const opened = await (await h.thread(get(`/api/codex/threads/${THREAD}`), THREAD)).json();
  assert.equal(opened.thread.messages[0].text, "hola");
  assert.deepEqual(engine.calls[0], ["read", THREAD, { includeMessages: true }]);
  const archived = await h.thread(write(`/api/codex/threads/${THREAD}`, { method: "DELETE" }), THREAD);
  assert.equal(archived.status, 200);
  assert.deepEqual(engine.calls[1], ["archive", THREAD]);
  assert.equal((await h.thread(get("/api/codex/threads/..%2Fx"), "../x")).status, 400);
  assert.equal((await h.thread(write("/api/codex/threads/x", { method: "DELETE" }), "x y")).status, 400);
  assert.equal(engine.calls.length, 2);
  const failing = handlers(fakeEngine({ readThread: async () => { throw new Error("raw detail"); } })).h;
  const failed = await failing.thread(get(`/api/codex/threads/${THREAD}`), THREAD);
  assert.equal(failed.status, 502);
  assert.ok(!(await failed.text()).includes("raw detail"), "Codex's own error text is not forwarded");
});

test("the activity route lists the background tasks and whether Dot is using the computer; the stop route interrupts it", async () => {
  const engine = fakeEngine();
  const tasks = [{ id: "task_aaaaaaaaaaaa", project: "/p", state: "running", latestLine: "x", startedAt: 5 }];
  const { h } = handlers(engine, { readTasks: () => tasks });
  const body = await (await h.activity(get("/api/agent/activity"))).json();
  assert.deepEqual(body, { tasks: [{ id: "task_aaaaaaaaaaaa", project: "/p", state: "running", latestLine: "x", startedAt: 5 }], computerUse: { active: true, servers: ["computer"] }, codexTurns: [] });
  const stopped = await (await h.stop(write("/api/codex/stop", { headers: { "content-type": "application/json" }, body: "{}" }))).json();
  assert.deepEqual(stopped, { stopped: 1 });
  assert.deepEqual(engine.calls.at(-1), ["stop"]);
  const none = await (await handlers(null).h.activity(get("/api/agent/activity"))).json();
  assert.deepEqual(none, { tasks: [], computerUse: { active: false, servers: [] }, codexTurns: [] });
  assert.deepEqual(await (await handlers(null).h.stop(write("/api/codex/stop"))).json(), { stopped: 0 });
});

test("the published task list is read through the bridge and a malformed one reads as no tasks", () => {
  const key = Symbol.for("click-dot.claude-tasks");
  const saved = globalThis[key];
  try {
    delete globalThis[key];
    assert.deepEqual(http.readClaudeTasks(), []);
    globalThis[key] = { list: () => "nope" };
    assert.deepEqual(http.readClaudeTasks(), []);
    globalThis[key] = { list: () => [{ id: "task_aaaaaaaaaaaa", project: "/p", state: "done", latestLine: null, startedAt: 1, secret: "must not leak" }, { id: 5 }] };
    assert.deepEqual(http.readClaudeTasks(), [{ id: "task_aaaaaaaaaaaa", project: "/p", state: "done", latestLine: null, startedAt: 1 }]);
  } finally { if (saved === undefined) delete globalThis[key]; else globalThis[key] = saved; }
});

// Recheck (S1) -------------------------------------------------------------------------------------------------------

test("the recheck route has the guard of the other write routes", async () => {
  let rechecks = 0;
  const { h } = handlers(fakeEngine({ recheck: async () => { rechecks += 1; } }));
  assert.equal((await h.recheck(write("/api/codex/recheck", { headers: { origin: "http://evil.example" } }))).status, 403);
  assert.equal((await h.recheck(write("/api/codex/recheck", { headers: { host: "evil.example" } }))).status, 403);
  assert.equal((await h.recheck(new Request("http://127.0.0.1:3000/api/codex/recheck", { method: "POST", headers: { host: "127.0.0.1:3000" } }))).status, 403, "a write must send the exact origin");
  await withToken(async () => {
    assert.equal((await h.recheck(write("/api/codex/recheck"))).status, 403, "no cookie");
    assert.equal((await h.recheck(write("/api/codex/recheck", { headers: { cookie: `click_dot_control=${TOKEN}` } }))).status, 200);
  });
  assert.equal(rechecks, 1, "only the guarded request reached the engine");
});

test("an update is picked up by the recheck and a Codex that was already signed in is used at once, without a second sign-in", async () => {
  let state = { state: "unavailable", restarts: 0, message: "Dot necesita Codex >=0.163.0 y tienes la versión 0.145.0. Instala esa versión de Codex; mientras tanto Dot usa su asistente integrado." };
  let logins = 0;
  const engine = fakeEngine({
    status: () => state,
    recheck: async () => { state = { state: "ready", restarts: 0, version: "0.163.0" }; return state; },
    startLogin: async () => { logins += 1; return { loginId: "L", authUrl: "https://auth.openai.com/x" }; },
  });
  const { h, flow } = handlers(engine);
  const before = await (await h.status(get("/api/codex/status"))).json();
  assert.equal(before.engine, "builtin");
  assert.equal(before.codex.state, "unavailable");

  const response = await h.recheck(write("/api/codex/recheck"));
  assert.equal(response.status, 200);
  assert.ok(response.headers.get("cache-control").includes("no-store"));
  const after = await response.json();
  assert.equal(after.engine, "codex");
  assert.equal(after.codex.state, "ready");
  assert.equal(after.codex.version, "0.163.0");
  assert.equal(after.codex.account.state, "signedIn");
  assert.equal(logins, 0);
  assert.equal(flow.view().state, "idle");
  const selected = await engineLib.selectEngine({ getEngine: () => engine });
  assert.equal(selected?.kind, "codex");
});

test("a recheck that still fails answers the new note, a failing recheck never leaks its error, and no engine is simply absent", async () => {
  const stillOld = fakeEngine({
    status: () => ({ state: "unavailable", restarts: 0, message: "Tu versión de Codex (0.170.0) es más nueva que la que Dot conoce; no pudo conectarse. Actualiza Dot." }),
    recheck: async () => { throw new Error("secret path /home/u/.codex"); },
  });
  const body = await (await handlers(stillOld).h.recheck(write("/api/codex/recheck"))).json();
  assert.equal(body.engine, "builtin");
  assert.match(body.codex.message, /más nueva que la que Dot conoce/);
  assert.ok(!JSON.stringify(body).includes("secret"));
  const none = await handlers(null).h.recheck(write("/api/codex/recheck"));
  assert.equal(none.status, 200);
  assert.equal((await none.json()).codex.state, "absent");
});

test("the activity route lists the Codex turns running now next to the Claude tasks", async () => {
  const engine = fakeEngine({ activity: () => ({ computerUse: { active: false, servers: [], threadIds: [] }, turns: [{ threadId: THREAD, title: "Revisa el README", startedAt: 5 }] }) });
  const body = await (await handlers(engine).h.activity(get("/api/agent/activity"))).json();
  assert.deepEqual(body.codexTurns, [{ id: THREAD, title: "Revisa el README", startedAt: 5 }]);
  const idle = await (await handlers(null).h.activity(get("/api/agent/activity"))).json();
  assert.deepEqual(idle.codexTurns, []);
  const notReady = await (await handlers(fakeEngine({ status: () => ({ state: "unavailable", restarts: 0, message: "x" }) })).h.activity(get("/api/agent/activity"))).json();
  assert.deepEqual(notReady.codexTurns, []);
});
