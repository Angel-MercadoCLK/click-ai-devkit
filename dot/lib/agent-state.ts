// What the mascot page knows about what Dot is doing for the user: the shapes `/api/agent/state` (Claude Code sessions and
// requests) and `/api/agent/activity` (Codex turns, background Claude Code tasks, computer use) answer with, how the page
// reads them defensively, and what they mean for the mascot's look. No Node imports: the page uses this file too.

import { projectName, type AgentSessionView, type SessionState } from "./agent-sessions.ts";

export type AgentQuestion = { question: string; header: string; multiSelect: boolean; options: { label: string; description: string }[] };
/** Where a request comes from: a Claude Code hook, one of Dot's own tools asking before it acts, or the Codex engine. */
export type PendingKind = "claude-code" | "dot-tool" | "codex";
export const PENDING_KINDS: readonly PendingKind[] = ["claude-code", "dot-tool", "codex"];
/** What the card needs and nothing more: no working directory, no raw payload. */
export type PendingView = { id: string; kind: PendingKind; tool: string; summary: string; detail: string; project: string; alwaysOffered: boolean; questions: AgentQuestion[] | null };
export type AgentState = { sessions: AgentSessionView[]; pending: PendingView | null; queued: number };
export type AgentAttention = "idle" | "working" | "needs";

/** The page polls this often; the card has 2 s to be acknowledged, so a slower poll would miss the window. */
export const AGENT_POLL_MS = 500;
/** Consecutive failed polls before the page forgets what it knew. One or two failed reads change nothing. */
export const AGENT_FAILURE_LIMIT = 3;

export const EMPTY_AGENT_STATE: AgentState = { sessions: [], pending: null, queued: 0 };
export type AgentPollState = { state: AgentState; failures: number };
export type AgentPollResult = { ok: true; state: AgentState } | { ok: false };
export const INITIAL_AGENT_POLL: AgentPollState = { state: EMPTY_AGENT_STATE, failures: 0 };

const SESSION_STATES: readonly SessionState[] = ["trabajando", "te necesita", "listo"];
const isRecord = (value: unknown): value is Record<string, unknown> => !!value && typeof value === "object" && !Array.isArray(value);

function readQuestions(value: unknown): AgentQuestion[] | null | undefined {
  if (value === null) return null;
  if (!Array.isArray(value)) return undefined;
  const questions: AgentQuestion[] = [];
  for (const item of value) {
    if (!isRecord(item) || typeof item.question !== "string" || typeof item.header !== "string" || typeof item.multiSelect !== "boolean" || !Array.isArray(item.options)) return undefined;
    const options: AgentQuestion["options"] = [];
    for (const option of item.options) {
      if (!isRecord(option) || typeof option.label !== "string" || typeof option.description !== "string") return undefined;
      options.push({ label: option.label, description: option.description });
    }
    questions.push({ question: item.question, header: item.header, multiSelect: item.multiSelect, options });
  }
  return questions;
}

function readPending(value: unknown): PendingView | null | undefined {
  if (value === null) return null;
  if (!isRecord(value)) return undefined;
  const { id, tool, summary, detail, project, alwaysOffered } = value;
  const kind = PENDING_KINDS.find((candidate) => candidate === value.kind);
  const questions = readQuestions(value.questions);
  if (typeof id !== "string" || typeof tool !== "string" || typeof summary !== "string" || typeof detail !== "string" || typeof project !== "string" || typeof alwaysOffered !== "boolean") return undefined;
  if (questions === undefined || !kind) return undefined;
  return { id, kind, tool, summary, detail, project, alwaysOffered, questions };
}

/** Reads a state response body; anything malformed is not a state. */
export function readAgentState(value: unknown): AgentState | undefined {
  if (!isRecord(value) || !Array.isArray(value.sessions)) return undefined;
  const sessions: AgentSessionView[] = [];
  for (const item of value.sessions) {
    if (!isRecord(item)) return undefined;
    const { id, project, state, action, updatedAt } = item;
    const known = SESSION_STATES.find((candidate) => candidate === state);
    if (typeof id !== "string" || typeof project !== "string" || !known || (action !== null && typeof action !== "string") || typeof updatedAt !== "number") return undefined;
    sessions.push({ id, project, state: known, action, updatedAt });
  }
  const pending = readPending(value.pending);
  const queued = value.queued;
  if (pending === undefined || typeof queued !== "number" || !Number.isInteger(queued) || queued < 0) return undefined;
  return { sessions, pending, queued };
}

export function applyAgentPoll(poll: AgentPollState, result: AgentPollResult): AgentPollState {
  if (result.ok) return { state: result.state, failures: 0 };
  const failures = Math.min(poll.failures + 1, AGENT_FAILURE_LIMIT);
  return { state: failures >= AGENT_FAILURE_LIMIT ? EMPTY_AGENT_STATE : poll.state, failures };
}

/** The mascot's look: a request or a session that needs the user wins over activity that is working. */
export function agentAttention(state: AgentState): AgentAttention {
  if (state.pending || state.sessions.some((session) => session.state === "te necesita")) return "needs";
  return state.sessions.some((session) => session.state === "trabajando") ? "working" : "idle";
}

// Background Claude Code tasks and computer use --------------------------------------------------------------------
//
// `/api/agent/activity` answers with what Dot is doing for the user: the Codex chats with a turn running, the background
// Claude Code tasks Codex started through Dot's MCP server, and whether a Chrome or mouse-and-keyboard call is running now.

export type ClaudeTaskState = "running" | "done" | "failed" | "cancelled" | "timed_out";
export type ClaudeTaskView = { id: string; project: string; state: ClaudeTaskState; latestLine: string | null; startedAt: number };
export type ComputerUseView = { active: boolean; servers: string[] };
/** A Codex chat whose turn is running now; the title is the user's message as one short line. */
export type CodexTurnView = { id: string; title: string; startedAt: number };
export type AgentActivity = { tasks: ClaudeTaskView[]; computerUse: ComputerUseView; codexTurns: CodexTurnView[] };

export const COMPUTER_USE_TEXT = "Dot está usando tu equipo";
export const EMPTY_ACTIVITY: AgentActivity = { tasks: [], computerUse: { active: false, servers: [] }, codexTurns: [] };
const TASK_STATES: readonly ClaudeTaskState[] = ["running", "done", "failed", "cancelled", "timed_out"];
/** A finished task stays in the menu this long after it started; a task that is still running always shows. */
export const TASK_RECENT_MS = 30 * 60_000;
const MAX_TASK_SESSIONS = 10;

/** Reads an activity response body; anything malformed is not an activity. */
export function readAgentActivity(value: unknown): AgentActivity | undefined {
  if (!isRecord(value) || !Array.isArray(value.tasks) || !isRecord(value.computerUse)) return undefined;
  const tasks: ClaudeTaskView[] = [];
  for (const item of value.tasks) {
    if (!isRecord(item)) return undefined;
    const { id, project, latestLine, startedAt } = item;
    const known = TASK_STATES.find((candidate) => candidate === item.state);
    if (typeof id !== "string" || typeof project !== "string" || !known || (latestLine !== null && typeof latestLine !== "string") || typeof startedAt !== "number") return undefined;
    tasks.push({ id, project, state: known, latestLine, startedAt });
  }
  const { active, servers } = value.computerUse;
  if (typeof active !== "boolean" || !Array.isArray(servers) || !servers.every((server) => typeof server === "string")) return undefined;
  // An answer from a shell that does not list Codex turns (older, or no Codex) simply has none.
  const codexTurns: CodexTurnView[] = [];
  if (value.codexTurns !== undefined) {
    if (!Array.isArray(value.codexTurns)) return undefined;
    for (const item of value.codexTurns) {
      if (!isRecord(item) || typeof item.id !== "string" || typeof item.title !== "string" || typeof item.startedAt !== "number") return undefined;
      codexTurns.push({ id: item.id, title: item.title, startedAt: item.startedAt });
    }
  }
  return { tasks, computerUse: { active, servers: servers as string[] }, codexTurns };
}

const TASK_ENDINGS: Record<Exclude<ClaudeTaskState, "running" | "done">, string> = { failed: "Terminó con error", cancelled: "Cancelada", timed_out: "Se agotó el tiempo" };

/** The tasks as island sessions: running is "trabajando", every other state is "listo" with a plain line saying how it ended. */
export function claudeTaskSessions(tasks: readonly ClaudeTaskView[], now: number): AgentSessionView[] {
  return tasks
    .filter((task) => task.state === "running" || now - task.startedAt <= TASK_RECENT_MS)
    .slice(0, MAX_TASK_SESSIONS)
    .map((task) => ({
      id: `task:${task.id}`,
      project: projectName(task.project),
      state: task.state === "running" ? ("trabajando" as const) : ("listo" as const),
      action: task.latestLine ?? (task.state === "running" ? "Claude Code está trabajando" : task.state === "done" ? "Tarea terminada" : TASK_ENDINGS[task.state]),
      updatedAt: task.startedAt,
    }));
}

const MAX_CODEX_TURNS = 10;
const UNTITLED_TURN = "Chat con Dot";

/** The Codex chats working now as island activity: always "trabajando", titled by what the user asked. */
export function codexTurnSessions(turns: readonly CodexTurnView[]): AgentSessionView[] {
  return turns.slice(0, MAX_CODEX_TURNS).map((turn) => ({
    id: `codex:${turn.id}`,
    project: turn.title.trim() || UNTITLED_TURN,
    state: "trabajando" as const,
    action: null,
    updatedAt: turn.startedAt,
  }));
}

/** Everything Dot is doing for the user, in the island's order: Codex chats in progress, background Claude tasks, Claude Code sessions. */
export function activityList({ sessions, tasks, codexTurns, now }: { sessions: readonly AgentSessionView[]; tasks: readonly ClaudeTaskView[]; codexTurns: readonly CodexTurnView[]; now: number }): AgentSessionView[] {
  return [...codexTurnSessions(codexTurns), ...claudeTaskSessions(tasks, now), ...sessions];
}

export type ActivityPollState = { activity: AgentActivity; failures: number };
export type ActivityPollResult = { ok: true; activity: AgentActivity } | { ok: false };
export const INITIAL_ACTIVITY_POLL: ActivityPollState = { activity: EMPTY_ACTIVITY, failures: 0 };

/** Like `applyAgentPoll`: one or two failed reads change nothing; three forget, so the indicator never outlives its source. */
export function applyActivityPoll(poll: ActivityPollState, result: ActivityPollResult): ActivityPollState {
  if (result.ok) return { activity: result.activity, failures: 0 };
  const failures = Math.min(poll.failures + 1, AGENT_FAILURE_LIMIT);
  return { activity: failures >= AGENT_FAILURE_LIMIT ? EMPTY_ACTIVITY : poll.activity, failures };
}
