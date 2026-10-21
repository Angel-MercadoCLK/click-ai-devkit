import { withCurrentConnection } from "../../../../lib/auth-store.ts";
import { authorizedConnection, isConnectionSnapshot, isRecord, noStore, readBoundedJson, rejectNonLocal } from "../../../../lib/chat-http.ts";
import { listAvailableModels, streamChatResponse, formatLocalEvent, ScreenImageRejectedError, type ChatInputMessage } from "../../../../lib/chat-provider.ts";
import { appendCompletedTurn, getConversation, listConversations } from "../../../../lib/conversation-store.ts";
import { PREVIEW_ID_PATTERN, screenPreviews, toDataUrl } from "../../../../lib/screen-previews.ts";

export const runtime = "nodejs";
export const maxDuration = 120;

/** Saved in history instead of the image, which is never stored. */
const SCREEN_ATTACHED_MARKER = "[screen attached]";
const conversationIdPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

export async function POST(request: Request) {
  const rejected = rejectNonLocal(request, true);
  if (rejected) return rejected;
  const snapshot = await authorizedConnection();
  if (!isConnectionSnapshot(snapshot)) return snapshot;

  let body: unknown;
  try { body = await readBoundedJson(request); }
  catch (error) {
    const message = error instanceof Error ? error.message : "The request body is invalid.";
    return Response.json({ error: message }, { status: 400, headers: noStore });
  }
  if (!isRecord(body) || Object.keys(body).some((key) => !["conversationId", "revision", "model", "prompt", "screenPreviewId"].includes(key))
    || typeof body.conversationId !== "string" || !conversationIdPattern.test(body.conversationId)
    || !Number.isSafeInteger(body.revision) || (body.revision as number) < 0
    || typeof body.model !== "string" || !/^[a-zA-Z0-9._:-]{1,120}$/.test(body.model)
    || typeof body.prompt !== "string" || body.prompt.trim().length === 0 || body.prompt.length > 8_000) {
    return Response.json({ error: "Send a valid message, model, and conversation." }, { status: 400, headers: noStore });
  }
  // The only way to attach a screen is the opaque id of a preview this server prepared; image bytes are never accepted.
  if (body.screenPreviewId !== undefined && (typeof body.screenPreviewId !== "string" || !PREVIEW_ID_PATTERN.test(body.screenPreviewId))) {
    return Response.json({ error: "Send a valid message, model, and conversation." }, { status: 400, headers: noStore });
  }
  const { conversationId, revision, model, prompt, screenPreviewId } = body as {
    conversationId: string; revision: number; model: string; prompt: string; screenPreviewId?: string;
  };

  let history;
  let prior;
  try {
    history = await listConversations(snapshot);
    if (history.revision !== revision) return Response.json({ error: "Conversation history changed. Reload it before sending." }, { status: 409, headers: noStore });
    prior = await getConversation(snapshot, conversationId);
    if (prior && prior.messages.length % 2 !== 0) return Response.json({ error: "This conversation has an unfinished message and cannot continue." }, { status: 409, headers: noStore });
  } catch {
    return Response.json({ error: "Local conversation history is unavailable." }, { status: 503, headers: noStore });
  }

  let models;
  try {
    models = await withCurrentConnection(snapshot, () => listAvailableModels(snapshot.connection));
  } catch {
    return Response.json({ error: "The ChatGPT model catalog is unavailable. Try again later." }, { status: 502, headers: noStore });
  }
  if (!models) return Response.json({ error: "Reconnect with ChatGPT before sending a message." }, { status: 401, headers: noStore });
  if (!models.some((candidate) => candidate.slug === model)) {
    return Response.json({ error: "Choose a model available to this ChatGPT account." }, { status: 400, headers: noStore });
  }

  const context: ChatInputMessage[] = [];
  let chars = 0;
  for (const item of (prior?.messages ?? []).slice(-20).reverse()) {
    if (chars + item.content.length > 32_000) break;
    context.unshift({ role: item.role, content: item.content });
    chars += item.content.length;
  }
  // Taken (and so spent) only after every other check passed. An unknown, used or expired id sends nothing.
  let userContent: ChatInputMessage["content"] = prompt;
  if (screenPreviewId !== undefined) {
    const screen = screenPreviews().take(screenPreviewId);
    if (!screen) return Response.json({ error: "The screen preview expired. Prepare it again before sending." }, { status: 410, headers: noStore });
    userContent = [{ type: "input_text", text: prompt }, { type: "input_image", image_url: toDataUrl(screen) }];
  }
  context.push({ role: "user", content: userContent });
  const activeConversationId = prior?.id ?? conversationId;
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
        let answer = "";
        try {
          for await (const event of streamChatResponse(snapshot.connection, model, context, fetch, signal)) {
            if (signal.aborted) return;
            if (event.type === "delta") {
              answer += event.text;
              if (!send(formatLocalEvent("delta", { text: event.text }))) return;
            } else {
              if (signal.aborted) return;
              await appendCompletedTurn(snapshot, revision, activeConversationId, model, screenPreviewId === undefined ? prompt : `${prompt}\n\n${SCREEN_ATTACHED_MARKER}`, answer, signal);
              if (signal.aborted) return;
              send(formatLocalEvent("completed", {}));
            }
          }
        } catch (error) {
          if (!signal.aborted) {
            send(formatLocalEvent("error", {
              message: error instanceof ScreenImageRejectedError
                ? error.message
                : "ChatGPT could not complete and save this response. Try again or reconnect if your session expired.",
            }));
          }
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
