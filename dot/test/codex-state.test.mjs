import assert from "node:assert/strict";
import { test } from "node:test";

const state = await import("../lib/agent-state.ts");
const brokerModule = await import("../lib/agent-broker.ts");

test("the page reads a codex pending card and still refuses an unknown kind", () => {
  const pending = { id: "p1", kind: "codex", tool: "Codex", summary: "npm test", detail: "Carpeta: /p", project: "Codex", alwaysOffered: true, questions: null };
  const read = state.readAgentState({ sessions: [], pending, queued: 0 });
  assert.deepEqual(read?.pending, pending);
  assert.deepEqual([...state.PENDING_KINDS], ["claude-code", "dot-tool", "codex"]);
  assert.equal(state.readAgentState({ sessions: [], pending: { ...pending, kind: "other" }, queued: 0 }), undefined);
  assert.equal(state.agentAttention(read), "needs");
});

test("the shared broker is rebuilt when an older one without Codex support is on globalThis", () => {
  const key = brokerModule.AGENT_BROKER_KEY;
  const saved = globalThis[key];
  try {
    globalThis[key] = { record() {}, enqueue() {}, requestToolApproval() {}, state() {}, ack() {}, decide() {}, releaseAll() {} };
    const broker = brokerModule.getAgentBroker();
    assert.equal(typeof broker.requestCodexApproval, "function");
  } finally {
    if (saved === undefined) delete globalThis[key]; else globalThis[key] = saved;
  }
});
