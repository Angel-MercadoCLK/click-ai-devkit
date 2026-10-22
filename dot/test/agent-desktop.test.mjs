import assert from "node:assert/strict";
import { request as httpRequest, Agent } from "node:http";
import { mkdtemp, readdir, readFile, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";

process.env.PORT = "3009";

const agentHook = await import("../desktop/agent-hook.mts");
const serverModule = await import("../desktop/server.mts");
const mascot = await import("../desktop/mascot.mts");
const windowMode = await import("../lib/window-mode.ts");
const brokerModule = await import("../lib/agent-broker.ts");
const hookModule = await import("../lib/agent-hook.ts");

const scratch = await mkdtemp(join(tmpdir(), "click-dot-agent-desktop-test-"));
const posix = process.platform !== "win32";
const TOKEN = "a1".repeat(32);

// --- the token and where the hook endpoint reads it -------------------------------------------------------------

test("the shell makes a fresh random 256-bit hook token per launch", () => {
  const first = agentHook.createAgentHookToken();
  assert.match(first, /^[0-9a-f]{64}$/);
  assert.notEqual(first, agentHook.createAgentHookToken());
});

test("the token is published where the endpoint reads it, and the names agree", () => {
  const target = {};
  agentHook.installAgentHook(target, TOKEN);
  assert.deepEqual(Reflect.get(target, Symbol.for(agentHook.AGENT_HOOK_KEY)), { token: TOKEN });
  assert.equal(Symbol.keyFor(hookModule.AGENT_HOOK_KEY), agentHook.AGENT_HOOK_KEY);
  assert.equal(Symbol.keyFor(brokerModule.AGENT_BROKER_KEY), agentHook.AGENT_BROKER_KEY);
  Reflect.set(globalThis, hookModule.AGENT_HOOK_KEY, { token: TOKEN });
  assert.equal(hookModule.readHookToken(), TOKEN);
  Reflect.deleteProperty(globalThis, hookModule.AGENT_HOOK_KEY);
});

test("a malformed token is never published", () => {
  for (const bad of ["", "short", "A".repeat(64), "g".repeat(64), undefined, 5, null, {}]) {
    assert.throws(() => agentHook.installAgentHook({}, bad), /malformed/, String(bad));
  }
});

// --- the discovery file ----------------------------------------------------------------------------------------

test("the discovery file is in the user data folder unless the override names another place", () => {
  assert.equal(agentHook.AGENT_HOOK_FILE_NAME, "agent-hook.json");
  assert.equal(agentHook.AGENT_HOOK_FILE_ENV, "CLICK_DOT_AGENT_HOOK_FILE");
  assert.equal(agentHook.agentHookFile(join(scratch, "data"), {}), join(scratch, "data", "agent-hook.json"));
  assert.equal(agentHook.agentHookFile(join(scratch, "data"), { CLICK_DOT_AGENT_HOOK_FILE: join(scratch, "other.json") }), join(scratch, "other.json"));
  assert.equal(agentHook.agentHookFile(join(scratch, "data"), { CLICK_DOT_AGENT_HOOK_FILE: "" }), join(scratch, "data", "agent-hook.json"));
});

test("the discovery file holds exactly the version, port, token and pid of the contract", async () => {
  const file = join(scratch, "nested", "agent-hook.json");
  await agentHook.writeAgentHookFile(file, { port: 47321, token: TOKEN, pid: 4242 });
  const text = await readFile(file, "utf8");
  assert.deepEqual(JSON.parse(text), { version: 1, port: 47321, token: TOKEN, pid: 4242 });
  assert.deepEqual(Object.keys(JSON.parse(text)), ["version", "port", "token", "pid"]);
});

test("the discovery file is readable only by its owner", { skip: !posix }, async () => {
  const file = join(scratch, "private", "agent-hook.json");
  await agentHook.writeAgentHookFile(file, { port: 1, token: TOKEN, pid: 1 });
  assert.equal((await stat(file)).mode & 0o777, 0o600);
});

test("writing replaces a stale file atomically and leaves no temporary file behind", async () => {
  const dir = await mkdtemp(join(scratch, "atomic-"));
  const file = join(dir, "agent-hook.json");
  await writeFile(file, "stale from a crashed launch");
  await agentHook.writeAgentHookFile(file, { port: 47321, token: TOKEN, pid: 7 });
  assert.equal(JSON.parse(await readFile(file, "utf8")).pid, 7);
  assert.deepEqual(await readdir(dir), ["agent-hook.json"]);
  const source = await readFile(new URL("../desktop/agent-hook.mts", import.meta.url), "utf8");
  assert.match(source, /rename\(/, "the file appears under its final name in one step");
});

test("quitting deletes the discovery file only while it still holds this launch's token", async () => {
  const file = join(await mkdtemp(join(scratch, "remove-")), "agent-hook.json");
  await agentHook.writeAgentHookFile(file, { port: 1, token: TOKEN, pid: 1 });
  assert.equal(agentHook.removeAgentHookFile(file, "b2".repeat(32)), false, "another launch's file is left alone");
  assert.equal(JSON.parse(await readFile(file, "utf8")).token, TOKEN);
  assert.equal(agentHook.removeAgentHookFile(file, TOKEN), true);
  await assert.rejects(readFile(file), { code: "ENOENT" });
  assert.equal(agentHook.removeAgentHookFile(file, TOKEN), false, "a missing file is not an error");
  await writeFile(file, "not json");
  assert.equal(agentHook.removeAgentHookFile(file, TOKEN), false, "a file that is not ours is left alone");
});

// --- quitting releases held requests ----------------------------------------------------------------------------

test("quitting asks the shared broker to release every held request, and does nothing without one", () => {
  const target = {};
  assert.doesNotThrow(() => agentHook.releaseAgentRequests(target));
  let released = 0;
  Reflect.set(target, Symbol.for(agentHook.AGENT_BROKER_KEY), { releaseAll: () => void (released += 1) });
  agentHook.releaseAgentRequests(target);
  assert.equal(released, 1);
  Reflect.set(target, Symbol.for(agentHook.AGENT_BROKER_KEY), { releaseAll: () => { throw new Error("boom"); } });
  assert.doesNotThrow(() => agentHook.releaseAgentRequests(target));
});

// --- the HTTP server does not cut a held request ----------------------------------------------------------------

test("the server timeouts leave room for a request held for 110 seconds", () => {
  const server = serverModule.createDotHttpServer((_request, response) => response.end());
  assert.equal(server.timeout, 0, "no socket timeout on a response that is still pending");
  assert.ok(server.requestTimeout === 0 || server.requestTimeout >= 120_000, "receiving the request is not what is held");
  assert.ok(serverModule.HELD_REQUEST_LIMIT_MS >= 110_000);
  server.close();
});

test("a response held longer than the idle keep-alive timeout is still delivered", async () => {
  const server = serverModule.createDotHttpServer((_request, response) => {
    setTimeout(() => {
      response.statusCode = 204;
      response.end();
    }, 400);
  });
  server.keepAliveTimeout = 50;
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const { port } = server.address();
  const status = await new Promise((resolve, reject) => {
    const call = httpRequest({ host: "127.0.0.1", port, method: "POST", agent: new Agent({ keepAlive: true }) }, (response) => {
      response.resume();
      response.on("end", () => resolve(response.statusCode));
    });
    call.on("error", reject);
    call.end("{}");
  });
  assert.equal(status, 204);
  server.closeAllConnections();
  server.close();
});

// --- showing the mascot without taking focus -------------------------------------------------------------------

function fakeWindow() {
  const calls = [];
  const record = (name) => () => calls.push(name);
  return {
    calls,
    window: { setBounds: record("bounds"), show: record("show"), showInactive: record("showInactive"), hide: record("hide"), focus: record("focus"), isVisible: () => false },
  };
}
const FULL_HD = { x: 0, y: 0, width: 1920, height: 1040 };
const controllerFor = (extra = {}) => {
  const { calls, window } = fakeWindow();
  const controller = mascot.createMascotController({ window, workAreas: () => [FULL_HD], primaryWorkArea: () => FULL_HD, savePosition: () => {}, ...extra });
  calls.length = 0;
  return { calls, controller };
};

test("attention brings the mascot up without focus, also when it was hidden, and tells the page", () => {
  let told = 0;
  const { calls, controller } = controllerFor({ onAttention: () => void (told += 1) });
  controller.attention();
  assert.deepEqual(calls.filter((call) => call !== "bounds"), ["showInactive"], "never show() or focus()");
  assert.equal(told, 1);
});

test("the controller installed for the routes offers showInactive and the page-facing parts stay as they were", () => {
  const { calls, controller } = controllerFor();
  const target = {};
  mascot.installWindowController(target, controller);
  const installed = Reflect.get(target, Symbol.for(mascot.WINDOW_CONTROLLER_KEY));
  assert.deepEqual(Object.keys(installed).sort(), ["moveTo", "setMode", "showInactive", "state"]);
  Reflect.set(globalThis, windowMode.WINDOW_CONTROLLER_KEY, installed);
  const read = windowMode.getWindowController();
  assert.equal(typeof read.showInactive, "function");
  read.showInactive();
  assert.deepEqual(calls.filter((call) => call !== "bounds"), ["showInactive"]);
  Reflect.deleteProperty(globalThis, windowMode.WINDOW_CONTROLLER_KEY);
});

test("a window controller without showInactive is still a controller, and a malformed one is not", () => {
  const base = { state: () => ({}), setMode: () => {}, moveTo: () => {} };
  Reflect.set(globalThis, windowMode.WINDOW_CONTROLLER_KEY, base);
  assert.notEqual(windowMode.getWindowController(), undefined);
  Reflect.set(globalThis, windowMode.WINDOW_CONTROLLER_KEY, { ...base, showInactive: "x" });
  assert.equal(windowMode.getWindowController(), undefined);
  Reflect.deleteProperty(globalThis, windowMode.WINDOW_CONTROLLER_KEY);
});

test("the approval card is a fourth window mode, sized beside the mascot and kept on screen", () => {
  assert.deepEqual(mascot.MASCOT_MODES, ["collapsed", "menu", "chat", "approval"]);
  assert.deepEqual([...windowMode.WINDOW_MODES], [...mascot.MASCOT_MODES]);
  assert.ok(mascot.MODE_SIZES.approval.width >= mascot.MASCOT_SIZE + 12 + 300, "room for a card next to the mascot");
  assert.ok(mascot.MODE_SIZES.approval.height >= 300);
  assert.deepEqual(windowMode.parseWindowRequest({ mode: "approval" }), { kind: "mode", mode: "approval" });
  const { calls, controller } = controllerFor();
  controller.setMode("approval", "page");
  assert.ok(calls.includes("bounds"));
  assert.equal(controller.state().mode, "approval");
  assert.throws(() => controller.setMode("nope", "page"), TypeError);
});

test("the window controller through the route stays closed to unknown modes", () => {
  assert.equal(windowMode.parseWindowRequest({ mode: "approvals" }), undefined);
  assert.equal(windowMode.parseWindowRequest({ mode: "approval", extra: 1 }), undefined);
});

// --- main wires it all in the right order ----------------------------------------------------------------------

test("main installs the hook token before the server starts, writes the discovery file after it listens and cleans up on quit", async () => {
  const source = await readFile(new URL("../desktop/main.mts", import.meta.url), "utf8");
  const install = source.indexOf("installAgentHook(globalThis");
  const start = source.indexOf("await startDotServer(");
  const write = source.indexOf("writeAgentHookFile(");
  assert.ok(install > 0 && install < start, "the token is published before the server exists");
  assert.ok(write > start, "the file is written once the server listens");
  assert.match(source, /will-quit[\s\S]*releaseAgentRequests\(globalThis\)[\s\S]*removeAgentHookFile\(/);
  assert.match(source, /onAttention/);
});
