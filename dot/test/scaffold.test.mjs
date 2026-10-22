import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import test from "node:test";

const root = fileURLToPath(new URL("../", import.meta.url));
const read = (path) => readFileSync(new URL(path, `file://${root}`), "utf8");

test("the agent disables default and parent-callable tools", () => {
  const agent = read("agent/agent.ts");
  assert.match(agent, /defaultTools:\s*false/);
  assert.match(agent, /tool:\s*false/);

  for (const capability of ["tools", "connections", "channels", "subagents", "skills"]) {
    assert.equal(existsSync(new URL(`agent/${capability}`, `file://${root}`)), false);
  }
});

test("the browser app offers honest per-install account setup and real chat without the eve agent runtime", () => {
  const page = read("app/page.tsx");
  const panel = read("app/auth-panel.tsx");
  assert.match(page, /AuthPanel/);
  assert.match(panel, /Continue with ChatGPT/);
  assert.match(panel, /sign-in alone does not grant model access/);
  assert.match(panel, /<ChatPanel/);
  assert.match(panel, /session expired/);
  assert.match(read("app/chat-panel.tsx"), /api\/chat\/stream/);
  assert.match(panel, /api\/auth\/status/);
  assert.doesNotMatch(page + panel, /useEveAgent|\/eve\/v1/);
});

test("development commands disable telemetry and use loopback only", () => {
  const runner = read("scripts/next.mjs");
  assert.match(runner, /NEXT_TELEMETRY_DISABLED\s*=\s*"1"/);
  assert.match(runner, /"127\.0\.0\.1"/);
  assert.match(runner, /process\.once\(signal/);
  assert.match(runner, /next\.kill\(signal\)/);
});
