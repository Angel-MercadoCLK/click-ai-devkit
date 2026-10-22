"use client";

import { useEffect, useRef, useState } from "react";
import { dragTarget, exceedsDragThreshold, nextMascotMode } from "../../lib/window-mode.ts";
import { useAuthConnection } from "../auth-panel";
import { CompactChat, type ChatSeed } from "../chat/compact-chat";
import { Spark } from "./spark";
import { useCaptureView } from "./use-capture";
import { useMascotWindow } from "./use-window";
import { useVoice } from "./use-voice";

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

/** The companion: a mascot that opens a three-button menu and a compact chat bubble. */
export function Mascot({ canonicalOrigin }: { canonicalOrigin: string }) {
  const auth = useAuthConnection(canonicalOrigin);
  const win = useMascotWindow();
  const capture = useCaptureView();
  const [seed, setSeed] = useState<ChatSeed | null>(null);
  const [chatOpened, setChatOpened] = useState(false);
  const { mode, anchor, setMode } = win;

  const voice = useVoice({
    onTranscript: (text) => {
      setSeed({ id: Date.now(), text });
      void setMode("chat");
    },
  });

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
    if (current && !current.dragging) void setMode(nextMascotMode(mode));
  }

  const { indicator } = capture;
  const sharing = indicator === "live" || indicator === "paused";
  const ring = voice.listening ? "listening" : indicator;
  const label = voice.listening ? "Escuchando…" : indicator === "live" ? "Viendo tu pantalla" : indicator === "paused" ? "Pantalla en pausa" : "Dot";

  const badge = voice.listening
    ? { glyph: "cancel" as const, name: "Cancelar", run: voice.cancel }
    : sharing
      ? { glyph: indicator === "paused" ? ("play" as const) : ("pause" as const), name: indicator === "paused" ? "Reanudar" : "Pausar", run: () => void capture.act(indicator === "paused" ? "resume" : "pause") }
      : undefined;

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
          <button className={`menu-button${voice.listening ? " is-listening" : ""}`} type="button" onClick={() => (voice.listening ? voice.cancel() : void voice.start())}>
            <span>{voice.listening ? "Escuchando… (cancelar)" : "Voz"}</span>
          </button>
          <button className="menu-button" type="button" onClick={() => void setMode("chat")}>
            <span>Mensaje</span>
          </button>
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

      {chatOpened && (
        <section className="mascot-bubble" hidden={mode !== "chat"} aria-label="Chat con Dot">
          <header className="bubble-header">
            <span>Dot</span>
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

      <div className="mascot-spot" data-ring={ring}>
        <div
          className="mascot-body"
          role="button"
          tabIndex={0}
          aria-label={label}
          aria-expanded={mode !== "collapsed"}
          title={label}
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
          onPointerCancel={() => { press.current = null; }}
          onKeyDown={(event) => {
            if (event.key === "Enter" || event.key === " ") {
              event.preventDefault();
              void setMode(nextMascotMode(mode));
            }
          }}
        >
          <Spark />
        </div>
        {badge && mode === "collapsed" && (
          <button className="mascot-badge" type="button" aria-label={badge.name} title={badge.name} onClick={badge.run}><Glyph kind={badge.glyph} /></button>
        )}
      </div>
    </main>
  );
}
