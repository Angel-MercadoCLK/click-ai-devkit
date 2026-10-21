import { mkdir, readFile, rename, writeFile } from "node:fs/promises";
import { randomBytes } from "node:crypto";
import { dirname } from "node:path";

export type Preferences = { startAtLogin: boolean; captureEnabled: boolean };

export const DEFAULT_PREFERENCES: Preferences = { startAtLogin: true, captureEnabled: false };
export const LOGIN_ITEM_ARGS = ["--hidden"];

const DISABLE_LOGIN_ITEM_FLAG = "--no-login-item";
const DISABLE_LOGIN_ITEM_ENV = "CLICK_DOT_DISABLE_LOGIN_ITEM";

export async function savePreferences(file: string, preferences: Preferences): Promise<Preferences> {
  const next: Preferences = { startAtLogin: preferences.startAtLogin === true, captureEnabled: preferences.captureEnabled === true };
  await mkdir(dirname(file), { recursive: true });
  const temporary = `${file}.${randomBytes(6).toString("hex")}.tmp`;
  await writeFile(temporary, `${JSON.stringify(next, null, 2)}\n`);
  await rename(temporary, file);
  return next;
}

/**
 * Reads the saved choice. A missing file is a first run: the default (on) is recorded.
 * Returns undefined when the file exists but cannot be read or understood, so callers leave the OS
 * login item untouched instead of re-enabling an autostart the user may have turned off.
 */
export async function loadPreferences(file: string): Promise<Preferences | undefined> {
  let raw: string;
  try {
    raw = await readFile(file, "utf8");
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return savePreferences(file, DEFAULT_PREFERENCES);
    return undefined;
  }
  try {
    const parsed: unknown = JSON.parse(raw);
    if (parsed && typeof parsed === "object" && typeof (parsed as Preferences).startAtLogin === "boolean") {
      const { startAtLogin, captureEnabled } = parsed as Partial<Preferences>;
      // Files written before screen capture existed have no captureEnabled: that means off, and they stay valid.
      if (captureEnabled === undefined) return { startAtLogin: startAtLogin as boolean, captureEnabled: false };
      if (typeof captureEnabled === "boolean") return { startAtLogin: startAtLogin as boolean, captureEnabled };
    }
  } catch {
    // Fall through: unknown.
  }
  return undefined;
}

/**
 * The preferences to start from. When the saved file is unknown, seed start-at-login from what the OS
 * login item already does, because the next save writes the seed back and must not re-enable autostart.
 */
export function initialPreferences(loaded: Preferences | undefined, osStartAtLogin: boolean): Preferences {
  return loaded ?? { startAtLogin: osStartAtLogin, captureEnabled: false };
}

/**
 * Holds the current preferences and saves changes one at a time, so two settings changed together
 * (for example from the tray and from the page) cannot overwrite each other.
 */
export function createPreferenceStore(file: string, initial: Preferences) {
  let current = initial;
  let queue: Promise<unknown> = Promise.resolve();
  return {
    get: () => current,
    update(patch: Partial<Preferences>): Promise<Preferences> {
      const result = queue.then(async () => {
        current = await savePreferences(file, { ...current, ...patch });
        return current;
      });
      queue = result.catch(() => {});
      return result;
    },
  };
}

/** Only packaged builds register a login item, and the smoke/testing switches can turn it off entirely. */
export function shouldManageLoginItem({ isPackaged, argv, env }: { isPackaged: boolean; argv: readonly string[]; env: NodeJS.ProcessEnv }) {
  return isPackaged && !argv.includes(DISABLE_LOGIN_ITEM_FLAG) && env[DISABLE_LOGIN_ITEM_ENV] !== "1";
}

export const startsHidden = (argv: readonly string[]) => argv.includes(LOGIN_ITEM_ARGS[0]);
