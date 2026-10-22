import { createClaudeRunner, createWhichClaude, type ClaudeRequest, type ClaudeResult, type WhichClaude } from "./tool-claude.mts";
import { createClassifier, type Classification } from "./tool-classify.mts";
import {
  checkDirectory,
  createKillTree,
  DEFAULT_LIMITS,
  decodeClixml,
  defaultHomedir,
  defaultSpawn,
  defaultStat,
  defaultTimers,
  encodePowerShell,
  powershellExecutable,
  runProcess,
  type KillTree,
  type ToolLimits,
  type ToolSpawn,
  type ToolStat,
  type ToolTimers,
} from "./tool-process.mts";

// The tool runtime behind Dot's agent tools (T1). The desktop shell publishes it to the in-process Next server at
// globalThis[Symbol.for("click-dot.tool-runner")], Windows only, before the server starts. The interface below is the
// contract shared with the agent loop (lib/); this module cannot import from lib/, so the shape is repeated there.
// A runner method never throws for a command failure: it reports the failure in its result.

export const TOOL_RUNNER_KEY = "click-dot.tool-runner";

export type PowerShellRequest = { command: string; cwd: string | null; signal: AbortSignal };
export type PowerShellResult = { exitCode: number | null; stdout: string; stderr: string; truncated: boolean; timedOut: boolean };

export interface ToolRunner {
  classifyPowerShell(command: string): Promise<Classification>;
  runPowerShell(req: PowerShellRequest): Promise<PowerShellResult>;
  runClaudeCode(req: ClaudeRequest): Promise<ClaudeResult>;
}

export type { Classification, ClaudeRequest, ClaudeResult };

export type ToolRunnerOptions = {
  spawn?: ToolSpawn;
  platform?: NodeJS.Platform;
  env?: NodeJS.ProcessEnv;
  homedir?: () => string;
  stat?: ToolStat;
  timers?: ToolTimers;
  killTree?: KillTree;
  /** Lists the `claude` launchers on PATH; `where.exe claude` by default. */
  whichClaude?: WhichClaude;
  limits?: Partial<ToolLimits>;
};

/**
 * The script PowerShell runs for a command. It sets UTF-8 output (so accents and symbols survive the pipe), runs the command
 * inside try/catch, and finishes with the command's real exit code: the last native command's $LASTEXITCODE when it is
 * non-zero, otherwise 0 or 1 by whether the last statement succeeded. A command that calls `exit N` ends the script itself.
 * The command text starts on its own line and is followed by a newline, so a trailing comment cannot swallow the footer.
 */
export function buildPowerShellScript(command: string) {
  return [
    "$ProgressPreference = 'SilentlyContinue'",
    "[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)",
    "$OutputEncoding = [Console]::OutputEncoding",
    "$global:LASTEXITCODE = 0",
    "$__ok = $true",
    "try {",
    command,
    "$__ok = $?",
    "} catch {",
    "[Console]::Error.WriteLine($_.Exception.Message)",
    "$__ok = $false",
    "}",
    "if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }",
    "if ($__ok) { exit 0 } else { exit 1 }",
    "",
  ].join("\n");
}

const refusal = (stderr: string): PowerShellResult => ({ exitCode: null, stdout: "", stderr, truncated: false, timedOut: false });

export function createToolRunner(options: ToolRunnerOptions = {}): ToolRunner {
  const spawn = options.spawn ?? defaultSpawn;
  const platform = options.platform ?? process.platform;
  const env = options.env ?? process.env;
  const homedir = options.homedir ?? defaultHomedir;
  const stat = options.stat ?? defaultStat;
  const timers = options.timers ?? defaultTimers;
  const limits: ToolLimits = { ...DEFAULT_LIMITS, ...options.limits };
  const killTree = options.killTree ?? createKillTree(spawn, env, platform);
  const executable = powershellExecutable(env, platform);
  const whichClaude = options.whichClaude ?? createWhichClaude({ spawn, env, timers, killTree });

  const classify = createClassifier({
    spawn,
    executable,
    env,
    platform,
    timers,
    killTree,
    timeoutMs: limits.classifyTimeoutMs,
    maxOutputBytes: limits.streamBytes,
    maxCommandChars: limits.maxCommandChars,
  });
  const runClaude = createClaudeRunner({ spawn, env, platform, stat, timers, killTree, whichClaude, limits });

  return {
    async classifyPowerShell(command) {
      if (platform !== "win32") return { readOnly: false, reason: "PowerShell tools are only available on Windows." };
      return classify(command);
    },

    async runPowerShell(request) {
      try {
        if (platform !== "win32") return refusal("PowerShell tools are only available on Windows.");
        const { command, cwd, signal } = request;
        if (typeof command !== "string" || command.trim().length === 0) return refusal("The command is empty.");
        if (command.includes("\u0000")) return refusal("The command contains an invalid character.");
        if (command.length > limits.maxCommandChars) return refusal(`The command is too long (more than ${limits.maxCommandChars} characters).`);
        const folder = cwd === null || cwd === undefined ? homedir() : cwd;
        const folderProblem = await checkDirectory(folder, platform, stat);
        if (folderProblem) return refusal(`The working folder must be an existing absolute directory. ${folderProblem}`);

        const outcome = await runProcess({
          spawn,
          executable,
          args: ["-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encodePowerShell(buildPowerShellScript(command))],
          options: {
            cwd: folder,
            env: { ...env, GIT_PAGER: "cat", GIT_TERMINAL_PROMPT: "0" },
            windowsHide: true,
            shell: false,
            stdio: ["pipe", "pipe", "pipe"],
          },
          timeoutMs: limits.powershellTimeoutMs,
          maxStreamBytes: limits.streamBytes,
          signal,
          timers,
          killTree,
        });
        if (outcome.startFailed) return refusal("PowerShell could not be started.");
        return { exitCode: outcome.exitCode, stdout: outcome.stdout, stderr: decodeClixml(outcome.stderr), truncated: outcome.truncated, timedOut: outcome.timedOut };
      } catch {
        return refusal("PowerShell could not be run.");
      }
    },

    runClaudeCode: runClaude,
  };
}

const isToolRunner = (value: unknown): value is ToolRunner =>
  !!value && typeof value === "object"
  && typeof (value as ToolRunner).classifyPowerShell === "function"
  && typeof (value as ToolRunner).runPowerShell === "function"
  && typeof (value as ToolRunner).runClaudeCode === "function";

/** Publishes the runner to the in-process Next server (same bridge pattern as the control token and the capture controller). */
export function installToolRunner(target: object, runner: unknown) {
  if (!isToolRunner(runner)) throw new Error("The tool runner is malformed.");
  Reflect.set(target, Symbol.for(TOOL_RUNNER_KEY), runner);
}
