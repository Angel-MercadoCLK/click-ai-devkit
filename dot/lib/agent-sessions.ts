// What the mascot knows about live Claude Code sessions, built from hook events.
//
// Everything here lives in memory only: nothing is written to disk and nothing is sent anywhere. A session is keyed by
// its id; the store keeps a bounded number of sessions and a bounded history for each, and forgets a session when it ends.

export type SessionState = "trabajando" | "te necesita" | "listo";
export type AgentSessionView = { id: string; project: string; state: SessionState; action: string | null; updatedAt: number };

export const MAX_SESSIONS = 20;
export const MAX_HISTORY = 10;
const MAX_SESSION_ID_LENGTH = 200;
/** Longest piece of user text (a command, a pattern) shown in an action line. */
const MAX_ACTION_TEXT = 80;
const FALLBACK_PROJECT = "Sesión";

const isRecord = (value: unknown): value is Record<string, unknown> => !!value && typeof value === "object" && !Array.isArray(value);
const text = (value: unknown) => (typeof value === "string" ? value : "");
const pieces = (path: string) => path.split(/[\\/]/).filter(Boolean);

/** The last folder of a working directory, on Windows and POSIX paths alike. */
export function projectName(cwd: unknown) {
  return pieces(text(cwd)).at(-1) ?? FALLBACK_PROJECT;
}

const fileName = (path: unknown) => pieces(text(path)).at(-1) ?? "archivo";

/** Lines in a piece of text; a trailing newline does not start a new line. */
export function countLines(value: unknown) {
  const content = text(value);
  if (!content) return 0;
  const lines = content.split("\n").length;
  return content.endsWith("\n") ? lines - 1 : lines;
}

/** One line of user text for an action: control characters become spaces and a long text is cut with an ellipsis. */
function clip(value: unknown, limit = MAX_ACTION_TEXT) {
  // eslint-disable-next-line no-control-regex
  const clean = text(value).replace(/[\u0000-\u001f\u007f]+/g, " ").trim();
  return clean.length > limit ? `${clean.slice(0, limit - 1)}…` : clean;
}

const firstLine = (value: unknown) => text(value).split(/\r?\n/, 1)[0] ?? "";

/** The Spanish line the mascot shows for a tool call, for example "Editando foo.ts +12 −3". */
export function describeToolAction(tool: unknown, input: unknown) {
  if (typeof tool !== "string" || !tool) return "Usando una herramienta";
  const args = isRecord(input) ? input : {};
  switch (tool) {
    case "Edit":
      return `Editando ${fileName(args.file_path)} +${countLines(args.new_string)} −${countLines(args.old_string)}`;
    case "MultiEdit": {
      const edits = Array.isArray(args.edits) ? args.edits.filter(isRecord) : [];
      const added = edits.reduce((sum, edit) => sum + countLines(edit.new_string), 0);
      const removed = edits.reduce((sum, edit) => sum + countLines(edit.old_string), 0);
      return `Editando ${fileName(args.file_path)} +${added} −${removed}`;
    }
    case "Write":
      return `Escribiendo ${fileName(args.file_path)} +${countLines(args.content)}`;
    case "Bash": {
      const command = clip(firstLine(args.command));
      return command ? `Ejecutando ${command}` : "Ejecutando un comando";
    }
    case "Read":
      return `Leyendo ${fileName(args.file_path)}`;
    case "Grep":
    case "Glob":
      return `Buscando ${clip(args.pattern) || "archivos"}`;
    case "WebSearch":
      return `Buscando ${clip(args.query) || "en la web"}`;
    default:
      return `Usando ${clip(tool, 40)}`;
  }
}

type Entry = { id: string; project: string; state: SessionState; action: string | null; updatedAt: number; pending: number; history: string[] };

export type SessionStoreOptions = { now?: () => number; maxSessions?: number; maxHistory?: number };

const RANK: Record<SessionState, number> = { "te necesita": 0, trabajando: 1, listo: 2 };

export function createSessionStore(options: SessionStoreOptions = {}) {
  const now = options.now ?? Date.now;
  const maxSessions = options.maxSessions ?? MAX_SESSIONS;
  const maxHistory = options.maxHistory ?? MAX_HISTORY;
  const sessions = new Map<string, Entry>();

  const idOf = (payload: unknown) => {
    const id = isRecord(payload) ? payload.session_id : undefined;
    return typeof id === "string" && id.length > 0 && id.length <= MAX_SESSION_ID_LENGTH ? id : undefined;
  };

  /** Makes room for a new session by dropping the least recent one, preferring sessions that are not waiting for the user. */
  function evict(keep: string) {
    while (sessions.size > maxSessions) {
      const others = [...sessions.values()].filter((entry) => entry.id !== keep);
      const idle = others.filter((entry) => entry.pending === 0);
      const pool = idle.length > 0 ? idle : others;
      const oldest = pool.reduce((best, entry) => (entry.updatedAt < best.updatedAt ? entry : best));
      sessions.delete(oldest.id);
    }
  }

  function touch(id: string, cwd: unknown) {
    let entry = sessions.get(id);
    if (!entry) {
      entry = { id, project: projectName(cwd), state: "listo", action: null, updatedAt: now(), pending: 0, history: [] };
      sessions.set(id, entry);
      evict(id);
    } else if (typeof cwd === "string" && cwd) entry.project = projectName(cwd);
    entry.updatedAt = now();
    return entry;
  }

  function setAction(entry: Entry, action: string) {
    entry.action = action;
    entry.history.push(action);
    if (entry.history.length > maxHistory) entry.history.splice(0, entry.history.length - maxHistory);
  }

  /** A session waiting for the user keeps saying so until its request is settled. */
  const setState = (entry: Entry, state: SessionState) => {
    entry.state = entry.pending > 0 ? "te necesita" : state;
  };

  return {
    /** Applies one hook event. Events without a usable session id change nothing. */
    apply(event: string, payload: unknown) {
      const id = idOf(payload);
      if (!id || !isRecord(payload)) return;
      if (event === "SessionEnd") {
        sessions.delete(id);
        return;
      }
      const entry = touch(id, payload.cwd);
      switch (event) {
        case "SessionStart":
          setState(entry, "listo");
          if (!entry.action) setAction(entry, "Sesión iniciada");
          break;
        case "Stop":
          setState(entry, "listo");
          break;
        case "UserPromptSubmit":
          setState(entry, "trabajando");
          setAction(entry, "Procesando tu mensaje");
          break;
        case "PreToolUse":
        case "PostToolUse":
          setState(entry, "trabajando");
          setAction(entry, describeToolAction(payload.tool_name, payload.tool_input));
          break;
        case "PostToolUseFailure":
          setState(entry, "trabajando");
          setAction(entry, `Falló ${clip(payload.tool_name, 40) || "una herramienta"}`);
          break;
        case "Notification":
          if (payload.notification_type === "idle_prompt") setState(entry, "listo");
          break;
        default:
          setState(entry, "trabajando");
      }
    },

    /** A request from this session is waiting for the user. */
    needsYou(sessionId: string, details: { cwd?: unknown; action: string }) {
      const entry = touch(sessionId, details.cwd);
      entry.pending += 1;
      entry.state = "te necesita";
      setAction(entry, details.action);
    },

    /**
     * One of the session's requests is over. After an answer or a disconnect Claude Code carries on; after a hand-back
     * to the terminal the terminal is the one asking, so the session still needs the user until its next event.
     */
    settle(sessionId: string, outcome: "answered" | "terminal") {
      const entry = sessions.get(sessionId);
      if (!entry) return;
      entry.pending = Math.max(0, entry.pending - 1);
      entry.updatedAt = now();
      if (entry.pending === 0) entry.state = outcome === "terminal" ? "te necesita" : "trabajando";
    },

    /** Sessions that need the user first, then the working ones, then the finished ones; newest first inside each group. */
    list(): AgentSessionView[] {
      return [...sessions.values()]
        .sort((a, b) => RANK[a.state] - RANK[b.state] || b.updatedAt - a.updatedAt)
        .map(({ id, project, state, action, updatedAt }) => ({ id, project, state, action, updatedAt }));
    },

    history: (sessionId: string): string[] => [...(sessions.get(sessionId)?.history ?? [])],
    clear: () => sessions.clear(),
  };
}

export type SessionStore = ReturnType<typeof createSessionStore>;
