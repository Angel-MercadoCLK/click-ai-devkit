// Whether the chat panel treats screen capture as active, decided from the periodic status polls. Kept pure so it
// can be tested without a browser. The server checks that capture is on and not paused again when a message is
// sent, so being slow to notice a stop here can never send a screen while capture is paused.

/** Consecutive failed polls before capture is treated as inactive. */
export const CAPTURE_FAILURE_LIMIT = 3;

export type CapturePollState = { active: boolean; failures: number };
export type CapturePollResult = { ok: true; active: boolean } | { ok: false };

export const INITIAL_CAPTURE_POLL: CapturePollState = { active: false, failures: 0 };

/** A readable status decides at once; a failed read changes nothing until it has failed several times in a row. */
export function applyCapturePoll(state: CapturePollState, result: CapturePollResult): CapturePollState {
  if (result.ok) return { active: result.active, failures: 0 };
  const failures = Math.min(state.failures + 1, CAPTURE_FAILURE_LIMIT);
  return { active: failures >= CAPTURE_FAILURE_LIMIT ? false : state.active, failures };
}

/** Reads a successful status response body. Active only when the app can capture, it is on and it is not paused. */
export function readCapturePoll(value: unknown): CapturePollResult {
  const { available, enabled, paused } = (value && typeof value === "object" ? value : {}) as Record<string, unknown>;
  return { ok: true, active: available === true && enabled === true && paused === false };
}

// What the mascot shows around itself. The same tolerance as the chat panel: one or two failed polls change nothing.

export type CaptureView = { available: boolean; enabled: boolean; paused: boolean };
export type CaptureIndicator = "unavailable" | "off" | "live" | "paused";
export type CaptureViewPollState = { view: CaptureView | null; failures: number };
export type CaptureViewPollResult = { ok: true; view: CaptureView } | { ok: false };

export const INITIAL_CAPTURE_VIEW_POLL: CaptureViewPollState = { view: null, failures: 0 };

/** Reads the three flags from a successful status body; anything malformed is not a status. */
export function readCaptureView(value: unknown): CaptureView | undefined {
  if (!value || typeof value !== "object") return undefined;
  const { available, enabled, paused } = value as Record<string, unknown>;
  if (typeof available !== "boolean" || typeof enabled !== "boolean" || typeof paused !== "boolean") return undefined;
  return { available, enabled, paused };
}

export function captureIndicator(view: CaptureView | null): CaptureIndicator {
  if (!view || !view.available) return "unavailable";
  if (!view.enabled) return "off";
  return view.paused ? "paused" : "live";
}

export function applyCaptureViewPoll(state: CaptureViewPollState, result: CaptureViewPollResult): CaptureViewPollState {
  if (result.ok) return { view: result.view, failures: 0 };
  const failures = Math.min(state.failures + 1, CAPTURE_FAILURE_LIMIT);
  return { view: failures >= CAPTURE_FAILURE_LIMIT ? null : state.view, failures };
}

// What the mascot shows when capture and "Voz" are both going on. Capture is what the user must always be able to see,
// so it keeps the ring, the visible text and the pause control; listening gets its own visual and its own cancel.

export type MascotIndicatorView = {
  /** Always the capture state; listening never replaces it. */
  ring: CaptureIndicator;
  sharing: boolean;
  /** Visible words while capture is on, or null when nothing is shared. */
  sharingText: string | null;
  pause: { action: "pause" | "resume"; name: "Pausar" | "Reanudar"; glyph: "pause" | "play" } | null;
  listening: boolean;
  cancel: boolean;
  /** Accessible name of the mascot: capture first, then listening. */
  label: string;
};

export function mascotIndicator(indicator: CaptureIndicator, listening: boolean): MascotIndicatorView {
  const sharing = indicator === "live" || indicator === "paused";
  const sharingText = indicator === "live" ? "Viendo tu pantalla" : indicator === "paused" ? "Pantalla en pausa" : null;
  const pause = indicator === "live"
    ? ({ action: "pause", name: "Pausar", glyph: "pause" } as const)
    : indicator === "paused"
      ? ({ action: "resume", name: "Reanudar", glyph: "play" } as const)
      : null;
  const label = [sharingText, listening ? "Escuchando…" : null].filter(Boolean).join(". ") || "Dot";
  return { ring: indicator, sharing, sharingText, pause, listening, cancel: listening, label };
}
