// One listener for the cursor events the desktop shell pushes, shared by every Orbi on the page. The listener exists only
// while something is subscribed, so a hidden or collapsed island keeps nothing running.

import { CURSOR_EVENT } from "../../lib/cursor-feed.ts";
import { readCursor, type Vec } from "./orbi-motion.ts";

type Listener = (cursor: Vec) => void;

const listeners = new Set<Listener>();
let latest: Vec | undefined;

function onEvent(event: Event) {
  const cursor = readCursor((event as CustomEvent).detail);
  if (!cursor) return;
  latest = cursor;
  for (const listener of listeners) listener(cursor);
}

/** The cursor in the window's own coordinates, as last pushed by the shell, or undefined outside the desktop app. */
export const latestCursor = () => latest;

export function subscribeCursor(listener: Listener): () => void {
  if (listeners.size === 0) window.addEventListener(CURSOR_EVENT, onEvent);
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
    if (listeners.size === 0) window.removeEventListener(CURSOR_EVENT, onEvent);
  };
}
