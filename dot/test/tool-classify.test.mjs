import assert from "node:assert/strict";
import { test } from "node:test";

const classify = await import("../desktop/tool-classify.mts");
const { decideReadOnly, parseClassifierOutput, createClassifier } = classify;

const stmt = (...elements) => elements.map(([command, ...args]) => ({ command, args }));
const decide = (...statements) => decideReadOnly(statements);
const allowed = (...statements) => {
  const result = decide(...statements);
  assert.equal(result.readOnly, true, `expected read-only: ${JSON.stringify(statements)} -> ${result.reason}`);
};
const rejected = (reason, ...statements) => {
  const result = decide(...statements);
  assert.equal(result.readOnly, false, `expected approval: ${JSON.stringify(statements)}`);
  assert.match(result.reason, reason);
};

// Allowlist -----------------------------------------------------------------------------------------------------

test("every allowlisted cmdlet is read-only with literal arguments", () => {
  for (const name of [
    "Get-ChildItem", "Get-Content", "Get-Item", "Get-Location", "Test-Path", "Resolve-Path", "Split-Path", "Join-Path",
    "Select-String", "Measure-Object", "Get-Process", "Get-Date", "Get-Command", "Select-Object", "Sort-Object",
    "Format-Table", "Format-List",
  ]) {
    allowed(stmt([name, "x"]));
    allowed(stmt([name.toUpperCase(), "-Force"]));
  }
});

test("the fixed alias map resolves to allowlisted cmdlets, case-insensitively", () => {
  for (const alias of ["ls", "dir", "gci", "cat", "type", "gc", "pwd", "gi", "sls", "measure", "select", "sort", "ft", "fl", "gps", "LS", "Cat"]) {
    allowed(stmt([alias, "x"]));
  }
});

test("aliases that write, delete or execute are rejected", () => {
  for (const alias of ["rm", "ri", "del", "erase", "rd", "rmdir", "cp", "copy", "mv", "move", "ni", "sc", "iex", "echo", "cd", "cls", "start", "saps", "kill", "spps"]) {
    rejected(/not on the read-only list/i, stmt([alias, "x"]));
  }
});

test("cmdlets that are not allowlisted are rejected", () => {
  for (const name of ["Remove-Item", "Set-Content", "Out-File", "Invoke-Expression", "Invoke-WebRequest", "Start-Process", "New-Item", "Stop-Process", "Get-Service", "Where-Object", "ForEach-Object", "tee", "powershell", "cmd", "node", ".\\run.ps1", "C:\\Windows\\notepad.exe"]) {
    rejected(/not on the read-only list/i, stmt([name]));
  }
});

test("whoami, hostname and where.exe are read-only", () => {
  allowed(stmt(["whoami"]));
  allowed(stmt(["whoami", "/all"]));
  allowed(stmt(["hostname"]));
  allowed(stmt(["where.exe", "git"]));
  allowed(stmt(["WHERE.EXE", "node"]));
});

test("hostname with arguments needs approval", () => {
  rejected(/hostname/, stmt(["hostname", "newname"]));
  rejected(/hostname/, stmt(["HOSTNAME", "-s"]));
});

test("a cmdlet pointed at another computer needs approval, including abbreviations and the cn alias", () => {
  rejected(/another computer/, stmt(["Get-Process", "-ComputerName", "srv"]));
  rejected(/another computer/, stmt(["gps", "-comp", "srv"]));
  rejected(/another computer/, stmt(["Get-Process", "-c", "srv"]));
  rejected(/another computer/, stmt(["Get-Process", "-Cn", "srv"]));
  rejected(/another computer/, stmt(["Get-Process", "-ComputerName:srv"]));
  allowed(stmt(["Get-Process", "-Name", "node"]));
});

test("pipelines of allowlisted commands and several pipeline statements are read-only", () => {
  allowed(stmt(["Get-Process"], ["Sort-Object", "CPU", "-Descending"], ["Select-Object", "-First", "5"]));
  allowed(stmt(["ls"]), stmt(["pwd"]));
});

test("one disallowed command anywhere makes the whole command need approval", () => {
  rejected(/Remove-Item/i, stmt(["Get-Content", "a.txt"], ["Remove-Item"]));
  rejected(/rm/i, stmt(["ls"]), stmt(["rm", "x"]));
  rejected(/rm/i, stmt(["rm", "x"]), stmt(["ls"]));
});

test("an empty command, or one without pipelines, needs approval", () => {
  rejected(/empty/i);
  rejected(/empty/i, []);
});

// git ----------------------------------------------------------------------------------------------------------

test("git allows its read-only subcommands", () => {
  for (const sub of ["status", "log", "diff", "show", "rev-parse"]) allowed(stmt(["git", sub]));
  allowed(stmt(["git", "status", "--short", "--branch"]));
  allowed(stmt(["git", "log", "--oneline", "-n", "5"]));
  allowed(stmt(["git", "diff", "HEAD~1", "--", "src/a.ts"]));
  allowed(stmt(["GIT", "show", "HEAD:README.md"]));
  allowed(stmt(["git", "rev-parse", "--show-toplevel"]));
});

test("git with another subcommand, or none, needs approval", () => {
  for (const sub of ["push", "commit", "checkout", "reset", "clean", "fetch", "pull", "add", "config", "stash", "rebase", "merge", "tag", "apply"]) {
    rejected(/git/i, stmt(["git", sub]));
  }
  rejected(/git/i, stmt(["git"]));
});

test("git global options before the subcommand are rejected, including -c and -C", () => {
  rejected(/git/i, stmt(["git", "-c", "core.pager=evil", "log"]));
  rejected(/git/i, stmt(["git", "-C", "..", "status"]));
  rejected(/git/i, stmt(["git", "--exec-path=x", "status"]));
  rejected(/git/i, stmt(["git", "--no-pager", "log"]));
});

test("git rejects --textconv, which runs filters from the repository's configuration", () => {
  rejected(/textconv/, stmt(["git", "show", "--textconv", "HEAD:a.txt"]));
  rejected(/textconv/, stmt(["git", "log", "-p", "--textconv"]));
});

test("git rejects -c and -C anywhere in its arguments", () => {
  rejected(/-c|option/i, stmt(["git", "log", "-c"]));
  rejected(/-C|option/i, stmt(["git", "status", "-C"]));
});

test("git rejects arguments that write files, run programs or change configuration", () => {
  rejected(/output/i, stmt(["git", "diff", "--output=x"]));
  rejected(/output/i, stmt(["git", "log", "--output", "x"]));
  rejected(/ext-diff/i, stmt(["git", "diff", "--ext-diff"]));
  rejected(/config/i, stmt(["git", "log", "--config=core.pager=x"]));
  rejected(/exec/i, stmt(["git", "show", "--exec=x"]));
  rejected(/upload-pack/i, stmt(["git", "diff", "--upload-pack=x"]));
  rejected(/receive-pack/i, stmt(["git", "diff", "--receive-pack=x"]));
  rejected(/output/i, stmt(["git", "diff", "--OUTPUT=x"]));
});

test("git rejects unambiguous abbreviations of the forbidden long options", () => {
  for (const arg of ["--out=x", "--outp", "--ext", "--ext-d", "--conf=x", "--exe=x", "--upload=x", "--receive=x", "--o=x"]) {
    rejected(/abbreviat|forbidden|option/i, stmt(["git", "diff", arg]));
  }
  // Options that merely start with the same letters are not abbreviations of a forbidden one.
  allowed(stmt(["git", "log", "--oneline", "--color", "--stat"]));
});

test("git branch is read-only only when it lists", () => {
  allowed(stmt(["git", "branch"]));
  allowed(stmt(["git", "branch", "-a"]));
  allowed(stmt(["git", "branch", "-vv"]));
  allowed(stmt(["git", "branch", "--show-current"]));
  allowed(stmt(["git", "branch", "--list", "feat/*"]));
  rejected(/branch/i, stmt(["git", "branch", "new-branch"]));
  rejected(/branch/i, stmt(["git", "branch", "-D", "main"]));
  rejected(/branch/i, stmt(["git", "branch", "-m", "a", "b"]));
  rejected(/branch/i, stmt(["git", "branch", "--delete", "x"]));
  rejected(/branch/i, stmt(["git", "branch", "--set-upstream-to=origin/x"]));
});

test("git remote is read-only only when it lists or reads a url", () => {
  allowed(stmt(["git", "remote"]));
  allowed(stmt(["git", "remote", "-v"]));
  allowed(stmt(["git", "remote", "get-url", "origin"]));
  for (const args of [["add", "x", "y"], ["remove", "x"], ["rename", "a", "b"], ["set-url", "x", "y"], ["prune", "origin"], ["update"], ["show", "origin"]]) {
    rejected(/remote/i, stmt(["git", "remote", ...args]));
  }
});

// Literal-only (the helper marks anything else unsupported) -----------------------------------------------------

test("parseClassifierOutput accepts a well-formed description and decodes base64 UTF-8", () => {
  const b64 = (text) => Buffer.from(text, "utf8").toString("base64");
  const output = JSON.stringify({ ok: true, statements: [[{ c: b64("git"), a: [b64("status"), b64("--short"), b64("ñandú")] }, { c: b64("Select-Object"), a: [] }]] });
  assert.deepEqual(parseClassifierOutput(output), {
    ok: true,
    statements: [[{ command: "git", args: ["status", "--short", "ñandú"] }, { command: "Select-Object", args: [] }]],
  });
});

test("parseClassifierOutput carries the helper's unsupported reason", () => {
  assert.deepEqual(parseClassifierOutput(JSON.stringify({ ok: false, reason: "redirection" })), { ok: false, reason: "redirection" });
});

test("parseClassifierOutput rejects malformed or oversized output", () => {
  const b64 = (text) => Buffer.from(text, "utf8").toString("base64");
  const bad = [
    "",
    "not json",
    "null",
    "[]",
    "{}",
    JSON.stringify({ ok: true }),
    JSON.stringify({ ok: true, statements: "x" }),
    JSON.stringify({ ok: true, statements: [["x"]] }),
    JSON.stringify({ ok: true, statements: [[{ c: "not base64!", a: [] }]] }),
    JSON.stringify({ ok: true, statements: [[{ c: b64("ls"), a: "x" }]] }),
    JSON.stringify({ ok: true, statements: [[{ c: b64("ls"), a: [5] }]] }),
    JSON.stringify({ ok: true, statements: [[{ c: b64("ls"), a: [], extra: 1 }]] }),
    JSON.stringify({ ok: true, statements: [[]] }),
    JSON.stringify({ ok: true, statements: Array.from({ length: 200 }, () => [{ c: b64("ls"), a: [] }]) }),
    JSON.stringify({ ok: true, statements: [Array.from({ length: 200 }, () => ({ c: b64("ls"), a: [] }))] }),
    JSON.stringify({ ok: false }),
    JSON.stringify({ ok: "yes", statements: [] }),
  ];
  for (const text of bad) assert.equal(parseClassifierOutput(text).ok, false, text.slice(0, 60));
});

// The async classifier: fail-closed on every helper failure ------------------------------------------------------

function fakeChild({ stdout = "", stderr = "", code = 0, never = false, spawnError = false } = {}) {
  const listeners = {};
  const state = { killed: false, stdin: [], ended: false };
  const child = {
    pid: 4242,
    stdin: { write: (chunk) => { state.stdin.push(String(chunk)); return true; }, end: () => { state.ended = true; }, on: () => {} },
    stdout: { on: (event, listener) => { listeners.stdout = listener; } },
    stderr: { on: (event, listener) => { listeners.stderr = listener; } },
    on: (event, listener) => { listeners[event] = listener; },
    kill: () => { state.killed = true; },
  };
  queueMicrotask(() => setTimeout(() => {
    if (never) return;
    if (spawnError) { listeners.error?.(new Error("ENOENT")); return; }
    if (stdout) listeners.stdout?.(Buffer.from(stdout));
    if (stderr) listeners.stderr?.(Buffer.from(stderr));
    listeners.close?.(code);
  }, 0));
  return { child, state };
}

function manualTimers() {
  const pending = new Map();
  let next = 1;
  return {
    setTimeout: (fn, ms) => { const id = next++; pending.set(id, { fn, ms }); return id; },
    clearTimeout: (id) => { pending.delete(id); },
    fire: (ms) => { for (const [id, timer] of [...pending]) if (timer.ms === ms) { pending.delete(id); timer.fn(); } },
    pending,
  };
}

const b64 = (text) => Buffer.from(text, "utf8").toString("base64");
const goodOutput = (command, ...args) => JSON.stringify({ ok: true, statements: [[{ c: b64(command), a: args.map(b64) }]] });

function makeClassifier(options = {}) {
  const calls = [];
  const timers = options.timers ?? manualTimers();
  const classifier = createClassifier({
    spawn: (executable, args, spawnOptions) => { calls.push({ executable, args, options: spawnOptions }); return options.child.child; },
    executable: "powershell.exe",
    timers,
    timeoutMs: 10_000,
    maxOutputBytes: 64 * 1024,
    killTree: (pid) => { calls.push({ kill: pid }); },
    ...options.extra,
  });
  return { classifier, calls, timers };
}

test("the classifier sends the command on stdin as base64 and never interpolates it", async () => {
  const command = "Get-Content 'a b.txt'; $(evil)";
  const child = fakeChild({ stdout: goodOutput("Get-Content", "a b.txt") });
  const { classifier, calls } = makeClassifier({ child });
  await classifier(command);
  const { executable, args, options } = calls[0];
  assert.equal(executable, "powershell.exe");
  const encodedIndex = args.indexOf("-EncodedCommand");
  assert.ok(encodedIndex > 0);
  const script = Buffer.from(args[encodedIndex + 1], "base64").toString("utf16le");
  assert.ok(!script.includes(command), "the command text is not part of the script");
  assert.ok(!script.includes("evil"));
  assert.match(script, /ParseInput/);
  assert.ok(args.includes("-NoProfile") && args.includes("-NonInteractive"));
  assert.equal(args[args.indexOf("-InputFormat") + 1], "None");
  assert.equal(options.windowsHide, true);
  assert.equal(options.shell, false);
  assert.equal(child.state.stdin.join(""), Buffer.from(command, "utf8").toString("base64"));
  assert.equal(child.state.ended, true);
});

test("the classifier returns the decision for a parsed read-only command", async () => {
  const { classifier } = makeClassifier({ child: fakeChild({ stdout: goodOutput("git", "status") }) });
  assert.deepEqual((await classifier("git status")).readOnly, true);
});

test("the classifier returns the decision for a parsed command that is not read-only", async () => {
  const { classifier } = makeClassifier({ child: fakeChild({ stdout: goodOutput("Remove-Item", "x") }) });
  const result = await classifier("Remove-Item x");
  assert.equal(result.readOnly, false);
  assert.match(result.reason, /Remove-Item/);
});

test("the classifier passes the helper's unsupported reason through as approval needed", async () => {
  const { classifier } = makeClassifier({ child: fakeChild({ stdout: JSON.stringify({ ok: false, reason: "redirection is not allowed" }) }) });
  const result = await classifier("Get-ChildItem > out.txt");
  assert.equal(result.readOnly, false);
  assert.match(result.reason, /redirection/);
});

test("the classifier fails closed on a helper that exits non-zero", async () => {
  const { classifier } = makeClassifier({ child: fakeChild({ stdout: goodOutput("ls"), code: 1 }) });
  const result = await classifier("ls");
  assert.equal(result.readOnly, false);
});

test("the classifier fails closed on empty, garbage or oversized helper output", async () => {
  for (const stdout of ["", "garbage", "x".repeat(70 * 1024)]) {
    const { classifier } = makeClassifier({ child: fakeChild({ stdout }) });
    assert.equal((await classifier("ls")).readOnly, false);
  }
});

test("the classifier fails closed when the helper cannot start", async () => {
  const { classifier } = makeClassifier({ child: fakeChild({ spawnError: true }) });
  assert.equal((await classifier("ls")).readOnly, false);
  const throwing = createClassifier({ spawn: () => { throw new Error("boom"); }, executable: "powershell.exe", timers: manualTimers() });
  assert.equal((await throwing("ls")).readOnly, false);
});

test("the classifier fails closed on timeout and kills the helper's process tree", async () => {
  const child = fakeChild({ never: true });
  const { classifier, calls, timers } = makeClassifier({ child });
  const pending = classifier("ls");
  await new Promise((resolve) => setTimeout(resolve, 5));
  timers.fire(10_000);
  const result = await pending;
  assert.equal(result.readOnly, false);
  assert.match(result.reason, /time/i);
  assert.ok(calls.some((call) => call.kill === 4242), "the tree kill was requested");
});

test("the classifier refuses empty, oversized or NUL-bearing commands without starting a process", async () => {
  const { classifier, calls } = makeClassifier({ child: fakeChild({ stdout: goodOutput("ls") }) });
  for (const command of ["", "   \n", "ls\u0000x", "ls ".repeat(5000), 5, null, undefined]) {
    assert.equal((await classifier(command)).readOnly, false);
  }
  assert.equal(calls.length, 0);
});

test("network paths need approval so a read never leaks credentials to another machine", () => {
  rejected(/network/i, stmt(["Get-ChildItem", "\\\\server\\share"]));
  rejected(/network/i, stmt(["cat", "//server/share/file"]));
  rejected(/network/i, stmt(["Get-Content", "-Path:\\\\server\\share\\file"]));
  allowed(stmt(["Get-ChildItem", "C:\\Users\\angel"]));
});

test("provider-qualified paths need approval, since they can name a network share", () => {
  rejected(/provider|network/i, stmt(["Get-Content", "FileSystem::\\\\server\\share\\file"]));
  rejected(/provider|network/i, stmt(["Get-ChildItem", "Microsoft.PowerShell.Core\\FileSystem::\\\\server\\share"]));
  rejected(/provider|network/i, stmt(["gci", "FileSystem:://server/share"]));
  rejected(/provider|network/i, stmt(["Test-Path", "-Path:FileSystem::\\\\server\\share"]));
});

test("git checks the pieces of an argument PowerShell 5.1 could re-split, and rejects quotes", () => {
  rejected(/output/i, stmt(["git", "log", "--since=a --output=x"]));
  rejected(/-c/i, stmt(["git", "log", "--grep=a -c"]));
  rejected(/quote/i, stmt(["git", "log", "--grep=\"x"]));
  allowed(stmt(["git", "log", "--grep=fix bug"]));
});
