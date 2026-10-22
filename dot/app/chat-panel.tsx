"use client";

import { useEffect, useRef, useState } from "react";
import { Composer, FULL_LABELS, MessageList } from "./chat/parts";
import { applyCapturePoll, INITIAL_CAPTURE_POLL, readCapturePoll, type CapturePollResult } from "../lib/capture-state.ts";
import { applyToolEvent, readToolEvent, type ToolEvent } from "../lib/tool-events.ts";
import { appendTurn, attachmentsExplain, MAX_ATTACHMENTS, mergeThreads, readThreadList, readThreadResponse, readUpload, roomForAttachments, type AttachmentChip } from "../lib/engine-client.ts";

export type Model = { slug: string; displayName: string };
export type Message = { role: "user" | "assistant"; content: string; createdAt: number };
export type Conversation = { id: string; title: string; updatedAt: number; messages: Message[] };
export type History = { revision: number; conversations: Conversation[] };
export type ChatStatus = "idle" | "submitted" | "streaming" | "complete" | "error";
export type ScreenPreview = { previewId: string; image: string; hiddenRegions: number; preparedAt: number };
export type ScreenState = { kind: "none" } | { kind: "preparing" } | { kind: "ready"; preview: ScreenPreview } | { kind: "failed"; message: string };

const CAPTURE_POLL_MS = 4000;
/** A preview older than this is refreshed (and shown again) before it can be sent. The server drops it after two minutes. */
const PREVIEW_MAX_AGE_MS = 60_000;

function readPreview(value: unknown): Omit<ScreenPreview, "preparedAt"> | undefined {
  if (!value || typeof value !== "object") return undefined;
  const { previewId, image, hiddenRegions } = value as Record<string, unknown>;
  if (typeof previewId !== "string" || !/^[A-Za-z0-9_-]{22}$/.test(previewId)) return undefined;
  if (typeof image !== "string" || !/^data:image\/(png|jpeg);base64,/.test(image)) return undefined;
  if (typeof hiddenRegions !== "number" || !Number.isInteger(hiddenRegions) || hiddenRegions < 0) return undefined;
  return { previewId, image, hiddenRegions };
}

function newId() {
  return globalThis.crypto.randomUUID();
}

/** The one "model" of a chat on Codex: Codex picks the real one from its own configuration. */
const CODEX_MODEL: Model = { slug: "codex", displayName: "Codex" };

/**
 * All of the chat behavior (models, history, streaming, screen attachment). The panels only decide how it looks. With
 * `engine: "codex"` the history is Codex's threads, a chat is a Codex thread and files can be attached; the default is the
 * built-in assistant and the local store, exactly as before.
 */
export function useChat({ onAuthorizationExpired, engine = "builtin" }: { onAuthorizationExpired: () => void; engine?: "codex" | "builtin" }) {
  const codex = engine === "codex";
  const [models, setModels] = useState<Model[]>([]);
  const [selectedModel, setSelectedModel] = useState("");
  const [loadingModels, setLoadingModels] = useState(false);
  const [history, setHistory] = useState<History>({ revision: 0, conversations: [] });
  const [activeId, setActiveId] = useState(newId);
  const [draft, setDraft] = useState("");
  const [submittedPrompt, setSubmittedPrompt] = useState("");
  const [status, setStatus] = useState<ChatStatus>("idle");
  const [liveText, setLiveText] = useState("");
  /** The tool calls of the message being answered (or of the last one), in the order the model made them. */
  const [tools, setTools] = useState<ToolEvent[]>([]);
  const [notice, setNotice] = useState("");
  const [historyLoading, setHistoryLoading] = useState(true);
  const controller = useRef<AbortController | null>(null);
  const [captureActive, setCaptureActive] = useState(false);
  const [includeScreen, setIncludeScreen] = useState(true);
  const [screen, setScreen] = useState<ScreenState>({ kind: "none" });
  const previewRequest = useRef<AbortController | null>(null);
  const [attachments, setAttachments] = useState<AttachmentChip[]>([]);
  const [historyQuery, setHistoryQuery] = useState("");
  /** The Codex thread the chat in progress got from the server, until the turn is saved into the history. */
  const codexThread = useRef<{ chatId: string; threadId: string } | null>(null);
  // The latest values for code that runs after an await.
  const live = useRef({ history, activeId, historyQuery });
  live.current = { history, activeId, historyQuery };

  async function refreshHistory(signal?: AbortSignal, query?: string) {
    if (codex) {
      const text = query ?? live.current.historyQuery;
      const response = await fetch(`/api/codex/threads${text ? `?q=${encodeURIComponent(text)}` : ""}`, { cache: "no-store", signal });
      const list = response.ok ? readThreadList(await response.json()) : undefined;
      if (!list) throw new Error("Dot no pudo leer tus chats de Codex.");
      const value: History = { revision: 0, conversations: mergeThreads(live.current.history.conversations, list.threads, live.current.activeId) };
      setHistory(value);
      return value;
    }
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

  // Whether Dot is currently allowed to see the screen. Only a yes/no is kept here.
  useEffect(() => {
    const current = new AbortController();
    let polls = INITIAL_CAPTURE_POLL;
    async function poll() {
      let result: CapturePollResult = { ok: false };
      try {
        const response = await fetch("/api/capture/status", { cache: "no-store", signal: current.signal });
        if (response.ok) result = readCapturePoll(await response.json());
      } catch {
        // A failed read is not a "stopped"; see applyCapturePoll.
      }
      if (current.signal.aborted) return;
      polls = applyCapturePoll(polls, result);
      setCaptureActive(polls.active);
    }
    void poll();
    const timer = window.setInterval(() => void poll(), CAPTURE_POLL_MS);
    return () => {
      current.abort();
      window.clearInterval(timer);
    };
  }, []);

  async function prepareScreen() {
    previewRequest.current?.abort();
    const request = new AbortController();
    previewRequest.current = request;
    setScreen({ kind: "preparing" });
    try {
      const response = await fetch("/api/capture/preview", { method: "POST", cache: "no-store", signal: request.signal });
      const payload = await response.json().catch(() => ({})) as { error?: string };
      const preview = response.ok ? readPreview(payload) : undefined;
      if (!preview) throw new Error(response.ok ? "Dot could not prepare the screen image. Nothing was attached." : payload.error ?? "Dot could not prepare the screen image. Nothing was attached.");
      setScreen({ kind: "ready", preview: { ...preview, preparedAt: Date.now() } });
    } catch (error) {
      if (request.signal.aborted) return;
      setScreen({ kind: "failed", message: error instanceof Error ? error.message : "Dot could not prepare the screen image. Nothing was attached." });
    } finally {
      if (previewRequest.current === request) previewRequest.current = null;
    }
  }

  // Prepare the redacted preview once the user starts a message; drop it when capture stops or the toggle goes off.
  const hasDraft = draft.trim().length > 0;
  // Codex looks at the screen itself (look_at_screen) when asked, so the "include my screen" attachment is the built-in assistant's.
  const wantsScreen = captureActive && includeScreen && !codex;
  useEffect(() => {
    if (!wantsScreen) {
      previewRequest.current?.abort();
      previewRequest.current = null;
      setScreen((current) => current.kind === "none" ? current : { kind: "none" });
      return;
    }
    if (hasDraft && screen.kind === "none" && status !== "submitted" && status !== "streaming") void prepareScreen();
  }, [wantsScreen, hasDraft, screen.kind, status]);

  useEffect(() => () => previewRequest.current?.abort(), []);

  async function loadModels() {
    if (codex) {
      setModels([CODEX_MODEL]);
      setSelectedModel(CODEX_MODEL.slug);
      return;
    }
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
    setTools([]);
    setNotice("");
    setStatus("idle");
    setAttachments([]);
    codexThread.current = null;
  }

  function cancelResponse() {
    controller.current?.abort();
    controller.current = null;
    setStatus("error");
    setNotice(codex ? "Detenido. Lo que Codex ya hizo no se deshace." : "Response stopped. Partial output was not saved, and anything a tool already did is not undone.");
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
    if (codex) {
      // Codex keeps archived chats on disk; they only stop being listed.
      const response = await fetch(`/api/codex/threads/${encodeURIComponent(id)}`, { method: "DELETE" }).catch(() => undefined);
      if (!response?.ok) {
        setNotice("No se pudo archivar este chat.");
        return;
      }
      setHistory((current) => ({ revision: 0, conversations: current.conversations.filter((item) => item.id !== id) }));
      if (live.current.activeId === id) beginNewConversation();
      return;
    }
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
    setTools([]);
    setDraft("");
    setSubmittedPrompt("");
    setNotice("");
    setStatus("complete");
    setAttachments([]);
    codexThread.current = null;
    if (codex && conversation.messages.length === 0) void openThread(conversation.id);
  }

  /** The messages of a Codex chat, read when it is opened (the list has none). */
  async function openThread(id: string) {
    setNotice("Abriendo el chat…");
    try {
      const response = await fetch(`/api/codex/threads/${encodeURIComponent(id)}`, { cache: "no-store" });
      const opened = response.ok ? readThreadResponse(await response.json()) : undefined;
      if (!opened) throw new Error();
      setHistory((current) => ({ revision: 0, conversations: current.conversations.map((item) => (item.id === id ? { ...item, messages: opened.messages } : item)) }));
      setNotice("");
    } catch {
      setNotice("Dot no pudo abrir ese chat de Codex.");
    }
  }

  /** Dropped or pasted files, sent to the guarded route that keeps a private copy. Only Codex can read them. */
  async function attachFiles(files: File[]) {
    if (!codex) {
      setNotice(attachmentsExplain("builtin"));
      return;
    }
    const room = roomForAttachments(attachments.length, files.length);
    if (room < files.length) setNotice(`Solo puedes adjuntar ${MAX_ATTACHMENTS} archivos por mensaje.`);
    for (const file of files.slice(0, room)) {
      try {
        const response = await fetch("/api/attachments", {
          method: "POST",
          headers: { "content-type": "application/octet-stream", "x-file-name": encodeURIComponent(file.name || "pegado") },
          body: file,
        });
        const payload = await response.json().catch(() => ({})) as { error?: string };
        const chip = response.ok ? readUpload(payload) : undefined;
        if (!chip) throw new Error(payload.error ?? "Dot no pudo adjuntar el archivo.");
        setAttachments((current) => (current.length >= MAX_ATTACHMENTS ? current : [...current, chip]));
      } catch (error) {
        setNotice(error instanceof Error ? error.message : "Dot no pudo adjuntar el archivo.");
      }
    }
  }

  function removeAttachment(id: string) {
    setAttachments((current) => current.filter((item) => item.id !== id));
    void fetch(`/api/attachments/${encodeURIComponent(id)}`, { method: "DELETE" }).catch(() => {});
  }

  async function searchHistory(query: string) {
    setHistoryQuery(query);
    live.current.historyQuery = query;
    try {
      await refreshHistory(undefined, query);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Dot no pudo leer tus chats de Codex.");
    }
  }

  async function sendMessage(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const prompt = draft.trim();
    if (!prompt || !selectedModel || controller.current || historyLoading) return;
    if (!wantsScreen) {
      await submit(prompt);
      return;
    }
    if (screen.kind !== "ready") return;
    // The thumbnail on screen is exactly what gets sent. An old one is replaced first, and the user sees the new one.
    if (Date.now() - screen.preview.preparedAt > PREVIEW_MAX_AGE_MS) {
      setNotice("The screen image was refreshed. Check it, then send again.");
      void prepareScreen();
      return;
    }
    await submit(prompt, screen.preview.previewId);
  }

  async function submit(prompt: string, screenPreviewId?: string) {
    setScreen({ kind: "none" });
    setDraft("");
    setSubmittedPrompt(prompt);
    setLiveText("");
    setTools([]);
    setNotice("");
    setStatus("submitted");
    const activeController = new AbortController();
    controller.current = activeController;
    let sawCompleted = false;
    const seenTools = new Set<string>();
    // The thread this chat already has on Codex: one the server told us about, or the chat is an opened thread.
    const knownThread = codexThread.current?.chatId === activeId ? codexThread.current.threadId : history.conversations.some((item) => item.id === activeId) ? activeId : undefined;
    const sentAttachments = attachments.map((item) => item.id);
    let answer = "";
    if (codex) setAttachments([]);
    try {
      const response = await fetch("/api/chat/stream", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(
          codex
            ? { model: CODEX_MODEL.slug, prompt, ...(knownThread ? { threadId: knownThread } : {}), ...(sentAttachments.length > 0 ? { attachments: sentAttachments } : {}) }
            : { conversationId: activeId, revision: history.revision, model: selectedModel, prompt, ...(screenPreviewId ? { screenPreviewId } : {}) },
        ),
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
            const payload = JSON.parse(data) as { text?: string; message?: string; threadId?: string };
            const tool = type === "tool" ? readToolEvent(payload) : undefined;
            if (tool) {
              setStatus("streaming");
              // Text streamed before a tool call was only the model thinking aloud; the saved answer is the last round's.
              if (!seenTools.has(tool.id)) { seenTools.add(tool.id); setLiveText(""); answer = ""; }
              setTools((current) => applyToolEvent(current, tool));
            } else if (type === "delta" && typeof payload.text === "string") {
              setStatus("streaming");
              answer += payload.text;
              setLiveText((current) => current + payload.text);
            } else if (type === "thread" && typeof payload.threadId === "string") {
              codexThread.current = { chatId: activeId, threadId: payload.threadId };
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
      if (!sawCompleted) throw new Error(codex ? "La conexión con Codex se cortó antes de terminar la respuesta." : "The local stream ended before ChatGPT confirmed completion. Partial output was not saved.");
      if (codex) {
        // Codex saved the chat itself; show the new turn now and let the list catch up in the background.
        const threadId = codexThread.current?.threadId;
        codexThread.current = null;
        if (threadId) {
          setHistory((current) => ({ revision: 0, conversations: appendTurn(current.conversations, { id: threadId, prompt, answer, now: Date.now() }) }));
          live.current.activeId = threadId;
          setActiveId(threadId);
        }
        setStatus("complete");
        setSubmittedPrompt("");
        void refreshHistory(activeController.signal).catch(() => {});
        return;
      }
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
        setNotice(error instanceof Error ? error.message : codex ? "Codex no pudo completar la respuesta." : "ChatGPT could not complete this response.");
      }
    } finally {
      if (controller.current === activeController) controller.current = null;
    }
  }

  const activeConversation = history.conversations.find((item) => item.id === activeId);
  const messages = activeConversation?.messages ?? [];
  const busy = status === "submitted" || status === "streaming";

  return {
    models, selectedModel, setSelectedModel, loadingModels, loadModels,
    history, historyLoading, activeId, draft, setDraft, status, liveText, submittedPrompt, notice,
    captureActive, includeScreen, setIncludeScreen, screen, prepareScreen, wantsScreen,
    activeConversation, messages, busy, tools,
    engine, attachments, attachFiles, removeAttachment, historyQuery, searchHistory,
    sendMessage, submit, cancelResponse, beginNewConversation, clearHistory, removeConversation, openConversation,
  };
}

export type Chat = ReturnType<typeof useChat>;


export function ChatPanel({ onAuthorizationExpired }: { onAuthorizationExpired: () => void }) {
  const chat = useChat({ onAuthorizationExpired });
  const { models, selectedModel, setSelectedModel, loadingModels, loadModels, history, historyLoading, activeId, status, notice, busy } = chat;
  const { beginNewConversation, clearHistory, removeConversation, openConversation } = chat;

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
          <MessageList chat={chat} labels={FULL_LABELS} />

          <Composer chat={chat} labels={FULL_LABELS} />
        </div>
      </div>
      <p className="chat-live" role="status" aria-live="polite">
        {notice || (status === "submitted" ? "Message submitted to ChatGPT." : status === "streaming" ? "ChatGPT is streaming a response." : status === "complete" ? "Response complete and saved locally." : status === "error" ? "Response ended without being saved as complete." : "Ready when you are.")}
      </p>
      <p className="privacy-note">Responses are requested with remote storage disabled. Chat history is stored separately from credentials in private local storage. In the Dot desktop app on Windows, Dot can also run PowerShell, hand tasks to Claude Code and look at your shared screen: read-only commands run on their own, anything else asks first on the mascot.</p>
    </section>
  );
}
