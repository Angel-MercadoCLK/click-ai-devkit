import assert from "node:assert/strict";
import { request as httpRequest } from "node:http";
import { test } from "node:test";

const mcp = await import("../desktop/dot-mcp.mts");
const tasksModule = await import("../desktop/claude-tasks.mts");

const REPO = "C:\\work\\repo";
const EXE = "C:\\Users\\angel\\.local\\bin\\claude.exe";
const PNG = Buffer.concat([Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]), Buffer.alloc(32, 7)]);
const JPEG = Buffer.concat([Buffer.from([0xff, 0xd8, 0xff, 0xe0]), Buffer.alloc(32, 9)]);

// Fakes ---------------------------------------------------------------------------------------------------------

function controlled(pid) {
  const listeners = {};
  return {
    child: {
      pid,
      stdin: { write: () => true, end: () => {}, on: () => {} },
      stdout: { on: (event, listener) => { listeners.stdout = listener; } },
      stderr: { on: () => {} },
      on: (event, listener) => { listeners[event] = listener; },
      kill: () => {},
    },
    out: (text) => listeners.stdout?.(Buffer.from(text)),
    close: (code) => listeners.close?.(code),
  };
}

function fakeTimers() {
  return { setTimeout: () => 1, clearTimeout: () => {} };
}

const tick = async () => { for (let i = 0; i < 6; i += 1) await new Promise((resolve) => setImmediate(resolve)); };
const line = (value) => `${JSON.stringify(value)}\n`;

const servers = [];
async function launch({ screen, token } = {}) {
  const claude = [];
  const kills = [];
  let pid = 10;
  const tasks = tasksModule.createClaudeTaskRegistry({
    platform: "win32",
    env: { SystemRoot: "C:\\Windows" },
    stat: async (path) => {
      if (path === REPO) return { isDirectory: () => true };
      throw Object.assign(new Error("ENOENT"), { code: "ENOENT" });
    },
    spawn: (executable, args) => {
      pid += 1;
      const proc = controlled(pid);
      if (/taskkill/i.test(executable)) kills.push(args[1]);
      else claude.push(proc);
      return proc.child;
    },
    whichClaude: async () => EXE,
    timers: fakeTimers(),
  });
  const server = await mcp.startDotMcpServer({ tasks, screen: screen ?? (async () => ({ kind: "off" })), ...(token ? { token } : {}) });
  servers.push(server);
  return { server, tasks, claude, kills, port: Number(new URL(server.url).port) };
}

test.after(async () => { await Promise.all(servers.map((server) => server.close())); });

/** Raw request, so the Host and Origin headers can be forged (fetch would refuse or rewrite them). */
function send({ server, port }, { method = "POST", path = "/mcp", headers = {}, body, auth = true, host }) {
  return new Promise((resolve, reject) => {
    const payload = body === undefined ? undefined : typeof body === "string" ? body : JSON.stringify(body);
    const req = httpRequest({
      host: "127.0.0.1",
      port,
      method,
      path,
      headers: {
        host: host ?? `127.0.0.1:${port}`,
        ...(auth ? { authorization: `Bearer ${server.token}` } : {}),
        ...(payload !== undefined ? { "content-type": "application/json", "content-length": String(Buffer.byteLength(payload)) } : {}),
        ...headers,
      },
    }, (response) => {
      const chunks = [];
      response.on("data", (chunk) => chunks.push(chunk));
      response.on("end", () => {
        const text = Buffer.concat(chunks).toString("utf8");
        let json;
        try { json = text ? JSON.parse(text) : undefined; } catch { json = undefined; }
        resolve({ status: response.statusCode, headers: response.headers, text, json });
      });
    });
    req.on("error", reject);
    if (payload !== undefined) req.write(payload);
    req.end();
  });
}

let nextId = 1;
const rpc = (target, method, params, extra = {}) => send(target, { body: { jsonrpc: "2.0", id: nextId++, method, ...(params === undefined ? {} : { params }), ...extra } });
const callTool = async (target, name, args) => (await rpc(target, "tools/call", { name, ...(args === undefined ? {} : { arguments: args }) })).json.result;
const textOf = (result) => result.content[0].text;

// Authentication and origin -------------------------------------------------------------------------------------

test("it listens on loopback only, on a random port, with a fresh token per launch", async () => {
  const a = await launch();
  const b = await launch();
  assert.match(a.server.url, /^http:\/\/127\.0\.0\.1:\d+\/mcp$/);
  assert.notEqual(a.server.url, b.server.url);
  assert.notEqual(a.server.token, b.server.token);
  assert.ok(a.server.token.length >= 40);
});

test("a request without a token, with a wrong token or with another scheme is refused with 401", async () => {
  const target = await launch();
  const body = { jsonrpc: "2.0", id: 1, method: "ping" };
  for (const headers of [{}, { authorization: "Bearer wrong" }, { authorization: `Basic ${target.server.token}` }, { authorization: `Bearer ${target.server.token}x` }, { authorization: "Bearer " }]) {
    const answer = await send(target, { auth: false, headers, body });
    assert.equal(answer.status, 401, JSON.stringify(headers));
    assert.equal(answer.headers["www-authenticate"], "Bearer");
  }
  assert.equal((await send(target, { body })).status, 200);
});

test("any Origin header is refused with 403 even with a valid token", async () => {
  const target = await launch();
  for (const origin of ["http://evil.example", "null", "http://127.0.0.1"]) {
    const answer = await send(target, { headers: { origin }, body: { jsonrpc: "2.0", id: 1, method: "ping" } });
    assert.equal(answer.status, 403, origin);
  }
});

test("a Host that is not loopback on this port is refused with 403, loopback names are accepted", async () => {
  const target = await launch();
  const body = { jsonrpc: "2.0", id: 1, method: "ping" };
  for (const host of ["evil.example", `evil.example:${target.port}`, `127.0.0.1:${target.port + 1}`, "127.0.0.1", `192.168.1.5:${target.port}`, `127.0.0.1.evil.example:${target.port}`]) {
    assert.equal((await send(target, { host, body })).status, 403, host);
  }
  for (const host of [`127.0.0.1:${target.port}`, `localhost:${target.port}`, `LOCALHOST:${target.port}`]) {
    assert.equal((await send(target, { host, body })).status, 200, host);
  }
});

test("the forbidden checks run before the token check, and the token is never echoed", async () => {
  const target = await launch();
  const answer = await send(target, { auth: false, headers: { origin: "http://evil.example" }, body: { jsonrpc: "2.0", id: 1, method: "ping" } });
  assert.equal(answer.status, 403);
  assert.ok(!answer.text.includes(target.server.token));
});

test("tokenMatches is exact and tolerant of header case only in the scheme", () => {
  assert.equal(mcp.tokenMatches("secret", "Bearer secret"), true);
  assert.equal(mcp.tokenMatches("secret", "bearer secret"), true);
  assert.equal(mcp.tokenMatches("secret", "Bearer Secret"), false);
  assert.equal(mcp.tokenMatches("secret", undefined), false);
  assert.equal(mcp.tokenMatches("", "Bearer "), false);
  assert.equal(mcp.tokenMatches("secret", "Bearer secret extra"), false);
});

test("only POST /mcp with JSON is served", async () => {
  const target = await launch();
  assert.equal((await send(target, { path: "/other", body: { jsonrpc: "2.0", id: 1, method: "ping" } })).status, 404);
  for (const method of ["GET", "DELETE", "PUT"]) {
    const answer = await send(target, { method });
    assert.equal(answer.status, 405, method);
    assert.equal(answer.headers.allow, "POST");
  }
  const wrongType = await send(target, { body: "{}", headers: { "content-type": "text/plain" } });
  assert.equal(wrongType.status, 415);
});

test("a body that is too large, not JSON or not JSON-RPC is rejected", async () => {
  const target = await launch();
  const huge = await send(target, { body: JSON.stringify({ jsonrpc: "2.0", id: 1, method: "ping", params: { pad: "x".repeat(mcp.MAX_BODY_BYTES) } }) });
  assert.equal(huge.status, 413);
  const garbage = await send(target, { body: "{nope" });
  assert.equal(garbage.status, 400);
  assert.equal(garbage.json.error.code, -32700);
  const notRpc = await send(target, { body: { hello: "world" } });
  assert.equal(notRpc.status, 400);
  assert.equal(notRpc.json.error.code, -32600);
  assert.equal((await send(target, { body: [] })).status, 400);
});

test("close stops the server", async () => {
  const target = await launch();
  await target.server.close();
  await assert.rejects(send(target, { body: { jsonrpc: "2.0", id: 1, method: "ping" } }));
});

// Protocol ------------------------------------------------------------------------------------------------------

test("initialize answers with the negotiated version, the tools capability and server info as plain JSON", async () => {
  const target = await launch();
  const answer = await rpc(target, "initialize", { protocolVersion: "2025-03-26", capabilities: {}, clientInfo: { name: "codex", version: "0" } });
  assert.equal(answer.status, 200);
  assert.match(answer.headers["content-type"], /^application\/json/);
  assert.equal(answer.headers["mcp-session-id"], undefined);
  assert.equal(answer.json.jsonrpc, "2.0");
  assert.equal(answer.json.result.protocolVersion, "2025-03-26");
  assert.deepEqual(answer.json.result.capabilities, { tools: { listChanged: false } });
  assert.equal(answer.json.result.serverInfo.name, "click-dot");
  const unknown = await rpc(target, "initialize", { protocolVersion: "1999-01-01" });
  assert.equal(unknown.json.result.protocolVersion, "2025-06-18");
});

test("notifications and client replies get 202 with no body", async () => {
  const target = await launch();
  for (const body of [
    { jsonrpc: "2.0", method: "notifications/initialized" },
    { jsonrpc: "2.0", method: "notifications/cancelled", params: { requestId: 1 } },
    { jsonrpc: "2.0", id: 9, result: {} },
  ]) {
    const answer = await send(target, { body });
    assert.equal(answer.status, 202);
    assert.equal(answer.text, "");
  }
});

test("ping, unknown methods and unknown tools answer with JSON-RPC errors where the spec says so", async () => {
  const target = await launch();
  assert.deepEqual((await rpc(target, "ping")).json.result, {});
  const method = await rpc(target, "resources/list");
  assert.equal(method.json.error.code, -32601);
  const tool = await rpc(target, "tools/call", { name: "rm_rf", arguments: {} });
  assert.equal(tool.json.error.code, -32602);
  const malformed = await rpc(target, "tools/call", { arguments: {} });
  assert.equal(malformed.json.error.code, -32602);
});

test("a batch is answered with an array and skips notifications", async () => {
  const target = await launch();
  const answer = await send(target, { body: [{ jsonrpc: "2.0", id: 1, method: "ping" }, { jsonrpc: "2.0", method: "notifications/initialized" }, { jsonrpc: "2.0", id: 2, method: "ping" }] });
  assert.equal(answer.status, 200);
  assert.deepEqual(answer.json.map((entry) => entry.id), [1, 2]);
});

test("tools/list returns the five tools with strict schemas and honest annotations", async () => {
  const target = await launch();
  const { tools } = (await rpc(target, "tools/list")).json.result;
  assert.deepEqual(tools.map((tool) => tool.name), ["delegate_to_claude", "claude_task_status", "claude_task_result", "claude_task_cancel", "look_at_screen"]);
  for (const tool of tools) {
    assert.equal(tool.inputSchema.type, "object", tool.name);
    assert.equal(tool.inputSchema.additionalProperties, false, tool.name);
    assert.ok(tool.description.length > 20);
  }
  const byName = Object.fromEntries(tools.map((tool) => [tool.name, tool]));
  assert.deepEqual(byName.delegate_to_claude.inputSchema.required, ["task", "project_dir"]);
  assert.deepEqual(byName.claude_task_status.inputSchema.required, ["task_id"]);
  assert.deepEqual(byName.look_at_screen.inputSchema.properties, {});
  assert.equal(byName.claude_task_status.annotations.readOnlyHint, true);
  assert.equal(byName.look_at_screen.annotations.readOnlyHint, true);
  assert.equal(byName.delegate_to_claude.annotations.readOnlyHint, false);
  assert.equal(byName.claude_task_cancel.annotations.destructiveHint, true);
});

// Tools ---------------------------------------------------------------------------------------------------------

test("delegate_to_claude returns a task id at once and the run starts in the background", async () => {
  const target = await launch();
  const result = await callTool(target, "delegate_to_claude", { task: "Fix the test", project_dir: REPO });
  assert.equal(result.isError, undefined);
  const { task_id } = JSON.parse(textOf(result));
  assert.match(task_id, tasksModule.TASK_ID_PATTERN);
  await tick();
  assert.equal(target.claude.length, 1);
  assert.equal(target.tasks.status(task_id).state, "running");
});

test("status follows the task: running with progress, then done; result is an error until then", async () => {
  const target = await launch();
  const { task_id } = JSON.parse(textOf(await callTool(target, "delegate_to_claude", { task: "Fix the test", project_dir: REPO })));
  await tick();
  target.claude[0].out(line({ type: "assistant", message: { content: [{ type: "tool_use", name: "Edit", input: { file_path: "C:\\work\\repo\\a.ts" } }] } }));
  const running = JSON.parse(textOf(await callTool(target, "claude_task_status", { task_id })));
  assert.equal(running.state, "running");
  assert.deepEqual(running.progress, ["Claude Code: Edit a.ts"]);
  assert.equal(typeof running.elapsed_seconds, "number");
  const early = await callTool(target, "claude_task_result", { task_id });
  assert.equal(early.isError, true);
  assert.match(textOf(early), /still running/i);

  target.claude[0].out(line({ type: "result", subtype: "success", is_error: false, result: "Fixed.", session_id: "s-1", total_cost_usd: 0.1, permission_denials: [{}] }));
  target.claude[0].close(0);
  await tick();
  assert.equal(JSON.parse(textOf(await callTool(target, "claude_task_status", { task_id }))).state, "done");
  const done = await callTool(target, "claude_task_result", { task_id });
  assert.equal(done.isError, undefined);
  assert.deepEqual(JSON.parse(textOf(done)), { task_id, state: "done", ok: true, result: "Fixed.", session_id: "s-1", cost_usd: 0.1, permission_denials: 1 });
});

test("a failed task reports its state and reason through status and result", async () => {
  const target = await launch();
  const { task_id } = JSON.parse(textOf(await callTool(target, "delegate_to_claude", { task: "x", project_dir: REPO })));
  await tick();
  target.claude[0].close(2);
  await tick();
  const result = JSON.parse(textOf(await callTool(target, "claude_task_result", { task_id })));
  assert.equal(result.state, "failed");
  assert.equal(result.ok, false);
  assert.match(result.error, /without a result/i);
});

test("claude_task_cancel ends a running task and reports a finished one as not cancelling", async () => {
  const target = await launch();
  const { task_id } = JSON.parse(textOf(await callTool(target, "delegate_to_claude", { task: "x", project_dir: REPO })));
  await tick();
  const first = JSON.parse(textOf(await callTool(target, "claude_task_cancel", { task_id })));
  assert.deepEqual(first, { task_id, cancelling: true, state: "running" });
  await tick();
  assert.equal(target.kills.length, 1, "the process tree was killed");
  assert.equal(JSON.parse(textOf(await callTool(target, "claude_task_status", { task_id }))).state, "cancelled");
  assert.deepEqual(JSON.parse(textOf(await callTool(target, "claude_task_cancel", { task_id }))), { task_id, cancelling: false, state: "cancelled" });
});

test("delegating with a bad folder, an unknown id or invalid arguments is a tool error the model can read", async () => {
  const target = await launch();
  const cases = [
    ["delegate_to_claude", { task: "x", project_dir: "C:\\missing" }, /project_dir/],
    ["delegate_to_claude", { task: "x", project_dir: "relative" }, /absolute/],
    ["delegate_to_claude", { task: "x" }, /"project_dir" is required/],
    ["delegate_to_claude", { task: "x", project_dir: REPO, extra: 1 }, /Unknown argument "extra"/],
    ["delegate_to_claude", { task: 5, project_dir: REPO }, /must be a string/],
    ["delegate_to_claude", { task: "x".repeat(tasksModule.MAX_TASK_CHARS + 1), project_dir: REPO }, /too long/],
    ["claude_task_status", { task_id: "task_000000000000" }, /no task/i],
    ["claude_task_status", { task_id: "../etc" }, /not valid/],
    ["claude_task_status", {}, /required/],
    ["claude_task_result", { task_id: "task_ffffffffffff" }, /no task/i],
    ["claude_task_cancel", { task_id: "task_ffffffffffff" }, /no task/i],
    ["look_at_screen", { anything: true }, /Unknown argument/],
  ];
  for (const [name, args, pattern] of cases) {
    const result = await callTool(target, name, args);
    assert.equal(result.isError, true, `${name} ${JSON.stringify(args).slice(0, 60)}`);
    assert.match(textOf(result), pattern);
  }
  assert.equal(target.claude.length, 0);
});

test("the concurrency cap and a missing claude surface through the tools", async () => {
  const target = await launch();
  for (let i = 0; i < 3; i += 1) assert.equal((await callTool(target, "delegate_to_claude", { task: "x", project_dir: REPO })).isError, undefined);
  const refused = await callTool(target, "delegate_to_claude", { task: "x", project_dir: REPO });
  assert.equal(refused.isError, true);
  assert.match(textOf(refused), /too many/i);
});

// Screen --------------------------------------------------------------------------------------------------------

test("look_at_screen says screen sharing is off when capture is off or paused", async () => {
  const target = await launch({ screen: async () => ({ kind: "off" }) });
  const result = await callTool(target, "look_at_screen", {});
  assert.equal(result.isError, undefined);
  assert.equal(result.content.length, 1);
  assert.equal(result.content[0].type, "text");
  assert.match(result.content[0].text, /^Screen sharing is off/);
});

test("look_at_screen returns the redacted frame as MCP image content", async () => {
  const target = await launch({ screen: async () => ({ kind: "image", image: PNG, mime: "image/png" }) });
  const result = await callTool(target, "look_at_screen");
  assert.deepEqual(result.content, [{ type: "image", data: PNG.toString("base64"), mimeType: "image/png" }]);
});

test("look_at_screen reports a frame that is not ready, or a source that throws, without raw errors", async () => {
  const notReady = await launch({ screen: async () => ({ kind: "unavailable", message: "Dot has no recent screen image yet." }) });
  const first = await callTool(notReady, "look_at_screen", {});
  assert.equal(first.isError, true);
  assert.match(textOf(first), /no recent screen image/);
  const broken = await launch({ screen: async () => { throw new Error("C:\\secret\\path exploded"); } });
  const second = await callTool(broken, "look_at_screen", {});
  assert.equal(second.isError, true);
  assert.ok(!textOf(second).includes("secret"));
});

// The capture controller as the screen source ---------------------------------------------------------------------

function controllerTarget(controller) {
  return { [Symbol.for("click-dot.capture-controller")]: controller };
}

test("the capture-controller source is off without a controller, while capture is off, and while paused", async () => {
  let prepared = 0;
  const prepareScreen = async () => { prepared += 1; return { image: PNG, mime: "image/png", hiddenRegions: 0, capturedAt: 1 }; };
  assert.deepEqual(await mcp.captureControllerScreenSource({})(), { kind: "off" });
  assert.deepEqual(await mcp.captureControllerScreenSource(controllerTarget({ status: () => ({ enabled: false, paused: false }), prepareScreen }))(), { kind: "off" });
  assert.deepEqual(await mcp.captureControllerScreenSource(controllerTarget({ status: () => ({ enabled: true, paused: true }), prepareScreen }))(), { kind: "off" });
  assert.deepEqual(await mcp.captureControllerScreenSource(controllerTarget({ status: () => ({ enabled: true, paused: false }) }))(), { kind: "off" });
  assert.equal(prepared, 0, "no frame is requested while sharing is off");
});

test("the capture-controller source returns the prepared image while capture is on", async () => {
  const source = mcp.captureControllerScreenSource(controllerTarget({
    status: () => ({ enabled: true, paused: false }),
    prepareScreen: async () => ({ image: JPEG, mime: "image/jpeg", hiddenRegions: 2, capturedAt: 1 }),
  }));
  const screen = await source();
  assert.equal(screen.kind, "image");
  assert.equal(screen.mime, "image/jpeg");
  assert.deepEqual(Buffer.from(screen.image), JPEG);
});

test("the capture-controller source maps preparation failures to fixed wording and rejects unusable images", async () => {
  const failing = (code) => mcp.captureControllerScreenSource(controllerTarget({
    status: () => ({ enabled: true, paused: false }),
    prepareScreen: async () => { throw Object.assign(new Error("C:\\raw\\detail"), { code }); },
  }));
  assert.deepEqual(await failing("capture-stopped")(), { kind: "off" });
  const waiting = await failing("no-frame")();
  assert.equal(waiting.kind, "unavailable");
  assert.match(waiting.message, /try again/i);
  const other = await failing("ocr-failed")();
  assert.equal(other.kind, "unavailable");
  assert.ok(!other.message.includes("raw"));

  const returning = (value) => mcp.captureControllerScreenSource(controllerTarget({ status: () => ({ enabled: true, paused: false }), prepareScreen: async () => value }))();
  for (const bad of [undefined, null, {}, { image: PNG, mime: "image/gif" }, { image: JPEG, mime: "image/png" }, { image: Buffer.alloc(8), mime: "image/png" }, { image: Buffer.alloc(mcp.SCREEN_IMAGE_MAX_BYTES + 1), mime: "image/png" }]) {
    assert.equal((await returning(bad)).kind, "unavailable");
  }
});

// Codex configuration -------------------------------------------------------------------------------------------

test("codexConfigOverrides returns the exact -c values: url, token env var, tool list and per-tool approval", async () => {
  const server = { url: "http://127.0.0.1:5123/mcp", token: "super-secret-token" };
  const overrides = mcp.codexConfigOverrides(server);
  assert.deepEqual(overrides, [
    'mcp_servers.dot.url="http://127.0.0.1:5123/mcp"',
    'mcp_servers.dot.bearer_token_env_var="DOT_MCP_TOKEN"',
    'mcp_servers.dot.enabled_tools=["delegate_to_claude","claude_task_status","claude_task_result","claude_task_cancel","look_at_screen"]',
    'mcp_servers.dot.tools.delegate_to_claude.approval_mode="prompt"',
    'mcp_servers.dot.tools.claude_task_status.approval_mode="auto"',
    'mcp_servers.dot.tools.claude_task_result.approval_mode="auto"',
    'mcp_servers.dot.tools.claude_task_cancel.approval_mode="prompt"',
    'mcp_servers.dot.tools.look_at_screen.approval_mode="auto"',
  ]);
  assert.ok(!overrides.join("\n").includes(server.token), "the token travels only in the environment");
  assert.deepEqual(mcp.codexMcpEnv(server), { DOT_MCP_TOKEN: "super-secret-token" });
  assert.deepEqual(mcp.codexConfigArgs(server).filter((_, index) => index % 2 === 0), Array(overrides.length).fill("-c"));
});

test("codexConfigOverrides honors a custom env var name, server name and read mode, and refuses unsafe names", () => {
  const server = { url: "http://127.0.0.1:1/mcp", token: "t" };
  const overrides = mcp.codexConfigOverrides(server, { envVarName: "OTHER_TOKEN", serverName: "dot-2", readApprovalMode: "approve" });
  assert.ok(overrides.includes('mcp_servers.dot-2.bearer_token_env_var="OTHER_TOKEN"'));
  assert.ok(overrides.includes('mcp_servers.dot-2.tools.look_at_screen.approval_mode="approve"'));
  assert.ok(overrides.includes('mcp_servers.dot-2.tools.delegate_to_claude.approval_mode="prompt"'));
  assert.deepEqual(mcp.codexMcpEnv(server, { envVarName: "OTHER_TOKEN" }), { OTHER_TOKEN: "t" });
  assert.throws(() => mcp.codexConfigOverrides(server, { serverName: 'dot"]\n[evil' }));
  assert.throws(() => mcp.codexConfigOverrides(server, { envVarName: "BAD NAME" }));
});
