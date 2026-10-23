"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { activityList, agentAttention, COMPUTER_USE_TEXT } from "../../lib/agent-state.ts";
import { chatGate } from "../../lib/engine-client.ts";
import type { ClaudeTaskView } from "../../lib/agent-state.ts";
import type { SessionState } from "../../lib/agent-sessions.ts";
import { mascotIndicator } from "../../lib/capture-state.ts";
import { nextMascotMode, type WindowMode } from "../../lib/window-mode.ts";
import { useAuthConnection } from "../auth-panel";
import { CompactChat, type ChatSeed } from "../chat/compact-chat";
import { EngineNote } from "../chat/engine-note";
import { useEngine } from "../chat/use-engine";
import { approvalKeyChoice } from "./agent-choices.ts";
import { AgentCard } from "./agent-card";
import { Icon } from "./icons";
import { ACTIVITY_ASK, ACTIVITY_EMPTY, ACTIVITY_TITLE, activityCountLabel, activitySourceLabel, approvalTitle, compactLine, inProgressLabel, isActive, islandLabel, newlyFailed, newlyFinished, orbiStateFor, raisedCardFor, restingMode, restingTarget, sessionDots, sessionStates, settleMode, splitAction, taskStates } from "./island-state.ts";
import { Orbi } from "./orbi";
import { useAgent } from "./use-agent";
import { useCaptureView } from "./use-capture";
import { useSleep } from "./use-sleep";
import { useMascotWindow } from "./use-window";
import { useVoice } from "./use-voice";
import { voiceAvailability } from "./voice-result.ts";

/** How long the pill stays after a session finishes, so the user notices that something is new. */
const FINISHED_MS = 8000;
/** How long the pill stays after the pointer leaves the strip it unfolded from. */
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

/** Dot as an island at the top of the screen: invisible at rest (hover the top edge to unfold it), a pill while something happens, and cards for sessions, permission and chat. */
export function Mascot({ canonicalOrigin }: { canonicalOrigin: string }) {
  const auth = useAuthConnection(canonicalOrigin);
  const win = useMascotWindow();
  const capture = useCaptureView();
  const agent = useAgent();
  const engine = useEngine({ chatOpen: win.mode === "chat" });
  const [seed, setSeed] = useState<ChatSeed | null>(null);
  const [chatOpened, setChatOpened] = useState(false);
  const [hovered, setHovered] = useState(false);
  const [finishedRecently, setFinishedRecently] = useState(false);
  const [failedRecently, setFailedRecently] = useState(false);
  const { mode, setMode, hidden, morph, leaving } = win;
  const expanded = mode !== "collapsed" && mode !== "compact";

  const voice = useVoice({
    onTranscript: (text) => {
      setSeed({ id: Date.now(), text });
      void setMode("chat");
    },
  });

  const { pending } = agent.state;
  // What Dot is doing for the user: Codex chats in progress, the background Claude Code tasks and the Claude Code sessions.
  const hookSessions = agent.state.sessions;
  const { tasks, codexTurns } = agent.activity;
  const sessions = useMemo(() => activityList({ sessions: hookSessions, tasks, codexTurns, now: Date.now() }), [hookSessions, tasks, codexTurns]);
  // While Dot controls Chrome or the mouse it says so, and the island is never at rest or hidden from it.
  const computerUse = agent.activity.computerUse.active;
  const pendingId = pending?.id ?? null;
  const { indicator } = capture;
  // Capture keeps the ring, the visible text and the pause control; listening has its own cancel and the character's working look.
  const attention = agentAttention({ ...agent.state, sessions });
  const view = mascotIndicator(indicator, voice.listening, attention);
  const { sharing } = view;
  const active = isActive({ attention, sharing, listening: voice.listening, finishedRecently: finishedRecently || failedRecently, computerUse });
  // Orbi dozes off after about three minutes of nothing going on; the pointer on the island wakes it.
  const { asleep, wake } = useSleep({ quiet: !active });
  // Something going on (an agent working, the screen shared, Voz listening) keeps the island as the pill, never as the resting strip.
  // Hidden by the user (tray: "Ocultar Dot"), the island is nothing at rest; only a shared screen still shows the pill.
  const resting = restingMode({ active, hovered, hidden, sharing: sharing || computerUse });
  const restingQuiet = restingMode({ active, hovered: false, hidden, sharing: sharing || computerUse });
  const label = islandLabel({ sharingText: view.sharingText, attention, listening: voice.listening, pending });
  const orbiState = orbiStateFor({ attention, listening: voice.listening, finished: finishedRecently, failed: failedRecently, asleep });

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
    void setMode("approval");
    // Only a new request forces the card; the user's own mode changes in between are theirs.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pendingId]);

  // Hiding Dot while a request waits must never hide the request: the card comes back up (without focus) at once.
  useEffect(() => {
    if (!hidden || !pendingId || mode === "approval") return;
    modeBeforeCard.current = mode;
    void setMode("approval");
    // Only the moment the user hides Dot matters; the effect above raises a new request's card.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hidden]);

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

  // The strip and the pill follow what is going on: the pill while it lasts or while the pointer is on the strip, the strip otherwise.
  // The card counts as up only once the window is in the approval mode; until then the resting rule leaves it alone.
  cardRaisedFor.current = raisedCardFor(cardRaisedFor.current, mode, pendingId);
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

  // A background task that just failed is news too: Orbi looks worried for the same few seconds.
  const knownTasks = useRef<Record<string, ClaudeTaskView["state"]>>({});
  const failedTimer = useRef<number | undefined>(undefined);
  useEffect(() => {
    const failed = newlyFailed(knownTasks.current, tasks);
    knownTasks.current = taskStates(tasks);
    if (!failed) return;
    setFailedRecently(true);
    window.clearTimeout(failedTimer.current);
    failedTimer.current = window.setTimeout(() => setFailedRecently(false), FINISHED_MS);
  }, [tasks]);

  const peekTimer = useRef<number | undefined>(undefined);
  useEffect(
    () => () => {
      window.clearTimeout(finishedTimer.current);
      window.clearTimeout(failedTimer.current);
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

  // Hovering the strip unfolds the pill; leaving it puts the strip back after a moment.
  const onPointerEnter = () => {
    wake();
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
  const countLabel = activityCountLabel(sessions);
  const line = compactLine({ sessions, pending, listening: voice.listening, finishedRecently, computerUse });
  const sharingNow = view.sharing && view.pause;

  return (
    <main
      className="mascot-root"
      data-mode={mode}
      data-ring={view.ring}
      data-listening={view.listening}
      data-agent={view.agent}
      data-sharing={sharing}
      data-hidden={hidden}
      data-leaving={leaving}
      onPointerEnter={onPointerEnter}
      onPointerLeave={onPointerLeave}
      onPointerMove={wake}
    >
      <div className="island" data-mode={mode} data-morph={morph.flip} data-grows={morph.grows} style={{ "--morph-x": `${morph.x}%`, "--morph-y": `${morph.y}%` } as React.CSSProperties}>
        <header className="island-head">
          <div
            className="island-toggle"
            role="button"
            tabIndex={0}
            aria-label={label}
            aria-expanded={expanded}
            title={label}
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
                <Orbi state={orbiState} size={34} ring={view.ring} />
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
              {mode === "chat" && <Orbi state={orbiState} size={28} ring={view.ring} />}
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
              {computerUse && (
                <div className="island-capture island-computer is-live" role="status" aria-label={COMPUTER_USE_TEXT}>
                  <span className="island-capture-dot" aria-hidden="true" />
                  <span>{COMPUTER_USE_TEXT}</span>
                  <button type="button" disabled={agent.stopping} onClick={() => void agent.stopComputer()}>Detener</button>
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
            <section className="island-card island-sessions" aria-label="Actividad de Dot">
              <Orbi state={orbiState} size={92} ring={view.ring} />
              <div className="island-sessions-main">
                <h2 className="island-section-title">{ACTIVITY_TITLE}</h2>
                {sessions.length > 0 ? (
                  <ul className="menu-sessions" aria-label={ACTIVITY_TITLE}>
                    {sessions.map((session) => (
                      <li className="menu-session" key={session.id} data-state={SESSION_LOOK[session.state]}>
                        <span className="menu-session-head">
                          <span className="session-dot" aria-hidden="true" />
                          <b>{session.project}</b>
                          <span className="menu-session-state">{session.state}</span>
                        </span>
                        <small title={session.action ?? undefined}>
                          <span className="menu-session-source">{activitySourceLabel(session)}</span>
                          {session.action && <> · <ActionText action={session.action} /></>}
                        </small>
                      </li>
                    ))}
                  </ul>
                ) : (
                  <div className="island-empty-state">
                    <p className="island-empty">{ACTIVITY_EMPTY}</p>
                    <button className="island-ask" type="button" onClick={() => void setMode("chat")}>{ACTIVITY_ASK}</button>
                  </div>
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
            {(() => {
              const gate = chatGate({ authConnected: auth.status === null ? null : auth.status.connected, engine: engine.status });
              if (gate.kind === "checking") return <p className="bubble-note" role="status">Comprobando la conexión…</p>;
              if (gate.kind === "chat") {
                return (
                  <CompactChat
                    key={gate.engine}
                    engine={gate.engine}
                    engineState={engine}
                    seed={seed}
                    onAuthorizationExpired={() => auth.setStatus({ connected: false, reauthorizationRequired: true })}
                  />
                );
              }
              return (
                <div className="bubble-connect">
                  <EngineNote engine={engine} signInShown />
                  <p>{auth.status?.reauthorizationRequired ? "Tu sesión de ChatGPT expiró. Conéctate de nuevo para chatear con Dot." : "Para chatear con Dot, conéctalo con tu cuenta de ChatGPT."}</p>
                  <button className="menu-button primary" type="button" onClick={() => void auth.connect()} disabled={auth.busy}>
                    {auth.busy ? "Abriendo ChatGPT…" : "Conectar con ChatGPT"}
                  </button>
                  <p className="bubble-note">El inicio de sesión se completa en tu navegador. Tus credenciales se quedan en este equipo.</p>
                  {auth.message && <p className="bubble-note" role="status">{auth.message}</p>}
                </div>
              );
            })()}
          </section>
        )}
      </div>
    </main>
  );
}
