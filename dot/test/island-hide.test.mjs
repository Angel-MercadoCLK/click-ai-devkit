import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { after, test } from "node:test";

process.env.PORT = "3013";

const mascot = await import("../desktop/mascot.mts");
const preferences = await import("../desktop/preferences.mts");
const windowMode = await import("../lib/window-mode.ts");
const island = await import("../app/mascot/island-state.ts");
const windowRoute = await import("../app/api/window/route.ts");

const read = (path) => readFile(new URL(`../${path}`, import.meta.url), "utf8");
const scratch = await mkdtemp(join(tmpdir(), "dot-hide-"));
after(() => rm(scratch, { recursive: true, force: true }));

const FULL_HD = { x: 0, y: 0, width: 1920, height: 1040 };

function fakeWindow() {
  const calls = [];
  const state = { visible: false };
  return {
    calls,
    state,
    setBounds: (rect) => calls.push(["setBounds", rect.width, rect.height]),
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

function controllerFor(extra = {}) {
  const win = fakeWindow();
  const events = { native: [], hidden: [], changes: 0 };
  const controller = mascot.createMascotController({
    window: win,
    primaryWorkArea: () => FULL_HD,
    onNativeMode: (mode) => events.native.push(mode),
    onHiddenChange: (hidden) => events.hidden.push(hidden),
    onChange: () => void (events.changes += 1),
    ...extra,
  });
  win.calls.length = 0;
  return { controller, win, events };
}
const visibilityCalls = (win) => win.calls.map(([name]) => name).filter((name) => ["show", "showInactive", "hide", "focus"].includes(name));

// --- the resting island has no visible bar ---------------------------------------------------------------------

test("with nothing going on the island is only an invisible strip at the top edge, a few pixels tall", () => {
  assert.ok(mascot.MODE_SIZES.collapsed.height >= 2 && mascot.MODE_SIZES.collapsed.height <= 4, "a 2 to 4 px hot strip");
  assert.ok(mascot.MODE_SIZES.collapsed.width >= 120);
  const bounds = mascot.islandBounds("collapsed", FULL_HD);
  assert.equal(bounds.y, 0);
});

test("the page knows the same sizes as the shell, so it can morph between them", () => {
  assert.deepEqual(windowMode.ISLAND_SIZES, mascot.MODE_SIZES);
});

test("growing from one size to another starts the island's shape at the old size, shrinking has nothing to grow", () => {
  const grow = windowMode.islandMorph("collapsed", "compact");
  assert.equal(grow.grows, true);
  const { width: cw, height: ch } = windowMode.ISLAND_SIZES.collapsed;
  const { width: pw, height: ph } = windowMode.ISLAND_SIZES.compact;
  assert.ok(Math.abs(grow.insetX - ((1 - cw / pw) / 2) * 100) < 0.01);
  assert.ok(Math.abs(grow.insetY - (1 - ch / ph) * 100) < 0.01);
  const shrink = windowMode.islandMorph("chat", "collapsed");
  assert.deepEqual(shrink, { grows: false, insetX: 0, insetY: 0 });
  assert.deepEqual(windowMode.islandMorph("menu", "menu"), { grows: false, insetX: 0, insetY: 0 });
  for (const from of windowMode.WINDOW_MODES) for (const to of windowMode.WINDOW_MODES) {
    const morph = windowMode.islandMorph(from, to);
    assert.ok(morph.insetX >= 0 && morph.insetX < 100 && morph.insetY >= 0 && morph.insetY < 100, `${from} to ${to}`);
  }
});

// --- hiding it -------------------------------------------------------------------------------------------------

test("a controller that starts hidden keeps the window off screen, also when the app asks for a quiet show", () => {
  const { controller, win } = controllerFor({ hidden: true });
  assert.equal(controller.state().hidden, true);
  controller.showInactive();
  assert.deepEqual(visibilityCalls(win), [], "a hidden island stays hidden at startup");
  assert.equal(win.state.visible, false);
});

test("hiding closes whatever is open, takes the island off screen, tells the page and records the choice", () => {
  const { controller, win, events } = controllerFor();
  controller.showInactive();
  controller.setMode("chat", "page");
  win.calls.length = 0;
  controller.setHidden(true);
  assert.equal(controller.state().mode, "collapsed");
  assert.equal(controller.state().hidden, true);
  assert.deepEqual(visibilityCalls(win), ["hide"]);
  assert.deepEqual(events.hidden, [true]);
  assert.deepEqual(events.native, ["collapsed"], "the page follows");
  assert.ok(events.changes >= 1);
  controller.setHidden(true);
  assert.deepEqual(events.hidden, [true], "hiding twice records nothing new");
});

test("showing it again brings back the strip without taking focus, and records the choice", () => {
  const { controller, win, events } = controllerFor({ hidden: true });
  controller.setHidden(false);
  assert.equal(controller.state().hidden, false);
  assert.deepEqual(visibilityCalls(win), ["showInactive"]);
  assert.deepEqual(events.hidden, [false]);
  assert.deepEqual(events.native, ["collapsed"]);
});

test("a request that needs the user brings the island up without focus even while hidden, and it hides again afterwards", () => {
  const { controller, win } = controllerFor({ hidden: true });
  controller.attention();
  assert.deepEqual(visibilityCalls(win), ["showInactive"], "never show() or focus()");
  controller.setMode("approval", "page");
  assert.equal(win.state.visible, true);
  win.calls.length = 0;
  controller.setMode("collapsed", "page");
  assert.deepEqual(visibilityCalls(win), ["hide"], "the card is gone, so the island goes away again");
  assert.equal(controller.state().hidden, true, "the user's choice is untouched");
});

test("a hidden island shows for any card the page asks for, never for the resting strip", () => {
  const { controller, win } = controllerFor({ hidden: true });
  controller.setMode("compact", "page");
  assert.deepEqual(visibilityCalls(win), ["showInactive"], "the page asks for the pill only while the screen is shared");
  controller.setMode("collapsed", "page");
  assert.equal(win.state.visible, false);
  win.calls.length = 0;
  controller.setMode("menu", "page");
  assert.deepEqual(visibilityCalls(win), ["showInactive"]);
});

test("the shortcut brings a hidden island back with the chat, and closing the chat puts it away again", () => {
  const { controller, win, events } = controllerFor({ hidden: true });
  controller.toggleChat();
  assert.equal(controller.state().mode, "chat");
  assert.equal(win.state.visible, true);
  assert.ok(win.calls.some(([name]) => name === "focus"), "the user asked for it, so it takes focus");
  assert.deepEqual(events.native, ["chat"]);
  controller.toggleChat();
  assert.equal(controller.state().mode, "collapsed");
  assert.equal(win.state.visible, false, "hidden by the user stays hidden once the chat is closed");
  assert.equal(controller.state().hidden, true);
});

test("summoning Dot from the tray icon opens the menu, because the resting island shows nothing", () => {
  const { controller, win } = controllerFor();
  controller.show();
  assert.equal(controller.state().mode, "menu");
  assert.deepEqual(visibilityCalls(win), ["show", "focus"]);
  const hiddenOne = controllerFor({ hidden: true });
  hiddenOne.controller.show();
  assert.equal(hiddenOne.controller.state().mode, "menu");
  assert.equal(hiddenOne.win.state.visible, true);
  assert.equal(hiddenOne.controller.state().hidden, true);
});

test("the page learns whether the user hid Dot, and nothing else about the controller changes", async () => {
  assert.deepEqual(windowMode.sanitizeWindowState({ mode: "collapsed", anchor: { horizontal: "left", vertical: "top" }, hidden: true }), {
    mode: "collapsed",
    anchor: { horizontal: "left", vertical: "top" },
    hidden: true,
  });
  for (const hidden of [false, undefined, "yes", 1, null]) {
    const state = windowMode.sanitizeWindowState({ mode: "chat", anchor: { horizontal: "left", vertical: "top" }, hidden });
    assert.equal("hidden" in state, false, `${hidden}`);
  }
  const { controller } = controllerFor({ hidden: true });
  const target = {};
  mascot.installWindowController(target, controller);
  const installed = Reflect.get(target, Symbol.for(mascot.WINDOW_CONTROLLER_KEY));
  assert.deepEqual(Object.keys(installed).sort(), ["moveTo", "setMode", "showInactive", "state"]);
  Reflect.set(globalThis, windowMode.WINDOW_CONTROLLER_KEY, installed);
  const response = await windowRoute.GET(new Request("http://127.0.0.1:3013/api/window", { headers: { host: "127.0.0.1:3013" } }));
  assert.deepEqual(await response.json(), { available: true, mode: "collapsed", anchor: { horizontal: "left", vertical: "top" }, hidden: true });
  Reflect.deleteProperty(globalThis, windowMode.WINDOW_CONTROLLER_KEY);
});

// --- what the page rests as ------------------------------------------------------------------------------------

test("hidden by the user the island rests as nothing, except that a shared screen is never hidden from you", () => {
  const quiet = { active: false, hovered: false };
  assert.equal(island.restingMode({ ...quiet, hidden: false }), "collapsed");
  assert.equal(island.restingMode({ active: true, hovered: true, hidden: true, sharing: false }), "collapsed", "an agent working or the pointer do not bring it back");
  assert.equal(island.restingMode({ active: true, hovered: false, hidden: true, sharing: true }), "compact", "the capture indicator always shows");
  assert.equal(island.restingMode({ active: true, hovered: false, hidden: false, sharing: true }), "compact");
});

test("the page reads the user's choice and keeps the capture indicator on screen", async () => {
  const page = await read("app/mascot/mascot.tsx");
  assert.match(page, /restingMode\(\{[^}]*hidden/);
  assert.match(page, /sharing/);
  const hook = await read("app/mascot/use-window.ts");
  assert.match(hook, /hidden/);
});

// --- the saved choice ------------------------------------------------------------------------------------------

test("hiding Dot is saved with the other choices and read back, and a file without it means visible", async () => {
  const file = join(scratch, "hidden.json");
  const saved = await preferences.savePreferences(file, { startAtLogin: true, captureEnabled: false, islandHidden: true });
  assert.deepEqual(saved, { startAtLogin: true, captureEnabled: false, islandHidden: true });
  assert.deepEqual(await preferences.loadPreferences(file), saved);
  const store = preferences.createPreferenceStore(file, saved);
  assert.deepEqual(await store.update({ captureEnabled: true }), { startAtLogin: true, captureEnabled: true, islandHidden: true }, "other settings keep it");
  assert.deepEqual(await store.update({ islandHidden: false }), { startAtLogin: true, captureEnabled: true });
  assert.equal("islandHidden" in JSON.parse(await readFile(file, "utf8")), false, "visible is never written");
  await writeFile(file, JSON.stringify({ startAtLogin: false, captureEnabled: true }));
  assert.deepEqual(await preferences.loadPreferences(file), { startAtLogin: false, captureEnabled: true });
});

test("a saved choice of the wrong shape makes the file unknown, like any other corrupt value", async () => {
  for (const [index, hidden] of ["yes", 1, null, [], {}].entries()) {
    const file = join(scratch, `bad-hidden-${index}.json`);
    const content = JSON.stringify({ startAtLogin: true, captureEnabled: false, islandHidden: hidden });
    await writeFile(file, content);
    assert.equal(await preferences.loadPreferences(file), undefined, content);
    assert.equal(await readFile(file, "utf8"), content, "left untouched");
  }
});

test("the position and the hidden choice are independent", async () => {
  const file = join(scratch, "both.json");
  const saved = await preferences.savePreferences(file, { startAtLogin: true, captureEnabled: false, mascotPosition: { x: 1, y: 2 }, islandHidden: true });
  assert.deepEqual(saved, { startAtLogin: true, captureEnabled: false, mascotPosition: { x: 1, y: 2 }, islandHidden: true });
  assert.deepEqual(await preferences.loadPreferences(file), saved);
});

// --- the shell -------------------------------------------------------------------------------------------------

test("the tray flips between Ocultar Dot and Mostrar Dot and keeps the choice; the shortcut stays the only global one", async () => {
  const main = await read("desktop/main.mts");
  assert.match(main, /"Ocultar Dot"/);
  assert.match(main, /"Mostrar Dot"/);
  assert.match(main, /islandHidden/);
  assert.match(main, /onHiddenChange/);
  assert.equal((main.match(/globalShortcut\.register/g) ?? []).length, 1);
  assert.match(main, /tray\.on\("click", showWindow\)/);
  for (const kept of ['"Let Dot see my screen"', '"Start with Windows"', 'label: "Quit"']) assert.ok(main.includes(kept), kept);
});

// --- the page keeps approvals, the capture indicator and the transitions ---------------------------------------

test("hiding Dot while a request waits raises its card again, and a new request still raises it", async () => {
  const page = await read("app/mascot/mascot.tsx");
  assert.match(page, /if \(!hidden \|\| !pendingId \|\| mode === "approval"\) return;[\s\S]{0,200}setMode\("approval"\)/);
  assert.match(page, /\[pendingId\]/, "a new request raises its card as before");
  assert.match(page, /agent\.ack\(pendingId\)/, "the card is still acknowledged once it is on screen");
});

test("the island never rests as nothing while the screen is shared, hidden by the user or not", () => {
  for (const hidden of [true, false]) {
    for (const hovered of [true, false]) {
      assert.equal(island.restingMode({ active: true, hovered, hidden, sharing: true }), "compact", `hidden=${hidden} hovered=${hovered}`);
    }
  }
});

test("the island's shape unfolds when it grows and the content fades out before it shrinks, and reduced motion only fades", async () => {
  const css = await read("app/globals.css");
  assert.match(css, /@keyframes island-grow-a/);
  assert.match(css, /@keyframes island-grow-b/);
  assert.match(css, /--morph-x/);
  assert.match(css, /data-leaving="true"/);
  const grow = css.split("@media").filter((block) => /animation:\s*island-grow-/.test(block));
  assert.ok(grow.length > 0);
  for (const block of grow) assert.match(block, /^\s*\(prefers-reduced-motion: no-preference\)/, "the shape morph is skipped under reduced motion");
  assert.match(css, /island-fade 140ms/, "reduced motion still fades the content in");
  assert.doesNotMatch(css, /transition:\s*all/);
  assert.doesNotMatch(css, /scale\(0\)/);
  const hook = await read("app/mascot/use-window.ts");
  assert.match(hook, /islandShrinks\(/);
  assert.match(hook, /islandMorph\(/);
  const page = await read("app/mascot/mascot.tsx");
  assert.match(page, /data-morph=\{morph\.flip\}/);
  assert.match(page, /data-leaving=\{leaving\}/);
});

test("closing is never delayed beyond the fade, and a newer request wins the window", () => {
  assert.equal(windowMode.islandShrinks("chat", "collapsed"), true);
  assert.equal(windowMode.islandShrinks("menu", "compact"), true);
  assert.equal(windowMode.islandShrinks("collapsed", "compact"), false);
  assert.equal(windowMode.islandShrinks("menu", "menu"), false);
});

test("the resting strip is a real target for the pointer but draws nothing visible", async () => {
  const css = await read("app/globals.css");
  const rule = css.match(/\.island\[data-mode="collapsed"\] \{[^}]*\}/)?.[0] ?? "";
  assert.match(rule, /rgba\(0, 0, 0, 0\.0\d*\)/);
  assert.doesNotMatch(rule, /background:\s*#/);
});
