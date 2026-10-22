import { engineHandlers } from "../../../../../lib/engine-http.ts";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(request: Request, context: { params: Promise<{ id: string }> }) {
  return engineHandlers.thread(request, (await context.params).id);
}

export async function DELETE(request: Request, context: { params: Promise<{ id: string }> }) {
  return engineHandlers.thread(request, (await context.params).id);
}
