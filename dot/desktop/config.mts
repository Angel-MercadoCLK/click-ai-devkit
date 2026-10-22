import { join } from "node:path";

/** The one place that defines where Click Dot listens. The OAuth callback URI is derived from this port. */
export const DOT_HOST = "127.0.0.1";
export const DOT_PORT = 47321;

export const TOGGLE_SHORTCUT = "CommandOrControl+Shift+Space";

export const AUTH_FILE_NAME = "auth.json";
export const CHAT_FILE_NAME = "conversations.json";

export const dotOrigin = () => `http://${DOT_HOST}:${DOT_PORT}`;
export const dotUrl = () => `${dotOrigin()}/`;
/** The mascot window's page. The home page (`dotUrl`) stays for the system browser, where sign-in finishes. */
export const mascotUrl = () => `${dotOrigin()}/mascot`;

/**
 * Prepares the environment Next.js and the storage layer read at load time.
 * Must run before the Next.js server code is imported.
 */
export function configureEnvironment(env: NodeJS.ProcessEnv, userDataDir: string) {
  env.PORT = String(DOT_PORT);
  env.NEXT_TELEMETRY_DISABLED = "1";
  env.CLICK_DOT_AUTH_FILE = join(userDataDir, AUTH_FILE_NAME);
  env.CLICK_DOT_CHAT_FILE = join(userDataDir, CHAT_FILE_NAME);
}
