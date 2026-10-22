import assert from "node:assert/strict";
import { test } from "node:test";

const tasksModule = await import("../desktop/claude-tasks.mts");
const claudeModule = await import("../desktop/tool-claude.mts");

const REPO = "C:\\work\\repo";
const OTHER = "C:\\work\\other";
const EXE = "C:\\Users\\angel\\.local\\bin\\claude.exe";
const MINUTE = 60_000;

// Fakes ---------------------------------------------------------------------------------------------------------

function controlled(pid) {
  const listeners = {};
  const state = { signals: [], stdin: [], ended: false };
  const child = {
    pid,
    stdin: { write: (chunk) => { state.stdin.push(String(chunk)); return true; }, end: () => { state.ended = true; }, on: () => {} },
    stdout: { on: (event, listener) => { listeners.stdout = listener; } },
    stderr: { on: (event, listener) => { listeners.stderr = listener; } },
    on: (event, listener) => { listeners[event] = listener; },
    kill: (signal) => { state.signals.push(signal ?? "default"); },
  };
  return {
    child,
    state,
    out: (text) => listeners.stdout?.(Buffer.from(text)),
    close: (code) => listeners.close?.(code),
  };
}

function manualTimers() {
  const pending = new Map();
  let next = 1;
  return {
    setTimeout: (fn, ms) => { const id = next++; pending.set(id, { fn, ms }); return id; },
    clearTimeout: (id) => { pending.delete(id); },
    fire: (ms) => { for (const [id, timer] of [...pending]) if (timer.ms === ms) { pending.delete(id); timer.fn(); } },
    pending,
  };
}

const tick = async () => { for (let i = 0; i < 6; i += 1) await new Promise((resolve) => setImmediate(resolve)); };
const line = (value) => `${JSON.stringify(value)}\n`;
const tool = (name, input) => ({ type: "assistant", message: { content: [{ type: "tool_use", name, input }] } });
const RESULT = { type: "result", subtype: "success", is_error: false, result: "All done.", session_id: "sess-9", total_cost_usd: 0.42, permission_denials: [{}, {}] };

function setup({ which = EXE, options = {} } = {}) {
  const claude = [];
  const kills = [];
  const calls = [];
  const timers = manualTimers();
  const clock = { now: 1_000_000 };
  let pid = 100;
  let counter = 0;
  const registry = tasksModule.createClaudeTaskRegistry({
    platform: "win32",
    env: { SystemRoot: "C:\\Windows" },
    stat: async (path) => {
      if (path === REPO || path === OTHER) return { isDirectory: () => true };
      throw Object.assign(new Error("ENOENT"), { code: "ENOENT" });
    },
    spawn: (executable, args, spawnOptions) => {
      pid += 1;
      const proc = controlled(pid);
      calls.push({ executable, args, options: spawnOptions });
      if (/taskkill/i.test(executable)) kills.push(args[1]);
      else claude.push(proc);
      return proc.child;
    },
    whichClaude: async () => which,
    timers,
    now: () => clock.now,
    newId: () => `task_${String(++counter).padStart(12, "0")}`,
    ...options,
  });
  return { registry, claude, kills, calls, timers, clock };
}

async function start(registry, overrides = {}) {
  const started = await registry.start({ task: "Fix the failing test", projectDir: REPO, ...overrides });
  await tick();
  return started;
}

// Starting ------------------------------------------------------------------------------------------------------

test("start returns a task id at once while Claude Code runs in the background with the exact arguments", async () => {
  const { registry, claude, calls } = setup();
  const started = await start(registry);
  assert.equal(started.ok, true);
  assert.match(started.taskId, tasksModule.TASK_ID_PATTERN);
  assert.equal(registry.status(started.taskId).state, "running");
  assert.equal(calls[0].executable, EXE);
  assert.deepEqual(calls[0].args, ["-p", "--output-format", "stream-json", "--verbose", "--permission-mode", "acceptEdits"]);
  assert.ok(!calls[0].args.includes("--bare"));
  assert.equal(calls[0].options.cwd, REPO);
  assert.equal(calls[0].options.shell, false);
  assert.deepEqual(claude[0].state.stdin, ["Fix the failing test"]);
  assert.equal(claude[0].state.ended, true);
});

test("a bad task or project folder is refused before anything starts", async () => {
  const { registry, calls } = setup();
  for (const [input, pattern] of [
    [{ task: "", projectDir: REPO }, /empty/i],
    [{ task: "   ", projectDir: REPO }, /empty/i],
    [{ task: 5, projectDir: REPO }, /empty/i],
    [{ task: "x".repeat(tasksModule.MAX_TASK_CHARS + 1), projectDir: REPO }, /too long/i],
    [{ task: "go", projectDir: "relative\\dir" }, /absolute/i],
    [{ task: "go", projectDir: "C:\\missing" }, /does not exist/i],
    [{ task: "go", projectDir: undefined }, /absolute/i],
  ]) {
    const started = await registry.start(input);
    assert.equal(started.ok, false);
    assert.match(started.error, pattern);
  }
  assert.equal(calls.length, 0);
  assert.deepEqual(registry.list(), []);
});

test("a missing claude becomes a failed task with the clear error", async () => {
  const { registry, calls } = setup({ which: null });
  const started = await start(registry);
  assert.equal(started.ok, true);
  const status = registry.status(started.taskId);
  assert.equal(status.state, "failed");
  assert.equal(status.error, claudeModule.CLAUDE_NOT_INSTALLED);
  assert.equal(calls.length, 0);
});

test("at most three tasks run at once, and a finished one frees its slot", async () => {
  const { registry, claude } = setup();
  const ids = [];
  for (let i = 0; i < 3; i += 1) ids.push((await start(registry)).taskId);
  const refused = await registry.start({ task: "one more", projectDir: REPO });
  assert.equal(refused.ok, false);
  assert.match(refused.error, /too many/i);
  assert.equal(claude.length, 3);
  claude[0].out(line(RESULT));
  claude[0].close(0);
  await tick();
  assert.equal(registry.status(ids[0]).state, "done");
  const accepted = await start(registry);
  assert.equal(accepted.ok, true);
});

test("two simultaneous starts cannot both take the last slot", async () => {
  const { registry } = setup({ options: { maxConcurrent: 1 } });
  const [a, b] = await Promise.all([
    registry.start({ task: "a", projectDir: REPO }),
    registry.start({ task: "b", projectDir: REPO }),
  ]);
  assert.deepEqual([a.ok, b.ok].sort(), [false, true]);
});

// Status and result ---------------------------------------------------------------------------------------------

test("status shows the state, the elapsed time on the fake clock and the latest ten progress lines", async () => {
  const { registry, claude, clock } = setup();
  const { taskId } = await start(registry);
  assert.deepEqual(registry.status(taskId).progress, []);
  for (let i = 1; i <= 12; i += 1) claude[0].out(line(tool("Edit", { file_path: `C:\\work\\repo\\f${i}.ts` })));
  clock.now += 95_000;
  const status = registry.status(taskId);
  assert.equal(status.state, "running");
  assert.equal(status.projectDir, REPO);
  assert.equal(status.elapsedMs, 95_000);
  assert.equal(status.progress.length, 10);
  assert.equal(status.progress[0], "Claude Code: Edit f3.ts");
  assert.equal(status.progress.at(-1), "Claude Code: Edit f12.ts");
  clock.now += 5_000;
  claude[0].out(line(RESULT));
  claude[0].close(0);
  await tick();
  clock.now += 60_000;
  assert.equal(registry.status(taskId).state, "done");
  assert.equal(registry.status(taskId).elapsedMs, 100_000, "elapsed stops when the task ends");
});

test("the result is refused while running, then carries the answer, session, cost and denials", async () => {
  const { registry, claude } = setup();
  const { taskId } = await start(registry);
  assert.equal(registry.result(taskId), "running");
  claude[0].out(line(RESULT));
  assert.equal(registry.result(taskId), "running", "the process has not exited yet");
  claude[0].close(0);
  await tick();
  assert.deepEqual(registry.result(taskId), {
    taskId,
    state: "done",
    ok: true,
    result: "All done.",
    truncated: false,
    sessionId: "sess-9",
    costUsd: 0.42,
    permissionDenials: 2,
  });
});

test("a long result is capped at 16 KiB and flagged", async () => {
  const { registry, claude } = setup();
  const { taskId } = await start(registry);
  claude[0].out(line({ ...RESULT, result: "a".repeat(40_000) }));
  claude[0].close(0);
  await tick();
  const result = registry.result(taskId);
  assert.equal(result.result.length, 16 * 1024);
  assert.equal(result.truncated, true);
});

test("a run that ends without a result, or with an error result, is failed with its reason", async () => {
  const { registry, claude } = setup();
  const first = (await start(registry)).taskId;
  claude[0].close(3);
  await tick();
  assert.equal(registry.status(first).state, "failed");
  assert.match(registry.status(first).error, /without a result/i);
  const second = (await start(registry)).taskId;
  claude[1].out(line({ ...RESULT, is_error: true, result: "boom" }));
  claude[1].close(1);
  await tick();
  assert.equal(registry.status(second).state, "failed");
  const result = registry.result(second);
  assert.equal(result.ok, false);
  assert.equal(result.result, "boom");
});

test("unknown ids are reported as unknown", async () => {
  const { registry } = setup();
  assert.equal(registry.status("task_000000000000"), undefined);
  assert.equal(registry.result("task_000000000000"), undefined);
  assert.equal(registry.cancel("task_000000000000"), undefined);
});

// Cancel and timeout --------------------------------------------------------------------------------------------

test("cancel sends SIGINT first and keeps the task running until the process stops", async () => {
  const { registry, claude, kills } = setup({ options: { gracefulInterrupt: true } });
  const { taskId } = await start(registry);
  assert.deepEqual(registry.cancel(taskId), { stopping: true, state: "running" });
  assert.deepEqual(claude[0].state.signals, ["SIGINT"]);
  assert.deepEqual(kills, []);
  assert.equal(registry.status(taskId).state, "running");
  claude[0].close(130);
  await tick();
  const status = registry.status(taskId);
  assert.equal(status.state, "cancelled");
  assert.match(status.error, /cancelled/i);
  assert.deepEqual(kills, [], "a process that stopped in time is not tree-killed");
  assert.equal(registry.cancel(taskId).stopping, false);
});

test("cancel kills the whole tree when SIGINT is not enough within the grace period", async () => {
  const { registry, claude, kills, timers } = setup({ options: { gracefulInterrupt: true, interruptGraceMs: 4_000 } });
  const { taskId } = await start(registry);
  registry.cancel(taskId);
  assert.deepEqual(kills, []);
  timers.fire(4_000);
  await tick();
  assert.deepEqual(kills, ["101"]);
  assert.deepEqual(claude[0].state.signals, ["SIGINT", "default"]);
  assert.equal(registry.status(taskId).state, "cancelled");
});

test("on Windows by default cancel kills the tree at once, because SIGINT only ends the direct child there", async () => {
  const { registry, claude, kills } = setup();
  const { taskId } = await start(registry);
  registry.cancel(taskId);
  await tick();
  assert.deepEqual(kills, ["101"]);
  assert.deepEqual(claude[0].state.signals, ["default"]);
  assert.equal(registry.status(taskId).state, "cancelled");
});

test("cancelling twice interrupts once, and a finished task is not cancelled", async () => {
  const { registry, claude } = setup({ options: { gracefulInterrupt: true } });
  const { taskId } = await start(registry);
  registry.cancel(taskId);
  registry.cancel(taskId);
  assert.deepEqual(claude[0].state.signals, ["SIGINT"]);
  claude[0].close(130);
  await tick();
  assert.deepEqual(registry.cancel(taskId), { stopping: false, state: "cancelled" });
});

test("once SIGINT was sent the task is cancelled even if a result line still arrives", async () => {
  const { registry, claude } = setup({ options: { gracefulInterrupt: true } });
  const { taskId } = await start(registry);
  registry.cancel(taskId);
  claude[0].out(line(RESULT));
  claude[0].close(0);
  await tick();
  assert.equal(registry.status(taskId).state, "cancelled");
});

test("the 30 minute timeout kills the tree and ends the task as timed_out", async () => {
  const { registry, kills, timers } = setup();
  const { taskId } = await start(registry);
  assert.ok([...timers.pending.values()].some((timer) => timer.ms === 30 * MINUTE));
  timers.fire(30 * MINUTE);
  await tick();
  const status = registry.status(taskId);
  assert.equal(status.state, "timed_out");
  assert.match(status.error, /timed out/i);
  assert.deepEqual(kills, ["101"]);
});

test("dispose stops every running task at once", async () => {
  const { registry, kills } = setup();
  const a = (await start(registry)).taskId;
  const b = (await start(registry)).taskId;
  registry.dispose();
  await tick();
  assert.deepEqual(kills, ["101", "102"]);
  assert.equal(registry.status(a).state, "failed");
  assert.equal(registry.status(b).state, "failed");
});

// Bookkeeping ---------------------------------------------------------------------------------------------------

test("stored tasks are capped: the oldest finished one goes first, running ones stay", async () => {
  const { registry, claude } = setup({ options: { maxStored: 2, maxConcurrent: 5 } });
  const first = (await start(registry)).taskId;
  const second = (await start(registry)).taskId;
  claude[1].out(line(RESULT));
  claude[1].close(0);
  await tick();
  const third = (await start(registry)).taskId;
  assert.equal(registry.status(second), undefined, "the finished task was dropped");
  assert.equal(registry.status(first).state, "running");
  assert.equal(registry.status(third).state, "running");
  const ids = registry.list().map((entry) => entry.id).sort();
  assert.deepEqual(ids, [first, third].sort());
});

test("list gives id, project, state, latest line and start time, newest first", async () => {
  const { registry, claude, clock } = setup();
  const first = (await start(registry)).taskId;
  clock.now += 1000;
  const second = (await start(registry, { projectDir: OTHER })).taskId;
  claude[1].out(line(tool("Bash", { command: "npm test" })));
  assert.deepEqual(registry.list(), [
    { id: second, project: OTHER, state: "running", latestLine: "Claude Code: Bash npm test", startedAt: 1_001_000 },
    { id: first, project: REPO, state: "running", latestLine: null, startedAt: 1_000_000 },
  ]);
});

test("installClaudeTasks publishes a read-only list under Symbol.for(\"click-dot.claude-tasks\")", async () => {
  const { registry } = setup();
  const target = {};
  tasksModule.installClaudeTasks(target, registry);
  assert.equal(tasksModule.CLAUDE_TASKS_KEY, "click-dot.claude-tasks");
  const published = target[Symbol.for("click-dot.claude-tasks")];
  assert.deepEqual(Object.keys(published), ["list"]);
  assert.equal(Object.isFrozen(published), true);
  assert.deepEqual(published.list(), []);
  const { taskId } = await start(registry);
  assert.equal(published.list()[0].id, taskId);
  assert.equal(published.start, undefined);
  assert.equal(published.cancel, undefined);
});

test("task ids are random by default and match the pattern", async () => {
  const { registry } = setup({ options: { newId: undefined } });
  const a = (await start(registry)).taskId;
  const b = (await start(registry)).taskId;
  assert.notEqual(a, b);
  assert.match(a, tasksModule.TASK_ID_PATTERN);
});
