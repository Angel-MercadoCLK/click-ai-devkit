import {
  createKillTree,
  DEFAULT_LIMITS,
  defaultSpawn,
  defaultTimers,
  encodePowerShell,
  powershellExecutable,
  runProcess,
  type KillTree,
  type ToolSpawn,
  type ToolTimers,
} from "./tool-process.mts";

// Read-only classification of a PowerShell command (S3). It is FAIL-CLOSED: the only way to "readOnly: true" is a
// command the PowerShell parser accepts without errors, made only of pipelines of allowlisted commands with literal
// arguments. Everything else (syntax error, unknown command, variable, subexpression, redirection, helper failure,
// timeout) means "needs approval". Two stages:
//   1. A helper script runs inside powershell.exe, parses the command text (given on stdin as base64, never
//      interpolated into the script) with the real PowerShell parser and prints a small description of it.
//   2. A pure TypeScript decision function applies the allowlist, the alias map and the git rules to that description.
// The helper's rules follow Codex's PowerShell parser helper (codex-rs/shell-command/src/command_safety).

export type Classification = { readOnly: boolean; reason: string };
export type PsCommand = { command: string; args: string[] };
export type PsStatement = PsCommand[];

/**
 * The helper. It reads the command (UTF-8, base64) from stdin, parses it, and prints exactly one JSON line:
 *   { "ok": true, "statements": [ [ { "c": base64(name), "a": [base64(arg)...] } ... ] ... ] }
 *   { "ok": false, "reason": "<fixed ASCII text>" }
 * Names and arguments travel as base64 so no escaping can go wrong. Any failure inside prints an "ok": false line.
 */
export const CLASSIFIER_SCRIPT = String.raw`$ErrorActionPreference = 'Stop'
$script:lits = New-Object System.Collections.Generic.List[string]
function Say([string]$json) { [Console]::Out.Write($json) }
function Fail([string]$reason) { Say ('{"ok":false,"reason":"' + $reason + '"}'); exit 0 }
function B64([string]$s) { [Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($s)) }
function Lit($e) {
  if ($e -is [System.Management.Automation.Language.StringConstantExpressionAst]) { $script:lits.Add([string]$e.Value); return $true }
  if ($e -is [System.Management.Automation.Language.ConstantExpressionAst]) { $script:lits.Add([string]$e.Extent.Text); return $true }
  if ($e -is [System.Management.Automation.Language.ArrayLiteralAst]) {
    foreach ($x in $e.Elements) { if (-not (Lit $x)) { return $false } }
    return $true
  }
  return $false
}
try {
  [Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
  $ms = New-Object System.IO.MemoryStream
  [Console]::OpenStandardInput().CopyTo($ms)
  $raw = [System.Text.Encoding]::ASCII.GetString($ms.ToArray()).Trim()
  $text = [System.Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($raw))
  $tokens = $null
  $errors = $null
  $ast = [System.Management.Automation.Language.Parser]::ParseInput($text, [ref]$tokens, [ref]$errors)
  if ($errors.Count -gt 0) { Fail 'the command has a syntax error' }
  foreach ($t in $tokens) { if ($t.Text -eq '--%') { Fail 'the stop-parsing token is not allowed' } }
  if ($null -ne $ast.ParamBlock -or $null -ne $ast.BeginBlock -or $null -ne $ast.ProcessBlock -or $null -ne $ast.DynamicParamBlock -or $ast.UsingStatements.Count -gt 0) { Fail 'only plain pipelines are allowed' }
  $end = $ast.EndBlock
  if ($null -eq $end) { Fail 'the command is empty' }
  if ($null -ne $end.Traps -and $end.Traps.Count -gt 0) { Fail 'only plain pipelines are allowed' }
  $out = New-Object System.Text.StringBuilder
  [void]$out.Append('{"ok":true,"statements":[')
  $firstStatement = $true
  foreach ($st in $end.Statements) {
    if ($st -isnot [System.Management.Automation.Language.PipelineAst]) { Fail 'only pipelines of commands are allowed' }
    if (-not $firstStatement) { [void]$out.Append(',') }
    $firstStatement = $false
    [void]$out.Append('[')
    $firstElement = $true
    foreach ($el in $st.PipelineElements) {
      if ($el -isnot [System.Management.Automation.Language.CommandAst]) { Fail 'only commands are allowed in a pipeline' }
      if ($el.InvocationOperator -ne [System.Management.Automation.Language.TokenKind]::Unknown) { Fail 'the call operators are not allowed' }
      if ($el.Redirections.Count -gt 0) { Fail 'redirection is not allowed' }
      $parts = $el.CommandElements
      if ($parts.Count -lt 1) { Fail 'the command is empty' }
      $name = $parts[0]
      if ($name -isnot [System.Management.Automation.Language.StringConstantExpressionAst] -or $name.StringConstantType -ne [System.Management.Automation.Language.StringConstantType]::BareWord) { Fail 'the command name must be a plain word' }
      $script:lits.Clear()
      for ($i = 1; $i -lt $parts.Count; $i++) {
        $p = $parts[$i]
        if ($p -is [System.Management.Automation.Language.CommandParameterAst]) {
          $script:lits.Add([string]$p.Extent.Text)
          if ($null -ne $p.Argument -and -not (Lit $p.Argument)) { Fail 'only literal arguments are allowed' }
        } elseif (-not (Lit $p)) {
          Fail 'only literal arguments are allowed'
        }
      }
      if (-not $firstElement) { [void]$out.Append(',') }
      $firstElement = $false
      [void]$out.Append('{"c":"' + (B64 ([string]$name.Value)) + '","a":[')
      $firstArg = $true
      foreach ($a in $script:lits) {
        if (-not $firstArg) { [void]$out.Append(',') }
        $firstArg = $false
        [void]$out.Append('"' + (B64 $a) + '"')
      }
      [void]$out.Append(']}')
    }
    [void]$out.Append(']')
  }
  [void]$out.Append(']}')
  Say $out.ToString()
} catch {
  Say '{"ok":false,"reason":"the classifier helper failed"}'
}
`;

// Helper output ------------------------------------------------------------------------------------------------

const MAX_STATEMENTS = 64;
const MAX_ELEMENTS = 64;
const MAX_ARGS = 256;
const MAX_TEXT = 4_096;
const BASE64 = /^[A-Za-z0-9+/]*={0,2}$/;

export type ClassifierOutput = { ok: true; statements: PsStatement[] } | { ok: false; reason: string };

function decodeField(value: unknown): string | null {
  if (typeof value !== "string" || value.length > MAX_TEXT * 2 || value.length % 4 !== 0 || !BASE64.test(value)) return null;
  const text = Buffer.from(value, "base64").toString("utf8");
  return text.length <= MAX_TEXT && !text.includes("\u0000") ? text : null;
}

const invalid = (reason = "The read-only check returned an unreadable result."): ClassifierOutput => ({ ok: false, reason });

/** Strict validation of the helper's stdout. Anything but a well-formed description is a failed classification. */
export function parseClassifierOutput(stdout: string): ClassifierOutput {
  let parsed: unknown;
  try {
    parsed = JSON.parse(stdout);
  } catch {
    return invalid();
  }
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return invalid();
  const record = parsed as Record<string, unknown>;
  if (record.ok === false) {
    const keys = Object.keys(record);
    if (keys.length !== 2 || typeof record.reason !== "string" || record.reason.length === 0 || record.reason.length > 200) return invalid();
    return { ok: false, reason: record.reason };
  }
  if (record.ok !== true || Object.keys(record).length !== 2 || !Array.isArray(record.statements) || record.statements.length > MAX_STATEMENTS) return invalid();
  const statements: PsStatement[] = [];
  for (const rawStatement of record.statements as unknown[]) {
    if (!Array.isArray(rawStatement) || rawStatement.length === 0 || rawStatement.length > MAX_ELEMENTS) return invalid();
    const statement: PsStatement = [];
    for (const rawElement of rawStatement as unknown[]) {
      if (!rawElement || typeof rawElement !== "object" || Array.isArray(rawElement)) return invalid();
      const element = rawElement as Record<string, unknown>;
      if (Object.keys(element).length !== 2 || !Array.isArray(element.a) || element.a.length > MAX_ARGS) return invalid();
      const command = decodeField(element.c);
      if (command === null || command.length === 0) return invalid();
      const args: string[] = [];
      for (const rawArg of element.a as unknown[]) {
        const arg = decodeField(rawArg);
        if (arg === null) return invalid();
        args.push(arg);
      }
      statement.push({ command, args });
    }
    statements.push(statement);
  }
  return { ok: true, statements };
}

// Decision ------------------------------------------------------------------------------------------------------

const CMDLETS = new Set([
  "get-childitem", "get-content", "get-item", "get-location", "test-path", "resolve-path", "split-path", "join-path",
  "select-string", "measure-object", "get-process", "get-date", "get-command", "select-object", "sort-object",
  "format-table", "format-list",
]);

/** The fixed alias map. Anything not listed here (rm, ri, del, iex, echo, cd, ...) is not an alias this check knows. */
const ALIASES: Record<string, string> = {
  ls: "get-childitem", dir: "get-childitem", gci: "get-childitem",
  cat: "get-content", type: "get-content", gc: "get-content",
  pwd: "get-location", gi: "get-item", sls: "select-string", measure: "measure-object",
  select: "select-object", sort: "sort-object", ft: "format-table", fl: "format-list", gps: "get-process",
};

const NATIVE = new Set(["whoami", "hostname", "where.exe"]);
const GIT_SUBCOMMANDS = new Set(["status", "log", "diff", "show", "branch", "rev-parse", "remote"]);

/** Long git options that write files, run programs or change configuration. */
const GIT_FORBIDDEN_LONG = ["--output", "--ext-diff", "--textconv", "--config", "--exec", "--upload-pack", "--receive-pack"];
const GIT_BRANCH_FLAGS = new Set(["-a", "--all", "-r", "--remotes", "-v", "-vv", "--verbose", "--list", "-l", "--show-current", "--no-color"]);
const NETWORK_PATH = /^(-[A-Za-z]+:)?(\\\\|\/\/)/;

/**
 * True for a parameter that sends the cmdlet to another computer: -ComputerName, any abbreviation PowerShell would
 * accept for it (down to -c) and its -cn alias, also in the -Name:value form.
 */
function targetsAnotherComputer(arg: string): boolean {
  if (!arg.startsWith("-")) return false;
  const name = arg.split(":")[0]!.toLowerCase();
  return name === "-cn" || (name.length >= 2 && "-computername".startsWith(name));
}

const needsApproval = (reason: string): Classification => ({ readOnly: false, reason });
const readOnly = (reason: string): Classification => ({ readOnly: true, reason });

function checkGit(args: string[]): Classification {
  const sub = args[0]?.toLowerCase();
  if (sub === undefined) return needsApproval("git needs a read-only subcommand to run without asking.");
  if (!GIT_SUBCOMMANDS.has(sub)) return needsApproval(`git ${args[0]} is not a read-only git command.`);
  const rest = args.slice(1);
  for (const arg of rest) {
    // PowerShell 5.1 builds the native command line itself and its quoting of embedded quotes and spaces is not reliable,
    // so an argument that could be re-split into several is checked piece by piece, and quotes need approval.
    if (arg.includes('"')) return needsApproval("git arguments containing quotes need approval.");
    for (const piece of [arg, ...arg.split(/\s+/)]) {
      if (piece === "-c" || piece === "-C") return needsApproval(`git option ${piece} is not allowed without asking.`);
      if (!piece.startsWith("--")) continue;
      const name = piece.split("=")[0]!.toLowerCase();
      if (name.length <= 2) continue; // `--` ends the options.
      // git accepts any unambiguous abbreviation of a long option, so a prefix of a forbidden option is forbidden too.
      const hit = GIT_FORBIDDEN_LONG.find((forbidden) => name.startsWith(forbidden) || forbidden.startsWith(name));
      if (hit) return needsApproval(`git option ${piece} is not allowed without asking (${hit} can write files or run programs).`);
    }
  }
  if (sub === "branch") {
    const listing = rest.some((arg) => arg === "--list" || arg === "-l");
    const safe = rest.every((arg) => GIT_BRANCH_FLAGS.has(arg) || (listing && !arg.startsWith("-")));
    if (!safe) return needsApproval("git branch with these arguments can create, rename or delete branches.");
  }
  if (sub === "remote") {
    const listing = rest.length === 0 || (rest.length === 1 && (rest[0] === "-v" || rest[0] === "--verbose"));
    const url = rest.length === 2 && rest[0] === "get-url" && !rest[1]!.startsWith("-");
    if (!listing && !url) return needsApproval("git remote with these arguments can change or contact remotes.");
  }
  return readOnly("git read-only command");
}

/** Applies the allowlist, the alias map and the git rules to a parsed command. Pure. */
export function decideReadOnly(statements: PsStatement[]): Classification {
  if (statements.length === 0 || statements.some((statement) => statement.length === 0)) return needsApproval("The command is empty.");
  for (const statement of statements) {
    for (const { command, args } of statement) {
      const name = command.toLowerCase();
      const canonical = ALIASES[name] ?? name;
      if (args.some((arg) => NETWORK_PATH.test(arg))) return needsApproval(`${command} with a network path needs approval.`);
      if (name === "git") {
        const verdict = checkGit(args);
        if (!verdict.readOnly) return verdict;
        continue;
      }
      if (!CMDLETS.has(canonical) && !NATIVE.has(name)) return needsApproval(`${command} is not on the read-only list.`);
      if (name === "hostname" && args.length > 0) return needsApproval("hostname with arguments needs approval.");
      if (CMDLETS.has(canonical) && args.some(targetsAnotherComputer)) return needsApproval(`${command} pointed at another computer needs approval.`);
    }
  }
  return readOnly("Only read-only commands.");
}

// Classifier ---------------------------------------------------------------------------------------------------

export type ClassifierOptions = {
  spawn?: ToolSpawn;
  /** The powershell.exe to run; the System32 one by default. */
  executable?: string;
  env?: NodeJS.ProcessEnv;
  platform?: NodeJS.Platform;
  timers?: ToolTimers;
  killTree?: KillTree;
  timeoutMs?: number;
  maxOutputBytes?: number;
  maxCommandChars?: number;
};

/** Builds `classifyPowerShell`. It never throws and never rejects: every failure is `readOnly: false`. */
export function createClassifier(options: ClassifierOptions = {}): (command: unknown) => Promise<Classification> {
  const spawn = options.spawn ?? defaultSpawn;
  const env = options.env ?? process.env;
  const platform = options.platform ?? process.platform;
  const timers = options.timers ?? defaultTimers;
  const killTree = options.killTree ?? createKillTree(spawn, env, platform);
  const executable = options.executable ?? powershellExecutable(env, platform);
  const timeoutMs = options.timeoutMs ?? DEFAULT_LIMITS.classifyTimeoutMs;
  const maxOutputBytes = options.maxOutputBytes ?? DEFAULT_LIMITS.streamBytes;
  const maxCommandChars = options.maxCommandChars ?? DEFAULT_LIMITS.maxCommandChars;
  const script = encodePowerShell(CLASSIFIER_SCRIPT);

  return async (command) => {
    if (typeof command !== "string" || command.trim().length === 0) return needsApproval("The command is empty.");
    if (command.includes("\u0000")) return needsApproval("The command contains an invalid character.");
    if (command.length > maxCommandChars) return needsApproval("The command is too long to check.");
    try {
      const outcome = await runProcess({
        spawn,
        executable,
        // -InputFormat None keeps PowerShell's host from consuming stdin, which carries the command.
        args: ["-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-InputFormat", "None", "-EncodedCommand", script],
        options: { env, windowsHide: true, shell: false, stdio: ["pipe", "pipe", "pipe"] },
        stdin: Buffer.from(command, "utf8").toString("base64"),
        timeoutMs,
        maxStreamBytes: maxOutputBytes,
        timers,
        killTree,
      });
      if (outcome.timedOut) return needsApproval("The read-only check timed out.");
      if (outcome.startFailed) return needsApproval("The read-only check could not start.");
      if (outcome.exitCode !== 0) return needsApproval("The read-only check failed.");
      if (outcome.truncated) return needsApproval("The read-only check returned too much data.");
      const parsed = parseClassifierOutput(outcome.stdout.trim());
      if (!parsed.ok) return needsApproval(parsed.reason);
      return decideReadOnly(parsed.statements);
    } catch {
      return needsApproval("The read-only check failed.");
    }
  };
}
