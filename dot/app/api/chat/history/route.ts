import { clearConversations, listConversations } from "../../../../lib/conversation-store.ts";
import { authorizedConnection, isConnectionSnapshot, noStore, rejectNonLocal } from "../../../../lib/chat-http.ts";

export const runtime = "nodejs";

export async function GET(request: Request) {
  const rejected = rejectNonLocal(request);
  if (rejected) return rejected;
  const snapshot = await authorizedConnection();
  if (!isConnectionSnapshot(snapshot)) return snapshot;
  try {
    const history = await listConversations(snapshot);
    return Response.json(history, { headers: noStore });
  } catch {
    return Response.json({ error: "Local conversation history is unavailable." }, { status: 503, headers: noStore });
  }
}

export async function DELETE(request: Request) {
  const rejected = rejectNonLocal(request, true);
  if (rejected) return rejected;
  const snapshot = await authorizedConnection();
  if (!isConnectionSnapshot(snapshot)) return snapshot;
  try {
    const revision = await clearConversations(snapshot);
    return Response.json({ revision, conversations: [] }, { headers: noStore });
  } catch {
    return Response.json({ error: "Local conversation history could not be cleared." }, { status: 503, headers: noStore });
  }
}
