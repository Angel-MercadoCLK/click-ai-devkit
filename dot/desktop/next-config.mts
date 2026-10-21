import { readFile } from "node:fs/promises";
import { join } from "node:path";

const BUILD_HINT = "Run `npm run desktop:build` first.";

/**
 * Reads the config Next.js serialized at build time. Handing it to the server
 * (as `next start` standalone output does) avoids loading and transpiling
 * next.config.ts at runtime, which would pull a native compiler from the network.
 */
export async function loadBuiltNextConfig(dir: string): Promise<Record<string, unknown>> {
  const file = join(dir, ".next", "required-server-files.json");
  let parsed: unknown;
  try {
    parsed = JSON.parse(await readFile(file, "utf8"));
  } catch {
    throw new Error(`The Next.js production build was not found or is unreadable. ${BUILD_HINT}`);
  }
  const config = (parsed as { config?: unknown } | null)?.config;
  if (!config || typeof config !== "object" || Array.isArray(config)) {
    throw new Error(`The Next.js production build output is incomplete. ${BUILD_HINT}`);
  }
  return config as Record<string, unknown>;
}
