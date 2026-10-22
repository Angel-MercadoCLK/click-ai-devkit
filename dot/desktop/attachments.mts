import { randomBytes } from "node:crypto";
import { chmodSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

// Files the user drops or pastes on the chat. The page cannot read a file's real path (there is no preload), so it
// sends the bytes to a guarded route and this store keeps a private copy in a temp folder. Codex then reads the copy: an
// image goes in as `localImage`, any other file as its path in the prompt. Everything is deleted when Dot quits.

export const ATTACHMENTS_KEY = "click-dot.attachments";
export const MAX_FILE_BYTES = 20 * 1024 * 1024;
export const MAX_FILES = 30;
export const MAX_TOTAL_BYTES = 200 * 1024 * 1024;
const MAX_NAME = 80;
const ID_PATTERN = /^att_[0-9a-f]{16}$/;

export type AttachmentKind = "image" | "file";
export type StoredAttachment = { id: string; path: string; name: string; kind: AttachmentKind; bytes: number };
export type SaveInput = { bytes: Uint8Array; name: string; kind: AttachmentKind; /** Without the dot; images only. */ extension?: string };
export type SaveResult = { ok: true; attachment: StoredAttachment } | { ok: false; error: string };

export type AttachmentStore = {
  save(input: SaveInput): SaveResult;
  /** Only an id this store issued resolves; anything else (a path, a guess) is undefined. */
  resolve(id: string): StoredAttachment | undefined;
  remove(id: string): boolean;
  /** Deletes every copy and the folder. Nothing is written afterwards. */
  dispose(): void;
};

export type AttachmentStoreOptions = { baseDir?: string; maxFileBytes?: number; maxFiles?: number; maxTotalBytes?: number };

/** A file name that cannot leave the folder: no separators, no control characters, bounded length, the extension kept. */
export function safeFileName(name: string): string {
  // eslint-disable-next-line no-control-regex
  const flat = name.replace(/[\u0000-\u001f\u007f]/g, "").split(/[\\/]/).pop() ?? "";
  const cleaned = flat.replace(/[<>:"|?*]/g, "_").replace(/^\.+/, "").trim();
  if (cleaned === "") return "archivo";
  if (cleaned.length <= MAX_NAME) return cleaned;
  const dot = cleaned.lastIndexOf(".");
  const extension = dot > 0 && cleaned.length - dot <= 10 ? cleaned.slice(dot) : "";
  return `${cleaned.slice(0, MAX_NAME - extension.length)}${extension}`;
}

export function createAttachmentStore(options: AttachmentStoreOptions = {}): AttachmentStore {
  const baseDir = options.baseDir ?? tmpdir();
  const maxFileBytes = options.maxFileBytes ?? MAX_FILE_BYTES;
  const maxFiles = options.maxFiles ?? MAX_FILES;
  const maxTotalBytes = options.maxTotalBytes ?? MAX_TOTAL_BYTES;
  const entries = new Map<string, StoredAttachment>();
  let directory: string | undefined;
  let total = 0;
  let disposed = false;

  const refuse = (error: string): SaveResult => ({ ok: false, error });

  return {
    save(input) {
      if (disposed) return refuse("Dot se está cerrando y ya no puede recibir archivos.");
      const size = input.bytes.byteLength;
      if (size === 0) return refuse("El archivo está vacío.");
      if (size > maxFileBytes) return refuse(`El archivo pesa demasiado (el límite es ${Math.round(maxFileBytes / 1024 / 1024)} MB).`);
      if (entries.size >= maxFiles) return refuse("Ya hay demasiados archivos adjuntos en esta sesión. Reinicia Dot para soltar más.");
      if (total + size > maxTotalBytes) return refuse("No queda espacio para más archivos adjuntos en esta sesión.");
      try {
        if (!directory) {
          mkdirSync(baseDir, { recursive: true });
          directory = mkdtempSync(join(baseDir, "click-dot-attachments-"));
          chmodSync(directory, 0o700);
        }
        const id = `att_${randomBytes(8).toString("hex")}`;
        const name = safeFileName(input.name);
        const extension = input.kind === "image" && input.extension && /^[a-z0-9]{1,5}$/.test(input.extension) ? `.${input.extension}` : "";
        const file = join(directory, `${id}-${name}${name.toLowerCase().endsWith(extension) ? "" : extension}`);
        writeFileSync(file, input.bytes, { mode: 0o600, flag: "wx" });
        const attachment: StoredAttachment = { id, path: file, name, kind: input.kind, bytes: size };
        entries.set(id, attachment);
        total += size;
        return { ok: true, attachment };
      } catch {
        return refuse("Dot no pudo guardar el archivo en este equipo.");
      }
    },

    resolve(id) {
      return ID_PATTERN.test(id) ? entries.get(id) : undefined;
    },

    remove(id) {
      const found = ID_PATTERN.test(id) ? entries.get(id) : undefined;
      if (!found) return false;
      entries.delete(id);
      total -= found.bytes;
      try { rmSync(found.path, { force: true }); } catch { /* A copy that cannot be deleted goes with the folder at quit. */ }
      return true;
    },

    dispose() {
      disposed = true;
      entries.clear();
      total = 0;
      if (!directory) return;
      try { rmSync(directory, { recursive: true, force: true }); } catch { /* Nothing more can be done while quitting. */ }
      directory = undefined;
    },
  };
}

/** Publishes the store for the in-process Next server (same bridge pattern as the other desktop services). */
export function installAttachments(target: object, store: AttachmentStore) {
  Reflect.set(target, Symbol.for(ATTACHMENTS_KEY), store);
}
