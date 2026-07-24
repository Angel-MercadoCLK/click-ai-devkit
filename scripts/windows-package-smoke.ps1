param(
  [Parameter(Mandatory = $true)]
  [string]$DistDir,
  [Parameter(Mandatory = $true)]
  [string]$ExpectedVersion
)

$ErrorActionPreference = 'Stop'

function Assert-Contains {
  param(
    [string]$Actual,
    [string]$Expected,
    [string]$Context
  )

  if (-not $Actual.Contains($Expected)) {
    throw "$Context did not contain '$Expected'. Actual: $Actual"
  }
}

function New-CmdStub {
  param(
    [string]$Path,
    [string[]]$Lines
  )

  [System.IO.File]::WriteAllText($Path, ((@("@echo off") + $Lines) -join "`r`n") + "`r`n")
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$distRoot = Join-Path $repoRoot $DistDir
$binary = Get-ChildItem -LiteralPath $distRoot -Recurse -Filter click.exe | Select-Object -First 1
if (-not $binary) {
  throw "No click.exe found under $distRoot"
}

$smokeRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("click-release-smoke-" + [System.Guid]::NewGuid().ToString('N'))
$packageRoot = Join-Path $smokeRoot 'package'
$extractRoot = Join-Path $smokeRoot 'extract'
$binRoot = Join-Path $smokeRoot 'bin'
$claudeHome = Join-Path $smokeRoot '.claude'
$openClawHome = Join-Path $smokeRoot '.openclaw'
$codexHome = Join-Path $smokeRoot '.codex'
$engramBinary = Join-Path $binRoot 'engram.cmd'
$claudeLog = Join-Path $smokeRoot 'claude.log'
$openClawLog = Join-Path $smokeRoot 'openclaw.log'
$openClawHelper = Join-Path $binRoot 'openclaw-helper.ps1'
$codexLog = Join-Path $smokeRoot 'codex.log'
$codexHelper = Join-Path $binRoot 'codex-helper.ps1'
$codexEngramRegisteredMarker = Join-Path $smokeRoot 'codex-engram-registered.marker'

New-Item -ItemType Directory -Path $packageRoot, $extractRoot, $binRoot, $claudeHome, $openClawHome, $codexHome | Out-Null

# Real-repro regression guard: a live OpenClaw instance's own `validate config` proved
# `mcpServers` is an unrecognized top-level key in the real schema — click used to write it,
# corrupting every affected install. Seed exactly that legacy-broken shape (plus a legitimate
# unrelated key that must survive untouched) so this smoke fails again if that regresses.
$openClawConfigPath = Join-Path $openClawHome 'openclaw.json'
@'
{
  "agents": { "defaults": { "model": { "primary": "openai/gpt-5.6-sol" } } },
  "mcpServers": { "engram": { "command": "engram", "args": ["mcp", "--tools=agent"], "transport": "stdio" } }
}
'@ | Set-Content -LiteralPath $openClawConfigPath -Encoding ASCII

$zipPath = Join-Path $packageRoot 'click_windows_smoke.zip'
Compress-Archive -LiteralPath $binary.FullName -DestinationPath $zipPath
Expand-Archive -LiteralPath $zipPath -DestinationPath $extractRoot
$clickExe = Join-Path $extractRoot 'click.exe'
if (-not (Test-Path -LiteralPath $clickExe)) {
  throw "Packaged click.exe was not extracted to $clickExe"
}

New-CmdStub -Path (Join-Path $binRoot 'git.cmd') -Lines @(
  'echo git version 2.45.1'
)
# Real-repro regression guard for SyncCodexMCP: mirrors the confirmed live-Codex behavior —
# `codex mcp get engram` fails until `codex mcp add engram -- engram mcp --tools=agent` has run,
# then succeeds (idempotency check) — so this smoke exercises both the first-time-registration and
# the already-registered no-op paths across the install/update calls below.
@"
# No param() block on purpose: a literal '--' argument (as in 'mcp add engram -- engram mcp ...')
# confuses PowerShell's positional-parameter binding when declared via
# [Parameter(ValueFromRemainingArguments=`$true)]. Reading the automatic `$args variable directly
# avoids that binding step entirely — a real codex.exe receives its argv straight from the OS via
# exec.CommandContext with no such parsing involved, so this is purely a test-stub concern.
`$joined = [string]::Join(' ', `$args)
Add-Content -LiteralPath '$codexLog' -Value `$joined
if (`$args.Count -ge 3 -and `$args[0] -eq 'mcp' -and `$args[1] -eq 'get' -and `$args[2] -eq 'engram') {
  if (Test-Path -LiteralPath '$codexEngramRegisteredMarker') { exit 0 } else { exit 1 }
}
if (`$args.Count -ge 3 -and `$args[0] -eq 'mcp' -and `$args[1] -eq 'add' -and `$args[2] -eq 'engram') {
  New-Item -ItemType File -Path '$codexEngramRegisteredMarker' -Force | Out-Null
  exit 0
}
exit 0
"@ | Set-Content -LiteralPath $codexHelper -Encoding ASCII
[System.IO.File]::WriteAllText((Join-Path $binRoot 'codex.cmd'), '@powershell -NoProfile -ExecutionPolicy Bypass -File "' + $codexHelper + '" %*' + "`r`n")
New-CmdStub -Path $engramBinary -Lines @(
  'echo engram stub >> "%TEMP%\\click-engram-stub.log"',
  'exit /b 0'
)
New-CmdStub -Path (Join-Path $binRoot 'claude.cmd') -Lines @(
  'echo %*>>"' + $claudeLog + '"',
  'exit /b 0'
)
@"
param([Parameter(ValueFromRemainingArguments = `$true)][string[]]`$Args)
`$joined = [string]::Join(' ', `$Args)
Add-Content -LiteralPath '$openClawLog' -Value `$joined
if (`$joined -eq 'config set --help') {
  'config set agents.defaults.model.primary agents.defaults.model.fallbacks --strict-json'
}
"@ | Set-Content -LiteralPath $openClawHelper -Encoding ASCII
[System.IO.File]::WriteAllText((Join-Path $binRoot 'openclaw.cmd'), '@powershell -NoProfile -ExecutionPolicy Bypass -File "' + $openClawHelper + '" %*' + "`r`n")

$env:PATH = "$binRoot;$env:PATH"
$env:CLICK_CLAUDE_HOME = $claudeHome
$env:CLICK_OPENCLAW_HOME = $openClawHome
$env:CODEX_HOME = $codexHome
$env:CLICK_ENGRAM_BINARY_PATH = $engramBinary

$versionOutput = & $clickExe --version
Assert-Contains -Actual $versionOutput -Expected $ExpectedVersion -Context 'click --version output'

$targetsOutput = & $clickExe targets --no-color
Assert-Contains -Actual $targetsOutput -Expected 'Claude Code: detectado' -Context 'click targets output'
Assert-Contains -Actual $targetsOutput -Expected 'OpenClaw: detectado' -Context 'click targets output'
Assert-Contains -Actual $targetsOutput -Expected 'Codex: detectado' -Context 'click targets output'
Assert-Contains -Actual $targetsOutput -Expected 'modelo nativo de config.toml' -Context 'click targets output'

& $clickExe configure-openclaw-model openai/gpt-5.6-sol google/gemini-2.5-pro | Out-Null
$openClawCalls = Get-Content -LiteralPath $openClawLog -Raw
Assert-Contains -Actual $openClawCalls -Expected 'config set agents.defaults.model.primary openai/gpt-5.6-sol' -Context 'openclaw command log'
Assert-Contains -Actual $openClawCalls -Expected 'config set agents.defaults.model.fallbacks ["google/gemini-2.5-pro"] --strict-json' -Context 'openclaw command log'

# No --skip-openclaw here: that flag PERSISTS OpenClaw=false to targets.json (install.go saves the
# resolved selection), which would make the later `update` call below skip OpenClaw too even
# without repeating the flag — defeating the openclaw.json healing assertion after `update`.
& $clickExe install --yes --profile balanced --codex-model gpt-5.6 | Out-Null
$codexAgents = Get-Content -LiteralPath (Join-Path $codexHome 'AGENTS.md') -Raw
$codexConfig = Get-Content -LiteralPath (Join-Path $codexHome 'config.toml') -Raw
Assert-Contains -Actual $codexAgents -Expected 'This block is managed by click' -Context 'Codex AGENTS.md'
Assert-Contains -Actual $codexConfig -Expected 'model = "gpt-5.6"' -Context 'Codex config.toml'

# First-time Engram registration with Codex: get (not-yet-registered) then add, exact real syntax.
$codexCallsAfterInstall = Get-Content -LiteralPath $codexLog -Raw
Assert-Contains -Actual $codexCallsAfterInstall -Expected 'mcp get engram' -Context 'codex command log (install)'
Assert-Contains -Actual $codexCallsAfterInstall -Expected 'mcp add engram -- engram mcp --tools=agent' -Context 'codex command log (install)'
if (-not (Test-Path -LiteralPath $codexEngramRegisteredMarker)) {
  throw 'SyncCodexMCP did not register Engram with Codex during install'
}

& $clickExe update | Out-Null
$claudeCalls = Get-Content -LiteralPath $claudeLog -Raw
Assert-Contains -Actual $claudeCalls -Expected 'plugin marketplace add https://github.com/Angel-MercadoCLK/click-ai-devkit --sparse .claude-plugin plugins' -Context 'claude command log'
Assert-Contains -Actual $claudeCalls -Expected 'plugin marketplace update click-ai-devkit' -Context 'claude command log'
Assert-Contains -Actual $claudeCalls -Expected 'plugin install click-sdd@click-ai-devkit' -Context 'claude command log'

# Idempotent Codex re-run: get now succeeds (already registered), add must NOT run again.
$codexCallsAfterUpdate = (Get-Content -LiteralPath $codexLog -Raw).Substring($codexCallsAfterInstall.Length)
Assert-Contains -Actual $codexCallsAfterUpdate -Expected 'mcp get engram' -Context 'codex command log (update)'
if ($codexCallsAfterUpdate.Contains('mcp add engram')) {
  throw "SyncCodexMCP re-added an already-registered Engram server on update. Actual: $codexCallsAfterUpdate"
}

# Real-repro regression guard: `click update` (OpenClaw now a target, no --skip-openclaw) must have
# HEALED the legacy invalid 'mcpServers' key seeded above, and preserved every other key untouched.
$healedConfig = Get-Content -LiteralPath $openClawConfigPath -Raw | ConvertFrom-Json
if ($null -ne $healedConfig.mcpServers) {
  throw "click update did not remove the invalid legacy 'mcpServers' key from openclaw.json. Actual: $(Get-Content -LiteralPath $openClawConfigPath -Raw)"
}
if ($healedConfig.agents.defaults.model.primary -ne 'openai/gpt-5.6-sol') {
  throw "click update did not preserve unrelated openclaw.json content during cleanup. Actual: $(Get-Content -LiteralPath $openClawConfigPath -Raw)"
}

Remove-Item -LiteralPath $smokeRoot -Recurse -Force
