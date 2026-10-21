import { createServer, type IncomingMessage, type Server, type ServerResponse } from "node:http";
import { DOT_HOST, DOT_PORT } from "./config.mts";
import { listenOnLoopback } from "./listen.mts";
import { loadBuiltNextConfig } from "./next-config.mts";

type CreateNext = (options: { dev: boolean; dir: string; hostname: string; port: number }) => {
  prepare(): Promise<void>;
  getRequestHandler(): (request: IncomingMessage, response: ServerResponse) => Promise<void>;
};

/**
 * Runs the production Next.js server inside this process, on loopback only.
 * Next.js is imported lazily so the environment is fully configured before it loads.
 */
export async function startDotServer(dir: string): Promise<Server> {
  // Same setup as Next.js standalone output: serve the config serialized at build time.
  Object.assign(process.env, { NODE_ENV: "production" });
  process.env.__NEXT_PRIVATE_STANDALONE_CONFIG = JSON.stringify(await loadBuiltNextConfig(dir));
  // Next.js resolves project paths against the working directory, which is arbitrary for a desktop launch.
  process.chdir(dir);
  // At runtime the CommonJS entry point is the factory itself; the bundled typings model it differently under NodeNext.
  const { default: next } = (await import("next")) as unknown as { default: CreateNext };
  const app = next({ dev: false, dir, hostname: DOT_HOST, port: DOT_PORT });
  await app.prepare();
  const handle = app.getRequestHandler();
  const server = createServer((request, response) => {
    handle(request, response).catch((error: unknown) => {
      console.error("Request failed.", error);
      if (!response.headersSent) response.statusCode = 500;
      response.end();
    });
  });
  await listenOnLoopback(server, DOT_PORT);
  return server;
}
