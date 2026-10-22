import { isLocalRequest, noStore } from "../../../lib/auth-http.ts";
import { getWindowController, readWindowRequest, readWindowState, UNAVAILABLE_WINDOW_STATE, WindowRequestError } from "../../../lib/window-mode.ts";

export const runtime = "nodejs";

export async function GET(request: Request) {
  if (!isLocalRequest(request)) return Response.json({ error: "Request rejected." }, { status: 403, headers: noStore });
  const controller = getWindowController();
  const state = controller ? readWindowState(controller) : undefined;
  if (!state) return Response.json({ available: false, ...UNAVAILABLE_WINDOW_STATE }, { headers: noStore });
  return Response.json({ available: true, ...state }, { headers: noStore });
}

export async function POST(request: Request) {
  if (!isLocalRequest(request, true)) return Response.json({ error: "Request rejected." }, { status: 403, headers: noStore });
  let action;
  try {
    action = await readWindowRequest(request);
  } catch (error) {
    if (error instanceof WindowRequestError) return Response.json({ error: error.message }, { status: error.status, headers: noStore });
    return Response.json({ error: "The request body is invalid." }, { status: 400, headers: noStore });
  }
  const controller = getWindowController();
  if (!controller) {
    return Response.json({ available: false, ...UNAVAILABLE_WINDOW_STATE, error: "The mascot window is available in the desktop app." }, { status: 503, headers: noStore });
  }
  try {
    if (action.kind === "mode") controller.setMode(action.mode);
    else controller.moveTo(action.x, action.y);
    const state = readWindowState(controller);
    if (!state) throw new Error("Unexpected state.");
    return Response.json({ available: true, ...state }, { headers: noStore });
  } catch {
    return Response.json({ error: "The window could not be changed." }, { status: 503, headers: noStore });
  }
}
