// The choices on the question card, as plain functions so they can be tested without a browser.
//
// A selection holds, per question, the labels picked so far. A single-select question holds at most one label.

import type { AgentQuestion, PendingView } from "../../lib/agent-state.ts";

export type Selection = string[][];
/** What the decision route takes: a label for a single-select question, a list of labels for a multi-select one. */
export type Answers = Record<string, string | string[]>;

/** One question with one choice is answered by clicking the choice; anything else is confirmed with "Responder". */
export const answersOnClick = (questions: AgentQuestion[]) => questions.length === 1 && !questions[0].multiSelect;

export const singleClickAnswers = (question: AgentQuestion, label: string): Answers => ({ [question.question]: label });

export const emptySelection = (questions: AgentQuestion[]): Selection => questions.map(() => []);

/** Picks an option (replacing the pick of a single-select question) or toggles it (multi-select). Unknown targets change nothing. */
export function toggleOption(questions: AgentQuestion[], selection: Selection, questionIndex: number, label: string): Selection {
  const question = questions[questionIndex];
  if (!question || !question.options.some((option) => option.label === label)) return selection;
  return selection.map((picked, index) => {
    if (index !== questionIndex) return picked;
    if (!question.multiSelect) return [label];
    return picked.includes(label) ? picked.filter((candidate) => candidate !== label) : [...picked, label];
  });
}

export const canSubmit = (questions: AgentQuestion[], selection: Selection) => questions.every((_, index) => (selection[index]?.length ?? 0) > 0);

/** The answers keyed by the exact question text; multi-select labels follow the order of the options. */
export function buildAnswers(questions: AgentQuestion[], selection: Selection): Answers {
  const answers: Answers = {};
  questions.forEach((question, index) => {
    const picked = selection[index] ?? [];
    answers[question.question] = question.multiSelect
      ? question.options.map((option) => option.label).filter((label) => picked.includes(label))
      : (picked[0] ?? "");
  });
  return answers;
}

// Keyboard: while a permission card is on screen and the island has focus, Y allows and N denies.

export type ApprovalKey = "allow" | "deny";
export type KeyLike = { key: string; ctrlKey?: boolean; metaKey?: boolean; altKey?: boolean; repeat?: boolean; target?: EventTarget | null };
export type ApprovalKeyContext = { pending: PendingView | null; cardShown: boolean; busy: boolean; focused: boolean };

const TEXT_FIELDS = ["INPUT", "TEXTAREA", "SELECT"];

/**
 * What a key press does to the permission card, or null when it does nothing. Questions are answered by choosing an option;
 * typing in a field, a modifier or a held key never answers a request.
 */
export function approvalKeyChoice(event: KeyLike, { pending, cardShown, busy, focused }: ApprovalKeyContext): ApprovalKey | null {
  if (!pending || pending.questions !== null || !cardShown || busy || !focused) return null;
  if (event.ctrlKey || event.metaKey || event.altKey || event.repeat) return null;
  const target = event.target as { tagName?: string; isContentEditable?: boolean } | null | undefined;
  if (target && (TEXT_FIELDS.includes(target.tagName ?? "") || target.isContentEditable)) return null;
  const key = event.key.toLowerCase();
  return key === "y" ? "allow" : key === "n" ? "deny" : null;
}
