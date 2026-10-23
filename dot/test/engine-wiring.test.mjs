import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { test } from "node:test";

const read = (path) => readFile(new URL(`../${path}`, import.meta.url), "utf8");

test("the shell wires Dot's MCP server, the external servers and the engine, and shuts them down on quit", async () => {
  const main = await read("desktop/main.mts");
  for (const needle of ["createClaudeTaskRegistry()", "installClaudeTasks(globalThis", "startDotMcpServer(", "detectExternalMcp()", "installExternalMcp(globalThis", "createCodexAppServer(", "codexMcpEnv(dotMcp)", "installCodexEngine(globalThis", "installSystemBrowser(globalThis", "createAttachmentStore()", "installAttachments(globalThis"]) {
    assert.ok(main.includes(needle), needle);
  }
  assert.match(main, /ANONYMIZED_TELEMETRY: "false"/);
  assert.match(main, /readApprovalMode: "approve"/);
  assert.match(main, /\.\.\.external\.overrides/);
  // Quit: tasks stopped, copies deleted, Codex and the MCP server stopped before the process leaves.
  assert.match(main, /claudeTasks\?\.dispose\(\)/);
  assert.match(main, /attachments\?\.dispose\(\)/);
  assert.match(main, /codexServer\?\.stop\(\)/);
  assert.match(main, /dotMcp\?\.close\(\)/);
  assert.match(main, /event\.preventDefault\(\)/);
});

test("a failed Codex setup is never fatal: it is logged and the built-in assistant stays", async () => {
  const main = await read("desktop/main.mts");
  const body = main.slice(main.indexOf("async function startCodexEngine"), main.indexOf("async function start()"));
  assert.match(body, /try \{/);
  assert.match(body, /catch \(error\)[\s\S]*built-in assistant/);
});

test("the island keeps Dot's own sign-in label for the packaged smoke and adds Codex's own beside it", async () => {
  const mascot = await read("app/mascot/mascot.tsx");
  const note = await read("app/chat/engine-note.tsx");
  const smoke = await readFile(new URL("../../scripts/dot-package-smoke.ps1", import.meta.url), "utf8");
  assert.match(smoke, /\$connectLabel = 'Conectar con ChatGPT'/);
  assert.match(smoke, /\$messageLabel = 'Mensaje'/);
  assert.ok(mascot.includes("Conectar con ChatGPT"), "the connect button the smoke waits for");
  assert.ok(note.includes("Conectar Codex con ChatGPT"));
  assert.ok(!"Conectar Codex con ChatGPT".includes("Conectar con ChatGPT"), "the two labels never match each other's check");
  assert.match(mascot, /chatGate\(/);
});

test("the island shows 'Dot está usando tu equipo' with a Detener that stops Codex, and the Actividad lists the Codex chats and background tasks", async () => {
  const mascot = await read("app/mascot/mascot.tsx");
  const hook = await read("app/mascot/use-agent.ts");
  assert.match(mascot, /COMPUTER_USE_TEXT/);
  assert.match(mascot, /agent\.stopComputer\(\)/);
  assert.match(mascot, /activityList\(/);
  assert.match(mascot, /codexTurns/);
  assert.match(hook, /\/api\/agent\/activity/);
  assert.match(hook, /\/api\/codex\/stop/);
});

test("the chat history on Codex lists and opens threads, searches, and archives instead of deleting", async () => {
  const panel = await read("app/chat-panel.tsx");
  const compact = await read("app/chat/compact-chat.tsx");
  assert.match(panel, /\/api\/codex\/threads/);
  assert.match(panel, /method: "DELETE"/);
  assert.match(compact, /Archivar/);
  assert.match(compact, /history-search/);
  // The built-in history is untouched.
  assert.match(panel, /\/api\/chat\/history/);
});

test("dropping and pasting files is wired on the composer and goes through the guarded route", async () => {
  const parts = await read("app/chat/parts.tsx");
  const panel = await read("app/chat-panel.tsx");
  assert.match(parts, /onDrop=/);
  assert.match(parts, /onPaste=/);
  assert.match(parts, /attachment-chip/);
  assert.match(panel, /\/api\/attachments/);
  assert.match(panel, /x-file-name/);
});
