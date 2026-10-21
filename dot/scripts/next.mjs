import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";

const [command, ...args] = process.argv.slice(2);
const supportedCommands = new Set(["dev", "build", "start"]);

if (!supportedCommands.has(command)) {
  throw new Error("Usage: node scripts/next.mjs <dev|build|start>");
}

process.env.NEXT_TELEMETRY_DISABLED = "1";

const nextCli = fileURLToPath(new URL("../node_modules/next/dist/bin/next", import.meta.url));
const commandArgs = [nextCli, command, ...args];

if (command !== "build") {
  commandArgs.push("--hostname", "127.0.0.1");
}

const next = spawn(process.execPath, commandArgs, {
  env: process.env,
  stdio: "inherit",
});

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.once(signal, () => {
    if (!next.killed) {
      next.kill(signal);
    }
  });
}

next.on("error", (error) => {
  process.stderr.write(`Could not start Next.js: ${error.message}\n`);
  process.exitCode = 1;
});

next.on("exit", (code, signal) => {
  if (signal) {
    process.exitCode = signal === "SIGINT" ? 130 : 143;
    return;
  }
  process.exitCode = code ?? 1;
});
