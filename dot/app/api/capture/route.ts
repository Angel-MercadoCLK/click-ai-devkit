import { isLocalRequest, noStore } from "../../../lib/auth-http.ts";
import { CaptureRequestError, getCaptureController, readCaptureAction, sanitizeCaptureStatus, UNAVAILABLE_STATUS } from "../../../lib/capture-controller.ts";

export const runtime = "nodejs";

export async function POST(request: Request) {
  if (!isLocalRequest(request, true)) return Response.json({ error: "Request rejected." }, { status: 403, headers: noStore });
  let action;
  try {
    action = await readCaptureAction(request);
  } catch (error) {
    if (error instanceof CaptureRequestError) return Response.json({ error: error.message }, { status: error.status, headers: noStore });
    return Response.json({ error: "The request body is invalid." }, { status: 400, headers: noStore });
  }
  const controller = getCaptureController();
  if (!controller) {
    return Response.json({ available: false, ...UNAVAILABLE_STATUS, error: "Screen capture is available in the desktop app." }, { status: 503, headers: noStore });
  }
  try {
    const status = sanitizeCaptureStatus(await controller.apply(action));
    if (!status) throw new Error("Unexpected status.");
    return Response.json({ available: true, ...status }, { headers: noStore });
  } catch {
    return Response.json({ error: "Screen capture could not be changed." }, { status: 503, headers: noStore });
  }
}
