import assert from "node:assert/strict";
import { after, before, test } from "node:test";
import { chmod, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

const directory = await mkdtemp(join(tmpdir(), "click-dot-chat-agent-test-"));
await chmod(directory, 0o700);
process.env.CLICK_DOT_AUTH_FILE = join(directory, "auth.json");
process.env.CLICK_DOT_CHAT_FILE = join(directory, "conversations.json");
process.env.PORT = "3000";

const authStore = await import("../lib/auth-store.ts");
const historyStore = await import("../lib/conversation-store.ts");
const provider = await import("../lib/chat-provider.ts");
const loop = await import("../lib/agent-loop.ts");
const bridge = await import("../lib/tool-runner.ts");
const brokerModule = await import("../lib/agent-broker.ts");
const probe = await import("../lib/vision-probe.ts");
const streamRoute = await import("../app/api/chat/stream/route.ts");
const historyRoute = await import("../app/api/chat/history/route.ts");
const originalFetch = globalThis.fetch;

const accessToken = "offline-fixture-token-never-a-real-credential";
const RUNNER_KEY = Symbol.for("click-dot.tool-runner");
const CONTROLLER_KEY = Symbol.for("click-dot.capture-controller");
const CONTROL_KEY = Symbol.for("click-dot.control-token");
const BROKER_KEY = Symbol.for("click-dot.agent-broker");
const PNG = probe.syntheticProbePng();
const tick = (ms = 0) => new Promise((resolve) => setTimeout(resolve, ms));

async function writeAuth() {
  await writeFile(process.env.CLICK_DOT_AUTH_FILE, JSON.stringify({
    hostId: "urn:uuid:offline-fixture", generation: 4, attempts: {}, issuedClientId: "client-offline-fixture",
    connection: { clientId: "client-offline-fixture", issuer: "https://auth.openai.com", subject: "account-one", accessToken, refreshToken: "fixture-refresh-token", idToken: "fixture-id-token", scopes: ["chatgpt.tokens.use.direct"], expiresAt: Date.now() + 60_000 },
  }), { mode: 0o600 });
  await chmod(process.env.CLICK_DOT_AUTH_FILE, 0o600);
}

function localRequest(url, init = {}) {
  const method = init.method ?? "GET";
  return new Request(`http://127.0.0.1:3000${url}`, {
    ...init, method,
    headers: { host: "127.0.0.1:3000", ...(method !== "GET" ? { origin: "http://127.0.0.1:3000" } : {}), ...(init.headers ?? {}) },
  });
}

const json = (body, headers = {}) => ({ method: "POST", headers: { "content-type": "application/json", ...headers }, body: JSON.stringify(body) });
const fakeModels = () => new Response(JSON.stringify({ models: [{ slug: "model-first", display_name: "First model", visibility: "list" }] }), { headers: { "content-type": "application/json" } });
const frame = (event) => `data: ${JSON.stringify(event)}\n\n`;
const sse = (chunks) => new Response(chunks.join(""), { headers: { "content-type": "text/event-stream" } });
const textRound = (text) => sse([frame({ type: "response.output_text.delta", delta: text }), frame({ type: "response.completed", response: {} })]);
const itemsRound = (items, text = "") => sse([
  ...(text ? [frame({ type: "response.output_text.delta", delta: text })] : []),
  ...items.map((item) => frame({ type: "response.output_item.done", item })),
  frame({ type: "response.completed", response: {} }),
]);
const reasoning = (n = 1) => ({ type: "reasoning", id: `rs_${n}`, summary: [], encrypted_content: `ENC${n}` });
const call = (name, args, n = 1) => ({ type: "function_call", id: `fc_${n}`, call_id: `call_${n}`, name, arguments: JSON.stringify(args), status: "completed" });

/** Plays the model: each request gets the next scripted response and is recorded. */
function scripted(rounds) {
  const requests = [];
  const fetcher = async (url, init) => {
    assert.equal(String(url), "https://api.openai.com/v1/responses");
    requests.push({ body: JSON.parse(init.body), signal: init.signal });
    const next = rounds[requests.length - 1];
    assert.ok(next, `unexpected request number ${requests.length}`);
    return typeof next === "function" ? next(init) : next;
  };
  return { fetcher, requests };
}

const READ_ONLY = new Set(["git status", "Get-ChildItem"]);
function fakeRunner(overrides = {}) {
  const calls = { classify: [], powershell: [], claude: [] };
  const runner = {
    classifyPowerShell: async (command) => { calls.classify.push(command); return { readOnly: READ_ONLY.has(command), reason: "fixture" }; },
    runPowerShell: async (request) => { calls.powershell.push(request); return { exitCode: 0, stdout: `ran ${request.command}`, stderr: "", truncated: false, timedOut: false }; },
    runClaudeCode: async (request) => {
      calls.claude.push(request);
      request.onProgress("Claude Code: Edit foo.ts");
      return { ok: true, result: "All edited", sessionId: "sess-1", costUsd: 0.25, permissionDenials: 0, truncated: false, timedOut: false };
    },
    ...overrides,
  };
  return { runner, calls };
}

function newBroker() {
  return brokerModule.createAgentBroker({ ackMs: 60_000, decisionMs: 60_000 });
}

async function pendingOf(broker) {
  for (let attempt = 0; attempt < 400; attempt += 1) {
    const { pending } = broker.state();
    if (pending) return pending;
    await tick(5);
  }
  throw new Error("no approval card appeared");
}

function turn(overrides = {}) {
  const events = [];
  const { runner, calls } = overrides.fake ?? fakeRunner();
  const broker = overrides.broker ?? newBroker();
  const options = {
    connection: { accessToken },
    model: "model-first",
    input: [{ role: "user", content: "Hola" }],
    runner,
    broker,
    signal: new AbortController().signal,
    emit: (event) => { events.push(event); return true; },
    alwaysAllowed: new Set(),
    lookAtScreen: async () => ({ ok: false, message: "screen sharing is off" }),
    ...overrides.options,
  };
  const done = loop.runAgentTurn(options);
  return { done, events, calls, broker, options };
}
const toolEvents = (events) => events.filter((event) => event.type === "tool").map((event) => event.tool);
const statuses = (events, id) => toolEvents(events).filter((event) => event.id === id).map((event) => event.status);
const replayedOutputs = (body) => body.input.filter((item) => item.type === "function_call_output");

before(async () => writeAuth());
after(async () => {
  globalThis.fetch = originalFetch;
  for (const key of [RUNNER_KEY, CONTROLLER_KEY, CONTROL_KEY, BROKER_KEY]) Reflect.deleteProperty(globalThis, key);
  await rm(directory, { recursive: true, force: true });
});

// The bridge -------------------------------------------------------------------------------------------------

test("the tool runner accessor returns the published runner and null for nothing or for anything that is not a runner", () => {
  assert.equal(bridge.getToolRunner(), null);
  const { runner } = fakeRunner();
  Reflect.set(globalThis, RUNNER_KEY, runner);
  assert.equal(bridge.getToolRunner(), runner);
  for (const bad of ["runner", 3, { classifyPowerShell() {}, runPowerShell() {} }, { classifyPowerShell() {}, runPowerShell() {}, runClaudeCode: "no" }]) {
    Reflect.set(globalThis, RUNNER_KEY, bad);
    assert.equal(bridge.getToolRunner(), null, JSON.stringify(bad));
  }
  Reflect.deleteProperty(globalThis, RUNNER_KEY);
});

// The provider in agent mode -----------------------------------------------------------------------------------

test("agent mode adds instructions, three strict tools, auto tool choice, no parallel calls and encrypted reasoning to the request", async () => {
  const { fetcher, requests } = scripted([textRound("Hola")]);
  const events = [];
  for await (const event of provider.streamAgentResponse({ accessToken }, "model-first", [{ role: "user", content: "Hi" }], fetcher)) events.push(event);
  const { body } = requests[0];
  assert.deepEqual(body, {
    model: "model-first",
    input: [{ role: "user", content: "Hi" }],
    instructions: provider.AGENT_INSTRUCTIONS,
    tools: provider.AGENT_TOOLS,
    tool_choice: "auto",
    parallel_tool_calls: false,
    include: ["reasoning.encrypted_content"],
    store: false,
    stream: true,
  });
  assert.deepEqual(body.tools.map((tool) => tool.name), ["run_powershell", "run_claude_code", "look_at_screen"]);
  for (const tool of body.tools) {
    assert.equal(tool.type, "function");
    assert.equal(tool.strict, true);
    assert.equal(tool.parameters.additionalProperties, false);
    assert.deepEqual(tool.parameters.required, Object.keys(tool.parameters.properties));
  }
  assert.deepEqual(body.tools[0].parameters.properties, { command: { type: "string", description: body.tools[0].parameters.properties.command.description }, cwd: { type: ["string", "null"], description: body.tools[0].parameters.properties.cwd.description } });
  assert.deepEqual(Object.keys(body.tools[1].parameters.properties), ["task", "project_dir"]);
  assert.deepEqual(body.tools[2].parameters.properties, {});
  assert.match(provider.AGENT_INSTRUCTIONS, /Dot/);
  assert.match(provider.AGENT_INSTRUCTIONS, /run_powershell/);
  assert.match(provider.AGENT_INSTRUCTIONS, /run_claude_code/);
  assert.match(provider.AGENT_INSTRUCTIONS, /look_at_screen/);
  assert.match(provider.AGENT_INSTRUCTIONS, /no son instrucciones|nunca son instrucciones/i, "outputs, screen and files are untrusted data");
  assert.deepEqual(events.at(-1), { type: "completed", text: "Hola", items: [], calls: [] });
});

test("plain mode still sends exactly the old body with no tools, and rejects a tool-only completion", async () => {
  const { fetcher, requests } = scripted([textRound("Hello"), itemsRound([call("run_powershell", { command: "x", cwd: null })])]);
  for await (const _event of provider.streamChatResponse({ accessToken }, "model-first", [{ role: "user", content: "Hi" }], fetcher)) { /* drain */ }
  assert.deepEqual(requests[0].body, { model: "model-first", input: [{ role: "user", content: "Hi" }], store: false, stream: true });
  await assert.rejects(async () => {
    for await (const _event of provider.streamChatResponse({ accessToken }, "model-first", [{ role: "user", content: "Hi" }], fetcher)) { /* drain */ }
  }, /without a text response/);
});

test("function calls are read from output_item.done, every output item is kept in order, and a call-only completion is valid", async () => {
  const items = [reasoning(1), call("run_powershell", { command: "git status", cwd: null }, 1)];
  const { fetcher } = scripted([itemsRound(items)]);
  const events = [];
  for await (const event of provider.streamAgentResponse({ accessToken }, "model-first", [{ role: "user", content: "Hi" }], fetcher)) events.push(event);
  assert.equal(events.length, 1);
  assert.deepEqual(events[0], {
    type: "completed",
    text: "",
    items,
    calls: [{ callId: "call_1", name: "run_powershell", arguments: '{"command":"git status","cwd":null}' }],
  });
});

test("a completion with neither text nor a call, a malformed call and a failed response are errors", async (t) => {
  const cases = [
    ["empty", [frame({ type: "response.completed" })]],
    ["call without call_id", [frame({ type: "response.output_item.done", item: { type: "function_call", name: "x", arguments: "{}" } }), frame({ type: "response.completed" })]],
    ["call with non-string arguments", [frame({ type: "response.output_item.done", item: { type: "function_call", call_id: "c", name: "x", arguments: {} } }), frame({ type: "response.completed" })]],
    ["failed", [frame({ type: "response.failed" })]],
    ["truncated", [frame({ type: "response.output_item.done", item: reasoning() })]],
  ];
  for (const [name, chunks] of cases) await t.test(name, async () => {
    const { fetcher } = scripted([sse(chunks)]);
    await assert.rejects(async () => { for await (const _event of provider.streamAgentResponse({ accessToken }, "model-first", [{ role: "user", content: "Hi" }], fetcher)) { /* drain */ } });
  });
});

test("replayed items lose only their id, and the replay input is validated before any request", async () => {
  assert.deepEqual(provider.toReplayItem(reasoning(3)), { type: "reasoning", summary: [], encrypted_content: "ENC3" });
  assert.deepEqual(provider.toReplayItem(call("look_at_screen", {}, 3)), { type: "function_call", call_id: "call_3", name: "look_at_screen", arguments: "{}", status: "completed" });
  let calls = 0;
  const fetcher = async () => { calls += 1; return textRound("x"); };
  const user = [{ role: "user", content: "Hi" }];
  for (const bad of [{ type: "shell_call" }, { type: "function_call_output", call_id: "c", output: 5 }, "text", null, { type: "function_call_output", call_id: "c", output: [{ type: "input_image", image_url: "https://evil.example/x.png" }] }]) {
    await assert.rejects(async () => { for await (const _event of provider.streamAgentResponse({ accessToken }, "model-first", [...user, bad], fetcher)) { /* drain */ } }, /too large or invalid/, JSON.stringify(bad));
  }
  assert.equal(calls, 0);
  const image = `data:image/png;base64,${PNG.toString("base64")}`;
  const output = { type: "function_call_output", call_id: "c", output: [{ type: "input_text", text: "screen" }, { type: "input_image", image_url: image }] };
  for await (const _event of provider.streamAgentResponse({ accessToken }, "model-first", [...user, output], fetcher)) { /* drain */ }
  assert.equal(calls, 1);
});

// The loop -------------------------------------------------------------------------------------------------

test("a read-only command runs without asking, and the next request replays the items then the output in order", async () => {
  const fake = fakeRunner();
  const first = [reasoning(1), call("run_powershell", { command: "git status", cwd: "C:\\work\\alpha" }, 1)];
  const { fetcher, requests } = scripted([itemsRound(first), textRound("Todo limpio")]);
  const { done, events, broker } = turn({ fake, options: { fetcher } });
  const result = await done;
  assert.equal(result.text, "Todo limpio");
  assert.equal(broker.state().pending, null, "no card for a read-only command");
  assert.deepEqual(fake.calls.powershell.map(({ command, cwd }) => [command, cwd]), [["git status", "C:\\work\\alpha"]]);
  assert.ok(fake.calls.powershell[0].signal instanceof AbortSignal);
  assert.deepEqual(requests[1].body.input, [
    { role: "user", content: "Hola" },
    { type: "reasoning", summary: [], encrypted_content: "ENC1" },
    { type: "function_call", call_id: "call_1", name: "run_powershell", arguments: '{"command":"git status","cwd":"C:\\\\work\\\\alpha"}', status: "completed" },
    { type: "function_call_output", call_id: "call_1", output: "exit code: 0\n--- stdout ---\nran git status" },
  ]);
  assert.equal(requests[0].body.tool_choice, "auto");
  assert.deepEqual(statuses(events, "call_1"), ["running", "done"]);
  assert.deepEqual(toolEvents(events)[0], { id: "call_1", name: "powershell", summary: "git status", status: "running" });
  assert.deepEqual(toolEvents(events)[1], { id: "call_1", name: "powershell", summary: "git status", status: "done", detail: "ran git status" });
  assert.deepEqual(events.filter((event) => event.type === "delta"), [{ type: "delta", text: "Todo limpio" }]);
  assert.deepEqual(result.tools, [{ label: "git status", outcome: "auto" }]);
});

test("a command that is not read-only asks on the mascot, and Permitir runs it", async () => {
  const fake = fakeRunner();
  const { fetcher } = scripted([itemsRound([call("run_powershell", { command: "Remove-Item old.txt", cwd: "C:\\work" })]), textRound("Borrado")]);
  const { done, events, broker } = turn({ fake, options: { fetcher } });
  const pending = await pendingOf(broker);
  assert.equal(pending.kind, "dot-tool");
  assert.equal(pending.tool, "PowerShell");
  assert.equal(pending.summary, "Remove-Item old.txt");
  assert.match(pending.detail, /C:\\work/);
  assert.match(pending.detail, /Puede modificar tu equipo/);
  assert.match(pending.detail, /Motivo: fixture/, "the card says why the classifier asked");
  assert.equal(pending.alwaysOffered, true);
  assert.deepEqual(fake.calls.powershell, [], "nothing runs before the user decides");
  assert.deepEqual(statuses(events, "call_1"), ["waiting_approval"]);
  assert.equal(broker.ack(pending.id), true);
  assert.deepEqual(broker.decide(pending.id, "allow"), { ok: true });
  const result = await done;
  assert.equal(result.text, "Borrado");
  assert.equal(fake.calls.powershell.length, 1);
  assert.deepEqual(statuses(events, "call_1"), ["waiting_approval", "running", "done"]);
  assert.deepEqual(result.tools, [{ label: "Remove-Item old.txt", outcome: "approved" }]);
});

test("denying, never acknowledging and aborting the card each return the denial and run nothing", async (t) => {
  await t.test("Denegar", async () => {
    const fake = fakeRunner();
    const { fetcher, requests } = scripted([itemsRound([call("run_powershell", { command: "Remove-Item a", cwd: null })]), textRound("Vale")]);
    const { done, events, broker } = turn({ fake, options: { fetcher } });
    broker.decide((await pendingOf(broker)).id, "deny");
    const result = await done;
    assert.deepEqual(fake.calls.powershell, []);
    assert.deepEqual(replayedOutputs(requests[1].body), [{ type: "function_call_output", call_id: "call_1", output: "The user denied this action." }]);
    assert.deepEqual(statuses(events, "call_1"), ["waiting_approval", "denied"]);
    assert.deepEqual(result.tools, [{ label: "Remove-Item a", outcome: "denied" }]);
  });
  await t.test("timeout (no decision)", async () => {
    const fake = fakeRunner();
    const broker = brokerModule.createAgentBroker({ ackMs: 20, decisionMs: 60_000 });
    const { fetcher, requests } = scripted([itemsRound([call("run_powershell", { command: "Remove-Item a", cwd: null })]), textRound("Vale")]);
    const { done, events } = turn({ fake, broker, options: { fetcher } });
    await done;
    assert.deepEqual(fake.calls.powershell, []);
    assert.equal(replayedOutputs(requests[1].body)[0].output, "The user denied this action.");
    assert.deepEqual(statuses(events, "call_1"), ["waiting_approval", "denied"]);
  });
});

test("Siempre runs the command and remembers its exact text, so the same command later runs without a card but a different one asks", async () => {
  const fake = fakeRunner();
  const alwaysAllowed = new Set();
  const { fetcher } = scripted([
    itemsRound([call("run_powershell", { command: "npm test", cwd: null })]),
    textRound("uno"),
    itemsRound([call("run_powershell", { command: "npm test", cwd: null }, 2)]),
    textRound("dos"),
    itemsRound([call("run_powershell", { command: "npm test ", cwd: null }, 3)]),
    textRound("tres"),
  ]);
  const first = turn({ fake, options: { fetcher, alwaysAllowed } });
  first.broker.decide((await pendingOf(first.broker)).id, "always");
  const one = await first.done;
  assert.deepEqual(one.tools, [{ label: "npm test", outcome: "always" }]);
  assert.equal(alwaysAllowed.size, 1);
  const second = turn({ fake, options: { fetcher, alwaysAllowed } });
  const two = await second.done;
  assert.equal(second.broker.state().pending, null, "the remembered command asks nothing");
  assert.equal(fake.calls.powershell.length, 2);
  assert.deepEqual(two.tools, [{ label: "npm test", outcome: "always" }]);
  const third = turn({ fake, options: { fetcher, alwaysAllowed } });
  const pending = await pendingOf(third.broker);
  assert.equal(pending.summary, "npm test ");
  third.broker.decide(pending.id, "deny");
  await third.done;
  assert.equal(fake.calls.powershell.length, 2);
});

test("Siempre remembers the command for its folder only, so the same command elsewhere asks again", async () => {
  const fake = fakeRunner();
  const alwaysAllowed = new Set();
  const { fetcher } = scripted([
    itemsRound([call("run_powershell", { command: "Remove-Item build", cwd: "C:\\work\\a" })]),
    textRound("uno"),
    itemsRound([call("run_powershell", { command: "Remove-Item build", cwd: "C:\\work\\b" }, 2)]),
    textRound("dos"),
  ]);
  const first = turn({ fake, options: { fetcher, alwaysAllowed } });
  first.broker.decide((await pendingOf(first.broker)).id, "always");
  await first.done;
  const second = turn({ fake, options: { fetcher, alwaysAllowed } });
  const pending = await pendingOf(second.broker);
  assert.match(pending.detail, /C:\\work\\b/);
  second.broker.decide(pending.id, "deny");
  await second.done;
  assert.equal(fake.calls.powershell.length, 1, "the command did not run in the other folder");
});

test("a classifier that fails means the command needs approval, and an over-long command is denied without asking", async () => {
  const failing = fakeRunner({ classifyPowerShell: async () => { throw new Error("parser exploded"); } });
  const { fetcher } = scripted([itemsRound([call("run_powershell", { command: "git status", cwd: null })]), textRound("ok")]);
  const first = turn({ fake: failing, options: { fetcher } });
  const pending = await pendingOf(first.broker);
  assert.equal(pending.summary, "git status");
  first.broker.decide(pending.id, "deny");
  await first.done;

  const fake = fakeRunner();
  const long = "x".repeat(8_001);
  const second = scripted([itemsRound([call("run_powershell", { command: long, cwd: null })]), textRound("ok")]);
  const run = turn({ fake, options: { fetcher: second.fetcher } });
  await run.done;
  assert.equal(run.broker.state().pending, null);
  assert.deepEqual(fake.calls.powershell, []);
  assert.match(replayedOutputs(second.requests[1].body)[0].output, /too long to review/);
  assert.equal(statuses(run.events, "call_1").at(-1), "error");
});

test("invalid tool arguments and unknown tools return an error to the model and run nothing", async () => {
  const fake = fakeRunner();
  const { fetcher, requests } = scripted([
    itemsRound([call("run_powershell", { command: 5, cwd: null }, 1)]),
    itemsRound([call("format_disk", {}, 2)]),
    sse([frame({ type: "response.output_item.done", item: { type: "function_call", call_id: "call_3", name: "run_powershell", arguments: "not json" } }), frame({ type: "response.completed" })]),
    textRound("fin"),
  ]);
  const { done, events } = turn({ fake, options: { fetcher } });
  await done;
  assert.deepEqual(fake.calls.powershell, []);
  assert.deepEqual(requests.map((request) => replayedOutputs(request.body).length), [0, 1, 2, 3]);
  assert.match(replayedOutputs(requests[3].body)[0].output, /Invalid arguments/);
  assert.match(replayedOutputs(requests[3].body)[1].output, /Unknown tool/);
  assert.equal(statuses(events, "call_1").at(-1), "error");
});

test("Claude Code always asks without offering Siempre, forwards progress lines and returns the result summary", async () => {
  const fake = fakeRunner();
  const { fetcher, requests } = scripted([itemsRound([call("run_claude_code", { task: "Fix the bug", project_dir: "C:\\work\\alpha" })]), textRound("Hecho")]);
  const { done, events, broker } = turn({ fake, options: { fetcher, alwaysAllowed: new Set(["Fix the bug"]) } });
  const pending = await pendingOf(broker);
  assert.deepEqual({ kind: pending.kind, tool: pending.tool, summary: pending.summary, alwaysOffered: pending.alwaysOffered }, { kind: "dot-tool", tool: "Claude Code", summary: "Fix the bug", alwaysOffered: false });
  assert.match(pending.detail, /C:\\work\\alpha/);
  broker.decide(pending.id, "allow");
  const result = await done;
  assert.deepEqual(fake.calls.claude.map(({ task, cwd }) => [task, cwd]), [["Fix the bug", "C:\\work\\alpha"]]);
  assert.deepEqual(statuses(events, "call_1"), ["waiting_approval", "running", "running", "done"]);
  const progress = toolEvents(events).find((event) => event.detail === "Claude Code: Edit foo.ts");
  assert.deepEqual(progress, { id: "call_1", name: "claude_code", summary: "Fix the bug", status: "running", detail: "Claude Code: Edit foo.ts" });
  const output = replayedOutputs(requests[1].body)[0].output;
  assert.match(output, /All edited/);
  assert.match(output, /sess-1/);
  assert.match(output, /0\.25/);
  assert.match(output, /permission denials: 0/);
  assert.deepEqual(result.tools, [{ label: "Claude Code", outcome: "approved" }]);
});

test("Claude Code that cannot start is an error result, not a crash", async () => {
  const fake = fakeRunner({ runClaudeCode: async () => ({ ok: false, result: "", sessionId: null, costUsd: null, permissionDenials: 0, truncated: false, timedOut: false, error: "Claude Code is not installed" }) });
  const { fetcher, requests } = scripted([itemsRound([call("run_claude_code", { task: "t", project_dir: "C:\\p" })]), textRound("No se pudo")]);
  const { done, events, broker } = turn({ fake, options: { fetcher } });
  broker.decide((await pendingOf(broker)).id, "allow");
  await done;
  assert.match(replayedOutputs(requests[1].body)[0].output, /Claude Code is not installed/);
  assert.equal(statuses(events, "call_1").at(-1), "error");
});

test("look_at_screen returns the redacted image in the tool output when sharing is on, and says so when it is off", async () => {
  const image = `data:image/png;base64,${PNG.toString("base64")}`;
  const on = scripted([itemsRound([call("look_at_screen", {})]), textRound("Veo un terminal")]);
  const seen = turn({ options: { fetcher: on.fetcher, lookAtScreen: async () => ({ ok: true, imageUrl: image }) } });
  const result = await seen.done;
  assert.equal(result.text, "Veo un terminal");
  const [output] = replayedOutputs(on.requests[1].body);
  assert.equal(output.call_id, "call_1");
  assert.equal(output.output.length, 2);
  assert.equal(output.output[0].type, "input_text");
  assert.deepEqual(output.output[1], { type: "input_image", image_url: image });
  assert.deepEqual(statuses(seen.events, "call_1"), ["running", "done"]);
  assert.deepEqual(toolEvents(seen.events)[0], { id: "call_1", name: "screen", summary: "Mirar la pantalla", status: "running" });
  assert.equal(seen.broker.state().pending, null, "looking needs no card");
  assert.deepEqual(result.tools, [{ label: "pantalla", outcome: "auto" }]);

  const off = scripted([itemsRound([call("look_at_screen", {})]), textRound("No puedo ver")]);
  const blind = turn({ options: { fetcher: off.fetcher } });
  await blind.done;
  assert.deepEqual(replayedOutputs(off.requests[1].body), [{ type: "function_call_output", call_id: "call_1", output: "screen sharing is off" }]);
  assert.equal(statuses(blind.events, "call_1").at(-1), "done");
});

test("the default screen look uses the redacted frame only while capture is on and not paused", async () => {
  const prepareCalls = [];
  const controller = (status) => ({
    status: () => status,
    apply: async () => ({}),
    prepareScreen: async () => { prepareCalls.push(1); return { image: Buffer.from(PNG), mime: "image/png", hiddenRegions: 1, capturedAt: 1 }; },
  });
  Reflect.deleteProperty(globalThis, CONTROLLER_KEY);
  assert.deepEqual(await loop.lookAtSharedScreen(), { ok: false, message: "screen sharing is off" });
  for (const status of [{ enabled: false, paused: false, frames: 0, lastCapturedAt: null }, { enabled: true, paused: true, frames: 1, lastCapturedAt: 1 }]) {
    Reflect.set(globalThis, CONTROLLER_KEY, controller(status));
    assert.deepEqual(await loop.lookAtSharedScreen(), { ok: false, message: "screen sharing is off" });
  }
  assert.equal(prepareCalls.length, 0, "nothing is captured while sharing is off or paused");
  Reflect.set(globalThis, CONTROLLER_KEY, controller({ enabled: true, paused: false, frames: 1, lastCapturedAt: 1 }));
  assert.deepEqual(await loop.lookAtSharedScreen(), { ok: true, imageUrl: `data:image/png;base64,${PNG.toString("base64")}` });
  Reflect.set(globalThis, CONTROLLER_KEY, { ...controller({ enabled: true, paused: false, frames: 1, lastCapturedAt: 1 }), prepareScreen: async () => { throw Object.assign(new Error("raw OCR text"), { code: "ocr-failed" }); } });
  const failed = await loop.lookAtSharedScreen();
  assert.equal(failed.ok, false);
  assert.doesNotMatch(failed.message, /raw OCR/);
  Reflect.deleteProperty(globalThis, CONTROLLER_KEY);
});

test("after 10 tool rounds the model is told to answer in text and no further tool runs", async () => {
  const fake = fakeRunner();
  const rounds = Array.from({ length: 10 }, (_, index) => itemsRound([call("run_powershell", { command: "Get-ChildItem", cwd: null }, index + 1)]));
  const { fetcher, requests } = scripted([...rounds, textRound("Basta")]);
  const { done } = turn({ fake, options: { fetcher } });
  const result = await done;
  assert.equal(result.text, "Basta");
  assert.equal(fake.calls.powershell.length, 10);
  assert.equal(requests.length, 11);
  assert.deepEqual(requests.slice(0, 10).map((request) => request.body.tool_choice), Array(10).fill("auto"));
  assert.equal(requests[10].body.tool_choice, "none");
});

test("a model that still asks for a tool after the cap, or ends with no text, is an error", async () => {
  const fake = fakeRunner();
  const rounds = Array.from({ length: 11 }, (_, index) => itemsRound([call("run_powershell", { command: "Get-ChildItem", cwd: null }, index + 1)]));
  const { fetcher } = scripted(rounds);
  await assert.rejects(turn({ fake, options: { fetcher } }).done, /tool limit/);
  assert.equal(fake.calls.powershell.length, 10);
});

test("only the last round's text is the answer; earlier text is dropped", async () => {
  const { fetcher } = scripted([itemsRound([call("run_powershell", { command: "Get-ChildItem", cwd: null })], "Voy a mirar. "), textRound("Resultado final")]);
  const { done, events } = turn({ options: { fetcher } });
  assert.equal((await done).text, "Resultado final");
  assert.deepEqual(events.filter((event) => event.type === "delta").map((event) => event.text), ["Voy a mirar. ", "Resultado final"]);
});

test("a very long output reaches the model as its tails, within 16 KiB, and the UI detail within 2,000 characters", async () => {
  const fake = fakeRunner({ runPowerShell: async () => ({ exitCode: 1, stdout: `${"a".repeat(30_000)}END-OUT`, stderr: `${"b".repeat(30_000)}END-ERR`, truncated: true, timedOut: false }) });
  const { fetcher, requests } = scripted([itemsRound([call("run_powershell", { command: "Get-ChildItem", cwd: null })]), textRound("ok")]);
  const { done, events } = turn({ fake, options: { fetcher } });
  await done;
  const output = replayedOutputs(requests[1].body)[0].output;
  assert.ok(Buffer.byteLength(output, "utf8") <= 16 * 1024);
  assert.match(output, /exit code: 1/);
  assert.match(output, /END-OUT/);
  assert.match(output, /END-ERR/);
  assert.match(output, /truncated/);
  const last = toolEvents(events).at(-1);
  assert.equal(last.status, "error", "a non-zero exit shows as an error chip");
  assert.ok(last.detail.length <= 2_000);
  assert.match(last.detail, /END-ERR$/);
});

test("stopping the turn aborts the model request and passes the signal to the running tool", async () => {
  const stop = new AbortController();
  let runnerSignal;
  const fake = fakeRunner({ runPowerShell: (request) => { runnerSignal = request.signal; return new Promise((resolve) => request.signal.addEventListener("abort", () => resolve({ exitCode: null, stdout: "", stderr: "", truncated: false, timedOut: false }), { once: true })); } });
  const { fetcher } = scripted([itemsRound([call("run_powershell", { command: "Get-ChildItem", cwd: null })])]);
  const { done } = turn({ fake, options: { fetcher, signal: stop.signal } });
  await tick(20);
  assert.ok(runnerSignal, "the tool is running");
  assert.equal(runnerSignal.aborted, false);
  stop.abort();
  await assert.rejects(done, /cancelled/);
  assert.equal(runnerSignal.aborted, true);

  const model = new AbortController();
  let upstream;
  const waiting = scripted([(init) => { upstream = init.signal; return new Promise((_resolve, reject) => init.signal.addEventListener("abort", () => reject(new Error("aborted")), { once: true })); }]);
  const pendingTurn = turn({ options: { fetcher: waiting.fetcher, signal: model.signal } });
  const rejected = assert.rejects(pendingTurn.done);
  await tick(10);
  model.abort();
  await rejected;
  assert.equal(upstream.aborted, true, "the model request is cancelled too");
});

test("stopping while the card is shown releases it and never reports a denial", async () => {
  const stop = new AbortController();
  const { fetcher } = scripted([itemsRound([call("run_powershell", { command: "Remove-Item a", cwd: null })])]);
  const { done, broker, events } = turn({ options: { fetcher, signal: stop.signal } });
  await pendingOf(broker);
  stop.abort();
  await assert.rejects(done, /cancelled/);
  assert.equal(broker.state().pending, null);
  assert.equal(statuses(events, "call_1").includes("denied"), false);
});

test("the summary line lists each tool with its outcome on one line", () => {
  assert.equal(loop.toolSummaryLine([]), "");
  assert.equal(loop.toolSummaryLine([{ label: "git status", outcome: "auto" }, { label: "Claude Code", outcome: "approved" }]), "[herramientas: git status (auto), Claude Code (aprobado)]");
  assert.equal(loop.toolSummaryLine([{ label: "a", outcome: "always" }, { label: "b", outcome: "denied" }, { label: "c", outcome: "error" }]), "[herramientas: a (siempre), b (denegado), c (error)]");
  const long = loop.toolSummaryLine([{ label: "x\n]y".repeat(300), outcome: "auto" }]);
  assert.ok(long.length <= 400);
  assert.doesNotMatch(long.slice(1, -1), /\n|\]/);
});

// The route -------------------------------------------------------------------------------------------------

function installRoute({ rounds, runner = fakeRunner() }) {
  Reflect.set(globalThis, RUNNER_KEY, runner.runner);
  Reflect.set(globalThis, BROKER_KEY, newBroker());
  const model = scripted(rounds);
  globalThis.fetch = async (url, init) => String(url).endsWith("/models") ? fakeModels() : model.fetcher(url, init);
  return { ...model, runner, broker: globalThis[BROKER_KEY] };
}
async function freshRevision() {
  const snapshot = await authStore.getConnectionSnapshot();
  await historyStore.clearConversations(snapshot);
  return (await historyStore.listConversations(snapshot)).revision;
}
const post = (body, headers = {}) => streamRoute.POST(localRequest("/api/chat/stream", json(body, headers)));
const parseSse = (text) => text.split("\n\n").filter(Boolean).map((chunk) => {
  const [event, data] = chunk.split("\n");
  return { event: event.slice(7), data: JSON.parse(data.slice(6)) };
});

test("with a tool runtime the route sends tool events, streams the final text and saves only the prompt and final text with a summary line", async () => {
  const revision = await freshRevision();
  const { requests, runner } = installRoute({ rounds: [itemsRound([reasoning(1), call("run_powershell", { command: "git status", cwd: null })]), textRound("Limpio")] });
  const response = await post({ conversationId: crypto.randomUUID(), revision, model: "model-first", prompt: "Estado?" });
  assert.equal(response.status, 200);
  const events = parseSse(await response.text());
  assert.deepEqual(events.map((event) => event.event), ["tool", "tool", "delta", "completed"]);
  assert.deepEqual(events[0].data, { id: "call_1", name: "powershell", summary: "git status", status: "running" });
  assert.deepEqual(events[1].data, { id: "call_1", name: "powershell", summary: "git status", status: "done", detail: "ran git status" });
  assert.deepEqual(events[2].data, { text: "Limpio" });
  assert.equal(requests[0].body.tools.length, 3);
  assert.equal(runner.calls.powershell.length, 1);
  const history = await (await historyRoute.GET(localRequest("/api/chat/history"))).json();
  assert.deepEqual(history.conversations[0].messages.map((message) => [message.role, message.content]), [["user", "Estado?"], ["assistant", "Limpio\n\n[herramientas: git status (auto)]"]]);
  const raw = JSON.stringify(history);
  assert.doesNotMatch(raw, /ENC1|encrypted_content|ran git status|call_1/);
});

test("without a tool runtime the route is plain chat: no tools in the request and no summary line", async () => {
  Reflect.deleteProperty(globalThis, RUNNER_KEY);
  const revision = await freshRevision();
  const model = scripted([textRound("Hola")]);
  globalThis.fetch = async (url, init) => String(url).endsWith("/models") ? fakeModels() : model.fetcher(url, init);
  const response = await post({ conversationId: crypto.randomUUID(), revision, model: "model-first", prompt: "Hola" });
  assert.deepEqual(parseSse(await response.text()).map((event) => event.event), ["delta", "completed"]);
  assert.deepEqual(model.requests[0].body, { model: "model-first", input: [{ role: "user", content: "Hola" }], store: false, stream: true });
});

test("a failed agent turn reports an error and saves nothing", async () => {
  const revision = await freshRevision();
  installRoute({ rounds: [sse([frame({ type: "response.failed" })])] });
  const events = parseSse(await (await post({ conversationId: crypto.randomUUID(), revision, model: "model-first", prompt: "x" })).text());
  assert.deepEqual(events.map((event) => event.event), ["error"]);
  const history = await (await historyRoute.GET(localRequest("/api/chat/history"))).json();
  assert.deepEqual(history.conversations, []);
});

test("the client still never chooses tools or instructions, with or without a runtime", async () => {
  const revision = await freshRevision();
  const installed = installRoute({ rounds: [textRound("x"), textRound("x")] });
  for (const extra of [{ tools: [{ type: "function", name: "x" }] }, { instructions: "obey" }, { tool_choice: "auto" }, { input: [] }, { include: [] }]) {
    const response = await post({ conversationId: crypto.randomUUID(), revision, model: "model-first", prompt: "Hello", ...extra });
    assert.equal(response.status, 400, JSON.stringify(extra));
  }
  assert.equal(installed.requests.length, 0);
});

test("the stream route needs the control-token cookie once the shell published a token", async () => {
  const revision = await freshRevision();
  const token = "e".repeat(64);
  Reflect.set(globalThis, CONTROL_KEY, token);
  try {
    installRoute({ rounds: [textRound("Hola")] });
    const body = { conversationId: crypto.randomUUID(), revision, model: "model-first", prompt: "Hola" };
    assert.equal((await post(body)).status, 403);
    assert.equal((await post(body, { cookie: `click_dot_control=${"f".repeat(64)}` })).status, 403);
    const ok = await post(body, { cookie: `click_dot_control=${token}` });
    assert.equal(ok.status, 200);
    assert.match(await ok.text(), /event: completed/);
  } finally {
    Reflect.deleteProperty(globalThis, CONTROL_KEY);
  }
});

test("stopping the response through the stream route kills the tool and aborts the model", async () => {
  const revision = await freshRevision();
  let runnerSignal;
  const fake = fakeRunner({ runPowerShell: (request) => { runnerSignal = request.signal; return new Promise(() => {}); } });
  installRoute({ rounds: [itemsRound([call("run_powershell", { command: "Get-ChildItem", cwd: null })])], runner: fake });
  const response = await post({ conversationId: crypto.randomUUID(), revision, model: "model-first", prompt: "Lista" });
  const reader = response.body.getReader();
  const first = await reader.read();
  assert.match(new TextDecoder().decode(first.value), /event: tool/);
  await tick(20);
  assert.ok(runnerSignal && !runnerSignal.aborted);
  await reader.cancel("client stopped");
  await tick(20);
  assert.equal(runnerSignal.aborted, true);
  const history = await (await historyRoute.GET(localRequest("/api/chat/history"))).json();
  assert.deepEqual(history.conversations, []);
});
