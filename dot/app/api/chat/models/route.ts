import { isConnectionSnapshot, noStore, rejectNonLocal, authorizedConnection } from "../../../../lib/chat-http.ts";
import { listAvailableModels } from "../../../../lib/chat-provider.ts";
import { withCurrentConnection } from "../../../../lib/auth-store.ts";

export const runtime = "nodejs";

export async function GET(request: Request) {
  const rejected = rejectNonLocal(request);
  if (rejected) return rejected;
  const snapshot = await authorizedConnection();
  if (!isConnectionSnapshot(snapshot)) return snapshot;
  try {
    const models = await withCurrentConnection(snapshot, () => listAvailableModels(snapshot.connection));
    if (!models) return Response.json({ error: "Reconnect with ChatGPT before loading models." }, { status: 401, headers: noStore });
    return Response.json({ models }, { headers: noStore });
  } catch {
    return Response.json({ error: "The ChatGPT model catalog is unavailable. Try again later." }, { status: 502, headers: noStore });
  }
}
