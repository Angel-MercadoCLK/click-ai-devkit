"use client";

import { useEffect, useRef, useState } from "react";

type CaptureStatus = { available: boolean; enabled: boolean; paused: boolean; frames: number; lastCapturedAt: number | null };
type Action = "enable" | "disable" | "pause" | "resume";

const POLL_MS = 4000;

function readStatus(value: unknown): CaptureStatus | undefined {
  if (!value || typeof value !== "object") return undefined;
  const { available, enabled, paused, frames, lastCapturedAt } = value as Partial<CaptureStatus>;
  if (typeof available !== "boolean" || typeof enabled !== "boolean" || typeof paused !== "boolean") return undefined;
  return { available, enabled, paused, frames: typeof frames === "number" ? frames : 0, lastCapturedAt: typeof lastCapturedAt === "number" ? lastCapturedAt : null };
}

/** Screen capture control. It only ever shows counts and state; the desktop app never sends pixels to the page. */
export function CapturePanel() {
  const [status, setStatus] = useState<CaptureStatus | null>(null);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const mounted = useRef(true);

  async function refresh(signal?: AbortSignal) {
    try {
      const response = await fetch("/api/capture/status", { cache: "no-store", signal });
      if (!response.ok) throw new Error();
      const next = readStatus(await response.json());
      if (next && mounted.current) setStatus(next);
    } catch {
      // Keep the last known state; the next poll tries again.
    }
  }

  useEffect(() => {
    mounted.current = true;
    const controller = new AbortController();
    void refresh(controller.signal);
    const timer = window.setInterval(() => void refresh(controller.signal), POLL_MS);
    return () => {
      mounted.current = false;
      controller.abort();
      window.clearInterval(timer);
    };
  }, []);

  async function act(action: Action) {
    setBusy(true);
    setMessage("");
    try {
      const response = await fetch("/api/capture", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ action }),
      });
      if (!response.ok) throw new Error();
      const next = readStatus(await response.json());
      if (next) setStatus(next);
    } catch {
      setMessage("Screen capture could not be changed. Check the desktop app and try again.");
    } finally {
      // The tray can change the state too, so always show what the app really reports.
      await refresh();
      if (mounted.current) setBusy(false);
    }
  }

  if (status === null) return null;
  if (!status.available) {
    return <p className="capture-note">Screen capture is available in the desktop app.</p>;
  }

  const watching = status.enabled && !status.paused;
  return (
    <section className={`capture-panel${watching ? " is-live" : ""}`} aria-labelledby="capture-title">
      <div className="capture-copy">
        <p className="panel-label" id="capture-title">Screen capture</p>
        {status.enabled ? (
          <p className="capture-state" role="status" aria-live="polite">
            <span className={`capture-dot${watching ? " is-live" : ""}`} aria-hidden="true" />
            {watching ? "Dot can see your screen" : "Screen capture is paused"}
          </p>
        ) : (
          <p className="panel-note">Dot sees nothing until you allow it. You can pause or turn it off at any time.</p>
        )}
        {status.enabled && (
          <p className="panel-note">Dot looks at your main display every few seconds and keeps only the last minute in memory. Nothing is saved to disk.</p>
        )}
      </div>
      <div className="capture-actions">
        {status.enabled ? (
          <>
            <button className="action-button secondary" type="button" onClick={() => act(status.paused ? "resume" : "pause")} disabled={busy}>
              {status.paused ? "Resume" : "Pause"}
            </button>
            <button className="action-button secondary" type="button" onClick={() => act("disable")} disabled={busy}>
              Turn off
            </button>
          </>
        ) : (
          <button className="action-button" type="button" onClick={() => act("enable")} disabled={busy}>
            Let Dot see your screen
          </button>
        )}
      </div>
      {message && <p className="feedback" role="status">{message}</p>}
    </section>
  );
}
