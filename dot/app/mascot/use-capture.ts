"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { applyCaptureViewPoll, captureIndicator, INITIAL_CAPTURE_VIEW_POLL, readCaptureView, type CaptureViewPollResult } from "../../lib/capture-state.ts";

export type CaptureAction = "enable" | "disable" | "pause" | "resume";
const POLL_MS = 4000;

/** The capture state for the mascot's ring and buttons, from the same routes as the home page. */
export function useCaptureView() {
  const [poll, setPoll] = useState(INITIAL_CAPTURE_VIEW_POLL);
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const alive = useRef(true);

  const refresh = useCallback(async (signal?: AbortSignal) => {
    let result: CaptureViewPollResult = { ok: false };
    try {
      const response = await fetch("/api/capture/status", { cache: "no-store", signal });
      const view = response.ok ? readCaptureView(await response.json()) : undefined;
      if (view) result = { ok: true, view };
    } catch {
      // A failed read is not a "stopped"; see applyCaptureViewPoll.
    }
    if (signal?.aborted || !alive.current) return;
    setPoll((current) => applyCaptureViewPoll(current, result));
  }, []);

  useEffect(() => {
    alive.current = true;
    const controller = new AbortController();
    void refresh(controller.signal);
    const timer = window.setInterval(() => void refresh(controller.signal), POLL_MS);
    return () => {
      alive.current = false;
      controller.abort();
      window.clearInterval(timer);
    };
  }, [refresh]);

  const act = useCallback(
    async (action: CaptureAction) => {
      setBusy(true);
      setFailed(false);
      try {
        const response = await fetch("/api/capture", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ action }) });
        if (!response.ok) throw new Error();
      } catch {
        if (alive.current) setFailed(true);
      } finally {
        // The tray can change the state too, so always show what the app really reports.
        await refresh();
        if (alive.current) setBusy(false);
      }
    },
    [refresh],
  );

  return { view: poll.view, indicator: captureIndicator(poll.view), busy, failed, act };
}
