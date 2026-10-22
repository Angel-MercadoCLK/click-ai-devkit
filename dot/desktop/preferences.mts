import { mkdir, readFile, rename, writeFile } from "node:fs/promises";
import { randomBytes } from "node:crypto";
import { dirname } from "node:path";

/** Top-left corner of the mascot in screen coordinates. Optional: older files and first runs have none. */
export type MascotPosition = { x: number; y: number };
export type Preferences = { startAtLogin: boolean; captureEnabled: boolean; mascotPosition?: MascotPosition };

/** Far beyond any real desktop; keeps a corrupt number from ever reaching window placement. */
const POSITION_LIMIT = 100_000;

export function isMascotPosition(value: unknown): value is MascotPosition {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const keys = Object.keys(value);
  if (keys.length !== 2 || !keys.includes("x") || !keys.includes("y")) return false;
  const { x, y } = value as Record<string, unknown>;
  return [x, y].every((n) => typeof n === "number" && Number.isInteger(n) && Math.abs(n) <= POSITION_LIMIT);
}

export const DEFAULT_PREFERENCES: Preferences = { startAtLogin: true, captureEnabled: false };
export const LOGIN_ITEM_ARGS = ["--hidden"];

const DISABLE_LOGIN_ITEM_FLAG = "--no-login-item";
const DISABLE_LOGIN_ITEM_ENV = "CLICK_DOT_DISABLE_LOGIN_ITEM";

export async function savePreferences(file: string, preferences: Preferences): Promise<Preferences> {
  const next: Preferences = {
    startAtLogin: preferences.startAtLogin === true,
    captureEnabled: preferences.captureEnabled === true,
    // Never written unless it is a valid position, so files stay as they were for users who never moved the mascot.
    ...(isMascotPosition(preferences.mascotPosition) ? { mascotPosition: { x: preferences.mascotPosition.x, y: preferences.mascotPosition.y } } : {}),
  };
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
      const { startAtLogin, captureEnabled, mascotPosition } = parsed as Partial<Preferences>;
      // Files written before the mascot existed have no position; one of the wrong shape makes the file unknown.
      if ("mascotPosition" in parsed && !isMascotPosition(mascotPosition)) return undefined;
      const position = mascotPosition ? { mascotPosition } : {};
      // Files written before screen capture existed have no captureEnabled: that means off, and they stay valid.
      if (captureEnabled === undefined) return { startAtLogin: startAtLogin as boolean, captureEnabled: false, ...position };
      if (typeof captureEnabled === "boolean") return { startAtLogin: startAtLogin as boolean, captureEnabled, ...position };
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

/**
 * Whether the mascot appears on start. A start with Windows (`--hidden`) keeps Dot in the tray until the shortcut
 * or the tray item shows the mascot; a manual start shows it.
 */
export const mascotStartup = (argv: readonly string[]): "hidden" | "shown" => (startsHidden(argv) ? "hidden" : "shown");
