// What the mascot page knows about Claude Code: the shapes `/api/agent/state` answers with, how the page reads them
// defensively, and what they mean for the mascot's look. No Node imports: the page uses this file too.

import type { AgentSessionView, SessionState } from "./agent-sessions.ts";

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

/** The mascot's look for Claude Code: a request or a session that needs the user wins over sessions that are working. */
export function agentAttention(state: AgentState): AgentAttention {
  if (state.pending || state.sessions.some((session) => session.state === "te necesita")) return "needs";
  return state.sessions.some((session) => session.state === "trabajando") ? "working" : "idle";
}
