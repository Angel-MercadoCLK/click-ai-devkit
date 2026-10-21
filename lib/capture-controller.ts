// Screen capture bridge for the web side.
//
// Capture itself runs in the Electron main process. Next runs in that same process, so main publishes a small
// controller on `globalThis[Symbol.for("click-dot.capture-controller")]` (the same pattern as the storage
// protector). Everything read from it here is validated. Status leaves only as counts and times; the one image
// that can leave is the already-redacted one from `prepareScreen`, never a raw frame.

import { SCREEN_IMAGE_MAX_BYTES, type ScreenImage } from "./screen-previews.ts";

export type CaptureStatus = { enabled: boolean; paused: boolean; frames: number; lastCapturedAt: number | null };
export type CaptureAction = "enable" | "disable" | "pause" | "resume";
export type CaptureController = {
  status(): unknown;
  apply(action: CaptureAction): Promise<unknown>;
  /** Present only in the desktop app. Resolves with an already-redacted image; see `prepareScreen` below. */
  prepareScreen?(): Promise<unknown>;
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

export type PreparedScreen = ScreenImage & { hiddenRegions: number; capturedAt: number };
export type ScreenPrepareCode =
  | "unavailable"
  | "no-frame"
  | "stale-frame"
  | "capture-stopped"
  | "ocr-unavailable"
  | "ocr-failed"
  | "patterns-unavailable"
  | "redaction-failed"
  | "too-large"
  | "invalid-result";

/** Fixed wording for the page. The desktop app's own error text never crosses this boundary. */
export const SCREEN_PREPARE_MESSAGES: Record<ScreenPrepareCode, string> = {
  unavailable: "Screen capture is available in the desktop app.",
  "no-frame": "Dot has no screen image yet. Wait a few seconds and try again.",
  "stale-frame": "The latest screen image is out of date. Try again in a few seconds.",
  "capture-stopped": "Screen capture is paused or off.",
  "ocr-unavailable": "This computer has no text recognition language installed, so Dot cannot hide personal data. Nothing was attached.",
  "ocr-failed": "Dot could not check the screen for personal data. Nothing was attached.",
  "patterns-unavailable": "Dot could not load its personal-data rules. Nothing was attached.",
  "redaction-failed": "Dot could not hide personal data in the screen image. Nothing was attached.",
  "too-large": "The screen image is too large to attach.",
  "invalid-result": "Dot could not prepare the screen image. Nothing was attached.",
};

export class ScreenPrepareError extends Error {
  readonly code: ScreenPrepareCode;

  constructor(code: ScreenPrepareCode) {
    super(SCREEN_PREPARE_MESSAGES[code]);
    this.code = code;
  }
}

const PNG_MAGIC = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a];
const hasPrefix = (bytes: Uint8Array, prefix: number[]) => bytes.length > prefix.length && prefix.every((value, index) => bytes[index] === value);

/** Accepts only a PNG or JPEG within the image budget, with its declared type matching its bytes. */
export function sanitizePreparedScreen(value: unknown): PreparedScreen | undefined {
  if (!value || typeof value !== "object") return undefined;
  const { image, mime, hiddenRegions, capturedAt } = value as Record<string, unknown>;
  if (!ArrayBuffer.isView(image) || image.byteLength < 16 || image.byteLength > SCREEN_IMAGE_MAX_BYTES) return undefined;
  const bytes = new Uint8Array(image.buffer, image.byteOffset, image.byteLength);
  if (mime === "image/png" ? !hasPrefix(bytes, PNG_MAGIC) : mime === "image/jpeg" ? !hasPrefix(bytes, [0xff, 0xd8, 0xff]) : true) return undefined;
  if (typeof hiddenRegions !== "number" || !Number.isInteger(hiddenRegions) || hiddenRegions < 0 || hiddenRegions > 10_000) return undefined;
  if (typeof capturedAt !== "number" || !Number.isFinite(capturedAt)) return undefined;
  return { image: new Uint8Array(bytes), mime: mime as PreparedScreen["mime"], hiddenRegions, capturedAt };
}

/** Asks the desktop app for the redacted screen. Every failure is mapped to a known code; no raw error crosses. */
export async function prepareScreen(controller: CaptureController): Promise<PreparedScreen> {
  if (typeof controller.prepareScreen !== "function") throw new ScreenPrepareError("unavailable");
  let result: unknown;
  try {
    result = await controller.prepareScreen();
  } catch (error) {
    const code = (error as { code?: unknown } | null)?.code;
    const known = Object.keys(SCREEN_PREPARE_MESSAGES).find((candidate) => candidate === code && candidate !== "unavailable" && candidate !== "invalid-result");
    throw new ScreenPrepareError((known as ScreenPrepareCode | undefined) ?? "invalid-result");
  }
  const prepared = sanitizePreparedScreen(result);
  if (!prepared) throw new ScreenPrepareError("invalid-result");
  return prepared;
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
