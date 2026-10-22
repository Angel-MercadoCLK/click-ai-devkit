"use client";

import { useEffect, useRef, useState } from "react";
import { agentAttention } from "../../lib/agent-state.ts";
import type { SessionState } from "../../lib/agent-sessions.ts";
import { mascotIndicator } from "../../lib/capture-state.ts";
import { nextMascotMode, type WindowMode } from "../../lib/window-mode.ts";
import { useAuthConnection } from "../auth-panel";
import { CompactChat, type ChatSeed } from "../chat/compact-chat";
import { approvalKeyChoice } from "./agent-choices.ts";
import { AgentCard } from "./agent-card";
import { Icon } from "./icons";
import { approvalTitle, compactLine, inProgressLabel, isActive, lenteStateFor, newlyFinished, restingMode, restingTarget, sessionCountLabel, sessionDots, sessionStates, settleMode, splitAction } from "./island-state.ts";
import { Lente } from "./lente";
import { useAgent } from "./use-agent";
import { useCaptureView } from "./use-capture";
import { useMascotWindow } from "./use-window";
import { useVoice } from "./use-voice";
import { voiceAvailability } from "./voice-result.ts";

/** How long the pill stays after a session finishes, so the user notices that something is new. */
const FINISHED_MS = 8000;
/** How long the pill stays after the pointer leaves the thin bar it peeked out of. */
const PEEK_MS = 700;

const SESSION_LOOK: Record<SessionState, "needs" | "working" | "done"> = { "te necesita": "needs", trabajando: "working", listo: "done" };

/** An action line with its +N and −M counts colored like a diff. */
function ActionText({ action }: { action: string | null }) {
  const { text, added, removed } = splitAction(action);
  return (
    <>
      {text}
      {added !== null && <span className="diff-add"> +{added}</span>}
      {removed !== null && <span className="diff-del"> −{removed}</span>}
    </>
  );
}

/** Dot as an island at the top of the screen: a thin bar, a pill while something happens, and cards for sessions, permission and chat. */
export function Mascot({ canonicalOrigin }: { canonicalOrigin: string }) {
  const auth = useAuthConnection(canonicalOrigin);
  const win = useMascotWindow();
  const capture = useCaptureView();
  const agent = useAgent();
  const [seed, setSeed] = useState<ChatSeed | null>(null);
  const [chatOpened, setChatOpened] = useState(false);
  const [hovered, setHovered] = useState(false);
  const [finishedRecently, setFinishedRecently] = useState(false);
  const { mode, setMode } = win;
  const expanded = mode !== "collapsed" && mode !== "compact";

  const voice = useVoice({
    onTranscript: (text) => {
      setSeed({ id: Date.now(), text });
      void setMode("chat");
    },
  });

  const { pending, sessions } = agent.state;
  const pendingId = pending?.id ?? null;
  const { indicator } = capture;
  // Capture keeps the ring, the visible text and the pause control; listening has its own cancel and the character's working look.
  const attention = agentAttention(agent.state);
  const view = mascotIndicator(indicator, voice.listening, attention);
  const { sharing } = view;
  const active = isActive({ attention, sharing, listening: voice.listening, finishedRecently });
  // Something going on (an agent working, the screen shared, Voz listening) keeps the island as the pill, never as the thin bar.
  const resting = restingMode({ active, hovered });
  const restingQuiet = restingMode({ active, hovered: false });
  const lenteState = lenteStateFor({ attention, listening: voice.listening, sessions });

  // The mode the card interrupted, so the menu or the chat comes back once the request is over.
  const modeBeforeCard = useRef<WindowMode | null>(null);
  const restingQuietRef = useRef(restingQuiet);
  restingQuietRef.current = restingQuiet;

  // A new request brings the card up; the shell has already shown the island without taking focus. Putting the card
  // away by hand (the island, Escape) is respected until the next request arrives.
  const cardRaisedFor = useRef<string | null>(null);
  useEffect(() => {
    if (!pendingId || mode === "approval") return;
    modeBeforeCard.current = mode;
    cardRaisedFor.current = pendingId;
    void setMode("approval");
    // Only a new request forces the card; the user's own mode changes in between are theirs.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pendingId]);

  // The card is gone (answered, handed back or timed out): leave its window size.
  useEffect(() => {
    if (mode !== "approval" || pendingId) return;
    const back = modeBeforeCard.current ?? "collapsed";
    modeBeforeCard.current = null;
    void setMode(settleMode(back === "approval" ? "collapsed" : back, restingQuietRef.current));
  }, [mode, pendingId, setMode]);

  // The card is on screen once it has rendered in a window that has its size: tell the server, or it hands the request
  // back to the terminal after 2 s.
  const cardShown = mode === "approval" && pendingId !== null;
  useEffect(() => {
    if (!cardShown || !pendingId) return;
    const frame = window.requestAnimationFrame(() => void agent.ack(pendingId));
    return () => window.cancelAnimationFrame(frame);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cardShown, pendingId]);

  // The bar and the pill follow what is going on: the pill while it lasts or while the pointer is on the bar, the thin bar otherwise.
  // A request waiting for its card is handled above, so the two never fight over the window size.
  useEffect(() => {
    const target = restingTarget({ mode, resting, pendingId, raisedFor: cardRaisedFor.current });
    if (target) void setMode(target);
  }, [mode, resting, setMode, pendingId]);

  // A session that just finished is news: the pill stays a few seconds.
  const knownStates = useRef<Record<string, SessionState>>({});
  const finishedTimer = useRef<number | undefined>(undefined);
  useEffect(() => {
    const news = newlyFinished(knownStates.current, sessions);
    knownStates.current = sessionStates(sessions);
    if (!news) return;
    setFinishedRecently(true);
    window.clearTimeout(finishedTimer.current);
    finishedTimer.current = window.setTimeout(() => setFinishedRecently(false), FINISHED_MS);
  }, [sessions]);

  const peekTimer = useRef<number | undefined>(undefined);
  useEffect(
    () => () => {
      window.clearTimeout(finishedTimer.current);
      window.clearTimeout(peekTimer.current);
    },
    [],
  );

  // The chat stays mounted once opened, so a conversation in progress survives closing it.
  useEffect(() => {
    if (mode !== "chat") return;
    setChatOpened(true);
    const frame = window.requestAnimationFrame(() => document.getElementById("message-input")?.focus());
    return () => window.cancelAnimationFrame(frame);
  }, [mode, seed?.id]);

  // Coming back from the browser after signing in: look at the connection again.
  useEffect(() => {
    const onFocus = () => {
      if (!auth.status?.connected) void auth.refresh();
    };
    window.addEventListener("focus", onFocus);
    return () => window.removeEventListener("focus", onFocus);
  }, [auth.status?.connected]);

  function close() {
    window.clearTimeout(peekTimer.current);
    setHovered(false);
    void setMode(settleMode("collapsed", restingQuiet));
  }

  // Escape puts a card away. While a permission card is shown and the island has focus, Y allows and N denies; the keys
  // are only heard here, in the island's own window, so no global shortcut is involved.
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape" && expanded) {
        close();
        return;
      }
      const choice = approvalKeyChoice(event, { pending, cardShown, busy: agent.busy, focused: document.hasFocus() });
      if (!choice || !pending) return;
      event.preventDefault();
      void agent.decide(pending.id, choice);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [expanded, pending, cardShown, agent.busy, restingQuiet]);

  function toggle() {
    const next = nextMascotMode(mode, pendingId !== null);
    if (next === "collapsed") close();
    else void setMode(next);
  }

  // Hovering the thin bar peeks the pill out; leaving it puts the bar back after a moment.
  const onPointerEnter = () => {
    window.clearTimeout(peekTimer.current);
    setHovered(true);
  };
  const onPointerLeave = () => {
    window.clearTimeout(peekTimer.current);
    peekTimer.current = window.setTimeout(() => setHovered(false), PEEK_MS);
  };

  const voiceChoice = voiceAvailability({ connected: auth.status === null ? null : auth.status.connected, listening: voice.listening });
  const pauseCapture = () => view.pause && void capture.act(view.pause.action);
  const dots = sessionDots(sessions);
  const countLabel = sessionCountLabel(sessions.length);
  const line = compactLine({ sessions, pending, listening: voice.listening, finishedRecently });
  const sharingNow = view.sharing && view.pause;

  return (
    <main
      className="mascot-root"
      data-mode={mode}
      data-ring={view.ring}
      data-listening={view.listening}
      data-agent={view.agent}
      data-sharing={sharing}
      onPointerEnter={onPointerEnter}
      onPointerLeave={onPointerLeave}
    >
      <div className="island" data-mode={mode}>
        <header className="island-head">
          <div
            className="island-toggle"
            role="button"
            tabIndex={0}
            aria-label={view.label}
            aria-expanded={expanded}
            title={view.label}
            onClick={toggle}
            onKeyDown={(event) => {
              if (event.key === "Enter" || event.key === " ") {
                event.preventDefault();
                toggle();
              }
            }}
          >
            {mode === "compact" && (
              <>
                <Lente state={lenteState} size={34} ring={view.ring} />
                <span className="island-line" title={line}>
                  <ActionText action={line} />
                </span>
                {countLabel && <span className="island-count">{countLabel}</span>}
                {dots.length > 0 && (
                  <span className="island-dots" aria-hidden="true">
                    {dots.map((state, index) => <span className="island-dot" data-state={SESSION_LOOK[state]} key={index} />)}
                  </span>
                )}
              </>
            )}
            {expanded && <Icon name="chevron-up" />}
          </div>
          {expanded && (
            <span className="island-title" data-needs={mode === "approval"}>
              {mode === "chat" && <Lente state={lenteState} size={28} ring={view.ring} />}
              {mode === "approval" && pending ? approvalTitle(pending) : "Dot"}
            </span>
          )}
          {mode !== "collapsed" && (
            <div className="island-side">
              {sharingNow && (
                <div className={`island-capture${indicator === "live" ? " is-live" : ""}`} role="status">
                  <span className="island-capture-dot" aria-hidden="true" />
                  <span>{view.sharingText}</span>
                  <button type="button" disabled={capture.busy} onClick={pauseCapture}>{view.pause?.name}</button>
                </div>
              )}
              {mode !== "menu" && view.cancel && (
                <button className="island-cancel" type="button" aria-label="Cancelar" title="Escuchando… Cancelar" onClick={voice.cancel}>
                  <Icon name="cancel" size={14} />
                  <span>Escuchando…</span>
                </button>
              )}
              {expanded && <span className="island-status">{mode === "approval" && agent.state.queued > 0 ? `${agent.state.queued} en espera` : inProgressLabel(sessions)}</span>}
            </div>
          )}
        </header>

        {mode === "menu" && (
          <div className="island-body island-menu">
            <section className="island-card island-sessions" aria-label="Estado de Dot">
              <Lente state={lenteState} size={76} ring={view.ring} />
              <div className="island-sessions-main">
                {sessions.length > 0 ? (
                  <ul className="menu-sessions" aria-label="Sesiones de Claude Code">
                    {sessions.map((session) => (
                      <li className="menu-session" key={session.id} data-state={SESSION_LOOK[session.state]}>
                        <span className="menu-session-head">
                          <span className="session-dot" aria-hidden="true" />
                          <b>{session.project}</b>
                          <span className="menu-session-state">{session.state}</span>
                        </span>
                        {session.action && <small title={session.action}><ActionText action={session.action} /></small>}
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="island-empty">Sin sesiones de Claude Code. Dot te avisa cuando haya una.</p>
                )}
              </div>
            </section>
            <nav className="mascot-menu island-card" aria-label="Dot">
              <button
                className={`menu-button${sharing ? " is-live" : ""}`}
                type="button"
                aria-pressed={sharing}
                disabled={capture.busy || indicator === "unavailable"}
                title={indicator === "unavailable" ? "Compartir la pantalla solo está disponible en la app de escritorio." : undefined}
                onClick={() => void capture.act(sharing ? "disable" : "enable")}
              >
                <Icon name="screen" />
                <span>Compartir mi pantalla</span>
              </button>
              <button
                className={`menu-button${voice.listening ? " is-listening" : ""}`}
                type="button"
                disabled={voiceChoice.disabled}
                aria-describedby={voiceChoice.hint ? "voice-hint" : undefined}
                title={voiceChoice.hint ?? undefined}
                onClick={() => (voice.listening ? voice.cancel() : void voice.start())}
              >
                <Icon name="mic" />
                <span className="menu-button-text">
                  <span>{voice.listening ? "Escuchando… (cancelar)" : "Voz"}</span>
                  {voiceChoice.hint && <small id="voice-hint">{voiceChoice.hint}</small>}
                </span>
              </button>
              <button className="menu-button" type="button" onClick={() => void setMode("chat")}>
                <Icon name="chat" />
                <span>Mensaje</span>
              </button>
              {(voice.message || capture.failed) && <p className="menu-note" role="status" title={voice.message || undefined}>{voice.message || "No se pudo cambiar la captura de pantalla."}</p>}
            </nav>
          </div>
        )}

        {mode === "approval" && pending && (
          <div className="island-body">
            <AgentCard pending={pending} busy={agent.busy} failed={agent.failed} onDecide={(choice, answers) => void agent.decide(pending.id, choice, answers)} />
          </div>
        )}

        {chatOpened && (
          <section className="island-body island-chat" hidden={mode !== "chat"} aria-label="Chat con Dot">
            {auth.status === null ? (
              <p className="bubble-note" role="status">Comprobando la conexión…</p>
            ) : auth.status.connected ? (
              <CompactChat seed={seed} onAuthorizationExpired={() => auth.setStatus({ connected: false, reauthorizationRequired: true })} />
            ) : (
              <div className="bubble-connect">
                <p>{auth.status.reauthorizationRequired ? "Tu sesión de ChatGPT expiró. Conéctate de nuevo para chatear con Dot." : "Para chatear con Dot, conéctalo con tu cuenta de ChatGPT."}</p>
                <button className="menu-button primary" type="button" onClick={() => void auth.connect()} disabled={auth.busy}>
                  {auth.busy ? "Abriendo ChatGPT…" : "Conectar con ChatGPT"}
                </button>
                <p className="bubble-note">El inicio de sesión se completa en tu navegador. Tus credenciales se quedan en este equipo.</p>
                {auth.message && <p className="bubble-note" role="status">{auth.message}</p>}
              </div>
            )}
          </section>
        )}
      </div>
    </main>
  );
}
