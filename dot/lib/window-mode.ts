// Page-facing side of the mascot window. The window itself lives in the Electron main process; main publishes a small
// controller on `globalThis[Symbol.for("click-dot.window-controller")]` (the same pattern as the capture controller)
// and the `/api/window` route validates everything before it reaches it. The page never gets more than a mode name and a
// pair of integers; there is no preload and no generic IPC.

/**
 * The island's window modes. collapsed: the thin bar (nothing is happening); compact: the pill (an agent works or something
 * is new, or the pointer is on the bar); menu: sessions and quick actions; chat: the conversation; approval: the card Dot needs answered.
 */
export type WindowMode = "collapsed" | "compact" | "menu" | "chat" | "approval";
export type WindowAnchor = { horizontal: "left" | "right"; vertical: "top" | "bottom" };
export type WindowState = { mode: WindowMode; anchor: WindowAnchor };
export type WindowController = {
  state(): unknown;
  setMode(mode: WindowMode): void;
  moveTo(x: number, y: number): void;
  /** Brings the window up without taking focus, also when it is hidden in the tray. Used when Claude Code needs the user. */
  showInactive?(): void;
};

export const WINDOW_MODES: readonly WindowMode[] = ["collapsed", "compact", "menu", "chat", "approval"];
export const WINDOW_CONTROLLER_KEY = Symbol.for("click-dot.window-controller");
export const WINDOW_MAX_BODY_BYTES = 256;
/** Far beyond any real desktop; a larger number is not a window position. */
export const WINDOW_COORDINATE_LIMIT = 100_000;
export const UNAVAILABLE_WINDOW_STATE: WindowState = { mode: "collapsed", anchor: { horizontal: "right", vertical: "bottom" } };

/** `move` is still accepted so an older page does not break, but the island is fixed at the top center and ignores it. */
export type WindowRequest = { kind: "mode"; mode: WindowMode } | { kind: "move"; x: number; y: number };

const isCoordinate = (value: unknown): value is number => typeof value === "number" && Number.isInteger(value) && Math.abs(value) <= WINDOW_COORDINATE_LIMIT;

/** Accepts exactly `{ mode }` with a known mode, or exactly `{ move: { x, y } }` with two bounded integers. */
export function parseWindowRequest(value: unknown): WindowRequest | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  const keys = Object.keys(value);
  if (keys.length !== 1) return undefined;
  if (keys[0] === "mode") {
    const mode = WINDOW_MODES.find((known) => known === (value as { mode: unknown }).mode);
    return mode ? { kind: "mode", mode } : undefined;
  }
  if (keys[0] === "move") {
    const move = (value as { move: unknown }).move;
    if (!move || typeof move !== "object" || Array.isArray(move)) return undefined;
    const moveKeys = Object.keys(move);
    if (moveKeys.length !== 2 || !moveKeys.includes("x") || !moveKeys.includes("y")) return undefined;
    const { x, y } = move as { x: unknown; y: unknown };
    return isCoordinate(x) && isCoordinate(y) ? { kind: "move", x, y } : undefined;
  }
  return undefined;
}

/** Copies only the known fields out of a state value; anything malformed is rejected whole. */
export function sanitizeWindowState(value: unknown): WindowState | undefined {
  if (!value || typeof value !== "object") return undefined;
  const { mode, anchor } = value as Record<string, unknown>;
  const knownMode = WINDOW_MODES.find((candidate) => candidate === mode);
  if (!knownMode || !anchor || typeof anchor !== "object") return undefined;
  const { horizontal, vertical } = anchor as Record<string, unknown>;
  if ((horizontal !== "left" && horizontal !== "right") || (vertical !== "top" && vertical !== "bottom")) return undefined;
  return { mode: knownMode, anchor: { horizontal, vertical } };
}

export function getWindowController(): WindowController | undefined {
  const candidate = (globalThis as Record<symbol, unknown>)[WINDOW_CONTROLLER_KEY];
  if (!candidate || typeof candidate !== "object") return undefined;
  const { state, setMode, moveTo, showInactive } = candidate as Partial<WindowController>;
  if (typeof state !== "function" || typeof setMode !== "function" || typeof moveTo !== "function") return undefined;
  if (showInactive !== undefined && typeof showInactive !== "function") return undefined;
  return candidate as WindowController;
}

export function readWindowState(controller: WindowController): WindowState | undefined {
  try {
    return sanitizeWindowState(controller.state());
  } catch {
    return undefined;
  }
}

export class WindowRequestError extends Error {
  readonly status: 400 | 413;

  constructor(status: 400 | 413, message: string) {
    super(message);
    this.status = status;
  }
}

/** Reads a tiny JSON body, refusing anything larger than WINDOW_MAX_BODY_BYTES while streaming. */
export async function readWindowRequest(request: Request): Promise<WindowRequest> {
  const contentType = request.headers.get("content-type")?.split(";", 1)[0].trim().toLowerCase();
  if (contentType !== "application/json") throw new WindowRequestError(400, "A JSON request is required.");
  const declared = Number(request.headers.get("content-length"));
  if (Number.isFinite(declared) && declared > WINDOW_MAX_BODY_BYTES) throw new WindowRequestError(413, "The request is too large.");
  if (!request.body) throw new WindowRequestError(400, "The request body is missing.");
  const reader = request.body.getReader();
  const chunks: Uint8Array[] = [];
  let length = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      length += value.byteLength;
      if (length > WINDOW_MAX_BODY_BYTES) {
        await reader.cancel();
        throw new WindowRequestError(413, "The request is too large.");
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
  let parsed: unknown;
  try {
    parsed = JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(bytes));
  } catch {
    throw new WindowRequestError(400, "The request body is invalid.");
  }
  const result = parseWindowRequest(parsed);
  if (!result) throw new WindowRequestError(400, "Unknown window request.");
  return result;
}

/**
 * Clicking the island opens the menu from the thin bar or the pill, or closes whatever is open. When Claude Code is waiting
 * for the user and the card was put away, clicking the island brings the card back instead of the menu.
 */
export function nextMascotMode(mode: WindowMode, requestWaiting = false): WindowMode {
  if (mode !== "collapsed" && mode !== "compact") return "collapsed";
  return requestWaiting ? "approval" : "menu";
}
