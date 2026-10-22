import { isLocalRequest, noStore } from "../../../../lib/auth-http.ts";
import { cancelListening, getVoiceController, listenOnce, readEmptyBody, VOICE_MESSAGES, VOICE_STATUS, VoiceError, VoiceRequestError } from "../../../../lib/voice-controller.ts";

export const runtime = "nodejs";
export const maxDuration = 30;

/**
 * Listens on the default microphone through the desktop app's speech helper and returns the transcript only. The
 * request takes no input and never carries audio. If the browser abandons the request, the helper is stopped.
 */
export async function POST(request: Request) {
  if (!isLocalRequest(request, true)) return Response.json({ error: "Request rejected." }, { status: 403, headers: noStore });
  try {
    await readEmptyBody(request);
  } catch (error) {
    if (error instanceof VoiceRequestError) return Response.json({ error: error.message }, { status: error.status, headers: noStore });
    return Response.json({ error: "The request body is invalid." }, { status: 400, headers: noStore });
  }
  const controller = getVoiceController();
  if (!controller) return Response.json({ code: "unavailable", error: VOICE_MESSAGES.unavailable }, { status: VOICE_STATUS.unavailable, headers: noStore });
  const stop = () => { cancelListening(controller); };
  request.signal.addEventListener("abort", stop, { once: true });
  try {
    return Response.json(await listenOnce(controller), { headers: noStore });
  } catch (error) {
    const { code } = error instanceof VoiceError ? error : new VoiceError("failed");
    return Response.json({ code, error: VOICE_MESSAGES[code] }, { status: VOICE_STATUS[code], headers: noStore });
  } finally {
    request.signal.removeEventListener("abort", stop);
  }
}
