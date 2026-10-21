import { DOT_HOST, DOT_PORT, dotUrl } from "./config.mts";

function parse(value: string) {
  try {
    return new URL(value);
  } catch {
    return undefined;
  }
}

/** True only for pages served by this app's own loopback server. */
export function isAllowedNavigation(value: string) {
  const url = parse(value);
  return (
    url !== undefined &&
    url.protocol === "http:" &&
    url.hostname === DOT_HOST &&
    url.port === String(DOT_PORT) &&
    url.username === "" &&
    url.password === ""
  );
}

/** Returns the URL when it may be handed to the system browser (https only, no embedded credentials). */
export function externalUrlToOpen(value: string) {
  const url = parse(value);
  if (!url || url.protocol !== "https:" || url.username !== "" || url.password !== "") return undefined;
  return url.toString();
}

const SIGN_IN_ORIGIN = "https://auth.openai.com";

/** The Sign in with ChatGPT hand-off, which completes in the system browser. */
export const isSignInUrl = (value: string) => parse(value)?.origin === SIGN_IN_ORIGIN;

/**
 * Where the system browser should go instead of navigating this window. The OAuth callback lands in
 * the system browser, so sign-in restarts there from Dot's own page: the pending-attempt cookie set by
 * the window's session would never reach the browser that receives the callback.
 */
export function systemBrowserTarget(value: string) {
  const target = externalUrlToOpen(value);
  if (!target) return undefined;
  return isSignInUrl(target) ? dotUrl() : target;
}
