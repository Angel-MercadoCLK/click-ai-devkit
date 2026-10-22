import { isLocalRequest, noStore } from "../../../../lib/auth-http.ts";
import { cancelListening, getVoiceController, readEmptyBody, VOICE_MESSAGES, VOICE_STATUS, VoiceRequestError } from "../../../../lib/voice-controller.ts";

export const runtime = "nodejs";

/** Stops a running listen (the second click on "Voz"). Nothing running is not an error. */
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
  if (!cancelListening(controller)) return Response.json({ code: "failed", error: VOICE_MESSAGES.failed }, { status: VOICE_STATUS.failed, headers: noStore });
  return Response.json({ cancelled: true }, { headers: noStore });
}
