import assert from "node:assert/strict";
import { test } from "node:test";

const state = await import("../lib/capture-state.ts");

const ok = (active) => ({ ok: true, active });
const failed = { ok: false };

test("a successful status read decides activity at once, in both directions", () => {
  assert.deepEqual(state.applyCapturePoll(state.INITIAL_CAPTURE_POLL, ok(true)), { active: true, failures: 0 });
  assert.deepEqual(state.applyCapturePoll({ active: true, failures: 2 }, ok(false)), { active: false, failures: 0 });
});

test("one or two failed polls keep the last known activity", () => {
  let current = state.applyCapturePoll(state.INITIAL_CAPTURE_POLL, ok(true));
  current = state.applyCapturePoll(current, failed);
  assert.equal(current.active, true);
  current = state.applyCapturePoll(current, failed);
  assert.equal(current.active, true);
});

test("three consecutive failures mark capture inactive", () => {
  assert.equal(state.CAPTURE_FAILURE_LIMIT, 3);
  let current = { active: true, failures: 0 };
  for (let index = 0; index < 3; index += 1) current = state.applyCapturePoll(current, failed);
  assert.equal(current.active, false);
  current = state.applyCapturePoll(current, failed);
  assert.equal(current.active, false, "stays inactive while failures continue");
});

test("a success resets the failure count so isolated failures never add up", () => {
  let current = { active: true, failures: 0 };
  for (let round = 0; round < 5; round += 1) {
    current = state.applyCapturePoll(current, failed);
    current = state.applyCapturePoll(current, failed);
    current = state.applyCapturePoll(current, ok(true));
    assert.equal(current.active, true);
  }
});

test("a status body is read as active only when available, enabled and not paused", () => {
  assert.deepEqual(state.readCapturePoll({ available: true, enabled: true, paused: false }), ok(true));
  assert.deepEqual(state.readCapturePoll({ available: true, enabled: true, paused: true }), ok(false));
  assert.deepEqual(state.readCapturePoll({ available: true, enabled: false, paused: false }), ok(false));
  assert.deepEqual(state.readCapturePoll({ available: false, enabled: true, paused: false }), ok(false));
  assert.deepEqual(state.readCapturePoll({}), ok(false));
});
