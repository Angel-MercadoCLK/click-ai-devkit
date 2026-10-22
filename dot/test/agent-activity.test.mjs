import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { test } from "node:test";

process.env.PORT = "3011";

const events = await import("../lib/tool-events.ts");
const agentState = await import("../lib/agent-state.ts");

const read = (path) => readFile(new URL(`../${path}`, import.meta.url), "utf8");
const event = (overrides = {}) => ({ id: "call_1", name: "powershell", summary: "git status", status: "running", ...overrides });

// --- the tool events the page reads ------------------------------------------------------------------------------

test("a well-formed tool event is read as it is, with or without a detail", () => {
  assert.deepEqual(events.readToolEvent(event()), event());
  assert.deepEqual(events.readToolEvent(event({ status: "done", detail: "out" })), event({ status: "done", detail: "out" }));
  for (const name of ["powershell", "claude_code", "screen"]) assert.equal(events.readToolEvent(event({ name }))?.name, name);
  for (const status of ["running", "waiting_approval", "done", "denied", "error"]) assert.equal(events.readToolEvent(event({ status }))?.status, status);
});

test("anything malformed is not a tool event, and a detail over 2,000 characters is refused", () => {
  const bad = [null, "x", [], {}, event({ id: "" }), event({ id: 5 }), event({ name: "shell" }), event({ status: "paused" }), event({ summary: 5 }), event({ detail: 5 }), event({ detail: "x".repeat(2_001) })];
  for (const value of bad) assert.equal(events.readToolEvent(value), undefined, JSON.stringify(value));
  assert.ok(events.readToolEvent(event({ detail: "x".repeat(2_000) })));
});

test("repeated events with the same id update that call, keep its place and keep an earlier detail", () => {
  let list = events.applyToolEvent([], event());
  list = events.applyToolEvent(list, event({ id: "call_2", name: "screen", summary: "Mirar la pantalla" }));
  assert.equal(events.isNewToolCall(list, event({ id: "call_1" })), false);
  assert.equal(events.isNewToolCall(list, event({ id: "call_3" })), true);
  list = events.applyToolEvent(list, event({ status: "running", detail: "Claude Code: Edit foo.ts" }));
  list = events.applyToolEvent(list, event({ status: "done" }));
  assert.deepEqual(list.map((item) => [item.id, item.status]), [["call_1", "done"], ["call_2", "running"]]);
  assert.equal(list[0].detail, "Claude Code: Edit foo.ts");
});

test("the chips use the Spanish words and the output tail keeps the end of a long text", () => {
  assert.deepEqual(events.TOOL_STATUS_LABELS, { running: "Ejecutando", waiting_approval: "Esperando tu permiso", done: "Listo", denied: "Denegado", error: "Error" });
  const clipped = events.clipDetail(`${"a".repeat(5_000)}FIN`);
  assert.equal(clipped.length, 2_000);
  assert.ok(clipped.endsWith("FIN"));
  assert.equal(events.clipDetail("short"), "short");
  assert.equal(events.oneLine("a\n  b\tc"), "a b c");
});

// --- the approval card ---------------------------------------------------------------------------------------

test("the state reader accepts a Dot tool request and keeps its kind", () => {
  const pending = { id: "p1", kind: "dot-tool", tool: "PowerShell", summary: "Remove-Item a", detail: "Carpeta: C:\\\\w\nPuede modificar tu equipo", project: "Dot", alwaysOffered: true, questions: null };
  const state = agentState.readAgentState({ sessions: [], pending, queued: 0 });
  assert.deepEqual(state.pending, pending);
});

test("the card shows a Dot request with Permitir, Denegar and Siempre only when offered, and hands nothing to the terminal", async () => {
  // The wording and the choices per kind live in agent-card-view.ts (and are exercised in engine-client.test.mjs).
  const source = await read("app/mascot/agent-card.tsx");
  const view = await read("app/mascot/agent-card-view.ts");
  assert.match(view, /pending\.kind === "dot-tool"/);
  assert.match(source, /view\.showTerminal &&[^\n]*choiceLabel\("terminal"/);
  assert.match(source + view, /Dot quiere usar/);
  assert.match(source, /alwaysOffered\s*&&/);
});

// --- the chats -----------------------------------------------------------------------------------------------

test("both chats show activity lines with chips and an output tail, and the bubble gets a Detener button", async () => {
  const parts = await read("app/chat/parts.tsx");
  assert.match(parts, /ToolActivityList/);
  const activity = await read("app/chat/tool-activity.tsx");
  assert.match(activity, /TOOL_STATUS_LABELS/);
  assert.match(activity, /<details/);
  const compact = await read("app/chat/compact-chat.tsx");
  assert.match(compact, /cancel: "Detener"/);
  const panel = await read("app/chat-panel.tsx");
  assert.match(panel, /readToolEvent/);
  assert.match(panel, /applyToolEvent/);
  assert.doesNotMatch(panel + await read("app/page.tsx"), /No tools are enabled|No external tools|operating-system tools are not enabled/);
  const css = await read("app/globals.css");
  for (const selector of [".tool-activity", ".tool-line", ".tool-chip", ".agent-card-note"]) assert.ok(css.includes(selector), selector);
});

test("the labels the packaged smoke test asserts are still on the mascot page", async () => {
  const mascot = await read("app/mascot/mascot.tsx");
  for (const label of ["Compartir mi pantalla", "Voz", "Mensaje", "Conectar con ChatGPT"]) assert.ok(mascot.includes(label), label);
});
