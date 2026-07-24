package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ConfigureCodexModel changes only Codex's documented user-level `model` key. It preserves all
// unrelated TOML text and is called only after the user explicitly selected/configured a model.
func ConfigureCodexModel(codexHome, model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return fmt.Errorf("installer: Codex native mutation requires an explicit model; re-run with --codex-model <model>")
	}
	if strings.ContainsAny(model, "\r\n") {
		return fmt.Errorf("installer: Codex model must not contain line breaks")
	}
	path := filepath.Join(codexHome, "config.toml")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("installer: read Codex config.toml: %w", err)
	}
	parsed, err := parseCodexModelConfig(data)
	if err != nil {
		return err
	}
	if parsed.rootModelLine >= 0 {
		prefix := parsed.lines[parsed.rootModelLine][:len(parsed.lines[parsed.rootModelLine])-len(strings.TrimLeft(parsed.lines[parsed.rootModelLine], " \t"))]
		parsed.lines[parsed.rootModelLine] = prefix + "model = \"" + escapeCodexTOMLString(model) + "\""
	} else {
		parsed.lines = append([]string{"model = \"" + escapeCodexTOMLString(model) + "\""}, parsed.lines...)
	}
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		return fmt.Errorf("installer: create CODEX_HOME: %w", err)
	}
	content := strings.Join(parsed.lines, parsed.newline)
	if parsed.trailingNewline {
		content += parsed.newline
	}
	if err := atomicWriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("installer: write Codex config.toml: %w", err)
	}
	return nil
}

type codexConfigParseResult struct {
	lines           []string
	newline         string
	trailingNewline bool
	rootModelLine   int
}

func parseCodexModelConfig(data []byte) (codexConfigParseResult, error) {
	text := string(data)
	newline := "\n"
	if strings.Contains(text, "\r\n") {
		newline = "\r\n"
	}
	trailingNewline := strings.HasSuffix(text, newline)
	if trailingNewline {
		text = strings.TrimSuffix(text, newline)
	}
	lines := []string{}
	if text != "" {
		lines = strings.Split(text, newline)
	}
	result := codexConfigParseResult{lines: lines, newline: newline, trailingNewline: trailingNewline, rootModelLine: -1}

	inTable := false
	for i, line := range result.lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			if !strings.HasSuffix(trimmed, "]") {
				return codexConfigParseResult{}, fmt.Errorf("installer: parse Codex config.toml: invalid TOML table header on line %d", i+1)
			}
			inTable = true
			continue
		}
		key, _, err := parseTOMLKeyValue(trimmed)
		if err != nil {
			return codexConfigParseResult{}, fmt.Errorf("installer: parse Codex config.toml: %w", err)
		}
		if !inTable && key == "model" && result.rootModelLine == -1 {
			result.rootModelLine = i
		}
	}
	return result, nil
}

func parseTOMLKeyValue(line string) (string, string, error) {
	inQuote := false
	escaped := false
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\\':
			if inQuote {
				escaped = !escaped
				continue
			}
		case '"':
			if !escaped {
				inQuote = !inQuote
			}
		case '=':
			if inQuote {
				break
			}
			key := strings.TrimSpace(line[:i])
			value := strings.TrimSpace(line[i+1:])
			if key == "" || value == "" {
				return "", "", fmt.Errorf("invalid TOML key/value line %q", line)
			}
			if strings.Count(value, "\"")%2 != 0 {
				return "", "", fmt.Errorf("invalid TOML quoted value in line %q", line)
			}
			return key, value, nil
		}
		escaped = false
	}
	if inQuote {
		return "", "", fmt.Errorf("invalid TOML quoted value in line %q", line)
	}
	return "", "", fmt.Errorf("invalid TOML key/value line %q", line)
}

func escapeCodexTOMLString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}
