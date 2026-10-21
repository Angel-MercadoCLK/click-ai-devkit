import { randomBytes } from "node:crypto";

// Redacted screen images waiting to be sent with a chat message. The page asks for a preview, sees a thumbnail, and
// later names it by an opaque id; the image itself never travels back from the browser. Entries live in memory only,
// expire quickly, can be used once, and the store is bounded, so a frame cannot pile up or be replayed.

/** Same value as SCREEN_IMAGE_MAX_BYTES in desktop/screen-prep.mts (a test keeps them equal). */
export const SCREEN_IMAGE_MAX_BYTES = 4 * 1024 * 1024;
export const PREVIEW_TTL_MS = 2 * 60_000;
export const PREVIEW_MAX_ENTRIES = 4;
export const PREVIEW_ID_PATTERN = /^[A-Za-z0-9_-]{22}$/;

export type ScreenImage = { image: Uint8Array; mime: "image/png" | "image/jpeg" };
type Entry = ScreenImage & { expiresAt: number };

export type PreviewStore = {
  /** Keeps the image and returns its opaque id and expiry time. */
  put(screen: ScreenImage): { id: string; expiresAt: number };
  /** Returns the image once; an unknown, used or expired id returns undefined. */
  take(id: string): ScreenImage | undefined;
  size(): number;
  clear(): void;
};

export function createPreviewStore({
  ttlMs = PREVIEW_TTL_MS,
  maxEntries = PREVIEW_MAX_ENTRIES,
  now = Date.now,
  newId = () => randomBytes(16).toString("base64url"),
}: { ttlMs?: number; maxEntries?: number; now?: () => number; newId?: () => string } = {}): PreviewStore {
  const entries = new Map<string, Entry>();
  const purge = () => {
    const current = now();
    for (const [id, entry] of entries) if (entry.expiresAt <= current) entries.delete(id);
  };
  return {
    put(screen) {
      purge();
      // Oldest first: a Map iterates in insertion order.
      while (entries.size >= maxEntries) entries.delete(entries.keys().next().value as string);
      let id = newId();
      while (entries.has(id)) id = newId();
      const expiresAt = now() + ttlMs;
      entries.set(id, { image: screen.image, mime: screen.mime, expiresAt });
      return { id, expiresAt };
    },
    take(id) {
      purge();
      const entry = entries.get(id);
      if (!entry) return undefined;
      entries.delete(id);
      return { image: entry.image, mime: entry.mime };
    },
    size() {
      purge();
      return entries.size;
    },
    clear() {
      entries.clear();
    },
  };
}

// Next may load the preview route and the chat route as separate module instances, so the shared store is kept on
// globalThis under a Symbol.for key (the same approach as the other in-process bridges) rather than module state.
const STORE_KEY = Symbol.for("click-dot.screen-previews");

export function screenPreviews(): PreviewStore {
  const holder = globalThis as Record<symbol, unknown>;
  const existing = holder[STORE_KEY];
  if (existing && typeof existing === "object" && typeof (existing as PreviewStore).put === "function") return existing as PreviewStore;
  const created = createPreviewStore();
  holder[STORE_KEY] = created;
  return created;
}

export function toDataUrl(screen: ScreenImage) {
  return `data:${screen.mime};base64,${Buffer.from(screen.image).toString("base64")}`;
}
