import { StringDecoder } from "node:string_decoder";
import { win32 } from "node:path";
import {
  capUtf8,
  checkDirectory,
  runProcess,
  type KillTree,
  type ToolLimits,
  type ToolSpawn,
  type ToolStat,
  type ToolTimers,
} from "./tool-process.mts";

// Claude Code runner (S4). The task text goes to the process on stdin and never into argv or a shell command line, so
// nothing in it can be interpreted by cmd.exe or PowerShell. The run is `claude -p --output-format stream-json
// --verbose --permission-mode acceptEdits` in the project folder; stdout is NDJSON that this module parses as it
// arrives: tool_use blocks become short progress lines, the final `result` line becomes the tool result.

export type ClaudeRequest = {
  task: string;
  cwd: string;
  signal: AbortSignal;
  onProgress(line: string): void;
  /** Background tasks: aborting this stops the run softly (see `ProcessRequest.interrupt`); `signal` stops it at once. */
  interrupt?: AbortSignal;
  interruptGraceMs?: number;
};
export type ClaudeResult = {
  ok: boolean;
  result: string;
  sessionId: string | null;
  costUsd: number | null;
  permissionDenials: number;
  truncated: boolean;
  timedOut: boolean;
  error?: string;
};

export const CLAUDE_ARGS = ["-p", "--output-format", "stream-json", "--verbose", "--permission-mode", "acceptEdits"] as const;
export const CLAUDE_NOT_INSTALLED = "Claude Code is not installed";

const TARGET_CHARS = 60;
const MAX_LINE_CHARS = 8 * 1024 * 1024;
const MAX_STDERR_QUOTE = 200;

const failure = (error: string, extra: Partial<ClaudeResult> = {}): ClaudeResult => ({
  ok: false,
  result: "",
  sessionId: null,
  costUsd: null,
  permissionDenials: 0,
  truncated: false,
  timedOut: false,
  error,
  ...extra,
});

// Progress ------------------------------------------------------------------------------------------------------

const oneLine = (text: string) => text.replace(/[\u0000-\u001f\u007f-\u009f\u2028\u2029]+/g, " ").replace(/\s+/g, " ").trim();
const asText = (value: unknown) => (typeof value === "string" && value.length > 0 ? value : null);

function targetOf(input: unknown): string | null {
  if (!input || typeof input !== "object" || Array.isArray(input)) return null;
  const fields = input as Record<string, unknown>;
  for (const key of ["file_path", "notebook_path"]) {
    const value = asText(fields[key]);
    if (value) return oneLine(value.split(/[\\/]/).filter(Boolean).pop() ?? value).slice(0, TARGET_CHARS);
  }
  for (const key of ["command", "pattern", "url", "query", "path", "description"]) {
    const value = asText(fields[key]);
    if (value) return oneLine(value).slice(0, TARGET_CHARS);
  }
  return null;
}

/** "Claude Code: Edit foo.ts": the tool name plus a short target (a file's name, or the first 60 characters of a command). */
export function describeToolUse(name: unknown, input: unknown) {
  const tool = typeof name === "string" ? name.replace(/[^\w:.-]/g, "").slice(0, 60) : "";
  const label = tool.length > 0 ? tool : "tool";
  const target = targetOf(input);
  return target ? `Claude Code: ${label} ${target}` : `Claude Code: ${label}`;
}

// Stream parser -------------------------------------------------------------------------------------------------

type ResultLine = { text: string; isError: boolean; subtype: string | null; sessionId: string | null; costUsd: number | null; denials: number };

export type ClaudeStreamParser = {
  write(chunk: Buffer): void;
  end(): void;
  readonly initSessionId: string | null;
  readonly result: ResultLine | null;
};

export function createClaudeStreamParser(onProgress: (line: string) => void): ClaudeStreamParser {
  const decoder = new StringDecoder("utf8");
  let buffer = "";
  let discarding = false;
  let initSessionId: string | null = null;
  let result: ResultLine | null = null;

  const handle = (raw: string) => {
    const text = raw.trim();
    if (text.length === 0) return;
    let parsed: unknown;
    try {
      parsed = JSON.parse(text);
    } catch {
      return;
    }
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return;
    const message = parsed as Record<string, unknown>;
    if (message.type === "system" && message.subtype === "init") {
      initSessionId = asText(message.session_id) ?? initSessionId;
    } else if (message.type === "assistant") {
      const content = (message.message as { content?: unknown } | null | undefined)?.content;
      if (!Array.isArray(content)) return;
      for (const block of content) {
        if (!block || typeof block !== "object" || (block as { type?: unknown }).type !== "tool_use") continue;
        const { name, input } = block as { name?: unknown; input?: unknown };
        try { onProgress(describeToolUse(name, input)); } catch { /* A UI failure never breaks the run. */ }
      }
    } else if (message.type === "result") {
      const cost = message.total_cost_usd;
      result = {
        text: typeof message.result === "string" ? message.result : "",
        isError: message.is_error === true,
        subtype: asText(message.subtype),
        sessionId: asText(message.session_id),
        costUsd: typeof cost === "number" && Number.isFinite(cost) ? cost : null,
        denials: Array.isArray(message.permission_denials) ? message.permission_denials.length : 0,
      };
    }
  };

  const feed = (text: string) => {
    buffer += text;
    let newline = buffer.indexOf("\n");
    while (newline >= 0) {
      const lineText = buffer.slice(0, newline);
      buffer = buffer.slice(newline + 1);
      if (discarding) discarding = false;
      else handle(lineText);
      newline = buffer.indexOf("\n");
    }
    if (buffer.length > MAX_LINE_CHARS) {
      buffer = "";
      discarding = true;
    }
  };

  return {
    write: (chunk) => feed(decoder.write(chunk)),
    end: () => {
      feed(decoder.end());
      if (!discarding && buffer.length > 0) handle(buffer);
      buffer = "";
    },
    get initSessionId() { return initSessionId; },
    get result() { return result; },
  };
}

// Finding claude ------------------------------------------------------------------------------------------------

export type WhichClaude = () => Promise<string | string[] | null | undefined>;

type Resolved = { kind: "exe"; path: string } | { kind: "shim"; path: string };

/** Prefers a real claude.exe; falls back to a .cmd/.bat shim (npm installs). Anything else `where` lists is ignored. */
function chooseClaude(found: string | string[] | null | undefined): Resolved | null {
  const entries = (Array.isArray(found) ? found : typeof found === "string" ? found.split(/\r?\n/) : [])
    .filter((entry): entry is string => typeof entry === "string")
    .map((entry) => entry.trim())
    .filter((entry) => entry.length > 0);
  const exe = entries.find((entry) => /\.exe$/i.test(entry));
  if (exe) return { kind: "exe", path: exe };
  const shim = entries.find((entry) => /\.(cmd|bat)$/i.test(entry));
  return shim ? { kind: "shim", path: shim } : null;
}

/** `where.exe claude`, with the same child-process discipline as every other helper here. */
export function createWhichClaude(deps: { spawn: ToolSpawn; env: NodeJS.ProcessEnv; timers: ToolTimers; killTree: KillTree; timeoutMs?: number }): WhichClaude {
  const where = win32.join(deps.env.SystemRoot ?? deps.env.windir ?? "C:\\Windows", "System32", "where.exe");
  return async () => {
    const outcome = await runProcess({
      spawn: deps.spawn,
      executable: where,
      args: ["claude"],
      options: { env: deps.env, windowsHide: true, shell: false, stdio: ["pipe", "pipe", "pipe"] },
      timeoutMs: deps.timeoutMs ?? 10_000,
      maxStreamBytes: 16 * 1024,
      timers: deps.timers,
      killTree: deps.killTree,
    });
    return outcome.exitCode === 0 ? outcome.stdout : null;
  };
}

// Runner --------------------------------------------------------------------------------------------------------

export type ClaudeDeps = {
  spawn: ToolSpawn;
  env: NodeJS.ProcessEnv;
  platform: NodeJS.Platform;
  stat: ToolStat;
  timers: ToolTimers;
  killTree: KillTree;
  whichClaude: WhichClaude;
  limits: ToolLimits;
  /** SIGINT first, tree kill after a grace period. Defaults to off on Windows, where SIGINT cannot be graceful. */
  gracefulInterrupt?: boolean;
};

export const DEFAULT_INTERRUPT_GRACE_MS = 5_000;

const stderrQuote = (stderr: string) => {
  const first = stderr.split(/\r?\n/).map((part) => oneLine(part)).find((part) => part.length > 0);
  return first ? `: ${first.slice(0, MAX_STDERR_QUOTE)}` : "";
};

export function createClaudeRunner(deps: ClaudeDeps): (request: ClaudeRequest) => Promise<ClaudeResult> {
  return async (request) => {
    try {
      if (deps.platform !== "win32") return failure("Claude Code tools are only available on Windows.");
      if (request.signal.aborted) return failure("Claude Code was stopped.");
      if (typeof request.task !== "string" || request.task.trim().length === 0) return failure("The Claude Code task is empty.");
      const folderProblem = await checkDirectory(request.cwd, deps.platform, deps.stat);
      if (folderProblem) return failure(`The project folder must be an existing absolute directory. ${folderProblem}`);

      let claude: Resolved | null = null;
      try {
        claude = chooseClaude(await deps.whichClaude());
      } catch {
        claude = null;
      }
      if (!claude) return failure(CLAUDE_NOT_INSTALLED);

      let executable: string = claude.path;
      let args: string[] = [...CLAUDE_ARGS];
      let verbatim: boolean | undefined;
      if (claude.kind === "shim") {
        // cmd.exe /d /s /c strips the outer quotes of the whole line. Only constants and the shim's own path are in it; the
        // path is refused if it could end the quoting or expand a variable.
        if (/["%\u0000-\u001f]/.test(claude.path)) return failure("The Claude Code launcher path is not supported.");
        executable = deps.env.ComSpec ?? win32.join(deps.env.SystemRoot ?? deps.env.windir ?? "C:\\Windows", "System32", "cmd.exe");
        args = ["/d", "/s", "/c", `""${claude.path}" ${CLAUDE_ARGS.join(" ")}"`];
        verbatim = true;
      }

      const parser = createClaudeStreamParser((line) => request.onProgress(line));
      const outcome = await runProcess({
        spawn: deps.spawn,
        executable,
        args,
        options: {
          cwd: request.cwd,
          env: deps.env,
          windowsHide: true,
          shell: false,
          stdio: ["pipe", "pipe", "pipe"],
          ...(verbatim ? { windowsVerbatimArguments: true } : {}),
        },
        stdin: request.task,
        timeoutMs: deps.limits.claudeTimeoutMs,
        maxStreamBytes: deps.limits.streamBytes,
        signal: request.signal,
        timers: deps.timers,
        killTree: deps.killTree,
        onStdout: (chunk) => parser.write(chunk),
        collectStdout: false,
        ...(request.interrupt
          ? {
              interrupt: {
                signal: request.interrupt,
                graceMs: request.interruptGraceMs ?? DEFAULT_INTERRUPT_GRACE_MS,
                graceful: deps.gracefulInterrupt ?? deps.platform !== "win32",
              },
            }
          : {}),
      });
      if (!outcome.aborted && !outcome.timedOut) parser.end();

      const sessionId = parser.result?.sessionId ?? parser.initSessionId;
      if (outcome.aborted) return failure("Claude Code was stopped.", { sessionId });
      if (outcome.timedOut) return failure("Claude Code timed out.", { timedOut: true, sessionId });
      if (outcome.startFailed) return failure("Claude Code could not be started.");

      const final = parser.result;
      if (!final) return failure(`Claude Code ended without a result (exit code ${outcome.exitCode ?? "unknown"})${stderrQuote(outcome.stderr)}`, { sessionId });
      const capped = capUtf8(final.text, deps.limits.claudeResultBytes);
      const failed = final.isError || (final.subtype !== null && final.subtype !== "success");
      const base: ClaudeResult = {
        ok: !failed,
        result: capped.text,
        sessionId,
        costUsd: final.costUsd,
        permissionDenials: final.denials,
        truncated: capped.truncated,
        timedOut: false,
      };
      if (!failed) return base;
      return { ...base, error: final.subtype && final.subtype !== "success" ? `Claude Code stopped: ${final.subtype}` : "Claude Code reported an error." };
    } catch {
      return failure("Claude Code could not be run.");
    }
  };
}
