import { createHash, randomUUID } from "node:crypto";
import { constants, type Stats } from "node:fs";
import { chmod, lstat, open, rename, unlink } from "node:fs/promises";
import { dirname, join } from "node:path";
import { PROTECTED_OVERHEAD_BYTES, openJson, sealText, windowsStorageProtector } from "./storage-protector.ts";
import { assertPrivateAccountStoragePath, getStoragePathForTests, withCurrentConnection, withPrivateAccountFileLock, type Connection, type ConnectionSnapshot } from "./auth-store.ts";

export type ChatMessage = { role: "user" | "assistant"; content: string; createdAt: number };
export type Conversation = { id: string; title: string; updatedAt: number; messages: ChatMessage[] };
type AccountHistory = { revision: number; conversations: Conversation[] };
type HistoryDocument = { version: 1; accounts: Record<string, AccountHistory> };

const MAX_FILE_BYTES = 2_000_000;
const MAX_CONVERSATIONS = 30;
const MAX_MESSAGES = 100;
const MAX_MESSAGE_CHARS = 16_000;
const idPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const uid = () => process.getuid?.();
const defaultDocument = (): HistoryDocument => ({ version: 1, accounts: {} });
const accountKey = (connection: Pick<Connection, "issuer" | "subject">) =>
  createHash("sha256").update(`${connection.issuer}\0${connection.subject}`).digest("hex");

async function historyPath() {
  const configured = process.env.CLICK_DOT_CHAT_FILE;
  if (configured) return configured;
  return join(dirname(await getStoragePathForTests()), "conversations.json");
}

function isMissing(error: unknown) {
  return (error as NodeJS.ErrnoException).code === "ENOENT";
}

function validateMessage(value: unknown): value is ChatMessage {
  if (!value || typeof value !== "object") return false;
  const candidate = value as Partial<ChatMessage>;
  return (candidate.role === "user" || candidate.role === "assistant")
    && typeof candidate.content === "string" && candidate.content.length > 0
    && candidate.content.length <= MAX_MESSAGE_CHARS && Number.isSafeInteger(candidate.createdAt);
}

function validateConversation(value: unknown): value is Conversation {
  if (!value || typeof value !== "object") return false;
  const candidate = value as Partial<Conversation>;
  return typeof candidate.id === "string" && idPattern.test(candidate.id)
    && typeof candidate.title === "string" && candidate.title.length <= 120
    && Number.isSafeInteger(candidate.updatedAt) && Array.isArray(candidate.messages)
    && candidate.messages.length <= MAX_MESSAGES && candidate.messages.every((message, index) =>
      validateMessage(message) && message.role === (index % 2 === 0 ? "user" : "assistant"));
}

function validateDocument(value: unknown): HistoryDocument {
  if (!value || typeof value !== "object") throw new Error("Local conversation history is invalid.");
  const document = value as Partial<HistoryDocument>;
  if (document.version !== 1 || !document.accounts || typeof document.accounts !== "object" || Array.isArray(document.accounts)) {
    throw new Error("Local conversation history is invalid.");
  }
  for (const account of Object.values(document.accounts)) {
    if (!account || !Number.isSafeInteger(account.revision) || account.revision! < 0
      || !Array.isArray(account.conversations) || account.conversations.length > MAX_CONVERSATIONS
      || !account.conversations.every(validateConversation)) throw new Error("Local conversation history is invalid.");
  }
  return document as HistoryDocument;
}

// On native Windows with a protector there are no uid or mode bits; the file is encrypted instead.
async function readDocument(path: string): Promise<HistoryDocument> {
  const protector = windowsStorageProtector();
  if (!protector && constants.O_NOFOLLOW === undefined) throw new Error("Safe local conversation storage is unavailable on this platform.");
  if (!await assertPrivateAccountStoragePath(path)) return defaultDocument();
  let handle;
  try {
    handle = await open(path, constants.O_RDONLY | (constants.O_NOFOLLOW ?? 0));
  } catch (error) {
    if (isMissing(error)) return defaultDocument();
    throw error;
  }
  try {
    const info = await handle.stat();
    const pathInfo = await lstat(path);
    if (!info.isFile() || pathInfo.isSymbolicLink() || info.dev !== pathInfo.dev || info.ino !== pathInfo.ino
      || info.nlink !== 1 || !isOwnerPrivate(info, 0o600) || info.size > (protector ? MAX_FILE_BYTES + PROTECTED_OVERHEAD_BYTES : MAX_FILE_BYTES)) {
      throw new Error("Local conversation history is not private or exceeds its storage limit.");
    }
    return validateDocument(protector ? openJson(protector, await handle.readFile()) : JSON.parse(await handle.readFile("utf8")));
  } finally {
    await handle.close();
  }
}

function isOwnerPrivate(info: Stats, mode: number) {
  if (windowsStorageProtector()) return true;
  return info.uid === uid() && (info.mode & 0o777) === mode;
}

function assertNotAborted(signal?: AbortSignal) {
  if (signal?.aborted) throw new Error("The conversation response was cancelled.");
}

async function writeDocument(path: string, document: HistoryDocument, signal?: AbortSignal) {
  assertNotAborted(signal);
  const protector = windowsStorageProtector();
  if (!protector && constants.O_NOFOLLOW === undefined) throw new Error("Safe local conversation storage is unavailable on this platform.");
  const content = JSON.stringify(document);
  if (Buffer.byteLength(content, "utf8") > MAX_FILE_BYTES) throw new Error("Local conversation history reached its storage limit.");
  const payload = protector ? sealText(protector, content) : content;
  const temporary = `${path}.${randomUUID()}.tmp`;
  const handle = await open(temporary, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL | (constants.O_NOFOLLOW ?? 0), 0o600);
  try {
    const info = await handle.stat();
    if (!info.isFile() || info.nlink !== 1 || !isOwnerPrivate(info, 0o600)) {
      throw new Error("Private local conversation storage could not be created.");
    }
    await handle.writeFile(payload, "utf8");
    await handle.sync();
    assertNotAborted(signal);
  } catch (error) {
    await unlink(temporary).catch(() => undefined);
    throw error;
  } finally {
    await handle.close();
  }
  try {
    try {
      const existing = await lstat(path);
      if (!existing.isFile() || existing.isSymbolicLink() || existing.nlink !== 1 || !isOwnerPrivate(existing, 0o600)) {
        throw new Error("Local conversation history is not private.");
      }
    } catch (error) {
      if (!isMissing(error)) throw error;
    }
    await rename(temporary, path);
    if (!protector) await chmod(path, 0o600);
  } catch (error) {
    await unlink(temporary).catch(() => undefined);
    throw error;
  }
}

async function mutateHistory<T>(snapshot: ConnectionSnapshot, change: (history: AccountHistory) => T, signal?: AbortSignal): Promise<T> {
  const path = await historyPath();
  const connection = snapshot.connection;
  const result = await withCurrentConnection(snapshot, () => withPrivateAccountFileLock(path, async () => {
    assertNotAborted(signal);
    const document = await readDocument(path);
    assertNotAborted(signal);
    const key = accountKey(connection);
    const history = document.accounts[key] ?? { revision: 0, conversations: [] };
    document.accounts[key] = history;
    const value = change(history);
    assertNotAborted(signal);
    await writeDocument(path, document, signal);
    return value;
  }));
  if (result === undefined) throw new Error("The local ChatGPT connection changed. Reconnect before continuing.");
  return result;
}

export async function listConversations(snapshot: ConnectionSnapshot) {
  const connection = snapshot.connection;
  const document = await readDocument(await historyPath());
  const history = document.accounts[accountKey(connection)] ?? { revision: 0, conversations: [] };
  return { revision: history.revision, conversations: history.conversations };
}

export async function getConversation(snapshot: ConnectionSnapshot, id: string) {
  if (!idPattern.test(id)) return undefined;
  const { conversations } = await listConversations(snapshot);
  return conversations.find((conversation) => conversation.id === id);
}

export async function appendCompletedTurn(
  snapshot: ConnectionSnapshot,
  expectedRevision: number,
  conversationId: string,
  model: string,
  prompt: string,
  answer: string,
  signal?: AbortSignal,
) {
  assertNotAborted(signal);
  if (!idPattern.test(conversationId) || prompt.length < 1 || prompt.length > MAX_MESSAGE_CHARS
    || answer.length < 1 || answer.length > MAX_MESSAGE_CHARS || model.length > 120) {
    throw new Error("The completed response could not be saved.");
  }
  return mutateHistory(snapshot, (history) => {
    if (history.revision !== expectedRevision) throw new Error("Conversation history changed during this response.");
    let conversation = history.conversations.find((item) => item.id === conversationId);
    if (!conversation) {
      if (history.conversations.length >= MAX_CONVERSATIONS) throw new Error("Conversation history reached its local limit.");
      conversation = { id: conversationId, title: prompt.slice(0, 80), updatedAt: Date.now(), messages: [] };
      history.conversations.push(conversation);
    }
    if (conversation.messages.length + 2 > MAX_MESSAGES) throw new Error("This conversation reached its local message limit.");
    const timestamp = Date.now();
    conversation.messages.push({ role: "user", content: prompt, createdAt: timestamp });
    conversation.messages.push({ role: "assistant", content: answer, createdAt: timestamp });
    conversation.updatedAt = timestamp;
    history.revision += 1;
    history.conversations.sort((a, b) => b.updatedAt - a.updatedAt);
    return conversation;
  }, signal);
}

export async function clearConversations(snapshot: ConnectionSnapshot) {
  return mutateHistory(snapshot, (history) => {
    history.revision += 1;
    history.conversations = [];
    return history.revision;
  });
}

export async function deleteConversation(snapshot: ConnectionSnapshot, id: string) {
  if (!idPattern.test(id)) return false;
  return mutateHistory(snapshot, (history) => {
    const length = history.conversations.length;
    history.conversations = history.conversations.filter((conversation) => conversation.id !== id);
    if (length !== history.conversations.length) history.revision += 1;
    return length !== history.conversations.length;
  });
}

export async function getHistoryPathForTests() {
  return process.env.CLICK_DOT_CHAT_FILE ?? join(dirname(await getStoragePathForTests()), "conversations.json");
}
