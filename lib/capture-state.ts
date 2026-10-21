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
