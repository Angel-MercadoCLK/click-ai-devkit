"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { readVoiceOutcome, VOICE_EMPTY_MESSAGE, VOICE_UNAVAILABLE_MESSAGE } from "./voice-result.ts";

/** Listening through `POST /api/voice/listen`; the transcript goes to `onTranscript` for review, never straight to the model. */
export function useVoice({ onTranscript }: { onTranscript: (text: string) => void }) {
  const [listening, setListening] = useState(false);
  const [message, setMessage] = useState("");
  const request = useRef<AbortController | null>(null);
  const transcript = useRef(onTranscript);
  transcript.current = onTranscript;

  useEffect(() => () => request.current?.abort(), []);

  const start = useCallback(async () => {
    if (request.current) return;
    const current = new AbortController();
    request.current = current;
    setMessage("");
    setListening(true);
    let outcome;
    try {
      const response = await fetch("/api/voice/listen", { method: "POST", cache: "no-store", signal: current.signal });
      outcome = readVoiceOutcome(response.status, await response.json().catch(() => undefined));
    } catch {
      if (current.signal.aborted) return;
      outcome = { kind: "error" as const, message: VOICE_UNAVAILABLE_MESSAGE };
    } finally {
      if (request.current === current) {
        request.current = null;
        setListening(false);
      }
    }
    if (current.signal.aborted) return;
    if (outcome.kind === "text") transcript.current(outcome.text);
    else if (outcome.kind === "empty") setMessage(VOICE_EMPTY_MESSAGE);
    else if (outcome.kind === "error") setMessage(outcome.message);
  }, []);

  const cancel = useCallback(() => {
    request.current?.abort();
    request.current = null;
    setListening(false);
    // Tells the speech helper to stop; a failure here changes nothing the user can see.
    void fetch("/api/voice/cancel", { method: "POST", cache: "no-store" }).catch(() => {});
  }, []);

  return { listening, message, start, cancel };
}
