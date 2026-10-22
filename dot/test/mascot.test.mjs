import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { test } from "node:test";

process.env.PORT = "3006";

const mascot = await import("../desktop/mascot.mts");
const windowMode = await import("../lib/window-mode.ts");
const captureState = await import("../lib/capture-state.ts");
const voiceResult = await import("../app/mascot/voice-result.ts");
const windowRoute = await import("../app/api/window/route.ts");

const SIZE = mascot.MASCOT_SIZE;
const MARGIN = mascot.MASCOT_MARGIN;
const area = (x, y, width, height) => ({ x, y, width, height });
const FULL_HD = area(0, 0, 1920, 1040); // work area: the taskbar takes the bottom 40 px
const within = (inner, outer) =>
  inner.x >= outer.x && inner.y >= outer.y && inner.x + inner.width <= outer.x + outer.width && inner.y + inner.height <= outer.y + outer.height;
const mascotRect = (position) => ({ ...position, width: SIZE, height: SIZE });

// --- geometry -------------------------------------------------------------------------------------------------

test("the mascot starts in the bottom-right corner of the primary work area with a margin", () => {
  assert.equal(SIZE, 96);
  assert.deepEqual(mascot.defaultMascotPosition(FULL_HD), { x: 1920 - SIZE - MARGIN, y: 1040 - SIZE - MARGIN });
  // A work area that does not start at the origin (taskbar on the left or a second primary offset).
  assert.deepEqual(mascot.defaultMascotPosition(area(48, 0, 1872, 1080)), { x: 48 + 1872 - SIZE - MARGIN, y: 1080 - SIZE - MARGIN });
});

test("a saved position inside a display is kept as is", () => {
  assert.deepEqual(mascot.clampMascotPosition({ x: 300, y: 200 }, [FULL_HD]), { x: 300, y: 200 });
});

test("a position past any edge is pulled back so the whole mascot stays visible", () => {
  assert.deepEqual(mascot.clampMascotPosition({ x: 5000, y: 5000 }, [FULL_HD]), { x: 1920 - SIZE, y: 1040 - SIZE });
  assert.deepEqual(mascot.clampMascotPosition({ x: -50, y: -50 }, [FULL_HD]), { x: 0, y: 0 });
  assert.deepEqual(mascot.clampMascotPosition({ x: 1900, y: 1030 }, [FULL_HD]), { x: 1920 - SIZE, y: 1040 - SIZE });
});

test("negative coordinates on a display left of or above the primary are valid positions", () => {
  const left = area(-1920, 0, 1920, 1040);
  const above = area(0, -1080, 1920, 1080);
  assert.deepEqual(mascot.clampMascotPosition({ x: -1000, y: 400 }, [left, FULL_HD]), { x: -1000, y: 400 });
  assert.deepEqual(mascot.clampMascotPosition({ x: 200, y: -600 }, [above, FULL_HD]), { x: 200, y: -600 });
  // Past the left edge of the left display, not onto the primary one.
  assert.deepEqual(mascot.clampMascotPosition({ x: -2500, y: 400 }, [left, FULL_HD]), { x: -1920, y: 400 });
});

test("a position on a display that is no longer connected moves to the nearest remaining one", () => {
  // The saved spot was on a second monitor to the right that is gone now.
  assert.deepEqual(mascot.clampMascotPosition({ x: 2500, y: 300 }, [FULL_HD]), { x: 1920 - SIZE, y: 300 });
  // Two displays: a position in the gap between them goes to the closest one.
  const right = area(2000, 0, 1920, 1040);
  assert.deepEqual(mascot.clampMascotPosition({ x: 1900, y: 100 }, [FULL_HD, right]), { x: 1920 - SIZE, y: 100 });
  assert.equal(within(mascotRect(mascot.clampMascotPosition({ x: 1990, y: 100 }, [FULL_HD, right])), right), true);
});

test("the nearest work area is the one holding the point, else the closest", () => {
  const right = area(2000, 0, 1920, 1040);
  assert.deepEqual(mascot.nearestWorkArea({ x: 2100, y: 10 }, [FULL_HD, right]), right);
  assert.deepEqual(mascot.nearestWorkArea({ x: 100, y: 10 }, [FULL_HD, right]), FULL_HD);
  assert.deepEqual(mascot.nearestWorkArea({ x: 5000, y: 10 }, [FULL_HD, right]), right);
});

test("the three window modes have fixed sizes: mascot only, compact menu, chat bubble beside the mascot", () => {
  assert.deepEqual(mascot.MASCOT_MODES, ["collapsed", "menu", "chat"]);
  assert.deepEqual(mascot.MODE_SIZES.collapsed, { width: SIZE, height: SIZE });
  assert.ok(mascot.MODE_SIZES.menu.width < 260 && mascot.MODE_SIZES.menu.height < 320, "the menu stays compact");
  // The chat bubble is about 360 x 520 and sits next to the mascot, so the window adds the mascot column.
  assert.equal(mascot.MODE_SIZES.chat.height, 520);
  assert.ok(mascot.MODE_SIZES.chat.width >= 360 + SIZE && mascot.MODE_SIZES.chat.width <= 360 + SIZE + 40);
});

test("the collapsed window is exactly the mascot", () => {
  const position = { x: 700, y: 400 };
  const anchor = mascot.anchorFor(position, FULL_HD);
  assert.deepEqual(mascot.placeWindow({ mascot: position, mode: "collapsed", anchor, areas: [FULL_HD] }), mascotRect(position));
});

test("near the bottom-right corner the menu and chat grow up and to the left, and the mascot does not move", () => {
  const position = mascot.defaultMascotPosition(FULL_HD);
  const anchor = mascot.anchorFor(position, FULL_HD);
  assert.deepEqual(anchor, { horizontal: "right", vertical: "bottom" });
  for (const mode of ["menu", "chat"]) {
    const bounds = mascot.placeWindow({ mascot: position, mode, anchor, areas: [FULL_HD] });
    assert.equal(within(bounds, FULL_HD), true, mode);
    const offset = mascot.mascotOffset(anchor, bounds);
    assert.deepEqual({ x: bounds.x + offset.x, y: bounds.y + offset.y }, position, `${mode}: mascot stays where it was`);
    assert.equal(bounds.y + bounds.height, position.y + SIZE, `${mode}: bottom edge shared with the mascot`);
  }
  const chat = mascot.placeWindow({ mascot: position, mode: "chat", anchor, areas: [FULL_HD] });
  assert.equal(chat.x + chat.width, position.x + SIZE, "the bubble is to the left of a mascot at the right edge");
});

test("near the top-left corner the menu and chat grow down and to the right", () => {
  const position = { x: MARGIN, y: MARGIN };
  const anchor = mascot.anchorFor(position, FULL_HD);
  assert.deepEqual(anchor, { horizontal: "left", vertical: "top" });
  const chat = mascot.placeWindow({ mascot: position, mode: "chat", anchor, areas: [FULL_HD] });
  assert.deepEqual({ x: chat.x, y: chat.y }, position);
  assert.equal(within(chat, FULL_HD), true);
  const menu = mascot.placeWindow({ mascot: position, mode: "menu", anchor, areas: [FULL_HD] });
  assert.equal(menu.y, position.y, "the menu hangs below a mascot at the top");
  assert.equal(within(menu, FULL_HD), true);
});

test("the anchor follows the half of the work area the mascot is in", () => {
  assert.deepEqual(mascot.anchorFor({ x: 100, y: 100 }, FULL_HD), { horizontal: "left", vertical: "top" });
  assert.deepEqual(mascot.anchorFor({ x: 1700, y: 100 }, FULL_HD), { horizontal: "right", vertical: "top" });
  assert.deepEqual(mascot.anchorFor({ x: 100, y: 900 }, FULL_HD), { horizontal: "left", vertical: "bottom" });
});

test("every mode stays inside the work area wherever the mascot is, including multi-display and negative coordinates", () => {
  const left = area(-1366, 100, 1366, 728);
  const areas = [left, area(0, 0, 1920, 1040)];
  const positions = [
    { x: -1366, y: 100 },
    { x: -96, y: 828 - 96 },
    { x: -700, y: 400 },
    { x: 0, y: 0 },
    { x: 960, y: 520 },
    { x: 1920 - SIZE, y: 1040 - SIZE },
  ];
  for (const raw of positions) {
    const position = mascot.clampMascotPosition(raw, areas);
    const workArea = mascot.nearestWorkArea({ x: position.x + SIZE / 2, y: position.y + SIZE / 2 }, areas);
    const anchor = mascot.anchorFor(position, workArea);
    for (const mode of mascot.MASCOT_MODES) {
      const bounds = mascot.placeWindow({ mascot: position, mode, anchor, areas });
      assert.equal(within(bounds, workArea), true, `${mode} at ${JSON.stringify(position)}`);
    }
  }
});

test("on a small work area the chat window is reduced to fit instead of leaving the screen", () => {
  const small = area(0, 0, 800, 440);
  const position = mascot.defaultMascotPosition(small);
  const anchor = mascot.anchorFor(position, small);
  const bounds = mascot.placeWindow({ mascot: position, mode: "chat", anchor, areas: [small] });
  assert.equal(within(bounds, small), true);
  assert.ok(bounds.height <= 440);
});

// --- mode transitions -----------------------------------------------------------------------------------------

test("clicking the mascot opens and closes the three-button menu; from the chat it closes everything", () => {
  assert.equal(mascot.nextModeOnMascotClick("collapsed"), "menu");
  assert.equal(mascot.nextModeOnMascotClick("menu"), "collapsed");
  assert.equal(mascot.nextModeOnMascotClick("chat"), "collapsed");
  assert.equal(windowMode.nextMascotMode("collapsed"), "menu");
  assert.equal(windowMode.nextMascotMode("menu"), "collapsed");
  assert.equal(windowMode.nextMascotMode("chat"), "collapsed");
});

test("Ctrl+Shift+Space opens the chat bubble, or closes it when it is the open bubble", () => {
  assert.equal(mascot.nextModeOnShortcut("collapsed", true), "chat");
  assert.equal(mascot.nextModeOnShortcut("menu", true), "chat");
  assert.equal(mascot.nextModeOnShortcut("chat", true), "collapsed");
  // A hidden mascot is shown with the chat open, whatever mode it was left in.
  assert.equal(mascot.nextModeOnShortcut("chat", false), "chat");
  assert.equal(mascot.nextModeOnShortcut("collapsed", false), "chat");
});

test("the page and the desktop shell agree on the mode names", () => {
  assert.deepEqual([...windowMode.WINDOW_MODES], [...mascot.MASCOT_MODES]);
});

// --- window options -------------------------------------------------------------------------------------------

test("the mascot window is frameless, transparent, always on top, off the taskbar and not user-resizable", () => {
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
  assert.deepEqual({ width: options.width, height: options.height }, { width: SIZE, height: SIZE });
});

test("the mascot window keeps the sandbox, context isolation and no preload", () => {
  const { webPreferences } = mascot.mascotWindowOptions();
  assert.equal(webPreferences.contextIsolation, true);
  assert.equal(webPreferences.nodeIntegration, false);
  assert.equal(webPreferences.sandbox, true);
  assert.equal("preload" in webPreferences, false);
});

// --- controller -----------------------------------------------------------------------------------------------

function fakeWindow() {
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
  };
}

function controllerFor({ saved, areas = [FULL_HD] } = {}) {
  const win = fakeWindow();
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

test("a fresh controller starts collapsed in the default corner and does not save anything", () => {
  const { controller, win, saves } = controllerFor();
  const position = mascot.defaultMascotPosition(FULL_HD);
  assert.deepEqual(controller.position(), position);
  assert.deepEqual(controller.state().mode, "collapsed");
  assert.deepEqual(win.state.bounds, mascotRect(position));
  assert.deepEqual(saves, []);
});

test("a saved position is used when it is on a display and clamped when it is not", () => {
  assert.deepEqual(controllerFor({ saved: { x: 400, y: 300 } }).controller.position(), { x: 400, y: 300 });
  assert.deepEqual(controllerFor({ saved: { x: 9000, y: 9000 } }).controller.position(), { x: 1920 - SIZE, y: 1040 - SIZE });
  const { saves } = controllerFor({ saved: { x: 9000, y: 9000 } });
  assert.deepEqual(saves, [], "a startup clamp does not overwrite the saved choice");
});

test("changing the mode resizes the window around the fixed mascot", () => {
  const { controller, win } = controllerFor();
  const position = controller.position();
  controller.setMode("menu", "page");
  assert.deepEqual(controller.state().mode, "menu");
  const menu = win.state.bounds;
  assert.deepEqual({ width: menu.width, height: menu.height }, mascot.MODE_SIZES.menu);
  controller.setMode("chat", "page");
  const chat = win.state.bounds;
  assert.deepEqual({ width: chat.width, height: chat.height }, mascot.MODE_SIZES.chat);
  assert.equal(within(chat, FULL_HD), true);
  controller.setMode("collapsed", "page");
  assert.deepEqual(win.state.bounds, mascotRect(position));
  assert.deepEqual(controller.position(), position);
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

test("the anchor is chosen when the mode changes, not while dragging, so the layout does not flip mid-drag", () => {
  const { controller } = controllerFor({ saved: { x: 1700, y: 900 } });
  controller.setMode("chat", "page");
  const opened = controller.state().anchor;
  assert.deepEqual(opened, { horizontal: "right", vertical: "bottom" });
  const target = { x: 100, y: 50 };
  controller.moveTo(target.x, target.y);
  assert.deepEqual(controller.state().anchor, opened);
});

test("dragging moves the mascot, keeps it on screen and saves the position it ends at", () => {
  const { controller, win, saves } = controllerFor();
  controller.moveTo(600, 300);
  assert.deepEqual(controller.position(), { x: 600, y: 300 });
  assert.deepEqual(win.state.bounds, mascotRect({ x: 600, y: 300 }));
  assert.deepEqual(saves.at(-1), { x: 600, y: 300 });
  controller.moveTo(99999, -99999);
  assert.deepEqual(controller.position(), { x: 1920 - SIZE, y: 0 });
  assert.deepEqual(saves.at(-1), { x: 1920 - SIZE, y: 0 });
});

test("dragging while the menu is open moves the whole window with the mascot inside it", () => {
  const { controller, win } = controllerFor({ saved: { x: 800, y: 800 } });
  controller.setMode("menu", "page");
  const before = { ...win.state.bounds };
  controller.moveTo(before.x + 100, before.y - 120);
  assert.deepEqual({ x: win.state.bounds.x, y: win.state.bounds.y }, { x: before.x + 100, y: before.y - 120 });
  assert.deepEqual(controller.position(), { x: 900, y: 680 });
});

test("a non-finite drag target is ignored", () => {
  const { controller, saves } = controllerFor();
  const before = controller.position();
  controller.moveTo(Number.NaN, 5);
  controller.moveTo(5, Number.POSITIVE_INFINITY);
  assert.deepEqual(controller.position(), before);
  assert.deepEqual(saves, []);
});

test("when a display goes away the mascot moves onto a visible one without overwriting the saved position", () => {
  const right = area(1920, 0, 1920, 1040);
  const { controller, win, saves, live } = controllerFor({ saved: { x: 2500, y: 200 }, areas: [FULL_HD, right] });
  assert.deepEqual(controller.position(), { x: 2500, y: 200 });
  live.areas = [FULL_HD];
  controller.reposition();
  assert.deepEqual(controller.position(), { x: 1920 - SIZE, y: 200 });
  assert.equal(within(win.state.bounds, FULL_HD), true);
  assert.deepEqual(saves, []);
});

test("showing the mascot puts it on screen and focuses it; a startup show does not steal focus", () => {
  const { controller, win } = controllerFor();
  controller.showInactive();
  assert.deepEqual(win.calls.at(-1), ["showInactive"]);
  win.calls.length = 0;
  controller.show();
  assert.deepEqual(win.calls.map(([name]) => name), ["setBounds", "show", "focus"]);
});

test("the shortcut shows a hidden mascot with the chat open and toggles the bubble otherwise", () => {
  const { controller, win, notified } = controllerFor();
  controller.toggleChat();
  assert.equal(win.state.visible, true);
  assert.equal(controller.state().mode, "chat");
  assert.deepEqual(notified, ["chat"]);
  controller.toggleChat();
  assert.equal(controller.state().mode, "collapsed");
  assert.equal(win.state.visible, true, "closing the bubble leaves the mascot, never an invisible window");
  assert.deepEqual(notified, ["chat", "collapsed"]);
});

test("the controller is published for the page the same way the capture controller is", () => {
  const target = {};
  const { controller } = controllerFor();
  mascot.installWindowController(target, controller);
  assert.equal(mascot.WINDOW_CONTROLLER_KEY, "click-dot.window-controller");
  assert.equal(Symbol.keyFor(windowMode.WINDOW_CONTROLLER_KEY), mascot.WINDOW_CONTROLLER_KEY);
  const installed = target[Symbol.for("click-dot.window-controller")];
  assert.deepEqual(Object.keys(installed).sort(), ["moveTo", "setMode", "state"]);
  installed.setMode("menu");
  assert.equal(controller.state().mode, "menu");
});

// --- window route ---------------------------------------------------------------------------------------------

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

// --- drag helpers ---------------------------------------------------------------------------------------------

test("a press only becomes a drag after the pointer moves a few pixels", () => {
  assert.equal(windowMode.exceedsDragThreshold(0, 0), false);
  assert.equal(windowMode.exceedsDragThreshold(2, 2), false);
  assert.equal(windowMode.exceedsDragThreshold(windowMode.DRAG_THRESHOLD_PX, 0), true);
  assert.equal(windowMode.exceedsDragThreshold(-3, -4), true);
});

test("the drag target follows the pointer from where the window was when the press began", () => {
  assert.deepEqual(windowMode.dragTarget({ x: 1000, y: 500 }, { x: 1020, y: 530 }, { x: 1050, y: 510 }), { x: 1030, y: 480 });
});

// --- capture indicator ----------------------------------------------------------------------------------------

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
  for (const label of ["Compartir mi pantalla", ">Voz<", ">Mensaje<", "Conectar con ChatGPT", "Viendo tu pantalla"]) {
    assert.ok(source.includes(label) || source.includes(label.replace(/[<>]/g, "")), label);
  }
  assert.match(source, /useAuthConnection\(canonicalOrigin\)/);
  assert.match(source, /connect\(\)/, "the prompt starts the existing sign-in flow");
});

test("the mascot is dragged by hand because a drag region would swallow its clicks", async () => {
  assert.doesNotMatch(await read("app/globals.css"), /app-region/);
  const source = await read("app/mascot/mascot.tsx");
  assert.match(source, /onPointerDown/);
  assert.match(source, /exceedsDragThreshold/);
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
