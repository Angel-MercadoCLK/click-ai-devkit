# Shared disposable-runner guard for Windows smoke scripts. Dot-source it, then call
# Assert-DisposableWindowsRunner before any mutation.
#
# This is an operational guard, not cryptographic runner detection. The real isolation boundary is
# the disposable GitHub-hosted windows-latest job.
function Assert-DisposableWindowsRunner {
  param(
    [Parameter(Mandatory = $true)]
    [bool]$DisposableRunner,
    [Parameter(Mandatory = $true)]
    [string]$Label
  )
  if (-not $DisposableRunner) {
    throw 'Pass -DisposableRunner only from the disposable GitHub-hosted Windows smoke workflow.'
  }
  if ([System.Environment]::OSVersion.Platform -ne [System.PlatformID]::Win32NT) {
    throw "The $Label requires a Windows runner."
  }
  if ($env:GITHUB_ACTIONS -ne 'true' -or
      $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
      $env:RUNNER_OS -ne 'Windows') {
    throw "The $Label requires a disposable GitHub-hosted Windows runner."
  }
}
