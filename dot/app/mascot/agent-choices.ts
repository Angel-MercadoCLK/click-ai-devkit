// The choices on the question card, as plain functions so they can be tested without a browser.
//
// A selection holds, per question, the labels picked so far. A single-select question holds at most one label.

import type { AgentQuestion } from "../../lib/agent-state.ts";

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
