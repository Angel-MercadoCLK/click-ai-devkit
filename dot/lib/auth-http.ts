import { callbackUri } from "./siwc.ts";

export const PENDING_COOKIE = "click_dot_auth_attempt";
export const noStore = { "cache-control": "no-store, private", "referrer-policy": "no-referrer", "x-content-type-options": "nosniff" };

export function localOrigin() {
  return new URL(callbackUri()).origin;
}

export function isLocalRequest(request: Request, requireOrigin = false) {
  const expected = localOrigin();
  const host = request.headers.get("host");
  const origin = request.headers.get("origin");
  return host === new URL(expected).host && (!requireOrigin || origin === expected) && (!origin || origin === expected);
}

export function cookieValue(request: Request, name: string) {
  const cookieHeader = request.headers.get("cookie") ?? "";
  for (const item of cookieHeader.split(";")) {
    const [key, ...value] = item.trim().split("=");
    if (key === name) return value.join("=");
  }
  return undefined;
}

export function setPendingCookie(id: string) {
  return `${PENDING_COOKIE}=${id}; Path=/; HttpOnly; SameSite=Lax; Max-Age=600${new URL(localOrigin()).protocol === "https:" ? "; Secure" : ""}`;
}

export function clearPendingCookie() {
  return `${PENDING_COOKIE}=; Path=/; HttpOnly; SameSite=Lax; Max-Age=0${new URL(localOrigin()).protocol === "https:" ? "; Secure" : ""}`;
}
