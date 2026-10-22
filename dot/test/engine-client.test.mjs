import assert from "node:assert/strict";
import { test } from "node:test";

const state = await import("../lib/agent-state.ts");
const island = await import("../app/mascot/island-state.ts");

const task = (extra = {}) => ({ id: "task_aaaaaaaaaaaa", project: "/home/u/proj-x", state: "running", latestLine: "Editando a.ts", startedAt: 1_000_000, ...extra });

test("the page reads the activity answer and refuses anything malformed", () => {
  const read = state.readAgentActivity({ tasks: [task()], computerUse: { active: true, servers: ["computer"] } });
  assert.deepEqual(read, { tasks: [task()], computerUse: { active: true, servers: ["computer"] } });
  assert.equal(state.readAgentActivity({ tasks: [], computerUse: { active: false, servers: [] } }).tasks.length, 0);
  assert.equal(state.readAgentActivity({ tasks: [task({ state: "weird" })], computerUse: { active: false, servers: [] } }), undefined);
  assert.equal(state.readAgentActivity({ tasks: [task({ id: 3 })], computerUse: { active: false, servers: [] } }), undefined);
  assert.equal(state.readAgentActivity({ tasks: [task({ latestLine: 3 })], computerUse: { active: false, servers: [] } }), undefined);
  assert.equal(state.readAgentActivity({ tasks: [], computerUse: { active: "yes", servers: [] } }), undefined);
  assert.equal(state.readAgentActivity({ tasks: "x" }), undefined);
  assert.equal(state.readAgentActivity(null), undefined);
});

test("background Claude tasks show as sessions: running works, finished is ready with a plain outcome line, old ones are dropped", () => {
  const now = 1_000_000 + 60_000;
  const sessions = state.claudeTaskSessions(
    [
      task(),
      task({ id: "task_bbbbbbbbbbbb", state: "done", latestLine: "Listo: 3 archivos", startedAt: now - 120_000 }),
      task({ id: "task_cccccccccccc", state: "failed", latestLine: null, startedAt: now - 130_000 }),
      task({ id: "task_dddddddddddd", state: "cancelled", latestLine: null, startedAt: now - 140_000 }),
      task({ id: "task_eeeeeeeeeeee", state: "timed_out", latestLine: null, startedAt: now - 150_000 }),
      task({ id: "task_ffffffffffff", state: "done", startedAt: now - 3 * 60 * 60_000 }),
      task({ id: "task_gggggggggggg", state: "running", latestLine: null, startedAt: now - 3 * 60 * 60_000 }),
    ],
    now,
  );
  assert.deepEqual(sessions.map((session) => [session.id, session.state, session.action]), [
    ["task:task_aaaaaaaaaaaa", "trabajando", "Editando a.ts"],
    ["task:task_bbbbbbbbbbbb", "listo", "Listo: 3 archivos"],
    ["task:task_cccccccccccc", "listo", "Terminó con error"],
    ["task:task_dddddddddddd", "listo", "Cancelada"],
    ["task:task_eeeeeeeeeeee", "listo", "Se agotó el tiempo"],
    ["task:task_gggggggggggg", "trabajando", "Claude Code está trabajando"],
  ]);
  assert.equal(sessions[0].project, "proj-x (tarea)");
  assert.equal(state.claudeTaskSessions([], now).length, 0);
});

test("a running task makes the island work, and the mix with Claude Code sessions keeps both", () => {
  const sessions = state.claudeTaskSessions([task()], 1_000_100);
  assert.equal(state.agentAttention({ sessions, pending: null, queued: 0 }), "working");
  const merged = state.mergeSessions([{ id: "s1", project: "p", state: "listo", action: null, updatedAt: 1 }], sessions);
  assert.deepEqual(merged.map((session) => session.id), ["s1", "task:task_aaaaaaaaaaaa"]);
  assert.equal(island.compactLine({ sessions, pending: null, listening: false, finishedRecently: false }), "Editando a.ts");
});

test("while Dot uses the computer the island says so, is never at rest, and keeps the pill when hidden", () => {
  assert.equal(state.COMPUTER_USE_TEXT, "Dot está usando tu equipo");
  assert.equal(island.isActive({ attention: "idle", sharing: false, listening: false, finishedRecently: false, computerUse: true }), true);
  assert.equal(island.isActive({ attention: "idle", sharing: false, listening: false, finishedRecently: false }), false);
  assert.equal(island.restingMode({ active: true, hovered: false, hidden: true, sharing: true }), "compact");
  assert.equal(island.compactLine({ sessions: [], pending: null, listening: false, finishedRecently: false, computerUse: true }), "Dot está usando tu equipo");
  assert.equal(island.compactLine({ sessions: [], pending: null, listening: false, finishedRecently: false }), "Dot");
});

test("a failed activity read keeps what was known and three in a row forget it", () => {
  const known = { tasks: [task()], computerUse: { active: true, servers: ["chrome"] } };
  let poll = state.applyActivityPoll(state.INITIAL_ACTIVITY_POLL, { ok: true, activity: known });
  assert.deepEqual(poll.activity, known);
  poll = state.applyActivityPoll(poll, { ok: false });
  poll = state.applyActivityPoll(poll, { ok: false });
  assert.deepEqual(poll.activity, known);
  poll = state.applyActivityPoll(poll, { ok: false });
  assert.deepEqual(poll.activity, state.EMPTY_ACTIVITY, "an indicator must never stay on for a stale reading");
});

// The chat card: engine status, gate, history and attachments ------------------------------------------------------

const client = await import("../lib/engine-client.ts");

const status = (overrides = {}) => ({
  engine: "builtin",
  codex: { state: "absent", message: null, version: null, account: null, login: { state: "idle" }, ...(overrides.codex ?? {}) },
  tools: null,
  ...(overrides.engine ? { engine: overrides.engine } : {}),
  ...(overrides.tools ? { tools: overrides.tools } : {}),
});
const signedIn = { state: "signedIn", email: "a@b.c", plan: "plus" };
const signedOut = { state: "signedOut", email: null, plan: null };

test("the page reads the engine status and refuses anything malformed", () => {
  const good = status({ engine: "codex", codex: { state: "ready", version: "0.163.0", account: signedIn } });
  assert.deepEqual(client.readEngineStatus(good), good);
  assert.equal(client.readEngineStatus({ ...good, engine: "other" }), undefined);
  assert.equal(client.readEngineStatus({ ...good, codex: { ...good.codex, state: "weird" } }), undefined);
  assert.equal(client.readEngineStatus({ ...good, codex: { ...good.codex, login: { state: "x" } } }), undefined);
  assert.equal(client.readEngineStatus({ ...good, codex: { ...good.codex, account: { state: "x" } } }), undefined);
  assert.equal(client.readEngineStatus(null), undefined);
  const withTools = status({ tools: { chrome: { state: "ready", hint: "h", link: "https://x.test" }, computer: { state: "missing", hint: "m" } } });
  assert.equal(client.readEngineStatus(withTools).tools.chrome.link, "https://x.test");
  assert.equal(client.readEngineStatus({ ...withTools, tools: { chrome: 1 } }), undefined);
});

test("the notice says why Codex is not the engine, in Spanish, and says nothing when it is or when it was never there", () => {
  assert.equal(client.engineNotice(status()), null);
  assert.equal(client.engineNotice(status({ engine: "codex", codex: { state: "ready", account: signedIn } })), null);
  assert.equal(client.engineNotice(status({ codex: { state: "unavailable", message: "Dot no encontró Codex en este equipo." } })), "Dot no encontró Codex en este equipo.");
  assert.match(client.engineNotice(status({ codex: { state: "starting" } })), /Iniciando Codex/);
  assert.match(client.engineNotice(status({ codex: { state: "restarting" } })), /reiniciando/);
  assert.match(client.engineNotice(status({ codex: { state: "ready", account: signedOut } })), /no has iniciado sesión/);
  assert.equal(client.engineNotice(status({ codex: { state: "ready", account: { state: "unavailable", email: null, plan: null, message: "Dot no pudo leer tu sesión de Codex." } } })), "Dot no pudo leer tu sesión de Codex.");
});

test("the chat gate: Codex needs no Dot sign-in, everything else keeps the old gate, and 'Conectar con ChatGPT' stays the built-in sign-in", () => {
  const codex = status({ engine: "codex", codex: { state: "ready", account: signedIn } });
  assert.deepEqual(client.chatGate({ authConnected: false, engine: codex }), { kind: "chat", engine: "codex" });
  assert.deepEqual(client.chatGate({ authConnected: null, engine: codex }), { kind: "chat", engine: "codex" }, "no need to wait for Dot's own status");
  assert.deepEqual(client.chatGate({ authConnected: true, engine: status() }), { kind: "chat", engine: "builtin" });
  assert.deepEqual(client.chatGate({ authConnected: null, engine: status() }), { kind: "checking" });
  assert.deepEqual(client.chatGate({ authConnected: true, engine: null }), { kind: "checking" });
  const missing = status({ codex: { state: "unavailable", message: "Dot no encontró Codex en este equipo." } });
  assert.deepEqual(client.chatGate({ authConnected: false, engine: missing }), { kind: "connect", notice: "Dot no encontró Codex en este equipo.", codexLogin: false });
  const out = status({ codex: { state: "ready", account: signedOut } });
  assert.equal(client.chatGate({ authConnected: false, engine: out }).codexLogin, true);
  assert.equal(client.chatGate({ authConnected: true, engine: out }).kind, "chat", "Dot's own sign-in still chats while Codex is signed out");
  assert.equal(client.chatGate({ authConnected: false, engine: status() }).codexLogin, false);
});

test("a Codex thread list becomes history rows, keeping the messages of chats already open and the chat in progress", () => {
  const open = [{ id: "t1", title: "Viejo", updatedAt: 1, messages: [{ role: "user", content: "hola", createdAt: 1 }] }];
  const listed = [{ id: "t1", title: "Chat 1", updatedAt: 5, cwd: "/p" }, { id: "t2", title: "Chat 2", updatedAt: 9, cwd: "/p" }];
  const merged = client.mergeThreads(open, listed, null);
  assert.deepEqual(merged.map((item) => [item.id, item.title, item.updatedAt, item.messages.length]), [["t1", "Chat 1", 5, 1], ["t2", "Chat 2", 9, 0]]);
  const keeping = client.mergeThreads([{ id: "t3", title: "Nuevo", updatedAt: 20, messages: [{ role: "user", content: "x", createdAt: 1 }] }], listed, "t3");
  assert.deepEqual(keeping.map((item) => item.id), ["t3", "t1", "t2"], "the chat being written stays even before the list shows it");
  assert.deepEqual(client.mergeThreads(open, [], null), []);
});

test("a finished turn is added to its conversation at once, with the first words as the title", () => {
  const first = client.appendTurn([], { id: "t1", prompt: "  Resume\nel archivo  ", answer: "Listo", now: 100 });
  assert.deepEqual(first, [{ id: "t1", title: "Resume el archivo", updatedAt: 100, messages: [{ role: "user", content: "Resume\nel archivo", createdAt: 100 }, { role: "assistant", content: "Listo", createdAt: 100 }] }]);
  const second = client.appendTurn(first, { id: "t1", prompt: "Gracias", answer: "De nada", now: 200 });
  assert.equal(second.length, 1);
  assert.equal(second[0].updatedAt, 200);
  assert.equal(second[0].messages.length, 4);
  assert.equal(second[0].title, "Resume el archivo");
  assert.equal(client.appendTurn([], { id: "t9", prompt: "x".repeat(200), answer: "", now: 1 })[0].title.length <= 60, true);
});

test("an opened thread reads into a conversation with its messages, and anything malformed is refused", () => {
  const read = client.readThreadResponse({ thread: { id: "t1", title: "Mi chat", updatedAt: 7, cwd: "/p", messages: [{ role: "user", text: "hola" }, { role: "assistant", text: "qué tal" }] } });
  assert.deepEqual(read, { id: "t1", title: "Mi chat", updatedAt: 7, messages: [{ role: "user", content: "hola", createdAt: 7 }, { role: "assistant", content: "qué tal", createdAt: 7 }] });
  assert.equal(client.readThreadResponse({ thread: { id: "t1", title: "x", updatedAt: 1, messages: [{ role: "robot", text: "x" }] } }), undefined);
  assert.equal(client.readThreadResponse({ thread: null }), undefined);
  const list = client.readThreadList({ threads: [{ id: "t1", title: "A", updatedAt: 1, cwd: "/p" }], nextCursor: null });
  assert.equal(list.threads.length, 1);
  assert.equal(client.readThreadList({ threads: [{ id: 1 }] }), undefined);
});

test("attachments: the upload answer is read, the chip count is bounded, and pasted or dropped files are told apart", () => {
  assert.deepEqual(client.readUpload({ id: "att_0123456789abcdef", name: "a.png", kind: "image", bytes: 5 }), { id: "att_0123456789abcdef", name: "a.png", kind: "image", bytes: 5 });
  assert.equal(client.readUpload({ id: "../x", name: "a", kind: "image", bytes: 5 }), undefined);
  assert.equal(client.readUpload({ id: "att_0123456789abcdef", name: "a", kind: "script", bytes: 5 }), undefined);
  assert.equal(client.MAX_ATTACHMENTS, 6);
  assert.equal(client.roomForAttachments(4, 5), 2, "only two more fit");
  assert.equal(client.roomForAttachments(6, 1), 0);
  assert.match(client.attachmentsExplain("builtin"), /Codex/);
});

// Cards ------------------------------------------------------------------------------------------------------------

const cardView = await import("../app/mascot/agent-card-view.ts");
const choices = await import("../app/mascot/agent-choices.ts");

const pending = (extra = {}) => ({ id: "p1", kind: "codex", tool: "Codex", summary: "npm test", detail: "npm test\nCarpeta: /p\nMotivo: probar", project: "Codex", alwaysOffered: true, questions: null, ...extra });

test("each kind of request gets its own source, title and actions; Claude Code and Dot's tools are unchanged", () => {
  const codex = cardView.cardView(pending());
  assert.deepEqual([codex.source, codex.sourceLabel, codex.ariaLabel, codex.showTerminal, codex.alwaysTitle], ["codex", "Codex", "Codex te pide permiso", false, "Permitir esto hasta que cierres Dot"]);
  const claude = cardView.cardView(pending({ kind: "claude-code", tool: "Bash", project: "proj" }));
  assert.deepEqual([claude.source, claude.sourceLabel, claude.ariaLabel, claude.showTerminal, claude.alwaysTitle], ["claude-code", "Claude Code", "Claude Code te necesita en proj", true, "Permitir y recordar esta regla"]);
  const own = cardView.cardView(pending({ kind: "dot-tool", tool: "PowerShell", project: "Dot" }));
  assert.deepEqual([own.source, own.sourceLabel, own.ariaLabel, own.showTerminal, own.alwaysTitle], ["dot-tool", "Dot", "Dot quiere usar PowerShell", false, "Permitir este comando exacto hasta que cierres Dot"]);
  assert.equal(cardView.cardView(pending({ kind: "claude-code" })).failedText, "No se pudo enviar tu respuesta. Responde en la terminal.");
  assert.equal(codex.failedText, "No se pudo enviar tu respuesta. Inténtalo de nuevo.");
});

test("a Codex card shows the command or files as the detail and the summary only when the detail does not already say it", () => {
  assert.deepEqual(cardView.codexCardBody(pending()), { heading: "Codex quiere ejecutar o cambiar algo en tu equipo", summary: null, detail: "npm test\nCarpeta: /p\nMotivo: probar" });
  const files = cardView.codexCardBody(pending({ summary: "Editando a.ts, b.ts", detail: "Motivo: arreglar\n/p/a.ts\n/p/b.ts" }));
  assert.equal(files.summary, "Editando a.ts, b.ts");
  const bare = cardView.codexCardBody(pending({ summary: "chrome: browser_click", detail: "" }));
  assert.deepEqual([bare.summary, bare.detail], [null, "chrome: browser_click"]);
});

test("Y and N answer a Codex card exactly as they answer the others, and a Codex question card is answered by choosing", () => {
  const ctx = (value) => ({ pending: value, cardShown: true, busy: false, focused: true });
  assert.equal(choices.approvalKeyChoice({ key: "y" }, ctx(pending())), "allow");
  assert.equal(choices.approvalKeyChoice({ key: "N" }, ctx(pending())), "deny");
  assert.equal(choices.approvalKeyChoice({ key: "y" }, ctx(pending({ questions: [{ question: "¿Cuál?", header: "", multiSelect: false, options: [{ label: "A", description: "" }] }] }))), null);
  assert.equal(choices.approvalKeyChoice({ key: "y", target: { tagName: "TEXTAREA" } }, ctx(pending())), null);
});

test("the choices a Codex card offers: Siempre only when the engine offered it, never the terminal", () => {
  assert.deepEqual(cardView.availableChoices(pending({ alwaysOffered: true })), ["deny", "always", "allow"]);
  assert.deepEqual(cardView.availableChoices(pending({ alwaysOffered: false })), ["deny", "allow"]);
  assert.deepEqual(cardView.availableChoices(pending({ kind: "claude-code", alwaysOffered: true })), ["deny", "always", "allow", "terminal"]);
  assert.deepEqual(cardView.availableChoices(pending({ kind: "dot-tool", alwaysOffered: false })), ["deny", "allow"]);
  assert.equal(cardView.choiceLabel("always", "codex"), "Siempre en esta sesión");
  assert.equal(cardView.choiceLabel("always", "claude-code"), "Siempre");
});
