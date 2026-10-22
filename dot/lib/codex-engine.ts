// The Codex engine: Dot's brain when the user has Codex. A Dot chat is a Codex thread; this file turns Codex's app-server
// protocol into what the rest of Dot already understands: the local SSE `delta` and `tool` events, approval cards on the
// agent broker, and a small history view. The process and the JSON-RPC plumbing live in `desktop/codex-app-server.mts`,
// which publishes a transport on `globalThis[Symbol.for("click-dot.codex-engine")]`; this file only reads it. Nothing here
// spawns anything, so everything is testable with a fake transport.
//
// Wire facts verified against codex-cli 0.163.0 (see lib/codex-protocol): thread settings are `approvalPolicy: "untrusted"`
// (not "unlessTrusted") with `sandbox: "workspace-write"`; the user-input request method is `item/tool/requestUserInput`;
// a command approval lists its `availableDecisions` only because Dot opts into the experimental API at `initialize`.
// Everything Codex sends is data: it picks what is shown, never what Dot allows.

import { getAgentBroker, parseQuestions, type CodexApproval } from "./agent-broker.ts";
import type { AgentQuestion } from "./agent-state.ts";
import type {
  CommandExecutionRequestApprovalParams,
  FileChangeRequestApprovalParams,
  McpServerElicitationRequestParams,
  PermissionsRequestApprovalParams,
  ThreadItem,
  ThreadListParams,
  ThreadResumeParams,
  ThreadStartParams,
  ToolRequestUserInputParams,
  TurnStartParams,
  UserInput,
} from "./codex-protocol/index.ts";
import { clipDetail, oneLine, type ToolEvent, type ToolName } from "./tool-events.ts";

export const CODEX_ENGINE_KEY = Symbol.for("click-dot.codex-engine");
const INSTANCE_KEY = Symbol.for("click-dot.codex-engine-instance");

/** Threads ask before anything that is not known safe, and can write only inside their folder. Reads are free. */
export const CODEX_APPROVAL_POLICY = "untrusted" as const;
export const CODEX_SANDBOX_MODE = "workspace-write" as const;
/**
 * The activity UI knows three tool names. Codex rows reuse one of them until `tool-events.ts` gets a "codex" name: change
 * this single constant then (the summaries already say what each row is).
 */
export const CODEX_TOOL_NAME: ToolName = "codex";
/** The MCP servers Dot registers for the user's Chrome and for mouse and keyboard (see desktop/external-mcp.mts). */
export const CHROME_SERVER = "chrome";
export const COMPUTER_SERVER = "computer";
/** Dot's own MCP server and the tools whose rows get their own name. */
const DOT_SERVER = "dot";
/** How long a turn waits for Codex to confirm an interrupt before it ends anyway. */
export const INTERRUPT_GRACE_MS = 10_000;

const CARD_TOOL = "Codex";
const HISTORY_PAGE = 30;
const MAX_DETAIL = 1_000;
const DETAIL_TAIL = 600;
const MCP_RESULT_TAIL = 300;
const MAX_TITLE = 120;
const MEMORY_SUMMARY = "Consultando memoria…";
const UNTITLED = "Chat sin título";
const CRASHED = "Codex se reinició o dejó de responder. Vuelve a intentarlo.";
const FAILED = "Codex no pudo completar la respuesta.";

// Transport (the shape desktop/codex-app-server.mts publishes) -----------------------------------------------------

export type CodexStatus = { state: "idle" | "starting" | "ready" | "restarting" | "unavailable" | "stopped"; message?: string; version?: string; restarts: number };
export interface CodexTransport {
  request(method: string, params?: unknown, options?: { timeoutMs?: number; signal?: AbortSignal }): Promise<unknown>;
  notify(method: string, params?: unknown): void;
  onNotification(listener: (notification: { method: string; params: unknown }) => void): () => void;
  setRequestHandler(handler: ((request: { id: string | number; method: string; params: unknown }) => unknown) | null): void;
  status(): CodexStatus;
  onStatus(listener: (status: CodexStatus) => void): () => void;
  onRestart(listener: () => void): () => void;
}

const TRANSPORT_METHODS = ["request", "notify", "onNotification", "setRequestHandler", "status", "onStatus", "onRestart"] as const;
const isRecord = (value: unknown): value is Record<string, unknown> => !!value && typeof value === "object" && !Array.isArray(value);
const isTransport = (value: unknown): value is CodexTransport => isRecord(value) && TRANSPORT_METHODS.every((name) => typeof value[name] === "function");
const text = (value: unknown): string | null => (typeof value === "string" && value.length > 0 ? value : null);
const list = (value: unknown): unknown[] => (Array.isArray(value) ? value : []);

// Public types ----------------------------------------------------------------------------------------------------

export type CodexAccountStatus =
  | { state: "signedIn"; method: "chatgpt" | "apiKey" | "other"; email: string | null; plan: string | null }
  | { state: "signedOut" }
  | { state: "unavailable"; message: string };
export type CodexTurnEvent = { type: "delta"; text: string } | { type: "tool"; tool: ToolEvent };
/** `error` is a user-facing message for a failed turn. The route turns the result into the SSE `completed` or `error` event. */
export type CodexTurnResult = { text: string; status: "completed" | "interrupted" | "failed"; error?: string };
export type CodexThreadView = { id: string; title: string; /** Milliseconds since the epoch. */ updatedAt: number; cwd: string };
export type CodexThreadMessage = { role: "user" | "assistant"; text: string };
export type CodexTurnRequest = {
  threadId: string;
  text: string;
  /** Absolute paths of images; other files go in the text as paths. */
  localImages?: string[];
  signal: AbortSignal;
  emit: (event: CodexTurnEvent) => unknown;
};

export type CodexEngineDeps = {
  broker?: { requestCodexApproval(input: Parameters<ReturnType<typeof getAgentBroker>["requestCodexApproval"]>[0]): Promise<CodexApproval> };
  setTimer?: (callback: () => void, ms: number) => unknown;
  clearTimer?: (handle: unknown) => void;
};

// Mapping helpers -------------------------------------------------------------------------------------------------

const baseName = (path: string) => path.split(/[\\/]/).filter(Boolean).pop() ?? path;

function fileNames(paths: string[]) {
  const shown = paths.slice(0, 3).map(baseName).join(", ");
  return paths.length > 3 ? `${shown} y ${paths.length - 3} más` : shown;
}

const isMemoryCall = (server: string, tool: string) => /engram/i.test(server) || /^mem_/.test(tool);

/** The activity name of an MCP call, and the row's summary: the tool alone when the name already says where it runs. */
function mcpRow(server: string, tool: string): { name: ToolName; summary: string } {
  if (isMemoryCall(server, tool)) return { name: "memory", summary: MEMORY_SUMMARY };
  if (server === CHROME_SERVER) return { name: "chrome", summary: tool };
  if (server === COMPUTER_SERVER) return { name: "computer", summary: tool };
  if (server === DOT_SERVER) return { name: tool === "look_at_screen" ? "screen" : "claude_code", summary: tool };
  return { name: CODEX_TOOL_NAME, summary: `${server}: ${tool}` };
}

type Known = { name: ToolName; summary: string; /** The files of a file change. */ paths?: string[] };

/** The tool row for a thread item, or undefined for items that are not activity (messages, reasoning, and the rest). */
function toolRowFor(thing: ThreadItem): { event: ToolEvent; known: Known } | undefined {
  const row = (summary: string, status: ToolEvent["status"], detail?: string, paths?: string[], name: ToolName = CODEX_TOOL_NAME) => {
    const detailText = detail === undefined || detail === "" ? undefined : clipDetail(detail, DETAIL_TAIL);
    return {
      event: { id: thing.id, name, summary, status, ...(detailText === undefined ? {} : { detail: detailText }) },
      known: { name, summary, ...(paths ? { paths } : {}) },
    };
  };
  if (thing.type === "commandExecution") {
    const summary = oneLine(thing.command, 160);
    if (thing.status === "inProgress") return row(summary, "running");
    if (thing.status === "declined") return row(summary, "denied");
    const output = (thing.aggregatedOutput ?? "").trimEnd();
    const failed = thing.status === "failed" || (thing.exitCode !== null && thing.exitCode !== 0);
    const header = thing.exitCode !== null && thing.exitCode !== 0 ? `Código de salida ${thing.exitCode}` : "";
    const tail = output === "" ? "" : clipDetail(output, header === "" ? DETAIL_TAIL : DETAIL_TAIL - header.length - 1);
    return row(summary, failed ? "error" : "done", [header, tail].filter(Boolean).join("\n"));
  }
  if (thing.type === "fileChange") {
    const paths = thing.changes.map((change) => change.path);
    const summary = `Editando ${fileNames(paths)}`;
    if (thing.status === "inProgress") return row(summary, "running", undefined, paths);
    return row(summary, thing.status === "completed" ? "done" : thing.status === "declined" ? "denied" : "error", paths.join("\n"), paths);
  }
  if (thing.type === "mcpToolCall") {
    const memory = isMemoryCall(thing.server, thing.tool);
    const { name, summary } = mcpRow(thing.server, thing.tool);
    if (thing.status === "inProgress") return row(summary, "running", undefined, undefined, name);
    if (thing.status === "failed") return row(summary, "error", thing.error?.message ?? "", undefined, name);
    // What Engram returned is the model's to read; the row only says it was consulted.
    const resultText = memory ? "" : list(thing.result?.content).map((part) => (isRecord(part) && part.type === "text" ? text(part.text) : null)).filter(Boolean).join("\n");
    return { ...row(summary, "done", undefined, undefined, name), event: { id: thing.id, name, summary, status: "done", ...(resultText ? { detail: clipDetail(resultText, MCP_RESULT_TAIL) } : {}) } };
  }
  return undefined;
}

function summarizeThread(thing: unknown): CodexThreadView | undefined {
  if (!isRecord(thing) || typeof thing.id !== "string") return undefined;
  const title = oneLine(text(thing.name) ?? text(thing.preview) ?? "", MAX_TITLE) || UNTITLED;
  const seconds = typeof thing.updatedAt === "number" && Number.isFinite(thing.updatedAt) ? thing.updatedAt : 0;
  return { id: thing.id, title, updatedAt: Math.round(seconds * 1000), cwd: text(thing.cwd) ?? "" };
}

function messagesOf(thing: Record<string, unknown>): CodexThreadMessage[] {
  const messages: CodexThreadMessage[] = [];
  for (const turn of list(thing.turns)) {
    for (const entry of list(isRecord(turn) ? turn.items : undefined)) {
      if (!isRecord(entry)) continue;
      if (entry.type === "userMessage") {
        const body = list(entry.content).map((part) => (isRecord(part) && part.type === "text" ? text(part.text) : null)).filter(Boolean).join("\n");
        if (body) messages.push({ role: "user", text: body });
      } else if (entry.type === "agentMessage" && text(entry.text)) {
        messages.push({ role: "assistant", text: entry.text as string });
      }
    }
  }
  return messages;
}

const clippedLines = (lines: Array<string | null | undefined | false>) => {
  const joined = lines.filter((line): line is string => typeof line === "string" && line.length > 0).join("\n");
  return joined.length > MAX_DETAIL ? `${joined.slice(0, MAX_DETAIL - 1)}…` : joined;
};

// Engine ----------------------------------------------------------------------------------------------------------

type Run = {
  threadId: string;
  turnId: string | null;
  signal: AbortSignal;
  emit: (event: CodexTurnEvent) => unknown;
  known: Map<string, Known>;
  streamed: Map<string, string>;
  completedMessages: Array<{ id: string; text: string; phase: string | null }>;
  order: string[];
  lastDeltaItem: string | null;
  emittedAny: boolean;
  lastError: string | null;
  stopRequested: boolean;
  /** Interrupts the turn from outside the request (the island's Detener while Dot uses the computer). */
  stop: () => void;
  finish: (result: CodexTurnResult) => void;
};

export function createCodexEngine(transport: CodexTransport, deps: CodexEngineDeps = {}) {
  const setTimer = deps.setTimer ?? ((callback: () => void, ms: number) => {
    const handle = setTimeout(callback, ms);
    handle.unref?.();
    return handle;
  });
  const clearTimer = deps.clearTimer ?? ((handle: unknown) => clearTimeout(handle as NodeJS.Timeout));
  const broker = () => deps.broker ?? getAgentBroker();
  const runs = new Map<string, Run>();
  const loaded = new Set<string>();
  const openRequests = new Map<string, AbortController>();
  /** MCP calls in progress on the Chrome and computer servers: while one runs, Dot is using the user's machine. */
  const computerCalls = new Map<string, { threadId: string; server: string }>();

  const notReady = () => {
    const status = transport.status();
    return status.state === "ready" ? null : (status.message ?? "Codex no está disponible.");
  };
  const requireReady = () => {
    const problem = notReady();
    if (problem) throw new Error(problem);
  };

  // A restart loses every loaded thread; a crash ends every running turn.
  transport.onRestart(() => loaded.clear());
  transport.onStatus((status) => {
    if (status.state === "ready") return;
    // The process that asked is gone: its cards must not stay up to be answered.
    for (const controller of [...openRequests.values()]) controller.abort();
    for (const run of [...runs.values()]) run.finish({ text: finalText(run), status: "failed", error: CRASHED });
  });

  function finalText(run: Run) {
    const answers = run.completedMessages.filter((message) => message.phase !== "commentary");
    const chosen = answers.at(-1) ?? run.completedMessages.at(-1);
    if (chosen) return chosen.text;
    return run.order.map((id) => run.streamed.get(id) ?? "").join("\n\n");
  }

  // Notifications -----------------------------------------------------------------------------------------------

  function toolEvent(run: Run, event: ToolEvent) {
    try { run.emit({ type: "tool", tool: event }); } catch { /* A UI failure never breaks the turn. */ }
  }

  function delta(run: Run, itemId: string, chunk: string) {
    if (chunk === "") return;
    if (run.lastDeltaItem !== null && run.lastDeltaItem !== itemId && run.emittedAny) {
      try { run.emit({ type: "delta", text: "\n\n" }); } catch { /* ditto */ }
    }
    run.lastDeltaItem = itemId;
    run.emittedAny = true;
    if (!run.streamed.has(itemId)) run.order.push(itemId);
    run.streamed.set(itemId, (run.streamed.get(itemId) ?? "") + chunk);
    try { run.emit({ type: "delta", text: chunk }); } catch { /* ditto */ }
  }

  function onNotification(method: string, params: unknown) {
    if (!isRecord(params)) return;
    if (method === "serverRequest/resolved") {
      // Codex settled a request itself (the turn was interrupted, say): its card has nothing left to ask.
      if (typeof params.requestId === "string" || typeof params.requestId === "number") openRequests.get(String(params.requestId))?.abort();
      return;
    }
    const threadId = text(params.threadId);
    const run = threadId ? runs.get(threadId) : undefined;
    if (!run) return;
    const turn = isRecord(params.turn) ? params.turn : undefined;
    const turnId = text(params.turnId) ?? (turn ? text(turn.id) : null);
    if (method === "turn/started") {
      if (run.turnId === null && turnId) run.turnId = turnId;
      return;
    }
    if (run.turnId !== null && turnId !== null && turnId !== run.turnId) return;

    if (method === "item/agentMessage/delta") {
      const itemId = text(params.itemId);
      if (itemId && typeof params.delta === "string") delta(run, itemId, params.delta);
    } else if (method === "item/started" || method === "item/completed") {
      const thing = params.item as ThreadItem | undefined;
      if (!isRecord(thing) || typeof thing.id !== "string") return;
      if (thing.type === "agentMessage" && method === "item/completed") {
        const body = typeof thing.text === "string" ? thing.text : "";
        if (!run.streamed.has(thing.id)) delta(run, thing.id, body);
        run.completedMessages.push({ id: thing.id, text: body, phase: text(thing.phase) });
        return;
      }
      if (thing.type === "mcpToolCall" && (thing.server === CHROME_SERVER || thing.server === COMPUTER_SERVER)) {
        if (thing.status === "inProgress") computerCalls.set(thing.id, { threadId: run.threadId, server: thing.server });
        else computerCalls.delete(thing.id);
      }
      const mapped = toolRowFor(thing);
      if (!mapped) return;
      run.known.set(thing.id, mapped.known);
      toolEvent(run, mapped.event);
    } else if (method === "error") {
      if (params.willRetry !== true && isRecord(params.error)) run.lastError = text(params.error.message);
    } else if (method === "turn/completed" && turn) {
      const status = turn.status;
      const error = isRecord(turn.error) ? text(turn.error.message) : null;
      if (status === "completed") run.finish({ text: finalText(run), status: "completed" });
      else if (status === "interrupted") run.finish({ text: finalText(run), status: "interrupted" });
      else run.finish({ text: finalText(run), status: "failed", error: error ?? run.lastError ?? FAILED });
    }
  }
  transport.onNotification((notification) => {
    try { onNotification(notification.method, notification.params); } catch { /* A malformed notification is skipped. */ }
  });

  // Approvals ---------------------------------------------------------------------------------------------------

  /** Asks on the island. The card is withdrawn when the turn is stopped or Codex resolves the request itself. */
  async function askUser(id: string | number, threadId: string | null, input: { summary: string; detail: string; alwaysOffered: boolean; itemId: string | null; questions?: AgentQuestion[] }): Promise<CodexApproval> {
    const run = threadId ? runs.get(threadId) : undefined;
    const controller = new AbortController();
    openRequests.set(String(id), controller);
    const signal = run ? AbortSignal.any([run.signal, controller.signal]) : controller.signal;
    const known = run && input.itemId ? run.known.get(input.itemId) : undefined;
    if (run && input.itemId) toolEvent(run, { id: input.itemId, name: known?.name ?? CODEX_TOOL_NAME, summary: known?.summary ?? input.summary, status: "waiting_approval" });
    let answer: CodexApproval;
    try {
      answer = await broker().requestCodexApproval({ tool: CARD_TOOL, summary: input.summary, detail: input.detail, alwaysOffered: input.alwaysOffered, ...(input.questions ? { questions: input.questions } : {}), signal });
    } finally {
      openRequests.delete(String(id));
    }
    if (run && input.itemId && !run.signal.aborted && !input.questions) {
      toolEvent(run, { id: input.itemId, name: known?.name ?? CODEX_TOOL_NAME, summary: known?.summary ?? input.summary, status: answer.choice === "allow" || answer.choice === "always" ? "running" : "denied" });
    }
    return answer;
  }

  async function commandApproval(id: string | number, params: CommandExecutionRequestApprovalParams) {
    const decisions = Array.isArray(params.availableDecisions) ? params.availableDecisions : null;
    const sessionOffered = decisions !== null && decisions.includes("acceptForSession");
    const canAccept = decisions === null || decisions.includes("accept");
    const command = text(params.command);
    const stdin = params.kind === "writeStdin";
    const summary = oneLine(command ?? text(params.reason) ?? (stdin ? "Escribir en un terminal en ejecución" : "Ejecutar un comando"), 200);
    const detail = clippedLines([command, text(params.cwd) && `Carpeta: ${params.cwd}`, text(params.reason) && `Motivo: ${params.reason}`]);
    const answer = await askUser(id, text(params.threadId), { summary, detail, alwaysOffered: sessionOffered, itemId: text(params.itemId) });
    const decision = !canAccept ? "decline" : answer.choice === "allow" ? "accept" : answer.choice === "always" && sessionOffered ? "acceptForSession" : "decline";
    return { decision };
  }

  async function fileChangeApproval(id: string | number, params: FileChangeRequestApprovalParams) {
    const threadId = text(params.threadId);
    const itemId = text(params.itemId);
    const known = threadId && itemId ? runs.get(threadId)?.known.get(itemId) : undefined;
    const summary = known?.summary ?? "Codex quiere modificar archivos";
    const detail = clippedLines([text(params.reason) && `Motivo: ${params.reason}`, text(params.grantRoot) && `Carpeta: ${params.grantRoot}`, ...(known?.paths ?? [])]);
    // The files are the ones Codex announced on the item; Siempre is the protocol's own acceptForSession for file changes.
    const answer = await askUser(id, threadId, { summary, detail, alwaysOffered: true, itemId });
    return { decision: answer.choice === "allow" ? "accept" : answer.choice === "always" ? "acceptForSession" : "decline" };
  }

  async function permissionsApproval(id: string | number, params: PermissionsRequestApprovalParams) {
    const asked = params.permissions;
    const network = asked.network && asked.network.enabled !== null ? asked.network : null;
    const reads = list(asked.fileSystem?.read).filter((path): path is string => typeof path === "string");
    const writes = list(asked.fileSystem?.write).filter((path): path is string => typeof path === "string");
    const detail = clippedLines([
      text(params.reason) && `Motivo: ${params.reason}`,
      text(params.cwd) && `Carpeta: ${params.cwd}`,
      network?.enabled ? "Acceso a la red" : null,
      reads.length > 0 && `Leer: ${reads.join(", ")}`,
      writes.length > 0 && `Escribir: ${writes.join(", ")}`,
    ]);
    const answer = await askUser(id, text(params.threadId), { summary: "Codex pide permisos adicionales", detail, alwaysOffered: true, itemId: null });
    if (answer.choice !== "allow" && answer.choice !== "always") return { permissions: {}, scope: "turn" };
    const granted: Record<string, unknown> = {};
    if (asked.network) granted.network = asked.network;
    if (asked.fileSystem) granted.fileSystem = asked.fileSystem;
    return { permissions: granted, scope: answer.choice === "always" ? "session" : "turn" };
  }

  async function userInputRequest(id: string | number, params: ToolRequestUserInputParams) {
    const declined = { answers: {} };
    const asked = list(params.questions);
    const byQuestion = new Map<string, string>();
    const shaped: unknown[] = [];
    for (const entry of asked) {
      if (!isRecord(entry) || entry.isSecret === true || !Array.isArray(entry.options) || entry.options.length === 0) return declined;
      const question = text(entry.question);
      const questionId = text(entry.id);
      if (!question || !questionId || byQuestion.has(question)) return declined;
      byQuestion.set(question, questionId);
      shaped.push({ question, header: text(entry.header) ?? "", multiSelect: false, options: entry.options.map((option) => (isRecord(option) ? { label: option.label, description: option.description } : option)) });
    }
    const questions = parseQuestions({ questions: shaped });
    if (!questions) return declined;
    const answer = await askUser(id, text(params.threadId), { summary: "Codex te hace una pregunta", detail: "", alwaysOffered: false, itemId: null, questions });
    if (answer.choice !== "allow" || !answer.answers) return declined;
    const answers: Record<string, { answers: string[] }> = {};
    for (const [question, questionId] of byQuestion) {
      const given = answer.answers[question];
      if (given === undefined) return declined;
      answers[questionId] = { answers: [given] };
    }
    return { answers };
  }

  async function elicitationRequest(id: string | number, params: McpServerElicitationRequestParams) {
    const declined = { action: "decline", content: null, _meta: null };
    if (params.mode !== "form") return declined;
    const meta = isRecord(params._meta) ? params._meta : {};
    const toolCall = meta.codex_approval_kind === "mcp_tool_call";
    const properties = isRecord(params.requestedSchema) && isRecord(params.requestedSchema.properties) ? Object.keys(params.requestedSchema.properties) : [];
    // A form that asks for values cannot be filled from here; only a plain confirmation can be answered.
    if (!toolCall && properties.length > 0) return declined;
    const persist = meta.persist;
    const sessionOffered = toolCall && (persist === "session" || (Array.isArray(persist) && persist.includes("session")));
    const message = oneLine(text(params.message) ?? "Confirmación", 200);
    const answer = await askUser(id, text(params.threadId), { summary: `${params.serverName}: ${message}`, detail: clippedLines([params.message]), alwaysOffered: sessionOffered, itemId: null });
    if (answer.choice === "allow") return { action: "accept", content: toolCall ? null : {}, _meta: null };
    if (answer.choice === "always" && sessionOffered) return { action: "accept", content: null, _meta: { persist: "session" } };
    return declined;
  }

  async function onServerRequest(request: { id: string | number; method: string; params: unknown }) {
    const params = isRecord(request.params) ? request.params : {};
    switch (request.method) {
      case "item/commandExecution/requestApproval": return commandApproval(request.id, params as unknown as CommandExecutionRequestApprovalParams);
      case "item/fileChange/requestApproval": return fileChangeApproval(request.id, params as unknown as FileChangeRequestApprovalParams);
      case "item/permissions/requestApproval": return permissionsApproval(request.id, params as unknown as PermissionsRequestApprovalParams);
      case "item/tool/requestUserInput": return userInputRequest(request.id, params as unknown as ToolRequestUserInputParams);
      case "mcpServer/elicitation/request": return elicitationRequest(request.id, params as unknown as McpServerElicitationRequestParams);
      default: throw new Error(`Method not found: ${request.method}`);
    }
  }
  transport.setRequestHandler(onServerRequest);

  // Public API --------------------------------------------------------------------------------------------------

  return {
    status: () => transport.status(),

    /** What Dot is doing to the user's machine right now: the servers with a call in progress and the chats they run in. */
    activity(): { computerUse: { active: boolean; servers: string[]; threadIds: string[] } } {
      const calls = [...computerCalls.values()];
      return { computerUse: { active: calls.length > 0, servers: [...new Set(calls.map((call) => call.server))], threadIds: [...new Set(calls.map((call) => call.threadId))] } };
    },

    /** Detener from the island: interrupts every turn that has a Chrome or computer call in progress. Returns how many were asked to stop. */
    stopComputerUse(): number {
      const threadIds = new Set([...computerCalls.values()].map((call) => call.threadId));
      let stopped = 0;
      for (const threadId of threadIds) {
        const run = runs.get(threadId);
        if (!run) continue;
        run.stop();
        stopped += 1;
      }
      return stopped;
    },

    /** Removes a chat from the history (Codex keeps archived chats on disk; they only stop being listed). */
    async archiveThread(threadId: string): Promise<void> {
      requireReady();
      await transport.request("thread/archive", { threadId });
      loaded.delete(threadId);
    },

    async accountStatus(): Promise<CodexAccountStatus> {
      const problem = notReady();
      if (problem) return { state: "unavailable", message: problem };
      try {
        const result = await transport.request("account/read", { refreshToken: false });
        const account = isRecord(result) && isRecord(result.account) ? result.account : null;
        if (!account) return isRecord(result) && result.requiresOpenaiAuth === false ? { state: "signedIn", method: "other", email: null, plan: null } : { state: "signedOut" };
        if (account.type === "chatgpt") return { state: "signedIn", method: "chatgpt", email: text(account.email), plan: text(account.planType) };
        if (account.type === "apiKey") return { state: "signedIn", method: "apiKey", email: null, plan: null };
        return { state: "signedIn", method: "other", email: null, plan: null };
      } catch {
        return { state: "unavailable", message: "Dot no pudo leer tu sesión de Codex." };
      }
    },

    /** Starts the ChatGPT sign-in: the caller opens `authUrl` and then waits with `waitForLogin`. */
    async startLogin(): Promise<{ loginId: string; authUrl: string }> {
      requireReady();
      const result = await transport.request("account/login/start", { type: "chatgpt" });
      const loginId = isRecord(result) ? text(result.loginId) : null;
      const authUrl = isRecord(result) ? text(result.authUrl) : null;
      if (!loginId || !authUrl) throw new Error("Codex no devolvió una dirección de inicio de sesión.");
      return { loginId, authUrl };
    },

    waitForLogin(loginId: string, signal: AbortSignal): Promise<{ success: boolean; error: string | null }> {
      return new Promise((resolve) => {
        const done = (value: { success: boolean; error: string | null }) => {
          off();
          signal.removeEventListener("abort", onAbort);
          resolve(value);
        };
        const off = transport.onNotification((notification) => {
          if (notification.method !== "account/login/completed" || !isRecord(notification.params) || notification.params.loginId !== loginId) return;
          done({ success: notification.params.success === true, error: text(notification.params.error) });
        });
        function onAbort() {
          void Promise.resolve().then(() => transport.request("account/login/cancel", { loginId })).catch(() => {});
          done({ success: false, error: "cancelled" });
        }
        if (signal.aborted) onAbort();
        else signal.addEventListener("abort", onAbort, { once: true });
      });
    },

    /** The thread to chat in: resumed when it has history, started otherwise (a thread that never had a turn has nothing to resume). */
    async startOrResumeThread(input: { threadId?: string; cwd: string }): Promise<{ threadId: string; resumed: boolean }> {
      requireReady();
      const settings = { cwd: input.cwd, approvalPolicy: CODEX_APPROVAL_POLICY, sandbox: CODEX_SANDBOX_MODE };
      const start = async () => {
        const result = await transport.request("thread/start", settings satisfies ThreadStartParams);
        const id = isRecord(result) && isRecord(result.thread) ? text(result.thread.id) : null;
        if (!id) throw new Error("Codex no devolvió el chat nuevo.");
        loaded.add(id);
        return { threadId: id, resumed: false };
      };
      const { threadId } = input;
      if (!threadId) return start();
      if (loaded.has(threadId)) return { threadId, resumed: true };
      try {
        await transport.request("thread/resume", { threadId, ...settings, excludeTurns: true } satisfies ThreadResumeParams);
        loaded.add(threadId);
        return { threadId, resumed: true };
      } catch (error) {
        if (error instanceof Error && /no rollout found/i.test(error.message)) return start();
        throw error;
      }
    },

    /**
     * Sends the user's message and streams the answer through `emit` as `delta` and `tool` events, the same shapes the chat
     * route sends from the built-in loop. Resolves when the turn ends: completed, interrupted (Detener) or failed. It throws
     * only when Codex cannot be reached to start the turn.
     */
    async runTurn(request: CodexTurnRequest): Promise<CodexTurnResult> {
      const { threadId, signal } = request;
      if (runs.has(threadId)) throw new Error("This Codex chat already has a response in progress.");
      if (signal.aborted) return { text: "", status: "interrupted" };
      requireReady();
      const input: UserInput[] = [{ type: "text", text: request.text, text_elements: [] }, ...(request.localImages ?? []).map((path): UserInput => ({ type: "localImage", path }))];

      let finishRun: (result: CodexTurnResult) => void = () => {};
      const finished = new Promise<CodexTurnResult>((resolve) => { finishRun = resolve; });
      let graceTimer: unknown;
      const run: Run = {
        threadId,
        turnId: null,
        signal,
        emit: request.emit,
        known: new Map(),
        streamed: new Map(),
        completedMessages: [],
        order: [],
        lastDeltaItem: null,
        emittedAny: false,
        lastError: null,
        stopRequested: false,
        stop: () => {},
        finish: (result) => {
          if (graceTimer !== undefined) clearTimer(graceTimer);
          runs.delete(threadId);
          for (const [itemId, call] of computerCalls) if (call.threadId === threadId) computerCalls.delete(itemId);
          finishRun(result);
        },
      };
      runs.set(threadId, run);

      const interrupt = () => {
        if (run.turnId === null || graceTimer !== undefined) return;
        // Codex confirms with turn/completed (interrupted); if it never does, the turn still ends.
        graceTimer = setTimer(() => run.finish({ text: finalText(run), status: "interrupted" }), INTERRUPT_GRACE_MS);
        void Promise.resolve().then(() => transport.request("turn/interrupt", { threadId, turnId: run.turnId }, { timeoutMs: 5_000 })).catch(() => {});
      };
      const onAbort = () => {
        run.stopRequested = true;
        interrupt();
      };
      run.stop = onAbort;
      signal.addEventListener("abort", onAbort, { once: true });
      try {
        let started: unknown;
        try {
          // Not cancellable: a turn that started on Codex's side must be interrupted, not forgotten.
          started = await transport.request("turn/start", { threadId, input } satisfies TurnStartParams);
        } catch (error) {
          run.finish({ text: "", status: "failed" });
          throw error;
        }
        const turnId = isRecord(started) && isRecord(started.turn) ? text(started.turn.id) : null;
        if (turnId && run.turnId === null) run.turnId = turnId;
        if (run.stopRequested || signal.aborted) interrupt();
        return await finished;
      } finally {
        signal.removeEventListener("abort", onAbort);
        runs.delete(threadId);
      }
    },

    /** Newest first, searchable, paged: the history view when Codex is the engine. */
    async listThreads(options: { cursor?: string | null; searchTerm?: string | null; limit?: number }): Promise<{ threads: CodexThreadView[]; nextCursor: string | null }> {
      requireReady();
      const params: ThreadListParams = { limit: options.limit ?? HISTORY_PAGE, sortKey: "updated_at", sortDirection: "desc", archived: false };
      if (options.cursor) params.cursor = options.cursor;
      if (options.searchTerm) params.searchTerm = options.searchTerm;
      const result = await transport.request("thread/list", params);
      const threads = list(isRecord(result) ? result.data : undefined).map(summarizeThread).filter((view): view is CodexThreadView => view !== undefined);
      return { threads, nextCursor: isRecord(result) ? text(result.nextCursor) : null };
    },

    /**
     * One thread as a small view. With `includeMessages`, also what was said: read with the thread, or, when Codex will not
     * hydrate turns that way (a thread that was never used), from `thread/turns/list`.
     */
    async readThread(threadId: string, options: { includeMessages?: boolean } = {}): Promise<CodexThreadView & { messages?: CodexThreadMessage[] }> {
      requireReady();
      const withMessages = options.includeMessages === true;
      let result: unknown;
      let fallback: unknown;
      try {
        result = await transport.request("thread/read", { threadId, includeTurns: withMessages });
      } catch (error) {
        if (!withMessages) throw error;
        result = await transport.request("thread/read", { threadId, includeTurns: false });
        fallback = await transport.request("thread/turns/list", { threadId, limit: HISTORY_PAGE, sortDirection: "asc", itemsView: "full" }).catch(() => undefined);
      }
      const found = isRecord(result) ? result.thread : undefined;
      const view = summarizeThread(found);
      if (!view || !isRecord(found)) throw new Error("Codex no encontró ese chat.");
      if (!withMessages) return view;
      const turns = fallback === undefined ? found : { turns: isRecord(fallback) ? fallback.data : [] };
      return { ...view, messages: messagesOf(turns) };
    },
  };
}

export type CodexEngine = ReturnType<typeof createCodexEngine>;

/**
 * The engine over the transport the desktop shell published, or null when there is none (plain development, other systems)
 * or it is not a transport. One engine is kept per transport on `globalThis`, so every route shares its threads and runs.
 */
export function getCodexEngine(): CodexEngine | null {
  const holder = globalThis as Record<symbol, unknown>;
  const transport = holder[CODEX_ENGINE_KEY];
  if (!isTransport(transport)) return null;
  const existing = holder[INSTANCE_KEY];
  if (isRecord(existing) && existing.transport === transport && existing.engine) return existing.engine as CodexEngine;
  const engine = createCodexEngine(transport);
  holder[INSTANCE_KEY] = { transport, engine };
  return engine;
}
