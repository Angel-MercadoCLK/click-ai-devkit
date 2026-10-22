// Where the cursor is, for the Lente character's eye. Only the main process can read the cursor anywhere on the screen,
// so it polls (about 30 Hz) and pushes the position to the island page as a DOM event. It polls only while the island
// shows something that could look at the cursor, and it stops by itself the moment that is no longer true.
//
// The page gets two bounded integers relative to its own window, written into a constant script: no text from the page or
// from anywhere else is ever interpolated, which keeps the same rule as the other `executeJavaScript` calls in the shell.

import type { Point, Rect } from "./mascot.mts";

/** The event the island page listens to. `lib/cursor-feed.ts` holds the page's copy of the name; a test keeps them equal. */
export const CURSOR_EVENT = "click-dot:cursor";
/** About 30 updates a second is smooth enough for a spring to follow and cheap enough to leave running. */
export const CURSOR_POLL_MS = 33;
/** Far beyond any real desktop; a larger number is not a cursor position. */
const LIMIT = 100_000;

/** The cursor in the island window's own coordinates, in whole pixels. */
export function relativeCursor(cursor: Point, bounds: Rect): Point {
  return { x: Math.round(cursor.x - bounds.x), y: Math.round(cursor.y - bounds.y) };
}

const isCoordinate = (value: unknown): value is number => typeof value === "number" && Number.isFinite(value) && Math.abs(value) <= LIMIT;

/** The script that hands one position to the page, or undefined when it is not two bounded numbers. */
export function cursorScript(point: unknown): string | undefined {
  if (!point || typeof point !== "object") return undefined;
  const { x, y } = point as Record<string, unknown>;
  if (!isCoordinate(x) || !isCoordinate(y)) return undefined;
  return `window.dispatchEvent(new CustomEvent("${CURSOR_EVENT}",{detail:{x:${Math.round(x)},y:${Math.round(y)}}}))`;
}

export type CursorFeedOptions = {
  /** `screen.getCursorScreenPoint()`. */
  getCursor: () => Point;
  /** The island window's current bounds. */
  getBounds: () => Rect;
  /** Whether the island is on screen and shows something other than the resting strip. */
  isActive: () => boolean;
  /** Runs the script in the island page. */
  send: (script: string) => void;
  setTimer?: (callback: () => void, ms: number) => unknown;
  clearTimer?: (id: unknown) => void;
};

export function createCursorFeed(options: CursorFeedOptions) {
  const setTimer = options.setTimer ?? ((callback, ms) => setInterval(callback, ms));
  const clearTimer = options.clearTimer ?? ((id) => clearInterval(id as NodeJS.Timeout));
  let timer: unknown;
  let running = false;
  let lastSent = "";

  function stop() {
    if (!running) return;
    running = false;
    clearTimer(timer);
    timer = undefined;
  }

  function tick() {
    if (!options.isActive()) {
      stop();
      return;
    }
    try {
      const script = cursorScript(relativeCursor(options.getCursor(), options.getBounds()));
      if (!script || script === lastSent) return;
      lastSent = script;
      options.send(script);
    } catch {
      // The cursor, the window or the page may be gone for a moment (a display change, a reload); the next tick tries again.
    }
  }

  /** Starts or stops the loop to match whether the island is active. Call it after anything that can change that. */
  function sync() {
    if (!options.isActive()) {
      stop();
      return;
    }
    if (running) return;
    running = true;
    lastSent = "";
    timer = setTimer(tick, CURSOR_POLL_MS);
  }

  return { sync, stop };
}

export type CursorFeed = ReturnType<typeof createCursorFeed>;
