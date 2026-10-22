import { clearConnection } from "../../../../lib/auth-store.ts";
import { isLocalRequest, noStore } from "../../../../lib/auth-http.ts";

export const runtime = "nodejs";

export async function POST(request: Request) {
  if (!isLocalRequest(request, true)) return Response.json({ error: "Request rejected." }, { status: 403, headers: noStore });
  try {
    await clearConnection();
    return Response.json({ connected: false }, { headers: noStore });
  } catch {
    return Response.json({ error: "The local account connection could not be removed." }, { status: 503, headers: noStore });
  }
}
