"use client";

import { useEffect, useRef, useState } from "react";

type Model = { slug: string; displayName: string };
type Message = { role: "user" | "assistant"; content: string; createdAt: number };
type Conversation = { id: string; title: string; updatedAt: number; messages: Message[] };
type History = { revision: number; conversations: Conversation[] };
type ChatStatus = "idle" | "submitted" | "streaming" | "complete" | "error";

function newId() {
  return globalThis.crypto.randomUUID();
}

export function ChatPanel({ onAuthorizationExpired }: { onAuthorizationExpired: () => void }) {
  const [models, setModels] = useState<Model[]>([]);
  const [selectedModel, setSelectedModel] = useState("");
  const [loadingModels, setLoadingModels] = useState(false);
  const [history, setHistory] = useState<History>({ revision: 0, conversations: [] });
  const [activeId, setActiveId] = useState(newId);
  const [draft, setDraft] = useState("");
  const [submittedPrompt, setSubmittedPrompt] = useState("");
  const [status, setStatus] = useState<ChatStatus>("idle");
  const [liveText, setLiveText] = useState("");
  const [notice, setNotice] = useState("");
  const [historyLoading, setHistoryLoading] = useState(true);
  const controller = useRef<AbortController | null>(null);

  async function refreshHistory(signal?: AbortSignal) {
    const response = await fetch("/api/chat/history", { cache: "no-store", signal });
    if (!response.ok) {
      if (response.status === 401) onAuthorizationExpired();
      throw new Error(response.status === 401 ? "Reconnect with ChatGPT to open local history." : "Local conversation history is unavailable.");
    }
    const value = await response.json() as History;
    setHistory(value);
    return value;
  }

  useEffect(() => {
    const current = new AbortController();
    setHistoryLoading(true);
    refreshHistory(current.signal)
      .catch((error: unknown) => {
        if (!current.signal.aborted) setNotice(error instanceof Error ? error.message : "Local conversation history is unavailable.");
      })
      .finally(() => { if (!current.signal.aborted) setHistoryLoading(false); });
    return () => {
      current.abort();
      controller.current?.abort();
    };
  }, []);

  async function loadModels() {
    setLoadingModels(true);
    setNotice("");
    try {
      const response = await fetch("/api/chat/models", { cache: "no-store" });
      if (!response.ok) {
        if (response.status === 401) onAuthorizationExpired();
        throw new Error(response.status === 401 ? "Your ChatGPT session expired. Reconnect to continue." : "The model list could not be loaded. Try again.");
      }
      const result = await response.json() as { models?: Model[] };
      if (!Array.isArray(result.models) || result.models.length === 0) throw new Error("No models are currently available to this ChatGPT account.");
      setModels(result.models);
      setSelectedModel((current) => result.models!.some((model) => model.slug === current) ? current : result.models![0].slug);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "The model list could not be loaded. Try again.");
    } finally {
      setLoadingModels(false);
    }
  }

  function beginNewConversation() {
    controller.current?.abort();
    controller.current = null;
    setActiveId(newId());
    setDraft("");
    setSubmittedPrompt("");
    setLiveText("");
    setNotice("");
    setStatus("idle");
  }

  function cancelResponse() {
    controller.current?.abort();
    controller.current = null;
    setStatus("error");
    setNotice("Response canceled. Partial output was not saved.");
  }

  async function clearHistory() {
    controller.current?.abort();
    controller.current = null;
    setNotice("");
    try {
      const response = await fetch("/api/chat/history", { method: "DELETE" });
      if (!response.ok) throw new Error();
      const cleared = await response.json() as History;
      setHistory(cleared);
      beginNewConversation();
      setNotice("Local conversation history cleared.");
    } catch {
      setNotice("Local conversation history could not be cleared.");
    }
  }

  async function removeConversation(id: string) {
    const response = await fetch(`/api/chat/history/${encodeURIComponent(id)}`, { method: "DELETE" });
    if (!response.ok) {
      setNotice("This conversation could not be removed.");
      return;
    }
    const next = await refreshHistory().catch(() => history);
    if (activeId === id) beginNewConversation();
    setHistory(next);
  }

  function openConversation(conversation: Conversation) {
    controller.current?.abort();
    controller.current = null;
    setActiveId(conversation.id);
    setLiveText("");
    setDraft("");
    setSubmittedPrompt("");
    setNotice("");
    setStatus("complete");
  }

  async function sendMessage(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const prompt = draft.trim();
    if (!prompt || !selectedModel || controller.current || historyLoading) return;
    setDraft("");
    setSubmittedPrompt(prompt);
    setLiveText("");
    setNotice("");
    setStatus("submitted");
    const activeController = new AbortController();
    controller.current = activeController;
    let sawCompleted = false;
    try {
      const response = await fetch("/api/chat/stream", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ conversationId: activeId, revision: history.revision, model: selectedModel, prompt }),
        signal: activeController.signal,
      });
      if (!response.ok || !response.body) {
        const payload = await response.json().catch(() => ({})) as { error?: string };
        if (response.status === 401) onAuthorizationExpired();
        throw new Error(payload.error ?? "ChatGPT could not start this response.");
      }
      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";
      while (true) {
        const { done, value } = await reader.read();
        buffer += done ? decoder.decode() : decoder.decode(value, { stream: true });
        let boundary = /\r?\n\r?\n/.exec(buffer);
        while (boundary) {
          const frame = buffer.slice(0, boundary.index);
          buffer = buffer.slice(boundary.index + boundary[0].length);
          const lines = frame.split(/\r?\n/);
          const type = lines.find((line) => line.startsWith("event:"))?.slice(6).trim();
          const data = lines.filter((line) => line.startsWith("data:")).map((line) => line.slice(5).trimStart()).join("\n");
          if (data) {
            const payload = JSON.parse(data) as { text?: string; message?: string };
            if (type === "delta" && typeof payload.text === "string") {
              setStatus("streaming");
              setLiveText((current) => current + payload.text);
            } else if (type === "completed") {
              sawCompleted = true;
            } else if (type === "error") {
              throw new Error(payload.message ?? "ChatGPT could not complete this response.");
            }
          }
          boundary = /\r?\n\r?\n/.exec(buffer);
        }
        if (done) break;
      }
      if (!sawCompleted) throw new Error("The local stream ended before ChatGPT confirmed completion. Partial output was not saved.");
      setStatus("complete");
      setSubmittedPrompt("");
      const next = await refreshHistory(activeController.signal);
      setHistory(next);
    } catch (error) {
      if (activeController.signal.aborted) return;
      if (sawCompleted) {
        setStatus("complete");
        setNotice("The response completed and was saved. Local history could not refresh; reload to see the updated conversation.");
      } else {
        setStatus("error");
        setNotice(error instanceof Error ? error.message : "ChatGPT could not complete this response.");
      }
    } finally {
      if (controller.current === activeController) controller.current = null;
    }
  }

  const activeConversation = history.conversations.find((item) => item.id === activeId);
  const messages = activeConversation?.messages ?? [];
  const busy = status === "submitted" || status === "streaming";

  return (
    <section className="chat-panel" aria-label="Local ChatGPT chat">
      <div className="chat-toolbar">
        <div>
          <p className="panel-label">ChatGPT plan usage</p>
          <p className="panel-note">Model requests use your current local ChatGPT session. If it expires, reconnect to continue.</p>
        </div>
        <label className="model-picker">
          <span>Model</span>
          <select value={selectedModel} onChange={(event) => setSelectedModel(event.target.value)} disabled={!models.length || busy}>
            {!models.length && <option value="">Load models first</option>}
            {models.map((model) => <option key={model.slug} value={model.slug}>{model.displayName}</option>)}
          </select>
        </label>
      </div>

      <div className="chat-actions">
        <button className="action-button secondary" type="button" onClick={loadModels} disabled={loadingModels || busy}>
          {loadingModels ? "Loading models…" : models.length ? "Refresh models" : "Load models"}
        </button>
        <button className="action-button secondary" type="button" onClick={beginNewConversation}>New chat</button>
        <button className="action-button secondary" type="button" onClick={clearHistory} disabled={!busy && history.conversations.length === 0}>Clear history</button>
      </div>

      <div className="chat-layout">
        <aside className="history-list" aria-label="Conversation history">
          <p className="panel-label">Local history</p>
          {historyLoading ? <p className="panel-note">Loading local history…</p> : history.conversations.length === 0
            ? <p className="panel-note">Completed chats will appear here.</p>
            : history.conversations.map((conversation) => (
              <div className={`history-item${conversation.id === activeId ? " is-active" : ""}`} key={conversation.id}>
                <button type="button" className="history-open" onClick={() => openConversation(conversation)}>{conversation.title}</button>
                <button type="button" className="history-delete" onClick={() => removeConversation(conversation.id)} aria-label={`Delete ${conversation.title}`}>×</button>
              </div>
            ))}
        </aside>

        <div className="chat-main">
          <div className="message-list" aria-label="Messages">
            {messages.map((message, index) => (
              <article className={`message ${message.role}`} key={`${message.createdAt}-${index}`}>
                <p className="message-role">{message.role === "user" ? "You" : "ChatGPT"}</p>
                <p className="message-content">{message.content}</p>
              </article>
            ))}
            {busy && submittedPrompt && <article className="message user pending"><p className="message-role">You · submitted</p><p className="message-content">{submittedPrompt}</p></article>}
            {busy && <article className="message assistant pending"><p className="message-role">ChatGPT · {status === "submitted" ? "Submitted" : "Streaming"}</p><p className="message-content">{liveText || "Waiting for the first response token…"}</p></article>}
            {status === "error" && submittedPrompt && <article className="message user partial"><p className="message-role">You · not saved</p><p className="message-content">{submittedPrompt}</p></article>}
            {status === "error" && liveText && <article className="message assistant partial"><p className="message-role">Partial output · not saved</p><p className="message-content">{liveText}</p></article>}
            {status === "complete" && liveText && !messages.length && <article className="message assistant"><p className="message-role">ChatGPT · completed</p><p className="message-content">{liveText}</p></article>}
            {!messages.length && !liveText && <p className="empty-chat">Your next message starts a local conversation.</p>}
          </div>

          <form className="chat-composer" onSubmit={sendMessage}>
            <label htmlFor="message-input">Message</label>
            <textarea id="message-input" value={draft} onChange={(event) => setDraft(event.target.value)} maxLength={8000} rows={3} placeholder="Ask a question…" disabled={busy || historyLoading} />
            <div className="composer-footer">
              <span className="panel-note">Only completed responses are saved on this device.</span>
              {busy
                ? <button className="action-button secondary" type="button" onClick={cancelResponse}>Cancel response</button>
                : <button className="action-button" type="submit" disabled={!selectedModel || !draft.trim() || historyLoading}>Send message</button>}
            </div>
          </form>
        </div>
      </div>
      <p className="chat-live" role="status" aria-live="polite">
        {notice || (status === "submitted" ? "Message submitted to ChatGPT." : status === "streaming" ? "ChatGPT is streaming a response." : status === "complete" ? "Response complete and saved locally." : status === "error" ? "Response ended without being saved as complete." : "Ready when you are.")}
      </p>
      <p className="privacy-note">Responses are requested with remote storage disabled. Chat history is stored separately from credentials in private local storage. No tools are enabled.</p>
    </section>
  );
}
