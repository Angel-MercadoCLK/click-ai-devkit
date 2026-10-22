"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { islandMorph, islandShrinks, UNAVAILABLE_WINDOW_STATE, type WindowAnchor, type WindowMode, WINDOW_MODES } from "../../lib/window-mode.ts";

type Reported = { mode: WindowMode; anchor: WindowAnchor; hidden: boolean };

/** How long the content fades out before the window gets smaller, so closing a card does not pop. */
const LEAVE_MS = 130;

function readReported(value: unknown): Reported | undefined {
  if (!value || typeof value !== "object") return undefined;
  const { available, mode, anchor, hidden } = value as Record<string, unknown>;
  // Without the desktop shell the route reports a placeholder; it must not override the page's own state.
  if (available !== true) return undefined;
  const known = WINDOW_MODES.find((candidate) => candidate === mode);
  const { horizontal, vertical } = (anchor && typeof anchor === "object" ? anchor : {}) as Record<string, unknown>;
  if (!known || (horizontal !== "left" && horizontal !== "right") || (vertical !== "top" && vertical !== "bottom")) return undefined;
  return { mode: known, anchor: { horizontal, vertical }, hidden: hidden === true };
}

/** How the island's shape unfolds into a bigger window: which of the two identical animations to run and where it starts. */
type Morph = { flip: "a" | "b"; x: number; y: number; grows: boolean };

/**
 * The island window's mode, owned by the desktop shell. The page asks for changes through `/api/window` and follows the
 * shell when the shortcut or tray changes it. Outside the desktop app the mode is only local, so the page still works in a browser.
 * `hidden` is the user's choice from the tray ("Ocultar Dot"). `morph` and `leaving` drive the open and close transitions:
 * the window resizes at once, so the page unfolds the island's shape when it grows and fades its content out before it shrinks.
 */
export function useMascotWindow() {
  const [state, setState] = useState<Reported>({ ...UNAVAILABLE_WINDOW_STATE, hidden: false });
  const [morph, setMorph] = useState<Morph>({ flip: "a", x: 0, y: 0, grows: false });
  const [leaving, setLeaving] = useState(false);
  const alive = useRef(true);
  const modeRef = useRef(state.mode);
  modeRef.current = state.mode;
  const request = useRef(0);

  const load = useCallback(async () => {
    try {
      const response = await fetch("/api/window", { cache: "no-store" });
      if (!response.ok) return;
      const next = readReported(await response.json());
      if (next && alive.current) setState(next);
    } catch {
      // Keep what is shown; the next change or focus reads it again.
    }
  }, []);

  const post = useCallback(async (body: unknown) => {
    try {
      const response = await fetch("/api/window", { method: "POST", cache: "no-store", headers: { "content-type": "application/json" }, body: JSON.stringify(body) });
      if (!response.ok) return;
      const next = readReported(await response.json());
      if (next && alive.current) setState(next);
    } catch {
      // The desktop shell is not there (a plain browser): the local state below is all there is.
    }
  }, []);

  const setMode = useCallback(
    async (mode: WindowMode) => {
      const token = ++request.current;
      if (islandShrinks(modeRef.current, mode)) {
        setLeaving(true);
        await new Promise((resolve) => window.setTimeout(resolve, LEAVE_MS));
        // A newer request owns the window now.
        if (token !== request.current || !alive.current) return;
        setLeaving(false);
      } else setLeaving(false);
      setState((current) => ({ ...current, mode }));
      await post({ mode });
    },
    [post],
  );

  // A change of mode, from the page or from the shell, unfolds the shape when the window grew.
  const shownMode = useRef(state.mode);
  useEffect(() => {
    const from = shownMode.current;
    if (from === state.mode) return;
    shownMode.current = state.mode;
    const next = islandMorph(from, state.mode);
    setMorph((current) => ({ flip: current.flip === "a" ? "b" : "a", x: next.insetX, y: next.insetY, grows: next.grows }));
  }, [state.mode]);

  useEffect(() => {
    alive.current = true;
    void load();
    // The shell announces mode changes it makes itself (the shortcut, the tray) with this event.
    window.addEventListener("click-dot:mode", load);
    window.addEventListener("focus", load);
    return () => {
      alive.current = false;
      window.removeEventListener("click-dot:mode", load);
      window.removeEventListener("focus", load);
    };
  }, [load]);

  return { mode: state.mode, anchor: state.anchor, hidden: state.hidden, morph, leaving, setMode };
}
