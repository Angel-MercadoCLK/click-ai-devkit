"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { UNAVAILABLE_WINDOW_STATE, type WindowAnchor, type WindowMode, WINDOW_MODES } from "../../lib/window-mode.ts";

type Reported = { mode: WindowMode; anchor: WindowAnchor };

function readReported(value: unknown): Reported | undefined {
  if (!value || typeof value !== "object") return undefined;
  const { available, mode, anchor } = value as Record<string, unknown>;
  // Without the desktop shell the route reports a placeholder; it must not override the page's own state.
  if (available !== true) return undefined;
  const known = WINDOW_MODES.find((candidate) => candidate === mode);
  const { horizontal, vertical } = (anchor && typeof anchor === "object" ? anchor : {}) as Record<string, unknown>;
  if (!known || (horizontal !== "left" && horizontal !== "right") || (vertical !== "top" && vertical !== "bottom")) return undefined;
  return { mode: known, anchor: { horizontal, vertical } };
}

/**
 * The mascot window's mode, owned by the desktop shell. The page asks for changes through `/api/window` and follows the
 * shell when the shortcut or tray changes it. Outside the desktop app the mode is only local, so the page still works in a browser.
 */
export function useMascotWindow() {
  const [state, setState] = useState<Reported>(UNAVAILABLE_WINDOW_STATE);
  const alive = useRef(true);

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
      setState((current) => ({ ...current, mode }));
      await post({ mode });
    },
    [post],
  );

  const moveTo = useCallback((x: number, y: number) => post({ move: { x: Math.round(x), y: Math.round(y) } }), [post]);

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

  return { mode: state.mode, anchor: state.anchor, setMode, moveTo };
}
