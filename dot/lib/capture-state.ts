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
