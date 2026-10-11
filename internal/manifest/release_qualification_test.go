package manifest

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestWindowsPackageSmokeUsesHermeticLifecycleAndRestoresEnvironment(t *testing.T) {
	path := filepath.Join("..", "..", "scripts", "windows-package-smoke.ps1")
	script, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	text := string(script)
	for _, expected := range []string{
		"[Parameter(Mandatory = $true)]\n  [switch]$DisposableRunner",
		"[switch]$DisposableRunner",
		"if (-not $DisposableRunner)",
		"$env:GITHUB_ACTIONS -ne 'true'",
		"$env:RUNNER_ENVIRONMENT -ne 'github-hosted'",
		"$env:RUNNER_OS -ne 'Windows'",
		"[System.Environment]::OSVersion.Platform -ne [System.PlatformID]::Win32NT",
		"This is an operational guard, not cryptographic runner detection.",
		"$originalEnvironment = @{",
		"$env:HOME = $smokeRoot",
		"$env:USERPROFILE = $smokeRoot",
		"$env:XDG_CONFIG_HOME = $configHome",
		"$env:CLICK_STATE_HOME = $clickStateHome",
		"$env:GOCACHE = Join-Path $smokeRoot '.go-cache'",
		"$env:GOENV = 'off'",
		"$env:GOPROXY = 'off'",
		"$env:GOOS = 'windows'",
		"$env:GOARCH = 'amd64'",
		"$env:GOFLAGS = ''",
		"$env:GOWORK = 'off'",
		"$env:ENGRAM_CLOUD_TOKEN = $null",
		"go build -o $commandStub",
		"Copy-SmokeExecutable -Source $commandStub -Destination $goStub",
		"go install github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@v4.0.0",
		"$env:CLICK_SMOKE_GO_LOG = $goLog",
		"$claudeModels",
		"$codexModelProfile",
		"$openClawModelProfile",
		"plugin list --json",
		"plugin uninstall click-sdd@click-ai-devkit --scope user --keep-data",
		"config.toml",
		"openclaw.json",
		"} finally {",
		"[System.Environment]::SetEnvironmentVariable($name, $originalEnvironment[$name], 'Process')",
		"Remove-Item -LiteralPath $smokeRoot -Recurse -Force -ErrorAction SilentlyContinue",
	} {
		if !strings.Contains(text, expected) {
			t.Errorf("Windows package smoke script missing hermetic environment contract %q", expected)
		}
	}
	for _, expected := range []string{
		"New-Item -ItemType Directory -Force -Path (Split-Path -Parent $claudeModels)",
		"New-Item -ItemType Directory -Force -Path (Split-Path -Parent $codexModelProfile)",
		"New-Item -ItemType Directory -Force -Path (Split-Path -Parent $openClawModelProfile)",
		"Remove-Item -LiteralPath (Join-Path $clickStateHome 'backups\\latest') -Recurse -Force",
		"gentle-ai-failure install",
		"No se pudo configurar gentle-ai para los runtimes seleccionados",
		"old-click-guidance-sentinel",
		"foreign-after-update-sentinel",
		"mcp remove context7",
		"mcp unset context7",
	} {
		if !strings.Contains(text, expected) {
			t.Errorf("Windows package smoke script missing regression contract %q", expected)
		}
	}
	for _, pair := range [][2]string{
		{"New-Item -ItemType Directory -Force -Path (Split-Path -Parent $claudeModels)", "[System.IO.File]::WriteAllText($claudeModels"},
		{"New-Item -ItemType Directory -Force -Path (Split-Path -Parent $codexModelProfile)", "[System.IO.File]::WriteAllText($codexModelProfile"},
		{"New-Item -ItemType Directory -Force -Path (Split-Path -Parent $openClawModelProfile)", "[System.IO.File]::WriteAllText($openClawModelProfile"},
	} {
		createDir := strings.Index(text, pair[0])
		writeFile := strings.Index(text, pair[1])
		if createDir < 0 || writeFile < 0 || createDir > writeFile {
			t.Errorf("legacy fixture parent directory must be created before writing %q", pair[1])
		}
	}
	failureBlock := strings.Index(text, "# This upstream failure occurs after snapshotting")
	if failureBlock < 0 {
		t.Fatal("Windows package smoke missing the explicitly classified upstream failure section")
	}
	failureSection := text[failureBlock:]
	deleteLatest := strings.Index(failureSection, "Remove-Item -LiteralPath (Join-Path $clickStateHome 'backups\\latest')")
	failedUpdate := strings.Index(failureSection, "$failureOutput = (& $clickExe update")
	failureEvent := strings.Index(failureSection, "gentle-ai-failure install")
	manifestCheck := strings.Index(failureSection, "$failureManifestPath = Join-Path $clickStateHome")
	if deleteLatest < 0 || failedUpdate < 0 || failureEvent < failedUpdate || manifestCheck < failedUpdate {
		t.Fatal("failed-update qualification must clear stale latest, observe fixture failure, and inspect its fresh manifest")
	}
	rollback := strings.Index(text, "& $clickExe rollback --yes")
	restoredBlock := strings.Index(text, "Rollback did not restore the prior Click block")
	uninstall := strings.Index(text, "& $clickExe uninstall")
	if rollback < 0 || restoredBlock < rollback || uninstall < restoredBlock {
		t.Fatal("smoke must assert meaningful owned-block rollback before uninstall")
	}
	for _, forbidden := range []string{
		"$env:CLICK_CLAUDE_HOME = $homeClaude",
		"$env:CLICK_OPENCLAW_HOME = $homeOpenClaw",
		"$env:CODEX_HOME = $homeCodex",
		"$codexProfilePath",
		"$managedGentleAI",
	} {
		if strings.Contains(text, forbidden) {
			t.Errorf("Windows package smoke script contains non-hermetic or retired lifecycle contract %q", forbidden)
		}
	}
	guardIndex := strings.Index(text, "if (-not $DisposableRunner)")
	filesystemIndex := strings.Index(text, "Get-ChildItem -LiteralPath $distRoot")
	if guardIndex < 0 || filesystemIndex < 0 || guardIndex > filesystemIndex {
		t.Fatal("disposable-runner guard must run before distribution filesystem access")
	}
	buildIndex := strings.Index(text, "go build -o $commandStub")
	goStubIndex := strings.Index(text, "Copy-SmokeExecutable -Source $commandStub -Destination $goStub")
	pathIndex := strings.Index(text, "$env:PATH = \"$stubRoot;$managedBin\"")
	if buildIndex < 0 || goStubIndex < 0 || pathIndex < 0 || buildIndex > goStubIndex || goStubIndex > pathIndex {
		t.Fatal("Windows package smoke must build the isolated executable fixture before installing fake go on PATH")
	}
}

func TestWindowsPackageSmokeHandlesEmptyGuidanceAndReportsScriptLocation(t *testing.T) {
	path := filepath.Join("..", "..", "scripts", "windows-package-smoke.ps1")
	script, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	text := string(script)
	for _, expected := range []string{
		"$guidance = [System.IO.File]::ReadAllText($path)",
		"if ($guidance.Contains('managed by click'))",
		"if ([string]::IsNullOrEmpty($Actual))",
		"$_.InvocationInfo.ScriptLineNumber",
		"$_.ScriptStackTrace",
		"if (-not $Actual.Contains($Expected))",
	} {
		if !strings.Contains(text, expected) {
			t.Errorf("Windows package smoke missing nullable-output diagnostic contract %q", expected)
		}
	}
	if strings.Contains(text, "(Get-Content -Raw -LiteralPath $path).Contains('managed by click')") {
		t.Fatal("empty managed guidance must be read as a non-null string before searching")
	}
}

func TestWindowsSmokeGoFixtureEnforcesPinnedProvisioningAndStagesItsBinary(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	goBinary := filepath.Join(t.TempDir(), "go-fixture")
	executableSuffix := ""
	if runtime.GOOS == "windows" {
		executableSuffix = ".exe"
		goBinary += executableSuffix
	}
	build := exec.Command("go", "build", "-o", goBinary, "./scripts/test-fixtures/windows-smoke-stub")
	build.Dir = repoRoot
	build.Env = append(os.Environ(), "GOFLAGS=", "GOWORK=off", "GOPROXY=off", "GOTOOLCHAIN=local")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build smoke fixture: %v\n%s", err, output)
	}
	goPath := filepath.Join(filepath.Dir(goBinary), "go"+executableSuffix)
	if err := os.Rename(goBinary, goPath); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	stage := filepath.Join(root, ".click", "bin", ".gentle-ai-build-fixture")
	if err := os.MkdirAll(stage, 0o700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "go.log")
	baseEnv := []string{
		"HOME=" + root,
		"USERPROFILE=" + root,
		"GENTLE_AI_NO_SELF_UPDATE=1",
		"CLICK_SMOKE_GO_LOG=" + logPath,
		"GOENV=off", "GOFLAGS=", "GOSUMDB=sum.golang.org", "GONOSUMDB=none",
		"GOPRIVATE=none", "GONOPROXY=none", "GOINSECURE=",
		"GOPROXY=https://proxy.golang.org", "GOTOOLCHAIN=local",
		"GOOS=windows", "GOARCH=amd64", "GOBIN=" + stage,
	}
	version := exec.Command(goPath, "version")
	version.Env = baseEnv
	if output, err := version.CombinedOutput(); err != nil || string(output) != "go version go1.25.10 windows/amd64\n" {
		t.Fatalf("fake go version = %q, %v", output, err)
	}
	install := exec.Command(goPath, "install", "github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@v4.0.0")
	install.Env = baseEnv
	if output, err := install.CombinedOutput(); err != nil {
		t.Fatalf("fake pinned go install = %q, %v", output, err)
	}
	staged, err := os.ReadFile(filepath.Join(stage, "gentle-ai.exe"))
	if err != nil {
		t.Fatalf("fake Go did not stage its own executable: %v", err)
	}
	fixture, err := os.ReadFile(goPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(staged, fixture) {
		t.Fatal("fake Go staged bytes other than its already-compiled local fixture")
	}
	log, err := os.ReadFile(logPath)
	if err != nil || !strings.Contains(string(log), "go version") || !strings.Contains(string(log), "go install github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@v4.0.0") {
		t.Fatalf("fake Go invocation log = %q, %v", log, err)
	}
	invalid := exec.Command(goPath, "install", "example.invalid/untrusted@latest")
	invalid.Env = baseEnv
	if output, err := invalid.CombinedOutput(); err == nil || !strings.Contains(string(output), "unexpected go install arguments") {
		t.Fatalf("untrusted fake go install = %q, %v; want fail closed", output, err)
	}
}

func TestWindowsSmokeGentleAIFixtureRequiresPinAndPreservesSharedState(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	binaryName := "gentle-ai-smoke-fixture"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binary := filepath.Join(t.TempDir(), binaryName)
	build := exec.Command("go", "build", "-o", binary, "./scripts/test-fixtures/windows-smoke-stub")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build smoke fixture: %v\n%s", err, output)
	}

	home := t.TempDir()
	for _, dir := range []string{".claude", ".codex", ".openclaw"} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	codexConfig := filepath.Join(home, ".codex", "config.toml")
	if err := os.WriteFile(codexConfig, []byte("model = 'native-model-sentinel'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	openClawConfig := filepath.Join(home, ".openclaw", "openclaw.json")
	if err := os.WriteFile(openClawConfig, []byte(`{"agents":{"defaults":{"model":{"primary":"native-primary"}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	command := func(args ...string) *exec.Cmd {
		cmd := exec.Command(binary, args...)
		cmd.Env = []string{
			"HOME=" + home,
			"USERPROFILE=" + home,
			"GENTLE_AI_NO_SELF_UPDATE=1",
			"CLICK_SMOKE_COMMAND_LOG=" + filepath.Join(home, "commands.log"),
		}
		return cmd
	}
	gentleAI := filepath.Join(filepath.Dir(binary), "gentle-ai"+map[bool]string{true: ".exe"}[runtime.GOOS == "windows"])
	if err := os.Rename(binary, gentleAI); err != nil {
		t.Fatal(err)
	}
	binary = gentleAI
	versionOutput, err := command("version").CombinedOutput()
	if err != nil || string(versionOutput) != "gentle-ai v4.0.0\n" {
		t.Fatalf("version fixture = %q, %v; want pinned v4.0.0", versionOutput, err)
	}
	args := []string{
		"install", "--agents", "claude-code,codex,openclaw",
		"--preset", "full-gentleman",
		"--components", "engram,skills,context7,permissions,persona",
		"--persona", "neutral", "--scope", "global",
	}
	if output, err := command(args...).CombinedOutput(); err != nil {
		t.Fatalf("qualified install fixture failed: %v\n%s", err, output)
	}
	claudeRaw, err := os.ReadFile(filepath.Join(home, ".claude.json"))
	if err != nil {
		t.Fatal(err)
	}
	var claude map[string]any
	if err := json.Unmarshal(claudeRaw, &claude); err != nil {
		t.Fatal(err)
	}
	claudeServers, ok := claude["mcpServers"].(map[string]any)
	if !ok || claudeServers["engram"] == nil || claudeServers["context7"] == nil {
		t.Fatalf("fixture Claude MCP registrations = %#v, want Engram and Context7", claude["mcpServers"])
	}
	codexRaw, err := os.ReadFile(codexConfig)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"model = 'native-model-sentinel'", "[mcp_servers.engram]", "[mcp_servers.context7]"} {
		if !bytes.Contains(codexRaw, []byte(want)) {
			t.Errorf("Codex fixture config missing %q: %s", want, codexRaw)
		}
	}
	if output, err := command(args...).CombinedOutput(); err != nil {
		t.Fatalf("repeated fixture install failed: %v\n%s", err, output)
	}
	codexRaw, err = os.ReadFile(codexConfig)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(codexRaw), "[mcp_servers.engram]") != 1 || strings.Count(string(codexRaw), "[mcp_servers.context7]") != 1 {
		t.Fatalf("repeated fixture install duplicated Codex MCP tables: %s", codexRaw)
	}
	openClawRaw, err := os.ReadFile(openClawConfig)
	if err != nil {
		t.Fatal(err)
	}
	var openClaw map[string]any
	if err := json.Unmarshal(openClawRaw, &openClaw); err != nil {
		t.Fatal(err)
	}
	openClawMCP, ok := openClaw["mcp"].(map[string]any)
	if !ok || openClawMCP["servers"] == nil || openClaw["agents"] == nil {
		t.Fatalf("fixture OpenClaw config = %s, want MCP registration preserving native model data", openClawRaw)
	}
	openClawAgents := openClaw["agents"].(map[string]any)
	defaults := openClawAgents["defaults"].(map[string]any)
	model := defaults["model"].(map[string]any)
	if model["primary"] != "native-primary" {
		t.Fatalf("fixture changed native OpenClaw model selection: %#v", model)
	}
	failingInstall := command(args...)
	failingInstall.Env = append(failingInstall.Env, "CLICK_SMOKE_FAIL_INSTALL=1")
	if output, err := failingInstall.CombinedOutput(); err == nil || !strings.Contains(string(output), "injected upstream failure") {
		t.Fatalf("fixture failure injection = %q, %v; want deterministic failure", output, err)
	}
	commandLog, err := os.ReadFile(filepath.Join(home, "commands.log"))
	if err != nil || !strings.Contains(string(commandLog), "gentle-ai-failure install") {
		t.Fatalf("fixture did not record exact failing install event: %q, %v", commandLog, err)
	}
}

type workflowFile struct {
	Jobs map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	RunsOn string         `yaml:"runs-on"`
	Needs  any            `yaml:"needs"`
	Steps  []workflowStep `yaml:"steps"`
}

type workflowStep struct {
	Name string `yaml:"name"`
	Run  string `yaml:"run"`
	Uses string `yaml:"uses"`
}

func TestCIWorkflow_RequiresWindowsPackageSmokeAndVet(t *testing.T) {
	ci := loadWorkflow(t, filepath.Join("..", "..", ".github", "workflows", "ci.yml"))
	job, ok := ci.Jobs["build-and-test"]
	if !ok {
		t.Fatal("ci workflow missing build-and-test job")
	}
	assertWorkflowContainsRun(t, job, "go test ./... -count=1")
	assertWorkflowContainsRun(t, job, "go vet ./...")
	assertWorkflowContainsRun(t, job, "goreleaser build --snapshot --clean --single-target")
	assertWorkflowContainsRun(t, job, "./scripts/windows-package-smoke.ps1 -DistDir dist -ExpectedVersion 'click version' -DisposableRunner")
}

func TestReleaseWorkflow_GatesPublicationOnWindowsQualification(t *testing.T) {
	release := loadWorkflow(t, filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	job, ok := release.Jobs["windows-release-qualification"]
	if !ok {
		t.Fatal("release workflow missing windows-release-qualification job")
	}
	if job.RunsOn != "windows-latest" {
		t.Fatalf("windows-release-qualification runs-on = %q, want windows-latest", job.RunsOn)
	}
	assertWorkflowContainsRun(t, job, "go test ./... -count=1")
	assertWorkflowContainsRun(t, job, "go vet ./...")
	assertWorkflowContainsRun(t, job, "goreleaser build --snapshot --clean --single-target")
	assertWorkflowContainsRun(t, job, "./scripts/windows-package-smoke.ps1 -DistDir dist -ExpectedVersion $version -DisposableRunner")

	goreleaser, ok := release.Jobs["goreleaser"]
	if !ok {
		t.Fatal("release workflow missing goreleaser job")
	}
	if !workflowNeeds(goreleaser, "windows-release-qualification") {
		t.Fatalf("goreleaser job needs = %#v, want it to depend on windows-release-qualification", goreleaser.Needs)
	}
}

func TestGoReleaserBeforeHooks_RunRepositoryValidation(t *testing.T) {
	data := readText(t, filepath.Join("..", "..", ".goreleaser.yaml"))
	if !strings.Contains(data, "- go test ./... -count=1") {
		t.Fatal(".goreleaser.yaml missing count=1 repository test hook")
	}
	if !strings.Contains(data, "- go vet ./...") {
		t.Fatal(".goreleaser.yaml missing go vet hook")
	}
}

func TestReleaseDocumentation_MatchesCurrentTargetContracts(t *testing.T) {
	readme := readText(t, filepath.Join("..", "..", "README.md"))
	assertContains(t, readme, "Current release metadata lives in `bucket/click.json`, `internal/manifest/manifest.yaml`, and `click --version`.")
	assertContains(t, readme, "Click no escribe modelos nativos en esos runtimes.")
	assertContains(t, readme, "Click conserva sus políticas de negocio/memoria")

	codex := readText(t, filepath.Join("..", "..", "documentacion", "codex-target.md"))
	assertContains(t, codex, "Click does not write Codex model/profile values, credentials, providers, or other native model")
	assertContains(t, codex, "A configured nonstandard root is rejected before")

	runbook := readText(t, filepath.Join("..", "..", "documentacion", "portability-runbook.md"))
	assertContains(t, runbook, "Click no longer ships or installs `click-sdd`")
	assertContains(t, runbook, "Click's model-selection commands are compatibility notices")
	assertContains(t, runbook, "that acceptance remains pending")
	assertContains(t, runbook, "-DisposableRunner")
	assertContains(t, runbook, "operational guard, not cryptographic runner detection")
}

func loadWorkflow(t *testing.T, path string) workflowFile {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	var workflow workflowFile
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatalf("yaml.Unmarshal(%s) error = %v", path, err)
	}
	return workflow
}

func workflowNeeds(job workflowJob, want string) bool {
	switch needs := job.Needs.(type) {
	case string:
		return needs == want
	case []any:
		for _, raw := range needs {
			if value, ok := raw.(string); ok && value == want {
				return true
			}
		}
	}
	return false
}

func assertWorkflowContainsRun(t *testing.T, job workflowJob, want string) {
	t.Helper()
	var runs []string
	for _, step := range job.Steps {
		runs = append(runs, step.Run)
		if strings.Contains(step.Run, want) {
			return
		}
	}
	t.Fatalf("workflow job missing run containing %q; runs=%q", want, runs)
}

func assertContains(t *testing.T, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("content missing %q", want)
	}
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(data)
}
