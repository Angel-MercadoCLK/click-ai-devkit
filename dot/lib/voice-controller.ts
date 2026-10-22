// Voice input bridge for the web side.
//
// Listening runs in the Electron main process (a Windows speech helper). Next runs in that same process, so main
// publishes a small controller on `globalThis[Symbol.for("click-dot.voice-controller")]` (the same pattern as the
// capture controller). Everything read from it here is validated. Only a sanitized transcript or a known error code
// leaves; the helper's own text, paths and audio never cross this boundary.

export type VoiceController = {
  /** Resolves with `{ text }`; rejects with an error carrying a known `code`. */
  listen(): Promise<unknown>;
  cancel(): void;
};

export const VOICE_CONTROLLER_KEY = Symbol.for("click-dot.voice-controller");
export const VOICE_MAX_TEXT_CHARS = 2000;
export const VOICE_MAX_BODY_BYTES = 16;

export const VOICE_CODES = ["unavailable", "no-recognizer", "no-microphone", "mic-denied", "privacy", "no-speech", "timeout", "failed", "busy", "cancelled"] as const;
export type VoiceCode = (typeof VOICE_CODES)[number];

/** Fixed wording for the page. The helper's own error text never crosses this boundary. */
export const VOICE_MESSAGES: Record<VoiceCode, string> = {
  unavailable: "La voz está disponible en la aplicación de escritorio de Dot, en Windows.",
  "no-recognizer": "Windows no tiene reconocimiento de voz instalado para español ni inglés. Agrégalo en Configuración > Hora e idioma > Voz.",
  "no-microphone": "Dot no encontró un micrófono. Conecta uno y vuelve a intentarlo.",
  "mic-denied": "Windows no deja que Dot use el micrófono. Actívalo en Configuración > Privacidad y seguridad > Micrófono.",
  privacy: "El reconocimiento de voz en línea de Windows está apagado. Actívalo en Configuración > Privacidad y seguridad > Voz.",
  "no-speech": "No te escuché. Pulsa Voz y habla de nuevo.",
  timeout: "La escucha tardó demasiado. Inténtalo otra vez.",
  failed: "Dot no pudo reconocer tu voz. Inténtalo otra vez.",
  busy: "Dot ya está escuchando.",
  cancelled: "Se detuvo la escucha.",
};

/** Status for each code: 4xx when the user's computer needs something, 5xx when the helper broke. */
export const VOICE_STATUS: Record<VoiceCode, number> = {
  unavailable: 503,
  "no-recognizer": 422,
  "no-microphone": 422,
  "mic-denied": 422,
  privacy: 422,
  "no-speech": 422,
  timeout: 504,
  failed: 500,
  busy: 409,
  cancelled: 409,
};

export class VoiceError extends Error {
  readonly code: VoiceCode;

  constructor(code: VoiceCode) {
    super(VOICE_MESSAGES[code]);
    this.code = code;
  }
}

export function getVoiceController(): VoiceController | undefined {
  const candidate = (globalThis as Record<symbol, unknown>)[VOICE_CONTROLLER_KEY];
  if (!candidate || typeof candidate !== "object") return undefined;
  const { listen, cancel } = candidate as Partial<VoiceController>;
  if (typeof listen !== "function" || typeof cancel !== "function") return undefined;
  return candidate as VoiceController;
}

// Control characters, line and paragraph separators, and NEL all become spaces.
const CONTROL = /[\p{Cc}\u2028\u2029]/gu;

/** Copies only `text` out of a result, flattens control characters, trims and caps it. Anything else is rejected whole. */
export function sanitizeTranscript(value: unknown): { text: string } | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  const { text } = value as Record<string, unknown>;
  if (typeof text !== "string") return undefined;
  const flat = text.replace(CONTROL, " ").replace(/ {2,}/g, " ").trim();
  const capped = Array.from(flat).slice(0, VOICE_MAX_TEXT_CHARS).join("").trim();
  return capped ? { text: capped } : undefined;
}

const toCode = (error: unknown): VoiceCode => {
  const code = (error as { code?: unknown } | null | undefined)?.code;
  return VOICE_CODES.find((known) => known === code) ?? "failed";
};

/** Listens once. Every failure is mapped to a known code; no raw error or helper text crosses. */
export async function listenOnce(controller: VoiceController): Promise<{ text: string }> {
  let result: unknown;
  try {
    result = await controller.listen();
  } catch (error) {
    throw new VoiceError(toCode(error));
  }
  const transcript = sanitizeTranscript(result);
  if (!transcript) throw new VoiceError("failed");
  return transcript;
}

/** Stops a running listen. Never throws. */
export function cancelListening(controller: VoiceController): boolean {
  try {
    controller.cancel();
    return true;
  } catch {
    return false;
  }
}

export class VoiceRequestError extends Error {
  readonly status: 400 | 413;

  constructor(status: 400 | 413, message: string) {
    super(message);
    this.status = status;
  }
}

/** The voice routes take no input: the body must be absent, empty, or exactly `{}`, and is bounded while streaming. */
export async function readEmptyBody(request: Request): Promise<void> {
  const declared = Number(request.headers.get("content-length"));
  if (Number.isFinite(declared) && declared > VOICE_MAX_BODY_BYTES) throw new VoiceRequestError(413, "The request is too large.");
  if (!request.body) return;
  const reader = request.body.getReader();
  const chunks: Uint8Array[] = [];
  let length = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      length += value.byteLength;
      if (length > VOICE_MAX_BODY_BYTES) {
        await reader.cancel();
        throw new VoiceRequestError(413, "The request is too large.");
      }
      chunks.push(value);
    }
  } finally {
    reader.releaseLock();
  }
  if (length === 0) return;
  const contentType = request.headers.get("content-type")?.split(";", 1)[0].trim().toLowerCase();
  if (contentType !== "application/json") throw new VoiceRequestError(400, "A JSON request is required.");
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
    throw new VoiceRequestError(400, "The request body is invalid.");
  }
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed) || Object.keys(parsed).length > 0) {
    throw new VoiceRequestError(400, "This request takes no input.");
  }
}
