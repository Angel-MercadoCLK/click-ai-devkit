import { randomBytes } from "node:crypto";
import { readFileSync, rmSync } from "node:fs";
import { chmod, mkdir, rename, rm, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";

/** Names shared with lib/agent-hook.ts and lib/agent-broker.ts, which read globalThis[Symbol.for(...)]. */
export const AGENT_HOOK_KEY = "click-dot.agent-hook";
export const AGENT_BROKER_KEY = "click-dot.agent-broker";

/** Where the relay (`click dot-hook`) looks for the port and token; both sides honour the override. */
export const AGENT_HOOK_FILE_NAME = "agent-hook.json";
export const AGENT_HOOK_FILE_ENV = "CLICK_DOT_AGENT_HOOK_FILE";

const TOKEN_PATTERN = /^[0-9a-f]{64}$/;

/** A fresh 256-bit token for this launch. */
export const createAgentHookToken = () => randomBytes(32).toString("hex");

/** Publishes the token to the in-process Next server, which checks hook requests against it. */
export function installAgentHook(target: object, token: unknown) {
  if (typeof token !== "string" || !TOKEN_PATTERN.test(token)) throw new Error("The agent hook token is malformed.");
  Reflect.set(target, Symbol.for(AGENT_HOOK_KEY), { token });
}

export function agentHookFile(userDataDir: string, env: NodeJS.ProcessEnv) {
  const override = env[AGENT_HOOK_FILE_ENV];
  return override ? override : join(userDataDir, AGENT_HOOK_FILE_NAME);
}

/**
 * Writes the discovery file: version, port, token and pid. It appears under its final name in one step (written beside
 * it, then renamed), so the relay never reads half a file, and only its owner can read it.
 */
export async function writeAgentHookFile(file: string, details: { port: number; token: string; pid: number }) {
  await mkdir(dirname(file), { recursive: true, mode: 0o700 });
  const temporary = `${file}.${process.pid}.${randomBytes(4).toString("hex")}.tmp`;
  try {
    await writeFile(temporary, `${JSON.stringify({ version: 1, port: details.port, token: details.token, pid: details.pid })}\n`, { mode: 0o600, flag: "wx" });
    await chmod(temporary, 0o600);
    await rename(temporary, file);
  } catch (error) {
    await rm(temporary, { force: true }).catch(() => {});
    throw error;
  }
}

/**
 * Deletes the discovery file on quit, but only while it still holds this launch's token: a newer launch's file, or a
 * file that is not ours, is left alone. Synchronous because the app is exiting.
 */
export function removeAgentHookFile(file: string, token: string): boolean {
  try {
    const content = JSON.parse(readFileSync(file, "utf8")) as { token?: unknown } | null;
    if (!content || content.token !== token) return false;
    rmSync(file, { force: true });
    return true;
  } catch {
    return false;
  }
}

/** Dot is quitting: every request still held for the user is answered "no decision" so Claude Code asks in its terminal. */
export function releaseAgentRequests(target: object) {
  const broker = Reflect.get(target, Symbol.for(AGENT_BROKER_KEY)) as { releaseAll?: unknown } | undefined;
  if (typeof broker?.releaseAll !== "function") return;
  try {
    broker.releaseAll();
  } catch (error) {
    console.error("Could not release the held agent requests.", error);
  }
}
