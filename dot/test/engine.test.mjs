import assert from "node:assert/strict";
import { test } from "node:test";

const engineLib = await import("../lib/engine.ts");

const tick = () => new Promise((resolve) => setImmediate(resolve));

/** An engine as far as selection and sign-in go. */
function fakeEngine({ state = "ready", message, account = { state: "signedIn", method: "chatgpt", email: "a@b.c", plan: "plus" }, version = "0.163.0" } = {}) {
  const calls = [];
  const waiters = [];
  return {
    calls,
    finishLogin: (value) => waiters.shift()?.(value),
    status: () => ({ state, restarts: 0, version, ...(message ? { message } : {}) }),
    accountStatus: async () => account,
    startLogin: async () => { calls.push("startLogin"); return { loginId: "L1", authUrl: "https://auth.openai.com/oauth/authorize?x=1" }; },
    waitForLogin: (loginId, signal) => new Promise((resolve) => {
      calls.push(`wait:${loginId}`);
      waiters.push(resolve);
      signal.addEventListener("abort", () => { calls.push("aborted"); resolve({ success: false, error: "cancelled" }); }, { once: true });
    }),
    activity: () => ({ computerUse: { active: false, servers: [], threadIds: [] } }),
  };
}

const tools = { chrome: { state: "ready", hint: "c" }, computer: { state: "missing", hint: "m" } };
const deps = (engine, extra = {}) => ({ getEngine: () => engine, readTools: () => tools, ...extra });

test("Codex is the engine only when it is ready and signed in; every other state falls back to the built-in loop", async () => {
  assert.equal((await engineLib.selectEngine(deps(fakeEngine())))?.kind, "codex");
  assert.equal(await engineLib.selectEngine(deps(null)), null, "no Codex integration (plain development)");
  assert.equal(await engineLib.selectEngine(deps(fakeEngine({ state: "unavailable", message: "no está" }))), null);
  assert.equal(await engineLib.selectEngine(deps(fakeEngine({ state: "starting" }))), null);
  assert.equal(await engineLib.selectEngine(deps(fakeEngine({ state: "restarting" }))), null);
  assert.equal(await engineLib.selectEngine(deps(fakeEngine({ account: { state: "signedOut" } }))), null, "signed out");
  assert.equal(await engineLib.selectEngine(deps(fakeEngine({ account: { state: "unavailable", message: "x" } }))), null);
});

test("the status says why Codex is not the engine, with the Spanish message the start produced", async () => {
  const missing = await engineLib.engineStatus(deps(fakeEngine({ state: "unavailable", message: "Dot no encontró Codex en este equipo." })));
  assert.deepEqual([missing.engine, missing.codex.state, missing.codex.message, missing.codex.account], ["builtin", "unavailable", "Dot no encontró Codex en este equipo.", null]);
  const absent = await engineLib.engineStatus(deps(null));
  assert.deepEqual([absent.engine, absent.codex.state, absent.codex.message], ["builtin", "absent", null]);
  const starting = await engineLib.engineStatus(deps(fakeEngine({ state: "starting" })));
  assert.deepEqual([starting.codex.state, starting.codex.account], ["starting", null], "the account is not asked while Codex is not ready");
  const out = await engineLib.engineStatus(deps(fakeEngine({ account: { state: "signedOut" } })));
  assert.deepEqual([out.engine, out.codex.state, out.codex.account.state], ["builtin", "ready", "signedOut"]);
  const ready = await engineLib.engineStatus(deps(fakeEngine()));
  assert.deepEqual([ready.engine, ready.codex.version, ready.codex.account.email, ready.tools], ["codex", "0.163.0", "a@b.c", tools]);
  assert.equal((await engineLib.engineStatus({ getEngine: () => null, readTools: () => null })).tools, null);
});

test("signing in opens the returned URL through the system browser and reports pending, then completed", async () => {
  const engine = fakeEngine({ account: { state: "signedOut" } });
  const opened = [];
  const flow = engineLib.createLoginFlow();
  const started = await flow.start(engine, (url) => { opened.push(url); return true; });
  assert.deepEqual(started, { ok: true, authUrl: "https://auth.openai.com/oauth/authorize?x=1", opened: true });
  assert.deepEqual(opened, ["https://auth.openai.com/oauth/authorize?x=1"]);
  assert.equal(flow.view().state, "pending");
  engine.finishLogin({ success: true, error: null });
  await tick();
  assert.equal(flow.view().state, "completed");
});

test("a failed or cancelled sign-in says so in Spanish, and starting again cancels the one in flight", async () => {
  const engine = fakeEngine({ account: { state: "signedOut" } });
  const flow = engineLib.createLoginFlow();
  await flow.start(engine, () => true);
  engine.finishLogin({ success: false, error: "access_denied" });
  await tick();
  assert.equal(flow.view().state, "failed");
  assert.match(flow.view().message, /Codex/);
  assert.ok(!flow.view().message.includes("access_denied"), "Codex's own error text is not shown");
  await flow.start(engine, () => true);
  await flow.start(engine, () => true);
  assert.ok(engine.calls.includes("aborted"), "the first wait was cancelled");
  assert.equal(flow.view().state, "pending");
  flow.cancel();
  await tick();
  assert.equal(flow.view().state, "idle");
});

test("without a system browser the URL is returned for the page to open, and a Codex that cannot start the sign-in is an error", async () => {
  const flow = engineLib.createLoginFlow();
  const started = await flow.start(fakeEngine(), null);
  assert.equal(started.opened, false);
  flow.cancel();
  const broken = { ...fakeEngine(), startLogin: async () => { throw new Error("raw boom"); } };
  const result = await flow.start(broken, () => true);
  assert.equal(result.ok, false);
  assert.ok(!result.error.includes("raw boom"));
  assert.equal(flow.view().state, "idle");
});

test("a URL the system browser refuses is not reported as opened", async () => {
  const flow = engineLib.createLoginFlow();
  const started = await flow.start(fakeEngine(), () => false);
  assert.equal(started.opened, false);
  flow.cancel();
});
