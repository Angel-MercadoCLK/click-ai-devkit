import { isLocalRequest, noStore, setPendingCookie } from "../../../../lib/auth-http.ts";
import { beginAuthorization } from "../../../../lib/siwc.ts";

export const runtime = "nodejs";

export async function POST(request: Request) {
  if (!isLocalRequest(request, true)) return Response.json({ error: "Request rejected." }, { status: 403, headers: noStore });
  try {
    const { id, url } = await beginAuthorization();
    return Response.json({ url }, { headers: { ...noStore, "set-cookie": setPendingCookie(id) } });
  } catch {
    return Response.json({ error: "Local account authorization is unavailable." }, { status: 503, headers: noStore });
  }
}
