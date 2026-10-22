import assert from "node:assert/strict";
import { chmod, mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { after, test } from "node:test";

const directory = await mkdtemp(join(tmpdir(), "click-dot-engine-stream-test-"));
await chmod(directory, 0o700);
process.env.CLICK_DOT_AUTH_FILE = join(directory, "auth.json");
process.env.CLICK_DOT_CHAT_FILE = join(directory, "conversations.json");
process.env.PORT = "3000";
after(() => rm(directory, { recursive: true, force: true }));

const streamModule = await import("../lib/engine-stream.ts");
const route = await import("../app/api/chat/stream/route.ts");

const THREAD = "01a165bf-926d-79f0-9b12-3ce7cd822aea";
const TURN = "turn-1";
const CONVERSATION = "11111111-1111-4111-8111-111111111111";
const BRIDGE = Symbol.for("click-dot.codex-engine");
const tick = () => new Promise((resolve) => setImmediate(resolve));
const settle = async (times = 6) => { for (let i = 0; i < times; i += 1) await tick(); };

const post = (body, init = {}) => new Request("http://127.0.0.1:3000/api/chat/stream", {
  method: "POST", signal: init.signal,
  headers: { host: "127.0.0.1:3000", origin: "http://127.0.0.1:3000", "content-type": "application/json" },
  body: JSON.stringify(body),
});

async function frames(response) {
  const text = await response.text();
  return text.split("\n\n").filter(Boolean).map((frame) => {
    const type = /^event: (.*)$/m.exec(frame)?.[1];
    return [type, JSON.parse(/^data: (.*)$/m.exec(frame)?.[1] ?? "null")];
  });
}

function fakeEngine(extra = {}) {
  const calls = { thread: [], turn: [] };
  return {
    calls,
    readThread: async () => ({ id: THREAD, title: "t", updatedAt: 1, cwd: "/work/proj" }),
    startOrResumeThread: async (input) => { calls.thread.push(input); return { threadId: input.threadId ?? THREAD, resumed: !!input.threadId }; },
    runTurn: async (request) => {
      calls.turn.push(request);
      request.emit({ type: "delta", text: "Ho" });
      request.emit({ type: "tool", tool: { id: "c1", name: "codex", summary: "ls", status: "running" } });
      request.emit({ type: "delta", text: "la" });
      return { text: "Hola", status: "completed" };
    },
    ...extra,
  };
}

const store = (paths) => ({ save: () => ({ ok: false, error: "x" }), remove: () => false, resolve: (id) => paths[id] });

test("a turn streams the thread, the same delta and tool events as the built-in loop, then completed", async () => {
  const engine = fakeEngine();
  const response = await streamModule.handleCodexStream(post({ conversationId: CONVERSATION, revision: 0, model: "codex", prompt: "hola" }), engine, { cwd: () => "/home/u", attachments: () => null });
  assert.equal(response.headers.get("content-type"), "text/event-stream; charset=utf-8");
  assert.deepEqual(await frames(response), [
    ["thread", { threadId: THREAD }],
    ["delta", { text: "Ho" }],
    ["tool", { id: "c1", name: "codex", summary: "ls", status: "running" }],
    ["delta", { text: "la" }],
    ["completed", {}],
  ]);
  assert.deepEqual(engine.calls.thread[0], { cwd: "/home/u" });
  assert.equal(engine.calls.turn[0].text, "hola");
  assert.deepEqual(engine.calls.turn[0].localImages, []);
});

test("a chat that already has a thread resumes it in its own folder; a new one starts in the user's home", async () => {
  const engine = fakeEngine();
  await streamModule.handleCodexStream(post({ prompt: "sigue", threadId: THREAD }), engine, { attachments: () => null });
  assert.equal(engine.calls.thread[0].threadId, THREAD);
  assert.equal(engine.calls.thread[0].cwd, "/work/proj");
  const home = fakeEngine();
  await streamModule.handleCodexStream(post({ prompt: "nuevo" }), home, { attachments: () => null });
  assert.equal(home.calls.thread[0].cwd, (await import("node:os")).homedir());
  const unreadable = fakeEngine({ readThread: async () => { throw new Error("gone"); } });
  await streamModule.handleCodexStream(post({ prompt: "sigue", threadId: THREAD }), unreadable, { cwd: () => "/home/u", attachments: () => null });
  assert.equal(unreadable.calls.thread[0].cwd, "/home/u", "a chat Codex cannot describe falls back to the home folder");
});

test("images go in as localImage, other files as their path in the text, and only ids the store issued are accepted", async () => {
  const engine = fakeEngine();
  const paths = {
    att_aaaaaaaaaaaaaaaa: { id: "att_aaaaaaaaaaaaaaaa", path: "/tmp/x/a.png", name: "a.png", kind: "image", bytes: 5 },
    att_bbbbbbbbbbbbbbbb: { id: "att_bbbbbbbbbbbbbbbb", path: "/tmp/x/n.md", name: "n.md", kind: "file", bytes: 5 },
  };
  await streamModule.handleCodexStream(post({ prompt: "mira", attachments: Object.keys(paths) }), engine, { attachments: () => store(paths) });
  assert.deepEqual(engine.calls.turn[0].localImages, ["/tmp/x/a.png"]);
  assert.match(engine.calls.turn[0].text, /^mira\n\nArchivos adjuntos .*\n- \/tmp\/x\/n\.md \(n\.md\)$/);
  const unknown = await streamModule.handleCodexStream(post({ prompt: "x", attachments: ["att_cccccccccccccccc"] }), fakeEngine(), { attachments: () => store(paths) });
  assert.equal(unknown.status, 400);
  const path = await streamModule.handleCodexStream(post({ prompt: "x", attachments: ["/etc/passwd"] }), fakeEngine(), { attachments: () => store(paths) });
  assert.equal(path.status, 400);
});

test("the body allowlist stays strict: unknown fields, a screen preview, a bad prompt or a bad thread id are all refused before Codex is touched", async () => {
  for (const body of [
    { prompt: "x", cwd: "/etc" },
    { prompt: "x", screenPreviewId: "A".repeat(22) },
    { prompt: "x", sandbox: "danger-full-access" },
    { prompt: "   " },
    { prompt: "x".repeat(8_001) },
    { prompt: 5 },
    { prompt: "x", threadId: "../x" },
    { prompt: "x", threadId: 5 },
    { prompt: "x", conversationId: "nope" },
    { prompt: "x", revision: -1 },
    { prompt: "x", model: "a b" },
    { prompt: "x", attachments: "att_aaaaaaaaaaaaaaaa" },
  ]) {
    const engine = fakeEngine();
    const response = await streamModule.handleCodexStream(post(body), engine, { attachments: () => null });
    assert.equal(response.status, 400, JSON.stringify(body));
    assert.equal(engine.calls.thread.length + engine.calls.turn.length, 0);
  }
});

test("a turn that fails, was stopped or cannot start ends with one fixed Spanish error, never Codex's raw text", async () => {
  const failed = await streamModule.handleCodexStream(post({ prompt: "x" }), fakeEngine({ runTurn: async () => ({ text: "", status: "failed", error: "Codex se reinició o dejó de responder. Vuelve a intentarlo." }) }), { attachments: () => null });
  assert.deepEqual((await frames(failed)).at(-1), ["error", { message: "Codex se reinició o dejó de responder. Vuelve a intentarlo." }]);
  const stopped = await streamModule.handleCodexStream(post({ prompt: "x" }), fakeEngine({ runTurn: async () => ({ text: "a", status: "interrupted" }) }), { attachments: () => null });
  assert.match((await frames(stopped)).at(-1)[1].message, /Detenido/);
  const thrown = await streamModule.handleCodexStream(post({ prompt: "x" }), fakeEngine({ runTurn: async () => { throw new Error("secret path /home/u/.codex"); } }), { attachments: () => null });
  const last = (await frames(thrown)).at(-1);
  assert.equal(last[0], "error");
  assert.ok(!JSON.stringify(last).includes("secret"));
  const busy = await streamModule.handleCodexStream(post({ prompt: "x" }), fakeEngine({ runTurn: async () => { throw new Error("This Codex chat already has a response in progress."); } }), { attachments: () => null });
  assert.match((await frames(busy)).at(-1)[1].message, /ya está respondiendo/);
  const open = await streamModule.handleCodexStream(post({ prompt: "x" }), fakeEngine({ startOrResumeThread: async () => { throw new Error("raw"); } }), { attachments: () => null });
  assert.equal(open.status, 502);
  assert.ok(!(await open.text()).includes("raw"));
});

test("Detener: the browser closing the stream aborts the turn's signal", async () => {
  let seen;
  const engine = fakeEngine({ runTurn: (request) => new Promise((resolve) => { seen = request.signal; request.signal.addEventListener("abort", () => resolve({ text: "", status: "interrupted" })); }) });
  const abort = new AbortController();
  const response = await streamModule.handleCodexStream(post({ prompt: "x" }, { signal: abort.signal }), engine, { attachments: () => null });
  const reader = response.body.getReader();
  await reader.read();
  assert.equal(seen.aborted, false);
  await reader.cancel();
  await settle();
  assert.equal(seen.aborted, true);
});

// The switch in the route ------------------------------------------------------------------------------------------

function fakeTransport(state, account) {
  const calls = [];
  const listeners = new Set();
  const transport = {
    calls,
    request: async (method, params) => {
      calls.push({ method, params });
      if (method === "account/read") return account;
      if (method === "thread/start") return { thread: { id: THREAD } };
      if (method === "turn/start") {
        setImmediate(() => {
          for (const listener of listeners) listener({ method: "item/agentMessage/delta", params: { threadId: THREAD, turnId: TURN, itemId: "m1", delta: "Hola desde Codex" } });
          for (const listener of listeners) listener({ method: "turn/completed", params: { threadId: THREAD, turn: { id: TURN, status: "completed", error: null } } });
        });
        return { turn: { id: TURN, status: "inProgress", items: [] } };
      }
      throw new Error(`unexpected ${method}`);
    },
    notify: () => {},
    onNotification: (listener) => { listeners.add(listener); return () => listeners.delete(listener); },
    setRequestHandler: () => {},
    status: () => ({ state, restarts: 0, ...(state === "ready" ? {} : { message: "Dot no encontró Codex en este equipo." }) }),
    onStatus: () => () => {},
    onRestart: () => () => {},
  };
  return transport;
}

async function withTransport(transport, run) {
  const saved = globalThis[BRIDGE];
  globalThis[BRIDGE] = transport;
  try { await run(); } finally { if (saved === undefined) delete globalThis[BRIDGE]; else globalThis[BRIDGE] = saved; }
}

const signedIn = { account: { type: "chatgpt", email: "a@b.c", planType: "plus" }, requiresOpenaiAuth: true, workspaceRouting: null };
const signedOut = { account: null, requiresOpenaiAuth: true, workspaceRouting: null };
const builtinBody = { conversationId: CONVERSATION, revision: 0, model: "model-first", prompt: "hola" };

test("with Codex ready and signed in the route answers from Codex without Dot's own sign-in or local history", async () => {
  const transport = fakeTransport("ready", signedIn);
  await withTransport(transport, async () => {
    const response = await route.POST(post(builtinBody));
    assert.equal(response.status, 200);
    const events = await frames(response);
    assert.deepEqual(events.map(([type]) => type), ["thread", "delta", "completed"]);
    assert.equal(events[1][1].text, "Hola desde Codex");
    assert.deepEqual(transport.calls.find((call) => call.method === "thread/start").params.approvalPolicy, "untrusted");
  });
});

test("with Codex missing, unsupported, restarting or signed out the route is the built-in one, unchanged", async () => {
  // No Dot sign-in exists in this test, so the built-in route's own answer is its 401, and it never calls Codex.
  for (const [state, account] of [["unavailable", signedIn], ["starting", signedIn], ["restarting", signedIn], ["ready", signedOut]]) {
    const transport = fakeTransport(state, account);
    await withTransport(transport, async () => {
      const response = await route.POST(post(builtinBody));
      assert.equal(response.status, 401, `${state}`);
      assert.deepEqual(await response.json(), { error: "Reconnect with ChatGPT to use chat." });
      assert.ok(!transport.calls.some((call) => call.method === "thread/start" || call.method === "turn/start"));
    });
  }
  const saved = globalThis[BRIDGE];
  delete globalThis[BRIDGE];
  try {
    assert.equal((await route.POST(post(builtinBody))).status, 401, "no Codex integration at all");
    assert.equal((await route.POST(post({ ...builtinBody, threadId: THREAD }))).status, 401);
  } finally { if (saved !== undefined) globalThis[BRIDGE] = saved; }
});
