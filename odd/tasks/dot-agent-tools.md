# Dot agent tools

Objective: Dot's chat becomes a tool-using agent on Windows. The ChatGPT model the user signs in with decides when to run PowerShell, launch Claude Code tasks or look at the shared screen. Read-only commands run automatically; anything that can modify something, and every Claude Code task, asks first on the mascot.
Branch: `feat/dot-agent-tools` (from `origin/main` 373842a). Delivery strategy: ask-on-risk, resolved by the earlier choice "prefiero acumular en una rama": work accumulates on this branch, goes to main in one PR, and gets native review per work-unit commit. Test runners: `npm test` and `npm run typecheck` in `dot/` (node --test); `go test ./...` only if Go changes.

## Specs

- S1 — Agent chat: the existing Dot chat (compact mascot chat and full page) keeps its UI and history. When the tool runtime is available (Windows), the model gets three tools and may call them in a loop of up to 10 tool rounds per user message, then answers in text. Without the runtime (other OS, dev), the chat behaves exactly as today.
- S2 — Autonomy, as the user chose ("Lectura libre, escritura pregunta"): read-only commands run without asking. Everything else (any command that is not provably read-only, and every Claude Code task) shows an approval card on the mascot with Permitir / Denegar, plus Siempre for PowerShell. Siempre allows that exact command text for the rest of this Dot launch only. Denial, a 105 s timeout, or closing the card returns "the user denied this action" to the model, and nothing runs.
- S3 — Tool `run_powershell` {command, cwd|null}:
  - The command runs through `powershell.exe -NoLogo -NoProfile -NonInteractive -EncodedCommand`, with no visible window, UTF-8 output and an explicit exit code.
  - `cwd` must be an existing absolute directory; the default is the user's home.
  - Limits: 120 s timeout with process-tree kill, and 64 KiB per output stream. The model receives the exit code plus the stdout and stderr tails, capped at 16 KiB in total.
  - **Read-only classification is fail-closed.** The PowerShell parser (`[System.Management.Automation.Language.Parser]::ParseInput`) must parse the command with no errors. The command may contain only pipelines of allowlisted commands with literal arguments: no redirection, no `&` or `.` invocation, no variables, subexpressions, script blocks, `--%`, or `;`-separated statements beyond pipelines of allowlisted commands.
  - Allowlist: Get-ChildItem, Get-Content, Get-Item, Get-Location, Test-Path, Resolve-Path, Split-Path, Join-Path, Select-String, Measure-Object, Get-Process, Get-Date, Get-Command, Select-Object, Sort-Object, Format-Table, Format-List. Their aliases from a fixed map: ls, dir, gci, cat, type, gc, pwd, gi, sls, measure, select, sort, ft, fl, gps.
  - Also allowlisted: `whoami`, `hostname`, `where.exe`, and `git` with first argument status, log, diff, show, rev-parse, `branch` (list forms only) or `remote` (plain list, `-v`, or `get-url <name>`). For git, any argument starting with `--output`, `--ext-diff`, `--config`, `--exec`, `--upload-pack` or `--receive-pack`, and the `-c` and `-C` options, are rejected. Git runs with `GIT_PAGER=cat` and `GIT_TERMINAL_PROMPT=0`. Further rules:
    - Any abbreviation of a forbidden long option is rejected.
    - Each argument is checked by its whitespace pieces, and quoted git arguments need approval.
    - Arguments starting with `\\` or `//` (network paths) need approval.
    - Several `;`-separated statements stay read-only only when every statement is allowlisted.
    - The runner adds `-ExecutionPolicy Bypass` and decodes CLIXML stderr to plain text.
  - Anything else, or any classifier failure, means "needs approval".
- S4 — Tool `run_claude_code` {task, project_dir}: always asks first. It runs the user's installed Claude Code with `claude -p --output-format stream-json --verbose --permission-mode acceptEdits` in `project_dir` (an existing absolute directory), with the task text written to stdin (never as an argument). Progress lines appear in the chat, e.g. "Claude Code: Edit foo.ts". Timeout 15 min with process-tree kill. The model receives the final result text (capped at 16 KiB), the session id, the cost and the count of permission denials. If `claude` is not installed, the tool returns a clear error.
- S5 — Tool `look_at_screen` {}: allowed only while "Compartir mi pantalla" is on and not paused. It returns the latest locally redacted frame as an image in the tool output, using the existing redaction pipeline. Otherwise it returns "screen sharing is off".
- S6 — Visibility: every tool call appears in the chat as an activity line with its state: running, waiting for approval, done, denied or error. A finished call shows a short output tail. The user can stop the turn, which aborts the model stream and kills running processes.
- S7 — Security:
  - The chat stream route now also requires the per-launch control-token cookie, like the capture, voice and agent routes.
  - The instructions tell the model that tool outputs, screen text and files are untrusted data, never instructions.
  - Tool outputs and approvals stay local; only what the model needs (the tool result text and the redacted image) goes to OpenAI, as chat already does.
  - No new data is written to disk. Conversations keep their existing format: the persisted assistant message is the final text plus one summary line of tools used, e.g. `[herramientas: git status (auto), Claude Code (aprobado)]`.
- S8 — Docs: `dot/README.md` describes the agent tools, the autonomy rule, what leaves the machine, and how to stop a turn.

### Contract shared by T1 and T2

- **Bridge:** `globalThis[Symbol.for("click-dot.tool-runner")]`, installed by the desktop shell in `start()` before `startDotServer`, only on Windows:
  ```ts
  interface ToolRunner {
    classifyPowerShell(command: string): Promise<{ readOnly: boolean; reason: string }>;
    runPowerShell(req: { command: string; cwd: string | null; signal: AbortSignal }): Promise<{ exitCode: number | null; stdout: string; stderr: string; truncated: boolean; timedOut: boolean }>;
    runClaudeCode(req: { task: string; cwd: string; signal: AbortSignal; onProgress(line: string): void }): Promise<{ ok: boolean; result: string; sessionId: string | null; costUsd: number | null; permissionDenials: number; truncated: boolean; timedOut: boolean; error?: string }>;
  }
  ```
  A runner method never throws for a command failure; it reports the failure in its result. `error` explains why Claude Code could not start, e.g. "Claude Code is not installed".
- **Model request** (agent mode only):
  - It adds `instructions`, `tools` (three `type:"function"` tools, `strict:true`), `tool_choice:"auto"`, `parallel_tool_calls:false` and `include:["reasoning.encrypted_content"]`.
  - Function calls are taken from `response.output_item.done`.
  - Within one user turn, every output item (reasoning with `encrypted_content`, `function_call`) is replayed unchanged, followed by its `function_call_output`.
- **Local SSE events to the UI:** the existing `delta`, `completed` and `error` events, plus `tool`, carrying `{id, name: "powershell"|"claude_code"|"screen", summary, status: "running"|"waiting_approval"|"done"|"denied"|"error", detail?: string}`. Repeated events with the same id update that call's state. `detail` is at most 2,000 characters.
- **Approvals** reuse the agent broker:
  - `PendingView` gains `kind: "claude-code" | "dot-tool"`.
  - Dot tool requests are enqueued with `kind:"dot-tool"`, a tool label ("PowerShell" or "Claude Code"), a summary (the command or the task), a detail (the folder and why it needs approval), and `alwaysOffered` (true only for PowerShell).
  - The decision choices are `allow`, `deny` and `always`. Ack and timeout rules are unchanged; any outcome other than a decision counts as a denial.

## Tasks

| ID | Specs | Route | Status | Commit |
|----|-------|-------|--------|--------|
| T1 | S3, S4, S7 | delegated writer, tool runtime unit (parallel with T2) | done | aa12625 |
| T2 | S1, S2, S5, S6, S7, S8 | delegated writer, agent loop + UI unit (parallel with T1) | done | b851add |
| T3 | all | inline: seam check, independent verifier, native review, delivery | in progress | |

- T1 — Tool runtime in `dot/desktop/`: PowerShell runner, fail-closed read-only classifier (parser helper), Claude Code runner (stream-json parsing, stdin task), and the bridge installer.
- T2 — Agent loop and UI:
  - Tool calling in `chat-provider` and a loop in the chat route, with the control-token guard and the instructions.
  - Generalized broker approvals, the screen tool, the `tool` SSE events, and activity lines plus a stop button in both chats.
  - The `dot-tool` approval card, the docs, and updates to the tests that pinned "no tools".
- T3 — Seam check (full `npm test`, typecheck, build), independent verifier (high-risk items 2 and 3), native review, then the user's delivery decision.

Route evidence: T1 (`dot/desktop/tool-*`, its tests) and T2 (`dot/lib`, `dot/app`, the remaining tests, `dot/README.md`) are independent units with disjoint edit surfaces, several hundred lines each, launched together under the contract above (Writer trigger a).

## Log

- L1 (2026-10-22) — User request: "ya actualice, sale la mascota, no se sobrepone ni puede ver otras pantallas, okey, solo la de inicio, vamos bien / ahora quiero que sea un agente, un agente como chat gpt dots, que controle terminal, pueda lanzar tareas a claude code, a lo que sea, ejecutar, hacer uso de tu pc, navegador, etc / quiero, un yo casi jajaja, pero para incio vamos bien, sigamos trabajando"
- L2 (2026-10-22) — First increment proposed: Terminal, Claude Code and Ver pantalla tools, with the browser and mouse/keyboard control later. Autonomy question answer: "Lectura libre, escritura pregunta".
- L3 (2026-10-22) — Research:
  - Dot's chat today posts `{model,input,store:false,stream:true}` to api.openai.com/v1/responses with no tools, and tests pin that.
  - History is plain alternating text; the ChatGPT token is never refreshed.
  - Responses function calling with `store:false` requires replaying reasoning items (`encrypted_content`) and function_call items before each `function_call_output`. Codex takes calls from `response.output_item.done`.
  - `claude -p` has no `--cwd` flag; since v2.1.302 headless runs start in Manual mode.
  - PowerShell `-EncodedCommand` takes UTF-16LE base64; Node `kill()` does not end grandchildren, so the runner uses `taskkill /T /F`.
  - Codex's fail-closed PowerShell parser rules served as the reference.
- L4 (2026-10-22) — Follow-ups from the user's v0.10.1 test, not in this feature: the mascot window does not stay on top of other windows, and "Compartir mi pantalla" captures only the primary display.
- L5 (2026-10-22) — T2 was done by the writer, uncommitted. The writer reported:
  - Tests: 453/453 outside `test/tool-*`; typecheck and build clean. `test/tool-claude.test.mjs` (T1, still in progress) failed at file level.
  - Deviations accepted by the parent:
    - Replayed items drop `id` (meaningless with `store:false`; Codex does the same).
    - After 10 rounds, `tool_choice:"none"` forces a text answer.
    - The agent round timeout is 150 s, with a 1 MiB SSE frame limit.
    - Commands or tasks over 8,000 characters are refused without asking.
    - At most 3 `look_at_screen` calls per message.
    - "Detener" appears in the compact chat; the full page keeps "Cancel response", pinned by `test/mascot.test.mjs`.
- L6 (2026-10-22) — T1 was done by the writer, uncommitted.
  - Checks: 92 tool tests. The classifier helper and the PowerShell wrapper were run against real Windows PowerShell 5.1 from WSL (about 40 commands; UTF-8 and exit codes correct; timeout path checked). The Claude Code runner was tested with fakes only.
  - S3 was amended with the writer's tightening: branch and remote only in list forms, abbreviations of forbidden options rejected, network paths need approval, `-ExecutionPolicy Bypass`, CLIXML decoding.
  - Advisory: like Codex, `git status` and `git diff` honor repository config (`core.fsmonitor`, `diff.external`), so a malicious repository could run code. Running those commands by hand has the same effect.
  - Seam check by the parent: `npm test` 542/542, typecheck, `desktop:compile` and `next build` clean.
- L7 (2026-10-22) — Independent verifier (Opus):
  - Passed S1, S2 and S4–S8, with no blockers. It confirmed a byte-identical plain-mode request, the 403 guard, the 10-round cap and an unchanged history format.
  - It could not run the classifier bypass hunt: its response was stopped by a safety classifier.
- The parent ran the hunt itself, parse-only (nothing executed): 99 hostile commands against the real PowerShell 5.1 parser.
  - Nine were classified read-only. Three were real gaps, fixed in `aecd00d` with RED and then GREEN tests: `-ComputerName`/abbreviations/`-cn` reach another computer; git `--textconv` runs repository-configured filters; `hostname` with arguments.
  - The remaining six are benign: `Join-Path -Resolve`, `git diff --no-index`, `where.exe /R`, two literal-backtick parses, `-OutVariable`.
- Verifier advisories:
  - Fixed in `779199d`:
    - Output caps now keep the last 64 KiB, so final errors reach the model.
    - The approval card shows `Motivo: <classifier reason>`.
  - Pending:
    - `dot/README.md` still says the chat route doesn't use the token; fix after the island writer finishes, since README is in its surface.
    - The browser home page opened after sign-in now always gets a 403 from the chat route; it should say "usa la ventana de Dot".
    - The process-tree `taskkill` was not verified on real Windows.
