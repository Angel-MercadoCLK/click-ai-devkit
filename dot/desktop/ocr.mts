import { spawn as nodeSpawn } from "node:child_process";
import { join } from "node:path";
import type { OcrWord } from "./redaction.mts";

// Local OCR. Windows only: Windows.Media.Ocr through Windows PowerShell 5.1. The PNG is streamed to the helper over
// stdin and decoded from memory there, so no frame is ever written to a temporary file. One helper process runs per
// frame. Every failure (no engine or language, timeout, crash, malformed or oversized output) throws, and the caller
// must treat a throw as "send no image".

export type OcrFn = (png: Buffer) => Promise<OcrWord[]>;

export const OCR_TIMEOUT_MS = 15_000;
export const OCR_MAX_INPUT_BYTES = 24 * 1024 * 1024;
export const OCR_MAX_OUTPUT_BYTES = 4 * 1024 * 1024;
export const OCR_MAX_WORDS = 20_000;
const MAX_WORD_CHARS = 200;
const MAX_COORDINATE = 100_000;

export class OcrError extends Error {
  readonly reason: "unavailable" | "timeout" | "failed" | "invalid-output";

  constructor(reason: OcrError["reason"], message: string) {
    super(message);
    this.reason = reason;
  }
}

// PowerShell's generic type names use a backtick, which cannot appear inside the template literal below.
const BACKTICK = String.fromCharCode(96);

/**
 * The helper script. It reads the raw PNG bytes from stdin and prints a JSON array of
 * { text, x, y, w, h, line } in the original image's pixel coordinates (an image larger than the engine's
 * MaxImageDimension is downscaled for recognition and the boxes are scaled back). Output is ASCII-only JSON.
 * Failures print only a short code to stderr and exit non-zero; no image data or text is ever echoed.
 */
export const OCR_SCRIPT = String.raw`$ErrorActionPreference = 'Stop'
try {
  Add-Type -AssemblyName System.Runtime.WindowsRuntime
  $null = [Windows.Storage.Streams.InMemoryRandomAccessStream, Windows.Storage.Streams, ContentType = WindowsRuntime]
  $null = [Windows.Storage.Streams.DataWriter, Windows.Storage.Streams, ContentType = WindowsRuntime]
  $null = [Windows.Graphics.Imaging.BitmapDecoder, Windows.Graphics.Imaging, ContentType = WindowsRuntime]
  $null = [Windows.Graphics.Imaging.BitmapTransform, Windows.Graphics.Imaging, ContentType = WindowsRuntime]
  $null = [Windows.Graphics.Imaging.SoftwareBitmap, Windows.Graphics.Imaging, ContentType = WindowsRuntime]
  $null = [Windows.Media.Ocr.OcrEngine, Windows.Foundation, ContentType = WindowsRuntime]
  $null = [Windows.Globalization.Language, Windows.Globalization, ContentType = WindowsRuntime]

  $asTask = ([System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
    $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation${BACKTICK}1'
  })[0]
  function Await($operation, $resultType) {
    $task = $asTask.MakeGenericMethod($resultType).Invoke($null, @($operation))
    $null = $task.Wait(-1)
    return $task.Result
  }

  $engine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromUserProfileLanguages()
  if ($null -eq $engine) {
    foreach ($tag in @('es-MX', 'es-ES', 'es', 'en-US', 'en-GB', 'en')) {
      $language = New-Object Windows.Globalization.Language($tag)
      if ([Windows.Media.Ocr.OcrEngine]::IsLanguageSupported($language)) {
        $engine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromLanguage($language)
        if ($null -ne $engine) { break }
      }
    }
  }
  if ($null -eq $engine) { [Console]::Error.Write('no-ocr-language'); exit 2 }

  $stdin = [Console]::OpenStandardInput()
  $buffer = New-Object System.IO.MemoryStream
  $stdin.CopyTo($buffer)
  $bytes = $buffer.ToArray()
  if ($bytes.Length -eq 0) { [Console]::Error.Write('empty-input'); exit 3 }

  $stream = New-Object Windows.Storage.Streams.InMemoryRandomAccessStream
  $writer = New-Object Windows.Storage.Streams.DataWriter($stream)
  $writer.WriteBytes($bytes)
  $null = Await ($writer.StoreAsync()) ([uint32])
  $null = $writer.DetachStream()
  $stream.Seek(0)

  $decoder = Await ([Windows.Graphics.Imaging.BitmapDecoder]::CreateAsync($stream)) ([Windows.Graphics.Imaging.BitmapDecoder])
  $width = [int]$decoder.PixelWidth
  $height = [int]$decoder.PixelHeight
  $limit = [int][Windows.Media.Ocr.OcrEngine]::MaxImageDimension
  $scale = 1.0
  $transform = New-Object Windows.Graphics.Imaging.BitmapTransform
  if ($width -gt $limit -or $height -gt $limit) {
    $scale = [double]$limit / [double][Math]::Max($width, $height)
    $transform.ScaledWidth = [uint32][Math]::Max(1, [Math]::Floor($width * $scale))
    $transform.ScaledHeight = [uint32][Math]::Max(1, [Math]::Floor($height * $scale))
  }
  $bitmap = Await ($decoder.GetSoftwareBitmapAsync(
    [Windows.Graphics.Imaging.BitmapPixelFormat]::Bgra8,
    [Windows.Graphics.Imaging.BitmapAlphaMode]::Premultiplied,
    $transform,
    [Windows.Graphics.Imaging.ExifOrientationMode]::IgnoreExifOrientation,
    [Windows.Graphics.Imaging.ColorManagementMode]::DoNotColorManage)) ([Windows.Graphics.Imaging.SoftwareBitmap])
  $result = Await ($engine.RecognizeAsync($bitmap)) ([Windows.Media.Ocr.OcrResult])

  $words = New-Object System.Collections.Generic.List[object]
  $lineIndex = 0
  foreach ($line in $result.Lines) {
    foreach ($word in $line.Words) {
      $rect = $word.BoundingRect
      $words.Add([ordered]@{
        text = [string]$word.Text
        x = [int][Math]::Floor($rect.X / $scale)
        y = [int][Math]::Floor($rect.Y / $scale)
        w = [int][Math]::Ceiling($rect.Width / $scale)
        h = [int][Math]::Ceiling($rect.Height / $scale)
        line = $lineIndex
      })
    }
    $lineIndex++
  }
  $json = if ($words.Count -eq 0) { '[]' } else { ConvertTo-Json -InputObject $words.ToArray() -Compress }
  # ASCII-only output, so the console code page can never change the text.
  $json = [regex]::Replace($json, '[^\x00-\x7F]', { param($m) '\u{0:x4}' -f [int][char]$m.Value })
  [Console]::Out.Write($json)
} catch {
  [Console]::Error.Write('ocr-failed')
  exit 1
}
`;

/** Windows PowerShell expects UTF-16LE base64 for -EncodedCommand. */
export function encodeCommand(script: string) {
  return Buffer.from(script, "utf16le").toString("base64");
}

export function powershellPath(env: NodeJS.ProcessEnv = process.env, platform: NodeJS.Platform = process.platform) {
  if (platform !== "win32") throw new OcrError("unavailable", "OCR is only available on Windows.");
  return join(env.SystemRoot ?? env.windir ?? "C:\\Windows", "System32", "WindowsPowerShell", "v1.0", "powershell.exe");
}

/** The slice of a child process this module uses; Node's ChildProcess satisfies it. */
export type OcrChild = {
  stdin: { write(chunk: Buffer | string): unknown; end(): unknown; on(event: "error", listener: (error: Error) => void): unknown };
  stdout: { on(event: "data", listener: (chunk: Buffer) => void): unknown };
  stderr: { on(event: "data", listener: (chunk: Buffer) => void): unknown };
  on(event: "error", listener: (error: Error) => void): unknown;
  on(event: "close", listener: (code: number | null) => void): unknown;
  kill(): unknown;
};
export type OcrSpawn = (executable: string, args: string[]) => OcrChild;

const defaultSpawn: OcrSpawn = (executable, args) =>
  nodeSpawn(executable, args, { windowsHide: true, stdio: ["pipe", "pipe", "pipe"] }) as unknown as OcrChild;

const isInteger = (value: unknown, min: number, max: number): value is number =>
  typeof value === "number" && Number.isInteger(value) && value >= min && value <= max;

/** Strict validation of the helper's stdout. Throws on anything but a well-formed word list. */
export function parseOcrOutput(stdout: string): OcrWord[] {
  let parsed: unknown;
  try {
    parsed = JSON.parse(stdout);
  } catch {
    throw new OcrError("invalid-output", "The OCR helper returned unreadable output.");
  }
  if (!Array.isArray(parsed) || parsed.length > OCR_MAX_WORDS) throw new OcrError("invalid-output", "The OCR helper returned an unexpected result.");
  return parsed.map((item): OcrWord => {
    if (!item || typeof item !== "object" || Array.isArray(item)) throw new OcrError("invalid-output", "The OCR helper returned an unexpected result.");
    const keys = Object.keys(item);
    const { text, x, y, w, h, line } = item as Record<string, unknown>;
    if (keys.length !== 6 || typeof text !== "string" || text.length < 1 || text.length > MAX_WORD_CHARS
      || !isInteger(x, -MAX_COORDINATE, MAX_COORDINATE) || !isInteger(y, -MAX_COORDINATE, MAX_COORDINATE)
      || !isInteger(w, 1, MAX_COORDINATE) || !isInteger(h, 1, MAX_COORDINATE) || !isInteger(line, 0, OCR_MAX_WORDS)) {
      throw new OcrError("invalid-output", "The OCR helper returned an unexpected result.");
    }
    return { text, x, y, w, h, line };
  });
}

/** Builds an OCR function that runs the Windows helper once per call. `spawn` and `executable` are injectable for tests. */
export function createWindowsOcr({
  spawn = defaultSpawn,
  executable,
  timeoutMs = OCR_TIMEOUT_MS,
  maxOutputBytes = OCR_MAX_OUTPUT_BYTES,
}: { spawn?: OcrSpawn; executable?: string; timeoutMs?: number; maxOutputBytes?: number } = {}): OcrFn {
  return (png) => {
    if (!Buffer.isBuffer(png) || png.length === 0 || png.length > OCR_MAX_INPUT_BYTES) {
      return Promise.reject(new OcrError("failed", "The screen image cannot be read."));
    }
    let program: string;
    try {
      program = executable ?? powershellPath();
    } catch (error) {
      return Promise.reject(error);
    }
    return new Promise<OcrWord[]>((resolve, reject) => {
      let child: OcrChild;
      try {
        // -InputFormat None keeps PowerShell's host from consuming stdin, which carries the PNG.
        child = spawn(program, ["-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-InputFormat", "None", "-EncodedCommand", encodeCommand(OCR_SCRIPT)]);
      } catch {
        reject(new OcrError("unavailable", "The OCR helper could not be started."));
        return;
      }
      const chunks: Buffer[] = [];
      let size = 0;
      let settled = false;
      const finish = (action: () => void) => {
        if (settled) return;
        settled = true;
        clearTimeout(timer);
        action();
      };
      const abort = (error: OcrError) => finish(() => {
        try { child.kill(); } catch { /* The process is already gone. */ }
        reject(error);
      });
      const timer = setTimeout(() => abort(new OcrError("timeout", "OCR took too long.")), timeoutMs);
      child.stdout.on("data", (chunk) => {
        size += chunk.length;
        if (size > maxOutputBytes) {
          abort(new OcrError("invalid-output", "The OCR helper returned too much data."));
          return;
        }
        chunks.push(chunk);
      });
      // stderr is drained and discarded; it is never read, logged or returned.
      child.stderr.on("data", () => {});
      child.on("error", () => abort(new OcrError("unavailable", "The OCR helper could not be started.")));
      child.stdin.on("error", () => {});
      child.on("close", (code) => finish(() => {
        if (code === 2) {
          reject(new OcrError("unavailable", "No OCR language is installed on this computer."));
          return;
        }
        if (code !== 0) {
          reject(new OcrError("failed", "The OCR helper failed."));
          return;
        }
        try {
          resolve(parseOcrOutput(Buffer.concat(chunks).toString("utf8")));
        } catch (error) {
          reject(error);
        }
      }));
      try {
        child.stdin.write(png);
        child.stdin.end();
      } catch {
        abort(new OcrError("failed", "The OCR helper failed."));
      }
    });
  };
}
