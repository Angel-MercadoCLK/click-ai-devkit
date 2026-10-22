import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { test } from "node:test";

process.env.PORT = "3012";

const cursor = await import("../desktop/cursor.mts");
const feedLib = await import("../lib/cursor-feed.ts");

const read = (path) => readFile(new URL(`../${path}`, import.meta.url), "utf8");

function fakeTimers() {
  const timers = new Map();
  let next = 1;
  return {
    timers,
    setTimer(callback, ms) {
      timers.set(next, { callback, ms });
      return next++;
    },
    clearTimer(id) {
      timers.delete(id);
    },
    tick() {
      for (const { callback } of [...timers.values()]) callback();
    },
  };
}

function feedFor({ active = true, point = { x: 960, y: 500 }, bounds = { x: 710, y: 0, width: 500, height: 44 } } = {}) {
  const timers = fakeTimers();
  const sent = [];
  const live = { active, point, bounds };
  const feed = cursor.createCursorFeed({
    getCursor: () => live.point,
    getBounds: () => live.bounds,
    isActive: () => live.active,
    send: (script) => sent.push(script),
    setTimer: timers.setTimer,
    clearTimer: timers.clearTimer,
  });
  return { feed, timers, sent, live };
}

// --- the position the page gets --------------------------------------------------------------------------------

test("the page gets the cursor relative to the island window, in whole pixels", () => {
  assert.deepEqual(cursor.relativeCursor({ x: 960.4, y: 500.6 }, { x: 710, y: 0, width: 500, height: 44 }), { x: 250, y: 501 });
  assert.deepEqual(cursor.relativeCursor({ x: 0, y: 0 }, { x: 710, y: 0, width: 500, height: 44 }), { x: -710, y: 0 });
});

test("the script the shell runs in the page is built only from two bounded integers", () => {
  const script = cursor.cursorScript({ x: 250, y: -12 });
  assert.equal(script, 'window.dispatchEvent(new CustomEvent("click-dot:cursor",{detail:{x:250,y:-12}}))');
  for (const bad of [{ x: Number.NaN, y: 0 }, { x: 0, y: Infinity }, { x: 1e9, y: 0 }, { x: "1); alert(1", y: 0 }, { x: 1, y: "2" }, null, undefined]) {
    assert.equal(cursor.cursorScript(bad), undefined, JSON.stringify(bad));
  }
  // Even a non-integer number is rounded before it is written, so nothing but digits and a sign reaches the script.
  assert.match(cursor.cursorScript({ x: 1.5, y: 2.4 }), /\{x:2,y:2\}\}\)\)$/);
});

test("the event name is the one the page listens to", async () => {
  assert.equal(cursor.CURSOR_EVENT, feedLib.CURSOR_EVENT);
  assert.equal(feedLib.CURSOR_EVENT, "click-dot:cursor");
  const store = await read("app/mascot/cursor-store.ts");
  assert.match(store, /CURSOR_EVENT/);
});

// --- polling ---------------------------------------------------------------------------------------------------

test("it polls about 30 times a second, and only while the island shows something to look at", () => {
  assert.ok(cursor.CURSOR_POLL_MS >= 30 && cursor.CURSOR_POLL_MS <= 40);
  const inactive = feedFor({ active: false });
  inactive.feed.sync();
  assert.equal(inactive.timers.timers.size, 0, "nothing runs while the island is hidden or collapsed");

  const { feed, timers, live } = feedFor();
  feed.sync();
  assert.equal(timers.timers.size, 1);
  assert.equal([...timers.timers.values()][0].ms, cursor.CURSOR_POLL_MS);
  feed.sync();
  assert.equal(timers.timers.size, 1, "syncing again never starts a second loop");
  live.active = false;
  feed.sync();
  assert.equal(timers.timers.size, 0, "the loop stops as soon as there is nothing to look at");
});

test("a tick that finds the island no longer active stops the loop by itself", () => {
  const { feed, timers, live, sent } = feedFor();
  feed.sync();
  live.active = false;
  timers.tick();
  assert.equal(timers.timers.size, 0);
  assert.deepEqual(sent, []);
});

test("it sends the position only when it changed", () => {
  const { feed, timers, sent, live } = feedFor();
  feed.sync();
  timers.tick();
  timers.tick();
  assert.equal(sent.length, 1);
  live.point = { x: 970, y: 500 };
  timers.tick();
  assert.equal(sent.length, 2);
  live.bounds = { x: 600, y: 0, width: 640, height: 300 };
  timers.tick();
  assert.equal(sent.length, 3, "the island moved under a still cursor");
});

test("starting again sends the position at once, even if it did not change", () => {
  const { feed, timers, sent, live } = feedFor();
  feed.sync();
  timers.tick();
  live.active = false;
  feed.sync();
  live.active = true;
  feed.sync();
  timers.tick();
  assert.equal(sent.length, 2);
});

test("a cursor that cannot be read or a failing send never stops the shell", () => {
  const timers = fakeTimers();
  let calls = 0;
  const feed = cursor.createCursorFeed({
    getCursor: () => {
      calls += 1;
      if (calls === 1) throw new Error("no display");
      return { x: 1, y: 1 };
    },
    getBounds: () => ({ x: 0, y: 0, width: 10, height: 10 }),
    isActive: () => true,
    send: () => {
      throw new Error("page is gone");
    },
    setTimer: timers.setTimer,
    clearTimer: timers.clearTimer,
  });
  feed.sync();
  assert.doesNotThrow(() => timers.tick());
  assert.doesNotThrow(() => timers.tick());
  feed.stop();
  assert.equal(timers.timers.size, 0);
});

// --- wiring ----------------------------------------------------------------------------------------------------

test("the shell feeds the page from the main process and polls nothing while the island is collapsed or hidden", async () => {
  const main = await read("desktop/main.mts");
  assert.match(main, /createCursorFeed\(/);
  assert.match(main, /getCursorScreenPoint/);
  assert.match(main, /mode !== "collapsed"/);
  assert.doesNotMatch(main, /executeJavaScript\(`/, "no script is built from a template string in the shell");
  const store = await read("app/mascot/cursor-store.ts");
  assert.doesNotMatch(store, /fetch\(/, "no HTTP polling for the cursor");
});
