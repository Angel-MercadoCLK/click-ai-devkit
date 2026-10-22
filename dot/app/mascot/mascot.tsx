"use client";

import { useEffect, useRef, useState } from "react";
import { agentAttention } from "../../lib/agent-state.ts";
import { mascotIndicator } from "../../lib/capture-state.ts";
import { dragTarget, exceedsDragThreshold, nextMascotMode, type WindowMode } from "../../lib/window-mode.ts";
import { useAuthConnection } from "../auth-panel";
import { CompactChat, type ChatSeed } from "../chat/compact-chat";
import { AgentCard } from "./agent-card";
import { Spark } from "./spark";
import { useAgent } from "./use-agent";
import { useCaptureView } from "./use-capture";
import { useMascotWindow } from "./use-window";
import { useVoice } from "./use-voice";
import { voiceAvailability } from "./voice-result.ts";

/** Small drawn icons: font glyphs for pause and play are missing on some systems and would show as boxes. */
function Glyph({ kind }: { kind: "pause" | "play" | "cancel" }) {
  return (
    <svg viewBox="0 0 12 12" width="12" height="12" aria-hidden="true" focusable="false">
      {kind === "pause" && <path d="M3 2h2v8H3zM7 2h2v8H7z" fill="currentColor" />}
      {kind === "play" && <path d="M3.5 2l6.5 4-6.5 4z" fill="currentColor" />}
      {kind === "cancel" && <path d="M3 3l6 6M9 3l-6 6" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />}
    </svg>
  );
}

type Press = { pointerStart: { x: number; y: number }; windowStart: { x: number; y: number }; dragging: boolean };

/** The companion: a mascot that opens a three-button menu, a compact chat bubble and the Claude Code approval card. */
export function Mascot({ canonicalOrigin }: { canonicalOrigin: string }) {
  const auth = useAuthConnection(canonicalOrigin);
  const win = useMascotWindow();
  const capture = useCaptureView();
  const agent = useAgent();
  const [seed, setSeed] = useState<ChatSeed | null>(null);
  const [chatOpened, setChatOpened] = useState(false);
  const { mode, anchor, setMode } = win;

  const voice = useVoice({
    onTranscript: (text) => {
      setSeed({ id: Date.now(), text });
      void setMode("chat");
    },
  });

  const { pending } = agent.state;
  const pendingId = pending?.id ?? null;
  // The mode the card interrupted, so the menu or the chat comes back once the request is over.
  const modeBeforeCard = useRef<WindowMode | null>(null);

  // A new request brings the card up; the shell has already shown the mascot without taking focus. Putting the card
  // away by hand (the mascot, Escape) is respected until the next request arrives.
  useEffect(() => {
    if (!pendingId || mode === "approval") return;
    modeBeforeCard.current = mode;
    void setMode("approval");
    // Only a new request forces the card; the user's own mode changes in between are theirs.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pendingId]);

  // The card is gone (answered, handed back or timed out): leave its window size.
  useEffect(() => {
    if (mode !== "approval" || pendingId) return;
    const back = modeBeforeCard.current ?? "collapsed";
    modeBeforeCard.current = null;
    void setMode(back === "approval" ? "collapsed" : back);
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

  const press = useRef<Press | null>(null);
  const pendingMove = useRef<{ x: number; y: number } | null>(null);
  const moving = useRef(false);

  // The bubble stays mounted once opened, so a conversation in progress survives closing it.
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

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape" && mode !== "collapsed") void setMode("collapsed");
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [mode, setMode]);

  async function pumpMoves() {
    if (moving.current) return;
    moving.current = true;
    while (pendingMove.current) {
      const target = pendingMove.current;
      pendingMove.current = null;
      await win.moveTo(target.x, target.y);
    }
    moving.current = false;
  }

  // The mascot is dragged by hand instead of with a native drag region: a drag region swallows clicks, and the mascot must also be clickable.
  function onPointerDown(event: React.PointerEvent<HTMLDivElement>) {
    if (event.button !== 0) return;
    event.currentTarget.setPointerCapture(event.pointerId);
    press.current = {
      pointerStart: { x: event.screenX, y: event.screenY },
      windowStart: { x: window.screenX, y: window.screenY },
      dragging: false,
    };
  }

  function onPointerMove(event: React.PointerEvent<HTMLDivElement>) {
    const current = press.current;
    if (!current) return;
    if (!current.dragging && !exceedsDragThreshold(event.screenX - current.pointerStart.x, event.screenY - current.pointerStart.y)) return;
    current.dragging = true;
    pendingMove.current = dragTarget(current.windowStart, current.pointerStart, { x: event.screenX, y: event.screenY });
    void pumpMoves();
  }

  function onPointerUp(event: React.PointerEvent<HTMLDivElement>) {
    const current = press.current;
    press.current = null;
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
    if (current && !current.dragging) void setMode(nextMascotMode(mode, pendingId !== null));
  }

  const { indicator } = capture;
  // Capture keeps the ring, the visible text and the pause control; listening has its own ring, cancel and label part.
  const attention = agentAttention(agent.state);
  const view = mascotIndicator(indicator, voice.listening, attention);
  const { sharing } = view;
  const voiceChoice = voiceAvailability({ connected: auth.status === null ? null : auth.status.connected, listening: voice.listening });
  const pauseCapture = () => view.pause && void capture.act(view.pause.action);

  return (
    <main className="mascot-root" data-mode={mode} data-h={anchor.horizontal} data-v={anchor.vertical}>
      {mode === "menu" && (
        <nav className="mascot-menu" aria-label="Dot">
          <button
            className={`menu-button${sharing ? " is-live" : ""}`}
            type="button"
            aria-pressed={sharing}
            disabled={capture.busy || indicator === "unavailable"}
            title={indicator === "unavailable" ? "Compartir la pantalla solo está disponible en la app de escritorio." : undefined}
            onClick={() => void capture.act(sharing ? "disable" : "enable")}
          >
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
            <span className="menu-button-text">
              <span>{voice.listening ? "Escuchando… (cancelar)" : "Voz"}</span>
              {voiceChoice.hint && <small id="voice-hint">{voiceChoice.hint}</small>}
            </span>
          </button>
          <button className="menu-button" type="button" onClick={() => void setMode("chat")}>
            <span>Mensaje</span>
          </button>
          {agent.state.sessions.length > 0 && (
            <ul className="menu-sessions" aria-label="Sesiones de Claude Code">
              {agent.state.sessions.map((session) => (
                <li className="menu-session" key={session.id} data-state={session.state === "te necesita" ? "needs" : session.state === "trabajando" ? "working" : "done"}>
                  <span className="menu-session-head">
                    <b>{session.project}</b>
                    <span className="menu-session-state">{session.state}</span>
                  </span>
                  {session.action && <small title={session.action}>{session.action}</small>}
                </li>
              ))}
            </ul>
          )}
          {(voice.message || capture.failed) && <p className="menu-note" role="status" title={voice.message || undefined}>{voice.message || "No se pudo cambiar la captura de pantalla."}</p>}
        </nav>
      )}

      {mode === "menu" && sharing && (
        <div className={`menu-chip${indicator === "live" ? " is-live" : ""}`} role="status">
          <span>{indicator === "paused" ? "En pausa" : "Viendo tu pantalla"}</span>
          <button type="button" disabled={capture.busy} onClick={() => void capture.act(indicator === "paused" ? "resume" : "pause")}>
            {indicator === "paused" ? "Reanudar" : "Pausar"}
          </button>
        </div>
      )}

      {mode === "approval" && pending && (
        <AgentCard pending={pending} queued={agent.state.queued} busy={agent.busy} failed={agent.failed} onDecide={(choice, answers) => void agent.decide(pending.id, choice, answers)} />
      )}

      {chatOpened && (
        <section className="mascot-bubble" hidden={mode !== "chat"} aria-label="Chat con Dot">
          <header className="bubble-header">
            <span>Dot</span>
            {view.sharing && view.pause && (
              <div className={`bubble-chip${indicator === "live" ? " is-live" : ""}`} role="status">
                <span>{view.sharingText}</span>
                <button type="button" disabled={capture.busy} onClick={pauseCapture}>{view.pause.name}</button>
              </div>
            )}
            <button className="bubble-close" type="button" aria-label="Cerrar" onClick={() => void setMode("collapsed")}>×</button>
          </header>
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

      <div className="mascot-spot" data-ring={view.ring} data-listening={view.listening} data-agent={view.agent}>
        <div
          className="mascot-body"
          role="button"
          tabIndex={0}
          aria-label={view.label}
          aria-expanded={mode !== "collapsed"}
          title={view.label}
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
          onPointerCancel={() => { press.current = null; }}
          onKeyDown={(event) => {
            if (event.key === "Enter" || event.key === " ") {
              event.preventDefault();
              void setMode(nextMascotMode(mode, pendingId !== null));
            }
          }}
        >
          <Spark />
        </div>
        {view.agent !== "idle" && <span className="mascot-agent" aria-hidden="true">{view.agent === "needs" ? "!" : ""}</span>}
        {mode === "collapsed" && view.sharingText && <span className="mascot-label" role="status">{view.sharingText}</span>}
        {mode === "collapsed" && view.pause && (
          <button className="mascot-badge" type="button" aria-label={view.pause.name} title={view.pause.name} disabled={capture.busy} onClick={pauseCapture}><Glyph kind={view.pause.glyph} /></button>
        )}
        {mode !== "menu" && view.cancel && (
          <button className="mascot-badge is-cancel" type="button" aria-label="Cancelar" title="Escuchando… Cancelar" onClick={voice.cancel}><Glyph kind="cancel" /></button>
        )}
      </div>
    </main>
  );
}
