// How the mascot reads the answer of `POST /api/voice/listen` (contract: `{ text }` on success, `{ code, error }` on
// failure). Kept pure so it can be tested without a browser.

export type VoiceOutcome =
  | { kind: "text"; text: string }
  | { kind: "empty" }
  | { kind: "cancelled" }
  | { kind: "error"; message: string };

export const VOICE_UNAVAILABLE_MESSAGE = "La voz no está disponible ahora.";
export const VOICE_EMPTY_MESSAGE = "No te escuché. Intenta de nuevo.";
export const VOICE_FAILED_MESSAGE = "No se pudo usar la voz.";

/** Same limit as the chat composer. */
const MAX_TRANSCRIPT = 8000;
const MAX_DETAIL = 200;

export function readVoiceOutcome(status: number, body: unknown): VoiceOutcome {
  // The route is missing or the voice helper is not running: one short message, whatever the details were.
  if (status === 404 || status === 503) return { kind: "error", message: VOICE_UNAVAILABLE_MESSAGE };
  const fields = body && typeof body === "object" ? (body as Record<string, unknown>) : {};
  if (typeof fields.code === "string") {
    if (fields.code === "cancelled" || fields.code === "canceled") return { kind: "cancelled" };
    const detail = typeof fields.error === "string" ? fields.error.trim().slice(0, MAX_DETAIL) : "";
    return { kind: "error", message: detail ? `${VOICE_FAILED_MESSAGE} ${detail}` : VOICE_FAILED_MESSAGE };
  }
  if (status < 200 || status > 299) return { kind: "error", message: VOICE_FAILED_MESSAGE };
  const text = typeof fields.text === "string" ? fields.text.trim().slice(0, MAX_TRANSCRIPT) : "";
  return text ? { kind: "text", text } : { kind: "empty" };
}
