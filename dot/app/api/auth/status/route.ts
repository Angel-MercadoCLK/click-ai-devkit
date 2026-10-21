import { getConnection } from "../../../../lib/auth-store.ts";
import { hasDirectUsageScope } from "../../../../lib/siwc.ts";
import { isLocalRequest, noStore } from "../../../../lib/auth-http.ts";

export const runtime = "nodejs";

export async function GET(request: Request) {
  if (!isLocalRequest(request)) return Response.json({ error: "Request rejected." }, { status: 403, headers: noStore });
  try {
    const connection = await getConnection();
    if (!connection) {
      return Response.json({ connected: false }, { headers: noStore });
    }
    if (!hasDirectUsageScope(connection)) return Response.json({ connected: false, reauthorizationRequired: true }, { headers: noStore });
    return Response.json({ connected: true, email: connection.email ?? null }, { headers: noStore });
  } catch {
    return Response.json({ connected: false, error: "Local account status is unavailable." }, { status: 503, headers: noStore });
  }
}
