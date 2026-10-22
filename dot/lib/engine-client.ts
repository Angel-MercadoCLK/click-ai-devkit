// What the chat card knows about the engine, as plain functions the page and the tests share: reading the status and thread
// answers defensively, deciding what the chat shows (the chat, or a way to connect), and turning Codex threads into the
// history rows the card already draws. No Node imports: the page uses this file too.

import type { EngineStatus, ExternalToolsView, ToolAvailability } from "./engine.ts";

const isRecord = (value: unknown): value is Record<string, unknown> => !!value && typeof value === "object" && !Array.isArray(value);

// Status -----------------------------------------------------------------------------------------------------------

const CODEX_STATES = ["absent", "idle", "starting", "ready", "restarting", "unavailable", "stopped"];
const LOGIN_STATES = ["idle", "pending", "completed", "failed"];
const ACCOUNT_STATES = ["signedIn", "signedOut", "unavailable"];

export const BUILTIN_STATUS: EngineStatus = { engine: "builtin", codex: { state: "absent", message: null, version: null, account: null, login: { state: "idle" } }, tools: null };

function readAvailability(value: unknown): ToolAvailability | undefined {
  if (!isRecord(value) || !["ready", "missing", "unsupported"].includes(value.state as string) || typeof value.hint !== "string") return undefined;
  if (value.link !== undefined && typeof value.link !== "string") return undefined;
  return { state: value.state as ToolAvailability["state"], hint: value.hint, ...(value.link === undefined ? {} : { link: value.link as string }) };
}

/** Reads a `/api/codex/status` body; anything malformed is not a status. */
export function readEngineStatus(value: unknown): EngineStatus | undefined {
  if (!isRecord(value) || (value.engine !== "codex" && value.engine !== "builtin") || !isRecord(value.codex)) return undefined;
  const { state, message, version, account, login } = value.codex;
  if (!CODEX_STATES.includes(state as string) || (message !== null && typeof message !== "string") || (version !== null && typeof version !== "string")) return undefined;
  if (!isRecord(login) || !LOGIN_STATES.includes(login.state as string) || (login.message !== undefined && typeof login.message !== "string")) return undefined;
  let readAccount: EngineStatus["codex"]["account"] = null;
  if (account !== null) {
    if (!isRecord(account) || !ACCOUNT_STATES.includes(account.state as string)) return undefined;
    if ((account.email !== null && typeof account.email !== "string") || (account.plan !== null && typeof account.plan !== "string")) return undefined;
    readAccount = { state: account.state as "signedIn" | "signedOut" | "unavailable", email: account.email as string | null, plan: account.plan as string | null, ...(typeof account.message === "string" ? { message: account.message } : {}) };
  }
  let tools: ExternalToolsView | null = null;
  if (value.tools !== null && value.tools !== undefined) {
    const chrome = isRecord(value.tools) ? readAvailability(value.tools.chrome) : undefined;
    const computer = isRecord(value.tools) ? readAvailability(value.tools.computer) : undefined;
    if (!chrome || !computer) return undefined;
    tools = { chrome, computer };
  }
  return {
    engine: value.engine,
    codex: {
      state: state as EngineStatus["codex"]["state"],
      message: message as string | null,
      version: version as string | null,
      account: readAccount,
      login: { state: login.state as EngineStatus["codex"]["login"]["state"], ...(typeof login.message === "string" ? { message: login.message } : {}) },
    },
    tools,
  };
}

/** Why Codex is not answering, as one Spanish line; null when it is, or when there is no Codex to speak of. */
export function engineNotice(status: EngineStatus): string | null {
  if (status.engine === "codex") return null;
  const { state, message, account } = status.codex;
  if (state === "unavailable") return message;
  if (state === "idle" || state === "starting") return "Iniciando Codex… mientras tanto Dot usa su asistente integrado.";
  if (state === "restarting") return "Codex se está reiniciando… mientras tanto Dot usa su asistente integrado.";
  if (state === "ready" && account?.state === "signedOut") return "Codex está instalado, pero no has iniciado sesión.";
  if (state === "ready" && account?.state === "unavailable") return account.message ?? "Dot no pudo leer tu sesión de Codex.";
  return null;
}

export type ChatGate =
  | { kind: "checking" }
  | { kind: "chat"; engine: "codex" | "builtin" }
  | { kind: "connect"; notice: string | null; codexLogin: boolean };

/**
 * What the chat card shows. Codex needs no sign-in of Dot's own (its login is the user's ChatGPT through Codex); without it
 * the built-in assistant needs Dot's connection, exactly as before.
 */
export function chatGate({ authConnected, engine }: { authConnected: boolean | null; engine: EngineStatus | null }): ChatGate {
  if (engine === null) return { kind: "checking" };
  if (engine.engine === "codex") return { kind: "chat", engine: "codex" };
  if (authConnected === null) return { kind: "checking" };
  if (authConnected) return { kind: "chat", engine: "builtin" };
  return { kind: "connect", notice: engineNotice(engine), codexLogin: engine.codex.state === "ready" && engine.codex.account?.state === "signedOut" };
}

// History ----------------------------------------------------------------------------------------------------------

export type ThreadSummary = { id: string; title: string; updatedAt: number; cwd: string };
type Message = { role: "user" | "assistant"; content: string; createdAt: number };
export type ThreadConversation = { id: string; title: string; updatedAt: number; messages: Message[] };

const TITLE_MAX = 60;
const oneLine = (text: string) => text.replace(/\s+/g, " ").trim();
const clipTitle = (text: string) => (text.length <= TITLE_MAX ? text : `${text.slice(0, TITLE_MAX - 1).trimEnd()}…`);

function readSummary(value: unknown): ThreadSummary | undefined {
  if (!isRecord(value) || typeof value.id !== "string" || typeof value.title !== "string" || typeof value.updatedAt !== "number" || typeof value.cwd !== "string") return undefined;
  return { id: value.id, title: value.title, updatedAt: value.updatedAt, cwd: value.cwd };
}

export function readThreadList(value: unknown): { threads: ThreadSummary[]; nextCursor: string | null } | undefined {
  if (!isRecord(value) || !Array.isArray(value.threads)) return undefined;
  const threads: ThreadSummary[] = [];
  for (const item of value.threads) {
    const thread = readSummary(item);
    if (!thread) return undefined;
    threads.push(thread);
  }
  return { threads, nextCursor: typeof value.nextCursor === "string" ? value.nextCursor : null };
}

/** An opened thread with what was said, as the conversation the message list draws. */
export function readThreadResponse(value: unknown): ThreadConversation | undefined {
  const thread = isRecord(value) ? value.thread : undefined;
  const summary = readSummary(isRecord(thread) ? { cwd: "", ...thread } : undefined);
  if (!summary || !isRecord(thread) || !Array.isArray(thread.messages)) return undefined;
  const messages: Message[] = [];
  for (const item of thread.messages) {
    if (!isRecord(item) || (item.role !== "user" && item.role !== "assistant") || typeof item.text !== "string") return undefined;
    messages.push({ role: item.role, content: item.text, createdAt: summary.updatedAt });
  }
  return { id: summary.id, title: summary.title, updatedAt: summary.updatedAt, messages };
}

/**
 * The rows of the history from Codex's thread list. A chat already open keeps its messages (the list has none), and `keep`
 * (the chat being written) stays even before the list shows it. The list's order is Codex's; the card sorts by time anyway.
 */
export function mergeThreads(existing: readonly ThreadConversation[], threads: readonly ThreadSummary[], keep: string | null): ThreadConversation[] {
  const merged: ThreadConversation[] = [];
  const kept = keep === null ? undefined : existing.find((item) => item.id === keep);
  if (kept && !threads.some((thread) => thread.id === keep)) merged.push(kept);
  for (const thread of threads) {
    const known = existing.find((item) => item.id === thread.id);
    merged.push({ id: thread.id, title: thread.title, updatedAt: thread.updatedAt, messages: known?.messages ?? [] });
  }
  return merged;
}

/** Adds a finished turn to its conversation (creating it), so the chat shows the answer without another round trip. */
export function appendTurn(conversations: readonly ThreadConversation[], turn: { id: string; prompt: string; answer: string; now: number }): ThreadConversation[] {
  const prompt = turn.prompt.trim();
  const messages: Message[] = [{ role: "user", content: prompt, createdAt: turn.now }, { role: "assistant", content: turn.answer, createdAt: turn.now }];
  const known = conversations.find((item) => item.id === turn.id);
  if (!known) return [{ id: turn.id, title: clipTitle(oneLine(prompt)) || "Chat sin título", updatedAt: turn.now, messages }, ...conversations];
  return conversations.map((item) => (item.id === turn.id ? { ...item, updatedAt: turn.now, messages: [...item.messages, ...messages] } : item));
}

// Attachments ------------------------------------------------------------------------------------------------------

export type AttachmentChip = { id: string; name: string; kind: "image" | "file"; bytes: number };
export const MAX_ATTACHMENTS = 6;
const ATTACHMENT_ID = /^att_[0-9a-f]{16}$/;

export function readUpload(value: unknown): AttachmentChip | undefined {
  if (!isRecord(value) || typeof value.id !== "string" || !ATTACHMENT_ID.test(value.id) || typeof value.name !== "string") return undefined;
  if ((value.kind !== "image" && value.kind !== "file") || typeof value.bytes !== "number") return undefined;
  return { id: value.id, name: value.name, kind: value.kind, bytes: value.bytes };
}

/** How many of `incoming` files still fit next to the `current` chips. */
export const roomForAttachments = (current: number, incoming: number) => Math.max(0, Math.min(incoming, MAX_ATTACHMENTS - current));

export const attachmentsExplain = (engine: "codex" | "builtin") =>
  engine === "codex" ? "" : "Adjuntar archivos necesita Codex como motor. Con el asistente integrado Dot no puede leerlos.";
