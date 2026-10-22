// What the island shows and when, as plain functions so it can be tested without a browser: which state the character
// is in, when the island rests as the invisible strip or becomes the pill, and the short Spanish lines the pill and the head use.

import type { AgentAttention, PendingView } from "../../lib/agent-state.ts";
import type { AgentSessionView, SessionState } from "../../lib/agent-sessions.ts";
import type { WindowMode } from "../../lib/window-mode.ts";
import type { LenteState } from "./lente-look.ts";

const MAX_DOTS = 5;

/** The character follows the agent: needing the user wins, then working (listening counts), then done. */
export function lenteStateFor({ attention, listening, sessions }: { attention: AgentAttention; listening: boolean; sessions: readonly AgentSessionView[] }): LenteState {
  if (attention === "needs") return "approval";
  if (attention === "working" || listening) return "working";
  return sessions.some((session) => session.state === "listo") ? "done" : "idle";
}

/** Something is going on: an agent works or needs the user, the screen is shared, Voz listens, or a session just finished. */
export function isActive({ attention, sharing, listening, finishedRecently }: { attention: AgentAttention; sharing: boolean; listening: boolean; finishedRecently: boolean }) {
  return attention !== "idle" || sharing || listening || finishedRecently;
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
export function compactLine({ sessions, pending, listening, finishedRecently }: { sessions: readonly AgentSessionView[]; pending: PendingView | null; listening: boolean; finishedRecently: boolean }) {
  if (pending) return approvalTitle(pending);
  const needs = latest(sessions, "te necesita");
  if (needs) return `Claude Code te necesita en ${needs.project}`;
  const working = latest(sessions, "trabajando");
  if (working) return working.action || `Trabajando en ${working.project}`;
  if (listening) return "Escuchando…";
  const done = finishedRecently ? latest(sessions, "listo") : undefined;
  if (done) return `Listo en ${done.project}`;
  return "Dot";
}

export function sessionCountLabel(count: number) {
  if (count <= 0) return "";
  return count === 1 ? "1 sesión" : `${count} sesiones`;
}

/** The right side of the expanded head. */
export function inProgressLabel(sessions: readonly AgentSessionView[]) {
  if (sessions.length === 0) return "Sin sesiones";
  const running = sessions.filter((session) => session.state !== "listo").length;
  return running === 0 ? "Todo listo" : `${running} en curso`;
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
