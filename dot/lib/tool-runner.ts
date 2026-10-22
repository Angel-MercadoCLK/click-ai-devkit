// The bridge to the tool runtime. The desktop shell (Windows only) publishes a ToolRunner on
// `globalThis[Symbol.for("click-dot.tool-runner")]` before the local server starts; this file only reads it. Everything
// found there is validated, and a missing or malformed value means "no tools": the chat then behaves as plain chat.

export const TOOL_RUNNER_KEY = Symbol.for("click-dot.tool-runner");

export type PowerShellClassification = { readOnly: boolean; reason: string };
export type PowerShellResult = { exitCode: number | null; stdout: string; stderr: string; truncated: boolean; timedOut: boolean };
export type ClaudeCodeResult = {
  ok: boolean;
  result: string;
  sessionId: string | null;
  costUsd: number | null;
  permissionDenials: number;
  truncated: boolean;
  timedOut: boolean;
  error?: string;
};

/** A runner method never throws for a command failure; it reports the failure in its result. */
export interface ToolRunner {
  classifyPowerShell(command: string): Promise<PowerShellClassification>;
  runPowerShell(request: { command: string; cwd: string | null; signal: AbortSignal }): Promise<PowerShellResult>;
  runClaudeCode(request: { task: string; cwd: string; signal: AbortSignal; onProgress(line: string): void }): Promise<ClaudeCodeResult>;
}

const METHODS = ["classifyPowerShell", "runPowerShell", "runClaudeCode"] as const;

/** The published runner, or null when there is none (other systems, plain development) or it is not a runner. */
export function getToolRunner(): ToolRunner | null {
  const candidate = (globalThis as Record<symbol, unknown>)[TOOL_RUNNER_KEY];
  if (!candidate || typeof candidate !== "object") return null;
  const record = candidate as Record<string, unknown>;
  return METHODS.every((name) => typeof record[name] === "function") ? (candidate as ToolRunner) : null;
}
