// Which brain answers the chat, and what the chat card tells the user about it (S1).
//
// Codex is the engine only while it is running, ready and signed in. Any other state (no Codex, an unsupported version, still
// starting, signed out, restarting) leaves the built-in loop in charge, and the status says why in Spanish so the chat card
// can explain it. The desktop shell publishes three things on `globalThis`, each read here and validated: the Codex transport
// (read by lib/codex-engine.ts), `click-dot.external-mcp` (what Chrome and computer use need) and `click-dot.system-browser`
// (the function that opens the Codex sign-in page in the user's browser).

import { getCodexEngine, type CodexAccountStatus, type CodexEngine } from "./codex-engine.ts";

export const EXTERNAL_MCP_KEY = Symbol.for("click-dot.external-mcp");
export const SYSTEM_BROWSER_KEY = Symbol.for("click-dot.system-browser");
const LOGIN_FLOW_KEY = Symbol.for("click-dot.codex-login");
/** The sign-in page stays open this long; after that the wait is cancelled. */
export const LOGIN_TIMEOUT_MS = 10 * 60_000;

export type ToolAvailability = { state: "ready" | "missing" | "unsupported"; hint: string; link?: string };
export type ExternalToolsView = { chrome: ToolAvailability; computer: ToolAvailability };
export type CodexLoginView = { state: "idle" | "pending" | "completed" | "failed"; message?: string };
export type CodexAccountView = { state: "signedIn" | "signedOut" | "unavailable"; email: string | null; plan: string | null; message?: string };
export type EngineStatus = {
  engine: "codex" | "builtin";
  codex: {
    state: "absent" | "idle" | "starting" | "ready" | "restarting" | "unavailable" | "stopped";
    message: string | null;
    version: string | null;
    account: CodexAccountView | null;
    login: CodexLoginView;
  };
  tools: ExternalToolsView | null;
};

export type EngineDeps = {
  getEngine?: () => CodexEngine | null;
  readTools?: () => ExternalToolsView | null;
  loginFlow?: LoginFlow;
};

const isRecord = (value: unknown): value is Record<string, unknown> => !!value && typeof value === "object" && !Array.isArray(value);
const isAvailability = (value: unknown): value is ToolAvailability =>
  isRecord(value) && ["ready", "missing", "unsupported"].includes(value.state as string) && typeof value.hint === "string" && (value.link === undefined || typeof value.link === "string");

/** What the shell published about Chrome and computer use, or null when it published nothing usable. */
export function readExternalTools(): ExternalToolsView | null {
  const published = (globalThis as Record<symbol, unknown>)[EXTERNAL_MCP_KEY];
  return isRecord(published) && isAvailability(published.chrome) && isAvailability(published.computer)
    ? { chrome: published.chrome, computer: published.computer }
    : null;
}

export type SystemBrowser = (url: string) => boolean;
export function getSystemBrowser(): SystemBrowser | null {
  const published = (globalThis as Record<symbol, unknown>)[SYSTEM_BROWSER_KEY];
  return typeof published === "function" ? (published as SystemBrowser) : null;
}

// Sign-in ----------------------------------------------------------------------------------------------------------

const LOGIN_FAILED = "Codex no pudo iniciar sesión con ChatGPT. Inténtalo de nuevo.";
const LOGIN_START_FAILED = "Dot no pudo iniciar el inicio de sesión de Codex.";

type LoginEngine = Pick<CodexEngine, "startLogin" | "waitForLogin">;

export type LoginFlow = {
  /** Asks Codex for the sign-in page and opens it. `opened` is false when there was no system browser (the page opens the URL itself). */
  start(engine: LoginEngine, open: SystemBrowser | null): Promise<{ ok: true; authUrl: string; opened: boolean } | { ok: false; error: string }>;
  cancel(): void;
  view(): CodexLoginView;
};

export function createLoginFlow(): LoginFlow {
  let view: CodexLoginView = { state: "idle" };
  let abort: AbortController | undefined;
  let timer: ReturnType<typeof setTimeout> | undefined;

  const stop = () => {
    clearTimeout(timer);
    timer = undefined;
    abort?.abort();
    abort = undefined;
  };

  return {
    async start(engine, open) {
      stop();
      let started: { loginId: string; authUrl: string };
      try {
        started = await engine.startLogin();
      } catch {
        view = { state: "idle" };
        return { ok: false, error: LOGIN_START_FAILED };
      }
      const mine = new AbortController();
      abort = mine;
      view = { state: "pending" };
      timer = setTimeout(() => mine.abort(), LOGIN_TIMEOUT_MS);
      timer.unref?.();
      void engine.waitForLogin(started.loginId, mine.signal).then((outcome) => {
        // A newer sign-in (or a cancel) owns the view now.
        if (abort !== mine) return;
        clearTimeout(timer);
        timer = undefined;
        abort = undefined;
        view = outcome.success ? { state: "completed" } : outcome.error === "cancelled" ? { state: "idle" } : { state: "failed", message: LOGIN_FAILED };
      });
      let opened = false;
      try {
        opened = open ? open(started.authUrl) === true : false;
      } catch {
        opened = false;
      }
      return { ok: true, authUrl: started.authUrl, opened };
    },
    cancel() {
      stop();
      view = { state: "idle" };
    },
    view: () => view,
  };
}

/** One flow per process: route files can be bundled apart, so it lives on globalThis like the other shared state. */
export function getLoginFlow(): LoginFlow {
  const holder = globalThis as Record<symbol, unknown>;
  const existing = holder[LOGIN_FLOW_KEY];
  if (isRecord(existing) && typeof existing.start === "function" && typeof existing.cancel === "function" && typeof existing.view === "function") return existing as unknown as LoginFlow;
  const created = createLoginFlow();
  holder[LOGIN_FLOW_KEY] = created;
  return created;
}

// Selection and status --------------------------------------------------------------------------------------------

export type SelectedEngine = { kind: "codex"; engine: CodexEngine; account: Extract<CodexAccountStatus, { state: "signedIn" }> };

/** The Codex engine when it can take the turn right now (ready and signed in), otherwise null. */
export async function selectEngine(deps: EngineDeps = {}): Promise<SelectedEngine | null> {
  const engine = (deps.getEngine ?? getCodexEngine)();
  if (!engine || engine.status().state !== "ready") return null;
  const account = await engine.accountStatus();
  return account.state === "signedIn" ? { kind: "codex", engine, account } : null;
}

export async function engineStatus(deps: EngineDeps = {}): Promise<EngineStatus> {
  const engine = (deps.getEngine ?? getCodexEngine)();
  const tools = (deps.readTools ?? readExternalTools)();
  const login = (deps.loginFlow ?? getLoginFlow()).view();
  if (!engine) return { engine: "builtin", codex: { state: "absent", message: null, version: null, account: null, login }, tools };
  const status = engine.status();
  const base = { state: status.state, message: status.message ?? null, version: status.version ?? null, login };
  if (status.state !== "ready") return { engine: "builtin", codex: { ...base, account: null }, tools };
  const account = await engine.accountStatus();
  const view: CodexAccountView =
    account.state === "signedIn"
      ? { state: "signedIn", email: account.email, plan: account.plan }
      : account.state === "signedOut"
        ? { state: "signedOut", email: null, plan: null }
        : { state: "unavailable", email: null, plan: null, message: account.message };
  return { engine: account.state === "signedIn" ? "codex" : "builtin", codex: { ...base, account: view }, tools };
}
