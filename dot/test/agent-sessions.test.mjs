import assert from "node:assert/strict";
import { test } from "node:test";

const sessions = await import("../lib/agent-sessions.ts");

const clock = () => {
  let now = 1_000;
  const read = () => now;
  read.advance = (ms) => {
    now += ms;
  };
  return read;
};

const newStore = (options = {}) => {
  const now = clock();
  return { now, store: sessions.createSessionStore({ now, ...options }) };
};

// --- project name ---------------------------------------------------------------------------------------------

test("the project name is the last folder of the working directory, on Windows and POSIX paths", () => {
  assert.equal(sessions.projectName("C:\\Users\\angel\\projects\\click-ai-devkit"), "click-ai-devkit");
  assert.equal(sessions.projectName("/home/angel/projects/dot/"), "dot");
  assert.equal(sessions.projectName("D:\\"), "D:");
  assert.equal(sessions.projectName(undefined), "Sesión");
  assert.equal(sessions.projectName(""), "Sesión");
  assert.equal(sessions.projectName(42), "Sesión");
});

// --- line counts and action text ------------------------------------------------------------------------------

test("lines are counted without counting a trailing newline as an extra line", () => {
  assert.equal(sessions.countLines(""), 0);
  assert.equal(sessions.countLines("a"), 1);
  assert.equal(sessions.countLines("a\n"), 1);
  assert.equal(sessions.countLines("a\nb"), 2);
  assert.equal(sessions.countLines("a\r\nb\r\n"), 2);
  assert.equal(sessions.countLines(undefined), 0);
});

test("an Edit shows the file name and the new and old line counts", () => {
  const input = { file_path: "C:\\work\\src\\foo.ts", old_string: "a\nb\nc", new_string: Array.from({ length: 12 }, (_, i) => `l${i}`).join("\n") };
  assert.equal(sessions.describeToolAction("Edit", input), "Editando foo.ts +12 −3");
});

test("a MultiEdit adds up every edit and a Write counts the lines of its content", () => {
  const multi = { file_path: "/p/a/foo.ts", edits: [{ old_string: "a", new_string: "a\nb" }, { old_string: "x\ny", new_string: "z" }] };
  assert.equal(sessions.describeToolAction("MultiEdit", multi), "Editando foo.ts +3 −3");
  assert.equal(sessions.describeToolAction("Write", { file_path: "/p/new.md", content: "one\ntwo\nthree\n" }), "Escribiendo new.md +3");
});

test("Bash, Read, search tools and unknown tools get a short Spanish line", () => {
  assert.equal(sessions.describeToolAction("Bash", { command: "npm test\nrm -rf nothing" }), "Ejecutando npm test");
  assert.equal(sessions.describeToolAction("Bash", { command: "x".repeat(300) }).length <= "Ejecutando ".length + 80, true);
  assert.match(sessions.describeToolAction("Bash", { command: "x".repeat(300) }), /…$/);
  assert.equal(sessions.describeToolAction("Read", { file_path: "/a/b/notes.txt" }), "Leyendo notes.txt");
  assert.equal(sessions.describeToolAction("Grep", { pattern: "TODO" }), "Buscando TODO");
  assert.equal(sessions.describeToolAction("Glob", { pattern: "**/*.ts" }), "Buscando **/*.ts");
  assert.equal(sessions.describeToolAction("WebSearch", { query: "node test runner" }), "Buscando node test runner");
  assert.equal(sessions.describeToolAction("TodoWrite", { todos: [] }), "Usando TodoWrite");
  assert.equal(sessions.describeToolAction("Edit", "not an object"), "Editando archivo +0 −0");
  assert.equal(sessions.describeToolAction(undefined, undefined), "Usando una herramienta");
});

// --- state machine --------------------------------------------------------------------------------------------

test("Stop makes a session listo, a prompt or a tool call makes it trabajando, and SessionEnd removes it", () => {
  const { store } = newStore();
  store.apply("UserPromptSubmit", { session_id: "s1", cwd: "C:\\p\\alpha" });
  assert.deepEqual(store.list().map(({ id, project, state }) => ({ id, project, state })), [{ id: "s1", project: "alpha", state: "trabajando" }]);
  store.apply("PreToolUse", { session_id: "s1", cwd: "C:\\p\\alpha", tool_name: "Bash", tool_input: { command: "npm test" } });
  assert.equal(store.list()[0].action, "Ejecutando npm test");
  store.apply("Stop", { session_id: "s1", cwd: "C:\\p\\alpha" });
  assert.equal(store.list()[0].state, "listo");
  assert.equal(store.list()[0].action, "Ejecutando npm test", "the last action stays visible");
  store.apply("PostToolUse", { session_id: "s1", cwd: "C:\\p\\alpha", tool_name: "Read", tool_input: { file_path: "a.ts" } });
  assert.equal(store.list()[0].state, "trabajando");
  store.apply("SessionEnd", { session_id: "s1" });
  assert.deepEqual(store.list(), []);
});

test("a started session that has done nothing yet is listo", () => {
  const { store } = newStore();
  store.apply("SessionStart", { session_id: "s1", cwd: "/p/beta" });
  assert.equal(store.list()[0].state, "listo");
});

test("a pending request makes a session te necesita and other events do not hide it", () => {
  const { store } = newStore();
  store.apply("UserPromptSubmit", { session_id: "s1", cwd: "/p/alpha" });
  store.needsYou("s1", { cwd: "/p/alpha", action: "Pide permiso para Bash" });
  assert.equal(store.list()[0].state, "te necesita");
  store.apply("PreToolUse", { session_id: "s1", cwd: "/p/alpha", tool_name: "Read", tool_input: { file_path: "x" } });
  assert.equal(store.list()[0].state, "te necesita");
  store.settle("s1", "answered");
  assert.equal(store.list()[0].state, "trabajando");
});

test("when the user answers in the terminal the session still needs them until its next event", () => {
  const { store } = newStore();
  store.needsYou("s1", { cwd: "/p/alpha", action: "Pide permiso para Bash" });
  store.settle("s1", "terminal");
  assert.equal(store.list()[0].state, "te necesita");
  store.apply("PostToolUse", { session_id: "s1", cwd: "/p/alpha", tool_name: "Bash", tool_input: { command: "ls" } });
  assert.equal(store.list()[0].state, "trabajando");
});

test("sessions list the ones that need the user first, then working, then done, newest first", () => {
  const { store, now } = newStore();
  store.apply("Stop", { session_id: "done", cwd: "/p/done" });
  now.advance(10);
  store.apply("UserPromptSubmit", { session_id: "busy-old", cwd: "/p/busy-old" });
  now.advance(10);
  store.apply("UserPromptSubmit", { session_id: "busy-new", cwd: "/p/busy-new" });
  now.advance(10);
  store.needsYou("ask", { cwd: "/p/ask", action: "Pide permiso" });
  assert.deepEqual(store.list().map((session) => session.id), ["ask", "busy-new", "busy-old", "done"]);
});

test("the store keeps a bounded number of sessions and a bounded history, and drops the least recent session first", () => {
  const { store, now } = newStore({ maxSessions: 3, maxHistory: 2 });
  for (const id of ["a", "b", "c", "d"]) {
    store.apply("UserPromptSubmit", { session_id: id, cwd: `/p/${id}` });
    now.advance(5);
  }
  assert.deepEqual(store.list().map((session) => session.id).sort(), ["b", "c", "d"]);
  for (const name of ["one", "two", "three"]) store.apply("PreToolUse", { session_id: "d", cwd: "/p/d", tool_name: "Grep", tool_input: { pattern: name } });
  assert.deepEqual(store.history("d"), ["Buscando two", "Buscando three"]);
});

test("a session waiting for the user is not evicted while another one can go", () => {
  const { store, now } = newStore({ maxSessions: 2 });
  store.needsYou("ask", { cwd: "/p/ask", action: "Pide permiso" });
  now.advance(5);
  store.apply("UserPromptSubmit", { session_id: "b", cwd: "/p/b" });
  now.advance(5);
  store.apply("UserPromptSubmit", { session_id: "c", cwd: "/p/c" });
  assert.deepEqual(store.list().map((session) => session.id).sort(), ["ask", "c"]);
});

test("events without a usable session id and malformed payloads change nothing", () => {
  const { store } = newStore();
  store.apply("PreToolUse", {});
  store.apply("PreToolUse", { session_id: 12 });
  store.apply("PreToolUse", { session_id: "x".repeat(500) });
  store.apply("PreToolUse", null);
  assert.deepEqual(store.list(), []);
});

test("the store never touches the disk or the network", async () => {
  const { readFile } = await import("node:fs/promises");
  const source = await readFile(new URL("../lib/agent-sessions.ts", import.meta.url), "utf8");
  assert.doesNotMatch(source, /node:fs|node:net|node:http|fetch\(|localStorage/);
});
