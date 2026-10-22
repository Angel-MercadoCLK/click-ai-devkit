import assert from "node:assert/strict";
import { test } from "node:test";

const engineModule = await import("../lib/codex-engine.ts");

const tick = () => new Promise((resolve) => setImmediate(resolve));
const settle = async (times = 4) => { for (let i = 0; i < times; i += 1) await tick(); };

function fakeTransport() {
  const calls = [];
  const listeners = new Set();
  const statusListeners = new Set();
  const restartListeners = new Set();
  const responders = new Map();
  let handler = null;
  let state = "ready";
  const transport = {
    calls,
    request: async (method, params, options) => {
      calls.push({ method, params, options });
      const responder = responders.get(method);
      if (!responder) throw new Error(`unexpected ${method}`);
      return responder(params, options);
    },
    notify: () => {},
    onNotification: (listener) => { listeners.add(listener); return () => listeners.delete(listener); },
    setRequestHandler: (next) => { handler = next; },
    status: () => ({ state, restarts: 0, ...(state === "ready" ? {} : { message: "Codex no está disponible." }) }),
    onStatus: (listener) => { statusListeners.add(listener); return () => statusListeners.delete(listener); },
    onRestart: (listener) => { restartListeners.add(listener); return () => restartListeners.delete(listener); },
  };
  return {
    transport,
    calls,
    on: (method, responder) => responders.set(method, typeof responder === "function" ? responder : () => responder),
    notify: (method, params) => { for (const listener of [...listeners]) listener({ method, params }); },
    ask: (method, params, id = 1) => handler({ id, method, params }),
    hasHandler: () => handler !== null,
    setState: (next) => { state = next; for (const listener of [...statusListeners]) listener({ state: next, restarts: 0 }); },
    restarted: () => { for (const listener of [...restartListeners]) listener(); },
    methods: () => calls.map((call) => call.method),
    last: (method) => calls.filter((call) => call.method === method).at(-1),
  };
}

/** A broker whose answers the test supplies, one per request, after it has seen the request. */
function fakeBroker() {
  const requests = [];
  const waiting = [];
  return {
    requests,
    requestCodexApproval: (input) => new Promise((resolve) => { requests.push(input); waiting.push(resolve); }),
    answer: (value, index = 0) => waiting[index]({ ...value }),
  };
}

function setup(options = {}) {
  const fake = fakeTransport();
  const broker = fakeBroker();
  const timers = [];
  const engine = engineModule.createCodexEngine(fake.transport, {
    broker,
    setTimer: (callback, ms) => { const timer = { callback, ms }; timers.push(timer); return timer; },
    clearTimer: (timer) => { const index = timers.indexOf(timer); if (index >= 0) timers.splice(index, 1); },
    ...options,
  });
  return { fake, broker, engine, timers };
}

const THREAD = "01a165bf-926d-79f0-9b12-3ce7cd822aea";
const TURN = "turn-1";
const thread = (extra = {}) => ({ id: THREAD, name: null, preview: "Hola Codex", updatedAt: 1_700_000_000, cwd: "/home/u/proj", ...extra });
const turnStart = (fake) => fake.on("turn/start", { turn: { id: TURN, status: "inProgress", items: [] } });
const item = (value, extra = {}) => ({ threadId: THREAD, turnId: TURN, item: value, ...extra });
const completed = (status = "completed", extra = {}) => ({ threadId: THREAD, turn: { id: TURN, status, error: null, ...extra } });

/** Runs a turn to the point where it is listening, returns the pieces a test drives. */
async function startTurn(env, extra = {}) {
  turnStart(env.fake);
  const events = [];
  const abort = new AbortController();
  const result = env.engine.runTurn({ threadId: THREAD, text: "hola", localImages: [], signal: abort.signal, emit: (event) => events.push(event), ...extra });
  await settle();
  return { events, abort, result };
}

// Accessor ---------------------------------------------------------------------------------------------------

test("getCodexEngine reads the click-dot.codex-engine bridge, validates it, and returns one engine for it", () => {
  const holder = globalThis;
  const key = Symbol.for("click-dot.codex-engine");
  const saved = holder[key];
  try {
    delete holder[key];
    assert.equal(engineModule.getCodexEngine(), null, "no bridge means no engine");
    holder[key] = { request() {} };
    assert.equal(engineModule.getCodexEngine(), null, "a malformed bridge means no engine");
    const { transport } = fakeTransport();
    holder[key] = transport;
    const engine = engineModule.getCodexEngine();
    assert.notEqual(engine, null);
    assert.equal(engineModule.getCodexEngine(), engine);
    assert.equal(engineModule.CODEX_ENGINE_KEY, key);
  } finally {
    if (saved === undefined) delete holder[key]; else holder[key] = saved;
  }
});

// Account ----------------------------------------------------------------------------------------------------

test("accountStatus tells signed in (ChatGPT or API key), signed out, and unavailable", async () => {
  const { fake, engine } = setup();
  fake.on("account/read", { account: { type: "chatgpt", email: "a@b.c", planType: "plus" }, requiresOpenaiAuth: true, workspaceRouting: null });
  assert.deepEqual(await engine.accountStatus(), { state: "signedIn", method: "chatgpt", email: "a@b.c", plan: "plus" });
  assert.deepEqual(fake.last("account/read").params, { refreshToken: false });
  fake.on("account/read", { account: { type: "apiKey" }, requiresOpenaiAuth: true, workspaceRouting: null });
  assert.deepEqual(await engine.accountStatus(), { state: "signedIn", method: "apiKey", email: null, plan: null });
  fake.on("account/read", { account: null, requiresOpenaiAuth: true, workspaceRouting: null });
  assert.deepEqual(await engine.accountStatus(), { state: "signedOut" });
  fake.on("account/read", () => { throw new Error("boom"); });
  assert.equal((await engine.accountStatus()).state, "unavailable");
  fake.setState("restarting");
  assert.deepEqual(await engine.accountStatus(), { state: "unavailable", message: "Codex no está disponible." });
});

test("startLogin begins the ChatGPT sign-in and waitForLogin ends with its completion notification", async () => {
  const { fake, engine } = setup();
  fake.on("account/login/start", { type: "chatgpt", loginId: "L1", authUrl: "https://auth.example/login" });
  assert.deepEqual(await engine.startLogin(), { loginId: "L1", authUrl: "https://auth.example/login" });
  assert.deepEqual(fake.last("account/login/start").params, { type: "chatgpt" });
  const waiting = engine.waitForLogin("L1", new AbortController().signal);
  fake.notify("account/login/completed", { loginId: "other", success: true, error: null });
  fake.notify("account/login/completed", { loginId: "L1", success: false, error: "cancelled" });
  assert.deepEqual(await waiting, { success: false, error: "cancelled" });
  const abort = new AbortController();
  const aborted = engine.waitForLogin("L2", abort.signal);
  abort.abort();
  assert.deepEqual(await aborted, { success: false, error: "cancelled" });
});

// Threads ----------------------------------------------------------------------------------------------------

test("a new thread starts in the chosen folder asking for approval unless trusted, with workspace-write", async () => {
  const { fake, engine } = setup();
  fake.on("thread/start", { thread: thread(), approvalPolicy: "untrusted" });
  assert.deepEqual(await engine.startOrResumeThread({ cwd: "/home/u/proj" }), { threadId: THREAD, resumed: false });
  assert.deepEqual(fake.last("thread/start").params, { cwd: "/home/u/proj", approvalPolicy: "untrusted", sandbox: "workspace-write" });
  assert.equal(engineModule.CODEX_APPROVAL_POLICY, "untrusted");
  assert.equal(engineModule.CODEX_SANDBOX_MODE, "workspace-write");
});

test("an existing thread resumes once per app-server process, with the same safety settings, and again after a restart", async () => {
  const { fake, engine } = setup();
  fake.on("thread/resume", { thread: thread() });
  assert.deepEqual(await engine.startOrResumeThread({ threadId: THREAD, cwd: "/home/u/proj" }), { threadId: THREAD, resumed: true });
  assert.deepEqual(fake.last("thread/resume").params, { threadId: THREAD, cwd: "/home/u/proj", approvalPolicy: "untrusted", sandbox: "workspace-write", excludeTurns: true });
  await engine.startOrResumeThread({ threadId: THREAD, cwd: "/home/u/proj" });
  assert.equal(fake.methods().filter((method) => method === "thread/resume").length, 1);
  fake.restarted();
  await engine.startOrResumeThread({ threadId: THREAD, cwd: "/home/u/proj" });
  assert.equal(fake.methods().filter((method) => method === "thread/resume").length, 2);
});

test("a thread that was never used has no rollout to resume, so a fresh one starts instead", async () => {
  const { fake, engine } = setup();
  fake.on("thread/resume", () => { throw Object.assign(new Error(`no rollout found for thread id ${THREAD}`), { code: -32600 }); });
  fake.on("thread/start", { thread: thread({ id: "fresh" }) });
  assert.deepEqual(await engine.startOrResumeThread({ threadId: THREAD, cwd: "/home/u/proj" }), { threadId: "fresh", resumed: false });
  fake.on("thread/resume", () => { throw new Error("disk exploded"); });
  await assert.rejects(engine.startOrResumeThread({ threadId: "another", cwd: "/x" }), /disk exploded/);
});

// Streaming --------------------------------------------------------------------------------------------------

test("the user's text and images go in as text and localImage inputs", async () => {
  const env = setup();
  await startTurn(env, { text: "mira esto /home/u/a.pdf", localImages: ["/tmp/a.png", "/tmp/b.png"] });
  assert.deepEqual(env.fake.last("turn/start").params, {
    threadId: THREAD,
    input: [
      { type: "text", text: "mira esto /home/u/a.pdf", text_elements: [] },
      { type: "localImage", path: "/tmp/a.png" },
      { type: "localImage", path: "/tmp/b.png" },
    ],
  });
});

test("agent message deltas stream as delta events, messages are separated, and the final text is the last answer", async () => {
  const env = setup();
  const { events, result } = await startTurn(env);
  env.fake.notify("item/agentMessage/delta", { threadId: THREAD, turnId: TURN, itemId: "m1", delta: "Voy a " });
  env.fake.notify("item/agentMessage/delta", { threadId: THREAD, turnId: TURN, itemId: "m1", delta: "revisar." });
  env.fake.notify("item/completed", item({ type: "agentMessage", id: "m1", text: "Voy a revisar.", phase: "commentary" }));
  env.fake.notify("item/agentMessage/delta", { threadId: THREAD, turnId: TURN, itemId: "m2", delta: "Listo." });
  env.fake.notify("item/completed", item({ type: "agentMessage", id: "m2", text: "Listo.", phase: "final_answer" }));
  env.fake.notify("turn/completed", completed());
  assert.deepEqual(await result, { text: "Listo.", status: "completed" });
  assert.deepEqual(events, [
    { type: "delta", text: "Voy a " },
    { type: "delta", text: "revisar." },
    { type: "delta", text: "\n\n" },
    { type: "delta", text: "Listo." },
  ]);
});

test("a message that never streamed is emitted whole when it completes, and notifications for other threads or turns are ignored", async () => {
  const env = setup();
  const { events, result } = await startTurn(env);
  env.fake.notify("item/agentMessage/delta", { threadId: "other-thread", turnId: TURN, itemId: "x", delta: "NO" });
  env.fake.notify("item/agentMessage/delta", { threadId: THREAD, turnId: "old-turn", itemId: "x", delta: "NO" });
  env.fake.notify("turn/completed", { threadId: THREAD, turn: { id: "old-turn", status: "completed", error: null } });
  env.fake.notify("item/completed", item({ type: "agentMessage", id: "m1", text: "Hola completo", phase: null }));
  env.fake.notify("turn/completed", completed());
  assert.deepEqual(await result, { text: "Hola completo", status: "completed" });
  assert.deepEqual(events, [{ type: "delta", text: "Hola completo" }]);
});

test("a failed turn reports its error and an Codex that goes away mid-turn fails the turn", async () => {
  const failed = setup();
  const one = await startTurn(failed);
  failed.fake.notify("turn/completed", completed("failed", { error: { message: "rate limited", codexErrorInfo: null, additionalDetails: null, misalignment: null } }));
  assert.deepEqual(await one.result, { text: "", status: "failed", error: "rate limited" });

  const crashed = setup();
  const two = await startTurn(crashed);
  crashed.fake.setState("restarting");
  const result = await two.result;
  assert.equal(result.status, "failed");
  assert.match(result.error, /Codex/);
});

test("a transport error starting the turn is thrown, and a second turn on the same thread is refused", async () => {
  const env = setup();
  env.fake.on("turn/start", () => { throw new Error("thread not found"); });
  await assert.rejects(env.engine.runTurn({ threadId: THREAD, text: "x", localImages: [], signal: new AbortController().signal, emit() {} }), /thread not found/);
  const busy = setup();
  await startTurn(busy);
  await assert.rejects(busy.engine.runTurn({ threadId: THREAD, text: "y", localImages: [], signal: new AbortController().signal, emit() {} }), /already/);
});

// Tool events ------------------------------------------------------------------------------------------------

const toolEvents = (events) => events.filter((event) => event.type === "tool").map((event) => event.tool);

test("a command execution is a tool row: running, then done with the end of its output, or error with the exit code, or denied", async () => {
  const env = setup();
  const { events, result } = await startTurn(env);
  const command = (extra) => item({ type: "commandExecution", id: "c1", command: "npm   test", cwd: "/p", status: "inProgress", commandActions: [], aggregatedOutput: null, exitCode: null, durationMs: null, processId: null, source: "agent", ...extra });
  env.fake.notify("item/started", command({}));
  env.fake.notify("item/completed", command({ status: "completed", exitCode: 0, aggregatedOutput: `${"x".repeat(900)}\nall tests passed\n` }));
  env.fake.notify("item/completed", item({ ...command({ status: "completed", exitCode: 2, aggregatedOutput: "boom" }).item, id: "c2" }));
  env.fake.notify("item/completed", item({ ...command({ status: "declined" }).item, id: "c3" }));
  env.fake.notify("turn/completed", completed());
  await result;
  const rows = toolEvents(events);
  assert.deepEqual(rows[0], { id: "c1", name: engineModule.CODEX_TOOL_NAME, summary: "npm test", status: "running" });
  assert.equal(rows[1].status, "done");
  assert.equal(rows[1].detail.length <= 600, true);
  assert.match(rows[1].detail, /all tests passed$/);
  assert.deepEqual([rows[2].id, rows[2].status, rows[2].detail], ["c2", "error", "Código de salida 2\nboom"]);
  assert.deepEqual([rows[3].id, rows[3].status], ["c3", "denied"]);
});

test("a file change is a tool row naming the files", async () => {
  const env = setup();
  const { events, result } = await startTurn(env);
  const change = (status) => item({ type: "fileChange", id: "f1", status, changes: ["/p/src/a.ts", "/p/src/b.ts", "/p/c.md", "/p/d.md"].map((path) => ({ path, kind: { type: "update", move_path: null }, diff: "" })) });
  env.fake.notify("item/started", change("inProgress"));
  env.fake.notify("item/completed", change("completed"));
  env.fake.notify("item/completed", item({ ...change("failed").item, id: "f2" }));
  env.fake.notify("turn/completed", completed());
  await result;
  const rows = toolEvents(events);
  assert.deepEqual(rows.map((row) => [row.id, row.status]), [["f1", "running"], ["f1", "done"], ["f2", "error"]]);
  assert.equal(rows[0].summary, "Editando a.ts, b.ts, c.md y 1 más");
  assert.match(rows[1].detail, /\/p\/src\/a\.ts/);
});

test("an MCP tool call is a tool row, and Engram's tools read as Consultando memoria… without echoing what they returned", async () => {
  const env = setup();
  const { events, result } = await startTurn(env);
  const call = (id, server, tool, status, extra = {}) => item({ type: "mcpToolCall", id, server, tool, status, arguments: {}, result: null, error: null, durationMs: null, ...extra });
  env.fake.notify("item/started", call("t1", "engram", "mem_search", "inProgress"));
  env.fake.notify("item/completed", call("t1", "engram", "mem_search", "completed", { result: { content: [{ type: "text", text: "secret memory" }] } }));
  env.fake.notify("item/started", call("t2", "playwright", "browser_navigate", "inProgress"));
  env.fake.notify("item/completed", call("t2", "playwright", "browser_navigate", "completed", { result: { content: [{ type: "text", text: "Page loaded" }, { type: "image", data: "xx" }] } }));
  env.fake.notify("item/completed", call("t3", "dot", "delegate_to_claude", "failed", { error: { message: "no claude" } }));
  env.fake.notify("item/started", call("t4", "other", "mem_save", "inProgress"));
  env.fake.notify("turn/completed", completed());
  await result;
  const rows = toolEvents(events);
  assert.deepEqual(rows[0], { id: "t1", name: engineModule.CODEX_TOOL_NAME, summary: "Consultando memoria…", status: "running" });
  assert.deepEqual(rows[1], { id: "t1", name: engineModule.CODEX_TOOL_NAME, summary: "Consultando memoria…", status: "done" });
  assert.deepEqual([rows[2].summary, rows[3].summary, rows[3].detail], ["playwright: browser_navigate", "playwright: browser_navigate", "Page loaded"]);
  assert.deepEqual([rows[4].status, rows[4].detail], ["error", "no claude"]);
  assert.equal(rows[5].summary, "Consultando memoria…", "the mem_* tools of any server are memory");
});

// Approvals --------------------------------------------------------------------------------------------------

const commandRequest = (extra = {}) => ({ kind: "command", threadId: THREAD, turnId: TURN, itemId: "c1", startedAtMs: 1, environmentId: null, command: "rm -rf build", cwd: "/p", reason: "limpiar", availableDecisions: ["accept", "acceptForSession", "decline", "cancel"], ...extra });

test("a command approval becomes a codex card with the command, folder and reason, and Permitir/Siempre/Denegar answer accept/acceptForSession/decline", async () => {
  for (const [choice, decision] of [["allow", "accept"], ["always", "acceptForSession"], ["deny", "decline"]]) {
    const env = setup();
    const { events, result } = await startTurn(env);
    env.fake.notify("item/started", item({ type: "commandExecution", id: "c1", command: "rm -rf build", cwd: "/p", status: "inProgress", commandActions: [], aggregatedOutput: null, exitCode: null, durationMs: null, processId: null, source: "agent" }));
    const reply = env.fake.ask("item/commandExecution/requestApproval", commandRequest());
    await settle();
    assert.equal(env.broker.requests.length, 1);
    const request = env.broker.requests[0];
    assert.equal(request.tool, "Codex");
    assert.equal(request.summary, "rm -rf build");
    assert.match(request.detail, /rm -rf build/);
    assert.match(request.detail, /Carpeta: \/p/);
    assert.match(request.detail, /limpiar/);
    assert.equal(request.alwaysOffered, true);
    assert.equal(request.signal instanceof AbortSignal, true);
    assert.equal(toolEvents(events).at(-1).status, "waiting_approval");
    env.broker.answer({ choice });
    assert.deepEqual(await reply, { decision });
    await settle();
    assert.equal(toolEvents(events).at(-1).status, choice === "deny" ? "denied" : "running");
    env.fake.notify("turn/completed", completed());
    await result;
  }
});

test("Siempre is offered for a command only when acceptForSession is in availableDecisions", async () => {
  for (const [decisions, offered] of [[["accept", "decline"], false], [["accept", "acceptForSession", "decline"], true], [undefined, false], [null, false]]) {
    const env = setup();
    await startTurn(env);
    const reply = env.fake.ask("item/commandExecution/requestApproval", commandRequest({ availableDecisions: decisions }));
    await settle();
    assert.equal(env.broker.requests[0].alwaysOffered, offered, JSON.stringify(decisions));
    env.broker.answer({ choice: "allow" });
    await reply;
  }
});

test("a timed out, dismissed or unanswered card answers decline, and the card is withdrawn when the turn is stopped", async () => {
  const env = setup();
  const { abort, result } = await startTurn(env);
  const reply = env.fake.ask("item/commandExecution/requestApproval", commandRequest());
  await settle();
  env.broker.answer({ choice: "none" });
  assert.deepEqual(await reply, { decision: "decline" });
  const second = env.fake.ask("item/fileChange/requestApproval", { threadId: THREAD, turnId: TURN, itemId: "f1", startedAtMs: 1, reason: null, grantRoot: null }, 2);
  await settle();
  const signal = env.broker.requests[1].signal;
  assert.equal(signal.aborted, false);
  abort.abort();
  assert.equal(signal.aborted, true);
  env.broker.answer({ choice: "none" }, 1);
  assert.deepEqual(await second, { decision: "decline" });
  env.fake.notify("turn/completed", completed("interrupted"));
  await result;
});

test("a request the server resolves itself withdraws its card", async () => {
  const env = setup();
  await startTurn(env);
  env.fake.ask("item/commandExecution/requestApproval", commandRequest(), 77);
  await settle();
  const { signal } = env.broker.requests[0];
  env.fake.notify("serverRequest/resolved", { threadId: THREAD, requestId: 77 });
  assert.equal(signal.aborted, true);
});

test("a file change approval offers Siempre and answers accept, acceptForSession or decline", async () => {
  for (const [choice, decision] of [["allow", "accept"], ["always", "acceptForSession"], ["deny", "decline"]]) {
    const env = setup();
    await startTurn(env);
    env.fake.notify("item/started", item({ type: "fileChange", id: "f1", status: "inProgress", changes: [{ path: "/p/a.ts", kind: { type: "add" }, diff: "" }] }));
    const reply = env.fake.ask("item/fileChange/requestApproval", { threadId: THREAD, turnId: TURN, itemId: "f1", startedAtMs: 1, reason: "necesito escribir", grantRoot: "/p" });
    await settle();
    const request = env.broker.requests[0];
    assert.equal(request.summary, "Editando a.ts");
    assert.match(request.detail, /necesito escribir/);
    assert.match(request.detail, /\/p\/a\.ts/);
    assert.equal(request.alwaysOffered, true);
    env.broker.answer({ choice });
    assert.deepEqual(await reply, { decision });
  }
});

test("a permissions approval grants exactly what was asked for the turn or the session, and nothing when denied", async () => {
  const permissions = { network: { enabled: true }, fileSystem: { read: null, write: ["/data"] } };
  const expected = { network: { enabled: true }, fileSystem: { read: null, write: ["/data"] } };
  for (const [choice, answer] of [["allow", { permissions: expected, scope: "turn" }], ["always", { permissions: expected, scope: "session" }], ["deny", { permissions: {}, scope: "turn" }]]) {
    const env = setup();
    await startTurn(env);
    const reply = env.fake.ask("item/permissions/requestApproval", { threadId: THREAD, turnId: TURN, itemId: "p1", environmentId: null, startedAtMs: 1, cwd: "/p", reason: "descargar datos", permissions });
    await settle();
    const request = env.broker.requests[0];
    assert.match(request.summary, /permisos/i);
    assert.match(request.detail, /descargar datos/);
    assert.match(request.detail, /red/i);
    assert.match(request.detail, /\/data/);
    assert.equal(request.alwaysOffered, true);
    env.broker.answer({ choice });
    assert.deepEqual(await reply, answer);
  }
  const env = setup();
  await startTurn(env);
  const reply = env.fake.ask("item/permissions/requestApproval", { threadId: THREAD, turnId: TURN, itemId: "p2", environmentId: null, startedAtMs: 1, cwd: "/p", reason: null, permissions: { network: null, fileSystem: null } });
  await settle();
  env.broker.answer({ choice: "allow" });
  assert.deepEqual(await reply, { permissions: {}, scope: "turn" }, "null permissions are not granted");
});

const userInput = (extra = {}) => ({
  threadId: THREAD, turnId: TURN, itemId: "q1", isBlocking: true, autoResolutionMs: null,
  questions: [
    { id: "db", header: "DB", question: "¿Qué base de datos?", isOther: false, isSecret: false, options: [{ label: "Postgres", description: "SQL" }, { label: "SQLite", description: "archivo" }] },
    { id: "ui", header: "UI", question: "¿Qué interfaz?", isOther: false, isSecret: false, options: [{ label: "Web", description: "" }] },
  ],
  ...extra,
});

test("questions from Codex become a card with options and come back keyed by question id", async () => {
  const env = setup();
  await startTurn(env);
  const reply = env.fake.ask("item/tool/requestUserInput", userInput());
  await settle();
  const request = env.broker.requests[0];
  assert.equal(request.summary, "Codex te hace una pregunta");
  assert.equal(request.alwaysOffered, false);
  assert.deepEqual(request.questions, [
    { question: "¿Qué base de datos?", header: "DB", multiSelect: false, options: [{ label: "Postgres", description: "SQL" }, { label: "SQLite", description: "archivo" }] },
    { question: "¿Qué interfaz?", header: "UI", multiSelect: false, options: [{ label: "Web", description: "" }] },
  ]);
  env.broker.answer({ choice: "allow", answers: { "¿Qué base de datos?": "SQLite", "¿Qué interfaz?": "Web" } });
  assert.deepEqual(await reply, { answers: { db: { answers: ["SQLite"] }, ui: { answers: ["Web"] } } });
});

test("declining questions answers with none, and questions that cannot be shown exactly are declined without a card", async () => {
  const env = setup();
  await startTurn(env);
  const declined = env.fake.ask("item/tool/requestUserInput", userInput());
  await settle();
  env.broker.answer({ choice: "deny" });
  assert.deepEqual(await declined, { answers: {} });
  for (const questions of [
    [{ id: "a", header: "", question: "Libre", isOther: true, isSecret: false, options: null }],
    [{ id: "a", header: "", question: "Secreta", isOther: false, isSecret: true, options: [{ label: "x", description: "" }] }],
    [{ id: "a", header: "", question: "Igual", isOther: false, isSecret: false, options: [{ label: "x", description: "" }] }, { id: "b", header: "", question: "Igual", isOther: false, isSecret: false, options: [{ label: "y", description: "" }] }],
    [],
  ]) {
    const before = env.broker.requests.length;
    assert.deepEqual(await env.fake.ask("item/tool/requestUserInput", userInput({ questions }), 9), { answers: {} });
    assert.equal(env.broker.requests.length, before);
  }
});

test("an MCP tool confirmation offers Siempre only when Codex offers session persistence, and a form that needs input is declined", async () => {
  const toolApproval = (persist) => ({ threadId: THREAD, turnId: TURN, serverName: "playwright", mode: "form", message: 'Allow the playwright MCP server to run tool "browser_click"?', requestedSchema: { type: "object", properties: {} }, _meta: { codex_approval_kind: "mcp_tool_call", ...(persist ? { persist } : {}) } });
  for (const [choice, answer] of [["allow", { action: "accept", content: null, _meta: null }], ["always", { action: "accept", content: null, _meta: { persist: "session" } }], ["deny", { action: "decline", content: null, _meta: null }]]) {
    const env = setup();
    await startTurn(env);
    const reply = env.fake.ask("mcpServer/elicitation/request", toolApproval(["session", "always"]));
    await settle();
    const request = env.broker.requests[0];
    assert.equal(request.alwaysOffered, true);
    assert.match(request.summary, /playwright/);
    assert.match(request.detail, /browser_click/);
    env.broker.answer({ choice });
    assert.deepEqual(await reply, answer);
  }
  const without = setup();
  await startTurn(without);
  const reply = without.fake.ask("mcpServer/elicitation/request", toolApproval(undefined));
  await settle();
  assert.equal(without.broker.requests[0].alwaysOffered, false);
  without.broker.answer({ choice: "none" });
  assert.deepEqual(await reply, { action: "decline", content: null, _meta: null });

  const plain = setup();
  await startTurn(plain);
  const generic = plain.fake.ask("mcpServer/elicitation/request", { threadId: THREAD, turnId: TURN, serverName: "x", mode: "form", message: "¿Seguro?", requestedSchema: { type: "object", properties: {} }, _meta: null });
  await settle();
  plain.broker.answer({ choice: "allow" });
  assert.deepEqual(await generic, { action: "accept", content: {}, _meta: null });

  const needsInput = setup();
  await startTurn(needsInput);
  const form = await needsInput.fake.ask("mcpServer/elicitation/request", { threadId: THREAD, turnId: TURN, serverName: "x", mode: "form", message: "Dame tu nombre", requestedSchema: { type: "object", properties: { name: { type: "string" } }, required: ["name"] }, _meta: null });
  assert.deepEqual(form, { action: "decline", content: null, _meta: null });
  const url = await needsInput.fake.ask("mcpServer/elicitation/request", { threadId: THREAD, turnId: TURN, serverName: "x", mode: "url", message: "Abre", url: "https://x", elicitationId: "e", _meta: null }, 5);
  assert.deepEqual(url, { action: "decline", content: null, _meta: null });
  assert.equal(needsInput.broker.requests.length, 0);
});

test("a server request Dot does not handle is refused so Codex is never left waiting on it", async () => {
  const { fake } = setup();
  await assert.rejects(Promise.resolve().then(() => fake.ask("item/tool/call", { threadId: THREAD })), /item\/tool\/call/);
  assert.equal(fake.hasHandler(), true);
});

// Interrupt --------------------------------------------------------------------------------------------------

test("Detener sends turn/interrupt for the running turn and ends with the interrupted turn", async () => {
  const env = setup();
  env.fake.on("turn/interrupt", {});
  const { abort, result } = await startTurn(env);
  abort.abort();
  await settle();
  assert.deepEqual(env.fake.last("turn/interrupt").params, { threadId: THREAD, turnId: TURN });
  env.fake.notify("item/agentMessage/delta", { threadId: THREAD, turnId: TURN, itemId: "m", delta: "a medias" });
  env.fake.notify("turn/completed", completed("interrupted"));
  assert.deepEqual(await result, { text: "a medias", status: "interrupted" });
});

test("stopping before the turn id is known interrupts as soon as it is, and a stop Codex never confirms still ends the turn", async () => {
  const env = setup();
  env.fake.on("turn/interrupt", {});
  let release;
  env.fake.on("turn/start", () => new Promise((resolve) => { release = () => resolve({ turn: { id: TURN, status: "inProgress", items: [] } }); }));
  const abort = new AbortController();
  const result = env.engine.runTurn({ threadId: THREAD, text: "x", localImages: [], signal: abort.signal, emit() {} });
  await settle();
  abort.abort();
  release();
  await settle();
  assert.deepEqual(env.fake.last("turn/interrupt").params, { threadId: THREAD, turnId: TURN });
  const grace = env.timers.find((timer) => timer.ms === engineModule.INTERRUPT_GRACE_MS);
  assert.notEqual(grace, undefined);
  grace.callback();
  assert.deepEqual(await result, { text: "", status: "interrupted" });
});

test("a signal that is already aborted never starts a turn", async () => {
  const env = setup();
  const abort = new AbortController();
  abort.abort();
  assert.deepEqual(await env.engine.runTurn({ threadId: THREAD, text: "x", localImages: [], signal: abort.signal, emit() {} }), { text: "", status: "interrupted" });
  assert.equal(env.fake.methods().includes("turn/start"), false);
});

// History ----------------------------------------------------------------------------------------------------

test("listThreads asks for the newest first, searchable and paged, and maps each thread to a small view", async () => {
  const { fake, engine } = setup();
  fake.on("thread/list", { data: [thread({ name: "Mi chat" }), thread({ id: "t2", name: null, preview: "  primera\n línea  larga ", updatedAt: 5 }), thread({ id: "t3", name: null, preview: "", cwd: "/x" })], nextCursor: "next-1", backwardsCursor: null });
  const page = await engine.listThreads({ cursor: "c0", searchTerm: "chat" });
  assert.deepEqual(fake.last("thread/list").params, { cursor: "c0", searchTerm: "chat", limit: 30, sortKey: "updated_at", sortDirection: "desc", archived: false });
  assert.deepEqual(page, {
    threads: [
      { id: THREAD, title: "Mi chat", updatedAt: 1_700_000_000_000, cwd: "/home/u/proj" },
      { id: "t2", title: "primera línea larga", updatedAt: 5_000, cwd: "/home/u/proj" },
      { id: "t3", title: "Chat sin título", updatedAt: 1_700_000_000_000, cwd: "/x" },
    ],
    nextCursor: "next-1",
  });
  await engine.listThreads({});
  assert.deepEqual(fake.last("thread/list").params, { limit: 30, sortKey: "updated_at", sortDirection: "desc", archived: false });
});

test("readThread maps one thread and, on request, its messages", async () => {
  const { fake, engine } = setup();
  fake.on("thread/read", { thread: thread({ name: "Mi chat", turns: [{ id: "u1", items: [
    { type: "userMessage", id: "i1", clientId: null, content: [{ type: "text", text: "Hola", text_elements: [] }, { type: "localImage", path: "/a.png" }] },
    { type: "reasoning", id: "i2", summary: [], content: [] },
    { type: "agentMessage", id: "i3", text: "¡Hola!", phase: "final_answer", memoryCitation: null, delivery: null, questions: null },
  ] }] }), readState: null });
  assert.deepEqual(await engine.readThread(THREAD), { id: THREAD, title: "Mi chat", updatedAt: 1_700_000_000_000, cwd: "/home/u/proj" });
  assert.deepEqual(fake.last("thread/read").params, { threadId: THREAD, includeTurns: false });
  assert.deepEqual(await engine.readThread(THREAD, { includeMessages: true }), {
    id: THREAD, title: "Mi chat", updatedAt: 1_700_000_000_000, cwd: "/home/u/proj",
    messages: [{ role: "user", text: "Hola" }, { role: "assistant", text: "¡Hola!" }],
  });
  assert.deepEqual(fake.last("thread/read").params, { threadId: THREAD, includeTurns: true });
  fake.setState("unavailable");
  await assert.rejects(engine.readThread(THREAD), /Codex/);
});

test("readThread falls back to the turns list when Codex will not hydrate turns with the thread", async () => {
  const { fake, engine } = setup();
  fake.on("thread/read", (params) => {
    if (params.includeTurns) throw Object.assign(new Error("list_turns is not supported yet"), { code: -32601 });
    return { thread: thread({ name: "Mi chat" }), readState: null };
  });
  fake.on("thread/turns/list", { data: [{ id: "u1", items: [{ type: "userMessage", id: "i1", clientId: null, content: [{ type: "text", text: "Hola", text_elements: [] }] }, { type: "agentMessage", id: "i2", text: "¡Hola!", phase: null, memoryCitation: null, delivery: null, questions: null }] }], nextCursor: null, backwardsCursor: null });
  const read = await engine.readThread(THREAD, { includeMessages: true });
  assert.deepEqual(read.messages, [{ role: "user", text: "Hola" }, { role: "assistant", text: "¡Hola!" }]);
  assert.deepEqual(fake.last("thread/turns/list").params, { threadId: THREAD, limit: 30, sortDirection: "asc", itemsView: "full" });
  fake.on("thread/read", () => { throw new Error("gone"); });
  await assert.rejects(engine.readThread(THREAD), /gone/);
});
