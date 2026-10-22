import { access, constants, readdir } from "node:fs/promises";
import { posix, win32 } from "node:path";
import { tomlLiteral, tomlLiteralArray } from "./external-mcp-toml.mts";

// The two outside MCP servers Dot hands to Codex when the machine can run them (S6):
//
//  - `chrome`: Playwright MCP in extension mode. It drives the user's own Chrome (their sign-ins, their tabs) through the
//    "Playwright Extension"; nothing is launched or logged in separately. Needs Node's `npx`.
//  - `computer`: Windows-MCP, for the mouse, the keyboard and the screen. Needs `uv` (`uvx`) and Windows. Only input and
//    screen tools are enabled, the control policy is `strict` and its telemetry is off.
//
// Both are added with `-c` overrides, never by editing the user's config, and both use `default_tools_approval_mode =
// "writes"`: what only reads (a snapshot, a screenshot) is free, everything that acts asks on the island (S4).
//
// Verified against: github.com/microsoft/playwright-mcp README (`--extension`, "requires the Playwright Extension"),
// github.com/microsoft/playwright packages/extension README (Chrome Web Store id below, `PLAYWRIGHT_MCP_EXTENSION_TOKEN`),
// github.com/CursorTouch/Windows-MCP README (`serve`, `--control-policy relaxed|balanced|strict`, the tool names,
// `ANONYMIZED_TELEMETRY`), and developers.openai.com/codex/config-reference (the `mcp_servers.<id>.*` keys).

/** Pinned exact versions: Dot never runs `@latest` code with the user's browser and mouse. Bump them deliberately. */
export const PLAYWRIGHT_MCP_VERSION = "0.0.83";
export const WINDOWS_MCP_VERSION = "0.8.8";
export const CHROME_EXTENSION_URL = "https://chromewebstore.google.com/detail/playwright-extension/mmlmfjhmonkocbjadbfplnigmagldckm";

export const EXTERNAL_MCP_KEY = "click-dot.external-mcp";
export const CHROME_SERVER_NAME = "chrome";
export const COMPUTER_SERVER_NAME = "computer";

/** Input and screen tools only. App, PowerShell, FileSystem, Registry, Process, Clipboard, Scrape and Notification stay off. */
export const COMPUTER_TOOLS: readonly string[] = [
  "Click",
  "Type",
  "Scroll",
  "Move",
  "Shortcut",
  "MultiSelect",
  "MultiEdit",
  "Wait",
  "WaitFor",
  "ControlStatus",
  "DisplayInventory",
  "Screenshot",
  "Snapshot",
];

/** First start downloads the package (and, for uv, a Python): the default 10 s would fail it. */
const CHROME_STARTUP_SECONDS = 60;
const COMPUTER_STARTUP_SECONDS = 180;

export type Availability = {
  state: "ready" | "missing" | "unsupported";
  /** A short Spanish line for the chat card. */
  hint: string;
  /** Where to get what is missing, when there is a place. */
  link?: string;
};
export type ExternalMcpInfo = { chrome: Availability; computer: Availability };

type PathOptions = {
  env?: NodeJS.ProcessEnv;
  platform?: NodeJS.Platform;
  /** Replaces the file check (tests). */
  exists?: (path: string) => Promise<boolean>;
  /** Replaces the folder listing (tests). */
  listDir?: (path: string) => Promise<string[]>;
};

const defaultExists = (platform: NodeJS.Platform) => async (path: string) => {
  try {
    await access(path, platform === "win32" ? constants.F_OK : constants.X_OK);
    return true;
  } catch {
    return false;
  }
};

/** The first launcher among `names` found on PATH (with PATHEXT on Windows), or null. */
export async function findOnPath(names: readonly string[], options: PathOptions = {}): Promise<string | null> {
  const platform = options.platform ?? process.platform;
  const env = options.env ?? process.env;
  const exists = options.exists ?? defaultExists(platform);
  const windows = platform === "win32";
  const path = windows ? win32 : posix;
  const pathValue = (windows ? (env.Path ?? env.PATH) : env.PATH) ?? "";
  const folders = pathValue.split(windows ? ";" : ":").map((folder) => folder.trim().replace(/^"|"$/g, "")).filter((folder) => folder.length > 0);
  const extensions = windows ? (env.PATHEXT ?? ".COM;.EXE;.BAT;.CMD").split(";").filter(Boolean).map((extension) => extension.toLowerCase()) : [""];
  for (const name of names) {
    for (const folder of folders) {
      for (const extension of extensions) {
        const candidate = path.join(folder, `${name}${extension}`);
        if (await exists(candidate)) return candidate;
      }
    }
  }
  return null;
}

export type ExternalMcpDetection = {
  /** Values for `-c`, each safe for a Windows `.cmd` shim. */
  overrides: string[];
  /** Extra environment for the app-server process. */
  env: Record<string, string>;
  info: ExternalMcpInfo;
};

export const CHROME_EXTENSION_ID = "mmlmfjhmonkocbjadbfplnigmagldckm";

const CHROME_READY = "Chrome: listo. Dot usa tu Chrome con la extensión Playwright.";
const CHROME_INSTALL = "Chrome: instala la extensión Playwright una vez para que Dot use tu Chrome.";
const CHROME_MISSING = "Chrome: requiere Node.js (npx) en este equipo.";
const COMPUTER_READY = "Mouse y teclado: listo. Dot te pregunta antes de cada acción.";
const COMPUTER_MISSING = "Mouse y teclado: requiere uv (instala uv y reinicia Dot).";
const COMPUTER_UNSUPPORTED = "Mouse y teclado: solo disponible en Windows.";

/**
 * Whether the Playwright extension is installed in any of the user's Chrome profiles (`Default`, `Profile N`), read
 * from Chrome's own folders. It tells installed from missing; whether the extension is connected right now is only known
 * when a Chrome tool runs. Null when it cannot be told here (off Windows, no LOCALAPPDATA).
 */
export async function chromeExtensionInstalled(options: PathOptions = {}): Promise<boolean | null> {
  const platform = options.platform ?? process.platform;
  const env = options.env ?? process.env;
  const local = env.LOCALAPPDATA;
  if (platform !== "win32" || !local) return null;
  const exists = options.exists ?? defaultExists(platform);
  const listDir = options.listDir ?? ((path: string) => readdir(path));
  const userData = win32.join(local, "Google", "Chrome", "User Data");
  let profiles: string[];
  try { profiles = await listDir(userData); } catch { return false; }
  for (const profile of profiles) {
    if (profile !== "Default" && !/^Profile \d+$/.test(profile)) continue;
    if (await exists(win32.join(userData, profile, "Extensions", CHROME_EXTENSION_ID))) return true;
  }
  return false;
}

/** Looks for `npx` and `uv`/`uvx` and returns the overrides, the environment and what to tell the user. */
export async function detectExternalMcp(options: PathOptions = {}): Promise<ExternalMcpDetection> {
  const platform = options.platform ?? process.platform;
  const overrides: string[] = [];
  const env: Record<string, string> = {};

  const npx = await findOnPath(["npx"], options);
  const installed = npx ? await chromeExtensionInstalled(options) : false;
  const chrome: Availability = !npx
    ? { state: "missing", hint: CHROME_MISSING }
    : installed === true
      ? { state: "ready", hint: CHROME_READY }
      : installed === null
        // Cannot be told here: the server is there, the card still points at the extension.
        ? { state: "ready", hint: CHROME_INSTALL, link: CHROME_EXTENSION_URL }
        : { state: "missing", hint: CHROME_INSTALL, link: CHROME_EXTENSION_URL };
  if (npx) {
    const prefix = `mcp_servers.${CHROME_SERVER_NAME}`;
    overrides.push(
      `${prefix}.command=${tomlLiteral("npx")}`,
      `${prefix}.args=${tomlLiteralArray(["-y", `@playwright/mcp@${PLAYWRIGHT_MCP_VERSION}`, "--extension"])}`,
      `${prefix}.default_tools_approval_mode=${tomlLiteral("writes")}`,
      `${prefix}.startup_timeout_sec=${CHROME_STARTUP_SECONDS}`,
    );
  }

  let computer: Availability;
  if (platform !== "win32") {
    computer = { state: "unsupported", hint: COMPUTER_UNSUPPORTED };
  } else {
    const uv = await findOnPath(["uvx", "uv"], options);
    if (!uv) {
      computer = { state: "missing", hint: COMPUTER_MISSING, link: "https://docs.astral.sh/uv/getting-started/installation/" };
    } else {
      computer = { state: "ready", hint: COMPUTER_READY };
      const viaUvx = /uvx(\.[a-z]+)?$/i.test(uv);
      const server = ["windows-mcp@" + WINDOWS_MCP_VERSION, "serve", "--control-policy", "strict"];
      const prefix = `mcp_servers.${COMPUTER_SERVER_NAME}`;
      overrides.push(
        `${prefix}.command=${tomlLiteral(viaUvx ? "uvx" : "uv")}`,
        `${prefix}.args=${tomlLiteralArray(viaUvx ? server : ["tool", "run", ...server])}`,
        `${prefix}.enabled_tools=${tomlLiteralArray(COMPUTER_TOOLS)}`,
        // Codex hands a stdio server only a small environment, so the opt-out has to be in its own env table.
        `${prefix}.env.ANONYMIZED_TELEMETRY=${tomlLiteral("false")}`,
        `${prefix}.default_tools_approval_mode=${tomlLiteral("writes")}`,
        `${prefix}.startup_timeout_sec=${COMPUTER_STARTUP_SECONDS}`,
      );
      env.ANONYMIZED_TELEMETRY = "false";
    }
  }
  return { overrides, env, info: { chrome, computer } };
}

const isAvailability = (value: unknown): value is Availability =>
  !!value && typeof value === "object" && ["ready", "missing", "unsupported"].includes((value as Availability).state) && typeof (value as Availability).hint === "string";

/** Publishes what is available for the in-process Next server (same bridge pattern as the other desktop services). */
export function installExternalMcp(target: object, info: ExternalMcpInfo) {
  if (!info || !isAvailability(info.chrome) || !isAvailability(info.computer)) throw new Error("The external MCP information is malformed.");
  Reflect.set(target, Symbol.for(EXTERNAL_MCP_KEY), info);
}
