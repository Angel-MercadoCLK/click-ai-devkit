package installer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func rawContentHash(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

type restoreOwnership uint8

const (
	restoreSkip restoreOwnership = iota
	restorePrivateFile
	restoreManagedMarkdown
	restoreSettingsHooks
)

func normalizedAbsolutePath(path string) (string, bool) {
	if path == "" || !filepath.IsAbs(path) {
		return "", false
	}
	return filepath.Clean(path), true
}

func isCanonicalAbsolutePath(path string) bool {
	clean, ok := normalizedAbsolutePath(path)
	return ok && path == clean
}

func pathInSet(path string, candidates ...string) bool {
	clean, ok := normalizedAbsolutePath(path)
	if !ok {
		return false
	}
	for _, candidate := range candidates {
		if candidateClean, candidateOK := normalizedAbsolutePath(candidate); candidateOK && clean == candidateClean {
			return true
		}
	}
	return false
}

func classifyRestorePath(cfg Config, path string) (restoreOwnership, bool) {
	if !filepath.IsAbs(path) {
		return restoreSkip, false
	}
	if cfg.ClaudeHome != "" {
		if pathInSet(path, cfg.ClaudeMDPath()) {
			return restoreManagedMarkdown, false
		}
		if pathInSet(path, cfg.SettingsPath()) {
			return restoreSettingsHooks, false
		}
		if pathInSet(path, cfg.Context7ConfigPath(), cfg.KnownMarketplacesPath(), cfg.InstalledPluginsPath()) {
			return restoreSkip, true
		}
	}
	if cfg.CodexHome != "" && pathInSet(path, cfg.CodexAgentsMDPath()) {
		return restoreManagedMarkdown, false
	}
	if cfg.CodexHome != "" && pathInSet(path, cfg.CodexConfigPath()) {
		return restoreSkip, true
	}
	if cfg.OpenClawHome != "" {
		if pathInSet(path, cfg.OpenClawAgentsMDPath(), cfg.OpenClawSoulMDPath()) {
			return restoreManagedMarkdown, false
		}
		if pathInSet(path, cfg.OpenClawMCPConfigPath()) {
			return restoreSkip, true
		}
		for _, rel := range openClawPluginRelPaths {
			if pathInSet(path, filepath.Join(cfg.OpenClawPluginDir(), filepath.FromSlash(rel))) {
				return restorePrivateFile, false
			}
		}
		for _, rel := range openClawSkillRelPaths {
			if pathInSet(path, filepath.Join(cfg.OpenClawSkillsDir(), filepath.FromSlash(rel))) {
				return restorePrivateFile, false
			}
		}
		if isRetiredOpenClawSkillPath(cfg, path) {
			return restorePrivateFile, false
		}
	}
	private := []string{}
	if cfg.ClaudeHome != "" {
		private = append(private,
			cfg.EngramStatePath(), cfg.EngramCloudStatePath(), cfg.EngramCloudImportOutcomePath(),
			cfg.Context7StatePath(), cfg.ModelsPath(), cfg.LegacyTargetSelectionPath(),
		)
	}
	if cfg.CodexHome != "" {
		private = append(private, cfg.CodexModelProfilePath())
	}
	if cfg.OpenClawHome != "" {
		private = append(private, cfg.OpenClawModelProfilePath())
	}
	if cfg.ClickStateHome != "" {
		private = append(private, cfg.TargetSelectionPath())
	}
	if pathInSet(path, private...) {
		return restorePrivateFile, false
	}
	return restoreSkip, false
}

// checkRestorePath prevents a manifest destination or its backup path from redirecting through a
// symlink. Missing components are allowed; every existing component must be a directory, except
// the final destination which must be a regular file when present.
func checkRestorePath(path string, leafMayBeFile bool) error {
	clean, ok := normalizedAbsolutePath(path)
	if !ok || clean != path {
		return fmt.Errorf("installer: la ruta de restauración debe ser absoluta y canónica")
	}
	volume := filepath.VolumeName(clean)
	remainder := strings.TrimPrefix(clean, volume)
	current := volume + string(filepath.Separator)
	parts := strings.Split(strings.TrimPrefix(remainder, string(filepath.Separator)), string(filepath.Separator))
	for i, part := range parts {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("installer: no se pudo inspeccionar una ruta de restauración")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("installer: la ruta de restauración contiene un componente redirigido")
		}
		isLeaf := i == len(parts)-1
		if isLeaf && leafMayBeFile {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("installer: el destino de restauración no es un archivo regular")
			}
		} else if !info.IsDir() {
			return fmt.Errorf("installer: un componente de la ruta de restauración no es un directorio")
		}
	}
	return nil
}

func extractManagedBlock(content string) (string, bool, error) {
	start, end, hasBlock, err := managedBlockSpan(content)
	if err != nil || !hasBlock {
		return "", hasBlock, err
	}
	return content[start:end], true, nil
}

func managedBlockSpan(content string) (int, int, bool, error) {
	type rawLine struct {
		text       string
		start, end int
	}
	lines := make([]rawLine, 0)
	for offset := 0; offset < len(content); {
		start := offset
		for offset < len(content) && content[offset] != '\n' {
			offset++
		}
		if offset < len(content) {
			offset++
		}
		lineEnd := offset
		text := content[start:lineEnd]
		text = strings.TrimSuffix(text, "\n")
		text = strings.TrimSuffix(text, "\r")
		lines = append(lines, rawLine{text: text, start: start, end: lineEnd})
	}
	beginCount, endCount := 0, 0
	logical := make([]string, len(lines))
	for i, line := range lines {
		logical[i] = line.text
		if line.text == managedBeginMarker {
			beginCount++
		}
		if line.text == managedEndMarker {
			endCount++
		}
	}
	if beginCount == 0 && endCount == 0 {
		return 0, 0, false, nil
	}
	begin, end := findMarkers(logical)
	if beginCount != 1 || endCount != 1 || begin < 0 || end < 0 {
		return 0, 0, false, fmt.Errorf("installer: los marcadores de Markdown gestionado son ambiguos")
	}
	return lines[begin].start, lines[end].end, true, nil
}

func prepareMarkdownRestore(current, backup []byte, existed bool) ([]byte, bool, bool, error) {
	_, currentHas, err := extractManagedBlock(string(current))
	if err != nil {
		return nil, false, false, err
	}
	backupBlock, backupHas := "", false
	if existed {
		backupBlock, backupHas, err = extractManagedBlock(string(backup))
		if err != nil {
			return nil, false, false, err
		}
	}
	if !currentHas {
		if !backupHas {
			return nil, false, false, nil
		}
		if len(current) == 0 || strings.HasSuffix(string(current), "\n") || strings.HasSuffix(string(current), "\r") {
			return append(append([]byte(nil), current...), []byte(backupBlock)...), true, false, nil
		}
		ending := detectLineEnding(string(current))
		return append(append(append([]byte(nil), current...), []byte(ending)...), []byte(backupBlock)...), true, false, nil
	}
	updated := append([]byte(nil), current...)
	if backupHas {
		start, end, _, _ := managedBlockSpan(string(current))
		suffix := current[end:]
		currentBlock := string(current[start:end])
		if len(suffix) > 0 && !endsWithLineBreak(backupBlock) && !startsWithLineBreak(string(suffix)) {
			backupBlock += managedBlockLineEnding(currentBlock, string(current))
		}
		updated = append(append(append([]byte(nil), current[:start]...), []byte(backupBlock)...), current[end:]...)
	} else {
		withoutBlock, _ := removeManagedBlock(string(updated))
		updated = []byte(withoutBlock)
	}
	if !existed && strings.TrimSpace(string(updated)) == "" {
		return nil, false, true, nil
	}
	if bytes.Equal(updated, current) {
		return nil, false, false, nil
	}
	return updated, true, false, nil
}

func endsWithLineBreak(value string) bool {
	return strings.HasSuffix(value, "\n") || strings.HasSuffix(value, "\r")
}

func startsWithLineBreak(value string) bool {
	return strings.HasPrefix(value, "\n") || strings.HasPrefix(value, "\r")
}

func managedBlockLineEnding(block, document string) string {
	if strings.HasSuffix(block, "\r\n") {
		return "\r\n"
	}
	if strings.HasSuffix(block, "\n") {
		return "\n"
	}
	return detectLineEnding(document)
}

func parseSettingsStrict(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := decodeJSONValue(decoder)
	if err != nil {
		return nil, fmt.Errorf("installer: el respaldo de settings.json no es válido")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("installer: el respaldo de settings.json no es válido")
	}
	settings, ok := value.(map[string]any)
	if !ok || settings == nil {
		return nil, fmt.Errorf("installer: el respaldo de settings.json no es válido")
	}
	if hooksValue, ok := settings["hooks"]; ok {
		hooks, ok := hooksValue.(map[string]any)
		if !ok || hooks == nil {
			return nil, fmt.Errorf("installer: la estructura de hooks de settings.json no es válida")
		}
		for _, event := range dotHookEvents {
			eventEntries, ok := hooks[event.name]
			if !ok {
				continue
			}
			entries, ok := eventEntries.([]any)
			if !ok {
				return nil, fmt.Errorf("installer: la estructura de hooks de settings.json no es válida")
			}
			for _, rawEntry := range entries {
				entry, ok := rawEntry.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("installer: la estructura de hooks de settings.json no es válida")
				}
				if matcher, ok := entry["matcher"]; ok {
					if _, ok := matcher.(string); !ok {
						return nil, fmt.Errorf("installer: la estructura de hooks de settings.json no es válida")
					}
				}
				if hooks, ok := entry["hooks"]; ok {
					if _, ok := hooks.([]any); !ok {
						return nil, fmt.Errorf("installer: la estructura de hooks de settings.json no es válida")
					}
				}
			}
		}
	}
	return settings, nil
}

func decodeJSONValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delim, isDelim := token.(json.Delim)
	if !isDelim {
		return token, nil
	}
	switch delim {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, fmt.Errorf("invalid JSON object key")
			}
			if _, exists := object[key]; exists {
				return nil, fmt.Errorf("duplicate JSON object key")
			}
			value, err := decodeJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return nil, fmt.Errorf("invalid JSON object")
		}
		return object, nil
	case '[':
		array := make([]any, 0)
		for decoder.More() {
			value, err := decodeJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return nil, fmt.Errorf("invalid JSON array")
		}
		return array, nil
	default:
		return nil, fmt.Errorf("invalid JSON delimiter")
	}
}

func clickGuardHooks(settings map[string]any) []map[string]any {
	var result []map[string]any
	for _, raw := range getPreToolUseEntries(settings) {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		hooks, _ := entry["hooks"].([]any)
		for _, hook := range hooks {
			if isClickGuardHook(hook) {
				result = append(result, map[string]any{"matcher": entry["matcher"], "hook": hook})
			}
		}
	}
	return result
}

func prepareSettingsRestore(current, backup []byte, existed bool) ([]byte, bool, bool, error) {
	currentSettings, err := parseSettingsStrict(current)
	if err != nil {
		return nil, false, false, err
	}
	var backupSettings map[string]any
	if existed {
		backupSettings, err = parseSettingsStrict(backup)
		if err != nil {
			return nil, false, false, err
		}
	} else {
		backupSettings = map[string]any{}
	}
	originalState, err := json.Marshal(currentSettings)
	if err != nil {
		return nil, false, false, fmt.Errorf("installer: no se pudo validar settings.json")
	}
	entries := getPreToolUseEntries(currentSettings)
	kept, _ := removeClickGuardHooks(entries, func(string) bool { return false })
	setPreToolUseEntries(currentSettings, kept)
	for _, saved := range clickGuardHooks(backupSettings) {
		matcher := saved["matcher"]
		hook := saved["hook"]
		entries = getPreToolUseEntries(currentSettings)
		merged := false
		for _, raw := range entries {
			entry, ok := raw.(map[string]any)
			if !ok || entry["matcher"] != matcher {
				continue
			}
			hooks, ok := entry["hooks"].([]any)
			if !ok {
				return nil, false, false, fmt.Errorf("installer: la estructura de hooks de settings.json no es válida")
			}
			entry["hooks"] = append(hooks, hook)
			merged = true
			break
		}
		if !merged {
			entries = append(entries, map[string]any{"matcher": matcher, "hooks": []any{hook}})
			setPreToolUseEntries(currentSettings, entries)
		}
	}
	if err := restoreClickDotHooks(currentSettings, backupSettings); err != nil {
		return nil, false, false, err
	}
	updatedState, err := json.Marshal(currentSettings)
	if err != nil {
		return nil, false, false, fmt.Errorf("installer: no se pudo validar settings.json")
	}
	if bytes.Equal(originalState, updatedState) {
		return nil, false, false, nil
	}
	if len(currentSettings) == 0 {
		return nil, false, true, nil
	}
	data, err := json.MarshalIndent(currentSettings, "", "  ")
	if err != nil {
		return nil, false, false, fmt.Errorf("installer: no se pudo preparar settings.json")
	}
	data = append(data, '\n')
	if string(data) == string(current) {
		return nil, false, false, nil
	}
	return data, true, false, nil
}

func validateRetiredSkillBackup(cfg Config, entry manifestEntry, backup []byte) error {
	if !isRetiredOpenClawSkillPath(cfg, entry.OriginalPath) {
		return nil
	}
	actual := sha256.Sum256(backup)
	for _, retired := range retiredOpenClawSkills {
		if filepath.Clean(filepath.Join(cfg.OpenClawSkillsDir(), filepath.FromSlash(retired.rel))) == filepath.Clean(entry.OriginalPath) {
			if hex.EncodeToString(actual[:]) != retired.sha256 {
				return fmt.Errorf("installer: el respaldo de una skill retirada no coincide con los bytes heredados verificados")
			}
			return nil
		}
	}
	return fmt.Errorf("installer: el respaldo de una skill retirada no está verificado")
}

func retiredSkillSignatureMatches(cfg Config, path string, content []byte) bool {
	if cfg.OpenClawHome == "" {
		return false
	}
	digest := sha256.Sum256(content)
	actual := hex.EncodeToString(digest[:])
	for _, retired := range retiredOpenClawSkills {
		candidate := filepath.Clean(filepath.Join(cfg.OpenClawSkillsDir(), filepath.FromSlash(retired.rel)))
		if candidate == filepath.Clean(path) {
			return actual == retired.sha256
		}
	}
	return false
}
