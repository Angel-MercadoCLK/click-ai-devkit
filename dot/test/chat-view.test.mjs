import assert from "node:assert/strict";
import { test } from "node:test";

const { isChatEmpty } = await import("../lib/chat-view.ts");

const idle = { messages: [], busy: false, submittedPrompt: "", liveText: "", tools: [] };

test("the chat shows its empty state only when nothing is there or on the way", () => {
  assert.equal(isChatEmpty(idle), true);
  assert.equal(isChatEmpty({ ...idle, messages: [{ role: "user", content: "hola" }] }), false);
  assert.equal(isChatEmpty({ ...idle, liveText: "Hola" }), false);
});

test("a first message in flight hides the empty state, also while only tools have run", () => {
  assert.equal(isChatEmpty({ ...idle, busy: true, submittedPrompt: "Corrige el build" }), false);
  assert.equal(isChatEmpty({ ...idle, tools: [{ id: "call_1" }] }), false);
});
