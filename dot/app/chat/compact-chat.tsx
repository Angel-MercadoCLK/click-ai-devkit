"use client";

import { useEffect, useState } from "react";
import { historyEntries } from "../../lib/chat-history-view.ts";
import { Composer, MessageList, type ChatLabels } from "./parts";
import { useChat } from "../chat-panel";

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

/**
 * The conversations saved on this device, newest first: open one to continue it, delete one after a confirmation. It is the
 * same history the home page lists; only the way it is shown is the island's.
 */
function HistoryView({ chat, onOpen }: { chat: ReturnType<typeof useChat>; onOpen: () => void }) {
  const [confirming, setConfirming] = useState<string | null>(null);
  // Taken once when the view opens, so "hace 5 min" does not change under the pointer.
  const [now] = useState(() => Date.now());
  const entries = historyEntries(chat.history.conversations, now);
  return (
    <section className="history-view" aria-label="Historial de chats">
      {chat.historyLoading ? (
        <p className="history-empty" role="status">Cargando el historial…</p>
      ) : entries.length === 0 ? (
        <p className="history-empty">Todavía no hay chats guardados. Tu próxima conversación aparecerá aquí.</p>
      ) : (
        <ul className="history-rows">
          {entries.map((entry) => (
            <li className={`history-row${entry.id === chat.activeId ? " is-active" : ""}`} key={entry.id}>
              {confirming === entry.id ? (
                <div className="history-confirm" role="alertdialog" aria-label={`Confirmar: borrar ${entry.title}`}>
                  <span className="history-confirm-text">¿Borrar este chat?</span>
                  <button className="history-danger" type="button" onClick={() => { setConfirming(null); void chat.removeConversation(entry.id); }}>Borrar</button>
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
                  <button className="history-delete" type="button" aria-label={`Borrar ${entry.title}`} title="Borrar" onClick={() => setConfirming(entry.id)}>×</button>
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
export function CompactChat({ onAuthorizationExpired, seed }: { onAuthorizationExpired: () => void; seed: ChatSeed | null }) {
  const chat = useChat({ onAuthorizationExpired });
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
        <select aria-label="Modelo" value={chat.selectedModel} onChange={(event) => chat.setSelectedModel(event.target.value)} disabled={!chat.models.length || chat.busy}>
          {!chat.models.length && <option value="">{chat.loadingModels ? "Cargando modelos…" : "Sin modelos"}</option>}
          {chat.models.map((model) => <option key={model.slug} value={model.slug}>{model.displayName}</option>)}
        </select>
        {!chat.models.length && !chat.loadingModels && <button className="compact-link" type="button" onClick={() => void loadModels()}>Reintentar</button>}
        <button className="compact-link" type="button" aria-pressed={showHistory} onClick={() => setShowHistory((current) => !current)}>{showHistory ? "Volver al chat" : "Historial"}</button>
        <button className="compact-link" type="button" onClick={() => { chat.beginNewConversation(); setShowHistory(false); }}>Nuevo chat</button>
      </div>
      {showHistory ? (
        <HistoryView chat={chat} onOpen={() => setShowHistory(false)} />
      ) : (
        <>
          <MessageList chat={chat} labels={COMPACT_LABELS} />
          {chat.notice && <p className="compact-notice" role="status">{chat.notice}</p>}
          <Composer chat={chat} labels={COMPACT_LABELS} rows={2} />
        </>
      )}
    </div>
  );
}
