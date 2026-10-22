import { isLocalRequest, noStore } from "../../../../lib/auth-http.ts";
import { getCaptureController, prepareScreen, ScreenPrepareError } from "../../../../lib/capture-controller.ts";
import { screenPreviews, toDataUrl } from "../../../../lib/screen-previews.ts";

export const runtime = "nodejs";
export const maxDuration = 30;

/**
 * Prepares the newest screen frame for chat: the desktop app runs local OCR and hides personal data, and only that
 * redacted image comes back. It is kept here under a short-lived, one-use id; the browser gets a thumbnail to show
 * and the id to name it later. No request body is read, and the browser never supplies image bytes.
 */
export async function POST(request: Request) {
  if (!isLocalRequest(request, true)) return Response.json({ error: "Request rejected." }, { status: 403, headers: noStore });
  const controller = getCaptureController();
  if (!controller) return Response.json({ code: "unavailable", error: "Screen capture is available in the desktop app." }, { status: 503, headers: noStore });
  try {
    const prepared = await prepareScreen(controller);
    const { id, expiresAt } = screenPreviews().put(prepared);
    return Response.json({
      previewId: id,
      image: toDataUrl(prepared),
      hiddenRegions: prepared.hiddenRegions,
      capturedAt: prepared.capturedAt,
      expiresAt,
    }, { headers: noStore });
  } catch (error) {
    const prepareError = error instanceof ScreenPrepareError ? error : new ScreenPrepareError("invalid-result");
    // Nothing was prepared, so nothing may be attached. 409: the screen is not in a state to be shared right now.
    return Response.json({ code: prepareError.code, error: prepareError.message }, { status: prepareError.code === "unavailable" ? 503 : 409, headers: noStore });
  }
}
