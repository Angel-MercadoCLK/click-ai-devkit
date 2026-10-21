import { deleteConversation } from "../../../../../lib/conversation-store.ts";
import { authorizedConnection, isConnectionSnapshot, noStore, rejectNonLocal } from "../../../../../lib/chat-http.ts";

export const runtime = "nodejs";

export async function DELETE(request: Request, context: { params: Promise<{ id: string }> }) {
  const rejected = rejectNonLocal(request, true);
  if (rejected) return rejected;
  const snapshot = await authorizedConnection();
  if (!isConnectionSnapshot(snapshot)) return snapshot;
  const { id } = await context.params;
  try {
    const deleted = await deleteConversation(snapshot, id);
    return Response.json({ deleted }, { headers: noStore });
  } catch {
    return Response.json({ error: "The conversation could not be removed." }, { status: 503, headers: noStore });
  }
}
