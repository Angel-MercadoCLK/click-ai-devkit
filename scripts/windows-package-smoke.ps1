param(
  [Parameter(Mandatory = $true)]
  [string]$DistDir,
  [Parameter(Mandatory = $true)]
  [string]$ExpectedVersion,
  [Parameter(Mandatory = $true)]
  [switch]$DisposableRunner
)

$ErrorActionPreference = 'Stop'

# This is an operational guard, not cryptographic runner detection. The real isolation boundary is
# the disposable GitHub-hosted windows-latest job.
if (-not $DisposableRunner) {
  throw 'Pass -DisposableRunner only from the disposable GitHub-hosted Windows smoke workflow.'
}
if ([System.Environment]::OSVersion.Platform -ne [System.PlatformID]::Win32NT) {
  throw 'The Windows package smoke requires a Windows runner.'
}
if ($env:GITHUB_ACTIONS -ne 'true' -or
    $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:RUNNER_OS -ne 'Windows') {
  throw 'The Windows package smoke requires a disposable GitHub-hosted Windows runner.'
}

function Assert-Contains {
  param([string]$Actual, [string]$Expected, [string]$Context)
  if ([string]::IsNullOrEmpty($Actual)) {
    throw "$Context produced no output; expected '$Expected'."
  }
  if (-not $Actual.Contains($Expected)) {
    throw "$Context did not contain '$Expected'. Actual: $Actual"
  }
}

function Copy-SmokeExecutable {
  param([string]$Source, [string]$Destination)
  Copy-Item -LiteralPath $Source -Destination $Destination -Force
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
$stubRoot = Join-Path $smokeRoot 'stubs'
$homeClaude = Join-Path $smokeRoot '.claude'
$homeCodex = Join-Path $smokeRoot '.codex'
$homeOpenClaw = Join-Path $smokeRoot '.openclaw'
$clickStateHome = Join-Path $smokeRoot '.click-state'
$configHome = Join-Path $smokeRoot '.config'
$managedBin = Join-Path $smokeRoot '.click\bin'
$goStub = Join-Path $stubRoot 'go.exe'
$commandStub = Join-Path $stubRoot 'command-stub.exe'
$engramBinary = Join-Path $stubRoot 'engram.exe'
$commandLog = Join-Path $smokeRoot 'commands.log'
$goLog = Join-Path $smokeRoot 'go-commands.log'
$pluginState = Join-Path $smokeRoot 'claude-plugins.json'
$claudeSettings = Join-Path $homeClaude 'settings.json'
$claudeJSON = Join-Path $smokeRoot '.claude.json'
$codexConfig = Join-Path $homeCodex 'config.toml'
$openClawConfig = Join-Path $homeOpenClaw 'openclaw.json'
$claudeModels = Join-Path $homeClaude 'click-ai-devkit\models.json'
$codexModelProfile = Join-Path $homeCodex 'click-ai-devkit\model-profile.json'
$openClawModelProfile = Join-Path $homeOpenClaw 'click-ai-devkit\model-profile.json'
$clickSddBackupDir = Join-Path $clickStateHome 'retired-plugin-options'

$environmentNames = @(
  'PATH', 'HOME', 'USERPROFILE', 'XDG_CONFIG_HOME', 'CLICK_STATE_HOME',
  'CLICK_CLAUDE_HOME', 'CLICK_OPENCLAW_HOME', 'CODEX_HOME', 'CLICK_ENGRAM_BINARY_PATH',
  'CLICK_ENGRAM_CLOUD_SERVER', 'CLICK_ENGRAM_CLOUD_PROJECT', 'ENGRAM_CLOUD_TOKEN',
  'CLICK_SMOKE_COMMAND_LOG', 'CLICK_SMOKE_PLUGIN_STATE', 'CLICK_SMOKE_GO_LOG', 'CLICK_SMOKE_FAIL_INSTALL',
  'GOCACHE', 'GOPATH', 'GOMODCACHE', 'GOENV', 'GOFLAGS', 'GOWORK', 'GOPROXY', 'GOTOOLCHAIN', 'GOOS', 'GOARCH', 'CGO_ENABLED'
)
$originalEnvironment = @{}
$locationPushed = $false
foreach ($name in $environmentNames) {
  $originalEnvironment[$name] = [System.Environment]::GetEnvironmentVariable($name, 'Process')
}

try {
  New-Item -ItemType Directory -Path $packageRoot, $extractRoot, $stubRoot, $homeClaude, $homeCodex, $homeOpenClaw, $clickStateHome, $configHome, $managedBin | Out-Null

  # Isolate the Go build cache and disable toolchain/module network fallback before compiling the
  # local, standard-library-only fixture. PATH remains untouched until both real executables exist.
  $env:HOME = $smokeRoot
  $env:USERPROFILE = $smokeRoot
  $env:XDG_CONFIG_HOME = $configHome
  $env:CLICK_STATE_HOME = $clickStateHome
  $env:GOCACHE = Join-Path $smokeRoot '.go-cache'
  $env:GOPATH = Join-Path $smokeRoot '.go-path'
  $env:GOMODCACHE = Join-Path $smokeRoot '.go-mod-cache'
  $env:GOENV = 'off'
  $env:GOFLAGS = ''
  $env:GOWORK = 'off'
  $env:GOPROXY = 'off'
  $env:GOTOOLCHAIN = 'local'
  $env:GOOS = 'windows'
  $env:GOARCH = 'amd64'
  $env:CGO_ENABLED = '0'

  # Compile the pinned-component and fake-Go fixtures before adding any stubs to PATH. Fake Go
  # validates the exact pinned provisioning contract and stages only its own compiled bytes.
  Push-Location -LiteralPath $repoRoot
  $locationPushed = $true
  try {
    & go build -o $commandStub ./scripts/test-fixtures/windows-smoke-stub
    if ($LASTEXITCODE -ne 0) {
      throw "Could not build the isolated command smoke fixture (exit $LASTEXITCODE)"
    }
  } finally {
    Pop-Location
    $locationPushed = $false
  }
  foreach ($name in @('claude.exe', 'git.exe', 'openclaw.exe', 'codex.exe')) {
    Copy-SmokeExecutable -Source $commandStub -Destination (Join-Path $stubRoot $name)
  }
  Copy-SmokeExecutable -Source $commandStub -Destination $goStub
  Copy-SmokeExecutable -Source $commandStub -Destination $engramBinary

  $zipPath = Join-Path $packageRoot 'click_windows_smoke.zip'
  Compress-Archive -LiteralPath $binary.FullName -DestinationPath $zipPath
  Expand-Archive -LiteralPath $zipPath -DestinationPath $extractRoot
  $clickExe = Join-Path $extractRoot 'click.exe'
  if (-not (Test-Path -LiteralPath $clickExe)) {
    throw "Packaged click.exe was not extracted to $clickExe"
  }

  # Snapshot all environment that can affect target, state, token, and executable resolution.
  # Click targets are the standard directories beneath HOME/USERPROFILE; no root overrides are used.
  $env:CLICK_ENGRAM_BINARY_PATH = $engramBinary
  $env:CLICK_SMOKE_COMMAND_LOG = $commandLog
  $env:CLICK_SMOKE_GO_LOG = $goLog
  $env:CLICK_SMOKE_PLUGIN_STATE = $pluginState
  $env:CLICK_CLAUDE_HOME = $null
  $env:CLICK_OPENCLAW_HOME = $null
  $env:CODEX_HOME = $null
  $env:CLICK_ENGRAM_CLOUD_SERVER = $null
  $env:CLICK_ENGRAM_CLOUD_PROJECT = $null
  $env:ENGRAM_CLOUD_TOKEN = $null
  $env:CLICK_SMOKE_FAIL_INSTALL = $null

  # Leave the managed pin absent to exercise Windows provisioning. Fake Go is first on PATH;
  # compilation happened offline above, and the staged executable is only the local fixture.
  $env:PATH = "$stubRoot;$managedBin"

  [System.IO.File]::WriteAllText($codexConfig, "model = 'codex-native-sentinel'`r`n")
  [System.IO.File]::WriteAllText((Join-Path $homeCodex 'sdd-strong.config.toml'), "model = 'codex-profile-sentinel'`r`n")
  New-Item -ItemType Directory -Force -Path (Split-Path -Parent $claudeModels) | Out-Null
  New-Item -ItemType Directory -Force -Path (Split-Path -Parent $codexModelProfile) | Out-Null
  New-Item -ItemType Directory -Force -Path (Split-Path -Parent $openClawModelProfile) | Out-Null
  [System.IO.File]::WriteAllText($claudeModels, '{"schemaVersion":1,"sentinel":"claude-legacy-profile"}')
  [System.IO.File]::WriteAllText($codexModelProfile, '{"schemaVersion":1,"sentinel":"codex-legacy-profile"}')
  [System.IO.File]::WriteAllText($openClawModelProfile, '{"schemaVersion":1,"sentinel":"openclaw-legacy-profile"}')
  [System.IO.File]::WriteAllText($claudeSettings, @'
{"env":{"ENGRAM_CLOUD_TOKEN":"snapshot-cloud-sentinel"},"model":"claude-native-sentinel","hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"user-owned-hook"}]}]},"pluginConfigs":{"click-sdd@click-ai-devkit":{"enabled":true,"options":{"sentinel":"preserve"}}}}
'@)
  [System.IO.File]::WriteAllText((Join-Path $homeClaude 'CLAUDE.md'), @'
Before install.
# >>> click-ai-devkit (managed) >>>
old-click-guidance-sentinel
# <<< click-ai-devkit (managed) <<<
After install.
'@)
  [System.IO.File]::WriteAllText($openClawConfig, @'
{"agents":{"defaults":{"model":{"primary":"openclaw-native-primary","fallbacks":["openclaw-native-fallback"]}}},"custom":{"sentinel":"preserve"}}
'@)
  [System.IO.File]::WriteAllText($pluginState, @'
  [{"id":"click-sdd@click-ai-devkit","scope":"user","enabled":true},{"id":"click-sdd@click-ai-devkit","scope":"project","projectPath":"C:\\smoke\\project-a","enabled":true},{"id":"click-sdd@click-ai-devkit","scope":"project","projectPath":"C:\\smoke\\project-b","enabled":true},{"id":"click-sdd@click-ai-devkit","scope":"local","enabled":true},{"id":"click-sdd@click-ai-devkit","scope":"managed","enabled":true},{"id":"click-sdd@click-ai-devkit","scope":"session","enabled":true},{"id":"click-sdd@click-ai-devkit","scope":"synced","enabled":true},{"id":"foreign-plugin@other-marketplace","scope":"user","enabled":true}]
'@)

  $versionOutput = & $clickExe --version
  if ($LASTEXITCODE -ne 0) { throw "click --version failed with exit $LASTEXITCODE" }
  Assert-Contains -Actual $versionOutput -Expected $ExpectedVersion -Context 'click --version output'

  $targetsOutput = & $clickExe targets --no-color
  if ($LASTEXITCODE -ne 0) { throw "click targets failed with exit $LASTEXITCODE" }
  foreach ($target in @('Claude Code: detectado', 'OpenClaw: detectado', 'Codex: detectado')) {
    Assert-Contains -Actual $targetsOutput -Expected $target -Context 'click targets output'
  }

  & $clickExe install --yes | Out-Null
  if ($LASTEXITCODE -ne 0) { throw "click install failed with exit $LASTEXITCODE" }

  # Model a previous Click guidance revision before update so rollback has an observable owned
  # block to restore, rather than merely succeeding after an upstream failure that wrote nothing.
  [System.IO.File]::WriteAllText((Join-Path $homeClaude 'CLAUDE.md'), @'
update preface sentinel
# >>> click-ai-devkit (managed) >>>
old-click-guidance-sentinel
# <<< click-ai-devkit (managed) <<<
update suffix sentinel
'@)
  & $clickExe update | Out-Null
  if ($LASTEXITCODE -ne 0) { throw "click update failed with exit $LASTEXITCODE" }

  $updatedGuidance = Get-Content -LiteralPath (Join-Path $homeClaude 'CLAUDE.md') -Raw
  $clickBlock = [System.Text.RegularExpressions.Regex]::Match($updatedGuidance, '(?s)# >>> click-ai-devkit \(managed\) >>>.*?# <<< click-ai-devkit \(managed\) <<<')
  if (-not $clickBlock.Success) { throw 'Successful update did not produce a bounded Click guidance block' }
  [System.IO.File]::WriteAllText((Join-Path $homeClaude 'CLAUDE.md'), "foreign-after-update-sentinel`r`n$($clickBlock.Value)`r`nforeign-suffix-sentinel")
  $currentSettings = Get-Content -LiteralPath $claudeSettings -Raw | ConvertFrom-Json
  $currentSettings.env.ENGRAM_CLOUD_TOKEN = 'current-synthetic-cloud-token'
  [System.IO.File]::WriteAllText($claudeSettings, ($currentSettings | ConvertTo-Json -Depth 32))
  & $clickExe rollback --yes | Out-Null
  if ($LASTEXITCODE -ne 0) { throw "click rollback failed with exit $LASTEXITCODE" }
  $rolledBackGuidance = Get-Content -LiteralPath (Join-Path $homeClaude 'CLAUDE.md') -Raw
  if (-not $rolledBackGuidance.Contains('old-click-guidance-sentinel') -or
      -not $rolledBackGuidance.Contains('foreign-after-update-sentinel') -or
      -not $rolledBackGuidance.Contains('foreign-suffix-sentinel')) {
    throw 'Rollback did not restore the prior Click block while preserving current surrounding text'
  }
  $rolledBackSettings = Get-Content -LiteralPath $claudeSettings -Raw | ConvertFrom-Json
  if ($rolledBackSettings.env.ENGRAM_CLOUD_TOKEN -ne 'current-synthetic-cloud-token') {
    throw 'Rollback changed the current synthetic Engram Cloud token'
  }
  Assert-Contains -Actual $rolledBackSettings.model -Expected 'claude-native-sentinel' -Context 'preserved Claude native model'
  if ($rolledBackSettings.hooks.UserPromptSubmit[0].hooks[0].command -ne 'user-owned-hook') {
    throw 'Rollback changed a foreign Claude hook'
  }
  & $clickExe update | Out-Null
  if ($LASTEXITCODE -ne 0) { throw "click update after rollback failed with exit $LASTEXITCODE" }

  $upstreamCalls = Get-Content -LiteralPath $commandLog -Raw
  $goCalls = Get-Content -LiteralPath $goLog -Raw
  Assert-Contains -Actual $goCalls -Expected 'go version' -Context 'isolated Go version probe'
  Assert-Contains -Actual $goCalls -Expected 'go install github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@v4.0.0' -Context 'isolated pinned Go install'
  Assert-Contains -Actual $upstreamCalls -Expected 'gentle-ai install --agents claude-code,codex,openclaw --preset full-gentleman --components engram,skills,context7,permissions,persona --persona neutral --scope global' -Context 'pinned upstream install invocation'
  if ($upstreamCalls.Contains('mcp remove engram') -or $upstreamCalls.Contains('mcp unset engram') -or
      $upstreamCalls.Contains('mcp remove context7') -or $upstreamCalls.Contains('mcp unset context7')) {
    throw 'Click smoke attempted to remove upstream-owned shared Engram or Context7 registrations'
  }
  $claudeCalls = Get-Content -LiteralPath $commandLog -Raw
  Assert-Contains -Actual $claudeCalls -Expected 'claude plugin list --json' -Context 'Claude user plugin inventory'
  Assert-Contains -Actual $claudeCalls -Expected 'plugin uninstall click-sdd@click-ai-devkit --scope user --keep-data' -Context 'legacy Click SDD retirement'
  if ($claudeCalls.IndexOf('claude plugin uninstall click-sdd@click-ai-devkit --scope user --keep-data') -gt
      $claudeCalls.IndexOf('claude plugin marketplace add')) {
    throw 'Click updated the marketplace before safely retiring the legacy user plugin'
  }
  foreach ($plugin in @('click-memory', 'click-review', 'click-skills')) {
    Assert-Contains -Actual $claudeCalls -Expected "plugin install $plugin@click-ai-devkit" -Context 'retained Click plugin installation'
  }
  if ($claudeCalls.Contains('plugin install click-sdd@click-ai-devkit')) {
    throw 'Click update reinstalled the retired click-sdd plugin'
  }

  $managedClaude = Get-Content -LiteralPath (Join-Path $homeClaude 'CLAUDE.md') -Raw
  Assert-Contains -Actual $managedClaude -Expected 'This block is managed by click' -Context 'Claude managed guidance'
  $codexAgents = Get-Content -LiteralPath (Join-Path $homeCodex 'AGENTS.md') -Raw
  Assert-Contains -Actual $codexAgents -Expected 'This block is managed by click' -Context 'Codex managed guidance'
  $openClawAgents = Get-Content -LiteralPath (Join-Path $homeOpenClaw 'workspace\AGENTS.md') -Raw
  Assert-Contains -Actual $openClawAgents -Expected 'This workspace is managed by click-ai-devkit' -Context 'OpenClaw managed guidance'

  $claudeMCP = Get-Content -LiteralPath $claudeJSON -Raw | ConvertFrom-Json
  if ($null -eq $claudeMCP.mcpServers.engram -or $null -eq $claudeMCP.mcpServers.context7) {
    throw 'Pinned upstream fixture did not register both Claude shared MCP servers'
  }
  $codexMCP = Get-Content -LiteralPath $codexConfig -Raw
  Assert-Contains -Actual $codexMCP -Expected '[mcp_servers.engram]' -Context 'Codex shared Engram MCP'
  Assert-Contains -Actual $codexMCP -Expected '[mcp_servers.context7]' -Context 'Codex shared Context7 MCP'
  $openClawMCP = Get-Content -LiteralPath $openClawConfig -Raw | ConvertFrom-Json
  if ($null -eq $openClawMCP.mcp.servers.engram -or $null -eq $openClawMCP.mcp.servers.context7) {
    throw 'Pinned upstream fixture did not register both OpenClaw shared MCP servers'
  }

  # This upstream failure occurs after snapshotting but before target writes. It is not a partial
  # target mutation test; T10c separately qualifies late-write recovery.
  Remove-Item -LiteralPath (Join-Path $clickStateHome 'backups\latest') -Recurse -Force
  $env:CLICK_SMOKE_FAIL_INSTALL = '1'
  $failureOutput = (& $clickExe update 2>&1 | Out-String)
  $failureExit = $LASTEXITCODE
  if ($failureExit -eq 0) { throw 'Injected gentle-ai update failure unexpectedly succeeded' }
  Assert-Contains -Actual $failureOutput -Expected 'No se pudo configurar gentle-ai para los runtimes seleccionados' -Context 'sanitized update failure'
  $env:CLICK_SMOKE_FAIL_INSTALL = $null
  $failureCalls = Get-Content -LiteralPath $commandLog -Raw
  Assert-Contains -Actual $failureCalls -Expected 'gentle-ai-failure install' -Context 'fixture-recorded failing upstream invocation'
  $failureManifestPath = Join-Path $clickStateHome 'backups\latest\manifest.json'
  if (-not (Test-Path -LiteralPath $failureManifestPath)) {
    throw 'Failed update did not retain a restorable post-run snapshot'
  }
  $failureManifest = Get-Content -LiteralPath $failureManifestPath -Raw | ConvertFrom-Json
  if (@($failureManifest.entries).Count -eq 0) {
    throw 'The current failing update produced an empty snapshot manifest'
  }

  & $clickExe uninstall | Out-Null
  if ($LASTEXITCODE -ne 0) { throw "click uninstall reported a teardown failure ($LASTEXITCODE)" }

  foreach ($path in @((Join-Path $homeClaude 'CLAUDE.md'), (Join-Path $homeCodex 'AGENTS.md'), (Join-Path $homeOpenClaw 'workspace\AGENTS.md'))) {
    if (Test-Path -LiteralPath $path) {
      $guidance = [System.IO.File]::ReadAllText($path)
      if ($guidance.Contains('managed by click')) {
        throw "click uninstall left Click-managed guidance in $path"
      }
    }
  }
  $remainingPlugins = Get-Content -LiteralPath $pluginState -Raw | ConvertFrom-Json
  if (@($remainingPlugins | Where-Object { $_.id -eq 'foreign-plugin@other-marketplace' }).Count -ne 1) {
    throw 'Click teardown changed a foreign Claude plugin'
  }
  if (@($remainingPlugins | Where-Object { $_.id -like '*@click-ai-devkit' -and $_.scope -eq 'user' }).Count -ne 0) {
    throw 'Click teardown left a user-scoped Click Claude plugin installed'
  }
  $retainedScopedSDD = @($remainingPlugins | Where-Object { $_.id -eq 'click-sdd@click-ai-devkit' })
  if ($retainedScopedSDD.Count -ne 6) {
    throw "Click teardown changed project/local/managed/session/synced click-sdd installs (remaining $($retainedScopedSDD.Count))"
  }
  $claudeCalls = Get-Content -LiteralPath $commandLog -Raw
  if ($claudeCalls.Contains('plugin marketplace remove') -or
      $claudeCalls.Contains('mcp remove engram') -or $claudeCalls.Contains('mcp unset engram') -or
      $claudeCalls.Contains('mcp remove context7') -or $claudeCalls.Contains('mcp unset context7')) {
    throw 'Click teardown attempted a marketplace cascade or shared Engram/Context7 removal'
  }
  foreach ($plugin in @('click-memory', 'click-review', 'click-skills')) {
    Assert-Contains -Actual $claudeCalls -Expected "plugin uninstall $plugin@click-ai-devkit --scope user --keep-data" -Context 'scoped Click plugin teardown'
  }

  $claudeMCP = Get-Content -LiteralPath $claudeJSON -Raw | ConvertFrom-Json
  $openClawMCP = Get-Content -LiteralPath $openClawConfig -Raw | ConvertFrom-Json
  $codexMCP = Get-Content -LiteralPath $codexConfig -Raw
  if ($null -eq $claudeMCP.mcpServers.engram -or $null -eq $claudeMCP.mcpServers.context7 -or
      $null -eq $openClawMCP.mcp.servers.engram -or $null -eq $openClawMCP.mcp.servers.context7 -or
      -not $codexMCP.Contains('[mcp_servers.engram]') -or -not $codexMCP.Contains('[mcp_servers.context7]')) {
    throw 'Click teardown removed shared upstream Engram/Context7 configuration'
  }
  Assert-Contains -Actual $codexMCP -Expected "model = 'codex-native-sentinel'" -Context 'preserved Codex native model'
  Assert-Contains -Actual (Get-Content -LiteralPath (Join-Path $homeCodex 'sdd-strong.config.toml') -Raw) -Expected 'codex-profile-sentinel' -Context 'preserved Codex native profile'
  $openClawJSON = Get-Content -LiteralPath $openClawConfig -Raw | ConvertFrom-Json
  if ($openClawJSON.agents.defaults.model.primary -ne 'openclaw-native-primary' -or
      $openClawJSON.agents.defaults.model.fallbacks[0] -ne 'openclaw-native-fallback') {
    throw 'Click teardown changed native OpenClaw model selections'
  }
  $settings = Get-Content -LiteralPath $claudeSettings -Raw | ConvertFrom-Json
  if ($settings.model -ne 'claude-native-sentinel' -or
      $settings.hooks.UserPromptSubmit[0].hooks[0].command -ne 'user-owned-hook') {
    throw 'Click teardown changed native Claude model or user-owned hook settings'
  }
  foreach ($pair in @(
    @{ Path = $claudeModels; Value = 'claude-legacy-profile' },
    @{ Path = $codexModelProfile; Value = 'codex-legacy-profile' },
    @{ Path = $openClawModelProfile; Value = 'openclaw-legacy-profile' }
  )) {
    Assert-Contains -Actual (Get-Content -LiteralPath $pair.Path -Raw) -Expected $pair.Value -Context "preserved legacy model artifact $($pair.Path)"
  }
  $retirementBackups = @(Get-ChildItem -LiteralPath $clickSddBackupDir -Filter 'click-sdd-*.json')
  if ($retirementBackups.Count -ne 1) {
    throw 'Retirement did not retain Click SDD plugin configuration backup'
  }
  $retirementBackup = Get-Content -LiteralPath $retirementBackups[0].FullName -Raw | ConvertFrom-Json
  if ($retirementBackup.pluginId -ne 'click-sdd@click-ai-devkit' -or
      $retirementBackup.scope -ne 'user' -or
      $retirementBackup.pluginConfig.options.sentinel -ne 'preserve') {
    throw 'Retirement backup did not preserve the exact Click SDD user plugin options'
  }
} catch {
  $scriptLine = $null
  if ($null -ne $_.InvocationInfo) {
    $scriptLine = $_.InvocationInfo.ScriptLineNumber
  }
  if ($null -eq $scriptLine -or $scriptLine -eq 0) {
    $scriptLine = 'unknown'
  }
  [Console]::Error.WriteLine("Windows package smoke failed at script line $scriptLine.")
  if (-not [string]::IsNullOrEmpty($_.ScriptStackTrace)) {
    [Console]::Error.WriteLine($_.ScriptStackTrace)
  }
  throw
} finally {
  foreach ($name in $environmentNames) {
    [System.Environment]::SetEnvironmentVariable($name, $originalEnvironment[$name], 'Process')
  }
  if ($locationPushed) { Pop-Location }
  Remove-Item -LiteralPath $smokeRoot -Recurse -Force -ErrorAction SilentlyContinue
}
