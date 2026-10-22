import { deflateSync } from "node:zlib";
import type { Connection } from "./auth-store.ts";
import { requestResponses, sseFrames } from "./chat-provider.ts";

// Vision probe. `/v1/models` has no modality field, so the only way to learn whether an account-visible model accepts
// image input is to send it one. The image is a synthetic 64x64 PNG (a flat colour pattern, no personal data) and the
// result keeps only the HTTP status, the event type names seen and an error code: never prompt, image or response text.
// Nothing calls this automatically; a person runs it on demand through POST /api/chat/vision-probe.

export type VisionProbeResult = {
  model: string;
  httpStatus: number;
  /** True only when the model finished a response to a request that carried the image. */
  accepted: boolean;
  eventTypes: string[];
  errorCode: string | null;
};

const CRC_TABLE = Array.from({ length: 256 }, (_, n) => {
  let c = n;
  for (let k = 0; k < 8; k += 1) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
  return c >>> 0;
});

function crc32(bytes: Uint8Array) {
  let crc = 0xffffffff;
  for (const byte of bytes) crc = CRC_TABLE[(crc ^ byte) & 0xff] ^ (crc >>> 8);
  return (crc ^ 0xffffffff) >>> 0;
}

function chunk(type: string, data: Uint8Array) {
  const out = Buffer.alloc(12 + data.length);
  out.writeUInt32BE(data.length, 0);
  out.write(type, 4, "ascii");
  Buffer.from(data).copy(out, 8);
  out.writeUInt32BE(crc32(out.subarray(4, 8 + data.length)), 8 + data.length);
  return out;
}

/** A 64x64 RGB PNG: four flat colour quadrants. Deterministic, tiny, and obviously not anyone's screen. */
export function syntheticProbePng(size = 64): Buffer {
  const header = Buffer.alloc(13);
  header.writeUInt32BE(size, 0);
  header.writeUInt32BE(size, 4);
  header.set([8, 2, 0, 0, 0], 8);
  const colors = [[220, 60, 60], [60, 160, 90], [70, 90, 210], [240, 200, 60]];
  const raw = Buffer.alloc((size * 3 + 1) * size);
  for (let y = 0; y < size; y += 1) {
    const row = y * (size * 3 + 1);
    raw[row] = 0;
    for (let x = 0; x < size; x += 1) {
      const color = colors[(y < size / 2 ? 0 : 2) + (x < size / 2 ? 0 : 1)];
      raw.set(color, row + 1 + x * 3);
    }
  }
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk("IHDR", header),
    chunk("IDAT", deflateSync(raw)),
    chunk("IEND", Buffer.alloc(0)),
  ]);
}

const SAFE_TOKEN = /^[A-Za-z0-9_.-]{1,64}$/;
const MAX_EVENT_TYPES = 30;
const MAX_ERROR_BODY_BYTES = 16_000;

function safeToken(value: unknown) {
  return typeof value === "string" && SAFE_TOKEN.test(value) ? value : undefined;
}

/** The `code` (or `type`) of an upstream error object, kept only when it is a short identifier. */
function errorCodeOf(value: unknown): string | null {
  if (!value || typeof value !== "object") return null;
  const { code, type } = value as { code?: unknown; type?: unknown };
  return safeToken(code) ?? safeToken(type) ?? "unknown";
}

async function readErrorCode(response: Response): Promise<string | null> {
  if (!response.body) return null;
  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let length = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      length += value.byteLength;
      if (length > MAX_ERROR_BODY_BYTES) { await reader.cancel(); return "unknown"; }
      chunks.push(value);
    }
    const parsed = JSON.parse(new TextDecoder().decode(Buffer.concat(chunks))) as { error?: unknown };
    return errorCodeOf(parsed.error);
  } catch {
    return "unknown";
  } finally {
    reader.releaseLock();
  }
}

/**
 * Sends the synthetic image to `model` with the same request builder chat uses and reports whether it was accepted.
 * `model` must already be checked against the account catalog by the caller.
 */
export async function runVisionProbe(connection: Connection, model: string, fetcher: typeof fetch = fetch, signal?: AbortSignal): Promise<VisionProbeResult> {
  const timeout = AbortSignal.timeout(45_000);
  const probeSignal = signal ? AbortSignal.any([signal, timeout]) : timeout;
  const dataUrl = `data:image/png;base64,${syntheticProbePng().toString("base64")}`;
  const response = await requestResponses(connection, model, [{
    role: "user",
    content: [{ type: "input_text", text: "Reply with the single word OK." }, { type: "input_image", image_url: dataUrl }],
  }], fetcher, probeSignal);
  const result: VisionProbeResult = { model, httpStatus: response.status, accepted: false, eventTypes: [], errorCode: null };
  if (!response.ok || !response.body) {
    result.errorCode = await readErrorCode(response);
    return result;
  }
  try {
    for await (const data of sseFrames(response.body, probeSignal)) {
      let event: unknown;
      try { event = JSON.parse(data); } catch { result.errorCode ??= "invalid-event"; break; }
      if (!event || typeof event !== "object") { result.errorCode ??= "invalid-event"; break; }
      const record = event as { type?: unknown; error?: unknown; response?: { error?: unknown } };
      const type = typeof record.type === "string" && /^[a-z0-9_.]{1,80}$/.test(record.type) ? record.type : "unknown";
      if (!result.eventTypes.includes(type) && result.eventTypes.length < MAX_EVENT_TYPES) result.eventTypes.push(type);
      if (type === "response.completed") { result.accepted = true; break; }
      if (type === "response.failed" || type === "response.incomplete" || type === "error") {
        result.errorCode = errorCodeOf(record.error ?? record.response?.error) ?? "unknown";
        break;
      }
    }
  } catch {
    result.errorCode ??= "stream-error";
  }
  return result;
}
