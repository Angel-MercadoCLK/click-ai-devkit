import { handleRemoveRequest } from "../../../../lib/attachments.ts";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function DELETE(request: Request, context: { params: Promise<{ id: string }> }) {
  const { id } = await context.params;
  return handleRemoveRequest(request, id);
}
