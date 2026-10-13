import { getConnectionSnapshot, type ConnectionSnapshot } from "./auth-store.ts";
import { hasDirectUsageScope } from "./siwc.ts";
import { isLocalRequest, noStore } from "./auth-http.ts";

export const CHAT_MAX_BODY_BYTES = 24_000;

export function rejectNonLocal(request: Request, requireOrigin = false) {
  if (!isLocalRequest(request, requireOrigin)) return Response.json({ error: "Request rejected." }, { status: 403, headers: noStore });
  return undefined;
}

export async function authorizedConnection(): Promise<ConnectionSnapshot | Response> {
  try {
    const snapshot = await getConnectionSnapshot();
    if (!snapshot || !hasDirectUsageScope(snapshot.connection)) {
      return Response.json({ error: "Reconnect with ChatGPT to use chat." }, { status: 401, headers: noStore });
    }
    return snapshot;
  } catch {
    return Response.json({ error: "Local account status is unavailable." }, { status: 503, headers: noStore });
  }
}

export function isConnectionSnapshot(value: ConnectionSnapshot | Response): value is ConnectionSnapshot {
  return !(value instanceof Response);
}

export async function readBoundedJson(request: Request): Promise<unknown> {
  const contentType = request.headers.get("content-type")?.split(";", 1)[0].trim().toLowerCase();
  if (contentType !== "application/json") {
    throw new Error("A JSON request is required.");
  }
  const declared = Number(request.headers.get("content-length"));
  if (Number.isFinite(declared) && declared > CHAT_MAX_BODY_BYTES) throw new Error("The request is too large.");
  if (!request.body) throw new Error("The request body is missing.");
  const reader = request.body.getReader();
  const chunks: Uint8Array[] = [];
  let length = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      length += value.byteLength;
      if (length > CHAT_MAX_BODY_BYTES) {
        await reader.cancel();
        throw new Error("The request is too large.");
      }
      chunks.push(value);
    }
  } finally {
    reader.releaseLock();
  }
  const bytes = new Uint8Array(length);
  let offset = 0;
  for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.byteLength; }
  try { return JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(bytes)); }
  catch { throw new Error("The request body is invalid."); }
}

export function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === "object" && !Array.isArray(value);
}

export { noStore };
