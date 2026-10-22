import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { test } from "node:test";

process.env.PORT = "3008";

const brokerModule = await import("../lib/agent-broker.ts");
const hookModule = await import("../lib/agent-hook.ts");
const hookRoute = await import("../app/api/agent/hook/route.ts");
const stateRoute = await import("../app/api/agent/state/route.ts");
const ackRoute = await import("../app/api/agent/ack/route.ts");
const decisionRoute = await import("../app/api/agent/decision/route.ts");

const HOST = "127.0.0.1:3008";
const TOKEN = "c".repeat(64);
const CONTROL = "d".repeat(64);
const HOOK_KEY = Symbol.for("click-dot.agent-hook");
const BROKER_KEY = Symbol.for("click-dot.agent-broker");
const CONTROL_KEY = Symbol.for("click-dot.control-token");
const WINDOW_KEY = Symbol.for("click-dot.window-controller");

const tick = (ms = 0) => new Promise((resolve) => setTimeout(resolve, ms));

function install({ ackMs = 5_000, decisionMs = 5_000, control = true } = {}) {
  const shown = [];
  Reflect.set(globalThis, HOOK_KEY, { token: TOKEN });
  if (control) Reflect.set(globalThis, CONTROL_KEY, CONTROL);
  else Reflect.deleteProperty(globalThis, CONTROL_KEY);
  Reflect.set(globalThis, WINDOW_KEY, { state: () => ({}), setMode: () => {}, moveTo: () => {}, showInactive: () => shown.push("shown") });
  const broker = brokerModule.createAgentBroker({ ackMs, decisionMs, onPending: () => globalThis[WINDOW_KEY].showInactive() });
  Reflect.set(globalThis, BROKER_KEY, broker);
  return { broker, shown };
}

function reset() {
  for (const key of [HOOK_KEY, BROKER_KEY, CONTROL_KEY, WINDOW_KEY]) Reflect.deleteProperty(globalThis, key);
}

const hookRequest = ({ body, headers = {}, raw, signal } = {}) =>
  new Request(`http://${HOST}/api/agent/hook`, {
    method: "POST",
    headers: { host: HOST, authorization: `Bearer ${TOKEN}`, "content-type": "application/json", ...headers },
    body: raw ?? JSON.stringify(body),
    signal,
  });

const envelope = (event, payload = {}, extra = {}) => ({ version: 1, agent: "claude-code", event, payload: { session_id: "s1", cwd: "C:\\work\\alpha", ...payload }, ...extra });
const permission = (payload = {}) =>
  envelope("PermissionRequest", {
    tool_name: "Bash",
    tool_input: { command: "npm test" },
    permission_suggestions: [{ type: "addRules", rules: [{ toolName: "Bash", ruleContent: "npm test" }], behavior: "allow", destination: "localSettings" }],
    ...payload,
  });

const cookie = (value = CONTROL) => ({ cookie: `click_dot_control=${value}` });
const get = (route, path, headers = {}) => route.GET(new Request(`http://${HOST}${path}`, { headers: { host: HOST, ...headers } }));
const post = (route, path, body, headers = {}) =>
  route.POST(
    new Request(`http://${HOST}${path}`, {
      method: "POST",
      headers: { host: HOST, origin: `http://${HOST}`, "content-type": "application/json", ...headers },
      body: JSON.stringify(body),
    }),
  );

// --- the hook route: who may call it ---------------------------------------------------------------------------

test("a request with an Origin header is refused, even the app's own origin, before the token is looked at", async () => {
  install();
  for (const origin of [`http://${HOST}`, "http://localhost:3008", "https://evil.example", "null"]) {
    const response = await hookRoute.POST(hookRequest({ body: envelope("Stop"), headers: { origin } }));
    assert.equal(response.status, 403, origin);
  }
  // Origin wins over a missing token, so a browser page learns nothing about the token.
  const noToken = await hookRoute.POST(hookRequest({ body: envelope("Stop"), headers: { origin: "https://evil.example", authorization: "" } }));
  assert.equal(noToken.status, 403);
  assert.deepEqual(installedBroker().state().sessions, []);
  reset();
});

test("a wrong Host is refused", async () => {
  install();
  for (const host of ["evil.example", "localhost:3008", "127.0.0.1:3009", "127.0.0.1"]) {
    assert.equal((await hookRoute.POST(hookRequest({ body: envelope("Stop"), headers: { host } }))).status, 403, host);
  }
  reset();
});

test("a missing, wrong or malformed bearer token is a 401 and nothing is recorded", async () => {
  install();
  const attempts = [
    { authorization: "" },
    { authorization: "Bearer" },
    { authorization: "Bearer " },
    { authorization: `Bearer ${"e".repeat(64)}` },
    { authorization: `Bearer ${TOKEN.slice(0, 63)}` },
    { authorization: `Bearer ${TOKEN}a` },
    { authorization: `Basic ${TOKEN}` },
    { authorization: TOKEN },
    { authorization: `Bearer ${TOKEN} extra` },
  ];
  for (const headers of attempts) {
    const response = await hookRoute.POST(hookRequest({ body: envelope("UserPromptSubmit"), headers }));
    assert.equal(response.status, 401, JSON.stringify(headers));
  }
  assert.deepEqual(installedBroker().state().sessions, []);
  reset();
});

test("without a published hook token no request is accepted", async () => {
  install();
  Reflect.deleteProperty(globalThis, HOOK_KEY);
  assert.equal((await hookRoute.POST(hookRequest({ body: envelope("Stop") }))).status, 401);
  Reflect.set(globalThis, HOOK_KEY, { token: "short" });
  assert.equal((await hookRoute.POST(hookRequest({ body: envelope("Stop"), headers: { authorization: "Bearer short" } }))).status, 401);
  reset();
});

test("the bearer comparison goes through the constant-time secret comparison", async () => {
  const source = await readFile(new URL("../lib/agent-hook.ts", import.meta.url), "utf8");
  assert.match(source, /sameSecret/);
  assert.doesNotMatch(source, /(?:===|!==)\s*match\[1\]|match\[1\]\s*(?:===|!==)/);
  assert.match(await readFile(new URL("../lib/control-token.ts", import.meta.url), "utf8"), /timingSafeEqual/);
});

// --- the hook route: size and shape ---------------------------------------------------------------------------

test("a body above 512 KiB is a 413, declared or streamed, and a body at the limit is read", async () => {
  install();
  assert.equal(hookModule.HOOK_MAX_BODY_BYTES, 512 * 1024);
  const declared = await hookRoute.POST(hookRequest({ body: envelope("Stop"), headers: { "content-length": String(hookModule.HOOK_MAX_BODY_BYTES + 1) } }));
  assert.equal(declared.status, 413);
  const streamed = await hookRoute.POST(hookRequest({ raw: JSON.stringify(envelope("Stop", { filler: "x".repeat(hookModule.HOOK_MAX_BODY_BYTES) }) ) }));
  assert.equal(streamed.status, 413);
  const padding = "x".repeat(hookModule.HOOK_MAX_BODY_BYTES - JSON.stringify(envelope("Stop", { session_id: "s1", cwd: "C:\\work\\alpha", filler: "" })).length);
  const atLimit = JSON.stringify(envelope("Stop", { filler: padding }));
  assert.equal(new TextEncoder().encode(atLimit).length, hookModule.HOOK_MAX_BODY_BYTES);
  assert.equal((await hookRoute.POST(hookRequest({ raw: atLimit }))).status, 204);
  reset();
});

test("malformed JSON, a wrong version, a wrong agent or a missing event is a 400", async () => {
  install();
  const bad = [
    { raw: "{" },
    { raw: "" },
    { raw: "[]" },
    { raw: "null" },
    { body: envelope("Stop", {}, { version: 2 }) },
    { body: envelope("Stop", {}, { version: "1" }) },
    { body: envelope("Stop", {}, { agent: "codex" }) },
    { body: { version: 1, agent: "claude-code", payload: {} } },
    { body: envelope("Not a name!") },
    { body: envelope("") },
    { body: { version: 1, agent: "claude-code", event: "Stop", payload: "x" } },
    { body: { version: 1, agent: "claude-code", event: "Stop" } },
    { body: permission({ tool_name: undefined }) },
    { body: permission({ tool_name: 5 }) },
  ];
  for (const attempt of bad) assert.equal((await hookRoute.POST(hookRequest(attempt))).status, 400, JSON.stringify(attempt));
  assert.equal((await hookRoute.POST(hookRequest({ body: envelope("Stop"), headers: { "content-type": "text/plain" } }))).status, 400);
  assert.equal(installedBroker().state().pending, null);
  reset();
});

test("only POST is exported by the hook route and the three UI routes export what they use", () => {
  assert.equal(typeof hookRoute.POST, "function");
  for (const method of ["GET", "PUT", "PATCH", "DELETE"]) assert.equal(hookRoute[method], undefined);
  assert.equal(typeof stateRoute.GET, "function");
  assert.equal(stateRoute.POST, undefined);
  for (const route of [ackRoute, decisionRoute]) {
    assert.equal(typeof route.POST, "function");
    assert.equal(route.GET, undefined);
  }
});

// --- plain events -----------------------------------------------------------------------------------------------

test("events other than PermissionRequest get 204 at once and update the sessions", async () => {
  install();
  for (const [event, payload] of [
    ["SessionStart", {}],
    ["UserPromptSubmit", {}],
    ["PreToolUse", { tool_name: "Edit", tool_input: { file_path: "C:\\work\\foo.ts", old_string: "a", new_string: "a\nb" } }],
    ["PostToolUse", {}],
    ["PostToolUseFailure", { tool_name: "Bash" }],
    ["Notification", { notification_type: "idle_prompt" }],
    ["SomethingNew", {}],
    ["Stop", {}],
  ]) {
    const response = await hookRoute.POST(hookRequest({ body: envelope(event, payload) }));
    assert.equal(response.status, 204, event);
    assert.equal(await response.text(), "");
  }
  assert.equal(installedBroker().state().sessions[0].state, "listo");
  const ended = await hookRoute.POST(hookRequest({ body: envelope("SessionEnd") }));
  assert.equal(ended.status, 204);
  assert.deepEqual(installedBroker().state().sessions, []);
  reset();
});

test("a plain event shows up as the latest action of its project", async () => {
  install();
  await hookRoute.POST(hookRequest({ body: envelope("PreToolUse", { tool_name: "Edit", tool_input: { file_path: "C:\\work\\foo.ts", old_string: "a\nb\nc", new_string: "x" } }) }));
  const [session] = installedBroker().state().sessions;
  assert.deepEqual({ project: session.project, state: session.state, action: session.action }, { project: "alpha", state: "trabajando", action: "Editando foo.ts +1 −3" });
  reset();
});

// --- PermissionRequest: hold and release -----------------------------------------------------------------------

async function pendingFor(promise) {
  await tick();
  const { pending } = installedBroker().state();
  assert.notEqual(pending, null);
  return { id: pending.id, promise };
}

test("a decision answers the held request with 200 and the decision body", async () => {
  const { shown } = install();
  const held = await pendingFor(hookRoute.POST(hookRequest({ body: permission() })));
  assert.deepEqual(shown, ["shown"], "the mascot is shown when the card is queued");
  assert.equal((await post(ackRoute, "/api/agent/ack", { id: held.id }, cookie())).status, 200);
  const decided = await post(decisionRoute, "/api/agent/decision", { id: held.id, choice: "allow" }, cookie());
  assert.equal(decided.status, 200);
  const response = await held.promise;
  assert.equal(response.status, 200);
  assert.equal(response.headers.get("content-type")?.startsWith("application/json"), true);
  assert.deepEqual(await response.json(), { behavior: "allow" });
  reset();
});

test("Denegar answers deny, and Siempre answers allow with only the first suggestion", async () => {
  install();
  const denied = await pendingFor(hookRoute.POST(hookRequest({ body: permission() })));
  await post(decisionRoute, "/api/agent/decision", { id: denied.id, choice: "deny" }, cookie());
  assert.deepEqual(await (await denied.promise).json(), { behavior: "deny" });

  const payload = permission().payload;
  const always = await pendingFor(hookRoute.POST(hookRequest({ body: permission() })));
  await post(decisionRoute, "/api/agent/decision", { id: always.id, choice: "always" }, cookie());
  assert.deepEqual(await (await always.promise).json(), { behavior: "allow", updatedPermissions: [payload.permission_suggestions[0]] });

  const bare = await pendingFor(hookRoute.POST(hookRequest({ body: permission({ permission_suggestions: [] }) })));
  const refused = await post(decisionRoute, "/api/agent/decision", { id: bare.id, choice: "always" }, cookie());
  assert.equal(refused.status, 400);
  await post(decisionRoute, "/api/agent/decision", { id: bare.id, choice: "terminal" }, cookie());
  assert.equal((await bare.promise).status, 204);
  reset();
});

test("a question is answered through the decision route and the hook returns the joined answers", async () => {
  install();
  const body = envelope("PermissionRequest", {
    tool_name: "AskUserQuestion",
    tool_input: { questions: [{ question: "Pick?", header: "P", multiSelect: true, options: [{ label: "A", description: "a" }, { label: "B", description: "b" }] }] },
  });
  const held = await pendingFor(hookRoute.POST(hookRequest({ body })));
  const state = await (await get(stateRoute, "/api/agent/state", cookie())).json();
  assert.equal(state.pending.questions[0].question, "Pick?");
  const wrong = await post(decisionRoute, "/api/agent/decision", { id: held.id, choice: "allow", answers: { "Pick?": ["Z"] } }, cookie());
  assert.equal(wrong.status, 400);
  const right = await post(decisionRoute, "/api/agent/decision", { id: held.id, choice: "allow", answers: { "Pick?": ["A", "B"] } }, cookie());
  assert.equal(right.status, 200);
  assert.deepEqual(await (await held.promise).json(), { behavior: "allow", answers: { "Pick?": "A, B" } });
  reset();
});

test("Responder en la terminal releases the held request with 204", async () => {
  install();
  const held = await pendingFor(hookRoute.POST(hookRequest({ body: permission() })));
  await post(decisionRoute, "/api/agent/decision", { id: held.id, choice: "terminal" }, cookie());
  const response = await held.promise;
  assert.equal(response.status, 204);
  assert.equal(await response.text(), "");
  reset();
});

test("without an acknowledgement the held request is released with 204 after the acknowledgement window", async () => {
  install({ ackMs: 30 });
  const started = Date.now();
  const response = await hookRoute.POST(hookRequest({ body: permission() }));
  assert.equal(response.status, 204);
  assert.ok(Date.now() - started >= 25);
  reset();
});

test("an acknowledged request is released with 204 when the decision window ends", async () => {
  install({ ackMs: 1_000, decisionMs: 60 });
  const held = await pendingFor(hookRoute.POST(hookRequest({ body: permission() })));
  await post(ackRoute, "/api/agent/ack", { id: held.id }, cookie());
  assert.equal((await held.promise).status, 204);
  reset();
});

test("a client that disconnects frees its request", async () => {
  install();
  const controller = new AbortController();
  const promise = hookRoute.POST(hookRequest({ body: permission(), signal: controller.signal }));
  await tick();
  assert.notEqual(installedBroker().state().pending, null);
  controller.abort();
  assert.equal((await promise).status, 204);
  assert.equal(installedBroker().state().pending, null);
  reset();
});

test("quitting releases a held request with 204", async () => {
  install();
  const held = await pendingFor(hookRoute.POST(hookRequest({ body: permission() })));
  installedBroker().releaseAll();
  assert.equal((await held.promise).status, 204);
  reset();
});

test("an AskUserQuestion that cannot be shown is handed back at once", async () => {
  install();
  const response = await hookRoute.POST(hookRequest({ body: envelope("PermissionRequest", { tool_name: "AskUserQuestion", tool_input: {} }) }));
  assert.equal(response.status, 204);
  reset();
});

// --- the cookie-guarded routes ----------------------------------------------------------------------------------

test("the state route needs the control cookie and the local Host, and does not need an Origin for a GET", async () => {
  install();
  const held = await pendingFor(hookRoute.POST(hookRequest({ body: permission() })));
  assert.equal((await get(stateRoute, "/api/agent/state")).status, 403);
  assert.equal((await get(stateRoute, "/api/agent/state", cookie("e".repeat(64)))).status, 403);
  assert.equal((await get(stateRoute, "/api/agent/state", { ...cookie(), host: "evil.example" })).status, 403);
  assert.equal((await get(stateRoute, "/api/agent/state", { ...cookie(), origin: "https://evil.example" })).status, 403);
  const ok = await get(stateRoute, "/api/agent/state", cookie());
  assert.equal(ok.status, 200);
  assert.equal(ok.headers.get("cache-control"), "no-store, private");
  const state = await ok.json();
  assert.equal(state.pending.id, held.id);
  assert.deepEqual(Object.keys(state.pending).sort(), ["alwaysOffered", "detail", "id", "project", "questions", "summary", "tool"]);
  assert.equal(state.pending.alwaysOffered, true);
  assert.equal(state.queued, 0);
  assert.equal(state.sessions[0].state, "te necesita");
  installedBroker().releaseAll();
  await held.promise;
  reset();
});

test("ack and decision need the control cookie, the exact Origin and the local Host", async () => {
  install();
  const held = await pendingFor(hookRoute.POST(hookRequest({ body: permission() })));
  for (const [route, path, body] of [
    [ackRoute, "/api/agent/ack", { id: held.id }],
    [decisionRoute, "/api/agent/decision", { id: held.id, choice: "allow" }],
  ]) {
    assert.equal((await post(route, path, body)).status, 403, "no cookie");
    assert.equal((await post(route, path, body, cookie("e".repeat(64)))).status, 403, "wrong cookie");
    assert.equal((await post(route, path, body, { ...cookie(), origin: "https://evil.example" })).status, 403, "wrong origin");
    assert.equal((await post(route, path, body, { ...cookie(), host: "evil.example" })).status, 403, "wrong host");
    const noOrigin = await route.POST(new Request(`http://${HOST}${path}`, { method: "POST", headers: { host: HOST, "content-type": "application/json", ...cookie() }, body: JSON.stringify(body) }));
    assert.equal(noOrigin.status, 403, "no origin");
  }
  assert.notEqual(installedBroker().state().pending, null, "nothing was decided by the refused calls");
  installedBroker().releaseAll();
  await held.promise;
  reset();
});

test("the cookie routes still answer when no control token is published (plain browser development)", async () => {
  install({ control: false });
  assert.equal((await get(stateRoute, "/api/agent/state")).status, 200);
  reset();
});

test("ack and decision validate their bodies and answer 404 for a request that is gone", async () => {
  install();
  const held = await pendingFor(hookRoute.POST(hookRequest({ body: permission() })));
  for (const body of [{}, { id: 5 }, { id: held.id, extra: 1 }, [], "x"]) assert.equal((await post(ackRoute, "/api/agent/ack", body, cookie())).status, 400, JSON.stringify(body));
  for (const body of [{}, { id: held.id }, { id: held.id, choice: "maybe" }, { id: held.id, choice: "allow", extra: 1 }, { id: 5, choice: "allow" }, { id: held.id, choice: "allow", answers: "x" }]) {
    assert.equal((await post(decisionRoute, "/api/agent/decision", body, cookie())).status, 400, JSON.stringify(body));
  }
  assert.equal((await post(ackRoute, "/api/agent/ack", { id: "nope" }, cookie())).status, 404);
  assert.equal((await post(decisionRoute, "/api/agent/decision", { id: "nope", choice: "allow" }, cookie())).status, 404);
  assert.equal((await post(ackRoute, "/api/agent/ack", { id: held.id }, cookie())).status, 200);
  const big = await decisionRoute.POST(new Request(`http://${HOST}/api/agent/decision`, { method: "POST", headers: { host: HOST, origin: `http://${HOST}`, "content-type": "application/json", ...cookie() }, body: JSON.stringify({ id: held.id, choice: "allow", pad: "x".repeat(70_000) }) }));
  assert.equal(big.status, 413);
  installedBroker().releaseAll();
  await held.promise;
  reset();
});

// --- the session summary is all the state route knows ---------------------------------------------------------

test("the state route lists sessions with project, state and latest action", async () => {
  install();
  await hookRoute.POST(hookRequest({ body: envelope("PreToolUse", { tool_name: "Bash", tool_input: { command: "npm test" } }) }));
  const state = await (await get(stateRoute, "/api/agent/state", cookie())).json();
  assert.deepEqual(state.sessions.map(({ project, state: label, action }) => ({ project, state: label, action })), [{ project: "alpha", state: "trabajando", action: "Ejecutando npm test" }]);
  assert.equal(state.pending, null);
  reset();
});

function installedBroker() {
  return globalThis[BROKER_KEY];
}
