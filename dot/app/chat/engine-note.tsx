"use client";

import { engineNotice, needsRecheck } from "../../lib/engine-client.ts";
import type { EngineState } from "./use-engine";

/**
 * What the chat card says about Codex when it is not the engine (why, and the one button that fixes it), and, when it is, the
 * Chrome and mouse-and-keyboard chips. The built-in sign-in button is not here: it keeps its own place and label.
 */
export function EngineNote({ engine, signInShown }: { engine: EngineState; signInShown?: boolean }) {
  const { status } = engine;
  if (!status) return null;
  const notice = engineNotice(status);
  const signedOut = status.codex.state === "ready" && status.codex.account?.state === "signedOut";
  const pending = status.codex.login.state === "pending";
  const failed = status.codex.login.state === "failed" ? status.codex.login.message : undefined;
  const tools = status.engine === "codex" ? status.tools : null;
  // A Codex that is missing, unsupported or failed may have been fixed outside Dot (an update): the button looks for it again.
  const recheckable = notice !== null && needsRecheck(status);
  if (!notice && !signedOut && !tools && !failed && !engine.loginError) return null;
  return (
    <div className="engine-note">
      {notice && <p className="bubble-note" role="status">{notice}</p>}
      {recheckable && (
        <button className="compact-link" type="button" onClick={() => void engine.recheck()} disabled={engine.rechecking}>{engine.rechecking ? "Comprobando…" : "Reintentar"}</button>
      )}
      {signedOut && (
        <div className="engine-login">
          <button className="menu-button primary" type="button" onClick={() => void engine.login()} disabled={engine.loginBusy || pending}>
            {pending ? "Esperando tu inicio de sesión…" : engine.loginBusy ? "Abriendo ChatGPT…" : "Conectar Codex con ChatGPT"}
          </button>
          {pending && <button className="compact-link" type="button" onClick={() => void engine.cancelLogin()}>Cancelar</button>}
          {!pending && !signInShown && <p className="bubble-note">Se abre en tu navegador. Dot usa tu cuenta de Codex, nunca guarda tu contraseña.</p>}
        </div>
      )}
      {(failed || engine.loginError) && <p className="compact-notice" role="status">{failed ?? engine.loginError}</p>}
      {tools && (
        <details className="engine-tools">
          <summary>Chrome y equipo</summary>
          <ul>
            <li data-state={tools.chrome.state}>
              <span>{tools.chrome.hint}</span>
              {tools.chrome.link && <a href={tools.chrome.link} target="_blank" rel="noreferrer noopener">Extensión</a>}
            </li>
            <li data-state={tools.computer.state}>
              <span>{tools.computer.hint}</span>
              {tools.computer.link && <a href={tools.computer.link} target="_blank" rel="noreferrer noopener">Instalar</a>}
            </li>
          </ul>
        </details>
      )}
    </div>
  );
}
