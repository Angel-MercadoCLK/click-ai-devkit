// The route side of dropped and pasted files (S2). The desktop shell keeps the private copies (desktop/attachments.mts)
// and publishes the store on `globalThis[Symbol.for("click-dot.attachments")]`; this file validates what the page sends
// before it reaches the store and turns stored files into what Codex takes: `localImage` paths and prompt text.
// Without a published store (plain browser development) nothing can be attached.

import { isLocalRequest, noStore } from "./auth-http.ts";
import { hasControlAccess } from "./control-token.ts";

export const ATTACHMENTS_KEY = Symbol.for("click-dot.attachments");
export const ATTACHMENT_ID_PATTERN = /^att_[0-9a-f]{16}$/;
/** An image larger than this is refused before it is read; it matches the limits in desktop/attachments.mts. */
export const MAX_UPLOAD_BYTES = 20 * 1024 * 1024;
export const MAX_ATTACHMENTS_PER_MESSAGE = 6;

export type AttachmentKind = "image" | "file";
export type AttachmentView = { id: string; path: string; name: string; kind: AttachmentKind; bytes: number };
type SaveResult = { ok: true; attachment: AttachmentView } | { ok: false; error: string };
export type AttachmentStore = {
  save(input: { bytes: Uint8Array; name: string; kind: AttachmentKind; extension?: string }): SaveResult;
  resolve(id: string): AttachmentView | undefined;
  remove(id: string): boolean;
};

const isStore = (value: unknown): value is AttachmentStore =>
  !!value && typeof value === "object" && ["save", "resolve", "remove"].every((name) => typeof (value as Record<string, unknown>)[name] === "function");

export function getAttachmentStore(): AttachmentStore | null {
  const published = (globalThis as Record<symbol, unknown>)[ATTACHMENTS_KEY];
  return isStore(published) ? published : null;
}

const startsWith = (bytes: Uint8Array, prefix: number[], offset = 0) => bytes.length >= offset + prefix.length && prefix.every((value, index) => bytes[offset + index] === value);
const ascii = (text: string) => [...text].map((character) => character.charCodeAt(0));

/** The four image types Codex takes as `localImage`, recognized by their first bytes. SVG is never one of them. */
export function sniffImage(bytes: Uint8Array): { mime: string; extension: string } | undefined {
  if (startsWith(bytes, [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a])) return { mime: "image/png", extension: "png" };
  if (startsWith(bytes, [0xff, 0xd8, 0xff])) return { mime: "image/jpeg", extension: "jpg" };
  if (startsWith(bytes, ascii("GIF87a")) || startsWith(bytes, ascii("GIF89a"))) return { mime: "image/gif", extension: "gif" };
  if (startsWith(bytes, ascii("RIFF")) && startsWith(bytes, ascii("WEBP"), 8)) return { mime: "image/webp", extension: "webp" };
  return undefined;
}

export const localImagePaths = (attachments: readonly AttachmentView[]) => attachments.filter((item) => item.kind === "image").map((item) => item.path);

/** The user's text plus one line per non-image file, so Codex reads each with its own tools. */
export function promptWithFiles(prompt: string, attachments: readonly AttachmentView[]): string {
  const files = attachments.filter((item) => item.kind === "file");
  if (files.length === 0) return prompt;
  return `${prompt}\n\nArchivos adjuntos (lee cada uno con tus herramientas):\n${files.map((file) => `- ${file.path} (${file.name})`).join("\n")}`;
}

/** Ids from a request body, each one an id the store issued. Undefined when any is malformed, unknown or repeated. */
export function resolveAttachmentIds(store: AttachmentStore | null, ids: unknown): AttachmentView[] | undefined {
  if (ids === undefined) return [];
  if (!Array.isArray(ids) || ids.length > MAX_ATTACHMENTS_PER_MESSAGE) return undefined;
  if (ids.length > 0 && !store) return undefined;
  const found: AttachmentView[] = [];
  for (const id of ids) {
    if (typeof id !== "string" || !ATTACHMENT_ID_PATTERN.test(id) || found.some((item) => item.id === id)) return undefined;
    const attachment = store!.resolve(id);
    if (!attachment) return undefined;
    found.push(attachment);
  }
  return found;
}

// Routes ----------------------------------------------------------------------------------------------------------

const fail = (status: number, error: string) => Response.json({ error }, { status, headers: noStore });
const guarded = (request: Request) => isLocalRequest(request, true) && hasControlAccess(request);

async function readLimited(request: Request, limit: number): Promise<Uint8Array | "too-large" | "missing"> {
  const declared = Number(request.headers.get("content-length"));
  if (Number.isFinite(declared) && declared > limit) return "too-large";
  if (!request.body) return "missing";
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
        return "too-large";
      }
      chunks.push(value);
    }
  } finally {
    reader.releaseLock();
  }
  const bytes = new Uint8Array(length);
  let offset = 0;
  for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.byteLength; }
  return bytes;
}

/**
 * POST /api/attachments: the raw bytes of one file, its name in `x-file-name` (percent-encoded). An image is checked by its
 * bytes; any other file is kept as it is. Answers the id the chat names the file by.
 */
export async function handleUploadRequest(request: Request): Promise<Response> {
  if (!guarded(request)) return fail(403, "Request rejected.");
  const store = getAttachmentStore();
  if (!store) return fail(501, "Adjuntar archivos solo está disponible en la app de escritorio.");
  let name = "archivo";
  try {
    const header = request.headers.get("x-file-name");
    if (header) name = decodeURIComponent(header).slice(0, 200);
  } catch {
    return fail(400, "The file name is invalid.");
  }
  const bytes = await readLimited(request, MAX_UPLOAD_BYTES);
  if (bytes === "too-large") return fail(413, "El archivo pesa demasiado.");
  if (bytes === "missing") return fail(400, "The file is missing.");
  const image = sniffImage(bytes);
  const saved = store.save({ bytes, name, kind: image ? "image" : "file", ...(image ? { extension: image.extension } : {}) });
  if (!saved.ok) return fail(422, saved.error);
  // The page learns the id, the name and the kind. The path stays on this side.
  const { id, name: storedName, kind, bytes: size } = saved.attachment;
  return Response.json({ id, name: storedName, kind, bytes: size }, { status: 201, headers: noStore });
}

/** DELETE /api/attachments/<id>: the user took the chip away before sending. */
export async function handleRemoveRequest(request: Request, id: string): Promise<Response> {
  if (!guarded(request)) return fail(403, "Request rejected.");
  const store = getAttachmentStore();
  if (!store || !ATTACHMENT_ID_PATTERN.test(id)) return fail(404, "There is no such attachment.");
  return store.remove(id) ? Response.json({ removed: true }, { headers: noStore }) : fail(404, "There is no such attachment.");
}
