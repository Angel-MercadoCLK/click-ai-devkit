import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { test } from "node:test";

const history = await import("../lib/chat-history-view.ts");

const read = (path) => readFile(new URL(`../${path}`, import.meta.url), "utf8");
const NOW = new Date(2026, 9, 22, 15, 30).getTime(); // 22 Oct 2026, local time
const ago = (ms) => NOW - ms;
const MIN = 60_000;
const HOUR = 60 * MIN;
const DAY = 24 * HOUR;
const conversation = (overrides = {}) => ({
  id: "11111111-1111-4111-8111-111111111111",
  title: "stored title",
  updatedAt: NOW,
  messages: [
    { role: "user", content: "  ¿Cómo   hago un\nbackup?  ", createdAt: 1 },
    { role: "assistant", content: "Así.", createdAt: 1 },
  ],
  ...overrides,
});

// --- titles ----------------------------------------------------------------------------------------------------

test("the title is the first user message, trimmed and on one line", () => {
  assert.equal(history.historyTitle(conversation()), "¿Cómo hago un backup?");
});

test("a long first message is cut with an ellipsis instead of overflowing the list", () => {
  const long = "a".repeat(200);
  const title = history.historyTitle(conversation({ messages: [{ role: "user", content: long, createdAt: 1 }, { role: "assistant", content: "x", createdAt: 1 }] }));
  assert.ok(title.length <= history.TITLE_MAX);
  assert.ok(title.endsWith("…"));
  const short = history.historyTitle(conversation({ messages: [{ role: "user", content: "x".repeat(history.TITLE_MAX), createdAt: 1 }, { role: "assistant", content: "x", createdAt: 1 }] }));
  assert.equal(short.length, history.TITLE_MAX, "a message that fits is left alone");
  assert.ok(!short.endsWith("…"));
});

test("without a message the stored title is used, and without either the conversation is named in Spanish", () => {
  assert.equal(history.historyTitle(conversation({ messages: [] })), "stored title");
  assert.equal(history.historyTitle(conversation({ messages: [], title: "   " })), "Chat sin título");
  assert.equal(history.historyTitle(conversation({ messages: [{ role: "assistant", content: "hola", createdAt: 1 }], title: "" })), "Chat sin título");
});

// --- relative time ---------------------------------------------------------------------------------------------

test("recent conversations say how long ago in Spanish", () => {
  assert.equal(history.relativeTimeEs(ago(5_000), NOW), "ahora");
  assert.equal(history.relativeTimeEs(ago(44_000), NOW), "ahora");
  assert.equal(history.relativeTimeEs(ago(2 * MIN), NOW), "hace 2 min");
  assert.equal(history.relativeTimeEs(ago(59 * MIN), NOW), "hace 59 min");
  assert.equal(history.relativeTimeEs(ago(HOUR), NOW), "hace 1 h");
  assert.equal(history.relativeTimeEs(ago(3 * HOUR), NOW), "hace 3 h");
});

test("a conversation from the future (a clock that moved) is just now", () => {
  assert.equal(history.relativeTimeEs(NOW + 5 * MIN, NOW), "ahora");
});

test("yesterday, a few days ago and an older date read naturally", () => {
  const yesterdayEvening = new Date(2026, 9, 21, 23, 0).getTime();
  assert.equal(history.relativeTimeEs(yesterdayEvening, NOW), "ayer");
  assert.equal(history.relativeTimeEs(new Date(2026, 9, 21, 9, 0).getTime(), NOW), "ayer");
  assert.equal(history.relativeTimeEs(new Date(2026, 9, 19, 9, 0).getTime(), NOW), "hace 3 días");
  assert.equal(history.relativeTimeEs(new Date(2026, 9, 12, 9, 0).getTime(), NOW), "12 oct");
  assert.equal(history.relativeTimeEs(new Date(2026, 0, 3, 9, 0).getTime(), NOW), "3 ene");
  assert.equal(history.relativeTimeEs(new Date(2025, 11, 31, 9, 0).getTime(), NOW), "31 dic 2025");
});

test("a time that is not a number does not break the list", () => {
  assert.equal(history.relativeTimeEs(Number.NaN, NOW), "");
});

// --- the list --------------------------------------------------------------------------------------------------

test("the list is newest first, each entry with its title, time and the id used to open or delete it", () => {
  const a = conversation({ id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", updatedAt: ago(3 * DAY) });
  const b = conversation({ id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", updatedAt: ago(2 * MIN) });
  const c = conversation({ id: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", updatedAt: ago(5 * HOUR) });
  const entries = history.historyEntries([a, b, c], NOW);
  assert.deepEqual(entries.map((entry) => entry.id), [b.id, c.id, a.id]);
  assert.deepEqual(entries[0], { id: b.id, title: "¿Cómo hago un backup?", when: "hace 2 min" });
  assert.deepEqual(history.historyEntries([], NOW), []);
});

test("conversations changed at the same moment keep the order they came in", () => {
  const first = conversation({ id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", updatedAt: ago(MIN) });
  const second = conversation({ id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", updatedAt: ago(MIN) });
  assert.deepEqual(history.historyEntries([first, second], NOW).map((entry) => entry.id), [first.id, second.id]);
});

test("listing never changes what was passed in", () => {
  const list = [conversation({ updatedAt: ago(5 * HOUR) }), conversation({ id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", updatedAt: ago(MIN) })];
  const copy = structuredClone(list);
  history.historyEntries(list, NOW);
  assert.deepEqual(list, copy);
});

// --- the card --------------------------------------------------------------------------------------------------

test("the chat card has a history view that opens, continues, starts a new chat and deletes with a confirmation, on the existing routes", async () => {
  const compact = await read("app/chat/compact-chat.tsx");
  assert.match(compact, /historyEntries\(/);
  assert.match(compact, /Historial/);
  assert.match(compact, /Nuevo chat/);
  assert.match(compact, /openConversation/);
  assert.match(compact, /removeConversation/);
  assert.match(compact, /¿Borrar/);
  assert.match(compact, /role="alertdialog"|role="group"[^>]*aria-label="Confirmar/);
  // Delete is never one click: removeConversation is only reached from the confirmation.
  const calls = compact.match(/removeConversation\(/g) ?? [];
  assert.equal(calls.length, 1);
  assert.match(compact, /confirm[\s\S]{0,400}removeConversation\(/);
  const panel = await read("app/chat-panel.tsx");
  assert.match(panel, /\/api\/chat\/history\/\$\{encodeURIComponent\(id\)\}/, "the existing delete route is the one used");
});

test("the persisted conversation format is untouched: the store only gained nothing and the history routes are the same ones", async () => {
  const store = await read("lib/conversation-store.ts");
  assert.match(store, /type HistoryDocument = \{ version: 1; accounts: Record<string, AccountHistory> \}/);
  assert.match(store, /type Conversation = \{ id: string; title: string; updatedAt: number; messages: ChatMessage\[\] \}/);
  await read("app/api/chat/history/route.ts");
  await read("app/api/chat/history/[id]/route.ts");
});
