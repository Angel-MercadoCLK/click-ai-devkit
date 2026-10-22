// The approval queue between Claude Code's hook relay and the mascot.
//
// A permission request from the relay is held here until the user decides on the mascot. The relay waits for the HTTP
// answer, so every way out has to end that wait: a decision answers it, and everything else lets go with "no decision",
// which makes Claude Code ask in its own terminal as usual. Nothing here ever allows or denies on its own.
//
// Timing (see the wire contract): the card has to be acknowledged by the page within 2 s of being shown, and the user has
// 105 s from the request's arrival. Requests queue in arrival order and the card shows the oldest, so the acknowledgement
// window of a queued request starts when its card is shown, while the 105 s limit counts from arrival.
//
// State is shared between the Electron main process and the Next routes through `globalThis`, like the other controllers.

import { randomUUID } from "node:crypto";
import { createSessionStore, describeToolAction, projectName, type SessionStore } from "./agent-sessions.ts";
import type { AgentQuestion, AgentState, PendingKind, PendingView } from "./agent-state.ts";
import { getWindowController } from "./window-mode.ts";

export const AGENT_BROKER_KEY = Symbol.for("click-dot.agent-broker");

/** Wire contract: the card must be on screen within this long of being shown, and the user decides within this long of arrival. */
export const ACK_TIMEOUT_MS = 2_000;
export const DECISION_TIMEOUT_MS = 105_000;
export const MAX_PENDING = 20;

const MAX_QUESTIONS = 8;
const MAX_OPTIONS = 12;
const MAX_QUESTION_TEXT = 500;
const MAX_LABEL = 200;
const MAX_DESCRIPTION = 500;
const MAX_HEADER = 60;
const MAX_DETAIL = 1_000;
/** A permission suggestion larger than this is not a rule; "Siempre" is not offered for it. */
const MAX_SUGGESTION_BYTES = 8_192;

export type { AgentQuestion, AgentState, PendingView } from "./agent-state.ts";
export type AgentDecision = { behavior: "allow" | "deny"; updatedPermissions?: unknown[]; answers?: Record<string, string> };
/** `choice` answers a Dot tool request, which has no Claude Code decision to send back. */
export type AgentOutcome = { kind: "none" } | { kind: "decision"; decision: AgentDecision } | { kind: "choice"; choice: "allow" | "deny" | "always" };
/** What a Dot tool learns from its approval card. Anything but a choice (timeout, no acknowledgement, quit, abort) is "none". */
export type ToolApproval = "allow" | "deny" | "always" | "none";
export type DecisionChoice = "allow" | "deny" | "always" | "terminal";
export const DECISION_CHOICES: readonly DecisionChoice[] = ["allow", "deny", "always", "terminal"];

export type DecideResult = { ok: true } | { ok: false; status: 400 | 404; error: string };

const NONE: AgentOutcome = { kind: "none" };
const isRecord = (value: unknown): value is Record<string, unknown> => !!value && typeof value === "object" && !Array.isArray(value);
const boundedText = (value: unknown, limit: number) => (typeof value === "string" && value.length <= limit ? value : undefined);

/** The questions of an AskUserQuestion call, or undefined when they cannot be shown and answered exactly. */
export function parseQuestions(toolInput: unknown): AgentQuestion[] | undefined {
  const list = isRecord(toolInput) ? toolInput.questions : undefined;
  if (!Array.isArray(list) || list.length < 1 || list.length > MAX_QUESTIONS) return undefined;
  const questions: AgentQuestion[] = [];
  for (const item of list) {
    if (!isRecord(item)) return undefined;
    const question = boundedText(item.question, MAX_QUESTION_TEXT);
    if (!question || questions.some((known) => known.question === question)) return undefined;
    if (!Array.isArray(item.options) || item.options.length < 1 || item.options.length > MAX_OPTIONS) return undefined;
    const options: AgentQuestion["options"] = [];
    for (const option of item.options) {
      if (!isRecord(option)) return undefined;
      const label = boundedText(option.label, MAX_LABEL);
      if (!label || options.some((known) => known.label === label)) return undefined;
      options.push({ label, description: boundedText(option.description, MAX_DESCRIPTION) ?? "" });
    }
    questions.push({ question, header: boundedText(item.header, MAX_HEADER) ?? "", multiSelect: item.multiSelect === true, options });
  }
  return questions;
}

/**
 * Checks the user's answers against the pending questions: every question answered, with its exact text as the key and
 * labels taken from its own options. A multi-select answer is a list of labels, joined with ", " as Claude Code expects.
 */
export function validateAnswers(questions: AgentQuestion[], answers: unknown): Record<string, string> | undefined {
  if (!isRecord(answers)) return undefined;
  const keys = Object.keys(answers);
  if (keys.length !== questions.length) return undefined;
  const result: [string, string][] = [];
  for (const { question, multiSelect, options } of questions) {
    if (!keys.includes(question)) return undefined;
    const given = answers[question];
    const known = (label: unknown): label is string => typeof label === "string" && options.some((option) => option.label === label);
    if (multiSelect) {
      if (!Array.isArray(given) || given.length === 0 || !given.every(known) || new Set(given).size !== given.length) return undefined;
      result.push([question, given.join(", ")]);
    } else {
      if (!known(given)) return undefined;
      result.push([question, given]);
    }
  }
  return Object.fromEntries(result);
}

/** The longer text the user needs to judge a request: the whole command, or the path being touched. */
function detailOf(tool: string, input: unknown) {
  const args = isRecord(input) ? input : {};
  const clipped = (value: string) => (value.length > MAX_DETAIL ? `${value.slice(0, MAX_DETAIL - 1)}…` : value);
  if (tool === "Bash" && typeof args.command === "string") return clipped(args.command);
  if (typeof args.file_path === "string") return clipped(args.file_path);
  if (typeof args.pattern === "string") return clipped(args.pattern);
  try {
    return clipped(JSON.stringify(args) ?? "");
  } catch {
    return "";
  }
}

/** The first rule Claude Code suggested, or undefined when it suggested none that can be offered. */
function firstSuggestion(payload: Record<string, unknown>) {
  const list = payload.permission_suggestions;
  const first = Array.isArray(list) ? list[0] : undefined;
  if (!isRecord(first)) return undefined;
  try {
    return JSON.stringify(first).length <= MAX_SUGGESTION_BYTES ? first : undefined;
  } catch {
    return undefined;
  }
}

/** A request is a Claude Code hook (it has a session) or a Dot tool asking before it acts. */
type Held = {
  id: string;
  kind: PendingKind;
  /** Null for a Dot tool request: it belongs to no Claude Code session. */
  sessionId: string | null;
  alwaysOffered: boolean;
  tool: string;
  summary: string;
  detail: string;
  project: string;
  suggestion: Record<string, unknown> | undefined;
  questions: AgentQuestion[] | null;
  resolve: (outcome: AgentOutcome) => void;
  shown: boolean;
  acked: boolean;
  ackTimer?: unknown;
  decisionTimer?: unknown;
  detach?: () => void;
};

export type AgentBrokerOptions = {
  store?: SessionStore;
  now?: () => number;
  setTimer?: (callback: () => void, ms: number) => unknown;
  clearTimer?: (handle: unknown) => void;
  ackMs?: number;
  decisionMs?: number;
  maxPending?: number;
  /** A card became the one to show: ask the shell to bring the mascot up without taking focus. */
  onPending?: () => void;
};

export function createAgentBroker(options: AgentBrokerOptions = {}) {
  const now = options.now ?? Date.now;
  const store = options.store ?? createSessionStore({ now });
  const setTimer =
    options.setTimer ??
    ((callback: () => void, ms: number) => {
      const handle = setTimeout(callback, ms);
      handle.unref?.();
      return handle;
    });
  const clearTimer = options.clearTimer ?? ((handle: unknown) => clearTimeout(handle as NodeJS.Timeout));
  const ackMs = options.ackMs ?? ACK_TIMEOUT_MS;
  const decisionMs = options.decisionMs ?? DECISION_TIMEOUT_MS;
  const maxPending = options.maxPending ?? MAX_PENDING;
  const queue: Held[] = [];

  /** Ends a request. `outcome` says who is asking next: Claude Code carries on after an answer, the terminal after a hand-back. */
  function finish(request: Held, outcome: AgentOutcome, settle: "answered" | "terminal") {
    const index = queue.indexOf(request);
    if (index === -1) return false;
    queue.splice(index, 1);
    clearTimer(request.ackTimer);
    clearTimer(request.decisionTimer);
    request.detach?.();
    if (request.sessionId !== null) store.settle(request.sessionId, settle);
    request.resolve(outcome);
    showHead();
    return true;
  }

  /** The oldest request is the card on screen: its acknowledgement window starts now. */
  function showHead() {
    const head = queue[0];
    if (!head || head.shown) return;
    head.shown = true;
    head.ackTimer = setTimer(() => finish(head, NONE, "terminal"), ackMs);
    try {
      options.onPending?.();
    } catch {
      // The shell failing to show the mascot only means the card goes unacknowledged and the request is handed back.
    }
  }

  const view = ({ id, kind, tool, summary, detail, project, alwaysOffered, questions }: Held): PendingView => ({
    id,
    kind,
    tool,
    summary,
    detail,
    project,
    alwaysOffered,
    questions,
  });

  /** Puts a request in the queue behind the others; the card shows the oldest. */
  function hold(request: Held, signal: AbortSignal | undefined) {
    queue.push(request);
    request.decisionTimer = setTimer(() => finish(request, NONE, "terminal"), decisionMs);
    if (signal) {
      // The caller went away (Claude Code answered in the terminal, the chat was stopped): nobody is waiting any more.
      const onAbort = () => void finish(request, NONE, "answered");
      signal.addEventListener("abort", onAbort, { once: true });
      request.detach = () => signal.removeEventListener("abort", onAbort);
    }
    showHead();
  }

  return {
    /** Records a plain hook event in the session list. */
    record(event: string, payload: unknown) {
      store.apply(event, payload);
    },

    /** Holds a permission request. Resolves with the decision, or with "none" for every way the wait can end without one. */
    enqueue(input: { sessionId: string; payload: Record<string, unknown>; signal?: AbortSignal }): Promise<AgentOutcome> {
      const { sessionId, payload, signal } = input;
      if (signal?.aborted || queue.length >= maxPending) return Promise.resolve(NONE);
      const tool = typeof payload.tool_name === "string" ? payload.tool_name : "";
      const asking = tool === "AskUserQuestion";
      const questions = asking ? (parseQuestions(payload.tool_input) ?? null) : null;
      // A question that cannot be shown exactly cannot be answered from here; the terminal can.
      if (asking && !questions) return Promise.resolve(NONE);
      const summary = asking ? "Te hace una pregunta" : describeToolAction(tool, payload.tool_input);
      const project = projectName(payload.cwd);
      return new Promise<AgentOutcome>((resolve) => {
        const suggestion = firstSuggestion(payload);
        const request: Held = {
          id: randomUUID(),
          kind: "claude-code",
          sessionId,
          alwaysOffered: !questions && suggestion !== undefined,
          tool,
          summary,
          detail: asking ? "" : detailOf(tool, payload.tool_input),
          project,
          suggestion,
          questions,
          resolve,
          shown: false,
          acked: false,
        };
        store.needsYou(sessionId, { cwd: payload.cwd, action: asking ? summary : `Pide permiso: ${summary}` });
        hold(request, signal);
      });
    },

    /**
     * Holds a request from one of Dot's own tools, with the same queue, acknowledgement and decision timers as a Claude
     * Code request. Resolves with the user's choice, or "none" for every way the wait can end without one.
     */
    requestToolApproval(input: { tool: string; summary: string; detail: string; alwaysOffered: boolean; signal?: AbortSignal }): Promise<ToolApproval> {
      const { signal } = input;
      if (signal?.aborted || queue.length >= maxPending) return Promise.resolve("none");
      return new Promise<ToolApproval>((resolve) => {
        const request: Held = {
          id: randomUUID(),
          kind: "dot-tool",
          sessionId: null,
          alwaysOffered: input.alwaysOffered,
          tool: input.tool,
          summary: input.summary,
          detail: input.detail,
          project: "Dot",
          suggestion: undefined,
          questions: null,
          resolve: (outcome) => resolve(outcome.kind === "choice" ? outcome.choice : "none"),
          shown: false,
          acked: false,
        };
        hold(request, signal);
      });
    },

    state(): AgentState {
      return { sessions: store.list(), pending: queue[0] ? view(queue[0]) : null, queued: Math.max(0, queue.length - 1) };
    },

    /** The page confirms the card of this request is on screen. */
    ack(id: string): boolean {
      const head = queue[0];
      if (!head || head.id !== id) return false;
      head.acked = true;
      clearTimer(head.ackTimer);
      return true;
    },

    /** The user's choice. A refused choice leaves the request waiting. */
    decide(id: string, choice: unknown, answers?: unknown): DecideResult {
      const request = queue.find((candidate) => candidate.id === id);
      if (!request) return { ok: false, status: 404, error: "There is no such pending request." };
      const refuse = (error: string): DecideResult => ({ ok: false, status: 400, error });
      if (!DECISION_CHOICES.some((known) => known === choice)) return refuse("Unknown choice.");
      if (request.kind === "dot-tool") {
        if (choice === "terminal") return refuse("A Dot request cannot be handed to the terminal.");
        if (answers !== undefined) return refuse("A Dot request carries no answers.");
        if (choice === "always" && !request.alwaysOffered) return refuse("Siempre is not offered for this request.");
        finish(request, { kind: "choice", choice: choice as "allow" | "deny" | "always" }, "answered");
        return { ok: true };
      }
      if (choice === "terminal") {
        if (answers !== undefined) return refuse("A hand-back to the terminal carries no answers.");
        finish(request, NONE, "terminal");
        return { ok: true };
      }
      if (choice === "deny") {
        if (answers !== undefined) return refuse("A denial carries no answers.");
        finish(request, { kind: "decision", decision: { behavior: "deny" } }, "answered");
        return { ok: true };
      }
      if (choice === "always") {
        if (answers !== undefined) return refuse("Always carries no answers.");
        if (request.questions || !request.suggestion) return refuse("Claude Code suggested no rule to always allow.");
        finish(request, { kind: "decision", decision: { behavior: "allow", updatedPermissions: [request.suggestion] } }, "answered");
        return { ok: true };
      }
      if (request.questions) {
        const checked = validateAnswers(request.questions, answers);
        if (!checked) return refuse("The answers do not match the questions.");
        finish(request, { kind: "decision", decision: { behavior: "allow", answers: checked } }, "answered");
        return { ok: true };
      }
      if (answers !== undefined) return refuse("This request asks no questions.");
      finish(request, { kind: "decision", decision: { behavior: "allow" } }, "answered");
      return { ok: true };
    },

    /** Dot is quitting: nobody may be left waiting for a window that is gone. */
    releaseAll() {
      for (const request of [...queue]) finish(request, NONE, "terminal");
    },
  };
}

export type AgentBroker = ReturnType<typeof createAgentBroker>;

/** The broker shared by the desktop shell and every route. It is created on first use and kept on `globalThis`. */
export function getAgentBroker(): AgentBroker {
  const target = globalThis as Record<symbol, unknown>;
  const existing = target[AGENT_BROKER_KEY];
  if (isRecord(existing) && ["record", "enqueue", "requestToolApproval", "state", "ack", "decide", "releaseAll"].every((name) => typeof existing[name] === "function")) {
    return existing as unknown as AgentBroker;
  }
  const created = createAgentBroker({ onPending: () => getWindowController()?.showInactive?.() });
  target[AGENT_BROKER_KEY] = created;
  return created;
}
