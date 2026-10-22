// What the chat card's history view shows, as plain functions: newest conversation first, a title taken from the first
// message and a short relative time in Spanish. It reads the conversations the existing store already lists; nothing here
// writes or changes them.

type HistoryMessage = { role: "user" | "assistant"; content: string };
type HistoryConversation = { id: string; title: string; updatedAt: number; messages: readonly HistoryMessage[] };

export type HistoryEntry = { id: string; title: string; when: string };

/** Longest title shown (characters, ellipsis included): the list is one line per conversation. */
export const TITLE_MAX = 60;
const UNTITLED = "Chat sin título";
const MONTHS = ["ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"];
const MINUTE = 60_000;
const HOUR = 60 * MINUTE;

const oneLine = (text: string) => text.replace(/\s+/g, " ").trim();

/** The first thing the user said, on one line; the stored title when there is no message; a Spanish name when there is neither. */
export function historyTitle(conversation: HistoryConversation): string {
  const first = conversation.messages.find((message) => message.role === "user");
  const text = oneLine(first?.content ?? "") || oneLine(conversation.title);
  if (!text) return UNTITLED;
  return text.length <= TITLE_MAX ? text : `${text.slice(0, TITLE_MAX - 1).trimEnd()}…`;
}

const startOfDay = (time: number) => {
  const date = new Date(time);
  return new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime();
};

/** "ahora", "hace 5 min", "hace 2 h", "ayer", "hace 3 días", then a date ("12 oct", with the year when it is not this one). */
export function relativeTimeEs(time: number, now: number): string {
  if (!Number.isFinite(time) || !Number.isFinite(now)) return "";
  const elapsed = now - time;
  if (elapsed < 45_000) return "ahora";
  if (elapsed < HOUR) return `hace ${Math.max(1, Math.round(elapsed / MINUTE))} min`;
  const days = Math.round((startOfDay(now) - startOfDay(time)) / 86_400_000);
  if (days <= 0) return `hace ${Math.max(1, Math.floor(elapsed / HOUR))} h`;
  if (days === 1) return "ayer";
  if (days < 7) return `hace ${days} días`;
  const date = new Date(time);
  const label = `${date.getDate()} ${MONTHS[date.getMonth()]}`;
  return date.getFullYear() === new Date(now).getFullYear() ? label : `${label} ${date.getFullYear()}`;
}

/** The list, newest first (conversations changed at the same moment keep their order). */
export function historyEntries(conversations: readonly HistoryConversation[], now: number): HistoryEntry[] {
  return conversations
    .map((conversation, index) => ({ conversation, index }))
    .sort((a, b) => b.conversation.updatedAt - a.conversation.updatedAt || a.index - b.index)
    .map(({ conversation }) => ({ id: conversation.id, title: historyTitle(conversation), when: relativeTimeEs(conversation.updatedAt, now) }));
}
