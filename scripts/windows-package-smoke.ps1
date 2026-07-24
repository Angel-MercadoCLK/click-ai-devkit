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

New-Item -ItemType Directory -Path $packageRoot, $extractRoot, $binRoot, $claudeHome, $openClawHome, $codexHome | Out-Null

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
New-CmdStub -Path (Join-Path $binRoot 'codex.cmd') -Lines @(
  'exit /b 0'
)
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

& $clickExe install --yes --profile balanced --skip-openclaw --codex-model gpt-5.6 | Out-Null
$codexAgents = Get-Content -LiteralPath (Join-Path $codexHome 'AGENTS.md') -Raw
$codexConfig = Get-Content -LiteralPath (Join-Path $codexHome 'config.toml') -Raw
Assert-Contains -Actual $codexAgents -Expected 'This block is managed by click' -Context 'Codex AGENTS.md'
Assert-Contains -Actual $codexConfig -Expected 'model = "gpt-5.6"' -Context 'Codex config.toml'

& $clickExe update | Out-Null
$claudeCalls = Get-Content -LiteralPath $claudeLog -Raw
Assert-Contains -Actual $claudeCalls -Expected 'plugin marketplace add https://github.com/Angel-MercadoCLK/click-ai-devkit --sparse .claude-plugin plugins' -Context 'claude command log'
Assert-Contains -Actual $claudeCalls -Expected 'plugin marketplace update click-ai-devkit' -Context 'claude command log'
Assert-Contains -Actual $claudeCalls -Expected 'plugin install click-sdd@click-ai-devkit' -Context 'claude command log'

Remove-Item -LiteralPath $smokeRoot -Recurse -Force
