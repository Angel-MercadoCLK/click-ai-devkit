// What the island shows and when, as plain functions so it can be tested without a browser: which state the character
// is in, when the island rests as the invisible strip or becomes the pill, and the short Spanish lines the pill and the head use.

import { COMPUTER_USE_TEXT, type AgentAttention, type PendingView } from "../../lib/agent-state.ts";
import type { AgentSessionView, SessionState } from "../../lib/agent-sessions.ts";
import type { WindowMode } from "../../lib/window-mode.ts";
import type { ClaudeTaskView } from "../../lib/agent-state.ts";
import type { OrbiState } from "./orbi-look.ts";

const MAX_DOTS = 5;

/**
 * Orbi follows what is going on: needing the user wins, then Voz listening, then an agent working, then a task that just
 * failed, then something that just finished, then sleeping (after the long quiet) and idle otherwise. News always wakes it.
 */
export function orbiStateFor({ attention, listening, finished = false, failed = false, asleep = false }: { attention: AgentAttention; listening: boolean; finished?: boolean; failed?: boolean; asleep?: boolean }): OrbiState {
  if (attention === "needs") return "needs";
  if (listening) return "listening";
  if (attention === "working") return "working";
  if (failed) return "error";
  if (finished) return "done";
  return asleep ? "sleeping" : "idle";
}

/** Something is going on: an agent works or needs the user, the screen is shared, Voz listens, or a session just finished. */
export function isActive({ attention, sharing, listening, finishedRecently, computerUse = false }: { attention: AgentAttention; sharing: boolean; listening: boolean; finishedRecently: boolean; computerUse?: boolean }) {
  return attention !== "idle" || sharing || listening || finishedRecently || computerUse;
}

/**
 * The resting strip (nothing drawn) when nothing is going on, the pill when something is or the pointer is on the strip.
 * Hidden by the user, only a shared screen brings the pill: the capture indicator is never hidden from you.
 */
export function restingMode({ active, hovered, hidden = false, sharing = false }: { active: boolean; hovered: boolean; hidden?: boolean; sharing?: boolean }): "collapsed" | "compact" {
  if (hidden) return sharing ? "compact" : "collapsed";
  return active || hovered ? "compact" : "collapsed";
}

/**
 * The bar or pill size the island should move to, or null to leave it. A new request owns the window until its card is
 * up; once raised, a card put away by hand falls back to the resting size like anything else, so sharing keeps the pill.
 */
export function restingTarget({ mode, resting, pendingId, raisedFor }: {
  mode: WindowMode;
  resting: "collapsed" | "compact";
  pendingId: string | null;
  raisedFor: string | null;
}): "collapsed" | "compact" | null {
  if (pendingId && raisedFor !== pendingId) return null;
  if (mode !== "collapsed" && mode !== "compact") return null;
  return mode === resting ? null : resting;
}

/** The request whose card has been on screen: it counts only once the window is in the approval mode. */
export function raisedCardFor(previous: string | null, mode: WindowMode, pendingId: string | null): string | null {
  return mode === "approval" && pendingId ? pendingId : previous;
}

/** Closing a card lands on the resting state, so the strip never hides the pill while something is going on. */
export function settleMode(next: WindowMode, resting: "collapsed" | "compact"): WindowMode {
  return next === "collapsed" ? resting : next;
}

/** "Editando foo.ts +12 −3" into its words and its line counts, so the counts can be colored. */
export function splitAction(action: string | null): { text: string; added: number | null; removed: number | null } {
  if (!action) return { text: "", added: null, removed: null };
  const match = /^(.*?)\s\+(\d+)(?:\s[−-](\d+))?$/.exec(action);
  if (!match) return { text: action, added: null, removed: null };
  return { text: match[1], added: Number(match[2]), removed: match[3] === undefined ? null : Number(match[3]) };
}

const isQuestion = (pending: PendingView) => pending.questions !== null;

/** The head of the permission card: Dot needs a permission, or an answer when it is a question. */
export function approvalTitle(pending: PendingView) {
  return isQuestion(pending) ? "Dot necesita tu respuesta" : "Dot necesita tu permiso";
}

const latest = (sessions: readonly AgentSessionView[], state: SessionState) =>
  sessions.filter((session) => session.state === state).reduce<AgentSessionView | undefined>((best, session) => (!best || session.updatedAt >= best.updatedAt ? session : best), undefined);

/** The one line of the pill: what is waiting for the user, what the agent is doing, or that it finished. */
export function compactLine({ sessions, pending, listening, finishedRecently, computerUse = false }: { sessions: readonly AgentSessionView[]; pending: PendingView | null; listening: boolean; finishedRecently: boolean; computerUse?: boolean }) {
  if (pending) return approvalTitle(pending);
  const needs = latest(sessions, "te necesita");
  if (needs) return `Claude Code te necesita en ${needs.project}`;
  // Dot controlling the mouse or Chrome is never hidden behind a quieter line.
  if (computerUse) return COMPUTER_USE_TEXT;
  const working = latest(sessions, "trabajando");
  if (working) return working.action || `Trabajando en ${working.project}`;
  if (listening) return "Escuchando…";
  const done = finishedRecently ? latest(sessions, "listo") : undefined;
  if (done) return `Listo en ${done.project}`;
  return "Dot";
}

const inProgress = (sessions: readonly AgentSessionView[]) => sessions.filter((session) => session.state !== "listo").length;

/** The count beside the pill's line: how many things are in progress, nothing when none is. */
export function activityCountLabel(sessions: readonly AgentSessionView[]) {
  const running = inProgress(sessions);
  return running === 0 ? "" : `${running} en curso`;
}

/** The right side of the expanded head. */
export function inProgressLabel(sessions: readonly AgentSessionView[]) {
  if (sessions.length === 0) return "Todo tranquilo";
  const running = inProgress(sessions);
  return running === 0 ? "Todo listo" : `${running} en curso`;
}

// The menu's "Actividad" -----------------------------------------------------------------------------------------

export const ACTIVITY_TITLE = "Actividad";
export const ACTIVITY_EMPTY = "Todo tranquilo. Pídele algo a Dot.";
export const ACTIVITY_ASK = "Pedir algo a Dot";

export type ActivitySource = "dot" | "claude-task" | "claude-code";

/** Where an activity item comes from, by the id its list gives it: `codex:` for Dot's own chats, `task:` for background Claude tasks. */
export function activitySource(item: Pick<AgentSessionView, "id">): ActivitySource {
  if (item.id.startsWith("codex:")) return "dot";
  return item.id.startsWith("task:") ? "claude-task" : "claude-code";
}

const SOURCE_LABELS: Record<ActivitySource, string> = { dot: "Dot", "claude-task": "Tarea en Claude Code", "claude-code": "Claude Code" };

/** The small line that says whose item it is: Claude Code is named only on the items that are Claude's. */
export const activitySourceLabel = (item: Pick<AgentSessionView, "id">) => SOURCE_LABELS[activitySource(item)];

/**
 * The island's accessible name: capture first, then what Dot is doing, then listening. Dot speaks for itself; Claude Code is
 * named only when it is the one that needs you (a request from Codex or from one of Dot's tools is Dot's).
 */
export function islandLabel({ sharingText, attention, listening, pending }: { sharingText: string | null; attention: AgentAttention; listening: boolean; pending: Pick<PendingView, "kind"> | null }) {
  const activity = attention === "working" ? "Dot está trabajando" : attention === "needs" ? (pending && pending.kind !== "claude-code" ? "Dot te necesita" : "Claude Code te necesita") : null;
  return [sharingText, activity, listening ? "Escuchando…" : null].filter(Boolean).join(". ") || "Dot";
}

/** One dot per session (at most five), colored by its state. */
export function sessionDots(sessions: readonly AgentSessionView[]): SessionState[] {
  return sessions.slice(0, MAX_DOTS).map((session) => session.state);
}

export const sessionStates = (sessions: readonly AgentSessionView[]): Record<string, SessionState> => Object.fromEntries(sessions.map((session) => [session.id, session.state]));

/** A session that was working (or needed the user) and is now ready is news. One the page first sees already finished is not. */
export function newlyFinished(previous: Readonly<Record<string, SessionState>>, sessions: readonly AgentSessionView[]) {
  return sessions.some((session) => session.state === "listo" && (previous[session.id] === "trabajando" || previous[session.id] === "te necesita"));
}

const FAILED_TASKS: readonly ClaudeTaskView["state"][] = ["failed", "timed_out"];

/** The background tasks by id and state, to tell later which one just failed. */
export const taskStates = (tasks: readonly Pick<ClaudeTaskView, "id" | "state">[]): Record<string, ClaudeTaskView["state"]> => Object.fromEntries(tasks.map((task) => [task.id, task.state]));

/** A task that was running and has now failed or timed out is a failure worth showing. Cancelled by the user is not, and neither is one first seen already failed. */
export function newlyFailed(previous: Readonly<Record<string, ClaudeTaskView["state"]>>, tasks: readonly Pick<ClaudeTaskView, "id" | "state">[]) {
  return tasks.some((task) => FAILED_TASKS.includes(task.state) && previous[task.id] === "running");
}
