import type { Server } from "node:http";
import { DOT_HOST } from "./config.mts";

export class PortInUseError extends Error {
  readonly port: number;

  constructor(port: number) {
    super(`Port ${port} is already in use.`);
    this.name = "PortInUseError";
    this.port = port;
  }
}

/** Binds to 127.0.0.1 only. A busy port is an error: the OAuth callback needs this exact port. */
export function listenOnLoopback(server: Server, port: number) {
  return new Promise<void>((resolve, reject) => {
    const onError = (error: NodeJS.ErrnoException) => {
      reject(error.code === "EADDRINUSE" ? new PortInUseError(port) : error);
    };
    server.once("error", onError);
    server.listen(port, DOT_HOST, () => {
      server.off("error", onError);
      resolve();
    });
  });
}
