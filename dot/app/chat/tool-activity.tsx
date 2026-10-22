"use client";

import { TOOL_NAME_LABELS, TOOL_STATUS_LABELS, type ToolEvent, type ToolStatus } from "../../lib/tool-events.ts";

/** A small stroke icon per status: a check when done, a clock while waiting or running, a mark for an error or a denial. */
function StatusIcon({ status }: { status: ToolStatus }) {
  return (
    <svg viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" strokeWidth="2.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
      {status === "done" && <path d="m5 12 5 5 9-10" />}
      {(status === "running" || status === "waiting_approval") && (
        <>
          <circle cx="12" cy="12" r="8" />
          <path d="M12 8v4l2.5 2" />
        </>
      )}
      {status === "error" && <path d="M12 7v6M12 17h.01" />}
      {status === "denied" && <path d="M6 6l12 12M18 6 6 18" />}
    </svg>
  );
}

/** One line per tool call: what it is, what it is doing, and (once there is any) the tail of its output, collapsed. */
export function ToolActivityList({ tools }: { tools: ToolEvent[] }) {
  return (
    <ul className="tool-activity" aria-label="Actividad de Dot">
      {tools.map((tool) => (
        <li className={`tool-line is-${tool.status}`} key={tool.id} data-tool={tool.name}>
          <div className="tool-line-head">
            <span className="tool-line-icon"><StatusIcon status={tool.status} /></span>
            <span className="tool-line-name">{TOOL_NAME_LABELS[tool.name]}</span>
            <span className="tool-line-summary" title={tool.summary}>{tool.summary}</span>
            <span className={`tool-chip is-${tool.status}`} role="status">{TOOL_STATUS_LABELS[tool.status]}</span>
          </div>
          {tool.detail && (
            <details className="tool-line-output">
              <summary>Salida</summary>
              <pre>{tool.detail}</pre>
            </details>
          )}
        </li>
      ))}
    </ul>
  );
}
