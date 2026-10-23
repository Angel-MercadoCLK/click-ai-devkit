import { StringDecoder } from "node:string_decoder";
import { win32 } from "node:path";
import {
  createKillTree,
  defaultSpawn,
  defaultTimers,
  runProcess,
  type KillTree,
  type ToolChild,
  type ToolSpawn,
  type ToolTimers,
} from "./tool-process.mts";

// The Codex engine's process: one long-lived `codex app-server` per Dot launch, spoken to over stdio as newline-delimited
// JSON-RPC (Codex's "lite" dialect: no `jsonrpc` field). This module owns the process (finding `codex`, the version check,
// the handshake, restart with backoff, clean shutdown) and the JSON-RPC plumbing (requests with ids and timeouts,
// notifications, requests from the server). It knows no Codex method: `lib/codex-engine.ts` builds on it through the
// bridge below, because desktop modules cannot import from lib/.
//
// The user's own `codex` and default CODEX_HOME are used on purpose: their login, `AGENTS.md` rules and `[mcp_servers]`
// (Engram) all apply. Dot adds its own MCP servers per launch with `-c` overrides, so no config file is ever edited.

/**
 * The one place that says which Codex versions Dot speaks to. The protocol types in lib/codex-protocol come from the lower
 * bound. There is no upper bound on purpose: Codex ships often and an update must not lock Dot out. A version newer than
 * `CODEX_PROTOCOL_VERSION` is tried, and when its handshake fails the note says Dot is the one to update.
 */
export const CODEX_SUPPORTED_RANGE = ">=0.163.0";
/** The Codex version the generated protocol types in lib/codex-protocol come from. */
export const CODEX_PROTOCOL_VERSION = "0.163.0";
const SUPPORTED_MIN = [0, 163, 0] as const;
const PROTOCOL = [0, 163, 0] as const;

export const CODEX_ENGINE_KEY = "click-dot.codex-engine";

const DEFAULT_REQUEST_TIMEOUT_MS = 30_000;
const HANDSHAKE_TIMEOUT_MS = 20_000;
const VERSION_TIMEOUT_MS = 10_000;
const STOP_GRACE_MS = 3_000;
/** Delays before restart attempts after a crash; when they run out Codex is unavailable until Dot restarts. */
export const RESTART_BACKOFF_MS: readonly number[] = [1_000, 2_000, 5_000, 10_000, 30_000];
const MAX_LINE_CHARS = 16 * 1024 * 1024;

// Versions ------------------------------------------------------------------------------------------------------

type Version = { major: number; minor: number; patch: number; prerelease: string | null };

/** The first `x.y.z[-pre]` in `codex --version` output ("codex-cli 0.163.0"). */
export function parseCodexVersion(output: string): Version | null {
  const match = /(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?/.exec(output);
  if (!match) return null;
  return { major: Number(match[1]), minor: Number(match[2]), patch: Number(match[3]), prerelease: match[4] ?? null };
}

function compare(version: Version, bound: readonly [number, number, number]) {
  for (const [index, part] of [version.major, version.minor, version.patch].entries()) {
    if (part !== bound[index]) return part < bound[index]! ? -1 : 1;
  }
  return 0;
}

/** A prerelease of the lower bound sorts below it, so it is not supported; a prerelease of anything newer is. */
export function isSupportedCodexVersion(output: string) {
  const version = parseCodexVersion(output);
  if (!version) return false;
  const order = compare(version, SUPPORTED_MIN);
  return order > 0 || (order === 0 && version.prerelease === null);
}

const isNewerThanProtocol = (seen: string) => {
  const version = parseCodexVersion(seen);
  return !!version && compare(version, PROTOCOL) > 0;
};

const MESSAGES = {
  missing: "Dot no encontró Codex en este equipo. Instálalo (npm i -g @openai/codex) e inicia sesión con ChatGPT; mientras tanto Dot usa su asistente integrado.",
  unsupported: (version: string | null) =>
    `Dot necesita Codex ${CODEX_SUPPORTED_RANGE}${version ? ` y tienes la versión ${version}` : " y no pudo leer tu versión"}. Instala esa versión de Codex; mientras tanto Dot usa su asistente integrado.`,
  failed: "Codex no pudo iniciarse. Dot usa su asistente integrado mientras tanto.",
  newer: (version: string) => `Tu versión de Codex (${version}) es más nueva que la que Dot conoce; no pudo conectarse. Actualiza Dot.`,
} as const;

// Errors --------------------------------------------------------------------------------------------------------

export class CodexRpcError extends Error {
  readonly code: number;
  readonly data: unknown;
  constructor(code: number, message: string, data?: unknown) {
    super(message);
    this.name = "CodexRpcError";
    this.code = code;
    this.data = data;
  }
}
export class CodexTimeoutError extends Error {
  constructor(method: string) {
    super(`Codex did not answer ${method} in time.`);
    this.name = "CodexTimeoutError";
  }
}
export class CodexClosedError extends Error {
  constructor(message = "The Codex app-server is not running.") {
    super(message);
    this.name = "CodexClosedError";
  }
}
export class CodexAbortError extends Error {
  constructor() {
    super("The request was cancelled.");
    this.name = "CodexAbortError";
  }
}

// Types ---------------------------------------------------------------------------------------------------------

export type CodexStartResult =
  | { ok: true; version: string; userAgent: string; codexHome: string }
  | { ok: false; reason: "missing" | "unsupported" | "failed"; message: string; version?: string };

export type CodexState = "idle" | "starting" | "ready" | "restarting" | "unavailable" | "stopped";
export type CodexStatus = { state: CodexState; message?: string; version?: string; restarts: number };
export type CodexNotification = { method: string; params: unknown };
export type CodexServerRequest = { id: string | number; method: string; params: unknown };
export type CodexRequestOptions = { timeoutMs?: number; signal?: AbortSignal };
/** Resolve with the response's `result`; throw to answer with a JSON-RPC error. */
export type CodexRequestHandler = (request: CodexServerRequest) => unknown | Promise<unknown>;
/** Lists the `codex` launchers on PATH: `where.exe codex` on Windows. Null or empty means there is none. */
export type WhichCodex = () => Promise<string | string[] | null | undefined>;

export type CodexAppServerOptions = {
  spawn?: ToolSpawn;
  platform?: NodeJS.Platform;
  /** The environment of the process, including whatever Dot adds for its MCP servers (for example a bearer token variable). */
  env?: NodeJS.ProcessEnv;
  /** Each becomes `-c <value>`. Through a `.cmd` shim they must be shell-safe (use TOML literal strings, 'like this'). */
  configOverrides?: readonly string[];
  timers?: ToolTimers;
  killTree?: KillTree;
  which?: WhichCodex;
  requestTimeoutMs?: number;
  clientVersion?: string;
};

export interface CodexTransport {
  request(method: string, params?: unknown, options?: CodexRequestOptions): Promise<unknown>;
  notify(method: string, params?: unknown): void;
  onNotification(listener: (notification: CodexNotification) => void): () => void;
  setRequestHandler(handler: CodexRequestHandler | null): void;
  status(): CodexStatus;
  onStatus(listener: (status: CodexStatus) => void): () => void;
  /** Called after the app-server came back from a crash: threads loaded in the old process are gone. */
  onRestart(listener: () => void): () => void;
}

export interface CodexAppServer extends CodexTransport {
  start(): Promise<CodexStartResult>;
  /**
   * Looks for Codex again (where it is, its version) and starts the app-server when it is valid, without restarting Dot. It
   * covers a Codex that was missing, unsupported or failed, or that gave up restarting; on a running or stopped server it does
   * nothing. Concurrent calls share one detection. Never throws; it answers with the status afterwards.
   */
  recheck(): Promise<CodexStatus>;
  stop(): Promise<void>;
}

// Finding and launching codex -----------------------------------------------------------------------------------

type Launcher = { kind: "direct"; path: string } | { kind: "shim"; path: string };

function chooseLauncher(found: string | string[] | null | undefined, platform: NodeJS.Platform): Launcher | null {
  const entries = (Array.isArray(found) ? found : typeof found === "string" ? found.split(/\r?\n/) : [])
    .filter((entry): entry is string => typeof entry === "string")
    .map((entry) => entry.trim())
    .filter((entry) => entry.length > 0);
  if (platform !== "win32") return entries[0] ? { kind: "direct", path: entries[0] } : null;
  const exe = entries.find((entry) => /\.exe$/i.test(entry));
  if (exe) return { kind: "direct", path: exe };
  const shim = entries.find((entry) => /\.(cmd|bat)$/i.test(entry));
  return shim ? { kind: "shim", path: shim } : null;
}

/** What cmd.exe cannot reinterpret: no quotes, spaces, `%`, `&`, `|`, `<`, `>`, `^`, `!` or control characters. */
export const SHIM_SAFE_ARG = /^[A-Za-z0-9_.\-:/'[\],@=+{}]+$/;

function createWhereCodex(spawn: ToolSpawn, env: NodeJS.ProcessEnv, timers: ToolTimers, killTree: KillTree): WhichCodex {
  const where = win32.join(env.SystemRoot ?? env.windir ?? "C:\\Windows", "System32", "where.exe");
  return async () => {
    const outcome = await runProcess({
      spawn,
      executable: where,
      args: ["codex"],
      options: { env, windowsHide: true, shell: false, stdio: ["pipe", "pipe", "pipe"] },
      timeoutMs: VERSION_TIMEOUT_MS,
      maxStreamBytes: 16 * 1024,
      timers,
      killTree,
    });
    return outcome.exitCode === 0 ? outcome.stdout : null;
  };
}

// Server --------------------------------------------------------------------------------------------------------

type Pending = { method: string; resolve(value: unknown): void; reject(error: Error): void; timer: unknown; detach?: () => void };
const isRecord = (value: unknown): value is Record<string, unknown> => !!value && typeof value === "object" && !Array.isArray(value);
const asText = (value: unknown, fallback: string) => (typeof value === "string" ? value : fallback);

export function createCodexAppServer(options: CodexAppServerOptions = {}): CodexAppServer {
  const spawn = options.spawn ?? defaultSpawn;
  const platform = options.platform ?? process.platform;
  const env = options.env ?? process.env;
  const timers = options.timers ?? defaultTimers;
  const killTree = options.killTree ?? createKillTree(spawn, env, platform);
  const which = options.which ?? (platform === "win32" ? createWhereCodex(spawn, env, timers, killTree) : async () => ["codex"]);
  const overrides = [...(options.configOverrides ?? [])];
  const requestTimeoutMs = options.requestTimeoutMs ?? DEFAULT_REQUEST_TIMEOUT_MS;
  const clientVersion = options.clientVersion ?? "0.1.0";

  let state: CodexState = "idle";
  let message: string | undefined;
  let version: string | undefined;
  let restarts = 0;
  let attempt = 0;
  let stopping = false;
  let connecting = false;
  let starting: Promise<CodexStartResult> | undefined;
  let launcher: Launcher | null = null;
  let child: ToolChild | undefined;
  let generation = 0;
  let nextId = 1;
  let restartTimer: unknown;
  let stopWaiters: Array<() => void> = [];
  let requestHandler: CodexRequestHandler | null = null;
  const pending = new Map<number, Pending>();
  const notificationListeners = new Set<(notification: CodexNotification) => void>();
  const statusListeners = new Set<(status: CodexStatus) => void>();
  const restartListeners = new Set<() => void>();

  const status = (): CodexStatus => ({ state, restarts, ...(message === undefined ? {} : { message }), ...(version === undefined ? {} : { version }) });
  function setState(next: CodexState, text?: string) {
    state = next;
    message = text;
    for (const listener of [...statusListeners]) {
      try { listener(status()); } catch { /* A listener failure never breaks the engine. */ }
    }
  }

  /** The command line for `codex <args>`, or a refusal reason when a shim could not carry it safely. */
  function commandFor(args: string[]): { executable: string; args: string[]; verbatim: boolean } | null {
    if (!launcher) return null;
    if (launcher.kind === "direct") return { executable: launcher.path, args, verbatim: false };
    if (/["%\u0000-\u001f]/.test(launcher.path) || !args.every((arg) => SHIM_SAFE_ARG.test(arg))) return null;
    // cmd.exe /d /s /c strips the outer quotes of the whole line. Only the shim's own path and vetted arguments are in it.
    const executable = env.ComSpec ?? win32.join(env.SystemRoot ?? env.windir ?? "C:\\Windows", "System32", "cmd.exe");
    return { executable, args: ["/d", "/s", "/c", `""${launcher.path}" ${args.join(" ")}"`], verbatim: true };
  }

  // JSON-RPC ----------------------------------------------------------------------------------------------------

  function write(value: unknown): boolean {
    if (!child) return false;
    try {
      child.stdin.write(`${JSON.stringify(value)}\n`);
      return true;
    } catch {
      return false;
    }
  }

  function settlePending(id: number, finish: (entry: Pending) => void) {
    const entry = pending.get(id);
    if (!entry) return;
    pending.delete(id);
    timers.clearTimeout(entry.timer);
    entry.detach?.();
    finish(entry);
  }

  function rejectAll(error: () => Error) {
    for (const id of [...pending.keys()]) settlePending(id, (entry) => entry.reject(error()));
  }

  function rpc(method: string, params: unknown, requestOptions: CodexRequestOptions = {}): Promise<unknown> {
    if (requestOptions.signal?.aborted) return Promise.reject(new CodexAbortError());
    const id = nextId;
    nextId += 1;
    return new Promise<unknown>((resolve, reject) => {
      const timer = timers.setTimeout(() => settlePending(id, (entry) => entry.reject(new CodexTimeoutError(method))), requestOptions.timeoutMs ?? requestTimeoutMs);
      const entry: Pending = { method, resolve, reject, timer };
      pending.set(id, entry);
      const { signal } = requestOptions;
      if (signal) {
        const onAbort = () => settlePending(id, (found) => found.reject(new CodexAbortError()));
        signal.addEventListener("abort", onAbort, { once: true });
        entry.detach = () => signal.removeEventListener("abort", onAbort);
      }
      if (!write(params === undefined ? { id, method } : { id, method, params })) settlePending(id, (found) => found.reject(new CodexClosedError()));
    });
  }

  async function answerServerRequest(request: CodexServerRequest) {
    const handler = requestHandler;
    if (!handler) {
      write({ id: request.id, error: { code: -32601, message: `Method not found: ${request.method}` } });
      return;
    }
    // A restarted Codex numbers its requests from scratch: an answer reaches only the process that asked.
    const askedBy = child;
    const askedIn = generation;
    const reply = (value: unknown) => { if (child === askedBy && generation === askedIn) write(value); };
    try {
      reply({ id: request.id, result: (await handler(request)) ?? {} });
    } catch (error) {
      reply({ id: request.id, error: { code: -32603, message: error instanceof Error ? error.message : "Internal error" } });
    }
  }

  function dispatch(line: string) {
    const text = line.trim();
    if (text.length === 0) return;
    let parsed: unknown;
    try { parsed = JSON.parse(text); } catch { return; }
    if (!isRecord(parsed)) return;
    const { id, method } = parsed;
    const hasId = typeof id === "number" || typeof id === "string";
    if (typeof method === "string") {
      if (hasId) void answerServerRequest({ id, method, params: parsed.params });
      else for (const listener of [...notificationListeners]) {
        try { listener({ method, params: parsed.params }); } catch { /* One listener failing never starves the others. */ }
      }
      return;
    }
    if (typeof id !== "number") return;
    if (isRecord(parsed.error)) {
      const error = parsed.error;
      settlePending(id, (entry) => entry.reject(new CodexRpcError(typeof error.code === "number" ? error.code : -32603, asText(error.message, "Codex reported an error."), error.data)));
    } else if ("result" in parsed) {
      settlePending(id, (entry) => entry.resolve(parsed.result));
    }
  }

  // Process lifecycle -------------------------------------------------------------------------------------------

  function terminate(target: ToolChild | undefined) {
    if (!target) return;
    if (typeof target.pid === "number") {
      try { killTree(target.pid); } catch { /* Best effort; kill() below is the fallback. */ }
    }
    try { target.kill(); } catch { /* The process is already gone. */ }
  }

  function onExit(forGeneration: number) {
    if (forGeneration !== generation) return;
    child = undefined;
    rejectAll(() => new CodexClosedError());
    if (stopping) {
      setState("stopped");
      const waiters = stopWaiters;
      stopWaiters = [];
      for (const done of waiters) done();
      return;
    }
    // While connecting, the connect attempt itself sees the failure through its rejected initialize.
    if (connecting) return;
    scheduleRestart();
  }

  /** Spawns app-server and performs the handshake. Never throws; false means no usable process. */
  async function connect(): Promise<{ ok: true; userAgent: string; codexHome: string } | { ok: false }> {
    const command = commandFor(["app-server", ...overrides.flatMap((override) => ["-c", override])]);
    if (!command) return { ok: false };
    connecting = true;
    const mine = ++generation;
    let spawned: ToolChild;
    try {
      spawned = spawn(command.executable, command.args, {
        env,
        windowsHide: true,
        shell: false,
        stdio: ["pipe", "pipe", "pipe"],
        ...(command.verbatim ? { windowsVerbatimArguments: true } : {}),
      });
    } catch {
      connecting = false;
      return { ok: false };
    }
    child = spawned;
    const decoder = new StringDecoder("utf8");
    let buffer = "";
    let discarding = false;
    spawned.stdout.on("data", (chunk) => {
      if (mine !== generation) return;
      buffer += decoder.write(chunk);
      let newline = buffer.indexOf("\n");
      while (newline >= 0) {
        const line = buffer.slice(0, newline);
        buffer = buffer.slice(newline + 1);
        if (discarding) discarding = false;
        else dispatch(line);
        newline = buffer.indexOf("\n");
      }
      if (buffer.length > MAX_LINE_CHARS) {
        buffer = "";
        discarding = true;
      }
    });
    // Drained so the pipe never fills; nothing from it is shown or stored.
    spawned.stderr.on("data", () => {});
    spawned.stdin.on("error", () => {});
    spawned.on("error", () => onExit(mine));
    spawned.on("close", () => onExit(mine));

    try {
      const info = await rpc(
        "initialize",
        { clientInfo: { name: "click-dot", title: "Click Dot", version: clientVersion }, capabilities: { experimentalApi: true, requestAttestation: false } },
        { timeoutMs: HANDSHAKE_TIMEOUT_MS },
      );
      if (mine !== generation || child !== spawned) throw new CodexClosedError();
      write({ method: "initialized" });
      connecting = false;
      const fields = isRecord(info) ? info : {};
      return { ok: true, userAgent: asText(fields.userAgent, ""), codexHome: asText(fields.codexHome, "") };
    } catch {
      connecting = false;
      if (mine === generation && child === spawned) {
        generation += 1;
        child = undefined;
        terminate(spawned);
      }
      return { ok: false };
    }
  }

  function scheduleRestart() {
    if (stopping) return;
    const delay = RESTART_BACKOFF_MS[attempt];
    if (delay === undefined) {
      setState("unavailable", MESSAGES.failed);
      return;
    }
    attempt += 1;
    setState("restarting");
    restartTimer = timers.setTimeout(() => {
      restartTimer = undefined;
      void (async () => {
        if (stopping) return;
        const result = await connect();
        if (stopping) {
          if (result.ok) terminate(child);
          return;
        }
        if (!result.ok) {
          scheduleRestart();
          return;
        }
        attempt = 0;
        restarts += 1;
        setState("ready");
        for (const listener of [...restartListeners]) {
          try { listener(); } catch { /* A listener failure never breaks the engine. */ }
        }
      })();
    }, delay);
  }

  async function startOnce(quiet = false): Promise<CodexStartResult> {
    stopping = false;
    version = undefined;
    // A recheck of a Codex that is already unavailable keeps showing its note until it knows something new.
    if (!quiet) setState("starting");
    let found: string | string[] | null | undefined;
    try { found = await which(); } catch { found = null; }
    launcher = chooseLauncher(found, platform);
    const unavailable = (reason: "missing" | "unsupported" | "failed", text: string, seen?: string): CodexStartResult => {
      setState("unavailable", text);
      return { ok: false, reason, message: text, ...(seen === undefined ? {} : { version: seen }) };
    };
    if (!launcher) return unavailable("missing", MESSAGES.missing);

    const check = commandFor(["--version"]);
    if (!check) return unavailable("failed", MESSAGES.failed);
    const outcome = await runProcess({
      spawn,
      executable: check.executable,
      args: check.args,
      options: { env, windowsHide: true, shell: false, stdio: ["pipe", "pipe", "pipe"], ...(check.verbatim ? { windowsVerbatimArguments: true } : {}) },
      timeoutMs: VERSION_TIMEOUT_MS,
      maxStreamBytes: 16 * 1024,
      timers,
      killTree,
    });
    if (outcome.timedOut) return unavailable("failed", MESSAGES.failed);
    if (outcome.startFailed || outcome.exitCode !== 0) return unavailable("missing", MESSAGES.missing);
    const parsed = parseCodexVersion(outcome.stdout);
    const seen = parsed ? `${parsed.major}.${parsed.minor}.${parsed.patch}${parsed.prerelease ? `-${parsed.prerelease}` : ""}` : undefined;
    if (!isSupportedCodexVersion(outcome.stdout)) return unavailable("unsupported", MESSAGES.unsupported(seen ?? null), seen);

    const connected = await connect();
    if (!connected.ok) return unavailable("failed", seen !== undefined && isNewerThanProtocol(seen) ? MESSAGES.newer(seen) : MESSAGES.failed, seen);
    version = seen;
    attempt = 0;
    setState("ready");
    return { ok: true, version: seen!, userAgent: connected.userAgent, codexHome: connected.codexHome };
  }

  return {
    start() {
      if (state === "ready" || state === "restarting") return Promise.resolve<CodexStartResult>({ ok: true, version: version ?? "", userAgent: "", codexHome: "" });
      starting ??= startOnce().finally(() => { starting = undefined; });
      return starting;
    },

    recheck() {
      if (state === "ready" || state === "restarting" || state === "stopped") return Promise.resolve(status());
      starting ??= startOnce(state === "unavailable").finally(() => { starting = undefined; });
      return starting.then(status, status);
    },

    async stop() {
      stopping = true;
      if (restartTimer !== undefined) {
        timers.clearTimeout(restartTimer);
        restartTimer = undefined;
      }
      const running = child;
      if (!running) {
        setState("stopped");
        return;
      }
      await new Promise<void>((resolve) => {
        stopWaiters.push(resolve);
        const grace = timers.setTimeout(() => {
          if (child === running) {
            generation += 1;
            child = undefined;
            terminate(running);
            rejectAll(() => new CodexClosedError());
            setState("stopped");
          }
          const waiters = stopWaiters;
          stopWaiters = [];
          for (const done of waiters) done();
        }, STOP_GRACE_MS);
        stopWaiters.push(() => timers.clearTimeout(grace));
        try { running.stdin.end(); } catch { /* A closed stdin means the process is already leaving. */ }
      });
    },

    status,

    request(method, params, requestOptions) {
      if (state !== "ready" || !child) return Promise.reject(new CodexClosedError(message ?? "The Codex app-server is not running."));
      return rpc(method, params, requestOptions);
    },

    notify(method, params) {
      if (state === "ready") write(params === undefined ? { method } : { method, params });
    },

    onNotification(listener) {
      notificationListeners.add(listener);
      return () => void notificationListeners.delete(listener);
    },

    setRequestHandler(handler) {
      requestHandler = handler;
    },

    onStatus(listener) {
      statusListeners.add(listener);
      return () => void statusListeners.delete(listener);
    },

    onRestart(listener) {
      restartListeners.add(listener);
      return () => void restartListeners.delete(listener);
    },
  };
}

// Bridge --------------------------------------------------------------------------------------------------------

const TRANSPORT_METHODS = ["request", "notify", "onNotification", "setRequestHandler", "status", "onStatus", "onRestart"] as const;

export const isCodexTransport = (value: unknown): value is CodexTransport =>
  isRecord(value) && TRANSPORT_METHODS.every((name) => typeof value[name] === "function");

/**
 * Publishes the Codex engine's transport to the in-process Next server at `globalThis[Symbol.for("click-dot.codex-engine")]`
 * (same bridge pattern as the tool runner). `lib/codex-engine.ts` validates it again before use.
 */
export function installCodexEngine(target: object, transport: unknown) {
  if (!isCodexTransport(transport)) throw new Error("The Codex engine is malformed.");
  Reflect.set(target, Symbol.for(CODEX_ENGINE_KEY), transport);
}

// System browser ------------------------------------------------------------------------------------------------

export const SYSTEM_BROWSER_KEY = "click-dot.system-browser";
const SIGN_IN_HOSTS: readonly string[] = ["auth.openai.com"];

/**
 * Codex's ChatGPT sign-in page, or undefined for anything else. It is its own function (and not `systemBrowserTarget`)
 * because that one swaps every auth.openai.com address for Dot's own page, which is right for Dot's sign-in and wrong here:
 * Codex's page must open exactly as Codex built it, since its callback goes to the Codex process on this computer.
 */
export function codexSignInUrl(value: unknown): string | undefined {
  if (typeof value !== "string") return undefined;
  try {
    const url = new URL(value);
    if (url.protocol !== "https:" || url.username !== "" || url.password !== "" || !SIGN_IN_HOSTS.includes(url.hostname)) return undefined;
    return url.toString();
  } catch {
    return undefined;
  }
}

/** Publishes the function the sign-in route uses to open Codex's page in the user's browser; it refuses any other address. */
export function installSystemBrowser(target: object, open: (url: string) => void) {
  Reflect.set(target, Symbol.for(SYSTEM_BROWSER_KEY), (value: unknown): boolean => {
    const url = codexSignInUrl(value);
    if (!url) return false;
    open(url);
    return true;
  });
}
