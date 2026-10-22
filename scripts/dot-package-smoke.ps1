# Disposable-runner smoke for the packaged Click Dot (click-dot_<version>_windows_amd64.zip).
#
# It extracts the zip, launches the real packaged `Click Dot.exe` with start-at-login disabled and an
# isolated user-data dir, and proves it serves locally, reports no connected account, and does not
# write into its own install folder.
#
# It also proves the page's JavaScript runs inside the packaged app: the exe is launched with Chromium's
# --remote-debugging-port, and the smoke drives the mascot page over the Chrome DevTools Protocol using only
# built-in .NET (ClientWebSocket). An HTML-only check cannot catch a page that never hydrates (v0.9.0 shipped
# one: the page sat on "Checking this device..." because it was prerendered with the wrong origin). The port is
# bound to loopback on a disposable runner only.
#
# Scope (A7): GitHub-hosted Windows runners do NOT enforce Smart App Control. This smoke proves
# runtime behavior (Electron fuses and asar loading, safeStorage/DPAPI availability, Next serving on
# 127.0.0.1:47321) but NOT what an end user with Smart App Control enabled sees. Dot ships unsigned;
# that risk is a documented, accepted limitation and is not covered here.
param(
  [Parameter(Mandatory = $true)]
  [string]$ZipPath,
  [int]$TimeoutSeconds = 90,
  [Parameter(Mandatory = $true)]
  [switch]$DisposableRunner
)

$ErrorActionPreference = 'Stop'

# Shared guard: refuses anything but the disposable GitHub-hosted Windows smoke workflow. It runs
# before any path resolution, extraction, or process launch.
. (Join-Path $PSScriptRoot 'windows-runner-guard.ps1')
Assert-DisposableWindowsRunner -DisposableRunner ([bool]$DisposableRunner) -Label 'Click Dot package smoke'

$dotOrigin = 'http://127.0.0.1:47321'
$mascotUrl = "$dotOrigin/mascot"
# Fixed high loopback port for Chromium's remote debugging (DevTools protocol over HTTP and WebSocket).
$debugPort = 9339
$cdpTimeoutSeconds = 30
$menuLabels = @('Compartir mi pantalla', 'Voz', 'Mensaje')
$messageLabel = 'Mensaje'
$connectLabel = 'Conectar con ChatGPT'
$script:cdpMessageId = 0

function Get-TreeSnapshot {
  param([string]$Root)
  $rootFull = (Resolve-Path -LiteralPath $Root).Path.TrimEnd('\')
  $snapshot = [ordered]@{}
  Get-ChildItem -LiteralPath $rootFull -Recurse -Force | Sort-Object FullName | ForEach-Object {
    $relative = $_.FullName.Substring($rootFull.Length + 1)
    if ($_.PSIsContainer) {
      $snapshot[$relative] = 'dir'
    } else {
      $snapshot[$relative] = $_.Length.ToString() + ':' + (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash
    }
  }
  return $snapshot
}

function Compare-TreeSnapshot {
  param($Before, $After)
  $problems = @()
  foreach ($key in $After.Keys) {
    if (-not $Before.Contains($key)) { $problems += "new: $key" }
    elseif ($Before[$key] -ne $After[$key]) { $problems += "changed: $key" }
  }
  foreach ($key in $Before.Keys) {
    if (-not $After.Contains($key)) { $problems += "removed: $key" }
  }
  return $problems
}

function Invoke-DotRequest {
  param([string]$Path)
  try {
    return Invoke-WebRequest -Uri ($dotOrigin + $Path) -UseBasicParsing -TimeoutSec 5
  } catch {
    return $null
  }
}

function Assert-DebugPortFree {
  param([int]$Port)
  $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, $Port)
  try {
    $listener.Start()
  } catch {
    throw "Loopback port $Port is already in use; the smoke needs it for --remote-debugging-port."
  } finally {
    $listener.Stop()
  }
}

# Finds the DevTools page target whose URL is the mascot page. Returns $null until it exists.
function Get-MascotTarget {
  param([int]$Port, [string]$PageUrl)
  try {
    $targets = @(Invoke-RestMethod -Uri "http://127.0.0.1:$Port/json" -TimeoutSec 5)
  } catch {
    return $null
  }
  # Windows PowerShell 5.1 returns the JSON array as one object while PowerShell 7 enumerates it; handle both.
  foreach ($entry in $targets) {
    foreach ($target in @($entry)) {
      if ($target.type -eq 'page' -and $target.url -like "$PageUrl*" -and $target.webSocketDebuggerUrl) { return $target }
    }
  }
  return $null
}

# Sends one CDP Runtime.evaluate and returns the evaluated value (returnByValue).
function Invoke-CdpEvaluate {
  param([System.Net.WebSockets.ClientWebSocket]$Socket, [string]$Expression)
  $script:cdpMessageId++
  $id = $script:cdpMessageId
  $payload = @{ id = $id; method = 'Runtime.evaluate'; params = @{ expression = $Expression; returnByValue = $true } } |
    ConvertTo-Json -Depth 5 -Compress
  $cts = [System.Threading.CancellationTokenSource]::new(10000)
  try {
    $bytes = [System.Text.Encoding]::UTF8.GetBytes($payload)
    $null = $Socket.SendAsync([System.ArraySegment[byte]]::new($bytes), [System.Net.WebSockets.WebSocketMessageType]::Text, $true, $cts.Token).GetAwaiter().GetResult()
    while ($true) {
      $buffer = New-Object byte[] 65536
      $stream = [System.IO.MemoryStream]::new()
      do {
        $received = $Socket.ReceiveAsync([System.ArraySegment[byte]]::new($buffer), $cts.Token).GetAwaiter().GetResult()
        if ($received.MessageType -eq [System.Net.WebSockets.WebSocketMessageType]::Close) {
          throw 'The DevTools WebSocket was closed by the app.'
        }
        $stream.Write($buffer, 0, $received.Count)
      } while (-not $received.EndOfMessage)
      $message = [System.Text.Encoding]::UTF8.GetString($stream.ToArray()) | ConvertFrom-Json
      if ($message.id -ne $id) { continue }
      if ($message.error) { throw "Runtime.evaluate failed: $($message.error.message)" }
      if ($message.result.exceptionDetails) { throw "Page script threw: $($message.result.exceptionDetails.text) $($message.result.exceptionDetails.exception.description)" }
      return $message.result.result.value
    }
  } finally {
    $cts.Dispose()
  }
}

# Re-evaluates the expression about once a second until the page text contains every required string.
# Fails with the last text the page showed, so a page stuck on a loading state is visible in the log.
function Wait-PageText {
  param(
    [System.Net.WebSockets.ClientWebSocket]$Socket,
    [string]$Expression,
    [string[]]$Required,
    [string]$Description
  )
  $lastText = '<no text evaluated>'
  $deadline = [DateTime]::UtcNow.AddSeconds($cdpTimeoutSeconds)
  while ([DateTime]::UtcNow -lt $deadline) {
    $value = Invoke-CdpEvaluate -Socket $Socket -Expression $Expression
    if ($null -ne $value -and $null -ne $value.text) { $lastText = [string]$value.text }
    $missing = @($Required | Where-Object { -not $lastText.Contains($_) })
    if ($missing.Count -eq 0) { return $lastText }
    Start-Sleep -Seconds 1
  }
  throw ("$Description did not happen within $cdpTimeoutSeconds s; the page's JavaScript did not produce: " +
    (($Required | ForEach-Object { "'$_'" }) -join ', ') + ". Evaluated page text:`n$lastText")
}

function Show-ProcessOutput {
  param([string]$StdoutPath, [string]$StderrPath)
  foreach ($pair in @(@('stdout', $StdoutPath), @('stderr', $StderrPath))) {
    [Console]::Error.WriteLine("--- Click Dot $($pair[0]) ---")
    if (Test-Path -LiteralPath $pair[1]) {
      [Console]::Error.WriteLine((Get-Content -LiteralPath $pair[1] -Raw))
    }
  }
}

$smokeRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("click-dot-smoke-" + [System.Guid]::NewGuid().ToString('N'))
$appRoot = Join-Path $smokeRoot 'app'
$userData = Join-Path $smokeRoot 'userdata'
$stdoutLog = Join-Path $smokeRoot 'dot-stdout.log'
$stderrLog = Join-Path $smokeRoot 'dot-stderr.log'
$originalDisableLoginItem = [System.Environment]::GetEnvironmentVariable('CLICK_DOT_DISABLE_LOGIN_ITEM', 'Process')
$dotProcess = $null
$cdpSocket = $null
$failed = $false

try {
  $zip = (Resolve-Path -LiteralPath $ZipPath).Path
  New-Item -ItemType Directory -Path $appRoot, $userData | Out-Null
  Expand-Archive -LiteralPath $zip -DestinationPath $appRoot

  $dotExe = Join-Path $appRoot 'Click Dot.exe'
  if (-not (Test-Path -LiteralPath $dotExe)) {
    throw "Click Dot.exe was not extracted to $dotExe"
  }

  if ($null -ne (Invoke-DotRequest -Path '/')) {
    throw "Something is already serving $dotOrigin before the smoke started; refusing to test the wrong process."
  }

  Assert-DebugPortFree -Port $debugPort

  $before = Get-TreeSnapshot -Root $appRoot
  Write-Host "Extracted $($before.Count) entries; launching Click Dot."

  # Never register a login item: both the flag and the environment switch disable it.
  $env:CLICK_DOT_DISABLE_LOGIN_ITEM = '1'
  $dotProcess = Start-Process -FilePath $dotExe `
    -ArgumentList @('--no-login-item', "--user-data-dir=$userData", "--remote-debugging-port=$debugPort") `
    -WorkingDirectory $appRoot `
    -RedirectStandardOutput $stdoutLog -RedirectStandardError $stderrLog `
    -PassThru

  $homePage = $null
  $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
  while ([DateTime]::UtcNow -lt $deadline) {
    if ($dotProcess.HasExited) {
      throw "Click Dot exited early with code $($dotProcess.ExitCode) before serving $dotOrigin/."
    }
    $response = Invoke-DotRequest -Path '/'
    if ($null -ne $response -and $response.StatusCode -eq 200) { $homePage = $response; break }
    Start-Sleep -Seconds 1
  }
  if ($null -eq $homePage) {
    throw "Timed out after $TimeoutSeconds s waiting for $dotOrigin/ to return 200."
  }
  if (-not $homePage.Content.Contains('Click Dot')) {
    throw "$dotOrigin/ returned 200 but the body does not contain 'Click Dot'."
  }
  # The page redirects to its canonical origin on load; a stale origin (for example a build-time default port)
  # is blocked by the app's navigation guard and leaves the page stuck before sign-in. The RSC payload escapes
  # quotes, so match both the escaped and the plain JSON form.
  $escapedOrigin = 'canonicalOrigin\":\"' + $dotOrigin + '\"'
  $plainOrigin = 'canonicalOrigin":"' + $dotOrigin + '"'
  if (-not ($homePage.Content.Contains($escapedOrigin) -or $homePage.Content.Contains($plainOrigin))) {
    throw "The home page does not use $dotOrigin as its canonical origin; the app would redirect to a blocked origin."
  }


  $status = Invoke-DotRequest -Path '/api/auth/status'
  if ($null -eq $status -or $status.StatusCode -ne 200) {
    throw "$dotOrigin/api/auth/status did not return 200 (safeStorage or the local account store may be unavailable)."
  }
  $statusBody = $status.Content | ConvertFrom-Json
  if ($statusBody.connected -ne $false) {
    throw "/api/auth/status reported connected=$($statusBody.connected); a fresh user-data dir must be disconnected. Body: $($status.Content)"
  }

  # Prove the page's JavaScript runs: find the mascot page over DevTools, open its menu, then open the chat.
  $target = $null
  $targetDeadline = [DateTime]::UtcNow.AddSeconds($cdpTimeoutSeconds)
  while ([DateTime]::UtcNow -lt $targetDeadline) {
    if ($dotProcess.HasExited) {
      throw "Click Dot exited early with code $($dotProcess.ExitCode) before the mascot page was inspected."
    }
    $target = Get-MascotTarget -Port $debugPort -PageUrl $mascotUrl
    if ($null -ne $target) { break }
    Start-Sleep -Seconds 1
  }
  if ($null -eq $target) {
    throw "No DevTools page target for $mascotUrl appeared on port $debugPort within $cdpTimeoutSeconds s (is --remote-debugging-port honored by the packaged app?)."
  }
  $cdpSocket = [System.Net.WebSockets.ClientWebSocket]::new()
  $connectCts = [System.Threading.CancellationTokenSource]::new(10000)
  try {
    $null = $cdpSocket.ConnectAsync([System.Uri]$target.webSocketDebuggerUrl, $connectCts.Token).GetAwaiter().GetResult()
  } finally {
    $connectCts.Dispose()
  }

  # The mascot is a pointer-driven role=button; Enter on it is the keyboard path to the same expand action.
  # The menu only renders once React has hydrated, so the three labels prove the page JavaScript ran.
  $expandExpression = @'
(() => {
  const text = document.body ? document.body.innerText : '';
  const toggle = document.querySelector('.mascot-root [role="button"][aria-expanded="false"]');
  if (toggle) toggle.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }));
  return { text: text };
})()
'@
  $null = Wait-PageText -Socket $cdpSocket -Expression $expandExpression -Required $menuLabels -Description 'Opening the mascot menu'
  Write-Host "Mascot menu rendered: $($menuLabels -join ', ')."

  # A fresh profile is not connected, so the chat bubble must offer to connect (not stay on "Comprobando la conexion").
  $messageExpression = @'
(() => {
  const text = document.body ? document.body.innerText : '';
  const button = Array.from(document.querySelectorAll('.mascot-menu button')).find((b) => b.textContent.trim() === '__LABEL__');
  if (button) button.click();
  return { text: text };
})()
'@
  $messageExpression = $messageExpression.Replace('__LABEL__', $messageLabel)
  $null = Wait-PageText -Socket $cdpSocket -Expression $messageExpression -Required @($connectLabel) -Description "Opening the chat with '$messageLabel'"
  Write-Host "Chat opened and offered '$connectLabel'."

  if (-not (Test-Path -LiteralPath $userData -PathType Container)) {
    throw "The user-data dir $userData does not exist."
  }
  if (@(Get-ChildItem -LiteralPath $userData -Force).Count -eq 0) {
    throw "Click Dot never wrote to the isolated user-data dir $userData."
  }

  $after = Get-TreeSnapshot -Root $appRoot
  $problems = @(Compare-TreeSnapshot -Before $before -After $after)
  if ($problems.Count -gt 0) {
    throw ("The extracted app folder changed while Click Dot ran:`n" + ($problems -join "`n"))
  }

  Write-Host 'Click Dot package smoke passed: served 200 with "Click Dot", page JavaScript ran (menu and chat), disconnected, app folder untouched.'
} catch {
  $failed = $true
  [Console]::Error.WriteLine("Click Dot package smoke failed: $($_.Exception.Message)")
  throw
} finally {
  if ($null -ne $cdpSocket) { $cdpSocket.Dispose() }
  if ($null -ne $dotProcess) {
    # Stop the whole Electron process tree (main, renderer, GPU, utility processes).
    & taskkill.exe /PID $dotProcess.Id /T /F 2>&1 | Out-Null
    Start-Sleep -Seconds 2
  }
  if ($failed) { Show-ProcessOutput -StdoutPath $stdoutLog -StderrPath $stderrLog }
  [System.Environment]::SetEnvironmentVariable('CLICK_DOT_DISABLE_LOGIN_ITEM', $originalDisableLoginItem, 'Process')
  Remove-Item -LiteralPath $smokeRoot -Recurse -Force -ErrorAction SilentlyContinue
}
