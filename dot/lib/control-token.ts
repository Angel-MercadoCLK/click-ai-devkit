// Per-launch control token for the routes that act on the machine (screen capture, voice, the mascot window).
//
// Origin and Host alone cannot tell the mascot page from another local process, because both can be forged and the
// loopback interface is shared with other users of the computer. The Electron main process generates a random token at
// startup, publishes it to this in-process server on `globalThis[Symbol.for("click-dot.control-token")]`, and puts it in an
// httpOnly, SameSite=Strict cookie that only the mascot window's session holds. The control routes then require the
// cookie to match. The token is never rendered into a page and no route returns it.
//
// Without a published token (plain `npm run dev`, browser only) nothing is enforced and the routes answer as before;
// a published value that is not a well-formed token fails closed.

import { createHash, timingSafeEqual } from "node:crypto";
import { cookieValue } from "./auth-http.ts";

export const CONTROL_TOKEN_KEY = Symbol.for("click-dot.control-token");
export const CONTROL_COOKIE = "click_dot_control";
/** 32 random bytes, as lowercase hex. */
export const CONTROL_TOKEN_PATTERN = /^[0-9a-f]{64}$/;

export type ControlGate = { kind: "none" } | { kind: "token"; token: string } | { kind: "invalid" };

export function readControlGate(): ControlGate {
  const published = (globalThis as Record<symbol, unknown>)[CONTROL_TOKEN_KEY];
  if (published === undefined) return { kind: "none" };
  if (typeof published === "string" && CONTROL_TOKEN_PATTERN.test(published)) return { kind: "token", token: published };
  return { kind: "invalid" };
}

/** Hashing first makes both inputs the same length, so the comparison never depends on where they differ. */
export function sameSecret(expected: string, supplied: string) {
  const digest = (value: string) => createHash("sha256").update(value).digest();
  return timingSafeEqual(digest(expected), digest(supplied));
}

/** True when the request may use a control route. Origin and Host are still checked by the route itself. */
export function hasControlAccess(request: Request): boolean {
  const gate = readControlGate();
  if (gate.kind === "none") return true;
  if (gate.kind === "invalid") return false;
  const supplied = cookieValue(request, CONTROL_COOKIE);
  return supplied !== undefined && sameSecret(gate.token, supplied);
}
