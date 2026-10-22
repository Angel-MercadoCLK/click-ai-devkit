// What the chat page knows about tool calls: the `tool` events the stream route sends and how the page folds them into
// activity lines. No Node imports: the page uses this file too.

export type ToolName = "powershell" | "claude_code" | "screen";
export type ToolStatus = "running" | "waiting_approval" | "done" | "denied" | "error";
/** Repeated events with the same id update that call's state. */
export type ToolEvent = { id: string; name: ToolName; summary: string; status: ToolStatus; detail?: string };

export const TOOL_NAMES: readonly ToolName[] = ["powershell", "claude_code", "screen"];
export const TOOL_STATUSES: readonly ToolStatus[] = ["running", "waiting_approval", "done", "denied", "error"];
export const MAX_TOOL_DETAIL = 2_000;
const MAX_TOOL_SUMMARY = 200;
const MAX_ACTIVITIES = 40;

export const TOOL_STATUS_LABELS: Record<ToolStatus, string> = {
  running: "Ejecutando",
  waiting_approval: "Esperando tu permiso",
  done: "Listo",
  denied: "Denegado",
  error: "Error",
};

export const TOOL_NAME_LABELS: Record<ToolName, string> = { powershell: "PowerShell", claude_code: "Claude Code", screen: "Pantalla" };

/** The end of a long output: what a finished command ended with matters most. */
export function clipDetail(text: string, limit = MAX_TOOL_DETAIL) {
  return text.length <= limit ? text : `…${text.slice(text.length - (limit - 1))}`;
}

/** One line of at most `limit` characters, for a summary. */
export function oneLine(text: string, limit = MAX_TOOL_SUMMARY) {
  const flat = text.replace(/\s+/g, " ").trim();
  return flat.length <= limit ? flat : `${flat.slice(0, limit - 1)}…`;
}

/** A well-formed `tool` event, or undefined for anything else. */
export function readToolEvent(value: unknown): ToolEvent | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  const { id, name, summary, status, detail } = value as Record<string, unknown>;
  const knownName = TOOL_NAMES.find((candidate) => candidate === name);
  const knownStatus = TOOL_STATUSES.find((candidate) => candidate === status);
  if (typeof id !== "string" || id.length < 1 || id.length > 200 || !knownName || !knownStatus || typeof summary !== "string") return undefined;
  if (detail !== undefined && (typeof detail !== "string" || detail.length > MAX_TOOL_DETAIL)) return undefined;
  return { id, name: knownName, summary, status: knownStatus, ...(detail === undefined ? {} : { detail }) };
}

/** Updates the call with the same id, or adds a new one at the end. A later event without a detail keeps the earlier one. */
export function applyToolEvent(list: ToolEvent[], event: ToolEvent): ToolEvent[] {
  const index = list.findIndex((item) => item.id === event.id);
  if (index === -1) return [...list, event].slice(-MAX_ACTIVITIES);
  return list.map((item, position) => (position === index ? { ...item, ...event, detail: event.detail ?? item.detail } : item));
}

export const isNewToolCall = (list: ToolEvent[], event: ToolEvent) => !list.some((item) => item.id === event.id);
