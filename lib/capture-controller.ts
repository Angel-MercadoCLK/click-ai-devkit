// Screen capture bridge for the web side.
//
// Capture itself runs in the Electron main process. Next runs in that same process, so main publishes a small
// controller on `globalThis[Symbol.for("click-dot.capture-controller")]` (the same pattern as the storage
// protector). Everything read from it here is validated, and only counts and times ever leave: no frame, and
// no way to reach one, is exposed to the page.

export type CaptureStatus = { enabled: boolean; paused: boolean; frames: number; lastCapturedAt: number | null };
export type CaptureAction = "enable" | "disable" | "pause" | "resume";
export type CaptureController = {
  status(): unknown;
  apply(action: CaptureAction): Promise<unknown>;
};

export const CAPTURE_CONTROLLER_KEY = Symbol.for("click-dot.capture-controller");
export const CAPTURE_ACTIONS: readonly CaptureAction[] = ["enable", "disable", "pause", "resume"];
export const CAPTURE_MAX_BODY_BYTES = 256;
export const UNAVAILABLE_STATUS: CaptureStatus = { enabled: false, paused: false, frames: 0, lastCapturedAt: null };

// More frames than any ring could hold means something other than a status is on the other end.
const MAX_REPORTED_FRAMES = 1000;

export function getCaptureController(): CaptureController | undefined {
  const candidate = (globalThis as Record<symbol, unknown>)[CAPTURE_CONTROLLER_KEY];
  if (!candidate || typeof candidate !== "object") return undefined;
  const { status, apply } = candidate as Partial<CaptureController>;
  if (typeof status !== "function" || typeof apply !== "function") return undefined;
  return candidate as CaptureController;
}

/** Copies only the known fields out of a status value; anything malformed is rejected whole. */
export function sanitizeCaptureStatus(value: unknown): CaptureStatus | undefined {
  if (!value || typeof value !== "object") return undefined;
  const { enabled, paused, frames, lastCapturedAt } = value as Record<string, unknown>;
  if (typeof enabled !== "boolean" || typeof paused !== "boolean") return undefined;
  if (typeof frames !== "number" || !Number.isInteger(frames) || frames < 0 || frames > MAX_REPORTED_FRAMES) return undefined;
  if (lastCapturedAt !== null && (typeof lastCapturedAt !== "number" || !Number.isFinite(lastCapturedAt))) return undefined;
  return { enabled, paused, frames, lastCapturedAt };
}

export function readCaptureStatus(controller: CaptureController): CaptureStatus | undefined {
  try {
    return sanitizeCaptureStatus(controller.status());
  } catch {
    return undefined;
  }
}

/** Accepts exactly `{ action }` with one of the four known actions. */
export function parseCaptureAction(value: unknown): CaptureAction | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  const keys = Object.keys(value);
  if (keys.length !== 1 || keys[0] !== "action") return undefined;
  const { action } = value as { action: unknown };
  return CAPTURE_ACTIONS.find((known) => known === action);
}

export class CaptureRequestError extends Error {
  readonly status: 400 | 413;

  constructor(status: 400 | 413, message: string) {
    super(message);
    this.status = status;
  }
}

/** Reads a tiny JSON body, refusing anything larger than CAPTURE_MAX_BODY_BYTES while streaming. */
export async function readCaptureAction(request: Request): Promise<CaptureAction> {
  const contentType = request.headers.get("content-type")?.split(";", 1)[0].trim().toLowerCase();
  if (contentType !== "application/json") throw new CaptureRequestError(400, "A JSON request is required.");
  const declared = Number(request.headers.get("content-length"));
  if (Number.isFinite(declared) && declared > CAPTURE_MAX_BODY_BYTES) throw new CaptureRequestError(413, "The request is too large.");
  if (!request.body) throw new CaptureRequestError(400, "The request body is missing.");
  const reader = request.body.getReader();
  const chunks: Uint8Array[] = [];
  let length = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      length += value.byteLength;
      if (length > CAPTURE_MAX_BODY_BYTES) {
        await reader.cancel();
        throw new CaptureRequestError(413, "The request is too large.");
      }
      chunks.push(value);
    }
  } finally {
    reader.releaseLock();
  }
  const bytes = new Uint8Array(length);
  let offset = 0;
  for (const chunk of chunks) {
    bytes.set(chunk, offset);
    offset += chunk.byteLength;
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(bytes));
  } catch {
    throw new CaptureRequestError(400, "The request body is invalid.");
  }
  const action = parseCaptureAction(parsed);
  if (!action) throw new CaptureRequestError(400, "Unknown screen capture request.");
  return action;
}
