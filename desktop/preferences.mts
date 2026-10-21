import { mkdir, readFile, rename, writeFile } from "node:fs/promises";
import { randomBytes } from "node:crypto";
import { dirname } from "node:path";

export type Preferences = { startAtLogin: boolean };

export const DEFAULT_PREFERENCES: Preferences = { startAtLogin: true };
export const LOGIN_ITEM_ARGS = ["--hidden"];

const DISABLE_LOGIN_ITEM_FLAG = "--no-login-item";
const DISABLE_LOGIN_ITEM_ENV = "CLICK_DOT_DISABLE_LOGIN_ITEM";

export async function savePreferences(file: string, preferences: Preferences): Promise<Preferences> {
  const next: Preferences = { startAtLogin: preferences.startAtLogin === true };
  await mkdir(dirname(file), { recursive: true });
  const temporary = `${file}.${randomBytes(6).toString("hex")}.tmp`;
  await writeFile(temporary, `${JSON.stringify(next, null, 2)}\n`);
  await rename(temporary, file);
  return next;
}

/** Reads the saved choice. A missing file is a first run: the default (on) is recorded. */
export async function loadPreferences(file: string): Promise<Preferences> {
  let raw: string;
  try {
    raw = await readFile(file, "utf8");
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return savePreferences(file, DEFAULT_PREFERENCES);
    return { ...DEFAULT_PREFERENCES };
  }
  try {
    const parsed: unknown = JSON.parse(raw);
    if (parsed && typeof parsed === "object" && typeof (parsed as Preferences).startAtLogin === "boolean") {
      return { startAtLogin: (parsed as Preferences).startAtLogin };
    }
  } catch {
    // Fall through to the default.
  }
  return { ...DEFAULT_PREFERENCES };
}

/** Only packaged builds register a login item, and the smoke/testing switches can turn it off entirely. */
export function shouldManageLoginItem({ isPackaged, argv, env }: { isPackaged: boolean; argv: readonly string[]; env: NodeJS.ProcessEnv }) {
  return isPackaged && !argv.includes(DISABLE_LOGIN_ITEM_FLAG) && env[DISABLE_LOGIN_ITEM_ENV] !== "1";
}

export const startsHidden = (argv: readonly string[]) => argv.includes(LOGIN_ITEM_ARGS[0]);
