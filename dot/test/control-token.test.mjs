import assert from "node:assert/strict";
import { mkdtemp, readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";

const temp = await mkdtemp(join(tmpdir(), "click-dot-control-token-test-"));
process.env.PORT = "3007";
process.env.CLICK_DOT_AUTH_FILE = join(temp, "private", "auth.json");

const token = await import("../lib/control-token.ts");
const desktopToken = await import("../desktop/control-token.mts");
const captureRoute = await import("../app/api/capture/route.ts");
const statusRoute = await import("../app/api/capture/status/route.ts");
const previewRoute = await import("../app/api/capture/preview/route.ts");
const listenRoute = await import("../app/api/voice/listen/route.ts");
const cancelRoute = await import("../app/api/voice/cancel/route.ts");
const windowRoute = await import("../app/api/window/route.ts");
const authStatusRoute = await import("../app/api/auth/status/route.ts");
const authStartRoute = await import("../app/api/auth/start/route.ts");
const modelsRoute = await import("../app/api/chat/models/route.ts");

const HOST = "127.0.0.1:3007";
const GOOD = "a".repeat(64);
const OTHER = "b".repeat(64);
const KEY = Symbol.for("click-dot.control-token");
const CAPTURE_KEY = Symbol.for("click-dot.capture-controller");
const VOICE_KEY = Symbol.for("click-dot.voice-controller");
const WINDOW_KEY = Symbol.for("click-dot.window-controller");

const publish = (value) => Reflect.set(globalThis, KEY, value);
const cookie = (value, name = token.CONTROL_COOKIE) => ({ cookie: `${name}=${value}` });

function reset() {
  for (const key of [KEY, CAPTURE_KEY, VOICE_KEY, WINDOW_KEY]) Reflect.deleteProperty(globalThis, key);
}

const call = (route, method, path, { headers = {}, body } = {}) =>
  route[method](
    new Request(`http://${HOST}${path}`, {
      method,
      headers: { host: HOST, ...(method === "POST" ? { origin: `http://${HOST}` } : {}), ...(body === undefined ? {} : { "content-type": "application/json" }), ...headers },
      ...(body === undefined ? {} : { body: JSON.stringify(body) }),
    }),
  );

function installControllers() {
  const calls = { capture: [], prepare: 0, listen: 0, cancel: 0, window: [] };
  Reflect.set(globalThis, CAPTURE_KEY, {
    status: () => ({ enabled: true, paused: false, frames: 2, lastCapturedAt: 1 }),
    apply: async (action) => {
      calls.capture.push(action);
      return { enabled: true, paused: false, frames: 2, lastCapturedAt: 1 };
    },
    prepareScreen: async () => {
      calls.prepare += 1;
      return { nope: true };
    },
  });
  Reflect.set(globalThis, VOICE_KEY, {
    listen: async () => {
      calls.listen += 1;
      return { text: "hola" };
    },
    cancel: () => {
      calls.cancel += 1;
    },
  });
  Reflect.set(globalThis, WINDOW_KEY, {
    state: () => ({ mode: "collapsed", anchor: { horizontal: "right", vertical: "bottom" } }),
    setMode: (mode) => calls.window.push(["mode", mode]),
    moveTo: (x, y) => calls.window.push(["move", x, y]),
  });
  return calls;
}

const untouched = (calls) => assert.deepEqual(calls, { capture: [], prepare: 0, listen: 0, cancel: 0, window: [] });

// --- reader ---------------------------------------------------------------------------------------------------

test("the control token lives on a shared global symbol and the cookie name is fixed", () => {
  assert.equal(Symbol.keyFor(token.CONTROL_TOKEN_KEY), "click-dot.control-token");
  assert.equal(token.CONTROL_COOKIE, "click_dot_control");
});

test("an absent global means no token was published", () => {
  reset();
  assert.deepEqual(token.readControlGate(), { kind: "none" });
});

test("a valid published token is read as is", () => {
  publish(GOOD);
  assert.deepEqual(token.readControlGate(), { kind: "token", token: GOOD });
  reset();
});

test("a malformed global is never treated as 'no token'", () => {
  for (const bad of [null, "", "short", "A".repeat(64), "g".repeat(64), "a".repeat(63), "a".repeat(65), 1234, true, {}, [GOOD], () => GOOD, Symbol("x")]) {
    publish(bad);
    assert.deepEqual(token.readControlGate(), { kind: "invalid" }, String(typeof bad));
    assert.equal(token.hasControlAccess(new Request(`http://${HOST}/x`, { headers: { host: HOST, ...cookie(GOOD) } })), false);
  }
  reset();
});

test("access follows the cookie: missing, wrong, truncated, extended and misnamed cookies are refused", () => {
  const check = (headers) => token.hasControlAccess(new Request(`http://${HOST}/x`, { headers: { host: HOST, ...headers } }));
  publish(GOOD);
  assert.equal(check(cookie(GOOD)), true);
  assert.equal(check({ cookie: `other=1; ${token.CONTROL_COOKIE}=${GOOD}; more=2` }), true);
  assert.equal(check({}), false);
  assert.equal(check(cookie("")), false);
  assert.equal(check(cookie(OTHER)), false);
  assert.equal(check(cookie(GOOD.slice(0, 63))), false);
  assert.equal(check(cookie(`${GOOD}a`)), false);
  assert.equal(check(cookie(GOOD, "click_dot_other")), false);
  assert.equal(check({ cookie: `${token.CONTROL_COOKIE}=${OTHER}; ${token.CONTROL_COOKIE}=${GOOD}` }), false, "the first value is the one read");
  reset();
});

test("without a published token the check does not interfere", () => {
  reset();
  assert.equal(token.hasControlAccess(new Request(`http://${HOST}/x`, { headers: { host: HOST } })), true);
});

test("the comparison is constant time", async () => {
  const source = await readFile(new URL("../lib/control-token.ts", import.meta.url), "utf8");
  assert.match(source, /timingSafeEqual/);
});

// --- desktop side ---------------------------------------------------------------------------------------------

test("the shell generates a random 256-bit token per launch", () => {
  const first = desktopToken.createControlToken();
  const second = desktopToken.createControlToken();
  assert.match(first, /^[0-9a-f]{64}$/);
  assert.notEqual(first, second);
});

test("the shell publishes the token where the Next side reads it, and the names agree", () => {
  const target = {};
  desktopToken.installControlToken(target, GOOD);
  assert.equal(Reflect.get(target, token.CONTROL_TOKEN_KEY), GOOD);
  assert.equal(desktopToken.CONTROL_TOKEN_KEY, Symbol.keyFor(token.CONTROL_TOKEN_KEY));
  assert.equal(desktopToken.CONTROL_COOKIE, token.CONTROL_COOKIE);
  publish(Reflect.get(target, token.CONTROL_TOKEN_KEY));
  assert.deepEqual(token.readControlGate(), { kind: "token", token: GOOD });
  reset();
});

test("the shell refuses to publish a malformed token", () => {
  for (const bad of ["", "short", "g".repeat(64), 5, null]) assert.throws(() => desktopToken.installControlToken({}, bad), /control token/i);
});

test("the cookie is httpOnly, SameSite Strict, session-only and scoped to the app's own origin", () => {
  const details = desktopToken.controlCookieDetails("http://127.0.0.1:47321", GOOD);
  assert.deepEqual(details, { url: "http://127.0.0.1:47321", name: token.CONTROL_COOKIE, value: GOOD, path: "/", httpOnly: true, sameSite: "strict" });
  assert.equal("expirationDate" in details, false, "a session cookie, not a persistent one");
});

test("the main process sets the cookie and publishes the token before the server and the window start", async () => {
  const source = await readFile(new URL("../desktop/main.mts", import.meta.url), "utf8");
  const cookieAt = source.indexOf("cookies.set(");
  assert.ok(cookieAt > 0, "cookies.set is called");
  assert.ok(source.indexOf("installControlToken(globalThis") > 0);
  assert.ok(cookieAt < source.indexOf("startDotServer(app.getAppPath()"), "before the server");
  assert.ok(cookieAt < source.lastIndexOf("createMascot();"), "before the window that loads the page");
  assert.doesNotMatch(source, /console\.\w+\([^)]*[Tt]oken/);
});

// --- every control route --------------------------------------------------------------------------------------

const CONTROL = {
  "capture action": (headers) => call(captureRoute, "POST", "/api/capture", { body: { action: "enable" }, headers }),
  "capture preview": (headers) => call(previewRoute, "POST", "/api/capture/preview", { headers }),
  "voice listen": (headers) => call(listenRoute, "POST", "/api/voice/listen", { headers }),
  "voice cancel": (headers) => call(cancelRoute, "POST", "/api/voice/cancel", { headers }),
  "window change": (headers) => call(windowRoute, "POST", "/api/window", { body: { mode: "menu" }, headers }),
};

test("every control route refuses a request with a valid Origin and Host but no or a wrong cookie, before touching the controller", async () => {
  const calls = installControllers();
  publish(GOOD);
  for (const [name, send] of Object.entries(CONTROL)) {
    for (const headers of [{}, cookie(OTHER), cookie(""), cookie(GOOD, "click_dot_other")]) {
      const response = await send(headers);
      assert.equal(response.status, 403, `${name} ${JSON.stringify(headers)}`);
      assert.deepEqual(await response.json(), { error: "Request rejected." });
      assert.equal(response.headers.get("cache-control"), "no-store, private");
    }
  }
  untouched(calls);
  reset();
});

test("every control route refuses everything when the published token is malformed", async () => {
  const calls = installControllers();
  publish("not-a-token");
  for (const [name, send] of Object.entries(CONTROL)) assert.equal((await send(cookie("not-a-token"))).status, 403, name);
  untouched(calls);
  reset();
});

test("every control route works with the right cookie", async () => {
  const calls = installControllers();
  publish(GOOD);
  assert.equal((await CONTROL["capture action"](cookie(GOOD))).status, 200);
  assert.deepEqual(calls.capture, ["enable"]);
  assert.notEqual((await CONTROL["capture preview"](cookie(GOOD))).status, 403);
  assert.equal(calls.prepare, 1);
  const listened = await CONTROL["voice listen"](cookie(GOOD));
  assert.equal(listened.status, 200);
  assert.deepEqual(await listened.json(), { text: "hola" });
  assert.equal((await CONTROL["voice cancel"](cookie(GOOD))).status, 200);
  assert.equal(calls.cancel, 1);
  assert.equal((await CONTROL["window change"](cookie(GOOD))).status, 200);
  assert.deepEqual(calls.window, [["mode", "menu"]]);
  reset();
});

test("the cookie does not replace the Origin and Host checks", async () => {
  const calls = installControllers();
  publish(GOOD);
  const foreign = await call(captureRoute, "POST", "/api/capture", { body: { action: "enable" }, headers: { ...cookie(GOOD), origin: "http://evil.example" } });
  assert.equal(foreign.status, 403);
  const badHost = await call(windowRoute, "POST", "/api/window", { body: { mode: "menu" }, headers: { ...cookie(GOOD), host: "evil.example" } });
  assert.equal(badHost.status, 403);
  untouched(calls);
  reset();
});

test("without a published token the routes keep their existing 'unavailable' answers", async () => {
  reset();
  const capture = await CONTROL["capture action"]({});
  assert.equal(capture.status, 503);
  assert.equal((await capture.json()).error, "Screen capture is available in the desktop app.");
  assert.equal((await CONTROL["capture preview"]({})).status, 503);
  for (const name of ["voice listen", "voice cancel"]) {
    const response = await CONTROL[name]({});
    assert.equal(response.status, 503, name);
    assert.equal((await response.json()).code, "unavailable");
  }
  const win = await CONTROL["window change"]({});
  assert.equal(win.status, 503);
  assert.equal((await win.json()).error, "The mascot window is available in the desktop app.");
});

// --- status: honest for a client without the cookie ----------------------------------------------------------

test("capture status reports 'not available' to a client without the cookie and the real state to one with it", async () => {
  installControllers();
  publish(GOOD);
  const hidden = await call(statusRoute, "GET", "/api/capture/status");
  assert.deepEqual(await hidden.json(), { available: false, enabled: false, paused: false, frames: 0, lastCapturedAt: null });
  const shown = await call(statusRoute, "GET", "/api/capture/status", { headers: cookie(GOOD) });
  assert.deepEqual(await shown.json(), { available: true, enabled: true, paused: false, frames: 2, lastCapturedAt: 1 });
  reset();
  const open = await call(statusRoute, "GET", "/api/capture/status");
  assert.equal((await open.json()).available, false, "no controller, no token: unchanged");
});

test("the window state stays readable without the cookie", async () => {
  installControllers();
  publish(GOOD);
  const response = await call(windowRoute, "GET", "/api/window");
  assert.equal(response.status, 200);
  assert.equal((await response.json()).available, true);
  reset();
});

// --- the sign-in, status, models routes are unaffected ---------------------------------------------------------

async function snapshot(response) {
  return { status: response.status, body: await response.json(), cache: response.headers.get("cache-control") };
}

test("sign-in start behaves the same with a token published and no cookie (the system browser has none)", async () => {
  const start = () => authStartRoute.POST(new Request(`http://${HOST}/api/auth/start`, { method: "POST", headers: { host: HOST, origin: `http://${HOST}` } }));
  reset();
  const before = await start();
  const beforeBody = await before.json();
  publish(GOOD);
  const after = await start();
  assert.equal(after.status, before.status);
  assert.equal(after.status, 200);
  assert.equal(new URL((await after.json()).url).origin, new URL(beforeBody.url).origin);
  assert.match(after.headers.get("set-cookie"), /click_dot_auth_attempt=/);
  reset();
});

test("auth status behaves the same with a token published and no cookie", async () => {
  const read = () => authStatusRoute.GET(new Request(`http://${HOST}/api/auth/status`, { headers: { host: HOST } }));
  reset();
  const before = await snapshot(await read());
  publish(GOOD);
  const after = await snapshot(await read());
  assert.deepEqual(after, before);
  assert.equal(after.status, 200);
  reset();
});

test("the models route (chat family) behaves the same with a token published and no cookie", async () => {
  const read = () => modelsRoute.GET(new Request(`http://${HOST}/api/chat/models`, { headers: { host: HOST } }));
  reset();
  const before = await snapshot(await read());
  publish(GOOD);
  const after = await snapshot(await read());
  assert.deepEqual(after, before);
  assert.notEqual(after.status, 403, "the failure is 'not connected', never a token rejection");
  reset();
});

test("no route renders or returns the token", async () => {
  installControllers();
  publish(GOOD);
  const responses = [
    await call(statusRoute, "GET", "/api/capture/status", { headers: cookie(GOOD) }),
    await call(windowRoute, "GET", "/api/window", { headers: cookie(GOOD) }),
    await call(captureRoute, "POST", "/api/capture", { body: { action: "enable" }, headers: cookie(GOOD) }),
    await call(captureRoute, "POST", "/api/capture", { body: { action: "enable" } }),
    await authStatusRoute.GET(new Request(`http://${HOST}/api/auth/status`, { headers: { host: HOST } })),
  ];
  for (const response of responses) {
    assert.equal((await response.text()).includes(GOOD), false);
    assert.equal(JSON.stringify([...response.headers]).includes(GOOD), false);
  }
  reset();
});
