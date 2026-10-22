"use client";

import { useEffect, useRef, useState } from "react";
import { historyEntries } from "../../lib/chat-history-view.ts";
import { Composer, MessageList, type ChatLabels } from "./parts";
import { useChat } from "../chat-panel";
import { EngineNote } from "./engine-note";
import type { EngineState } from "./use-engine";

/** A transcript from voice: it lands in the composer for the user to review, and is never sent by itself. */
export type ChatSeed = { id: number; text: string };

const COMPACT_LABELS: ChatLabels = {
  you: "Tú",
  assistant: "ChatGPT",
  youSubmitted: "Tú · enviado",
  assistantSubmitted: "ChatGPT · enviado",
  assistantStreaming: "ChatGPT · respondiendo",
  waitingForToken: "Esperando la respuesta…",
  youNotSaved: "Tú · no guardado",
  partialNotSaved: "Respuesta parcial · no guardada",
  assistantCompleted: "ChatGPT · completo",
  empty: "Escribe un mensaje para empezar.",
  message: "Mensaje",
  placeholder: "Pregúntale algo a Dot…",
  includeScreen: "Incluir mi pantalla en este mensaje",
  screenHint: "Empieza a escribir y Dot preparará tu pantalla sin datos personales.",
  screenPreparing: "Ocultando datos personales en tu pantalla…",
  screenAlt: "Vista previa de la pantalla que se enviará, sin datos personales",
  regionHidden: "1 zona oculta",
  regionsHidden: (count) => `${count} zonas ocultas`,
  screenCaveat: "El reconocimiento de texto puede fallar y las caras y fotos no se ocultan. Esta imagen se envía a ChatGPT y no se guarda aquí.",
  tryAgain: "Reintentar",
  sendWithoutScreen: "Enviar sin la pantalla",
  footerNote: "",
  cancel: "Detener",
  send: "Enviar",
};

/** With Codex as the engine the assistant is Codex, not ChatGPT. */
const CODEX_LABELS: ChatLabels = {
  ...COMPACT_LABELS,
  assistant: "Codex",
  assistantSubmitted: "Codex · enviado",
  assistantStreaming: "Codex · respondiendo",
  partialNotSaved: "Respuesta parcial",
  assistantCompleted: "Codex · completo",
  placeholder: "Pídele algo a Dot… (puedes soltar archivos aquí)",
  screenCaveat: "",
};

/**
 * The conversations saved on this device, newest first: open one to continue it, delete one after a confirmation. It is the
 * same history the home page lists; only the way it is shown is the island's.
 */
function HistoryView({ chat, onOpen }: { chat: ReturnType<typeof useChat>; onOpen: () => void }) {
  const [confirming, setConfirming] = useState<string | null>(null);
  const codex = chat.engine === "codex";
  const [search, setSearch] = useState(chat.historyQuery);
  const searched = useRef(chat.historyQuery);
  // Codex searches its own threads; the search waits for the user to stop typing.
  useEffect(() => {
    if (!codex || search === searched.current) return;
    const timer = window.setTimeout(() => {
      searched.current = search;
      void chat.searchHistory(search);
    }, 300);
    return () => window.clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [search, codex]);
  // Taken once when the view opens, so "hace 5 min" does not change under the pointer.
  const [now] = useState(() => Date.now());
  const entries = historyEntries(chat.history.conversations, now);
  return (
    <section className="history-view" aria-label="Historial de chats">
      {codex && <input className="history-search" type="search" aria-label="Buscar en tus chats de Codex" placeholder="Buscar en tus chats…" value={search} onChange={(event) => setSearch(event.target.value)} />}
      {chat.historyLoading ? (
        <p className="history-empty" role="status">Cargando el historial…</p>
      ) : entries.length === 0 ? (
        <p className="history-empty">{codex && search ? "Ningún chat coincide con tu búsqueda." : "Todavía no hay chats guardados. Tu próxima conversación aparecerá aquí."}</p>
      ) : (
        <ul className="history-rows">
          {entries.map((entry) => (
            <li className={`history-row${entry.id === chat.activeId ? " is-active" : ""}`} key={entry.id}>
              {confirming === entry.id ? (
                <div className="history-confirm" role="alertdialog" aria-label={`Confirmar: borrar ${entry.title}`}>
                  <span className="history-confirm-text">{codex ? "¿Archivar este chat?" : "¿Borrar este chat?"}</span>
                  <button className="history-danger" type="button" onClick={() => { setConfirming(null); void chat.removeConversation(entry.id); }}>{codex ? "Archivar" : "Borrar"}</button>
                  <button className="history-keep" type="button" onClick={() => setConfirming(null)}>Cancelar</button>
                </div>
              ) : (
                <>
                  <button
                    className="history-open"
                    type="button"
                    onClick={() => {
                      const conversation = chat.history.conversations.find((item) => item.id === entry.id);
                      if (!conversation) return;
                      chat.openConversation(conversation);
                      onOpen();
                    }}
                  >
                    <span className="history-title">{entry.title}</span>
                    <small className="history-when">{entry.when}</small>
                  </button>
                  <button className="history-delete" type="button" aria-label={`${codex ? "Archivar" : "Borrar"} ${entry.title}`} title={codex ? "Archivar" : "Borrar"} onClick={() => setConfirming(entry.id)}>×</button>
                </>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

/** The chat card of the island: the same chat as the home page, in a compact layout, with its history one click away. */
export function CompactChat({ onAuthorizationExpired, seed, engine, engineState }: { onAuthorizationExpired: () => void; seed: ChatSeed | null; engine: "codex" | "builtin"; engineState: EngineState }) {
  const chat = useChat({ onAuthorizationExpired, engine });
  const codex = engine === "codex";
  const { loadModels, setDraft } = chat;
  const [showHistory, setShowHistory] = useState(false);

  // The bubble has no "Load models" step: the list is requested as soon as the bubble exists.
  useEffect(() => {
    void loadModels();
  }, []);

  useEffect(() => {
    if (!seed) return;
    setDraft((current) => (current.trim() ? `${current.trim()} ${seed.text}` : seed.text));
    // A transcript lands in the message box, so the card goes back to the conversation.
    setShowHistory(false);
  }, [seed?.id]);

  return (
    <div className="compact-chat">
      <div className="compact-toolbar">
        {codex ? (
          <span className="compact-engine" title="Dot usa tu Codex con tu cuenta de ChatGPT">Codex</span>
        ) : (
          <select aria-label="Modelo" value={chat.selectedModel} onChange={(event) => chat.setSelectedModel(event.target.value)} disabled={!chat.models.length || chat.busy}>
            {!chat.models.length && <option value="">{chat.loadingModels ? "Cargando modelos…" : "Sin modelos"}</option>}
            {chat.models.map((model) => <option key={model.slug} value={model.slug}>{model.displayName}</option>)}
          </select>
        )}
        {!codex && !chat.models.length && !chat.loadingModels && <button className="compact-link" type="button" onClick={() => void loadModels()}>Reintentar</button>}
        <button className="compact-link" type="button" aria-pressed={showHistory} onClick={() => setShowHistory((current) => !current)}>{showHistory ? "Volver al chat" : "Historial"}</button>
        <button className="compact-link" type="button" onClick={() => { chat.beginNewConversation(); setShowHistory(false); }}>Nuevo chat</button>
      </div>
      {showHistory ? (
        <HistoryView chat={chat} onOpen={() => setShowHistory(false)} />
      ) : (
        <>
          <MessageList chat={chat} labels={codex ? CODEX_LABELS : COMPACT_LABELS} />
          {chat.notice && <p className="compact-notice" role="status">{chat.notice}</p>}
          <EngineNote engine={engineState} />
          <Composer chat={chat} labels={codex ? CODEX_LABELS : COMPACT_LABELS} rows={2} />
        </>
      )}
    </div>
  );
}
