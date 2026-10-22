// The routes the chat card and the island use around the Codex engine (S1, S3, S5, S6): its status and sign-in, the chat
// history (Codex threads), the background Claude Code tasks and the "Detener" of computer use. Every route has the same guard
// as the other control routes: the exact local Host (and Origin for a write) and the per-launch control cookie.
//
// The handlers are built by `createEngineHandlers` so tests can supply the engine; the route files use the default set, which
// reads everything the desktop shell published on `globalThis`. Error text from Codex never reaches the page: each failure
// has one fixed Spanish message.

import { isLocalRequest, noStore } from "./auth-http.ts";
import type { ClaudeTaskView } from "./agent-state.ts";
import { getCodexEngine, type CodexEngine } from "./codex-engine.ts";
import { hasControlAccess } from "./control-token.ts";
import { engineStatus, getLoginFlow, getSystemBrowser, readExternalTools, type ExternalToolsView, type LoginFlow, type SystemBrowser } from "./engine.ts";

const CLAUDE_TASKS_KEY = Symbol.for("click-dot.claude-tasks");
const THREAD_ID = /^[0-9A-Za-z._-]{8,100}$/;
const MAX_QUERY = 200;
const MAX_CURSOR = 500;
const TASK_STATES = ["running", "done", "failed", "cancelled", "timed_out"];

const isRecord = (value: unknown): value is Record<string, unknown> => !!value && typeof value === "object" && !Array.isArray(value);
const fail = (status: number, error: string) => Response.json({ error }, { status, headers: noStore });
const ok = (body: unknown, status = 200) => Response.json(body, { status, headers: noStore });
const guarded = (request: Request, write: boolean) => isLocalRequest(request, write) && hasControlAccess(request);

const NOT_AVAILABLE = "Codex no está disponible.";
const HISTORY_FAILED = "Dot no pudo leer tus chats de Codex.";

/** The background tasks the shell published (desktop/claude-tasks.mts), as plain views. Anything malformed is skipped. */
export function readClaudeTasks(): ClaudeTaskView[] {
  const published = (globalThis as Record<symbol, unknown>)[CLAUDE_TASKS_KEY];
  if (!isRecord(published) || typeof published.list !== "function") return [];
  let listed: unknown;
  try {
    listed = (published.list as () => unknown)();
  } catch {
    return [];
  }
  if (!Array.isArray(listed)) return [];
  const tasks: ClaudeTaskView[] = [];
  for (const item of listed) {
    if (!isRecord(item)) continue;
    const { id, project, state, latestLine, startedAt } = item;
    if (typeof id !== "string" || typeof project !== "string" || !TASK_STATES.includes(state as string) || (latestLine !== null && typeof latestLine !== "string") || typeof startedAt !== "number") continue;
    tasks.push({ id, project, state: state as ClaudeTaskView["state"], latestLine, startedAt });
  }
  return tasks;
}

export type EngineHandlerDeps = {
  getEngine?: () => CodexEngine | null;
  readTools?: () => ExternalToolsView | null;
  loginFlow?: LoginFlow;
  getSystemBrowser?: () => SystemBrowser | null;
  readTasks?: () => ClaudeTaskView[];
};

export function createEngineHandlers(deps: EngineHandlerDeps = {}) {
  const engine = () => (deps.getEngine ?? getCodexEngine)();
  /** The engine when it is running; otherwise the reason, as the response to send. */
  const running = (): { engine: CodexEngine } | { response: Response } => {
    const found = engine();
    if (!found) return { response: fail(409, NOT_AVAILABLE) };
    const status = found.status();
    return status.state === "ready" ? { engine: found } : { response: fail(409, status.message ?? NOT_AVAILABLE) };
  };

  return {
    /** GET /api/codex/status */
    async status(request: Request): Promise<Response> {
      if (!guarded(request, false)) return fail(403, "Request rejected.");
      return ok(await engineStatus({ getEngine: engine, readTools: deps.readTools ?? readExternalTools, ...(deps.loginFlow ? { loginFlow: deps.loginFlow } : {}) }));
    },

    /** POST /api/codex/login starts the ChatGPT sign-in and opens its page; DELETE cancels it. */
    async login(request: Request): Promise<Response> {
      if (!guarded(request, true)) return fail(403, "Request rejected.");
      const flow = deps.loginFlow ?? getLoginFlow();
      if (request.method === "DELETE") {
        flow.cancel();
        return ok({ cancelled: true });
      }
      const found = running();
      if ("response" in found) return found.response;
      const started = await flow.start(found.engine, (deps.getSystemBrowser ?? getSystemBrowser)());
      return started.ok ? ok({ authUrl: started.authUrl, opened: started.opened }) : fail(502, started.error);
    },

    /** GET /api/codex/threads?q=&cursor= : the history when Codex is the engine, newest first. */
    async threads(request: Request): Promise<Response> {
      if (!guarded(request, false)) return fail(403, "Request rejected.");
      const found = running();
      if ("response" in found) return found.response;
      const params = new URL(request.url).searchParams;
      const q = params.get("q");
      const cursor = params.get("cursor");
      if ((q !== null && q.length > MAX_QUERY) || (cursor !== null && cursor.length > MAX_CURSOR)) return fail(400, "The search is too long.");
      try {
        const page = await found.engine.listThreads({ ...(q ? { searchTerm: q } : {}), ...(cursor ? { cursor } : {}) });
        return ok({ threads: page.threads, nextCursor: page.nextCursor });
      } catch {
        return fail(502, HISTORY_FAILED);
      }
    },

    /** GET /api/codex/threads/<id> opens a chat with its messages; DELETE archives it. */
    async thread(request: Request, id: string): Promise<Response> {
      const write = request.method === "DELETE";
      if (!guarded(request, write)) return fail(403, "Request rejected.");
      if (!THREAD_ID.test(id)) return fail(400, "The chat id is not valid.");
      const found = running();
      if ("response" in found) return found.response;
      try {
        if (write) {
          await found.engine.archiveThread(id);
          return ok({ archived: true });
        }
        return ok({ thread: await found.engine.readThread(id, { includeMessages: true }) });
      } catch {
        return fail(502, HISTORY_FAILED);
      }
    },

    /** GET /api/agent/activity: the background Claude Code tasks and whether Dot is using the computer now. */
    async activity(request: Request): Promise<Response> {
      if (!guarded(request, false)) return fail(403, "Request rejected.");
      const found = engine();
      const computer = found && found.status().state === "ready" ? found.activity().computerUse : undefined;
      return ok({
        tasks: (deps.readTasks ?? readClaudeTasks)(),
        computerUse: { active: computer?.active ?? false, servers: computer?.servers ?? [] },
      });
    },

    /** POST /api/codex/stop: Detener on "Dot está usando tu equipo". */
    async stop(request: Request): Promise<Response> {
      if (!guarded(request, true)) return fail(403, "Request rejected.");
      const found = engine();
      return ok({ stopped: found && found.status().state === "ready" ? found.stopComputerUse() : 0 });
    },
  };
}

export const engineHandlers = createEngineHandlers();
