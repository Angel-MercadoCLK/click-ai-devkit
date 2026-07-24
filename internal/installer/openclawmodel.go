package installer

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const openClawStrictJSONFlag = "--strict-json"

var openClawQualificationTokens = []string{
	"config",
	"set",
	"agents.defaults.model.primary",
	"agents.defaults.model.fallbacks",
	openClawStrictJSONFlag,
}

type openClawQualifiedContract struct {
	BinaryPath string
	ProbeArgs  []string
	WorkingDir string
	Timeout    time.Duration
	Evidence   string
}

type OpenClawNativeModelMenuStatus struct {
	Available bool
	Detail    string
}

var openClawNativeModelMenuStatusOverride *OpenClawNativeModelMenuStatus

func SetOpenClawNativeModelMenuStatusForTests(status OpenClawNativeModelMenuStatus) func() {
	previous := openClawNativeModelMenuStatusOverride
	copy := status
	openClawNativeModelMenuStatusOverride = &copy
	return func() { openClawNativeModelMenuStatusOverride = previous }
}

func OpenClawNativeModelActionStatus() OpenClawNativeModelMenuStatus {
	if openClawNativeModelMenuStatusOverride != nil {
		return *openClawNativeModelMenuStatusOverride
	}
	contract, err := qualifyOpenClawModelContract(commandRunnerFactory())
	if err != nil {
		return OpenClawNativeModelMenuStatus{Available: false, Detail: err.Error()}
	}
	return OpenClawNativeModelMenuStatus{
		Available: true,
		Detail:    fmt.Sprintf("qualified via %s in %s", contract.BinaryPath, contract.WorkingDir),
	}
}

// ConfigureOpenClawModels delegates native model configuration to OpenClaw's documented CLI. It
// intentionally does not read or rewrite openclaw.json, preserving JSON5/comments/includes and
// letting OpenClaw enforce its strict schema and hot-reload behavior.
func ConfigureOpenClawModels(primary string, fallbacks []string) error {
	if err := validateOpenClawModelRef(primary); err != nil {
		return fmt.Errorf("installer: OpenClaw primary model: %w", err)
	}
	for _, fallback := range fallbacks {
		if err := validateOpenClawModelRef(fallback); err != nil {
			return fmt.Errorf("installer: OpenClaw fallback model: %w", err)
		}
	}

	contract, err := qualifyOpenClawModelContract(commandRunnerFactory())
	if err != nil {
		return err
	}

	runner := commandRunnerFactory()
	if err := runner.Run(contract.BinaryPath, "config", "set", "agents.defaults.model.primary", primary); err != nil {
		return fmt.Errorf("installer: OpenClaw qualified contract failed while setting the primary model: %w", err)
	}
	if len(fallbacks) == 0 {
		return nil
	}
	fallbackJSON, err := json.Marshal(fallbacks)
	if err != nil {
		return fmt.Errorf("installer: encode OpenClaw fallback models: %w", err)
	}
	if err := runner.Run(contract.BinaryPath, "config", "set", "agents.defaults.model.fallbacks", string(fallbackJSON), openClawStrictJSONFlag); err != nil {
		return fmt.Errorf("installer: OpenClaw qualified contract failed while setting fallback models: %w", err)
	}
	return nil
}

func qualifyOpenClawModelContract(runner CommandRunner) (openClawQualifiedContract, error) {
	path, ok := OpenClawPath()
	if !ok {
		return openClawQualifiedContract{}, fmt.Errorf("installer: OpenClaw contract is not qualified because the binary is missing; install OpenClaw and re-run `click configure-openclaw-model`")
	}
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return openClawQualifiedContract{}, fmt.Errorf("installer: qualify OpenClaw contract: resolve absolute binary path: %w", err)
		}
		path = abs
	}
	workingDir, err := execCommandRunner{}.commandDir()
	if err != nil {
		return openClawQualifiedContract{}, fmt.Errorf("installer: qualify OpenClaw contract: resolve safe working directory: %w", err)
	}
	contract := openClawQualifiedContract{
		BinaryPath: path,
		ProbeArgs:  []string{"config", "set", "--help"},
		WorkingDir: workingDir,
		Timeout:    commandOutputTimeout,
	}
	out, err := runner.Output(contract.BinaryPath, contract.ProbeArgs...)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "timed out") {
			return openClawQualifiedContract{}, fmt.Errorf("installer: OpenClaw contract qualification timed out after %s in %s: %w", contract.Timeout, contract.WorkingDir, err)
		}
		return openClawQualifiedContract{}, fmt.Errorf("installer: OpenClaw contract qualification failed in %s: %w", contract.WorkingDir, err)
	}
	contract.Evidence = strings.TrimSpace(string(out))
	for _, token := range openClawQualificationTokens {
		if !strings.Contains(contract.Evidence, token) {
			return openClawQualifiedContract{}, fmt.Errorf("installer: OpenClaw contract probe returned unexpected output; missing %q in evidence %q", token, contract.Evidence)
		}
	}
	return contract, nil
}

func validateOpenClawModelRef(ref string) error {
	if strings.TrimSpace(ref) != ref || strings.Count(ref, "/") != 1 {
		return fmt.Errorf("model reference %q must use provider/model", ref)
	}
	parts := strings.SplitN(ref, "/", 2)
	if parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("model reference %q must use provider/model", ref)
	}
	return nil
}
