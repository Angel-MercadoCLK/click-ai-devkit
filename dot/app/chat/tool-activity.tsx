"use client";

import { TOOL_NAME_LABELS, TOOL_STATUS_LABELS, type ToolEvent } from "../../lib/tool-events.ts";

/** One line per tool call: what it is, what it is doing, and (once there is any) the tail of its output, collapsed. */
export function ToolActivityList({ tools }: { tools: ToolEvent[] }) {
  return (
    <ul className="tool-activity" aria-label="Actividad de Dot">
      {tools.map((tool) => (
        <li className={`tool-line is-${tool.status}`} key={tool.id} data-tool={tool.name}>
          <div className="tool-line-head">
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
