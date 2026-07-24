package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

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
	assertWorkflowContainsRun(t, job, "scripts/windows-package-smoke.ps1")
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
	assertWorkflowContainsRun(t, job, "scripts/windows-package-smoke.ps1")

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
	assertContains(t, readme, "Codex updates `AGENTS.md` and changes the root `model` key in `config.toml` only when an explicit native model was selected.")

	codex := readText(t, filepath.Join("..", "..", "documentacion", "codex-target.md"))
	assertContains(t, codex, "Click can update the root `model` key in `config.toml` only when an explicit native model was selected during install.")
	assertContains(t, codex, "Click never changes credentials, providers, or any table-scoped `model` keys.")

	runbook := readText(t, filepath.Join("..", "..", "documentacion", "portability-runbook.md"))
	assertContains(t, runbook, "Click's automated OpenClaw mutation is qualified by the `openclaw config set --help` probe.")
	assertContains(t, runbook, "Release evidence still requires a recorded run against a real installed OpenClaw CLI before publication.")
	assertContains(t, runbook, "Portability runbook (`documentacion/portability-runbook.md`) passed on a clean/gentle-ai-absent profile for this version.")
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
	for _, step := range job.Steps {
		if strings.Contains(step.Run, want) {
			return
		}
	}
	t.Fatalf("workflow job missing run containing %q", want)
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
