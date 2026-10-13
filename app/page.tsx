import { AuthPanel } from "./auth-panel";
import { callbackUri } from "../lib/siwc.ts";

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

        <AuthPanel canonicalOrigin={canonicalOrigin} />
        <p className="next-step">This alpha has no chat or agent tools yet. Your account connection is not a model response.</p>
      </section>

      <footer className="page-footer">
        <span>Runs on this machine</span>
        <span>Credentials stay local · No external tools</span>
      </footer>
    </main>
  );
}
