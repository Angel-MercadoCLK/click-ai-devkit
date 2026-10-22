import { spawn as nodeSpawn } from "node:child_process";
import { stat as nodeStat } from "node:fs/promises";
import { homedir as nodeHomedir } from "node:os";
import { posix, win32 } from "node:path";

// Child-process discipline shared by the agent tool runtime (PowerShell, the read-only classifier helper, Claude Code).
// Every external effect is injectable so the whole runtime is unit-testable on any OS: spawn, platform, home directory,
// file system checks, timers, the tree kill and the limits. Nothing in here throws for a process failure; a failure is an
// outcome the caller maps to its own result.

/** The slice of a child process this runtime uses; Node's ChildProcess satisfies it. */
export type ToolChild = {
  pid?: number;
  stdin: { write(chunk: Buffer | string): unknown; end(): unknown; on(event: "error", listener: (error: Error) => void): unknown };
  stdout: { on(event: "data", listener: (chunk: Buffer) => void): unknown };
  stderr: { on(event: "data", listener: (chunk: Buffer) => void): unknown };
  on(event: "error", listener: (error: Error) => void): unknown;
  on(event: "close", listener: (code: number | null) => void): unknown;
  kill(): unknown;
};

export type ToolSpawnOptions = {
  cwd?: string;
  env?: NodeJS.ProcessEnv;
  windowsHide: true;
  shell: false;
  windowsVerbatimArguments?: boolean;
  stdio: ["pipe", "pipe", "pipe"];
};

export type ToolSpawn = (executable: string, args: string[], options: ToolSpawnOptions) => ToolChild;
export type ToolTimers = { setTimeout(callback: () => void, ms: number): unknown; clearTimeout(handle: unknown): void };
export type ToolStat = (path: string) => Promise<{ isDirectory(): boolean }>;
export type KillTree = (pid: number) => void;

export const defaultSpawn: ToolSpawn = (executable, args, options) =>
  nodeSpawn(executable, args, options) as unknown as ToolChild;

export const defaultTimers: ToolTimers = {
  setTimeout: (callback, ms) => setTimeout(callback, ms),
  clearTimeout: (handle) => clearTimeout(handle as ReturnType<typeof setTimeout>),
};

export const defaultStat: ToolStat = (path) => nodeStat(path);
export const defaultHomedir = () => nodeHomedir();

/** `taskkill /PID <pid> /T /F`: Node's kill() ends only the direct child, never the processes it started. */
export function createKillTree(spawn: ToolSpawn, env: NodeJS.ProcessEnv = process.env, platform: NodeJS.Platform = process.platform): KillTree {
  const root = env.SystemRoot ?? env.windir ?? "C:\\Windows";
  const executable = platform === "win32" ? win32.join(root, "System32", "taskkill.exe") : "taskkill";
  return (pid) => {
    try {
      const killer = spawn(executable, ["/PID", String(pid), "/T", "/F"], { windowsHide: true, shell: false, stdio: ["pipe", "pipe", "pipe"] });
      killer.on("error", () => {});
      killer.stdin.on("error", () => {});
      killer.stdout.on("data", () => {});
      killer.stderr.on("data", () => {});
      killer.stdin.end();
    } catch {
      // The tree is already gone, or taskkill is unavailable; the direct kill() below is the fallback.
    }
  };
}

export type ToolLimits = {
  /** PowerShell commands (S3). */
  powershellTimeoutMs: number;
  /** The read-only classifier helper. */
  classifyTimeoutMs: number;
  /** Claude Code tasks (S4). */
  claudeTimeoutMs: number;
  /** Per output stream. */
  streamBytes: number;
  /** The Claude Code result text. */
  claudeResultBytes: number;
  /** The longest PowerShell command accepted; -EncodedCommand has to fit in one command line. */
  maxCommandChars: number;
};

export const DEFAULT_LIMITS: ToolLimits = {
  powershellTimeoutMs: 120_000,
  classifyTimeoutMs: 10_000,
  claudeTimeoutMs: 15 * 60_000,
  streamBytes: 64 * 1024,
  claudeResultBytes: 16 * 1024,
  maxCommandChars: 8_000,
};

export type ProcessRequest = {
  spawn: ToolSpawn;
  executable: string;
  args: string[];
  options: ToolSpawnOptions;
  stdin?: Buffer | string;
  timeoutMs: number;
  maxStreamBytes: number;
  signal?: AbortSignal;
  timers: ToolTimers;
  killTree: KillTree;
  /** Receives every stdout chunk as it arrives. */
  onStdout?: (chunk: Buffer) => void;
  /** False when `onStdout` consumes the stream and nothing should be buffered. */
  collectStdout?: boolean;
};

export type ProcessOutcome = {
  exitCode: number | null;
  stdout: string;
  stderr: string;
  truncated: boolean;
  timedOut: boolean;
  aborted: boolean;
  /** The process could not be started. */
  startFailed: boolean;
};

class CappedBuffer {
  private chunks: Buffer[] = [];
  private size = 0;
  truncated = false;
  private readonly limit: number;

  constructor(limit: number) {
    this.limit = limit;
  }

  push(chunk: Buffer) {
    const room = this.limit - this.size;
    if (room <= 0) {
      this.truncated = this.truncated || chunk.length > 0;
      return;
    }
    if (chunk.length > room) {
      this.chunks.push(chunk.subarray(0, room));
      this.size += room;
      this.truncated = true;
      return;
    }
    this.chunks.push(chunk);
    this.size += chunk.length;
  }

  text() {
    return decodeUtf8(Buffer.concat(this.chunks), this.truncated);
  }
}

/** UTF-8 decode that drops a partial character left at the end by a byte cap. */
export function decodeUtf8(bytes: Buffer, capped = false) {
  let end = bytes.length;
  if (capped) {
    for (let back = 1; back <= 4 && back <= bytes.length; back += 1) {
      const byte = bytes[bytes.length - back]!;
      if ((byte & 0xc0) === 0x80) continue;
      const needed = byte >= 0xf0 ? 4 : byte >= 0xe0 ? 3 : byte >= 0xc0 ? 2 : 1;
      if (needed > back) end = bytes.length - back;
      break;
    }
  }
  return bytes.subarray(0, end).toString("utf8");
}

/** Cuts `text` to at most `maxBytes` of UTF-8 without splitting a character. */
export function capUtf8(text: string, maxBytes: number): { text: string; truncated: boolean } {
  const bytes = Buffer.from(text, "utf8");
  if (bytes.length <= maxBytes) return { text, truncated: false };
  let end = maxBytes;
  // Back up over continuation bytes so the cut lands on a character boundary.
  while (end > 0 && (bytes[end]! & 0xc0) === 0x80) end -= 1;
  return { text: bytes.subarray(0, end).toString("utf8"), truncated: true };
}

/**
 * Runs one process to completion. Timeout and abort both end the process tree and resolve immediately with what was
 * read so far; a failure to start resolves with `startFailed`. It never rejects.
 */
export function runProcess(request: ProcessRequest): Promise<ProcessOutcome> {
  const { spawn, executable, args, options, timers, killTree, signal } = request;
  const collectStdout = request.collectStdout !== false;
  return new Promise<ProcessOutcome>((resolve) => {
    const stdout = new CappedBuffer(request.maxStreamBytes);
    const stderr = new CappedBuffer(request.maxStreamBytes);
    let child: ToolChild | undefined;
    let settled = false;
    let timer: unknown;

    const outcome = (partial: Partial<ProcessOutcome>): ProcessOutcome => ({
      exitCode: null,
      stdout: stdout.text(),
      stderr: stderr.text(),
      truncated: stdout.truncated || stderr.truncated,
      timedOut: false,
      aborted: false,
      startFailed: false,
      ...partial,
    });

    const finish = (result: ProcessOutcome) => {
      if (settled) return;
      settled = true;
      if (timer !== undefined) timers.clearTimeout(timer);
      signal?.removeEventListener("abort", onAbort);
      resolve(result);
    };

    const terminate = () => {
      const pid = child?.pid;
      if (typeof pid === "number") {
        try { killTree(pid); } catch { /* Best effort; kill() below is the fallback. */ }
      }
      try { child?.kill(); } catch { /* The process is already gone. */ }
    };

    function onAbort() {
      if (settled) return;
      terminate();
      finish(outcome({ aborted: true }));
    }

    if (signal?.aborted) {
      resolve(outcome({ aborted: true }));
      return;
    }

    try {
      child = spawn(executable, args, options);
    } catch {
      resolve(outcome({ startFailed: true }));
      return;
    }
    const process = child;

    timer = timers.setTimeout(() => {
      if (settled) return;
      terminate();
      finish(outcome({ timedOut: true }));
    }, request.timeoutMs);
    signal?.addEventListener("abort", onAbort, { once: true });

    process.stdout.on("data", (chunk) => {
      if (settled) return;
      if (collectStdout) stdout.push(chunk);
      try { request.onStdout?.(chunk); } catch { /* A consumer failure never breaks the process handling. */ }
    });
    process.stderr.on("data", (chunk) => {
      if (!settled) stderr.push(chunk);
    });
    process.stdin.on("error", () => {});
    process.on("error", () => {
      if (settled) return;
      terminate();
      finish(outcome({ startFailed: true }));
    });
    process.on("close", (code) => finish(outcome({ exitCode: typeof code === "number" ? code : null })));

    try {
      if (request.stdin !== undefined) process.stdin.write(request.stdin);
      process.stdin.end();
    } catch {
      // A closed stdin is only a problem if the process needed it; its exit code will say so.
    }
  });
}

/** Windows PowerShell expects UTF-16LE base64 for -EncodedCommand (same encoding the OCR and voice helpers use). */
export function encodePowerShell(script: string) {
  return Buffer.from(script, "utf16le").toString("base64");
}

export function powershellExecutable(env: NodeJS.ProcessEnv, platform: NodeJS.Platform) {
  if (platform !== "win32") return "powershell.exe";
  return win32.join(env.SystemRoot ?? env.windir ?? "C:\\Windows", "System32", "WindowsPowerShell", "v1.0", "powershell.exe");
}

/** An absolute path for the given platform: a drive letter or UNC share on Windows, a leading slash elsewhere. */
export function isAbsoluteDirectoryPath(path: string, platform: NodeJS.Platform) {
  if (path.length === 0 || path.includes("\u0000")) return false;
  if (platform === "win32") return /^[A-Za-z]:[\\/]/.test(path) || /^\\\\[^\\/]+\\[^\\/]+/.test(path);
  return posix.isAbsolute(path);
}

/** Returns an error message when `path` is not an existing absolute directory, otherwise null. */
export async function checkDirectory(path: unknown, platform: NodeJS.Platform, stat: ToolStat): Promise<string | null> {
  if (typeof path !== "string" || !isAbsoluteDirectoryPath(path, platform)) return "The folder must be an absolute path.";
  try {
    return (await stat(path)).isDirectory() ? null : "The folder is not a directory.";
  } catch {
    return "The folder does not exist.";
  }
}

const XML_ENTITIES: Record<string, string> = { "&amp;": "&", "&lt;": "<", "&gt;": ">", "&quot;": '"', "&apos;": "'" };

/**
 * Windows PowerShell serializes everything written to a redirected stderr as CLIXML (`#< CLIXML` and an <Objs> document
 * of <S S="Error">text</S> items) when the script comes from -EncodedCommand. This turns that back into the plain text the
 * user's command wrote: the Error and Warning items, with `_xHHHH_` escapes and XML entities decoded. Anything that is
 * not CLIXML, or has no readable items, is returned unchanged.
 */
export function decodeClixml(stderr: string) {
  if (!stderr.startsWith("#< CLIXML")) return stderr;
  const items = [...stderr.matchAll(/<S S="(?:Error|Warning)">([\s\S]*?)<\/S>/g)];
  if (items.length === 0) return stderr;
  return items
    .map((item) => item[1]!
      .replace(/&(?:amp|lt|gt|quot|apos);/g, (entity) => XML_ENTITIES[entity]!)
      .replace(/_x([0-9A-Fa-f]{4})_/g, (_match, hex: string) => String.fromCharCode(Number.parseInt(hex, 16))))
    .join("");
}
