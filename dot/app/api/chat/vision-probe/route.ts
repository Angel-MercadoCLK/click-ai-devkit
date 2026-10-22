import { withCurrentConnection } from "../../../../lib/auth-store.ts";
import { authorizedConnection, isConnectionSnapshot, isRecord, noStore, readBoundedJson, rejectNonLocal } from "../../../../lib/chat-http.ts";
import { listAvailableModels } from "../../../../lib/chat-provider.ts";
import { runVisionProbe } from "../../../../lib/vision-probe.ts";

export const runtime = "nodejs";
export const maxDuration = 60;

/**
 * On-demand check of whether one account-visible model accepts image input. Sends a synthetic 64x64 PNG (no personal
 * data) and returns only the status, event type names and an error code. Never called by the app on its own.
 *   curl -X POST http://127.0.0.1:47321/api/chat/vision-probe -H 'origin: http://127.0.0.1:47321' -H 'content-type: application/json' -d '{"model":"<slug>"}'
 */
export async function POST(request: Request) {
  const rejected = rejectNonLocal(request, true);
  if (rejected) return rejected;
  const snapshot = await authorizedConnection();
  if (!isConnectionSnapshot(snapshot)) return snapshot;
  let body: unknown;
  try { body = await readBoundedJson(request); }
  catch { return Response.json({ error: "The request body is invalid." }, { status: 400, headers: noStore }); }
  if (!isRecord(body) || Object.keys(body).length !== 1 || typeof body.model !== "string" || !/^[a-zA-Z0-9._:-]{1,120}$/.test(body.model)) {
    return Response.json({ error: "Send a model." }, { status: 400, headers: noStore });
  }
  const model = body.model;
  let models;
  try { models = await withCurrentConnection(snapshot, () => listAvailableModels(snapshot.connection)); }
  catch { return Response.json({ error: "The ChatGPT model catalog is unavailable. Try again later." }, { status: 502, headers: noStore }); }
  if (!models) return Response.json({ error: "Reconnect with ChatGPT before probing a model." }, { status: 401, headers: noStore });
  if (!models.some((candidate) => candidate.slug === model)) {
    return Response.json({ error: "Choose a model available to this ChatGPT account." }, { status: 400, headers: noStore });
  }
  try {
    return Response.json(await runVisionProbe(snapshot.connection, model, fetch, request.signal), { headers: noStore });
  } catch {
    return Response.json({ error: "The probe could not run." }, { status: 502, headers: noStore });
  }
}
