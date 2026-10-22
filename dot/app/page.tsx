import { AuthPanel } from "./auth-panel";
import { CapturePanel } from "./capture-panel";
import { callbackUri } from "../lib/siwc.ts";

// Rendered per request: the canonical origin must come from the running PORT (47321 in the desktop app), not
// from the build environment.
export const dynamic = "force-dynamic";

export default function HomePage() {
  const canonicalOrigin = new URL(callbackUri()).origin;
  return (
    <main className="page-shell">
      <header className="topbar" aria-label="Click Dot local alpha">
        <a className="wordmark" href="#home" aria-label="Click Dot home">
          <span className="wordmark-mark" aria-hidden="true" />
          <span>click dot</span>
        </a>
        <span className="build-label">LOCAL ALPHA</span>
      </header>

      <section className="welcome" id="home" aria-labelledby="welcome-title">
        <div className="status-row" role="status">
          <span className="status-indicator" aria-hidden="true" />
          <span>Local account setup</span>
        </div>

        <h1 id="welcome-title">A quieter place to think.</h1>
        <p className="intro">
          Click Dot runs locally and connects to your own ChatGPT account. Authorize this installation
          in your browser before using plan-backed features.
        </p>

        <CapturePanel />

        <AuthPanel canonicalOrigin={canonicalOrigin} />
        <p className="next-step">Chat uses your account’s available models. Conversations stay in private local storage. In the Dot desktop app on Windows, Dot can also run PowerShell, hand tasks to Claude Code and look at your shared screen, and it asks first before anything that could change your computer.</p>
      </section>

      <footer className="page-footer">
        <span>Runs on this machine</span>
        <span>Credentials stay local · Tools ask before they change anything</span>
      </footer>
    </main>
  );
}
