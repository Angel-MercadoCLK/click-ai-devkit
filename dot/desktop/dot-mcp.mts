import { createHash, randomBytes, timingSafeEqual } from "node:crypto";
import { createServer, type IncomingMessage, type Server, type ServerResponse } from "node:http";
import type { AddressInfo } from "node:net";
import { CAPTURE_CONTROLLER_KEY } from "./capture.mts";
import { TASK_ID_PATTERN, MAX_TASK_CHARS, type ClaudeTaskRegistry } from "./claude-tasks.mts";
import { tomlLiteral, tomlLiteralArray } from "./external-mcp-toml.mts";

// Dot's MCP server for Codex (S5): MCP streamable HTTP on 127.0.0.1, one bearer token per launch. Codex reaches it
// through `-c mcp_servers.dot.*` overrides, so no user config file is edited. Every response is a single
// `application/json` body (the spec lets a server answer a POST that way); there is no SSE stream, so GET is refused
// with 405, which the spec also allows. The server is stateless: it hands out no session id.

export const MCP_PATH = "/mcp";
export const MCP_SERVER_NAME = "dot";
export const MCP_TOKEN_ENV_VAR = "DOT_MCP_TOKEN";
export const SUPPORTED_PROTOCOL_VERSIONS = ["2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"] as const;
export const MAX_BODY_BYTES = 1024 * 1024;
export const SCREEN_IMAGE_MAX_BYTES = 4 * 1024 * 1024;
export const SCREEN_OFF_TEXT = "Screen sharing is off. Ask the user to turn it on in Dot, then try again.";

// Screen ----------------------------------------------------------------------------------------------------------

export type ScreenOutcome =
  | { kind: "image"; image: Uint8Array; mime: "image/png" | "image/jpeg" }
  | { kind: "off" }
  | { kind: "unavailable"; message: string };

export type ScreenSource = () => Promise<ScreenOutcome>;

const PNG_MAGIC = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a];
const hasPrefix = (bytes: Uint8Array, prefix: number[]) => bytes.length > prefix.length && prefix.every((value, index) => bytes[index] === value);

const FRAME_WAIT_TEXT = "Dot has no recent screen image yet. Try again in a few seconds.";
const FRAME_FAILED_TEXT = "Dot could not prepare the screen image.";

/**
 * The screen as the capture controller publishes it (desktop/capture.mts): only while capture is on and not paused, and
 * only the image `prepareScreen` returns, already redacted. No raw frame is reachable from here, and the controller's own
 * error text never reaches Codex.
 */
export function captureControllerScreenSource(target: object = globalThis): ScreenSource {
  return async () => {
    const controller = Reflect.get(target, Symbol.for(CAPTURE_CONTROLLER_KEY)) as
      | { status?: () => { enabled: boolean; paused: boolean }; prepareScreen?: () => Promise<unknown> }
      | undefined;
    if (!controller || typeof controller.status !== "function" || typeof controller.prepareScreen !== "function") return { kind: "off" };
    let status: { enabled: boolean; paused: boolean };
    try {
      status = controller.status();
    } catch {
      return { kind: "off" };
    }
    if (!status.enabled || status.paused) return { kind: "off" };
    let prepared: unknown;
    try {
      prepared = await controller.prepareScreen();
    } catch (error) {
      const code = (error as { code?: unknown } | null)?.code;
      if (code === "capture-stopped") return { kind: "off" };
      return { kind: "unavailable", message: code === "no-frame" || code === "stale-frame" ? FRAME_WAIT_TEXT : FRAME_FAILED_TEXT };
    }
    if (!prepared || typeof prepared !== "object") return { kind: "unavailable", message: FRAME_FAILED_TEXT };
    const { image, mime } = prepared as { image?: unknown; mime?: unknown };
    if (!ArrayBuffer.isView(image) || image.byteLength < 16 || image.byteLength > SCREEN_IMAGE_MAX_BYTES) return { kind: "unavailable", message: FRAME_FAILED_TEXT };
    const bytes = new Uint8Array(image.buffer, image.byteOffset, image.byteLength);
    const valid = mime === "image/png" ? hasPrefix(bytes, PNG_MAGIC) : mime === "image/jpeg" ? hasPrefix(bytes, [0xff, 0xd8, 0xff]) : false;
    if (!valid) return { kind: "unavailable", message: FRAME_FAILED_TEXT };
    return { kind: "image", image: bytes, mime: mime as "image/png" | "image/jpeg" };
  };
}

// Tools -----------------------------------------------------------------------------------------------------------

type JsonSchema = {
  type: "object";
  properties: Record<string, { type: "string"; description: string; maxLength?: number; pattern?: string }>;
  required: string[];
  additionalProperties: false;
};

type ToolDefinition = {
  name: string;
  title: string;
  description: string;
  inputSchema: JsonSchema;
  annotations: { readOnlyHint: boolean; destructiveHint: boolean; idempotentHint: boolean; openWorldHint: boolean };
  /** How Codex asks the user before calling the tool: `prompt` always asks, `auto` follows Codex's own policy. */
  approval: "prompt" | "auto";
};

const taskIdSchema: JsonSchema = {
  type: "object",
  properties: { task_id: { type: "string", description: "The id returned by delegate_to_claude.", pattern: TASK_ID_PATTERN.source } },
  required: ["task_id"],
  additionalProperties: false,
};

export const TOOLS: readonly ToolDefinition[] = [
  {
    name: "delegate_to_claude",
    title: "Delegate a task to Claude Code",
    description:
      "Starts Claude Code in the background on a task in a project folder and returns a task_id at once. Claude edits files in that folder with accept-edits permissions. Use claude_task_status to follow it and claude_task_result for the outcome.",
    inputSchema: {
      type: "object",
      properties: {
        task: { type: "string", description: "What Claude Code should do, written as a complete self-contained brief.", maxLength: MAX_TASK_CHARS },
        project_dir: { type: "string", description: "Absolute path of an existing project folder to run in." },
      },
      required: ["task", "project_dir"],
      additionalProperties: false,
    },
    annotations: { readOnlyHint: false, destructiveHint: false, idempotentHint: false, openWorldHint: true },
    approval: "prompt",
  },
  {
    name: "claude_task_status",
    title: "Claude Code task status",
    description: "Returns the state of a background Claude Code task (running, done, failed, cancelled or timed_out), the elapsed time and its latest progress lines.",
    inputSchema: taskIdSchema,
    annotations: { readOnlyHint: true, destructiveHint: false, idempotentHint: true, openWorldHint: false },
    approval: "auto",
  },
  {
    name: "claude_task_result",
    title: "Claude Code task result",
    description: "Returns the final answer of a finished Claude Code task with its session id, cost and the number of permission denials. It is an error while the task is still running.",
    inputSchema: taskIdSchema,
    annotations: { readOnlyHint: true, destructiveHint: false, idempotentHint: true, openWorldHint: false },
    approval: "auto",
  },
  {
    name: "claude_task_cancel",
    title: "Cancel a Claude Code task",
    description: "Stops a running Claude Code task: it is interrupted first and its whole process tree is killed if it does not stop soon.",
    inputSchema: taskIdSchema,
    annotations: { readOnlyHint: false, destructiveHint: true, idempotentHint: true, openWorldHint: false },
    approval: "prompt",
  },
  {
    name: "look_at_screen",
    title: "Look at the user's screen",
    description: "Returns the latest image of the user's screen, with personal data already hidden. Only works while the user has screen sharing on in Dot.",
    inputSchema: { type: "object", properties: {}, required: [], additionalProperties: false },
    annotations: { readOnlyHint: true, destructiveHint: false, idempotentHint: false, openWorldHint: false },
    approval: "auto",
  },
];

type ToolContent = { type: "text"; text: string } | { type: "image"; data: string; mimeType: string };
type ToolResult = { content: ToolContent[]; isError?: boolean };

const textResult = (value: unknown, isError = false): ToolResult => ({
  content: [{ type: "text", text: typeof value === "string" ? value : JSON.stringify(value) }],
  ...(isError ? { isError: true } : {}),
});
const errorResult = (message: string) => textResult(message, true);

/** Strict argument check against the tool's schema: required strings only, nothing extra. */
function checkArguments(tool: ToolDefinition, args: unknown): { ok: true; value: Record<string, string> } | { ok: false; error: string } {
  if (args === undefined || args === null) args = {};
  if (typeof args !== "object" || Array.isArray(args)) return { ok: false, error: "The arguments must be an object." };
  const given = args as Record<string, unknown>;
  const known = tool.inputSchema.properties;
  for (const key of Object.keys(given)) if (!Object.hasOwn(known, key)) return { ok: false, error: `Unknown argument "${key}".` };
  const value: Record<string, string> = {};
  for (const [key, schema] of Object.entries(known)) {
    const field = given[key];
    if (field === undefined) {
      if (tool.inputSchema.required.includes(key)) return { ok: false, error: `The argument "${key}" is required.` };
      continue;
    }
    if (typeof field !== "string") return { ok: false, error: `The argument "${key}" must be a string.` };
    if (schema.maxLength !== undefined && field.length > schema.maxLength) return { ok: false, error: `The argument "${key}" is too long.` };
    if (schema.pattern !== undefined && !new RegExp(schema.pattern).test(field)) return { ok: false, error: `The argument "${key}" is not valid.` };
    value[key] = field;
  }
  return { ok: true, value };
}

const seconds = (ms: number) => Math.round(ms / 1000);
const UNKNOWN_TASK = "There is no task with that task_id.";

async function callTool(tool: ToolDefinition, args: Record<string, string>, deps: { tasks: ClaudeTaskRegistry; screen: ScreenSource }): Promise<ToolResult> {
  switch (tool.name) {
    case "delegate_to_claude": {
      const started = await deps.tasks.start({ task: args.task, projectDir: args.project_dir });
      return started.ok ? textResult({ task_id: started.taskId }) : errorResult(started.error);
    }
    case "claude_task_status": {
      const status = deps.tasks.status(args.task_id!);
      if (!status) return errorResult(UNKNOWN_TASK);
      return textResult({
        task_id: status.taskId,
        state: status.state,
        elapsed_seconds: seconds(status.elapsedMs),
        project_dir: status.projectDir,
        progress: status.progress,
        ...(status.error ? { error: status.error } : {}),
      });
    }
    case "claude_task_result": {
      const result = deps.tasks.result(args.task_id!);
      if (result === undefined) return errorResult(UNKNOWN_TASK);
      if (result === "running") return errorResult("The task is still running. Check claude_task_status and ask again when it is done.");
      return textResult({
        task_id: result.taskId,
        state: result.state,
        ok: result.ok,
        result: result.result,
        ...(result.truncated ? { truncated: true } : {}),
        session_id: result.sessionId,
        cost_usd: result.costUsd,
        permission_denials: result.permissionDenials,
        ...(result.error ? { error: result.error } : {}),
      });
    }
    case "claude_task_cancel": {
      const cancelled = deps.tasks.cancel(args.task_id!);
      if (!cancelled) return errorResult(UNKNOWN_TASK);
      return textResult({ task_id: args.task_id, cancelling: cancelled.stopping, state: cancelled.state });
    }
    case "look_at_screen": {
      let screen: ScreenOutcome;
      try {
        screen = await deps.screen();
      } catch {
        return errorResult(FRAME_FAILED_TEXT);
      }
      if (screen.kind === "off") return textResult(SCREEN_OFF_TEXT);
      if (screen.kind === "unavailable") return errorResult(screen.message);
      return { content: [{ type: "image", data: Buffer.from(screen.image).toString("base64"), mimeType: screen.mime }] };
    }
    default:
      return errorResult("Unknown tool.");
  }
}

// JSON-RPC --------------------------------------------------------------------------------------------------------

type RpcId = string | number | null;
type RpcResponse = { jsonrpc: "2.0"; id: RpcId; result: unknown } | { jsonrpc: "2.0"; id: RpcId; error: { code: number; message: string } };

const rpcError = (id: RpcId, code: number, message: string): RpcResponse => ({ jsonrpc: "2.0", id, error: { code, message } });
const isRecord = (value: unknown): value is Record<string, unknown> => !!value && typeof value === "object" && !Array.isArray(value);
const isRpcId = (value: unknown): value is string | number => typeof value === "string" || (typeof value === "number" && Number.isFinite(value));

const INSTRUCTIONS =
  "Dot's tools. delegate_to_claude runs a long task in Claude Code in the background and returns at once; poll claude_task_status, then read claude_task_result. look_at_screen shows the user's screen only while they have sharing on.";

/** Returns the response for one JSON-RPC message, or undefined when it needs none (a notification or a client reply). */
async function handleMessage(message: unknown, deps: { tasks: ClaudeTaskRegistry; screen: ScreenSource }): Promise<RpcResponse | undefined> {
  if (!isRecord(message) || message.jsonrpc !== "2.0") return rpcError(null, -32600, "Invalid request.");
  const { id, method, params } = message;
  if (typeof method !== "string") {
    // A response to something this server never sent: nothing to answer.
    return "result" in message || "error" in message ? undefined : rpcError(isRpcId(id) ? id : null, -32600, "Invalid request.");
  }
  if (id === undefined) return undefined; // Notification: notifications/initialized, notifications/cancelled, ...
  if (!isRpcId(id)) return rpcError(null, -32600, "Invalid request.");

  switch (method) {
    case "initialize": {
      const requested = isRecord(params) ? params.protocolVersion : undefined;
      const protocolVersion = SUPPORTED_PROTOCOL_VERSIONS.find((version) => version === requested) ?? SUPPORTED_PROTOCOL_VERSIONS[1];
      return {
        jsonrpc: "2.0",
        id,
        result: {
          protocolVersion,
          capabilities: { tools: { listChanged: false } },
          serverInfo: { name: "click-dot", title: "Click Dot", version: "0.1.0" },
          instructions: INSTRUCTIONS,
        },
      };
    }
    case "ping":
      return { jsonrpc: "2.0", id, result: {} };
    case "tools/list":
      return {
        jsonrpc: "2.0",
        id,
        result: { tools: TOOLS.map(({ name, title, description, inputSchema, annotations }) => ({ name, title, description, inputSchema, annotations })) },
      };
    case "tools/call": {
      if (!isRecord(params) || typeof params.name !== "string") return rpcError(id, -32602, "Invalid params.");
      const tool = TOOLS.find((candidate) => candidate.name === params.name);
      if (!tool) return rpcError(id, -32602, `Unknown tool: ${params.name.slice(0, 80)}`);
      const checked = checkArguments(tool, params.arguments);
      if (!checked.ok) return { jsonrpc: "2.0", id, result: errorResult(checked.error) };
      let result: ToolResult;
      try {
        result = await callTool(tool, checked.value, deps);
      } catch {
        result = errorResult("The tool failed.");
      }
      return { jsonrpc: "2.0", id, result };
    }
    default:
      return rpcError(id, -32601, "Method not found.");
  }
}

// HTTP ------------------------------------------------------------------------------------------------------------

const digest = (value: string) => createHash("sha256").update(value).digest();

/** Constant-time compare: both sides are hashed first so the length of the secret does not leak either. */
export function tokenMatches(expected: string, header: string | undefined) {
  const match = /^Bearer ([^\s]+)$/i.exec(header ?? "");
  const presented = match?.[1] ?? "";
  const equal = timingSafeEqual(digest(expected), digest(presented));
  return equal && match !== null;
}

function reply(response: ServerResponse, status: number, body?: unknown, headers: Record<string, string> = {}) {
  const payload = body === undefined ? undefined : JSON.stringify(body);
  response.writeHead(status, {
    "cache-control": "no-store",
    ...(payload === undefined ? {} : { "content-type": "application/json", "content-length": String(Buffer.byteLength(payload)) }),
    ...headers,
  });
  response.end(payload);
}

class BodyTooLarge extends Error {}

function readBody(request: IncomingMessage, limit: number): Promise<string> {
  return new Promise((resolve, reject) => {
    const chunks: Buffer[] = [];
    let size = 0;
    let tooLarge = false;
    request.on("data", (chunk: Buffer) => {
      if (tooLarge) return; // Keep draining (bounded by the request timeout) so the 413 reaches the client.
      size += chunk.length;
      if (size > limit) {
        tooLarge = true;
        chunks.length = 0;
        reject(new BodyTooLarge());
        return;
      }
      chunks.push(chunk);
    });
    request.on("end", () => resolve(Buffer.concat(chunks).toString("utf8")));
    request.on("error", reject);
  });
}

export type DotMcpServer = { url: string; token: string; close(): Promise<void> };

export type DotMcpOptions = {
  tasks: ClaudeTaskRegistry;
  screen: ScreenSource;
  /** Fixed token (tests). By default a fresh random one per launch. */
  token?: string;
  maxBodyBytes?: number;
};

export async function startDotMcpServer(options: DotMcpOptions): Promise<DotMcpServer> {
  const token = options.token ?? randomBytes(32).toString("base64url");
  const limit = options.maxBodyBytes ?? MAX_BODY_BYTES;
  const deps = { tasks: options.tasks, screen: options.screen };
  let port = 0;

  const hostAllowed = (host: string | undefined) => {
    if (!host) return false;
    const match = /^(127\.0\.0\.1|localhost|\[::1\]):(\d+)$/i.exec(host);
    return match !== null && Number(match[2]) === port;
  };

  const server: Server = createServer((request, response) => {
    void (async () => {
      // A browser page cannot reach this server: any Origin is refused, and so is any Host that is not loopback
      // (DNS rebinding). Both checks run before the token is even looked at.
      if (request.headers.origin !== undefined || !hostAllowed(request.headers.host)) return reply(response, 403, { error: "Forbidden." });
      if (!tokenMatches(token, request.headers.authorization)) return reply(response, 401, { error: "Unauthorized." }, { "www-authenticate": "Bearer" });
      const path = (request.url ?? "").split("?", 1)[0];
      if (path !== MCP_PATH) return reply(response, 404, { error: "Not found." });
      if (request.method !== "POST") return reply(response, 405, { error: "Method not allowed." }, { allow: "POST" });
      if (!/^application\/json\s*(;|$)/i.test(request.headers["content-type"] ?? "")) return reply(response, 415, { error: "Send application/json." });

      let body: string;
      try {
        body = await readBody(request, limit);
      } catch (error) {
        const large = error instanceof BodyTooLarge;
        return reply(response, large ? 413 : 400, { error: large ? "Request too large." : "Bad request." }, { connection: "close" });
      }
      let message: unknown;
      try {
        message = JSON.parse(body);
      } catch {
        return reply(response, 400, rpcError(null, -32700, "Parse error."));
      }

      if (Array.isArray(message)) {
        if (message.length === 0 || message.length > 16) return reply(response, 400, rpcError(null, -32600, "Invalid request."));
        const answers = (await Promise.all(message.map((entry) => handleMessage(entry, deps)))).filter((answer): answer is RpcResponse => answer !== undefined);
        return answers.length === 0 ? reply(response, 202) : reply(response, 200, answers);
      }
      const answer = await handleMessage(message, deps);
      if (answer === undefined) return reply(response, 202);
      // An invalid request has no usable id; the spec's HTTP mapping for that is a 400.
      return reply(response, "error" in answer && answer.id === null ? 400 : 200, answer);
    })().catch(() => {
      if (!response.headersSent) reply(response, 500, { error: "Internal error." });
      else response.destroy();
    });
  });
  server.requestTimeout = 30_000;
  server.headersTimeout = 10_000;

  await new Promise<void>((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      server.off("error", reject);
      resolve();
    });
  });
  port = (server.address() as AddressInfo).port;

  let closing: Promise<void> | undefined;
  return {
    url: `http://127.0.0.1:${port}${MCP_PATH}`,
    token,
    close() {
      closing ??= new Promise<void>((resolve) => {
        server.close(() => resolve());
        server.closeAllConnections();
      });
      return closing;
    },
  };
}

// Codex configuration ---------------------------------------------------------------------------------------------

export type CodexConfigOptions = {
  /** Environment variable that carries the token to app-server; the token itself never appears in a config string. */
  envVarName?: string;
  /** The name the server is registered under, which prefixes Codex's tool names (`mcp__dot__...`). */
  serverName?: string;
  /** Approval mode for the tools that only read. `auto` follows Codex's own policy and the tools' read-only hints. */
  readApprovalMode?: "auto" | "approve";
};

const TOML_KEY = /^[A-Za-z0-9_-]+$/;

/**
 * The values for Codex's `-c key=value` flag that register this server. The value part is a TOML literal string
 * ('like this', see external-mcp-toml.mts) so it passes through a Windows `.cmd` shim unchanged. Codex 0.163 keys:
 * `mcp_servers.<name>.url`, `.bearer_token_env_var`, `.enabled_tools`, and `.tools.<tool>.approval_mode`, whose values are
 * `auto | prompt | writes | approve`. Starting a task and cancelling one are `prompt`: Codex always asks the user.
 */
export function codexConfigOverrides(server: { url: string }, options: CodexConfigOptions = {}): string[] {
  const envVarName = options.envVarName ?? MCP_TOKEN_ENV_VAR;
  const name = options.serverName ?? MCP_SERVER_NAME;
  const readMode = options.readApprovalMode ?? "auto";
  if (!TOML_KEY.test(name)) throw new Error("The MCP server name is not a valid config key.");
  if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(envVarName)) throw new Error("The token environment variable name is not valid.");
  const prefix = `mcp_servers.${name}`;
  return [
    `${prefix}.url=${tomlLiteral(server.url)}`,
    `${prefix}.bearer_token_env_var=${tomlLiteral(envVarName)}`,
    `${prefix}.enabled_tools=${tomlLiteralArray(TOOLS.map((tool) => tool.name))}`,
    ...TOOLS.map((tool) => `${prefix}.tools.${tool.name}.approval_mode=${tomlLiteral(tool.approval === "prompt" ? "prompt" : readMode)}`),
  ];
}

/** The same overrides as an argv fragment: `-c`, value, `-c`, value... */
export function codexConfigArgs(server: { url: string }, options: CodexConfigOptions = {}): string[] {
  return codexConfigOverrides(server, options).flatMap((override) => ["-c", override]);
}

/** The environment to add to the `codex app-server` process so it can read the token. */
export function codexMcpEnv(server: { token: string }, options: { envVarName?: string } = {}): Record<string, string> {
  return { [options.envVarName ?? MCP_TOKEN_ENV_VAR]: server.token };
}
