import { randomBytes } from "node:crypto";

/** Names shared with lib/control-token.ts, which reads globalThis[Symbol.for(CONTROL_TOKEN_KEY)] and the cookie. */
export const CONTROL_TOKEN_KEY = "click-dot.control-token";
export const CONTROL_COOKIE = "click_dot_control";

const TOKEN_PATTERN = /^[0-9a-f]{64}$/;

/** A fresh 256-bit token for this launch. */
export const createControlToken = () => randomBytes(32).toString("hex");

/** Publishes the token to the in-process Next server. */
export function installControlToken(target: object, token: unknown) {
  if (typeof token !== "string" || !TOKEN_PATTERN.test(token)) throw new Error("The control token is malformed.");
  Reflect.set(target, Symbol.for(CONTROL_TOKEN_KEY), token);
}

/**
 * The cookie only this app's own session holds: httpOnly (the page cannot read it), SameSite Strict, no expiry (a session
 * cookie, replaced on every launch) and no Secure flag because the app serves plain http on loopback.
 */
export function controlCookieDetails(origin: string, token: string) {
  return { url: origin, name: CONTROL_COOKIE, value: token, path: "/", httpOnly: true, sameSite: "strict" as const };
}
