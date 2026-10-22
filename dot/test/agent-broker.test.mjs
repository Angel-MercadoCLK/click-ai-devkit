import assert from "node:assert/strict";
import { test } from "node:test";

const brokerModule = await import("../lib/agent-broker.ts");

/** A manual clock: timers only fire when the test advances time. */
function fakeTimers() {
  let now = 0;
  let next = 1;
  const timers = new Map();
  return {
    now: () => now,
    setTimer: (callback, ms) => {
      const id = next++;
      timers.set(id, { at: now + ms, callback });
      return id;
    },
    clearTimer: (id) => void timers.delete(id),
    advance(ms) {
      const target = now + ms;
      for (;;) {
        const due = [...timers.entries()].filter(([, timer]) => timer.at <= target).sort((a, b) => a[1].at - b[1].at)[0];
        if (!due) break;
        timers.delete(due[0]);
        now = due[1].at;
        due[1].callback();
      }
      now = target;
    },
    pending: () => timers.size,
  };
}

function setup(options = {}) {
  const timers = fakeTimers();
  const shown = [];
  const broker = brokerModule.createAgentBroker({ ...timers, onPending: () => shown.push(timers.now()), ...options });
  return { broker, timers, shown };
}

const bash = (extra = {}) => ({
  session_id: "s1",
  cwd: "C:\\work\\alpha",
  tool_name: "Bash",
  tool_input: { command: "npm test" },
  permission_suggestions: [{ type: "addRules", rules: [{ toolName: "Bash", ruleContent: "npm test" }], behavior: "allow", destination: "localSettings" }],
  ...extra,
});

const question = (extra = {}) => ({
  session_id: "s1",
  cwd: "/work/alpha",
  tool_name: "AskUserQuestion",
  tool_input: {
    questions: [
      { question: "Which database?", header: "DB", multiSelect: false, options: [{ label: "Postgres", description: "SQL" }, { label: "SQLite", description: "file" }] },
      { question: "Which features?", header: "Features", multiSelect: true, options: [{ label: "Auth" }, { label: "Billing" }, { label: "Search" }] },
    ],
  },
  ...extra,
});

const settled = (promise) => {
  const box = { done: false, value: undefined };
  promise.then((value) => {
    box.done = true;
    box.value = value;
  });
  return box;
};
const tick = () => new Promise((resolve) => setImmediate(resolve));

// --- the pending card -----------------------------------------------------------------------------------------

test("a permission request becomes the pending card and asks the shell to show the mascot", () => {
  const { broker, shown } = setup();
  broker.enqueue({ sessionId: "s1", payload: bash() });
  const { pending, queued } = broker.state();
  assert.equal(shown.length, 1);
  assert.equal(queued, 0);
  assert.equal(pending.kind, "claude-code");
  assert.equal(pending.tool, "Bash");
  assert.equal(pending.project, "alpha");
  assert.equal(pending.summary, "Ejecutando npm test");
  assert.equal(pending.detail, "npm test");
  assert.equal(pending.alwaysOffered, true);
  assert.equal(pending.questions, null);
  assert.match(pending.id, /^[0-9a-f-]{36}$/);
  assert.equal(broker.state().sessions[0].state, "te necesita");
});

test("Siempre is only offered when Claude Code sent permission suggestions", () => {
  for (const suggestions of [undefined, [], "x", [null], ["text"], [[1]]]) {
    const { broker } = setup();
    broker.enqueue({ sessionId: "s1", payload: bash({ permission_suggestions: suggestions }) });
    assert.equal(broker.state().pending.alwaysOffered, false, JSON.stringify(suggestions));
  }
});

test("requests queue in arrival order and the card shows the oldest", () => {
  const { broker } = setup();
  broker.enqueue({ sessionId: "s1", payload: bash({ tool_input: { command: "first" } }) });
  broker.enqueue({ sessionId: "s2", payload: bash({ session_id: "s2", tool_input: { command: "second" } }) });
  assert.equal(broker.state().pending.detail, "first");
  assert.equal(broker.state().queued, 1);
});

test("the queue is bounded: past the limit a request is released at once without a card", async () => {
  const { broker } = setup({ maxPending: 2 });
  broker.enqueue({ sessionId: "s1", payload: bash() });
  broker.enqueue({ sessionId: "s1", payload: bash() });
  assert.deepEqual(await broker.enqueue({ sessionId: "s1", payload: bash() }), { kind: "none" });
  assert.equal(broker.state().queued, 1);
});

// --- decisions ------------------------------------------------------------------------------------------------

test("Permitir answers allow and Denegar answers deny, with nothing else", async () => {
  const { broker } = setup();
  const allow = settled(broker.enqueue({ sessionId: "s1", payload: bash() }));
  assert.deepEqual(broker.decide(broker.state().pending.id, "allow"), { ok: true });
  await tick();
  assert.deepEqual(allow.value, { kind: "decision", decision: { behavior: "allow" } });
  const deny = settled(broker.enqueue({ sessionId: "s1", payload: bash() }));
  broker.decide(broker.state().pending.id, "deny");
  await tick();
  assert.deepEqual(deny.value, { kind: "decision", decision: { behavior: "deny" } });
  assert.equal(broker.state().sessions[0].state, "trabajando");
});

test("Siempre allows and sends only the first suggestion as updatedPermissions", async () => {
  const { broker } = setup();
  const second = { type: "addRules", rules: [{ toolName: "Bash", ruleContent: "other" }], behavior: "allow", destination: "session" };
  const payload = bash({ permission_suggestions: [bash().permission_suggestions[0], second] });
  const result = settled(broker.enqueue({ sessionId: "s1", payload }));
  assert.deepEqual(broker.decide(broker.state().pending.id, "always"), { ok: true });
  await tick();
  assert.deepEqual(result.value, { kind: "decision", decision: { behavior: "allow", updatedPermissions: [payload.permission_suggestions[0]] } });
});

test("Siempre without a suggestion is refused and leaves the card in place", () => {
  const { broker } = setup();
  broker.enqueue({ sessionId: "s1", payload: bash({ permission_suggestions: [] }) });
  const refused = broker.decide(broker.state().pending.id, "always");
  assert.equal(refused.ok, false);
  assert.equal(refused.status, 400);
  assert.notEqual(broker.state().pending, null);
});

test("an unknown request id or choice is refused", () => {
  const { broker } = setup();
  broker.enqueue({ sessionId: "s1", payload: bash() });
  assert.deepEqual(broker.decide("nope", "allow"), { ok: false, status: 404, error: "There is no such pending request." });
  assert.equal(broker.decide(broker.state().pending.id, "maybe").status, 400);
  assert.equal(broker.decide(broker.state().pending.id, "allow", { x: "y" }).status, 400, "answers belong to questions only");
  assert.notEqual(broker.state().pending, null);
});

test("a decision for a request that is already gone is a 404, not a second answer", async () => {
  const { broker } = setup();
  const result = settled(broker.enqueue({ sessionId: "s1", payload: bash() }));
  const { id } = broker.state().pending;
  broker.decide(id, "allow");
  assert.equal(broker.decide(id, "deny").status, 404);
  await tick();
  assert.equal(result.value.decision.behavior, "allow");
});

// --- release without a decision -------------------------------------------------------------------------------

test("without an acknowledgement the request is released after 2 seconds and not before", async () => {
  const { broker, timers } = setup();
  const result = settled(broker.enqueue({ sessionId: "s1", payload: bash() }));
  timers.advance(1_999);
  await tick();
  assert.equal(result.done, false);
  timers.advance(1);
  await tick();
  assert.deepEqual(result.value, { kind: "none" });
  assert.equal(broker.state().pending, null);
});

test("an acknowledged card waits for the user up to 105 seconds and then lets go", async () => {
  const { broker, timers } = setup();
  const result = settled(broker.enqueue({ sessionId: "s1", payload: bash() }));
  assert.equal(broker.ack(broker.state().pending.id), true);
  timers.advance(104_999);
  await tick();
  assert.equal(result.done, false);
  timers.advance(1);
  await tick();
  assert.deepEqual(result.value, { kind: "none" });
});

test("Responder en la terminal releases the request with no decision", async () => {
  const { broker } = setup();
  const result = settled(broker.enqueue({ sessionId: "s1", payload: bash() }));
  assert.deepEqual(broker.decide(broker.state().pending.id, "terminal"), { ok: true });
  await tick();
  assert.deepEqual(result.value, { kind: "none" });
  assert.equal(broker.state().sessions[0].state, "te necesita", "the terminal is now asking");
});

test("a client that disconnects releases its request", async () => {
  const { broker } = setup();
  const controller = new AbortController();
  const result = settled(broker.enqueue({ sessionId: "s1", payload: bash(), signal: controller.signal }));
  controller.abort();
  await tick();
  assert.deepEqual(result.value, { kind: "none" });
  assert.equal(broker.state().pending, null);
  assert.equal(broker.state().sessions[0].state, "trabajando");
  const already = new AbortController();
  already.abort();
  assert.deepEqual(await broker.enqueue({ sessionId: "s1", payload: bash(), signal: already.signal }), { kind: "none" });
});

test("quitting releases every request, shown or queued", async () => {
  const { broker } = setup();
  const first = settled(broker.enqueue({ sessionId: "s1", payload: bash() }));
  const second = settled(broker.enqueue({ sessionId: "s2", payload: bash({ session_id: "s2" }) }));
  broker.releaseAll();
  await tick();
  assert.deepEqual(first.value, { kind: "none" });
  assert.deepEqual(second.value, { kind: "none" });
  assert.equal(broker.state().pending, null);
});

// --- queue timing ---------------------------------------------------------------------------------------------

test("the 2 second acknowledgement window of a queued request starts when its card is shown", async () => {
  const { broker, timers, shown } = setup();
  const first = settled(broker.enqueue({ sessionId: "s1", payload: bash() }));
  const second = settled(broker.enqueue({ sessionId: "s2", payload: bash({ session_id: "s2" }) }));
  broker.ack(broker.state().pending.id);
  timers.advance(10_000);
  await tick();
  assert.equal(second.done, false, "a queued request is not dropped for lack of an ack while another card is shown");
  broker.decide(broker.state().pending.id, "allow");
  await tick();
  assert.equal(first.value.decision.behavior, "allow");
  assert.equal(shown.length, 2, "the next card asks the shell to show the mascot again");
  timers.advance(1_999);
  await tick();
  assert.equal(second.done, false);
  timers.advance(1);
  await tick();
  assert.deepEqual(second.value, { kind: "none" });
});

test("the 105 second limit counts from arrival even while a request waits in the queue", async () => {
  const { broker, timers } = setup();
  settled(broker.enqueue({ sessionId: "s1", payload: bash() }));
  const second = settled(broker.enqueue({ sessionId: "s2", payload: bash({ session_id: "s2" }) }));
  broker.ack(broker.state().pending.id);
  timers.advance(105_000);
  await tick();
  assert.deepEqual(second.value, { kind: "none" });
});

test("acknowledging a request that is not the shown one does nothing", () => {
  const { broker } = setup();
  broker.enqueue({ sessionId: "s1", payload: bash() });
  assert.equal(broker.ack("nope"), false);
});

test("timers do not leak: a settled request leaves none behind", async () => {
  const { broker, timers } = setup();
  const result = settled(broker.enqueue({ sessionId: "s1", payload: bash() }));
  broker.ack(broker.state().pending.id);
  broker.decide(broker.state().pending.id, "allow");
  await tick();
  assert.equal(result.done, true);
  assert.equal(timers.pending(), 0);
});

// --- questions ------------------------------------------------------------------------------------------------

test("a question request shows its questions, options and multi-select flag, and never offers Siempre", () => {
  const { broker } = setup();
  broker.enqueue({ sessionId: "s1", payload: question({ permission_suggestions: bash().permission_suggestions }) });
  const { pending } = broker.state();
  assert.equal(pending.alwaysOffered, false);
  assert.deepEqual(pending.questions, [
    { question: "Which database?", header: "DB", multiSelect: false, options: [{ label: "Postgres", description: "SQL" }, { label: "SQLite", description: "file" }] },
    { question: "Which features?", header: "Features", multiSelect: true, options: [{ label: "Auth", description: "" }, { label: "Billing", description: "" }, { label: "Search", description: "" }] },
  ]);
  assert.equal(broker.decide(pending.id, "always").status, 400);
});

test("a question answered with exact labels returns allow with the answers; multi-select labels are joined with a comma and space", async () => {
  const { broker } = setup();
  const result = settled(broker.enqueue({ sessionId: "s1", payload: question() }));
  const answers = { "Which database?": "SQLite", "Which features?": ["Auth", "Search"] };
  assert.deepEqual(broker.decide(broker.state().pending.id, "allow", answers), { ok: true });
  await tick();
  assert.deepEqual(result.value, { kind: "decision", decision: { behavior: "allow", answers: { "Which database?": "SQLite", "Which features?": "Auth, Search" } } });
});

test("answers that do not match the questions exactly are refused and the card stays", () => {
  const { broker } = setup();
  broker.enqueue({ sessionId: "s1", payload: question() });
  const id = broker.state().pending.id;
  const good = { "Which database?": "SQLite", "Which features?": ["Auth"] };
  const bad = [
    undefined,
    {},
    { "Which database?": "SQLite" },
    { ...good, "Extra?": "x" },
    { ...good, "Which database?": "sqlite" },
    { ...good, "Which database?": "MySQL" },
    { ...good, "Which database?": ["SQLite"] },
    { ...good, "Which features?": "Auth" },
    { ...good, "Which features?": [] },
    { ...good, "Which features?": ["Auth", "Auth"] },
    { ...good, "Which features?": ["Auth", "Nope"] },
    { ...good, "which database?": "SQLite" },
    "text",
    [],
  ];
  for (const answers of bad) {
    const refused = broker.decide(id, "allow", answers);
    assert.equal(refused.ok, false, JSON.stringify(answers));
    assert.equal(refused.status, 400);
  }
  assert.notEqual(broker.state().pending, null);
  assert.deepEqual(broker.decide(id, "allow", good), { ok: true });
});

test("denying a question is allowed without answers", async () => {
  const { broker } = setup();
  const result = settled(broker.enqueue({ sessionId: "s1", payload: question() }));
  broker.decide(broker.state().pending.id, "deny");
  await tick();
  assert.deepEqual(result.value, { kind: "decision", decision: { behavior: "deny" } });
});

test("a question request whose questions cannot be shown is handed back to the terminal at once", async () => {
  const { broker } = setup();
  for (const tool_input of [{}, { questions: [] }, { questions: "x" }, { questions: [{ question: "q", options: [] }] }, { questions: [{ question: "", options: [{ label: "a" }] }] }, { questions: [{ question: "q", options: [{ label: "" }] }] }]) {
    assert.deepEqual(await broker.enqueue({ sessionId: "s1", payload: question({ tool_input }) }), { kind: "none" }, JSON.stringify(tool_input));
  }
  assert.equal(broker.state().pending, null);
});

// --- plain events ---------------------------------------------------------------------------------------------

test("recording an event updates the sessions without a card", () => {
  const { broker } = setup();
  broker.record("PreToolUse", { session_id: "s1", cwd: "/p/alpha", tool_name: "Read", tool_input: { file_path: "a.ts" } });
  assert.equal(broker.state().sessions[0].action, "Leyendo a.ts");
  assert.equal(broker.state().pending, null);
});

test("the pending card never exposes the working directory or the raw payload", () => {
  const { broker } = setup();
  broker.enqueue({ sessionId: "s1", payload: bash({ transcript_path: "C:\\secret\\t.jsonl" }) });
  const serialized = JSON.stringify(broker.state());
  assert.doesNotMatch(serialized, /secret|C:\\\\work/);
});

// --- Dot tool requests (kind "dot-tool") ----------------------------------------------------------------------

const powershell = (extra = {}) => ({
  tool: "PowerShell",
  summary: "Remove-Item old.txt",
  detail: "Carpeta: C:\\work\\alpha\nPuede modificar tu equipo",
  alwaysOffered: true,
  ...extra,
});

test("a Dot tool request becomes the pending card with its own kind, label, command and detail", () => {
  const { broker, shown } = setup();
  broker.requestToolApproval(powershell());
  const { pending, queued, sessions } = broker.state();
  assert.equal(shown.length, 1);
  assert.equal(queued, 0);
  assert.deepEqual({ ...pending, id: "x" }, {
    id: "x",
    kind: "dot-tool",
    tool: "PowerShell",
    summary: "Remove-Item old.txt",
    detail: "Carpeta: C:\\work\\alpha\nPuede modificar tu equipo",
    project: "Dot",
    alwaysOffered: true,
    questions: null,
  });
  assert.deepEqual(sessions, [], "a Dot request is not a Claude Code session");
});

test("Siempre is offered for a Dot request only when the request asks for it", () => {
  const { broker } = setup();
  broker.requestToolApproval(powershell({ tool: "Claude Code", alwaysOffered: false }));
  assert.equal(broker.state().pending.alwaysOffered, false);
  assert.deepEqual(broker.decide(broker.state().pending.id, "always"), { ok: false, status: 400, error: "Siempre is not offered for this request." });
  assert.notEqual(broker.state().pending, null, "the refused choice leaves the card in place");
});

test("Permitir, Denegar and Siempre answer a Dot request with exactly that choice", async () => {
  const { broker } = setup();
  for (const choice of ["allow", "deny", "always"]) {
    const answer = settled(broker.requestToolApproval(powershell()));
    assert.deepEqual(broker.decide(broker.state().pending.id, choice), { ok: true });
    await tick();
    assert.equal(answer.value, choice);
  }
});

test("a Dot request is never handed to a terminal and carries no answers", () => {
  const { broker } = setup();
  broker.requestToolApproval(powershell());
  const { id } = broker.state().pending;
  assert.equal(broker.decide(id, "terminal").ok, false);
  assert.equal(broker.decide(id, "allow", { q: "a" }).ok, false);
  assert.notEqual(broker.state().pending, null);
});

test("a Dot request keeps the ack and decision timers: no ack in 2 s or no decision in 105 s means none", async () => {
  const { broker, timers } = setup();
  const unacked = settled(broker.requestToolApproval(powershell()));
  timers.advance(1_999);
  await tick();
  assert.equal(unacked.done, false);
  timers.advance(1);
  await tick();
  assert.equal(unacked.value, "none");

  const acked = settled(broker.requestToolApproval(powershell()));
  assert.equal(broker.ack(broker.state().pending.id), true);
  timers.advance(104_999);
  await tick();
  assert.equal(acked.done, false);
  timers.advance(1);
  await tick();
  assert.equal(acked.value, "none");
  assert.equal(timers.pending(), 0);
});

test("an aborted Dot request releases its card and Dot requests queue behind Claude Code requests in arrival order", async () => {
  const { broker } = setup();
  broker.enqueue({ sessionId: "s1", payload: bash() });
  const abort = new AbortController();
  const dot = settled(broker.requestToolApproval(powershell({ signal: abort.signal })));
  assert.equal(broker.state().pending.kind, "claude-code");
  assert.equal(broker.state().queued, 1);
  abort.abort();
  await tick();
  assert.equal(dot.value, "none");
  assert.equal(broker.state().queued, 0);
  const aborted = new AbortController();
  aborted.abort();
  assert.equal(await broker.requestToolApproval(powershell({ signal: aborted.signal })), "none");
});

test("quitting releases a Dot request too, and a full queue refuses at once", async () => {
  const { broker } = setup({ maxPending: 1 });
  const first = settled(broker.requestToolApproval(powershell()));
  assert.equal(await broker.requestToolApproval(powershell()), "none");
  broker.releaseAll();
  await tick();
  assert.equal(first.value, "none");
});

// --- Codex requests (kind "codex") ----------------------------------------------------------------------------

const codex = (extra = {}) => ({
  tool: "Codex",
  summary: "npm test",
  detail: "Carpeta: C:\\work\\alpha",
  alwaysOffered: true,
  ...extra,
});

const codexQuestions = [
  { question: "Which database?", header: "DB", multiSelect: false, options: [{ label: "Postgres", description: "SQL" }, { label: "SQLite", description: "" }] },
];

test("a Codex request becomes a pending card of kind codex and is not a Claude Code session", () => {
  const { broker, shown } = setup();
  broker.requestCodexApproval(codex());
  const { pending, sessions } = broker.state();
  assert.equal(shown.length, 1);
  assert.deepEqual({ ...pending, id: "x" }, { id: "x", kind: "codex", tool: "Codex", summary: "npm test", detail: "Carpeta: C:\\work\\alpha", project: "Codex", alwaysOffered: true, questions: null });
  assert.deepEqual(sessions, []);
});

test("Permitir, Denegar and Siempre answer a Codex request with exactly that choice, Siempre only when offered", async () => {
  const { broker } = setup();
  for (const choice of ["allow", "deny", "always"]) {
    const answer = settled(broker.requestCodexApproval(codex()));
    assert.deepEqual(broker.decide(broker.state().pending.id, choice), { ok: true });
    await tick();
    assert.deepEqual(answer.value, { choice });
  }
  broker.requestCodexApproval(codex({ alwaysOffered: false }));
  assert.deepEqual(broker.decide(broker.state().pending.id, "always"), { ok: false, status: 400, error: "Siempre is not offered for this request." });
  assert.equal(broker.decide(broker.state().pending.id, "terminal").ok, false);
});

test("a Codex request keeps the 2 s ack and 105 s decision timers and answers none when they run out", async () => {
  const { broker, timers } = setup();
  const unacked = settled(broker.requestCodexApproval(codex()));
  timers.advance(2_000);
  await tick();
  assert.deepEqual(unacked.value, { choice: "none" });
  const acked = settled(broker.requestCodexApproval(codex()));
  assert.equal(broker.ack(broker.state().pending.id), true);
  timers.advance(105_000);
  await tick();
  assert.deepEqual(acked.value, { choice: "none" });
  assert.equal(timers.pending(), 0);
});

test("a Codex request with questions needs answers that match them, and carries them back keyed by question", async () => {
  const { broker } = setup();
  const answer = settled(broker.requestCodexApproval(codex({ summary: "Te hace una pregunta", questions: codexQuestions, alwaysOffered: false })));
  const { id, questions } = broker.state().pending;
  assert.deepEqual(questions, codexQuestions);
  assert.equal(broker.decide(id, "allow").ok, false, "answers are required");
  assert.equal(broker.decide(id, "allow", { "Which database?": "Oracle" }).ok, false, "a label that is not an option is refused");
  assert.equal(broker.decide(id, "always").ok, false);
  assert.equal(broker.decide(id, "deny", { "Which database?": "Postgres" }).ok, false, "a denial carries no answers");
  assert.deepEqual(broker.decide(id, "allow", { "Which database?": "SQLite" }), { ok: true });
  await tick();
  assert.deepEqual(answer.value, { choice: "allow", answers: { "Which database?": "SQLite" } });
});

test("a Codex request aborted by its caller, a full queue and quitting all answer none", async () => {
  const { broker } = setup({ maxPending: 2 });
  const abort = new AbortController();
  const first = settled(broker.requestCodexApproval(codex({ signal: abort.signal })));
  const second = settled(broker.requestCodexApproval(codex()));
  assert.deepEqual(await broker.requestCodexApproval(codex()), { choice: "none" });
  abort.abort();
  await tick();
  assert.deepEqual(first.value, { choice: "none" });
  broker.releaseAll();
  await tick();
  assert.deepEqual(second.value, { choice: "none" });
  const aborted = new AbortController();
  aborted.abort();
  assert.deepEqual(await broker.requestCodexApproval(codex({ signal: aborted.signal })), { choice: "none" });
});

test("the Dot tool and Claude Code behaviors are unchanged next to Codex requests", async () => {
  const { broker } = setup();
  const dot = settled(broker.requestToolApproval(powershell()));
  const code = settled(broker.requestCodexApproval(codex()));
  assert.equal(broker.state().pending.kind, "dot-tool");
  assert.equal(broker.state().queued, 1);
  assert.deepEqual(broker.decide(broker.state().pending.id, "always"), { ok: true });
  await tick();
  assert.equal(dot.value, "always");
  assert.equal(broker.state().pending.kind, "codex");
  assert.equal(broker.decide(broker.state().pending.id, "allow", { q: "a" }).ok, false);
  broker.decide(broker.state().pending.id, "deny");
  await tick();
  assert.deepEqual(code.value, { choice: "deny" });
});
