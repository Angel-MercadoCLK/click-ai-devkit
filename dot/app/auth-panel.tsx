"use client";

import { useEffect, useState } from "react";
import { ChatPanel } from "./chat-panel";

export type AuthStatus = { connected: boolean; email?: string | null; reauthorizationRequired?: boolean; error?: string };

/**
 * Connection state and the sign-in / disconnect actions, shared by the home page and the mascot. Sign-in still
 * navigates this page to the ChatGPT URL; the desktop shell turns that into the system-browser hand-off.
 */
export function useAuthConnection(canonicalOrigin: string) {
  const [status, setStatus] = useState<AuthStatus | null>(null);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");

  async function refresh() {
    try {
      const response = await fetch("/api/auth/status", { cache: "no-store" });
      if (!response.ok) throw new Error();
      setStatus(await response.json() as AuthStatus);
    } catch {
      setStatus({ connected: false, error: "Connection status is unavailable." });
    }
  }

  useEffect(() => {
    if (window.location.origin !== canonicalOrigin) {
      window.location.replace(`${canonicalOrigin}${window.location.pathname}${window.location.search}`);
      return;
    }
    const result = new URLSearchParams(window.location.search).get("auth");
    if (result === "connected") setMessage("ChatGPT plan access is authorized for this device. Load the account’s current model list to start chatting.");
    if (result === "authorization-error") setMessage("Connection was not completed. No ChatGPT access was enabled.");
    window.history.replaceState(null, "", window.location.pathname);
    void refresh();
  }, [canonicalOrigin]);

  async function connect() {
    setBusy(true);
    setMessage("");
    try {
      const response = await fetch("/api/auth/start", { method: "POST" });
      const result = await response.json() as { url?: string };
      const url = result.url ? new URL(result.url) : undefined;
      if (!response.ok || !url || url.origin !== "https://auth.openai.com") throw new Error();
      window.location.assign(url.toString());
    } catch {
      setMessage("Could not start secure ChatGPT authorization. Check the local connection and try again.");
      setBusy(false);
    }
  }

  async function disconnect() {
    setBusy(true);
    try {
      const response = await fetch("/api/auth/disconnect", { method: "POST" });
      if (!response.ok) throw new Error();
      setStatus({ connected: false });
      setMessage("The local connection was removed. You can revoke the app from your ChatGPT account settings.");
    } catch {
      setMessage("The local connection could not be removed.");
    } finally {
      setBusy(false);
    }
  }

  return { status, setStatus, busy, message, connect, disconnect, refresh };
}

export function AuthPanel({ canonicalOrigin }: { canonicalOrigin: string }) {
  const { status, setStatus, busy, message, connect, disconnect } = useAuthConnection(canonicalOrigin);

  return (
    <section className="connection-panel" aria-labelledby="connection-title">
      <div className="connection-copy">
        <p className="panel-label" id="connection-title">Your connection</p>
        <p className="panel-value" role="status" aria-live="polite">
          {status === null ? "Checking this device…" : status.connected ? `Plan access authorized${status.email ? ` for ${status.email}` : ""}` : status.reauthorizationRequired ? "ChatGPT session expired" : "Not connected"}
        </p>
          <p className="panel-note">Click Dot requests ChatGPT plan access for this local installation. Your credentials stay on this device.</p>
      </div>
      {status?.connected ? (
        <button className="action-button secondary" type="button" onClick={disconnect} disabled={busy}>
          {busy ? "Removing…" : "Disconnect this device"}
        </button>
      ) : (
        <button className="action-button" type="button" onClick={connect} disabled={busy || status === null}>
          {busy ? "Opening ChatGPT…" : status?.reauthorizationRequired ? "Reconnect with ChatGPT" : "Continue with ChatGPT"}
        </button>
      )}
      {message && <p className="feedback" role="status">{message}</p>}
      {status?.reauthorizationRequired && <p className="feedback" role="status">The local session expired. This alpha requires you to authorize again; it does not refresh expired tokens automatically.</p>}
      <p className="privacy-note">No API key is needed. ChatGPT will ask you to authorize this app; sign-in alone does not grant model access.</p>
      {status?.connected && <ChatPanel onAuthorizationExpired={() => setStatus({ connected: false, reauthorizationRequired: true })} />}
    </section>
  );
}
