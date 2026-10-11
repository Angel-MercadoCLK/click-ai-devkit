package installer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const (
	clickSDDRetirementScope = "user"
	clickSDDRetirementDir   = "retired-plugin-options"
)

type claudePluginInstallation struct {
	ID          string `json:"id"`
	Scope       string `json:"scope"`
	ProjectPath string `json:"projectPath"`
}

type clickSDDRetirementBackup struct {
	PluginID     string          `json:"pluginId"`
	Scope        string          `json:"scope"`
	ConfigExists bool            `json:"configExists"`
	PluginConfig json.RawMessage `json:"pluginConfig,omitempty"`
}

var (
	retirementFileSync = func(file *os.File) error { return file.Sync() }
	retirementDirSync  = syncRetirementDirectory
)

type safeRetirementError struct {
	message string
	cause   error
}

func (e *safeRetirementError) Error() string { return e.message }
func (e *safeRetirementError) Unwrap() error { return e.cause }

// RetireLegacyClickSDD backs up the exact user-scoped click-sdd plugin configuration before asking
// Claude Code to uninstall that plugin. It never changes Claude's settings or registry files.
// Other plugin IDs and non-user installations are left untouched by this operation.
func RetireLegacyClickSDD(cfg Config) (string, error) {
	return retireLegacyClickSDDWithRunner(cfg, commandRunnerFactory())
}

func retireLegacyClickSDDWithRunner(cfg Config, runner CommandRunner) (string, error) {
	if cfg.ClaudeHome == "" {
		return "", fmt.Errorf("installer: no se puede retirar click-sdd sin un directorio de Claude configurado")
	}
	if runner == nil {
		return "", fmt.Errorf("installer: no se puede retirar click-sdd sin un ejecutor de Claude disponible")
	}

	output, err := runner.Output(pluginCLIBinary, "plugin", "list", "--json")
	if err != nil {
		return "", &safeRetirementError{message: "installer: no se pudo consultar la lista de plugins de Claude; no se retiró click-sdd", cause: err}
	}
	installations, err := parseClaudePluginInventory(output)
	if err != nil {
		return "", err
	}
	userInstallCount := 0
	for _, installation := range installations {
		if installation.ID == ClickSDDPluginID && installation.Scope == clickSDDRetirementScope {
			userInstallCount++
		}
	}
	if userInstallCount > 1 {
		return "", fmt.Errorf("installer: la lista de plugins de Claude contiene instalaciones de usuario ambiguas de click-sdd; no se retiró el plugin")
	}
	if userInstallCount == 0 {
		return "", nil
	}

	backup, err := loadClickSDDPluginConfig(cfg.SettingsPath())
	if err != nil {
		return "", err
	}
	backupPath, err := persistClickSDDRetirementBackup(cfg.ClickStateHome, backup)
	if err != nil {
		return "", err
	}

	if err := runner.Run(pluginCLIBinary, "plugin", "uninstall", ClickSDDPluginID, "--scope", clickSDDRetirementScope, "--keep-data"); err != nil {
		return backupPath, &safeRetirementError{message: fmt.Sprintf("installer: Claude no pudo retirar click-sdd con alcance de usuario; la copia de recuperación se conservó en %s", backupPath), cause: err}
	}
	return backupPath, nil
}

func parseClaudePluginInventory(data []byte) ([]claudePluginInstallation, error) {
	var installations []claudePluginInstallation
	if err := json.Unmarshal(data, &installations); err != nil || installations == nil {
		return nil, fmt.Errorf("installer: la lista de plugins de Claude no tiene el formato JSON esperado; no se retiró click-sdd")
	}
	for _, installation := range installations {
		if installation.ID == "" || installation.Scope == "" {
			return nil, fmt.Errorf("installer: la lista de plugins de Claude omite una identidad o alcance; no se retiró click-sdd")
		}
		switch installation.Scope {
		case "user", "project", "local", "managed", "session", "synced":
		default:
			return nil, fmt.Errorf("installer: la lista de plugins de Claude contiene un alcance desconocido; no se retiró click-sdd")
		}
	}
	return installations, nil
}

func loadClickSDDPluginConfig(settingsPath string) (clickSDDRetirementBackup, error) {
	backup := clickSDDRetirementBackup{PluginID: ClickSDDPluginID, Scope: clickSDDRetirementScope}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return backup, nil
		}
		return clickSDDRetirementBackup{}, fmt.Errorf("installer: no se pudo leer la configuración de Claude; no se retiró click-sdd")
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(data, &settings); err != nil || settings == nil {
		return clickSDDRetirementBackup{}, fmt.Errorf("installer: la configuración de Claude no es un objeto JSON válido; no se retiró click-sdd")
	}
	pluginConfigsData, exists := settings["pluginConfigs"]
	if !exists {
		return backup, nil
	}
	var pluginConfigs map[string]json.RawMessage
	if err := json.Unmarshal(pluginConfigsData, &pluginConfigs); err != nil || pluginConfigs == nil {
		return clickSDDRetirementBackup{}, fmt.Errorf("installer: pluginConfigs en la configuración de Claude no es un objeto válido; no se retiró click-sdd")
	}
	pluginConfig, exists := pluginConfigs[ClickSDDPluginID]
	if !exists {
		return backup, nil
	}
	trimmed := bytes.TrimSpace(pluginConfig)
	if len(trimmed) == 0 || trimmed[0] != '{' || !json.Valid(trimmed) {
		return clickSDDRetirementBackup{}, fmt.Errorf("installer: la configuración guardada de click-sdd no es un objeto válido; no se retiró el plugin")
	}
	backup.ConfigExists = true
	backup.PluginConfig = append(json.RawMessage(nil), trimmed...)
	return backup, nil
}

func persistClickSDDRetirementBackup(stateHome string, backup clickSDDRetirementBackup) (string, error) {
	if stateHome == "" || !filepath.IsAbs(stateHome) {
		return "", fmt.Errorf("installer: no hay un directorio absoluto de estado de Click para respaldar click-sdd; no se retiró el plugin")
	}
	_, stateHomeErr := os.Lstat(stateHome)
	if stateHomeErr != nil && !errors.Is(stateHomeErr, os.ErrNotExist) {
		return "", fmt.Errorf("installer: no se pudo validar el directorio de estado de Click para respaldar click-sdd")
	}
	if err := os.MkdirAll(stateHome, 0o700); err != nil {
		return "", fmt.Errorf("installer: no se pudo preparar el directorio de estado de Click para respaldar click-sdd")
	}
	stateInfo, err := os.Lstat(stateHome)
	if err != nil || !stateInfo.IsDir() || stateInfo.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("installer: el directorio de estado de Click para respaldar click-sdd no es seguro; no se retiró el plugin")
	}
	if err := retirementDirSync(filepath.Dir(stateHome)); err != nil {
		return "", fmt.Errorf("installer: no se pudo confirmar el directorio de estado de Click en disco; no se retiró el plugin")
	}
	data, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return "", fmt.Errorf("installer: no se pudo preparar la copia de recuperación de click-sdd")
	}
	data = append(data, '\n')
	sum := sha256.Sum256(data)
	filename := "click-sdd-" + hex.EncodeToString(sum[:]) + ".json"
	dir := filepath.Join(stateHome, clickSDDRetirementDir)
	dirInfo, dirErr := os.Lstat(dir)
	dirCreated := errors.Is(dirErr, os.ErrNotExist)
	if dirErr != nil && !dirCreated {
		return "", fmt.Errorf("installer: no se pudo validar el directorio de recuperación de click-sdd")
	}
	if dirCreated {
		if err := os.Mkdir(dir, 0o700); err != nil {
			return "", fmt.Errorf("installer: no se pudo crear el directorio privado de recuperación de click-sdd")
		}
		if err := setRetirementPathPrivacy(dir, true); err != nil {
			return "", fmt.Errorf("installer: no se pudo proteger el directorio de recuperación de click-sdd; no se retiró el plugin")
		}
		private, privacyErr := retirementPathPrivacySatisfied(dir, true)
		if privacyErr != nil || !private {
			return "", fmt.Errorf("installer: el directorio de recuperación de click-sdd no quedó privado; no se retiró el plugin")
		}
	} else if dirInfo.IsDir() && dirInfo.Mode()&os.ModeSymlink == 0 {
		private, privacyErr := retirementPathPrivacySatisfied(dir, true)
		if privacyErr != nil || !private {
			return "", fmt.Errorf("installer: el directorio de recuperación de click-sdd no es privado y seguro; no se retiró el plugin")
		}
	} else {
		return "", fmt.Errorf("installer: el directorio de recuperación de click-sdd no es privado y seguro; no se retiró el plugin")
	}
	if err := retirementDirSync(stateHome); err != nil {
		return "", fmt.Errorf("installer: no se pudo confirmar el directorio de recuperación de click-sdd en disco; no se retiró el plugin")
	}
	path := filepath.Join(dir, filename)
	info, err := os.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("installer: la copia de recuperación existente de click-sdd no es un archivo privado válido; no se retiró el plugin")
		}
		private, privacyErr := retirementPathPrivacySatisfied(path, false)
		if privacyErr != nil || !private {
			return "", fmt.Errorf("installer: la copia de recuperación existente de click-sdd no es un archivo privado válido; no se retiró el plugin")
		}
		existing, readErr := os.ReadFile(path)
		if readErr != nil || !bytes.Equal(existing, data) {
			return "", fmt.Errorf("installer: la copia de recuperación existente de click-sdd no coincide; no se retiró el plugin")
		}
		file, openErr := os.OpenFile(path, os.O_RDWR, 0)
		if openErr != nil {
			return "", fmt.Errorf("installer: no se pudo validar la copia existente de click-sdd en disco; no se retiró el plugin")
		}
		syncErr := retirementFileSync(file)
		closeErr := file.Close()
		if syncErr != nil || closeErr != nil {
			return "", fmt.Errorf("installer: no se pudo confirmar la copia existente de click-sdd en disco; no se retiró el plugin")
		}
		if err := retirementDirSync(dir); err != nil {
			return "", fmt.Errorf("installer: no se pudo confirmar la copia de recuperación de click-sdd en disco; no se retiró el plugin")
		}
		return path, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("installer: no se pudo validar la copia de recuperación de click-sdd")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("installer: no se pudo crear la copia de recuperación de click-sdd; no se retiró el plugin")
	}
	if err := setRetirementPathPrivacy(path, false); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("installer: no se pudo proteger la copia de recuperación de click-sdd; no se retiró el plugin")
	}
	if private, err := retirementPathPrivacySatisfied(path, false); err != nil || !private {
		_ = file.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("installer: la copia de recuperación de click-sdd no quedó privada; no se retiró el plugin")
	}
	writeErr := writeAndSyncRetirementFile(file, data)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("installer: no se pudo completar la copia de recuperación de click-sdd; no se retiró el plugin")
	}
	if err := retirementDirSync(dir); err != nil {
		return "", fmt.Errorf("installer: no se pudo confirmar la copia de recuperación de click-sdd en disco; no se retiró el plugin")
	}
	return path, nil
}

func writeAndSyncRetirementFile(file *os.File, data []byte) error {
	if _, err := file.Write(data); err != nil {
		return err
	}
	return retirementFileSync(file)
}

func syncRetirementDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := dir.Sync()
	closeErr := dir.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}
