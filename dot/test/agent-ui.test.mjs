import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { test } from "node:test";

process.env.PORT = "3010";

const state = await import("../lib/agent-state.ts");
const captureState = await import("../lib/capture-state.ts");
const windowMode = await import("../lib/window-mode.ts");
const card = await import("../app/mascot/agent-choices.ts");

const read = (path) => readFile(new URL(`../${path}`, import.meta.url), "utf8");

const session = (overrides = {}) => ({ id: "s1", project: "alpha", state: "trabajando", action: "Ejecutando npm test", updatedAt: 1, ...overrides });
const pending = (overrides = {}) => ({ id: "p1", kind: "claude-code", tool: "Bash", summary: "Ejecutando npm test", detail: "npm test", project: "alpha", alwaysOffered: true, questions: null, ...overrides });
const body = (overrides = {}) => ({ sessions: [session()], pending: null, queued: 0, ...overrides });

// --- reading the state -----------------------------------------------------------------------------------------

test("a well-formed state is read as it is", () => {
  const questions = [{ question: "Q?", header: "H", multiSelect: false, options: [{ label: "A", description: "" }] }];
  assert.deepEqual(state.readAgentState(body({ pending: pending({ questions }), queued: 2 })), body({ pending: pending({ questions }), queued: 2 }));
  assert.deepEqual(state.readAgentState({ sessions: [], pending: null, queued: 0 }), state.EMPTY_AGENT_STATE);
});

test("anything malformed is not a state", () => {
  const bad = [
    null,
    "x",
    [],
    {},
    { sessions: [], pending: null },
    { sessions: [], pending: null, queued: -1 },
    { sessions: [], pending: null, queued: 1.5 },
    body({ sessions: [session({ state: "dormido" })] }),
    body({ sessions: [session({ action: 5 })] }),
    body({ sessions: [session({ id: 5 })] }),
    body({ sessions: ["x"] }),
    body({ pending: pending({ alwaysOffered: "yes" }) }),
    body({ pending: pending({ kind: "other" }) }),
    body({ pending: { ...pending(), kind: undefined } }),
    body({ pending: pending({ questions: "x" }) }),
    body({ pending: pending({ questions: [{ question: "Q?", header: "H", multiSelect: "no", options: [] }] }) }),
    body({ pending: pending({ questions: [{ question: "Q?", header: "H", multiSelect: false, options: [{ label: 1, description: "" }] }] }) }),
    body({ pending: "x" }),
    body({ pending: undefined }),
  ];
  for (const value of bad) assert.equal(state.readAgentState(value), undefined, JSON.stringify(value));
});

test("a failed read changes nothing until it has failed several times in a row", () => {
  const known = { state: state.readAgentState(body()), failures: 0 };
  let poll = known;
  for (let i = 1; i < state.AGENT_FAILURE_LIMIT; i += 1) {
    poll = state.applyAgentPoll(poll, { ok: false });
    assert.deepEqual(poll.state, known.state);
  }
  poll = state.applyAgentPoll(poll, { ok: false });
  assert.deepEqual(poll.state, state.EMPTY_AGENT_STATE);
  assert.deepEqual(state.applyAgentPoll(poll, { ok: true, state: known.state }), known);
});

test("the page polls about twice a second, inside the 2 s acknowledgement window", () => {
  assert.equal(state.AGENT_POLL_MS, 500);
});

// --- what the mascot shows -------------------------------------------------------------------------------------

test("needing the user wins over working, and working wins over idle", () => {
  assert.equal(state.agentAttention(state.EMPTY_AGENT_STATE), "idle");
  assert.equal(state.agentAttention(body({ sessions: [session({ state: "listo" })] })), "idle");
  assert.equal(state.agentAttention(body()), "working");
  assert.equal(state.agentAttention(body({ sessions: [session(), session({ id: "s2", state: "te necesita" })] })), "needs");
  assert.equal(state.agentAttention(body({ sessions: [], pending: pending() })), "needs");
  assert.equal(state.agentAttention(body({ sessions: [session()], pending: pending() })), "needs");
});

test("the indicator adds Claude Code to the mascot without touching the capture or listening parts", () => {
  for (const indicator of ["live", "paused", "off", "unavailable"]) {
    for (const listening of [false, true]) {
      const before = captureState.mascotIndicator(indicator, listening);
      for (const agent of ["idle", "working", "needs"]) {
        const view = captureState.mascotIndicator(indicator, listening, agent);
        const { label: _label, agent: shown, ...rest } = view;
        const { label: _before, agent: idle, ...restBefore } = before;
        assert.deepEqual(rest, restBefore, `${indicator} ${listening} ${agent}`);
        assert.equal(shown, agent);
        assert.equal(idle, "idle");
      }
    }
  }
});

test("while sharing the screen, Claude Code needing the user never hides the capture ring, text or pause control", () => {
  const view = captureState.mascotIndicator("live", false, "needs");
  assert.equal(view.ring, "live");
  assert.equal(view.sharingText, "Viendo tu pantalla");
  assert.deepEqual(view.pause, { action: "pause", name: "Pausar", glyph: "pause" });
  assert.ok(view.label.startsWith("Viendo tu pantalla"), "capture is named first");
  assert.match(view.label, /Claude Code te necesita/);
  assert.match(captureState.mascotIndicator("off", true, "working").label, /Escuchando/);
});

test("the label names what Claude Code is doing, and the mascot alone still says Dot", () => {
  assert.equal(captureState.mascotIndicator("off", false, "idle").label, "Dot");
  assert.equal(captureState.mascotIndicator("off", false, "working").label, "Claude Code está trabajando");
  assert.equal(captureState.mascotIndicator("off", false, "needs").label, "Claude Code te necesita");
});

// --- window modes ----------------------------------------------------------------------------------------------

test("with a request waiting, clicking the collapsed or compact island brings the card back; otherwise the menu works as before", () => {
  assert.equal(windowMode.nextMascotMode("compact", true), "approval");
  assert.equal(windowMode.nextMascotMode("compact"), "menu");
  assert.equal(windowMode.nextMascotMode("collapsed"), "menu");
  assert.equal(windowMode.nextMascotMode("collapsed", false), "menu");
  assert.equal(windowMode.nextMascotMode("menu", true), "collapsed");
  assert.equal(windowMode.nextMascotMode("chat", true), "collapsed");
  assert.equal(windowMode.nextMascotMode("collapsed", true), "approval");
  assert.equal(windowMode.nextMascotMode("approval", true), "collapsed");
  assert.equal(windowMode.nextMascotMode("approval"), "collapsed");
});

// --- the card's choices ----------------------------------------------------------------------------------------

const single = { question: "Which database?", header: "DB", multiSelect: false, options: [{ label: "Postgres", description: "" }, { label: "SQLite", description: "" }] };
const multi = { question: "Which features?", header: "F", multiSelect: true, options: [{ label: "Auth", description: "" }, { label: "Billing", description: "" }, { label: "Search", description: "" }] };

test("one single-select question is answered with one click", () => {
  assert.equal(card.answersOnClick([single]), true);
  assert.equal(card.answersOnClick([multi]), false);
  assert.equal(card.answersOnClick([single, multi]), false);
  assert.equal(card.answersOnClick([single, { ...single, question: "Other?" }]), false);
  assert.deepEqual(card.singleClickAnswers(single, "SQLite"), { "Which database?": "SQLite" });
});

test("multi-select needs a confirm: toggling checks and unchecks, and Responder needs every question answered", () => {
  let selection = card.emptySelection([single, multi]);
  assert.equal(card.canSubmit([single, multi], selection), false);
  selection = card.toggleOption([single, multi], selection, 1, "Auth");
  selection = card.toggleOption([single, multi], selection, 1, "Search");
  assert.equal(card.canSubmit([single, multi], selection), false, "the single-select question is still open");
  selection = card.toggleOption([single, multi], selection, 0, "Postgres");
  selection = card.toggleOption([single, multi], selection, 0, "SQLite");
  assert.equal(card.canSubmit([single, multi], selection), true);
  assert.deepEqual(card.buildAnswers([single, multi], selection), { "Which database?": "SQLite", "Which features?": ["Auth", "Search"] });
  selection = card.toggleOption([single, multi], selection, 1, "Auth");
  selection = card.toggleOption([single, multi], selection, 1, "Search");
  assert.equal(card.canSubmit([single, multi], selection), false, "unchecking everything leaves the question open");
});

test("multi-select labels keep the order of the options, not the order of the clicks", () => {
  let selection = card.emptySelection([multi]);
  for (const label of ["Search", "Auth"]) selection = card.toggleOption([multi], selection, 0, label);
  assert.deepEqual(card.buildAnswers([multi], selection), { "Which features?": ["Auth", "Search"] });
});

test("a label that is not one of the question's options changes nothing", () => {
  const selection = card.emptySelection([single]);
  assert.deepEqual(card.toggleOption([single], selection, 0, "MySQL"), selection);
  assert.deepEqual(card.toggleOption([single], selection, 7, "Postgres"), selection);
});

// --- the page --------------------------------------------------------------------------------------------------

test("the existing labels the packaged smoke test asserts are still there, and the menu still has them as buttons", async () => {
  const mascot = await read("app/mascot/mascot.tsx");
  for (const label of ["Compartir mi pantalla", "Voz", "Mensaje", "Conectar con ChatGPT"]) assert.ok(mascot.includes(label), label);
  assert.match(mascot, /className="mascot-menu/);
  const toggles = mascot.match(/role="button"/g) ?? [];
  assert.equal(toggles.length, 1, "the mascot body stays the only role=button (the smoke test clicks the first one)");
});

test("the card offers Permitir, Denegar, Siempre only when it is offered, and Responder en la terminal", async () => {
  const source = await read("app/mascot/agent-card.tsx") + await read("app/mascot/agent-card-view.ts");
  for (const label of ["Permitir", "Denegar", "Siempre", "Responder en la terminal", "Responder"]) assert.ok(source.includes(label), label);
  assert.match(source, /alwaysOffered\s*&&/);
  assert.match(source, /"checkbox"/);
  assert.match(source, /multiSelect/);
  assert.match(source, /key=\{pending\.id\}/, "the card's own state is reset for every request");
});

test("the page lists sessions in the menu with project, state and latest action", async () => {
  const mascot = await read("app/mascot/mascot.tsx");
  assert.match(mascot, /className="menu-sessions"/);
  assert.match(mascot, /session\.project/);
  assert.match(mascot, /session\.state/);
  assert.match(mascot, /session\.action/);
  assert.match(mascot, /data-agent=/);
});

test("the page polls the state, acknowledges the card right after it renders, and posts decisions", async () => {
  const hook = await read("app/mascot/use-agent.ts");
  assert.match(hook, /AGENT_POLL_MS/);
  assert.match(hook, /\/api\/agent\/state/);
  assert.match(hook, /\/api\/agent\/ack/);
  assert.match(hook, /\/api\/agent\/decision/);
  assert.match(hook, /click-dot:agent/);
  const mascot = await read("app/mascot/mascot.tsx");
  assert.match(mascot, /useAgent\(/);
  assert.match(mascot, /agent\.ack\(/);
  assert.match(mascot, /setMode\("approval"\)/);
  assert.match(mascot, /mascotIndicator\(indicator, voice\.listening, attention\)/);
});

test("the styles define the new pieces", async () => {
  const css = await read("app/globals.css");
  for (const selector of [".agent-card", ".menu-sessions", ".island-capture", ".diff-add", '[data-mode="approval"]']) assert.ok(css.includes(selector), selector);
});
