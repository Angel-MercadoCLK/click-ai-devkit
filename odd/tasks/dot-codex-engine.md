# Dot Codex engine

Objective: Dot's brain becomes the user's Codex, driven through `codex app-server` with the user's ChatGPT login. Codex brings Click rules, Engram, shell, code editing, images and thread history. Dot stays the face: island, voice, screen, approvals. Dot adds background Claude Code delegation, the user's real Chrome, and mouse and keyboard.
Branch: `feat/dot-alive` (accumulates with the alive feature; one PR). Test runners: `npm test` and `npm run typecheck` in `dot/`; `go test ./...` if the devkit installer changes.

## Specs

- S1 — Engine. User choice: "Codex como motor (Recomendado)".
  - Dot starts one long-lived `codex app-server` (stdio JSON-RPC) per launch, using the user's installed `codex` and their default `CODEX_HOME`, so the `~/.codex/auth.json` login, the `AGENTS.md` Click rules and the `[mcp_servers]` Engram server all apply.
  - Handshake: `initialize` and `initialized`. Then `account/read`; if not signed in, Dot offers ChatGPT sign-in through `account/login/start`.
  - Supported Codex versions are checked with `codex --version` against a pinned range. An unsupported or missing Codex shows a clear Spanish message, and Dot falls back to its current built-in agent loop.
  - The protocol types are generated from the pinned version (`codex app-server generate-ts`) and committed.
- S2 — Chat on Codex threads.
  - A Dot chat is a Codex thread: `thread/start` with cwd = the chosen project folder (default: the user's home), then `turn/start` with text.
  - Images dropped or pasted go in as `localImage`. Other files dropped go in as their absolute paths in the text, so Codex reads them with its tools.
  - Streaming `item/agentMessage/delta` renders live. Command executions, file changes and MCP tool calls appear as activity rows, reusing the existing tool activity UI.
  - Detener sends `turn/interrupt`.
- S3 — History. User words: "quiero tener historial de chats".
  - When Codex is the engine, the history view lists `thread/list` (newest first, searchable) and continues one with `thread/resume`.
  - The fallback loop keeps the local store.
- S4 — Approvals. Earlier autonomy choice: "Lectura libre, escritura pregunta". Threads start with `approvalPolicy: "untrusted"` and sandbox `"workspace-write"`, the spellings codex 0.163 accepts.
  - Every `item/commandExecution/requestApproval`, `item/fileChange/requestApproval`, `item/permissions/requestApproval`, `item/tool/requestUserInput` and `mcpServer/elicitation/request` becomes an island card through the existing agent broker.
  - The card offers Permitir (`accept`), Siempre en esta sesión (`acceptForSession`, only when offered in `availableDecisions`) and Denegar (`decline`).
  - The JSON-RPC request stays open until the user answers. A 105 s timeout or a dismissed card answers `decline`. Y and N keys keep working.
- S5 — Dot MCP server for Codex.
  - Dot runs a local MCP server (streamable HTTP on 127.0.0.1, bearer token per launch) and passes it to app-server with `-c` config overrides, so no user config file is edited.
  - Tools:
    - `delegate_to_claude {task, project_dir}` returns a task id immediately and starts `claude -p --output-format stream-json --verbose --permission-mode acceptEdits` in the background, with the task on stdin. Never `--bare`.
    - `claude_task_status {task_id}` returns the state and the latest progress lines.
    - `claude_task_result {task_id}` returns the final result, the session id and the cost.
    - `claude_task_cancel {task_id}` ends the task with SIGINT, then a tree kill.
    - `look_at_screen {}` returns the latest locally redacted frame, only while screen sharing is on.
  - Background Claude tasks appear as sessions in the island. Claude's own permission prompts reach the island via the existing Claude Code hooks.
  - User words: "si yo le decia 'sabes que, esta tarea la ejecutas en claude mientras tu haces x' lo haga".
- S6 — The user's Chrome and computer use. User words: "quiero que pueda usar mi computadora, mi chrome, todo".
  - Chrome: Playwright MCP in extension mode (`@playwright/mcp --extension`), added to app-server through `-c` overrides when available. Dot detects whether the "Playwright Extension" is connected and guides the user to install it once.
  - Mouse, keyboard and screen: Windows-MCP (`uvx windows-mcp serve`) with `--control-policy strict`, only input and screen tools enabled (`enabled_tools`), and telemetry off (`ANONYMIZED_TELEMETRY=false`), added the same way when `uv` is present.
  - Every Chrome or computer action follows S4 approvals (`default_tools_approval_mode = "writes"`; reads like screenshots and snapshots are free).
  - While Dot controls the mouse, the island shows a visible "Dot está usando tu equipo" indicator with Detener.
- S7 — Engram. User words: "quiero que pueda consultar engram". It comes through Codex's configured Engram MCP server. Dot surfaces Engram tool calls as activity rows ("Consultando memoria…").
- S8 — Docs: `dot/README.md` and the root `README.md` describe the engine, its requirements (Codex installed and signed in; optional `uv` and the Chrome extension), what leaves the machine, and the approvals.

## Tasks

| ID | Specs | Route | Status | Commit |
|----|-------|-------|--------|--------|
| E1 | S1, S2, S3, S4, S7 | delegated writer: Codex app-server client + broker mapping (parallel with E2) | done | 11ccdbf |
| E2 | S5 | delegated writer: Dot MCP server + background Claude tasks (parallel with E1) | done | 94df7e9 |
| E3 | S6 | after E1/E2: Chrome + Windows-MCP wiring, detection and the control indicator | pending | |
| E4 | S2, S3 | after the alive writer: chat/history UI on Codex threads, file drop | pending | |
| E5 | S8, all | inline: docs, seam check, verifier, native review, delivery | pending | |

## Log

- L1 (2026-10-22) — User message, verbatim in `odd/tasks/dot-alive.md` L1.
- L2 (2026-10-22) — Research:
  - `codex app-server` (stdio JSON-RPC) is the only Codex interface with live approval requests, `thread/list`, `turn/interrupt` and `localImage`. The TS SDK wraps `codex exec` with no approval API, and `codex mcp-server` no longer exists. Current release: rust-v0.163.0.
  - Codex reads `~/.codex/AGENTS.md` and its `config.toml` MCP servers.
  - The MCP tool timeout defaults to 60 s, so delegation must return immediately.
  - Since Chrome 136, `--remote-debugging-port` is ignored on the default profile, which leaves Playwright MCP `--extension` or chrome-devtools-mcp `--autoConnect`.
  - Windows-MCP has full system access; use `strict` with input and screen tools only.
- L3 (2026-10-22) — Engine question answer: "Codex como motor (Recomendado)".
- L4 (2026-10-22) — E2 was done by the writer and committed as `94df7e9`.
  - Checks: 113 related tests pass, as re-run by the parent. The writer reported `npm test` 704/704 at the time.
  - Verified end to end against real codex-cli 0.163.0 with a scratch `CODEX_HOME`: `mcpServerStatus/list` showed the 5 tools, and `mcpServer/tool/call` worked.
  - Override keys: `mcp_servers.dot.url`, `.bearer_token_env_var`, `.enabled_tools`, `.tools.<tool>.approval_mode` (values `auto|prompt|writes|approve`; `prompt` always asks).
  - Deviations:
    - Tool arguments are named `task_id` (S5 updated).
    - On Windows, cancel tree-kills at once, because Node's SIGINT there is an abrupt kill of the direct child only.
    - The result cap keeps the head of the text with a truncated flag.
- L5 (2026-10-22) — E1 was done by the writer.
  - Checks: real `codex app-server` with a scratch HOME (handshake, `account/read`, `thread/start`, `thread/list`, `thread/read`); no model turn, so streaming and approvals are tested with fakes only. The writer reported `npm test` 741/741.
  - Protocol facts for 0.163.0:
    - There is no `jsonrpc` field.
    - `approvalPolicy` takes `untrusted|on-request|granular|never`; sandbox takes kebab-case `workspace-write`.
    - The user-input method is `item/tool/requestUserInput`.
    - `availableDecisions` needs `capabilities.experimentalApi:true`.
    - A thread without turns cannot be resumed.
    - On Windows, `.cmd` shim overrides need TOML literal strings with no spaces or `%&|<>^!`.
  - S4 spellings were updated.
  - Integration still open (next task): wiring in `main.mts`; the chat route switches to Codex; history from `thread/list`; `codex` cards in `agent-card.tsx`; a `codex` tool name in `tool-events.ts`; Codex sign-in; file drop.
