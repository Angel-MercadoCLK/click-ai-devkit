// A chat turn on the Codex engine (S2): the same SSE stream the built-in loop sends (`delta`, `tool`, `completed`, `error`),
// plus one `thread` event that tells the page which Codex thread the chat is. Codex keeps the history itself, so nothing is
// saved here; the body allowlist is as strict as the built-in route's, with `threadId` and `attachments` added.

import { homedir } from "node:os";
import { isRecord, readBoundedJson } from "./chat-http.ts";
import { noStore } from "./auth-http.ts";
import { getAttachmentStore, localImagePaths, promptWithFiles, resolveAttachmentIds, type AttachmentStore } from "./attachments.ts";
import type { CodexEngine } from "./codex-engine.ts";
import type { ToolEvent } from "./tool-events.ts";

const ALLOWED_KEYS = ["conversationId", "revision", "model", "prompt", "threadId", "attachments"];
const THREAD_ID = /^[0-9A-Za-z._-]{8,100}$/;
const CONVERSATION_ID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const MAX_PROMPT = 8_000;

const encoder = new TextEncoder();
export type CodexSseEvent = "delta" | "tool" | "completed" | "error" | "thread";
export const formatCodexEvent = (type: CodexSseEvent, payload: { text?: string; message?: string; threadId?: string } | ToolEvent) =>
  encoder.encode(`event: ${type}\ndata: ${JSON.stringify(payload)}\n\n`);

const MESSAGES = {
  invalid: "Send a valid message.",
  openFailed: "Codex no pudo abrir el chat. Vuelve a intentarlo.",
  busy: "Este chat de Codex ya está respondiendo.",
  failed: "Codex no pudo completar la respuesta.",
  stopped: "Detenido. Lo que Codex ya hizo no se deshace.",
} as const;

const fail = (status: number, error: string) => Response.json({ error }, { status, headers: noStore });

export type CodexStreamDeps = {
  /** The folder new chats start in. Defaults to the user's home. */
  cwd?: () => string;
  attachments?: () => AttachmentStore | null;
};

type Body = { prompt: string; threadId?: string; attachments?: unknown };

/** The body, checked. A field the built-in route would not know is refused too, so the page cannot smuggle options in. */
function readBody(body: unknown): Body | undefined {
  if (!isRecord(body) || Object.keys(body).some((key) => !ALLOWED_KEYS.includes(key))) return undefined;
  const { prompt, threadId, conversationId, revision, model } = body;
  if (typeof prompt !== "string" || prompt.trim().length === 0 || prompt.length > MAX_PROMPT) return undefined;
  if (threadId !== undefined && (typeof threadId !== "string" || !THREAD_ID.test(threadId))) return undefined;
  if (conversationId !== undefined && (typeof conversationId !== "string" || !CONVERSATION_ID.test(conversationId))) return undefined;
  if (revision !== undefined && (!Number.isSafeInteger(revision) || (revision as number) < 0)) return undefined;
  if (model !== undefined && (typeof model !== "string" || !/^[a-zA-Z0-9._:-]{1,120}$/.test(model))) return undefined;
  return { prompt, ...(threadId === undefined ? {} : { threadId }), ...(body.attachments === undefined ? {} : { attachments: body.attachments }) };
}

export async function handleCodexStream(request: Request, engine: CodexEngine, deps: CodexStreamDeps = {}): Promise<Response> {
  let raw: unknown;
  try {
    raw = await readBoundedJson(request);
  } catch (error) {
    return fail(400, error instanceof Error ? error.message : "The request body is invalid.");
  }
  const body = readBody(raw);
  if (!body) return fail(400, MESSAGES.invalid);
  const files = resolveAttachmentIds((deps.attachments ?? getAttachmentStore)(), body.attachments);
  if (!files) return fail(400, MESSAGES.invalid);

  let threadId: string;
  try {
    // A chat that already exists keeps its own folder (a chat started from the Codex CLI in a project stays in that project);
    // a new one starts in the user's home.
    let cwd = (deps.cwd ?? homedir)();
    if (body.threadId) cwd = (await engine.readThread(body.threadId).catch(() => undefined))?.cwd || cwd;
    ({ threadId } = await engine.startOrResumeThread({ ...(body.threadId ? { threadId: body.threadId } : {}), cwd }));
  } catch {
    return fail(502, MESSAGES.openFailed);
  }

  const responseAbort = new AbortController();
  const signal = AbortSignal.any([request.signal, responseAbort.signal]);
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      const send = (event: Uint8Array) => {
        if (signal.aborted) return false;
        try { controller.enqueue(event); return true; }
        catch { return false; }
      };
      void (async () => {
        try {
          send(formatCodexEvent("thread", { threadId }));
          const result = await engine.runTurn({
            threadId,
            text: promptWithFiles(body.prompt, files),
            localImages: localImagePaths(files),
            signal,
            emit: (event) => (event.type === "delta" ? send(formatCodexEvent("delta", { text: event.text })) : send(formatCodexEvent("tool", event.tool))),
          });
          if (signal.aborted) return;
          if (result.status === "completed") send(formatCodexEvent("completed", {}));
          else send(formatCodexEvent("error", { message: result.status === "interrupted" ? MESSAGES.stopped : (result.error ?? MESSAGES.failed) }));
        } catch (error) {
          if (!signal.aborted) send(formatCodexEvent("error", { message: error instanceof Error && /already has a response/i.test(error.message) ? MESSAGES.busy : MESSAGES.failed }));
        } finally {
          try { controller.close(); } catch { /* The browser already closed the local stream. */ }
        }
      })();
    },
    cancel(reason) {
      responseAbort.abort(reason);
    },
  });
  return new Response(stream, { headers: { ...noStore, "content-type": "text/event-stream; charset=utf-8", connection: "keep-alive", "x-accel-buffering": "no" } });
}
