"use client";

import { useState } from "react";
import type { PendingView } from "../../lib/agent-state.ts";
import { answersOnClick, buildAnswers, canSubmit, emptySelection, singleClickAnswers, toggleOption } from "./agent-choices.ts";
import type { AgentChoice } from "./use-agent";
import { Lente } from "./lente";
import type { Answers } from "./agent-choices.ts";

type CardProps = {
  pending: PendingView;
  busy: boolean;
  failed: boolean;
  onDecide: (choice: AgentChoice, answers?: Answers) => void;
};

/** The permission request or the question Claude Code is waiting on. Its own state is reset for every request. */
export function AgentCard({ pending, busy, failed, onDecide }: CardProps) {
  return <CardContent key={pending.id} pending={pending} busy={busy} failed={failed} onDecide={onDecide} />;
}

function CardContent({ pending, busy, failed, onDecide }: CardProps) {
  const { questions } = pending;
  const [selection, setSelection] = useState(() => emptySelection(questions ?? []));
  const asking = questions !== null;
  // A request from one of Dot's own tools (PowerShell, Claude Code): there is no terminal to hand it back to.
  const own = pending.kind === "dot-tool";
  return (
    <section className="agent-card" role="group" aria-label={own ? `Dot quiere usar ${pending.tool}` : `Claude Code te necesita en ${pending.project}`}>
      <div className="agent-card-glow" aria-hidden="true" />
      <div className="agent-card-lente">
        <Lente state="approval" size={96} />
      </div>
      <div className="agent-card-main">
        <header className="agent-card-header">
          <span className="agent-dot" aria-hidden="true" />
          <span className="agent-card-project">{pending.project}</span>
          <span className="agent-card-source">{own ? "Dot" : "Claude Code"}</span>
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
          {failed && <p className="agent-card-error" role="status">{own ? "No se pudo enviar tu respuesta. Inténtalo de nuevo." : "No se pudo enviar tu respuesta. Responde en la terminal."}</p>}
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
                <button className="agent-action" type="button" disabled={busy} title={own ? "Permitir este comando exacto hasta que cierres Dot" : "Permitir y recordar esta regla"} onClick={() => onDecide("always")}>Siempre</button>
              )}
              <button className="agent-action primary" type="button" disabled={busy} onClick={() => onDecide("allow")}>Permitir <kbd>Y</kbd></button>
            </>
          )}
          {!own && <button className="agent-link" type="button" disabled={busy} onClick={() => onDecide("terminal")}>Responder en la terminal</button>}
        </footer>
      </div>
    </section>
  );
}
