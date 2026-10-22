// HTTP side of the Claude Code companion: the hook endpoint the relay posts to, and the routes the mascot page uses.
//
// The hook endpoint is called by another local program (`click dot-hook`), not by the mascot page, so it cannot use the
// control cookie. It authenticates with the per-launch bearer token the desktop shell publishes on
// `globalThis[Symbol.for("click-dot.agent-hook")]` and writes to a discovery file only the user can read. A browser page
// can never call it: any request carrying an `Origin` header is refused before the token is even looked at.
//
// Without a published token (plain browser development) the hook endpoint accepts nothing.

import { getAgentBroker } from "./agent-broker.ts";
import { isLocalRequest, noStore } from "./auth-http.ts";
import { CONTROL_TOKEN_PATTERN, hasControlAccess, sameSecret } from "./control-token.ts";

export const AGENT_HOOK_KEY = Symbol.for("click-dot.agent-hook");
/** Wire contract: the relay never sends more than this, and the endpoint refuses more with 413. */
export const HOOK_MAX_BODY_BYTES = 512 * 1024;
/** The mascot page sends an id, a choice and a few answers. */
export const UI_MAX_BODY_BYTES = 64 * 1024;

const MAX_EVENT_LENGTH = 64;
const MAX_SESSION_ID_LENGTH = 200;
const EVENT_NAME = /^[A-Za-z]+$/;

export class AgentRequestError extends Error {
  readonly status: 400 | 413;

  constructor(status: 400 | 413, message: string) {
    super(message);
    this.status = status;
  }
}

const isRecord = (value: unknown): value is Record<string, unknown> => !!value && typeof value === "object" && !Array.isArray(value);
const fail = (status: number, error: string) => Response.json({ error }, { status, headers: noStore });
const noContent = () => new Response(null, { status: 204, headers: noStore });

/** The token the desktop shell published, if it is a well-formed one. */
export function readHookToken(): string | undefined {
  const published = (globalThis as Record<symbol, unknown>)[AGENT_HOOK_KEY];
  const token = isRecord(published) ? published.token : undefined;
  return typeof token === "string" && CONTROL_TOKEN_PATTERN.test(token) ? token : undefined;
}

/** True when the request carries the launch's bearer token. Compared in constant time. */
export function hasHookAccess(request: Request): boolean {
  const token = readHookToken();
  if (!token) return false;
  const match = /^Bearer (\S+)$/i.exec(request.headers.get("authorization") ?? "");
  return match !== null && sameSecret(token, match[1]);
}

/** Reads a JSON body, refusing anything larger than `limit` bytes while streaming. */
export async function readJsonBody(request: Request, limit: number): Promise<unknown> {
  const contentType = request.headers.get("content-type")?.split(";", 1)[0].trim().toLowerCase();
  if (contentType !== "application/json") throw new AgentRequestError(400, "A JSON request is required.");
  const declared = Number(request.headers.get("content-length"));
  if (Number.isFinite(declared) && declared > limit) throw new AgentRequestError(413, "The request is too large.");
  if (!request.body) throw new AgentRequestError(400, "The request body is missing.");
  const reader = request.body.getReader();
  const chunks: Uint8Array[] = [];
  let length = 0;
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      length += value.byteLength;
      if (length > limit) {
        await reader.cancel();
        throw new AgentRequestError(413, "The request is too large.");
      }
      chunks.push(value);
    }
  } finally {
    reader.releaseLock();
  }
  const bytes = new Uint8Array(length);
  let offset = 0;
  for (const chunk of chunks) {
    bytes.set(chunk, offset);
    offset += chunk.byteLength;
  }
  try {
    return JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(bytes));
  } catch {
    throw new AgentRequestError(400, "The request body is invalid.");
  }
}

async function readBody(request: Request, limit: number) {
  try {
    return { body: await readJsonBody(request, limit) };
  } catch (error) {
    if (error instanceof AgentRequestError) return { response: fail(error.status, error.message) };
    return { response: fail(400, "The request body is invalid.") };
  }
}

/** `{ version: 1, agent: "claude-code", event, payload }`, or undefined for anything else. */
function parseEnvelope(value: unknown): { event: string; payload: Record<string, unknown> } | undefined {
  if (!isRecord(value) || value.version !== 1 || value.agent !== "claude-code") return undefined;
  const { event, payload } = value;
  if (typeof event !== "string" || event.length > MAX_EVENT_LENGTH || !EVENT_NAME.test(event) || !isRecord(payload)) return undefined;
  return { event, payload };
}

/** POST /api/agent/hook */
export async function handleHookRequest(request: Request): Promise<Response> {
  // A browser page can send any Host-matching request but cannot remove its Origin header: refuse those first.
  if (request.headers.has("origin") || !isLocalRequest(request)) return fail(403, "Request rejected.");
  if (!hasHookAccess(request)) return fail(401, "Unauthorized.");
  const read = await readBody(request, HOOK_MAX_BODY_BYTES);
  if (read.response) return read.response;
  const envelope = parseEnvelope(read.body);
  if (!envelope) return fail(400, "The hook request is invalid.");
  const { event, payload } = envelope;
  const broker = getAgentBroker();
  if (event !== "PermissionRequest") {
    broker.record(event, payload);
    return noContent();
  }
  if (typeof payload.tool_name !== "string" || !payload.tool_name) return fail(400, "The permission request names no tool.");
  const id = payload.session_id;
  const sessionId = typeof id === "string" && id.length > 0 && id.length <= MAX_SESSION_ID_LENGTH ? id : "unknown";
  // Held until the user decides; every other way out answers 204, which makes Claude Code ask in its terminal.
  const outcome = await broker.enqueue({ sessionId, payload, signal: request.signal });
  return outcome.kind === "decision" ? Response.json(outcome.decision, { headers: noStore }) : noContent();
}

/** The page's routes: the same guard as the other control routes. A GET sends no Origin, a POST must send the exact one. */
const guarded = (request: Request, write: boolean) => isLocalRequest(request, write) && hasControlAccess(request);

/** GET /api/agent/state */
export function handleStateRequest(request: Request): Response {
  if (!guarded(request, false)) return fail(403, "Request rejected.");
  return Response.json(getAgentBroker().state(), { headers: noStore });
}

/** POST /api/agent/ack */
export async function handleAckRequest(request: Request): Promise<Response> {
  if (!guarded(request, true)) return fail(403, "Request rejected.");
  const read = await readBody(request, UI_MAX_BODY_BYTES);
  if (read.response) return read.response;
  const body = read.body;
  if (!isRecord(body) || Object.keys(body).length !== 1 || typeof body.id !== "string") return fail(400, "An id is required.");
  return getAgentBroker().ack(body.id) ? Response.json({ acknowledged: true }, { headers: noStore }) : fail(404, "There is no such pending request.");
}

/** POST /api/agent/decision */
export async function handleDecisionRequest(request: Request): Promise<Response> {
  if (!guarded(request, true)) return fail(403, "Request rejected.");
  const read = await readBody(request, UI_MAX_BODY_BYTES);
  if (read.response) return read.response;
  const body = read.body;
  if (!isRecord(body) || typeof body.id !== "string" || typeof body.choice !== "string") return fail(400, "An id and a choice are required.");
  if (Object.keys(body).some((key) => key !== "id" && key !== "choice" && key !== "answers")) return fail(400, "The request has unknown fields.");
  const result = getAgentBroker().decide(body.id, body.choice, body.answers);
  return result.ok ? Response.json({ decided: true }, { headers: noStore }) : fail(result.status, result.error);
}
