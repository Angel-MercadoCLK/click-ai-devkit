import { createServer, type IncomingMessage, type Server, type ServerResponse } from "node:http";
import { DOT_HOST, DOT_PORT } from "./config.mts";
import { listenOnLoopback } from "./listen.mts";
import { loadBuiltNextConfig } from "./next-config.mts";

/**
 * A permission request from the Claude Code hook is held for up to 105 s (the relay itself waits 110 s), so the server
 * must not cut a request that is waiting for its answer. Node does not time out a pending response on its own:
 * `timeout` (an idle socket) is off, `keepAliveTimeout` only applies between requests, and `requestTimeout` and
 * `headersTimeout` only watch the incoming request. They are set here so a Node default change cannot cut a held request.
 */
export const HELD_REQUEST_LIMIT_MS = 110_000;

export function createDotHttpServer(handle: (request: IncomingMessage, response: ServerResponse) => void): Server {
  const server = createServer(handle);
  server.timeout = 0;
  server.requestTimeout = 300_000;
  server.headersTimeout = 60_000;
  server.keepAliveTimeout = 5_000;
  return server;
}

type CreateNext = (options: { dev: boolean; dir: string; hostname: string; port: number }) => {
  prepare(): Promise<void>;
  getRequestHandler(): (request: IncomingMessage, response: ServerResponse) => Promise<void>;
};

/**
 * Runs the production Next.js server inside this process, on loopback only.
 * Next.js is imported lazily so the environment is fully configured before it loads.
 */
export async function startDotServer(dir: string, options: { workingDirectory?: string } = {}): Promise<Server> {
  // Same setup as Next.js standalone output: serve the config serialized at build time.
  Object.assign(process.env, { NODE_ENV: "production" });
  process.env.__NEXT_PRIVATE_STANDALONE_CONFIG = JSON.stringify(await loadBuiltNextConfig(dir));
  // Next.js resolves project paths against the working directory, which is arbitrary for a desktop launch.
  // `dir` may be inside app.asar, which is not a real directory and cannot be the working directory,
  // so the caller can name a real one (a per-user directory) instead.
  process.chdir(options.workingDirectory ?? dir);
  // At runtime the CommonJS entry point is the factory itself; the bundled typings model it differently under NodeNext.
  const { default: next } = (await import("next")) as unknown as { default: CreateNext };
  const app = next({ dev: false, dir, hostname: DOT_HOST, port: DOT_PORT });
  await app.prepare();
  const handle = app.getRequestHandler();
  const server = createDotHttpServer((request, response) => {
    handle(request, response).catch((error: unknown) => {
      console.error("Request failed.", error);
      if (!response.headersSent) response.statusCode = 500;
      response.end();
    });
  });
  await listenOnLoopback(server, DOT_PORT);
  return server;
}
