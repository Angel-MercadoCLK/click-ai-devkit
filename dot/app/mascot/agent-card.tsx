"use client";

import { useState } from "react";
import type { PendingView } from "../../lib/agent-state.ts";
import { answersOnClick, buildAnswers, canSubmit, emptySelection, singleClickAnswers, toggleOption } from "./agent-choices.ts";
import type { AgentChoice } from "./use-agent";
import type { Answers } from "./agent-choices.ts";

type CardProps = {
  pending: PendingView;
  queued: number;
  busy: boolean;
  failed: boolean;
  onDecide: (choice: AgentChoice, answers?: Answers) => void;
};

/** The permission request or the question Claude Code is waiting on. Its own state is reset for every request. */
export function AgentCard({ pending, queued, busy, failed, onDecide }: CardProps) {
  return <CardContent key={pending.id} pending={pending} queued={queued} busy={busy} failed={failed} onDecide={onDecide} />;
}

function CardContent({ pending, queued, busy, failed, onDecide }: CardProps) {
  const { questions } = pending;
  const [selection, setSelection] = useState(() => emptySelection(questions ?? []));
  const asking = questions !== null;
  return (
    <section className="agent-card" role="group" aria-label={`Claude Code te necesita en ${pending.project}`}>
      <header className="agent-card-header">
        <span className="agent-card-project">{pending.project}</span>
        {queued > 0 && <span className="agent-card-queue" title="Peticiones en espera">+{queued} más</span>}
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
        ) : (
          <>
            <p className="agent-card-summary">Quiere usar <b>{pending.tool}</b></p>
            {pending.detail && <pre className="agent-card-detail">{pending.detail}</pre>}
          </>
        )}
        {failed && <p className="agent-card-error" role="status">No se pudo enviar tu respuesta. Responde en la terminal.</p>}
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
            <button className="agent-action primary" type="button" disabled={busy} onClick={() => onDecide("allow")}>Permitir</button>
            <button className="agent-action" type="button" disabled={busy} onClick={() => onDecide("deny")}>Denegar</button>
            {pending.alwaysOffered && (
              <button className="agent-action" type="button" disabled={busy} title="Permitir y recordar esta regla" onClick={() => onDecide("always")}>Siempre</button>
            )}
          </>
        )}
        <button className="agent-link" type="button" disabled={busy} onClick={() => onDecide("terminal")}>Responder en la terminal</button>
      </footer>
    </section>
  );
}
