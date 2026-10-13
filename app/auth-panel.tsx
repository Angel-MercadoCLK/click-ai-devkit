"use client";

import { useEffect, useState } from "react";

type Status = { connected: boolean; email?: string | null; error?: string };

export function AuthPanel({ canonicalOrigin }: { canonicalOrigin: string }) {
  const [status, setStatus] = useState<Status | null>(null);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");

  useEffect(() => {
    if (window.location.origin !== canonicalOrigin) {
      window.location.replace(`${canonicalOrigin}${window.location.pathname}${window.location.search}`);
      return;
    }
    const result = new URLSearchParams(window.location.search).get("auth");
    if (result === "connected") setMessage("ChatGPT plan access is authorized for this device. Chat is not enabled in this alpha.");
    if (result === "authorization-error") setMessage("Connection was not completed. No ChatGPT access was enabled.");
    window.history.replaceState(null, "", window.location.pathname);
    fetch("/api/auth/status", { cache: "no-store" })
      .then(async (response) => {
        if (!response.ok) throw new Error();
        setStatus(await response.json() as Status);
      })
      .catch(() => setStatus({ connected: false, error: "Connection status is unavailable." }));
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

  return (
    <section className="connection-panel" aria-labelledby="connection-title">
      <div className="connection-copy">
        <p className="panel-label" id="connection-title">Your connection</p>
        <p className="panel-value" role="status" aria-live="polite">
          {status === null ? "Checking this device…" : status.connected ? `Plan access authorized${status.email ? ` for ${status.email}` : ""}` : "Not connected"}
        </p>
        <p className="panel-note">Click Dot requests ChatGPT plan access for this local installation. Your credentials stay on this device.</p>
      </div>
      {status?.connected ? (
        <button className="action-button secondary" type="button" onClick={disconnect} disabled={busy}>
          {busy ? "Removing…" : "Disconnect this device"}
        </button>
      ) : (
        <button className="action-button" type="button" onClick={connect} disabled={busy || status === null}>
          {busy ? "Opening ChatGPT…" : "Continue with ChatGPT"}
        </button>
      )}
      {message && <p className="feedback" role="status">{message}</p>}
      <p className="privacy-note">No API key is needed. ChatGPT will ask you to authorize this app; sign-in alone does not grant model access.</p>
    </section>
  );
}
