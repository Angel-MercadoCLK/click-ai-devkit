import { randomBytes } from "node:crypto";
import {
  DEFAULT_LIMITS,
  checkDirectory,
  createKillTree,
  defaultSpawn,
  defaultStat,
  defaultTimers,
  type KillTree,
  type ToolLimits,
  type ToolSpawn,
  type ToolStat,
  type ToolTimers,
} from "./tool-process.mts";
import {
  DEFAULT_INTERRUPT_GRACE_MS,
  createClaudeRunner,
  createWhichClaude,
  type ClaudeRequest,
  type ClaudeResult,
  type WhichClaude,
} from "./tool-claude.mts";

// Background Claude Code tasks (S5). `delegate_to_claude` returns a task id at once and the run goes on in the
// background; this registry owns every task's state, its latest progress lines and its final result. The run itself is
// the existing Claude Code runner (tool-claude.mts): task text on stdin, stream-json parsing, tree kill. This module only
// adds the bookkeeping: concurrency cap, bounded history, a 30 minute timeout, and SIGINT-then-kill cancellation.

/** Name of the read-only task list on globalThis, for the island to show background Claude sessions. */
export const CLAUDE_TASKS_KEY = "click-dot.claude-tasks";

export const CLAUDE_TASK_TIMEOUT_MS = 30 * 60_000;
export const MAX_CONCURRENT_TASKS = 3;
export const MAX_STORED_TASKS = 50;
export const PROGRESS_LINES_KEPT = 10;
export const MAX_TASK_CHARS = 100_000;
export const TASK_ID_PATTERN = /^task_[0-9a-f]{12}$/;

export type ClaudeTaskState = "running" | "done" | "failed" | "cancelled" | "timed_out";

export type ClaudeTaskStatus = {
  taskId: string;
  state: ClaudeTaskState;
  projectDir: string;
  elapsedMs: number;
  progress: string[];
  /** Why the task did not finish, when it did not. */
  error?: string;
};

export type ClaudeTaskResult = {
  taskId: string;
  state: Exclude<ClaudeTaskState, "running">;
  ok: boolean;
  result: string;
  truncated: boolean;
  sessionId: string | null;
  costUsd: number | null;
  permissionDenials: number;
  error?: string;
};

/** What the island may read: no way to start, change or stop a task from here. */
export type ClaudeTaskSummary = { id: string; project: string; state: ClaudeTaskState; latestLine: string | null; startedAt: number };
export type ClaudeTaskList = { list(): ClaudeTaskSummary[] };

export type Outcome<T> = ({ ok: true } & T) | { ok: false; error: string };

export type ClaudeTaskRegistry = ClaudeTaskList & {
  start(input: { task: unknown; projectDir: unknown }): Promise<Outcome<{ taskId: string }>>;
  status(taskId: string): ClaudeTaskStatus | undefined;
  /** Undefined for an unknown id, `running` while the task is still going. */
  result(taskId: string): ClaudeTaskResult | "running" | undefined;
  /** Undefined for an unknown id; otherwise whether a running task was asked to stop. */
  cancel(taskId: string): { stopping: boolean; state: ClaudeTaskState } | undefined;
  /** Stops every running task at once (the app is quitting). */
  dispose(): void;
};

export type ClaudeTaskRegistryOptions = {
  /** Replaces the Claude Code runner (tests). By default one is built from the process dependencies below. */
  runner?: (request: ClaudeRequest) => Promise<ClaudeResult>;
  spawn?: ToolSpawn;
  platform?: NodeJS.Platform;
  env?: NodeJS.ProcessEnv;
  stat?: ToolStat;
  timers?: ToolTimers;
  killTree?: KillTree;
  whichClaude?: WhichClaude;
  limits?: Partial<ToolLimits>;
  /** Overrides the Windows default of an immediate tree kill on cancel (see `ClaudeDeps.gracefulInterrupt`). */
  gracefulInterrupt?: boolean;
  now?: () => number;
  newId?: () => string;
  maxConcurrent?: number;
  maxStored?: number;
  interruptGraceMs?: number;
};

type Task = {
  id: string;
  projectDir: string;
  state: ClaudeTaskState;
  startedAt: number;
  endedAt: number | null;
  progress: string[];
  error?: string;
  result: ClaudeResult | null;
  cancelRequested: boolean;
  abort: AbortController;
  interrupt: AbortController;
};

const defaultNewId = () => `task_${randomBytes(6).toString("hex")}`;

export function createClaudeTaskRegistry(options: ClaudeTaskRegistryOptions = {}): ClaudeTaskRegistry {
  const platform = options.platform ?? process.platform;
  const env = options.env ?? process.env;
  const stat = options.stat ?? defaultStat;
  const timers = options.timers ?? defaultTimers;
  const spawn = options.spawn ?? defaultSpawn;
  const now = options.now ?? Date.now;
  const newId = options.newId ?? defaultNewId;
  const maxConcurrent = options.maxConcurrent ?? MAX_CONCURRENT_TASKS;
  const maxStored = options.maxStored ?? MAX_STORED_TASKS;
  const graceMs = options.interruptGraceMs ?? DEFAULT_INTERRUPT_GRACE_MS;
  const limits: ToolLimits = { ...DEFAULT_LIMITS, claudeTimeoutMs: CLAUDE_TASK_TIMEOUT_MS, ...options.limits };
  const killTree = options.killTree ?? createKillTree(spawn, env, platform);
  const runner =
    options.runner ??
    createClaudeRunner({
      spawn,
      env,
      platform,
      stat,
      timers,
      killTree,
      whichClaude: options.whichClaude ?? createWhichClaude({ spawn, env, timers, killTree }),
      limits,
      ...(options.gracefulInterrupt === undefined ? {} : { gracefulInterrupt: options.gracefulInterrupt }),
    });

  const tasks = new Map<string, Task>();
  const running = () => [...tasks.values()].filter((task) => task.state === "running").length;

  /** Makes room for one more task by dropping the oldest finished ones. Running tasks are never dropped. */
  function evict() {
    for (const [id, task] of tasks) {
      if (tasks.size < maxStored) return;
      if (task.state !== "running") tasks.delete(id);
    }
  }

  function settle(task: Task, outcome: ClaudeResult) {
    task.result = outcome;
    task.endedAt = now();
    if (outcome.ok) task.state = "done";
    else if (task.cancelRequested) task.state = "cancelled";
    else if (outcome.timedOut) task.state = "timed_out";
    else task.state = "failed";
    if (task.state !== "done") task.error = task.state === "cancelled" ? "Claude Code was cancelled." : outcome.error ?? "Claude Code failed.";
  }

  async function execute(task: Task, text: string) {
    let outcome: ClaudeResult;
    try {
      outcome = await runner({
        task: text,
        cwd: task.projectDir,
        signal: task.abort.signal,
        interrupt: task.interrupt.signal,
        interruptGraceMs: graceMs,
        onProgress: (line) => {
          task.progress.push(line);
          if (task.progress.length > PROGRESS_LINES_KEPT) task.progress.splice(0, task.progress.length - PROGRESS_LINES_KEPT);
        },
      });
    } catch {
      outcome = { ok: false, result: "", sessionId: null, costUsd: null, permissionDenials: 0, truncated: false, timedOut: false, error: "Claude Code could not be run." };
    }
    settle(task, outcome);
  }

  return {
    async start({ task: text, projectDir }) {
      if (typeof text !== "string" || text.trim().length === 0) return { ok: false, error: "The task is empty." };
      if (text.length > MAX_TASK_CHARS) return { ok: false, error: `The task is too long (more than ${MAX_TASK_CHARS} characters).` };
      const folderProblem = await checkDirectory(projectDir, platform, stat);
      if (folderProblem) return { ok: false, error: `project_dir must be an existing absolute directory. ${folderProblem}` };
      // Checked after the await so two simultaneous starts cannot both pass the cap.
      if (running() >= maxConcurrent) return { ok: false, error: `Too many Claude Code tasks are running (the limit is ${maxConcurrent}). Wait for one to finish or cancel one.` };
      evict();
      let id = newId();
      while (tasks.has(id)) id = newId();
      const task: Task = {
        id,
        projectDir: projectDir as string,
        state: "running",
        startedAt: now(),
        endedAt: null,
        progress: [],
        result: null,
        cancelRequested: false,
        abort: new AbortController(),
        interrupt: new AbortController(),
      };
      tasks.set(id, task);
      void execute(task, text);
      return { ok: true, taskId: id };
    },

    status(taskId) {
      const task = tasks.get(taskId);
      if (!task) return undefined;
      return {
        taskId: task.id,
        state: task.state,
        projectDir: task.projectDir,
        elapsedMs: Math.max(0, (task.endedAt ?? now()) - task.startedAt),
        progress: [...task.progress],
        ...(task.error ? { error: task.error } : {}),
      };
    },

    result(taskId) {
      const task = tasks.get(taskId);
      if (!task) return undefined;
      if (task.state === "running" || !task.result) return "running";
      const { result } = task;
      return {
        taskId: task.id,
        state: task.state,
        ok: result.ok,
        result: result.result,
        truncated: result.truncated,
        sessionId: result.sessionId,
        costUsd: result.costUsd,
        permissionDenials: result.permissionDenials,
        ...(task.error ? { error: task.error } : {}),
      };
    },

    cancel(taskId) {
      const task = tasks.get(taskId);
      if (!task) return undefined;
      if (task.state !== "running") return { stopping: false, state: task.state };
      if (!task.cancelRequested) {
        task.cancelRequested = true;
        task.interrupt.abort();
      }
      return { stopping: true, state: task.state };
    },

    list() {
      return [...tasks.values()]
        .map((task) => ({
          id: task.id,
          project: task.projectDir,
          state: task.state,
          latestLine: task.progress.at(-1) ?? null,
          startedAt: task.startedAt,
        }))
        .sort((a, b) => b.startedAt - a.startedAt);
    },

    dispose() {
      for (const task of tasks.values()) if (task.state === "running") task.abort.abort();
    },
  };
}

/** Publishes the read-only list for the in-process Next server (same bridge pattern as the other desktop services). */
export function installClaudeTasks(target: object, registry: ClaudeTaskList) {
  Reflect.set(target, Symbol.for(CLAUDE_TASKS_KEY), Object.freeze({ list: () => registry.list() }));
}
