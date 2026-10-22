import { isLocalRequest, noStore } from "../../../../lib/auth-http.ts";
import { getCaptureController, readCaptureStatus, UNAVAILABLE_STATUS } from "../../../../lib/capture-controller.ts";

export const runtime = "nodejs";

export async function GET(request: Request) {
  if (!isLocalRequest(request)) return Response.json({ error: "Request rejected." }, { status: 403, headers: noStore });
  const controller = getCaptureController();
  const status = controller ? readCaptureStatus(controller) : undefined;
  if (!status) return Response.json({ available: false, ...UNAVAILABLE_STATUS }, { headers: noStore });
  return Response.json({ available: true, ...status }, { headers: noStore });
}
