// What each kind of request looks like on the card, as plain functions so it can be tested without a browser.
//
// Three kinds ask the user: a Claude Code hook (which can be handed back to its terminal), one of Dot's own tools, and the
// Codex engine (a command, a file change, extra permissions, an MCP confirmation; nothing to hand back). The Claude Code
// and Dot-tool wording is what it always was.

import type { PendingKind, PendingView } from "../../lib/agent-state.ts";
import type { AgentChoice } from "./use-agent";

export type CardView = {
  source: PendingKind;
  /** The small label in the card's header. */
  sourceLabel: string;
  ariaLabel: string;
  /** Whether "Responder en la terminal" is offered. */
  showTerminal: boolean;
  alwaysTitle: string;
  failedText: string;
};

export function cardView(pending: PendingView): CardView {
  if (pending.kind === "codex") {
    return {
      source: "codex",
      sourceLabel: "Codex",
      ariaLabel: "Codex te pide permiso",
      showTerminal: false,
      alwaysTitle: "Permitir esto hasta que cierres Dot",
      failedText: "No se pudo enviar tu respuesta. Inténtalo de nuevo.",
    };
  }
  if (pending.kind === "dot-tool") {
    return {
      source: "dot-tool",
      sourceLabel: "Dot",
      ariaLabel: `Dot quiere usar ${pending.tool}`,
      showTerminal: false,
      alwaysTitle: "Permitir este comando exacto hasta que cierres Dot",
      failedText: "No se pudo enviar tu respuesta. Inténtalo de nuevo.",
    };
  }
  return {
    source: "claude-code",
    sourceLabel: "Claude Code",
    ariaLabel: `Claude Code te necesita en ${pending.project}`,
    showTerminal: true,
    alwaysTitle: "Permitir y recordar esta regla",
    failedText: "No se pudo enviar tu respuesta. Responde en la terminal.",
  };
}

/**
 * The text of a Codex card. The detail is the command or the files; the summary is shown on top only when the detail does not
 * already contain it (a file change's summary names the files, a command's detail starts with the command itself).
 */
export function codexCardBody(pending: PendingView): { heading: string; summary: string | null; detail: string } {
  const heading = "Codex quiere ejecutar o cambiar algo en tu equipo";
  if (pending.detail === "") return { heading, summary: null, detail: pending.summary };
  return { heading, summary: pending.detail.includes(pending.summary) ? null : pending.summary, detail: pending.detail };
}

/** The buttons of a permission card, in the order they are laid out. Questions are answered by their options instead. */
export function availableChoices(pending: PendingView): AgentChoice[] {
  const choices: AgentChoice[] = ["deny"];
  if (pending.alwaysOffered) choices.push("always");
  choices.push("allow");
  if (cardView(pending).showTerminal) choices.push("terminal");
  return choices;
}

export function choiceLabel(choice: AgentChoice, kind: PendingKind): string {
  if (choice === "always") return kind === "codex" ? "Siempre en esta sesión" : "Siempre";
  if (choice === "allow") return "Permitir";
  if (choice === "deny") return "Denegar";
  return "Responder en la terminal";
}
