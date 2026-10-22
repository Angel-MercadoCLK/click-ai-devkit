import type { CaptureFrame } from "./capture.mts";
import { OcrError, type OcrFn } from "./ocr.mts";
import { findSensitiveRegions, redactBitmap, type CompiledGuardRule, type Rect } from "./redaction.mts";

// Prepares the newest captured frame for chat: OCR, find personal data, pixelate it, hand back only the redacted
// image. The original frame stays in the capture ring (T2 retention rules) and is not kept anywhere else. Any
// failure throws a ScreenPrepareError with a fixed code; the caller shows its own fixed wording and sends nothing.

/** Same value as SCREEN_IMAGE_MAX_BYTES in lib/screen-previews.ts (a test keeps them equal). */
export const SCREEN_IMAGE_MAX_BYTES = 4 * 1024 * 1024;
/** A frame older than this is stale: capture may be failing, and the page would be showing old content. */
export const MAX_FRAME_AGE_MS = 2 * 60_000;

export type ScreenPrepareCode =
  | "no-frame"
  | "stale-frame"
  | "capture-stopped"
  | "ocr-unavailable"
  | "ocr-failed"
  | "patterns-unavailable"
  | "redaction-failed"
  | "too-large";

export class ScreenPrepareError extends Error {
  readonly code: ScreenPrepareCode;

  constructor(code: ScreenPrepareCode) {
    super(code);
    this.code = code;
  }
}

export type PreparedScreen = { image: Buffer; mime: "image/png" | "image/jpeg"; hiddenRegions: number; capturedAt: number };

/** Decodes a PNG, redacts the rectangles and encodes the result. Electron's nativeImage implements it in main. */
export type RedactImage = (png: Buffer, size: { width: number; height: number }, rects: Rect[]) => Promise<{ image: Buffer; mime: PreparedScreen["mime"] }>;

type ImageLike = {
  isEmpty(): boolean;
  getSize(): { width: number; height: number };
  toBitmap(): Buffer;
  toPNG(): Buffer;
  toJPEG(quality: number): Buffer;
};
export type NativeImageDeps = {
  createFromBuffer(buffer: Buffer): ImageLike;
  createFromBitmap(buffer: Buffer, options: { width: number; height: number }): ImageLike;
};

/**
 * Electron `nativeImage` wrapper around the pure `redactBitmap`. Encodes PNG, and falls back to JPEG quality 85 only
 * when the lossless image would exceed the image budget. The redacted bitmap is verified to still have the frame's size.
 */
export function createNativeRedactor(nativeImage: NativeImageDeps): RedactImage {
  return async (png, size, rects) => {
    const source = nativeImage.createFromBuffer(png);
    const actual = source.isEmpty() ? undefined : source.getSize();
    if (!actual || actual.width !== size.width || actual.height !== size.height) throw new ScreenPrepareError("redaction-failed");
    const redacted = nativeImage.createFromBitmap(redactBitmap(source.toBitmap(), size.width, size.height, rects), size);
    if (redacted.isEmpty()) throw new ScreenPrepareError("redaction-failed");
    const encoded = redacted.toPNG();
    if (encoded.length <= SCREEN_IMAGE_MAX_BYTES) return { image: encoded, mime: "image/png" };
    const compact = redacted.toJPEG(85);
    if (compact.length === 0 || compact.length > SCREEN_IMAGE_MAX_BYTES) throw new ScreenPrepareError("too-large");
    return { image: compact, mime: "image/jpeg" };
  };
}

export type ScreenPreparer = () => Promise<PreparedScreen>;

export function createScreenPreparer({
  frames,
  isCapturing,
  session,
  ocr,
  patterns,
  redact,
  now = Date.now,
  maxFrameAgeMs = MAX_FRAME_AGE_MS,
}: {
  frames(): CaptureFrame[];
  /** False once capture is paused or off; a result that finishes after that is discarded. */
  isCapturing(): boolean;
  /** Identity of the capture session; it changes on every pause, off, resume and on (CaptureService.session). */
  session(): number;
  ocr: OcrFn;
  /** Loads (once) the compiled guard patterns. Rejecting here means nothing is sent. */
  patterns(): Promise<CompiledGuardRule[]>;
  redact: RedactImage;
  now?: () => number;
  maxFrameAgeMs?: number;
}): ScreenPreparer {
  let inFlight: Promise<PreparedScreen> | undefined;

  async function prepare(): Promise<PreparedScreen> {
    if (!isCapturing()) throw new ScreenPrepareError("capture-stopped");
    const startedIn = session();
    const frame = frames().at(-1);
    if (!frame) throw new ScreenPrepareError("no-frame");
    if (now() - frame.capturedAt > maxFrameAgeMs) throw new ScreenPrepareError("stale-frame");
    let rules: CompiledGuardRule[];
    try {
      rules = await patterns();
    } catch {
      throw new ScreenPrepareError("patterns-unavailable");
    }
    let words;
    try {
      words = await ocr(frame.image);
    } catch (error) {
      throw new ScreenPrepareError(error instanceof OcrError && error.reason === "unavailable" ? "ocr-unavailable" : "ocr-failed");
    }
    const regions = findSensitiveRegions(words, { width: frame.width, height: frame.height }, rules);
    let result;
    try {
      result = await redact(frame.image, { width: frame.width, height: frame.height }, regions);
    } catch (error) {
      throw error instanceof ScreenPrepareError ? error : new ScreenPrepareError("redaction-failed");
    }
    // Paused or turned off while this was running, even if capture came back on since (a different session): the
    // user asked Dot to stop looking, so the result is dropped.
    if (!isCapturing() || session() !== startedIn) throw new ScreenPrepareError("capture-stopped");
    return { image: result.image, mime: result.mime, hiddenRegions: regions.length, capturedAt: frame.capturedAt };
  }

  // Concurrent callers share one OCR run instead of starting several helpers.
  return () => {
    inFlight ??= prepare().finally(() => {
      inFlight = undefined;
    });
    return inFlight;
  };
}
