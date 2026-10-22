"use client";

import { useEffect } from "react";
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
  cancel: "Cancelar",
  send: "Enviar",
};

/** The chat bubble next to the mascot: the same chat as the home page, in a compact layout. */
export function CompactChat({ onAuthorizationExpired, seed }: { onAuthorizationExpired: () => void; seed: ChatSeed | null }) {
  const chat = useChat({ onAuthorizationExpired });
  const { loadModels, setDraft } = chat;

  // The bubble has no "Load models" step: the list is requested as soon as the bubble exists.
  useEffect(() => {
    void loadModels();
  }, []);

  useEffect(() => {
    if (!seed) return;
    setDraft((current) => (current.trim() ? `${current.trim()} ${seed.text}` : seed.text));
  }, [seed?.id]);

  return (
    <div className="compact-chat">
      <div className="compact-toolbar">
        <select aria-label="Modelo" value={chat.selectedModel} onChange={(event) => chat.setSelectedModel(event.target.value)} disabled={!chat.models.length || chat.busy}>
          {!chat.models.length && <option value="">{chat.loadingModels ? "Cargando modelos…" : "Sin modelos"}</option>}
          {chat.models.map((model) => <option key={model.slug} value={model.slug}>{model.displayName}</option>)}
        </select>
        {!chat.models.length && !chat.loadingModels && <button className="compact-link" type="button" onClick={() => void loadModels()}>Reintentar</button>}
        <button className="compact-link" type="button" onClick={chat.beginNewConversation}>Nuevo chat</button>
      </div>
      <MessageList chat={chat} labels={COMPACT_LABELS} />
      {chat.notice && <p className="compact-notice" role="status">{chat.notice}</p>}
      <Composer chat={chat} labels={COMPACT_LABELS} rows={2} />
    </div>
  );
}
