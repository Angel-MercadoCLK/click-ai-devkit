# Disposable-runner smoke for the packaged Click Dot (click-dot_<version>_windows_amd64.zip).
#
# It extracts the zip, launches the real packaged `Click Dot.exe` with start-at-login disabled and an
# isolated user-data dir, and proves it serves locally, reports no connected account, and does not
# write into its own install folder.
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

  $before = Get-TreeSnapshot -Root $appRoot
  Write-Host "Extracted $($before.Count) entries; launching Click Dot."

  # Never register a login item: both the flag and the environment switch disable it.
  $env:CLICK_DOT_DISABLE_LOGIN_ITEM = '1'
  $dotProcess = Start-Process -FilePath $dotExe `
    -ArgumentList @('--no-login-item', "--user-data-dir=$userData") `
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

  Write-Host 'Click Dot package smoke passed: served 200 with "Click Dot", disconnected, app folder untouched.'
} catch {
  $failed = $true
  [Console]::Error.WriteLine("Click Dot package smoke failed: $($_.Exception.Message)")
  throw
} finally {
  if ($null -ne $dotProcess) {
    # Stop the whole Electron process tree (main, renderer, GPU, utility processes).
    & taskkill.exe /PID $dotProcess.Id /T /F 2>&1 | Out-Null
    Start-Sleep -Seconds 2
  }
  if ($failed) { Show-ProcessOutput -StdoutPath $stdoutLog -StderrPath $stderrLog }
  [System.Environment]::SetEnvironmentVariable('CLICK_DOT_DISABLE_LOGIN_ITEM', $originalDisableLoginItem, 'Process')
  Remove-Item -LiteralPath $smokeRoot -Recurse -Force -ErrorAction SilentlyContinue
}
