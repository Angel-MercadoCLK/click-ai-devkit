import type { Connection } from "./auth-store.ts";
import { SCREEN_IMAGE_MAX_BYTES } from "./screen-previews.ts";

const API = "https://api.openai.com/v1";
const MAX_CATALOG_BYTES = 2 * 1024 * 1024;
const MAX_INPUT_BYTES = 128_000;
const MAX_RESPONSE_CHARS = 16_000;
const MAX_EVENTS = 1_200;
const encoder = new TextEncoder();

export type ChatContentPart = { type: "input_text"; text: string } | { type: "input_image"; image_url: string };
/** Plain text, or for one user message the text plus exactly one redacted screen image. */
export type ChatInputMessage = { role: "user" | "assistant"; content: string | ChatContentPart[] };
export type ListedModel = { slug: string; displayName: string };
export type ProviderEvent = { type: "delta"; text: string } | { type: "completed"; text: string };

async function readBounded(response: Response, maximum: number) {
  if (!response.body) throw new Error("ChatGPT returned an empty response.");
  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let length = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      length += value.byteLength;
      if (length > maximum) {
        await reader.cancel();
        throw new Error("The ChatGPT response exceeded its safe size limit.");
      }
      chunks.push(value);
    }
  } finally {
    reader.releaseLock();
  }
  const joined = new Uint8Array(length);
  let offset = 0;
  for (const chunk of chunks) { joined.set(chunk, offset); offset += chunk.byteLength; }
  return new TextDecoder().decode(joined);
}

export async function listAvailableModels(connection: Connection, fetcher: typeof fetch = fetch): Promise<ListedModel[]> {
  const response = await fetcher(`${API}/models`, {
    headers: { authorization: `Bearer ${connection.accessToken}`, accept: "application/json" },
    cache: "no-store",
    signal: AbortSignal.timeout(12_000),
  });
  if (!response.ok) throw new Error("The ChatGPT model catalog is unavailable.");
  let payload: unknown;
  try { payload = JSON.parse(await readBounded(response, MAX_CATALOG_BYTES)); }
  catch { throw new Error("The ChatGPT model catalog was invalid or too large."); }
  if (!payload || typeof payload !== "object" || !Array.isArray((payload as { models?: unknown }).models)) {
    throw new Error("The ChatGPT model catalog was invalid.");
  }
  return (payload as { models: unknown[] }).models.flatMap((item): ListedModel[] => {
    if (!item || typeof item !== "object") return [];
    const model = item as { slug?: unknown; display_name?: unknown; visibility?: unknown };
    if (model.visibility !== "list" || typeof model.slug !== "string" || !/^[a-zA-Z0-9._:-]{1,120}$/.test(model.slug)
      || typeof model.display_name !== "string" || model.display_name.length < 1 || model.display_name.length > 120) return [];
    return [{ slug: model.slug, displayName: model.display_name }];
  }).slice(0, 500);
}

function parseSseFrame(frame: string) {
  const data: string[] = [];
  for (const line of frame.split(/\r?\n/)) {
    if (line.startsWith("data:")) data.push(line.slice(5).replace(/^ /, ""));
  }
  return data.join("\n");
}

/** Thrown when the model refused the attached screen image. Its message is safe to show; it carries no upstream text. */
export class ScreenImageRejectedError extends Error {
  constructor() {
    super("This model did not accept the screen image. Choose another model or send the message without the screen.");
  }
}

const IMAGE_DATA_URL = /^data:image\/(?:png|jpeg);base64,([A-Za-z0-9+/]+={0,2})$/;
const IMAGE_ERROR_HINT = /image|vision|modalit/i;

function decodedBytes(base64: string) {
  const padding = base64.endsWith("==") ? 2 : base64.endsWith("=") ? 1 : 0;
  return Math.floor((base64.length * 3) / 4) - padding;
}

function validContent(message: ChatInputMessage) {
  const { content } = message;
  if (typeof content === "string") return content.length >= 1 && content.length <= 16_000;
  if (message.role !== "user" || !Array.isArray(content) || content.length !== 2) return false;
  const [text, image] = content;
  if (text?.type !== "input_text" || Object.keys(text).length !== 2 || typeof text.text !== "string" || text.text.length < 1 || text.text.length > 16_000) return false;
  if (image?.type !== "input_image" || Object.keys(image).length !== 2 || typeof image.image_url !== "string") return false;
  const match = IMAGE_DATA_URL.exec(image.image_url);
  if (!match) return false;
  const size = decodedBytes(match[1]);
  return size >= 16 && size <= SCREEN_IMAGE_MAX_BYTES;
}

/** The request without image payloads: images have their own byte budget, so the 128,000-byte text budget ignores them. */
function textBudgetView(input: ChatInputMessage[]) {
  return input.map((message) => typeof message.content === "string"
    ? message
    : { ...message, content: message.content.map((part) => part.type === "input_image" ? { ...part, image_url: "" } : part) });
}

function countImages(input: ChatInputMessage[]) {
  return input.reduce((total, message) => total + (typeof message.content === "string" ? 0 : message.content.filter((part) => part.type === "input_image").length), 0);
}

function validateInput(model: string, input: ChatInputMessage[]) {
  if (!/^[a-zA-Z0-9._:-]{1,120}$/.test(model) || input.length < 1 || input.length > 40
    || input.some((message) => (message.role !== "user" && message.role !== "assistant") || !validContent(message))
    || countImages(input) > 1
    || Buffer.byteLength(JSON.stringify(textBudgetView(input)), "utf8") > MAX_INPUT_BYTES) {
    throw new Error("The message is too large or invalid.");
  }
}

/**
 * Validates the input and sends it to `POST /v1/responses` (store:false, stream:true, no tools). Returns the raw response;
 * callers decide how to read it. Shared by the chat stream and the vision probe so both build the same request.
 */
export async function requestResponses(
  connection: Connection,
  model: string,
  input: ChatInputMessage[],
  fetcher: typeof fetch = fetch,
  signal?: AbortSignal,
): Promise<Response> {
  validateInput(model, input);
  const timeout = AbortSignal.timeout(90_000);
  const upstreamSignal = signal ? AbortSignal.any([signal, timeout]) : timeout;
  return fetcher(`${API}/responses`, {
    method: "POST",
    headers: {
      authorization: `Bearer ${connection.accessToken}`,
      "content-type": "application/json",
      accept: "text/event-stream",
    },
    body: JSON.stringify({
      model,
      input: input.map(({ role, content }) => ({ role, content })),
      store: false,
      stream: true,
    }),
    cache: "no-store",
    signal: upstreamSignal,
  });
}

export async function* sseFrames(body: ReadableStream<Uint8Array>, signal?: AbortSignal): AsyncGenerator<string> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  try {
    while (true) {
      if (signal?.aborted) throw new Error("The response was cancelled.");
      const { done, value } = await reader.read();
      if (signal?.aborted) throw new Error("The response was cancelled.");
      buffer += done ? decoder.decode() : decoder.decode(value, { stream: true });
      if (buffer.length > 128_000) throw new Error("The ChatGPT stream event was too large.");
      let boundary = /\r?\n\r?\n/.exec(buffer);
      while (boundary) {
        const frame = buffer.slice(0, boundary.index);
        buffer = buffer.slice(boundary.index + boundary[0].length);
        const data = parseSseFrame(frame);
        if (data) yield data;
        boundary = /\r?\n\r?\n/.exec(buffer);
      }
      if (done) {
        if (buffer.trim()) {
          const data = parseSseFrame(buffer);
          if (data) yield data;
        }
        return;
      }
    }
  } finally {
    await reader.cancel().catch(() => undefined);
    reader.releaseLock();
  }
}

export async function* streamChatResponse(
  connection: Connection,
  model: string,
  input: ChatInputMessage[],
  fetcher: typeof fetch = fetch,
  signal?: AbortSignal,
): AsyncGenerator<ProviderEvent> {
  const timeout = AbortSignal.timeout(90_000);
  const upstreamSignal = signal ? AbortSignal.any([signal, timeout]) : timeout;
  const response = await requestResponses(connection, model, input, fetcher, upstreamSignal);
  if (!response.ok || !response.body) {
    // A refused screen image gets its own fixed message; the upstream body is only inspected, never forwarded.
    if (!response.ok && response.body && countImages(input) > 0 && [400, 415, 422].includes(response.status)) {
      let hint = false;
      try { hint = IMAGE_ERROR_HINT.test(await readBounded(response, 16_000)); }
      catch { /* An unreadable error body is treated as a generic failure. */ }
      if (hint) throw new ScreenImageRejectedError();
    }
    await response.body?.cancel().catch(() => undefined);
    throw new Error("ChatGPT could not start the response.");
  }
  let output = "";
  let eventCount = 0;
  let completed = false;
  for await (const data of sseFrames(response.body, upstreamSignal)) {
    if (++eventCount > MAX_EVENTS) throw new Error("The ChatGPT response exceeded its event limit.");
    let event: unknown;
    try { event = JSON.parse(data); }
    catch { throw new Error("ChatGPT returned an invalid stream event."); }
    if (!event || typeof event !== "object") throw new Error("ChatGPT returned an invalid stream event.");
    const record = event as { type?: unknown; delta?: unknown; error?: unknown; response?: { error?: unknown } };
    if (record.type === "response.output_text.delta") {
      if (typeof record.delta !== "string") throw new Error("ChatGPT returned an invalid text delta.");
      if (record.delta.length === 0) continue;
      if (output.length + record.delta.length > MAX_RESPONSE_CHARS) throw new Error("The response exceeded its safe size limit.");
      output += record.delta;
      yield { type: "delta", text: record.delta };
      continue;
    }
    if (record.type === "response.failed" || record.type === "response.incomplete" || record.type === "error") {
      // Only the error object is inspected for an image complaint, and only when an image was sent.
      if (countImages(input) > 0 && IMAGE_ERROR_HINT.test(JSON.stringify(record.error ?? record.response?.error ?? null))) throw new ScreenImageRejectedError();
      throw new Error("ChatGPT did not complete this response.");
    }
    if (record.type === "response.completed") {
      completed = true;
      if (!output.trim()) throw new Error("ChatGPT completed without a text response.");
      yield { type: "completed", text: output };
      break;
    }
  }
  if (!completed) throw new Error("The ChatGPT stream ended before completion.");
}

export function formatLocalEvent(type: "delta" | "completed" | "error", payload: { text?: string; message?: string }) {
  return encoder.encode(`event: ${type}\ndata: ${JSON.stringify(payload)}\n\n`);
}
