import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { test } from "node:test";

process.env.PORT = "3006";

const mascot = await import("../desktop/mascot.mts");
const windowMode = await import("../lib/window-mode.ts");
const captureState = await import("../lib/capture-state.ts");
const voiceResult = await import("../app/mascot/voice-result.ts");
const windowRoute = await import("../app/api/window/route.ts");

const area = (x, y, width, height) => ({ x, y, width, height });
const FULL_HD = area(0, 0, 1920, 1040); // work area: the taskbar takes the bottom 40 px
const within = (inner, outer) =>
  inner.x >= outer.x && inner.y >= outer.y && inner.x + inner.width <= outer.x + outer.width && inner.y + inner.height <= outer.y + outer.height;
const centered = (bounds, workArea) => bounds.x * 2 + bounds.width === workArea.x * 2 + workArea.width;

// --- island geometry ------------------------------------------------------------------------------------------

test("the island has five window modes: a thin bar, the compact pill and three expanded cards", () => {
  assert.deepEqual(mascot.MASCOT_MODES, ["collapsed", "compact", "menu", "chat", "approval"]);
  assert.deepEqual(mascot.MODE_SIZES.collapsed, { width: 180, height: 7 });
  assert.ok(mascot.MODE_SIZES.compact.height <= 48 && mascot.MODE_SIZES.compact.width >= 400, "the pill is one line tall");
  for (const mode of ["menu", "chat", "approval"]) {
    assert.ok(mascot.MODE_SIZES[mode].width > mascot.MODE_SIZES.compact.width, `${mode} is wider than the pill`);
    assert.ok(mascot.MODE_SIZES[mode].height > mascot.MODE_SIZES.compact.height, `${mode} is taller than the pill`);
  }
  assert.ok(mascot.MODE_SIZES.chat.height >= 480, "room for the conversation");
});

test("every mode hangs from the top edge, centered on the primary work area", () => {
  for (const mode of mascot.MASCOT_MODES) {
    const bounds = mascot.islandBounds(mode, FULL_HD);
    assert.equal(bounds.y, FULL_HD.y, `${mode}: flush with the top edge`);
    assert.equal(centered(bounds, FULL_HD), true, mode);
    assert.deepEqual({ width: bounds.width, height: bounds.height }, mascot.MODE_SIZES[mode], mode);
  }
});

test("a work area that does not start at the origin (taskbar on top or left, second primary offset) keeps the island under its own top edge", () => {
  const offset = area(48, 40, 1872, 1040);
  for (const mode of mascot.MASCOT_MODES) {
    const bounds = mascot.islandBounds(mode, offset);
    assert.equal(bounds.y, 40, mode);
    assert.equal(centered(bounds, offset), true, mode);
    assert.equal(within(bounds, offset), true, mode);
  }
  const negative = area(-1920, -200, 1920, 1080);
  assert.equal(centered(mascot.islandBounds("menu", negative), negative), true);
  assert.equal(mascot.islandBounds("menu", negative).y, -200);
});

test("on a small work area the expanded island is reduced to fit instead of leaving the screen", () => {
  const small = area(0, 0, 500, 300);
  for (const mode of mascot.MASCOT_MODES) {
    const bounds = mascot.islandBounds(mode, small);
    assert.equal(within(bounds, small), true, mode);
    assert.equal(centered(bounds, small), true, mode);
  }
});

// --- mode transitions -----------------------------------------------------------------------------------------

test("clicking the island opens the menu from the thin bar or the pill and closes any open card", () => {
  assert.equal(mascot.nextModeOnMascotClick("collapsed"), "menu");
  assert.equal(mascot.nextModeOnMascotClick("compact"), "menu");
  assert.equal(mascot.nextModeOnMascotClick("menu"), "collapsed");
  assert.equal(mascot.nextModeOnMascotClick("chat"), "collapsed");
  assert.equal(mascot.nextModeOnMascotClick("approval"), "collapsed");
  assert.equal(windowMode.nextMascotMode("collapsed"), "menu");
  assert.equal(windowMode.nextMascotMode("compact"), "menu");
  assert.equal(windowMode.nextMascotMode("menu"), "collapsed");
  assert.equal(windowMode.nextMascotMode("chat"), "collapsed");
});

test("Ctrl+Shift+Space opens the chat card, or closes it when it is the open card", () => {
  assert.equal(mascot.nextModeOnShortcut("collapsed", true), "chat");
  assert.equal(mascot.nextModeOnShortcut("compact", true), "chat");
  assert.equal(mascot.nextModeOnShortcut("menu", true), "chat");
  assert.equal(mascot.nextModeOnShortcut("chat", true), "collapsed");
  // A hidden island is shown with the chat open, whatever mode it was left in.
  assert.equal(mascot.nextModeOnShortcut("chat", false), "chat");
  assert.equal(mascot.nextModeOnShortcut("collapsed", false), "chat");
});

test("the page and the desktop shell agree on the mode names", () => {
  assert.deepEqual([...windowMode.WINDOW_MODES], [...mascot.MASCOT_MODES]);
  assert.deepEqual(windowMode.parseWindowRequest({ mode: "compact" }), { kind: "mode", mode: "compact" });
});

// --- window options -------------------------------------------------------------------------------------------

test("the island window is frameless, transparent, always on top, off the taskbar and not user-resizable", () => {
  const options = mascot.mascotWindowOptions();
  assert.equal(options.frame, false);
  assert.equal(options.transparent, true);
  assert.equal(options.alwaysOnTop, true);
  assert.equal(options.skipTaskbar, true);
  assert.equal(options.resizable, false);
  assert.equal(options.hasShadow, false);
  assert.equal(options.thickFrame, false, "no frame shadow or resize border around the transparent window");
  assert.equal(options.maximizable, false);
  assert.equal(options.minimizable, false);
  assert.equal(options.fullscreenable, false);
  assert.equal(options.show, false);
  assert.equal(options.backgroundColor, "#00000000");
  assert.deepEqual({ width: options.width, height: options.height }, mascot.MODE_SIZES.collapsed);
});

test("the island window can take focus (the composer and the buttons need it) and keeps the sandbox, context isolation and no preload", () => {
  const options = mascot.mascotWindowOptions();
  assert.notEqual(options.focusable, false, "a non-focusable window could not type into the composer");
  const { webPreferences } = options;
  assert.equal(webPreferences.contextIsolation, true);
  assert.equal(webPreferences.nodeIntegration, false);
  assert.equal(webPreferences.sandbox, true);
  assert.equal("preload" in webPreferences, false);
});

// --- controller -----------------------------------------------------------------------------------------------

function fakeWindow({ withTopmost = true } = {}) {
  const calls = [];
  const state = { visible: false, bounds: area(0, 0, 0, 0) };
  return {
    calls,
    state,
    setBounds(rect) {
      state.bounds = { ...rect };
      calls.push(["setBounds", { ...rect }]);
    },
    show() {
      state.visible = true;
      calls.push(["show"]);
    },
    showInactive() {
      state.visible = true;
      calls.push(["showInactive"]);
    },
    hide() {
      state.visible = false;
      calls.push(["hide"]);
    },
    focus: () => calls.push(["focus"]),
    isVisible: () => state.visible,
    ...(withTopmost
      ? {
          setAlwaysOnTop: (flag, level) => calls.push(["setAlwaysOnTop", flag, level]),
          setVisibleOnAllWorkspaces: (visible, options) => calls.push(["setVisibleOnAllWorkspaces", visible, options]),
          moveTop: () => calls.push(["moveTop"]),
        }
      : {}),
  };
}

function controllerFor({ saved, areas = [FULL_HD], withTopmost = true } = {}) {
  const win = fakeWindow({ withTopmost });
  const saves = [];
  const notified = [];
  const live = { areas };
  const controller = mascot.createMascotController({
    window: win,
    workAreas: () => live.areas,
    primaryWorkArea: () => live.areas[0],
    position: saved,
    savePosition: (position) => saves.push(position),
    onNativeMode: (mode) => notified.push(mode),
  });
  return { controller, win, saves, notified, live };
}

test("a fresh controller starts as the thin bar at the top center of the primary work area and saves nothing", () => {
  const { controller, win, saves } = controllerFor();
  assert.equal(controller.state().mode, "collapsed");
  assert.deepEqual(win.state.bounds, mascot.islandBounds("collapsed", FULL_HD));
  assert.deepEqual(saves, []);
});

test("changing the mode resizes the island under the same top edge, centered, and never focuses the window", () => {
  const { controller, win } = controllerFor();
  for (const mode of ["compact", "menu", "chat", "approval", "collapsed"]) {
    controller.setMode(mode, "page");
    assert.equal(controller.state().mode, mode);
    assert.deepEqual(win.state.bounds, mascot.islandBounds(mode, FULL_HD), mode);
  }
  assert.equal(win.calls.some(([name]) => name === "focus" || name === "show"), false, "resizing never takes focus");
});

test("a saved position from the old draggable mascot is harmless: the island stays at the top center and nothing is saved", () => {
  const { controller, win, saves } = controllerFor({ saved: { x: 400, y: 300 } });
  assert.deepEqual(win.state.bounds, mascot.islandBounds("collapsed", FULL_HD));
  controller.setMode("menu", "page");
  assert.deepEqual(win.state.bounds, mascot.islandBounds("menu", FULL_HD));
  controller.moveTo(600, 300);
  controller.moveTo(Number.NaN, 5);
  assert.deepEqual(win.state.bounds, mascot.islandBounds("menu", FULL_HD), "the island is fixed; a move request changes nothing");
  assert.deepEqual(saves, []);
});

test("the page learns about mode changes it did not make itself", () => {
  const { controller, notified } = controllerFor();
  controller.setMode("menu", "page");
  assert.deepEqual(notified, []);
  controller.setMode("chat", "native");
  assert.deepEqual(notified, ["chat"]);
});

test("an unknown mode is rejected and changes nothing", () => {
  const { controller, win } = controllerFor();
  const before = structuredClone(win.state.bounds);
  assert.throws(() => controller.setMode("fullscreen", "page"));
  assert.deepEqual(win.state.bounds, before);
  assert.equal(controller.state().mode, "collapsed");
});

test("when the primary display changes the island follows its work area", () => {
  const { controller, win, live } = controllerFor();
  controller.setMode("menu", "page");
  live.areas = [area(1920, 0, 1366, 728)];
  controller.reposition();
  assert.deepEqual(win.state.bounds, mascot.islandBounds("menu", live.areas[0]));
});

test("the island stays above every other window: the always-on-top level is set at creation and again each time it is shown or resized", () => {
  const { controller, win } = controllerFor();
  const tops = () => win.calls.filter(([name]) => name === "setAlwaysOnTop");
  assert.deepEqual(tops().at(0), ["setAlwaysOnTop", true, "screen-saver"], "set when the controller is created");
  assert.deepEqual(win.calls.find(([name]) => name === "setVisibleOnAllWorkspaces"), ["setVisibleOnAllWorkspaces", true, { visibleOnFullScreen: true }]);
  win.calls.length = 0;
  controller.showInactive();
  controller.attention();
  controller.show();
  controller.setMode("menu", "page");
  assert.ok(tops().length >= 4, "re-asserted on every show and resize, because Windows can drop the topmost state");
  for (const call of tops()) assert.deepEqual(call, ["setAlwaysOnTop", true, "screen-saver"]);
  assert.ok(win.calls.some(([name]) => name === "moveTop"));
});

test("a window without the topmost methods still works (the same controller drives the tests and older shells)", () => {
  const { controller, win } = controllerFor({ withTopmost: false });
  controller.showInactive();
  controller.setMode("chat", "page");
  assert.deepEqual(win.state.bounds, mascot.islandBounds("chat", FULL_HD));
});

test("showing the island puts it on screen and focuses it; a startup show or attention does not steal focus", () => {
  const { controller, win } = controllerFor();
  controller.showInactive();
  assert.deepEqual(win.calls.filter(([name]) => name === "showInactive" || name === "show" || name === "focus").map(([name]) => name), ["showInactive"]);
  win.calls.length = 0;
  controller.show();
  assert.deepEqual(win.calls.filter(([name]) => ["show", "focus", "showInactive"].includes(name)).map(([name]) => name), ["show", "focus"]);
});

test("the shortcut shows a hidden island with the chat open and toggles the chat card otherwise", () => {
  const { controller, win, notified } = controllerFor();
  controller.toggleChat();
  assert.equal(win.state.visible, true);
  assert.equal(controller.state().mode, "chat");
  assert.deepEqual(notified, ["chat"]);
  controller.toggleChat();
  assert.equal(controller.state().mode, "collapsed");
  assert.equal(win.state.visible, true, "closing the chat leaves the island, never an invisible window");
  assert.deepEqual(notified, ["chat", "collapsed"]);
});

test("the controller is published for the page the same way the capture controller is", () => {
  const target = {};
  const { controller } = controllerFor();
  mascot.installWindowController(target, controller);
  assert.equal(mascot.WINDOW_CONTROLLER_KEY, "click-dot.window-controller");
  assert.equal(Symbol.keyFor(windowMode.WINDOW_CONTROLLER_KEY), mascot.WINDOW_CONTROLLER_KEY);
  const installed = target[Symbol.for("click-dot.window-controller")];
  assert.deepEqual(Object.keys(installed).sort(), ["moveTo", "setMode", "showInactive", "state"]);
  installed.setMode("menu");
  assert.equal(controller.state().mode, "menu");
});


const KEY = Symbol.for("click-dot.window-controller");
const HOST = "127.0.0.1:3006";
const request = (method, body, { headers = {}, raw } = {}) =>
  new Request(`http://${HOST}/api/window`, {
    method,
    headers: { host: HOST, origin: `http://${HOST}`, "content-type": "application/json", ...headers },
    ...(method === "POST" ? { body: raw ?? JSON.stringify(body) } : {}),
  });

function installFakeWindow() {
  const calls = [];
  let state = { mode: "collapsed", anchor: { horizontal: "right", vertical: "bottom" }, secret: "nope" };
  Reflect.set(globalThis, KEY, {
    state: () => state,
    setMode: (mode) => {
      calls.push(["mode", mode]);
      state = { ...state, mode };
    },
    moveTo: (x, y) => calls.push(["move", x, y]),
  });
  return calls;
}

test("the window request parser accepts exactly a mode or a move and nothing else", () => {
  assert.deepEqual(windowMode.parseWindowRequest({ mode: "menu" }), { kind: "mode", mode: "menu" });
  assert.deepEqual(windowMode.parseWindowRequest({ move: { x: -1200, y: 40 } }), { kind: "move", x: -1200, y: 40 });
  for (const bad of [
    null,
    [],
    "menu",
    {},
    { mode: "MENU" },
    { mode: "huge" },
    { mode: 1 },
    { mode: "menu", extra: true },
    { mode: "menu", move: { x: 1, y: 2 } },
    { move: { x: 1 } },
    { move: { x: 1, y: 2, z: 3 } },
    { move: { x: 1.5, y: 2 } },
    { move: { x: "1", y: 2 } },
    { move: { x: Number.NaN, y: 2 } },
    { move: { x: 100001, y: 0 } },
    { move: { x: 0, y: -100001 } },
    { move: [1, 2] },
    { move: null },
  ]) {
    assert.equal(windowMode.parseWindowRequest(bad), undefined, JSON.stringify(bad));
  }
});

test("a window state is read field by field and malformed ones are rejected whole", () => {
  assert.deepEqual(windowMode.sanitizeWindowState({ mode: "chat", anchor: { horizontal: "left", vertical: "top" }, extra: 1 }), {
    mode: "chat",
    anchor: { horizontal: "left", vertical: "top" },
  });
  for (const bad of [null, {}, { mode: "chat" }, { mode: "chat", anchor: { horizontal: "middle", vertical: "top" } }, { mode: "x", anchor: { horizontal: "left", vertical: "top" } }]) {
    assert.equal(windowMode.sanitizeWindowState(bad), undefined, JSON.stringify(bad));
  }
});

test("GET reports the mode and anchor only, and requires a local Host", async () => {
  installFakeWindow();
  const ok = await windowRoute.GET(request("GET"));
  assert.equal(ok.status, 200);
  assert.deepEqual(await ok.json(), { available: true, mode: "collapsed", anchor: { horizontal: "right", vertical: "bottom" } });
  assert.match(ok.headers.get("cache-control"), /no-store/);
  const foreign = await windowRoute.GET(request("GET", undefined, { headers: { host: "evil.example" } }));
  assert.equal(foreign.status, 403);
});

test("GET without the desktop app says the window cannot be controlled", async () => {
  Reflect.deleteProperty(globalThis, KEY);
  const response = await windowRoute.GET(request("GET"));
  assert.equal(response.status, 200);
  assert.equal((await response.json()).available, false);
});

test("POST changes the mode and returns the new state", async () => {
  const calls = installFakeWindow();
  const response = await windowRoute.POST(request("POST", { mode: "chat" }));
  assert.equal(response.status, 200);
  const body = await response.json();
  assert.equal(body.mode, "chat");
  assert.equal("secret" in body, false);
  assert.deepEqual(calls, [["mode", "chat"]]);
});

test("POST moves the window to the validated target", async () => {
  const calls = installFakeWindow();
  const response = await windowRoute.POST(request("POST", { move: { x: 120, y: -40 } }));
  assert.equal(response.status, 200);
  assert.deepEqual(calls, [["move", 120, -40]]);
});

test("POST rejects foreign origins, missing origin, wrong content type, oversized and malformed bodies before the controller is touched", async () => {
  const calls = installFakeWindow();
  const results = await Promise.all([
    windowRoute.POST(request("POST", { mode: "menu" }, { headers: { origin: "http://evil.example" } })),
    windowRoute.POST(request("POST", { mode: "menu" }, { headers: { host: "evil.example" } })),
    windowRoute.POST(new Request(`http://${HOST}/api/window`, { method: "POST", headers: { host: HOST, "content-type": "application/json" }, body: '{"mode":"menu"}' })),
    windowRoute.POST(request("POST", { mode: "menu" }, { headers: { "content-type": "text/plain" } })),
    windowRoute.POST(request("POST", undefined, { raw: JSON.stringify({ mode: "menu", pad: "x".repeat(400) }) })),
    windowRoute.POST(request("POST", undefined, { raw: "{nope" })),
    windowRoute.POST(request("POST", { mode: "everything" })),
    windowRoute.POST(request("POST", { move: { x: 1e9, y: 0 } })),
  ]);
  assert.deepEqual(results.map((response) => response.status), [403, 403, 403, 400, 413, 400, 400, 400]);
  assert.deepEqual(calls, []);
});

test("POST without the desktop app answers 503", async () => {
  Reflect.deleteProperty(globalThis, KEY);
  const response = await windowRoute.POST(request("POST", { mode: "menu" }));
  assert.equal(response.status, 503);
});

test("POST answers 503 without leaking the error when the controller throws", async () => {
  Reflect.set(globalThis, KEY, {
    state: () => ({ mode: "collapsed", anchor: { horizontal: "left", vertical: "top" } }),
    setMode: () => {
      throw new Error("C:\\secret\\path exploded");
    },
    moveTo: () => {},
  });
  const response = await windowRoute.POST(request("POST", { mode: "menu" }));
  assert.equal(response.status, 503);
  assert.doesNotMatch(await response.text(), /secret/);
  Reflect.deleteProperty(globalThis, KEY);
});


test("the mascot ring shows what capture really does", () => {
  assert.equal(captureState.captureIndicator(null), "unavailable");
  assert.equal(captureState.captureIndicator({ available: false, enabled: true, paused: false }), "unavailable");
  assert.equal(captureState.captureIndicator({ available: true, enabled: false, paused: false }), "off");
  assert.equal(captureState.captureIndicator({ available: true, enabled: true, paused: false }), "live");
  assert.equal(captureState.captureIndicator({ available: true, enabled: true, paused: true }), "paused");
});

test("a status body is read into the three flags and anything malformed is rejected", () => {
  assert.deepEqual(captureState.readCaptureView({ available: true, enabled: true, paused: false, frames: 3 }), { available: true, enabled: true, paused: false });
  for (const bad of [null, "x", {}, { available: true, enabled: true }, { available: "yes", enabled: true, paused: false }]) {
    assert.equal(captureState.readCaptureView(bad), undefined, JSON.stringify(bad));
  }
});

test("the indicator view survives isolated failed polls and drops after the same limit as the chat panel", () => {
  const live = { available: true, enabled: true, paused: false };
  let state = captureState.applyCaptureViewPoll(captureState.INITIAL_CAPTURE_VIEW_POLL, { ok: true, view: live });
  assert.deepEqual(state, { view: live, failures: 0 });
  for (let index = 1; index < captureState.CAPTURE_FAILURE_LIMIT; index += 1) {
    state = captureState.applyCaptureViewPoll(state, { ok: false });
    assert.deepEqual(state.view, live);
  }
  state = captureState.applyCaptureViewPoll(state, { ok: false });
  assert.equal(state.view, null);
  assert.equal(captureState.captureIndicator(state.view), "unavailable");
  state = captureState.applyCaptureViewPoll(state, { ok: true, view: live });
  assert.deepEqual(state, { view: live, failures: 0 });
});

// --- voice results --------------------------------------------------------------------------------------------

test("a transcript is trimmed and returned for review", () => {
  assert.deepEqual(voiceResult.readVoiceOutcome(200, { text: "  hola dot  " }), { kind: "text", text: "hola dot" });
  assert.equal(voiceResult.readVoiceOutcome(200, { text: "x".repeat(9000) }).text.length, 8000, "the composer limit");
});

test("silence is reported as nothing heard", () => {
  assert.deepEqual(voiceResult.readVoiceOutcome(200, { text: "   " }), { kind: "empty" });
  assert.deepEqual(voiceResult.readVoiceOutcome(200, {}), { kind: "empty" });
});

test("a missing or unavailable voice route gets one short Spanish message", () => {
  for (const status of [404, 503]) {
    assert.deepEqual(voiceResult.readVoiceOutcome(status, { code: "x", error: "English details" }), { kind: "error", message: voiceResult.VOICE_UNAVAILABLE_MESSAGE });
    assert.deepEqual(voiceResult.readVoiceOutcome(status, undefined), { kind: "error", message: voiceResult.VOICE_UNAVAILABLE_MESSAGE });
  }
  assert.match(voiceResult.VOICE_UNAVAILABLE_MESSAGE, /voz/i);
});

test("other voice failures keep the helper's message, bounded, and a cancel is not an error", () => {
  const outcome = voiceResult.readVoiceOutcome(500, { code: "no-microphone", error: "No microphone was found." });
  assert.equal(outcome.kind, "error");
  assert.match(outcome.message, /No microphone was found\./);
  assert.ok(voiceResult.readVoiceOutcome(500, { code: "x", error: "y".repeat(1000) }).message.length < 300);
  assert.deepEqual(voiceResult.readVoiceOutcome(499, { code: "cancelled", error: "Listening was cancelled." }), { kind: "cancelled" });
  assert.equal(voiceResult.readVoiceOutcome(500, "garbage").kind, "error");
  assert.equal(voiceResult.readVoiceOutcome(200, { code: "x", error: "late failure" }).kind, "error");
});

// --- the page -------------------------------------------------------------------------------------------------

test("the mascot page is rendered per request so its canonical origin uses the running port", async () => {
  const source = await readFile(new URL("../app/mascot/page.tsx", import.meta.url), "utf8");
  assert.match(source, /^export const dynamic = "force-dynamic";$/m);
});

test("the mascot window loads the /mascot page and the home page stays for the system browser", async () => {
  const config = await import("../desktop/config.mts");
  assert.equal(config.mascotUrl(), "http://127.0.0.1:47321/mascot");
  assert.equal(config.dotUrl(), "http://127.0.0.1:47321/");
  const navigation = await import("../desktop/navigation.mts");
  assert.equal(navigation.isAllowedNavigation(config.mascotUrl()), true);
});

// --- source-level guards (there is no DOM or Electron here, so these read the files like the scaffold tests do) -----

const read = (path) => readFile(new URL(`../${path}`, import.meta.url), "utf8");

test("the menu carries the user's three labels exactly and the chat bubble offers the ChatGPT connection", async () => {
  const source = await read("app/mascot/mascot.tsx");
  for (const label of ["Compartir mi pantalla", ">Voz<", ">Mensaje<", "Conectar con ChatGPT"]) {
    assert.ok(source.includes(label) || source.includes(label.replace(/[<>]/g, "")), label);
  }
  assert.match(source, /useAuthConnection\(canonicalOrigin\)/);
  assert.match(source, /connect\(\)/, "the prompt starts the existing sign-in flow");
});

test("the island is fixed at the top center: no drag region and no hand-made dragging", async () => {
  assert.doesNotMatch(await read("app/globals.css"), /app-region/);
  const source = await read("app/mascot/mascot.tsx");
  assert.doesNotMatch(source, /onPointerDown|setPointerCapture|exceedsDragThreshold|dragTarget/);
  assert.match(source, /onClick=\{toggle\}/, "clicking the island still opens and closes it");
});

test("the shell keeps the sandbox without a preload and the tray gets Mostrar a Dot beside the unchanged items", async () => {
  const main = await read("desktop/main.mts");
  assert.doesNotMatch(main, /preload/);
  assert.match(main, /label: "Mostrar a Dot"/);
  assert.doesNotMatch(main, /Open Dot/);
  for (const kept of ['"Let Dot see my screen"', '"Start with Windows"', 'label: "Quit"']) assert.ok(main.includes(kept), kept);
  assert.match(main, /setPermissionRequestHandler\(\(_contents, _permission, callback\) => callback\(false\)\)/);
  assert.match(main, /requestSingleInstanceLock/);
  assert.match(main, /loadURL\(mascotUrl\(\)\)/);
});

test("the shared chat parts keep the home page wording and the bubble gets its own", async () => {
  const parts = await read("app/chat/parts.tsx");
  for (const wording of ["Ask a question…", "Include my screen in this message", "Send message", "Cancel response"]) assert.ok(parts.includes(wording), wording);
  const compact = await read("app/chat/compact-chat.tsx");
  assert.match(compact, /useChat/);
  assert.match(await read("app/chat-panel.tsx"), /api\/chat\/stream/);
});

// --- the sharing indicator is never hidden: capture state has priority over listening --------------------------

test("capture keeps the ring and the visible label while Voz is listening, in every capture state", () => {
  for (const listening of [false, true]) {
    assert.equal(captureState.mascotIndicator("live", listening).ring, "live");
    assert.equal(captureState.mascotIndicator("paused", listening).ring, "paused");
    assert.equal(captureState.mascotIndicator("off", listening).ring, "off");
    assert.equal(captureState.mascotIndicator("unavailable", listening).ring, "unavailable");
  }
});

test("while sharing there is always visible text and a pause or resume control, listening or not", () => {
  for (const listening of [false, true]) {
    const live = captureState.mascotIndicator("live", listening);
    assert.equal(live.sharing, true);
    assert.equal(live.sharingText, "Viendo tu pantalla");
    assert.deepEqual(live.pause, { action: "pause", name: "Pausar", glyph: "pause" });
    const paused = captureState.mascotIndicator("paused", listening);
    assert.equal(paused.sharing, true);
    assert.equal(paused.sharingText, "Pantalla en pausa");
    assert.deepEqual(paused.pause, { action: "resume", name: "Reanudar", glyph: "play" });
  }
});

test("when nothing is shared there is no sharing text or pause control", () => {
  for (const indicator of ["off", "unavailable"]) {
    for (const listening of [false, true]) {
      const view = captureState.mascotIndicator(indicator, listening);
      assert.equal(view.sharing, false);
      assert.equal(view.sharingText, null);
      assert.equal(view.pause, null);
    }
  }
});

test("listening has its own visual and cancel control without replacing the capture ones", () => {
  const both = captureState.mascotIndicator("live", true);
  assert.equal(both.listening, true);
  assert.equal(both.cancel, true);
  assert.notEqual(both.pause, null);
  assert.match(both.label, /Viendo tu pantalla/);
  assert.match(both.label, /Escuchando/);
  const only = captureState.mascotIndicator("off", true);
  assert.equal(only.listening, true);
  assert.equal(only.cancel, true);
  assert.match(only.label, /Escuchando/);
  const quiet = captureState.mascotIndicator("off", false);
  assert.equal(quiet.listening, false);
  assert.equal(quiet.cancel, false);
  assert.equal(quiet.label, "Dot");
});

test("the accessible label names capture first so a screen reader never misses it", () => {
  assert.equal(captureState.mascotIndicator("live", false).label, "Viendo tu pantalla");
  assert.equal(captureState.mascotIndicator("paused", false).label, "Pantalla en pausa");
  assert.ok(captureState.mascotIndicator("live", true).label.startsWith("Viendo tu pantalla"));
});

test("the island shows the sharing text and the pause control in the pill, the menu, the chat and the permission card", async () => {
  const source = await readFile(new URL("../app/mascot/mascot.tsx", import.meta.url), "utf8");
  assert.doesNotMatch(source, /voice\.listening \? "listening"/, "listening no longer replaces the capture ring");
  assert.match(source, /mascotIndicator\(/);
  // One capture chip lives in the head, which every mode except the thin bar renders; the thin bar is never the resting state while sharing.
  assert.match(source, /mode !== "collapsed" && \(/);
  assert.match(source, /className=\{`island-capture/);
  assert.match(source, /view\.sharingText/);
  assert.match(source, /data-listening=/);
  assert.match(source, /ring=\{view\.ring\}/, "the character carries the capture ring");
});

// --- "Voz" needs a ChatGPT connection -------------------------------------------------------------------------

test("Voz is disabled with a short Spanish hint until ChatGPT is connected", () => {
  for (const connected of [null, false]) {
    const decision = voiceResult.voiceAvailability({ connected, listening: false });
    assert.equal(decision.disabled, true, String(connected));
    assert.match(decision.hint, /ChatGPT/);
    assert.ok(decision.hint.length <= 60, "short");
  }
  assert.deepEqual(voiceResult.voiceAvailability({ connected: true, listening: false }), { disabled: false, hint: null });
});

test("a running listen can always be cancelled, even if the connection state changes under it", () => {
  for (const connected of [null, false, true]) assert.equal(voiceResult.voiceAvailability({ connected, listening: true }).disabled, false);
});

test("the Voz button uses the decision and shows the hint", async () => {
  const source = await readFile(new URL("../app/mascot/mascot.tsx", import.meta.url), "utf8");
  assert.match(source, /voiceAvailability\(/);
});
