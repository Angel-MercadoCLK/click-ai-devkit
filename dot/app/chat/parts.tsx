"use client";

import type { Chat } from "../chat-panel";

/** Every string the shared parts show, so the full page and the compact bubble can speak their own wording. */
export type ChatLabels = {
  you: string;
  assistant: string;
  youSubmitted: string;
  assistantSubmitted: string;
  assistantStreaming: string;
  waitingForToken: string;
  youNotSaved: string;
  partialNotSaved: string;
  assistantCompleted: string;
  empty: string;
  message: string;
  placeholder: string;
  includeScreen: string;
  screenHint: string;
  screenPreparing: string;
  screenAlt: string;
  regionHidden: string;
  regionsHidden: (count: number) => string;
  screenCaveat: string;
  tryAgain: string;
  sendWithoutScreen: string;
  footerNote: string;
  cancel: string;
  send: string;
};

export const FULL_LABELS: ChatLabels = {
  you: "You",
  assistant: "ChatGPT",
  youSubmitted: "You · submitted",
  assistantSubmitted: "ChatGPT · Submitted",
  assistantStreaming: "ChatGPT · Streaming",
  waitingForToken: "Waiting for the first response token…",
  youNotSaved: "You · not saved",
  partialNotSaved: "Partial output · not saved",
  assistantCompleted: "ChatGPT · completed",
  empty: "Your next message starts a local conversation.",
  message: "Message",
  placeholder: "Ask a question…",
  includeScreen: "Include my screen in this message",
  screenHint: "Start typing and Dot will prepare your screen with personal data hidden.",
  screenPreparing: "Hiding personal data on your screen…",
  screenAlt: "Preview of the screen image that will be sent, with personal data hidden",
  regionHidden: "1 region hidden",
  regionsHidden: (count) => `${count} regions hidden`,
  screenCaveat: "Text recognition can miss things, and faces and photos are not hidden. This image is sent to ChatGPT and is not saved here.",
  tryAgain: "Try again",
  sendWithoutScreen: "Send without the screen",
  footerNote: "Only completed responses are saved on this device.",
  cancel: "Cancel response",
  send: "Send message",
};

export function MessageList({ chat, labels }: { chat: Chat; labels: ChatLabels }) {
  const { messages, busy, status, submittedPrompt, liveText } = chat;
  return (
    <div className="message-list" aria-label="Messages">
      {messages.map((message, index) => (
        <article className={`message ${message.role}`} key={`${message.createdAt}-${index}`}>
          <p className="message-role">{message.role === "user" ? labels.you : labels.assistant}</p>
          <p className="message-content">{message.content}</p>
        </article>
      ))}
      {busy && submittedPrompt && <article className="message user pending"><p className="message-role">{labels.youSubmitted}</p><p className="message-content">{submittedPrompt}</p></article>}
      {busy && <article className="message assistant pending"><p className="message-role">{status === "submitted" ? labels.assistantSubmitted : labels.assistantStreaming}</p><p className="message-content">{liveText || labels.waitingForToken}</p></article>}
      {status === "error" && submittedPrompt && <article className="message user partial"><p className="message-role">{labels.youNotSaved}</p><p className="message-content">{submittedPrompt}</p></article>}
      {status === "error" && liveText && <article className="message assistant partial"><p className="message-role">{labels.partialNotSaved}</p><p className="message-content">{liveText}</p></article>}
      {status === "complete" && liveText && !messages.length && <article className="message assistant"><p className="message-role">{labels.assistantCompleted}</p><p className="message-content">{liveText}</p></article>}
      {!messages.length && !liveText && <p className="empty-chat">{labels.empty}</p>}
    </div>
  );
}

export function Composer({ chat, labels, rows = 3 }: { chat: Chat; labels: ChatLabels; rows?: number }) {
  const { draft, setDraft, busy, historyLoading, captureActive, includeScreen, setIncludeScreen, screen, selectedModel, wantsScreen } = chat;
  return (
    <form className="chat-composer" onSubmit={chat.sendMessage}>
      <label htmlFor="message-input">{labels.message}</label>
      <textarea id="message-input" value={draft} onChange={(event) => setDraft(event.target.value)} maxLength={8000} rows={rows} placeholder={labels.placeholder} disabled={busy || historyLoading} />
      {captureActive && (
        <div className="screen-attach">
          <label className="screen-toggle">
            <input type="checkbox" checked={includeScreen} onChange={(event) => setIncludeScreen(event.target.checked)} disabled={busy} />
            <span>{labels.includeScreen}</span>
          </label>
          {includeScreen && screen.kind === "none" && <p className="panel-note">{labels.screenHint}</p>}
          {includeScreen && screen.kind === "preparing" && <p className="panel-note" role="status">{labels.screenPreparing}</p>}
          {includeScreen && screen.kind === "ready" && (
            <figure className="screen-preview">
              <img src={screen.preview.image} alt={labels.screenAlt} />
              <figcaption>
                {screen.preview.hiddenRegions === 1 ? labels.regionHidden : labels.regionsHidden(screen.preview.hiddenRegions)}. {labels.screenCaveat}
              </figcaption>
            </figure>
          )}
          {includeScreen && screen.kind === "failed" && (
            <div className="screen-failed" role="alert">
              <p>{screen.message}</p>
              <div className="screen-failed-actions">
                <button className="action-button secondary" type="button" onClick={() => void chat.prepareScreen()} disabled={busy}>{labels.tryAgain}</button>
                <button className="action-button secondary" type="button" onClick={() => void chat.submit(draft.trim())} disabled={busy || !draft.trim() || !selectedModel || historyLoading}>{labels.sendWithoutScreen}</button>
              </div>
            </div>
          )}
        </div>
      )}
      <div className="composer-footer">
        {labels.footerNote && <span className="panel-note">{labels.footerNote}</span>}
        {busy
          ? <button className="action-button secondary" type="button" onClick={chat.cancelResponse}>{labels.cancel}</button>
          : <button className="action-button" type="submit" disabled={!selectedModel || !draft.trim() || historyLoading || (wantsScreen && screen.kind !== "ready")}>{labels.send}</button>}
      </div>
    </form>
  );
}
