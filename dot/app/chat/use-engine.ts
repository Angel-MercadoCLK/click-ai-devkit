"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { BUILTIN_STATUS, needsRecheck, readEngineStatus, RECHECK_INTERVAL_MS, recheckDue, type RecheckTrigger } from "../../lib/engine-client.ts";
import type { EngineStatus } from "../../lib/engine.ts";

const POLL_MS = 2000;

/**
 * Which brain answers the chat (Codex or the built-in assistant) and the Codex sign-in. The status is read when the card
 * mounts, when the window comes back to the front and every two seconds while Codex is starting or a sign-in is waiting.
 * A failed read means "built-in": the chat must never be stuck waiting for Codex.
 *
 * While Codex is missing, unsupported or failed, Dot looks for it again (`POST /api/codex/recheck`) every time the chat card
 * opens, at most every 15 s while it stays open, and when the user presses "Reintentar": an update of Codex is noticed
 * without restarting Dot.
 */
export function useEngine({ chatOpen = false }: { chatOpen?: boolean } = {}) {
  const [status, setStatus] = useState<EngineStatus | null>(null);
  const [loginBusy, setLoginBusy] = useState(false);
  const [loginError, setLoginError] = useState("");
  const [rechecking, setRechecking] = useState(false);
  const alive = useRef(true);
  const statusRef = useRef<EngineStatus | null>(null);
  statusRef.current = status;
  const openRef = useRef(chatOpen);
  openRef.current = chatOpen;
  const lastRecheck = useRef<number | null>(null);
  const rechecks = useRef(false);

  const refresh = useCallback(async () => {
    let next: EngineStatus = BUILTIN_STATUS;
    try {
      const response = await fetch("/api/codex/status", { cache: "no-store" });
      next = (response.ok ? readEngineStatus(await response.json()) : undefined) ?? BUILTIN_STATUS;
    } catch {
      // Keep the built-in assistant.
    }
    if (alive.current) setStatus(next);
    return next;
  }, []);

  useEffect(() => {
    alive.current = true;
    void refresh();
    const onFocus = () => void refresh();
    window.addEventListener("focus", onFocus);
    return () => {
      alive.current = false;
      window.removeEventListener("focus", onFocus);
    };
  }, [refresh]);

  /** Asks Dot to look for Codex again, if this trigger is due; one look at a time. */
  const attempt = useCallback(async (trigger: RecheckTrigger, current?: EngineStatus | null) => {
    const now = Date.now();
    if (rechecks.current || !recheckDue({ status: current ?? statusRef.current, open: openRef.current, trigger, lastAt: lastRecheck.current, now })) return;
    rechecks.current = true;
    lastRecheck.current = now;
    if (alive.current) setRechecking(true);
    try {
      const response = await fetch("/api/codex/recheck", { method: "POST", cache: "no-store" });
      const next = response.ok ? readEngineStatus(await response.json()) : undefined;
      if (next && alive.current) setStatus(next);
    } catch {
      // The note stays; the next trigger looks again.
    } finally {
      rechecks.current = false;
      if (alive.current) setRechecking(false);
    }
  }, []);

  // Opening the chat card looks again, with a fresh read of the status first.
  useEffect(() => {
    if (!chatOpen) return;
    void (async () => {
      const next = await refresh();
      await attempt("open", next);
    })();
  }, [chatOpen, refresh, attempt]);

  // Staying on the card with a Codex that needs it: every 15 s.
  const recheckable = needsRecheck(status);
  useEffect(() => {
    if (!chatOpen || !recheckable) return;
    const timer = window.setInterval(() => void attempt("tick"), RECHECK_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [chatOpen, recheckable, attempt]);

  /** "Reintentar" in the engine note. */
  const recheck = useCallback(() => attempt("button"), [attempt]);

  const waiting = status !== null && (status.codex.state === "starting" || status.codex.state === "idle" || status.codex.state === "restarting" || status.codex.login.state === "pending");
  useEffect(() => {
    if (!waiting) return;
    const timer = window.setInterval(() => void refresh(), POLL_MS);
    return () => window.clearInterval(timer);
  }, [waiting, refresh]);

  /** "Conectar Codex con ChatGPT": Codex's own sign-in, opened in the system browser. */
  const login = useCallback(async () => {
    setLoginBusy(true);
    setLoginError("");
    try {
      const response = await fetch("/api/codex/login", { method: "POST", cache: "no-store" });
      const payload = await response.json().catch(() => ({})) as { authUrl?: string; opened?: boolean; error?: string };
      if (!response.ok) throw new Error(payload.error ?? "Dot no pudo iniciar el inicio de sesión de Codex.");
      // Without the desktop shell nobody opened it for us; a plain browser can.
      if (payload.opened !== true && typeof payload.authUrl === "string") window.open(payload.authUrl, "_blank", "noopener");
      await refresh();
    } catch (error) {
      setLoginError(error instanceof Error ? error.message : "Dot no pudo iniciar el inicio de sesión de Codex.");
    } finally {
      if (alive.current) setLoginBusy(false);
    }
  }, [refresh]);

  const cancelLogin = useCallback(async () => {
    try {
      await fetch("/api/codex/login", { method: "DELETE", cache: "no-store" });
    } catch {
      // The sign-in times out by itself.
    }
    await refresh();
  }, [refresh]);

  return { status, refresh, recheck, rechecking, login, cancelLogin, loginBusy, loginError };
}

export type EngineState = ReturnType<typeof useEngine>;
