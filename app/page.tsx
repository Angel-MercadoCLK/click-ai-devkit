export default function HomePage() {
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
          <span>Demo · Not connected</span>
        </div>

        <h1 id="welcome-title">A quieter place to think.</h1>
        <p className="intro">
          Click Dot is starting locally. This preview is not connected to an AI provider, and it
          does not send messages anywhere.
        </p>

        <div className="connection-panel" aria-label="Connection status">
          <div>
            <p className="panel-label">Connection</p>
          <p className="panel-value">No provider connected</p>
          </div>
          <span className="connection-badge">DEMO MODE</span>
        </div>

        <p className="next-step">Chat and provider settings will arrive in the next alpha step.</p>
      </section>

      <footer className="page-footer">
        <span>Runs on this machine</span>
        <span>No model calls · No external tools</span>
      </footer>
    </main>
  );
}
