"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { AGENT_POLL_MS, applyAgentPoll, INITIAL_AGENT_POLL, readAgentState, type AgentPollResult } from "../../lib/agent-state.ts";
import type { Answers } from "./agent-choices.ts";

export type AgentChoice = "allow" | "deny" | "always" | "terminal";

/**
 * Claude Code sessions and the request waiting for the user, polled from the local server. The desktop shell also
 * announces a new request with the `click-dot:agent` event so the page looks at once instead of on its next poll.
 */
export function useAgent() {
  const [poll, setPoll] = useState(INITIAL_AGENT_POLL);
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const alive = useRef(true);
  const reading = useRef(false);

  const refresh = useCallback(async () => {
    if (reading.current) return;
    reading.current = true;
    let result: AgentPollResult = { ok: false };
    try {
      const response = await fetch("/api/agent/state", { cache: "no-store" });
      const state = response.ok ? readAgentState(await response.json()) : undefined;
      if (state) result = { ok: true, state };
    } catch {
      // A failed read is not "nothing is waiting"; see applyAgentPoll.
    } finally {
      reading.current = false;
    }
    if (alive.current) setPoll((current) => applyAgentPoll(current, result));
  }, []);

  useEffect(() => {
    alive.current = true;
    void refresh();
    const timer = window.setInterval(() => void refresh(), AGENT_POLL_MS);
    const look = () => void refresh();
    window.addEventListener("click-dot:agent", look);
    window.addEventListener("focus", look);
    return () => {
      alive.current = false;
      window.clearInterval(timer);
      window.removeEventListener("click-dot:agent", look);
      window.removeEventListener("focus", look);
    };
  }, [refresh]);

  const post = useCallback(async (path: string, body: unknown) => {
    const response = await fetch(path, { method: "POST", cache: "no-store", headers: { "content-type": "application/json" }, body: JSON.stringify(body) });
    return response.ok;
  }, []);

  /** Tells the server the card of this request is on screen. Without it the request goes back to the terminal after 2 s. */
  const ack = useCallback(
    async (id: string) => {
      try {
        await post("/api/agent/ack", { id });
      } catch {
        // The request is handed back to the terminal; nothing else to do.
      }
    },
    [post],
  );

  const decide = useCallback(
    async (id: string, choice: AgentChoice, answers?: Answers) => {
      setBusy(true);
      setFailed(false);
      try {
        const ok = await post("/api/agent/decision", answers ? { id, choice, answers } : { id, choice });
        if (!ok && alive.current) setFailed(true);
      } catch {
        if (alive.current) setFailed(true);
      } finally {
        // Whatever happened, show what the server really holds.
        await refresh();
        if (alive.current) setBusy(false);
      }
    },
    [post, refresh],
  );

  return { state: poll.state, busy, failed, ack, decide };
}
