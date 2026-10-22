import { spawn as nodeSpawn } from "node:child_process";
import { join } from "node:path";
import { encodeCommand, type OcrSpawn } from "./ocr.mts";

// Voice input. Windows only, through Windows PowerShell 5.1 and the speech recognition Windows itself provides, started
// on demand and killed when the user stops or after a fixed time. The helper listens on the default microphone, prints
// ONE JSON line with the transcript (or a short error code) and exits. No audio is ever written to disk, printed or
// sent by Dot: the microphone is opened by the helper process, so the page's web permissions stay denied.
//
// Engine choice (checked on Windows 11 26100 with a Spanish (Mexico) profile):
// - System.Speech (SAPI, `SpeechRecognitionEngine` + `DictationGrammar`) recognizes fully on the device. But it only
//   sees the classic desktop recognizers, and a stock Windows 11 install has none (InstalledRecognizers() was empty).
// - Windows.Media.SpeechRecognition (`SpeechRecognizer`) uses the speech language packs the user already has (the
//   probe listed es-MX) and works from an unpackaged process. Its free-text dictation grammar is a web service per
//   Microsoft's docs, so it needs "Online speech recognition" (Settings > Privacy > Speech) and Windows sends the audio
//   to Microsoft for that recognition; Dot itself still never stores or forwards it.
// The script collects the candidates from both, ranks them by the preferred cultures and picks the best, preferring the
// on-device System.Speech engine on a tie. If nothing matches it fails with `no-recognizer`.

/** Name shared with lib/voice-controller.ts, which reads globalThis[Symbol.for(VOICE_CONTROLLER_KEY)]. */
export const VOICE_CONTROLLER_KEY = "click-dot.voice-controller";

export const SPEECH_MAX_LISTEN_SECONDS = 20;
export const SPEECH_TIMEOUT_MS = 30_000;
export const SPEECH_MAX_OUTPUT_BYTES = 16 * 1024;
export const SPEECH_MAX_TEXT_CHARS = 2_000;

/** Codes the helper itself may print. `busy` and `cancelled` only ever come from the controller. */
export const HELPER_CODES = ["unavailable", "no-recognizer", "no-microphone", "mic-denied", "privacy", "no-speech", "timeout", "failed"] as const;
export type SpeechCode = (typeof HELPER_CODES)[number] | "busy" | "cancelled";

export class SpeechError extends Error {
  readonly code: SpeechCode;

  constructor(code: SpeechCode, message = "Voice input failed.") {
    super(message);
    this.code = code;
  }
}

export type VoiceController = {
  listen(): Promise<{ text: string }>;
  cancel(): void;
};

// PowerShell's generic type names use a backtick, which cannot appear inside the template literal below.
const BACKTICK = String.fromCharCode(96);

/**
 * The helper script. stdin carries one line of comma-separated culture tags in order of preference. Output is one
 * ASCII-only JSON line: { text } or { code, error }. Handled outcomes exit 0; anything unexpected exits non-zero and
 * the caller treats it as a failure. Recognizer text is never logged; nothing is written to disk.
 */
export const SPEECH_SCRIPT = String.raw`$ErrorActionPreference = 'Stop'
function Emit($object) {
  $json = ConvertTo-Json -InputObject $object -Compress
  # ASCII-only output, so the console code page can never change the text.
  $json = [regex]::Replace($json, '[^\x00-\x7F]', { param($m) '\u{0:x4}' -f [int][char]$m.Value })
  [Console]::Out.Write($json)
}
function Fail($code, $message) { Emit ([ordered]@{ code = $code; error = $message }); exit 0 }
function Codes($exception) {
  $list = New-Object System.Collections.Generic.List[int]
  while ($null -ne $exception) { $list.Add([int]$exception.HResult); $exception = $exception.InnerException }
  return $list
}
function FailFor($exception) {
  $codes = Codes $exception
  if ($codes -contains -2147199735) { Fail 'privacy' 'Online speech recognition is off.' }
  if ($codes -contains -2147024891) { Fail 'mic-denied' 'Microphone access is denied.' }
  if ($codes -contains -1072845856) { Fail 'no-microphone' 'No microphone is available.' }
  Fail 'failed' 'Speech recognition failed.'
}
try {
  $reader = New-Object System.IO.StreamReader([Console]::OpenStandardInput())
  $line = $reader.ReadToEnd().Trim()
  $tags = @($line -split ',' | Where-Object { $_.Length -le 35 -and $_ -match '^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$' })
  if ($tags.Count -eq 0) { Fail 'failed' 'No language preference.' }

  # Lower is better. An exact tag wins; a tag of the same language ranks half a step behind the first preference.
  function Rank($name) {
    for ($i = 0; $i -lt $tags.Count; $i++) { if ($tags[$i] -ieq $name) { return [double]$i } }
    $language = $name.Split('-')[0]
    for ($i = 0; $i -lt $tags.Count; $i++) { if ($tags[$i].Split('-')[0] -ieq $language) { return [double]$i + 0.5 } }
    return $null
  }

  $best = $null
  try {
    Add-Type -AssemblyName System.Speech
    foreach ($info in [System.Speech.Recognition.SpeechRecognitionEngine]::InstalledRecognizers()) {
      $rank = Rank $info.Culture.Name
      if ($null -ne $rank -and ($null -eq $best -or $rank -lt $best.Rank)) { $best = @{ Engine = 'system'; Rank = $rank; Info = $info } }
    }
  } catch { }
  try {
    Add-Type -AssemblyName System.Runtime.WindowsRuntime
    $null = [Windows.Media.SpeechRecognition.SpeechRecognizer, Windows.Media.SpeechRecognition, ContentType = WindowsRuntime]
    $null = [Windows.Media.SpeechRecognition.SpeechRecognitionResult, Windows.Media.SpeechRecognition, ContentType = WindowsRuntime]
    $null = [Windows.Media.SpeechRecognition.SpeechRecognitionCompilationResult, Windows.Media.SpeechRecognition, ContentType = WindowsRuntime]
    foreach ($language in [Windows.Media.SpeechRecognition.SpeechRecognizer]::SupportedTopicLanguages) {
      $rank = Rank $language.LanguageTag
      if ($null -ne $rank -and ($null -eq $best -or $rank -lt $best.Rank)) { $best = @{ Engine = 'winrt'; Rank = $rank; Language = $language } }
    }
  } catch { }
  if ($null -eq $best) { Fail 'no-recognizer' 'No speech recognizer is installed for the preferred languages.' }

  # --- listen ---
  $text = $null
  if ($best.Engine -eq 'system') {
    $engine = New-Object System.Speech.Recognition.SpeechRecognitionEngine($best.Info)
    try {
      $engine.LoadGrammar((New-Object System.Speech.Recognition.DictationGrammar))
      try { $engine.SetInputToDefaultAudioDevice() } catch { Fail 'no-microphone' 'No microphone is available.' }
      $engine.InitialSilenceTimeout = [TimeSpan]::FromSeconds(6)
      $engine.EndSilenceTimeout = [TimeSpan]::FromSeconds(1.2)
      $engine.EndSilenceTimeoutAmbiguous = [TimeSpan]::FromSeconds(1.5)
      $result = $engine.Recognize([TimeSpan]::FromSeconds(${SPEECH_MAX_LISTEN_SECONDS}))
      if ($null -ne $result) { $text = [string]$result.Text }
    } finally { $engine.Dispose() }
  } else {
    $asTask = ([System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
      $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation${BACKTICK}1'
    })[0]
    function Await($operation, $resultType) {
      $task = $asTask.MakeGenericMethod($resultType).Invoke($null, @($operation))
      $null = $task.Wait(-1)
      return $task.Result
    }
    $recognizer = $null
    try {
      $recognizer = New-Object Windows.Media.SpeechRecognition.SpeechRecognizer($best.Language)
      $recognizer.Timeouts.InitialSilenceTimeout = [TimeSpan]::FromSeconds(6)
      $recognizer.Timeouts.EndSilenceTimeout = [TimeSpan]::FromSeconds(1.2)
      $compiled = Await ($recognizer.CompileConstraintsAsync()) ([Windows.Media.SpeechRecognition.SpeechRecognitionCompilationResult])
      if ([string]$compiled.Status -ne 'Success') { Fail 'failed' 'Speech recognition could not be prepared.' }
      $result = Await ($recognizer.RecognizeAsync()) ([Windows.Media.SpeechRecognition.SpeechRecognitionResult])
      $status = [string]$result.Status
      if ($status -eq 'MicrophoneUnavailable') { Fail 'no-microphone' 'No microphone is available.' }
      if ($status -eq 'TimeoutExceeded') { Fail 'no-speech' 'No speech was heard.' }
      if ($status -ne 'Success') { Fail 'failed' 'Speech recognition failed.' }
      $text = [string]$result.Text
    } catch {
      FailFor $_.Exception
    } finally {
      if ($null -ne $recognizer) { $recognizer.Dispose() }
    }
  }
  if ([string]::IsNullOrWhiteSpace($text)) { Fail 'no-speech' 'No speech was heard.' }
  Emit ([ordered]@{ text = $text })
} catch {
  [Console]::Error.Write('speech-failed')
  exit 1
}
`;

const TAG = /^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$/;
const DEFAULT_CULTURES = ["es-MX", "es", "es-ES", "en-US", "en"];

/** The user's UI culture first, then its language, then Spanish, then English. Anything that is not a plain tag is dropped. */
export function preferredCultureTags(uiLocale?: unknown): string[] {
  const tags: string[] = [];
  const add = (tag: string) => { if (!tags.includes(tag)) tags.push(tag); };
  if (typeof uiLocale === "string" && uiLocale.length <= 35 && TAG.test(uiLocale)) {
    add(uiLocale);
    add(uiLocale.split("-", 1)[0]);
  }
  for (const tag of DEFAULT_CULTURES) add(tag);
  return tags;
}

const isHelperCode = (value: unknown): value is (typeof HELPER_CODES)[number] => HELPER_CODES.some((code) => code === value);

/** Strict validation of the helper's stdout: one line, exactly `{ text }` or `{ code, error }`. Throws a SpeechError otherwise. */
export function parseSpeechOutput(stdout: string): { text: string } {
  const lines = stdout.split(/\r?\n/).filter((line) => line.trim().length > 0);
  let parsed: unknown;
  try {
    if (lines.length !== 1) throw new Error("expected one line");
    parsed = JSON.parse(lines[0]);
  } catch {
    throw new SpeechError("failed", "The speech helper returned unreadable output.");
  }
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) throw new SpeechError("failed", "The speech helper returned an unexpected result.");
  const keys = Object.keys(parsed).sort().join(",");
  const { text, code, error } = parsed as Record<string, unknown>;
  if (keys === "text" && typeof text === "string") {
    if (text.length > SPEECH_MAX_TEXT_CHARS) throw new SpeechError("failed", "The speech helper returned too much text.");
    if (text.trim().length === 0) throw new SpeechError("no-speech", "No speech was heard.");
    return { text };
  }
  if (keys === "code,error" && typeof error === "string") {
    // The helper's own wording is dropped on purpose: only the code is kept.
    throw new SpeechError(isHelperCode(code) ? code : "failed");
  }
  throw new SpeechError("failed", "The speech helper returned an unexpected result.");
}

export function powershellPath(env: NodeJS.ProcessEnv = process.env, platform: NodeJS.Platform = process.platform) {
  if (platform !== "win32") throw new SpeechError("unavailable", "Voice input is only available on Windows.");
  return join(env.SystemRoot ?? env.windir ?? "C:\\Windows", "System32", "WindowsPowerShell", "v1.0", "powershell.exe");
}

const defaultSpawn: OcrSpawn = (executable, args) =>
  nodeSpawn(executable, args, { windowsHide: true, stdio: ["pipe", "pipe", "pipe"] }) as unknown as ReturnType<OcrSpawn>;

type Run = { abort(error: SpeechError): void };

/**
 * Builds the voice controller. One helper runs at a time: a second `listen` while one is running is rejected with
 * `busy`, and `cancel` kills the running helper (its `listen` rejects with `cancelled`). `spawn`, `executable` and
 * `locale` are injectable for tests.
 */
export function createWindowsSpeech({
  spawn = defaultSpawn,
  executable,
  locale = Intl.DateTimeFormat().resolvedOptions().locale,
  timeoutMs = SPEECH_TIMEOUT_MS,
  maxOutputBytes = SPEECH_MAX_OUTPUT_BYTES,
}: { spawn?: OcrSpawn; executable?: string; locale?: string; timeoutMs?: number; maxOutputBytes?: number } = {}): VoiceController {
  let active: Run | undefined;

  return {
    listen() {
      if (active) return Promise.reject(new SpeechError("busy", "Voice input is already listening."));
      let program: string;
      try {
        program = executable ?? powershellPath();
      } catch (error) {
        return Promise.reject(error);
      }
      return new Promise<{ text: string }>((resolve, reject) => {
        let child: ReturnType<OcrSpawn>;
        try {
          // -InputFormat None keeps PowerShell's host from consuming stdin, which carries the language preferences.
          child = spawn(program, ["-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-InputFormat", "None", "-EncodedCommand", encodeCommand(SPEECH_SCRIPT)]);
        } catch {
          reject(new SpeechError("unavailable", "The speech helper could not be started."));
          return;
        }
        const chunks: Buffer[] = [];
        let size = 0;
        let settled = false;
        const run: Run = { abort: (error) => finish(() => {
          try { child.kill(); } catch { /* The process is already gone. */ }
          reject(error);
        }) };
        const finish = (action: () => void) => {
          if (settled) return;
          settled = true;
          clearTimeout(timer);
          if (active === run) active = undefined;
          action();
        };
        const timer = setTimeout(() => run.abort(new SpeechError("timeout", "Listening took too long.")), timeoutMs);
        active = run;
        child.stdout.on("data", (chunk) => {
          size += chunk.length;
          if (size > maxOutputBytes) {
            run.abort(new SpeechError("failed", "The speech helper returned too much data."));
            return;
          }
          chunks.push(chunk);
        });
        // stderr is drained and discarded; it is never read, logged or returned.
        child.stderr.on("data", () => {});
        child.on("error", () => run.abort(new SpeechError("unavailable", "The speech helper could not be started.")));
        child.stdin.on("error", () => {});
        child.on("close", (code) => finish(() => {
          if (code !== 0) {
            reject(new SpeechError("failed", "The speech helper failed."));
            return;
          }
          try {
            resolve(parseSpeechOutput(Buffer.concat(chunks).toString("utf8")));
          } catch (error) {
            reject(error);
          }
        }));
        try {
          child.stdin.write(`${preferredCultureTags(locale).join(",")}\n`);
          child.stdin.end();
        } catch {
          run.abort(new SpeechError("failed", "The speech helper failed."));
        }
      });
    },
    cancel() {
      active?.abort(new SpeechError("cancelled", "Listening was cancelled."));
    },
  };
}

/**
 * Publishes the voice controller for the Next.js server code running in this same process. Windows only: elsewhere
 * nothing is published and the voice routes answer "unavailable". Returns whether it was installed.
 */
export function installVoiceController(
  target: object,
  controller: VoiceController = createWindowsSpeech(),
  platform: NodeJS.Platform = process.platform,
) {
  if (platform !== "win32") return false;
  Reflect.set(target, Symbol.for(VOICE_CONTROLLER_KEY), controller);
  return true;
}
