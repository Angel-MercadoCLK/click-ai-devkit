"use client";

import { useState } from "react";
import type { PendingView } from "../../lib/agent-state.ts";
import { answersOnClick, buildAnswers, canSubmit, emptySelection, singleClickAnswers, toggleOption } from "./agent-choices.ts";
import type { AgentChoice } from "./use-agent";
import { Orbi } from "./orbi";
import type { Answers } from "./agent-choices.ts";
import { cardView, choiceLabel, codexCardBody } from "./agent-card-view.ts";

type CardProps = {
  pending: PendingView;
  busy: boolean;
  failed: boolean;
  onDecide: (choice: AgentChoice, answers?: Answers) => void;
};

/** The permission request or the question Dot (or one of its tools, Claude Code, Codex) is waiting on. Its own state is reset for every request. */
export function AgentCard({ pending, busy, failed, onDecide }: CardProps) {
  return <CardContent key={pending.id} pending={pending} busy={busy} failed={failed} onDecide={onDecide} />;
}

function CardContent({ pending, busy, failed, onDecide }: CardProps) {
  const { questions } = pending;
  const [selection, setSelection] = useState(() => emptySelection(questions ?? []));
  const asking = questions !== null;
  // A request from one of Dot's own tools (PowerShell, Claude Code) or from Codex: there is no terminal to hand it back to.
  const view = cardView(pending);
  const own = view.source === "dot-tool";
  const codex = view.source === "codex";
  const codexBody = codex ? codexCardBody(pending) : null;
  return (
    <section className="agent-card" role="group" aria-label={view.ariaLabel} data-source={view.source}>
      <div className="agent-card-glow" aria-hidden="true" />
      <div className="agent-card-orbi">
        <Orbi state="needs" size={96} />
      </div>
      <div className="agent-card-main">
        <header className="agent-card-header">
          <span className="agent-dot" aria-hidden="true" />
          <span className="agent-card-project">{pending.project}</span>
          <span className="agent-card-source">{view.sourceLabel}</span>
        </header>
        <div className="agent-card-body">
          {asking ? (
            <>
              {questions.map((question, index) => (
                <fieldset className="agent-question" key={question.question} disabled={busy}>
                  <legend>{question.header ? <b>{question.header}</b> : null} {question.question}</legend>
                  <div className="agent-options">
                    {question.options.map((option) =>
                      answersOnClick(questions) ? (
                        <button className="agent-option" type="button" key={option.label} title={option.description || undefined} onClick={() => onDecide("allow", singleClickAnswers(question, option.label))}>
                          <span>{option.label}</span>
                          {option.description && <small>{option.description}</small>}
                        </button>
                      ) : (
                        <label className="agent-option" key={option.label} title={option.description || undefined}>
                          <input
                            type={question.multiSelect ? "checkbox" : "radio"}
                            name={`question-${index}`}
                            checked={selection[index].includes(option.label)}
                            onChange={() => setSelection((current) => toggleOption(questions, current, index, option.label))}
                          />
                          <span>{option.label}</span>
                          {option.description && <small>{option.description}</small>}
                        </label>
                      ),
                    )}
                  </div>
                </fieldset>
              ))}
            </>
          ) : codexBody ? (
            <>
              <p className="agent-card-summary">{codexBody.heading}</p>
              {codexBody.summary && <p className="agent-card-note">{codexBody.summary}</p>}
              <pre className="agent-card-detail">{codexBody.detail}</pre>
            </>
          ) : own ? (
            <>
              <p className="agent-card-summary">Dot quiere usar <b>{pending.tool}</b></p>
              <pre className="agent-card-detail">{pending.summary}</pre>
              {pending.detail && <p className="agent-card-note">{pending.detail}</p>}
            </>
          ) : (
            <>
              <p className="agent-card-summary">Quiere usar <b>{pending.tool}</b></p>
              {pending.detail && <pre className="agent-card-detail">{pending.detail}</pre>}
            </>
          )}
          {failed && <p className="agent-card-error" role="status">{view.failedText}</p>}
        </div>
        <footer className="agent-card-actions">
          {asking ? (
            !answersOnClick(questions) && (
              <button className="agent-action primary" type="button" disabled={busy || !canSubmit(questions, selection)} onClick={() => onDecide("allow", buildAnswers(questions, selection))}>
                Responder
              </button>
            )
          ) : (
            <>
              <button className="agent-action" type="button" disabled={busy} onClick={() => onDecide("deny")}>Denegar <kbd>N</kbd></button>
              {pending.alwaysOffered && (
                <button className="agent-action" type="button" disabled={busy} title={view.alwaysTitle} onClick={() => onDecide("always")}>{choiceLabel("always", pending.kind)}</button>
              )}
              <button className="agent-action primary" type="button" disabled={busy} onClick={() => onDecide("allow")}>Permitir <kbd>Y</kbd></button>
            </>
          )}
          {view.showTerminal && <button className="agent-link" type="button" disabled={busy} onClick={() => onDecide("terminal")}>{choiceLabel("terminal", pending.kind)}</button>}
        </footer>
      </div>
    </section>
  );
}
