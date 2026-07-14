package agentbuilder

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// finalMarkdownRequiredHeadings are the section headings RenderAgentMarkdown always
// produces. ValidateFinalMarkdown requires all of them so that a hand-edited preview
// can't silently drop a section the rest of the product (and any agent reading this
// file later) expects to exist.
var finalMarkdownRequiredHeadings = []string{
	"# Role", "## Tasks", "## Triggers", "## Hard Rules", "## SDD Integration",
	"## Tone", "## Domain Knowledge", "## Good Output",
}

// finalMarkdownAllowedFrontmatterKeys is the exact set of top-level frontmatter keys a
// confirmed agent markdown file may contain.
//
// Threat defended: rejecting anything outside this set stops a hand-edited preview
// from injecting an extra, unreviewed native Claude Code agent field (e.g. a forged
// permissions/trigger key) that the wizard never asked about and the user never
// explicitly confirmed (R2-004).
var finalMarkdownAllowedFrontmatterKeys = map[string]bool{
	"name":        true,
	"description": true,
	"model":       true,
	"tools":       true,
}

var implicitNumberScalarPattern = regexp.MustCompile(`^[+-]?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)

// ValidateFinalMarkdown validates a confirmed agent markdown document — either the
// wizard's generated draft or a user-edited version of it — before it is ever written
// to disk.
//
// It is exported, and InstallFinalMarkdown calls it internally, so the safety
// guarantees below apply to every caller, not just the interactive wizard: a caller
// cannot forget to validate (R1-001, R2-005).
//
// If expectedName is provided, the frontmatter "name" field must match it exactly.
// Threat defended: this stops a hand-edited preview from silently renaming the agent
// to collide with (or impersonate) a different agent than the one the user actually
// walked through the wizard for.
func ValidateFinalMarkdown(content string, expectedName ...string) error {
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("agentbuilder: final markdown is required")
	}
	normalized := normalizeLineBreaks(content)
	if !strings.HasPrefix(normalized, "---\n") {
		return fmt.Errorf("agentbuilder: final markdown must start with YAML frontmatter")
	}
	rest := strings.TrimPrefix(normalized, "---\n")
	frontmatterEnd := strings.Index(rest, "\n---\n")
	if frontmatterEnd < 0 {
		return fmt.Errorf("agentbuilder: final markdown must close YAML frontmatter")
	}
	frontmatter := rest[:frontmatterEnd]
	body := rest[frontmatterEnd+len("\n---\n"):]
	if err := validateFrontmatterIndentation(frontmatter); err != nil {
		return err
	}
	if err := validateNativeFrontmatterKeys(frontmatter); err != nil {
		return err
	}
	frontmatterValues := make(map[string]string, 4)
	for _, field := range []string{"name", "description", "model", "tools"} {
		value, err := frontmatterScalarValue(frontmatter, field)
		if err != nil {
			return err
		}
		frontmatterValues[field] = value
	}
	if err := ValidateAgentName(frontmatterValues["name"]); err != nil {
		return err
	}
	if len(expectedName) > 0 && frontmatterValues["name"] != expectedName[0] {
		return fmt.Errorf("agentbuilder: final markdown frontmatter name %q must match generated name %q", frontmatterValues["name"], expectedName[0])
	}
	for _, heading := range finalMarkdownRequiredHeadings {
		if !strings.Contains(body, heading+"\n") {
			return fmt.Errorf("agentbuilder: final markdown missing %s section", heading)
		}
	}
	return nil
}

// normalizeLineBreaks folds every Unicode line-break-like character onto \n before this
// package inspects final markdown line by line.
//
// Threat defended (R1-002): treating only \n/\r\n/\r as line breaks would let a crafted
// field smuggle a structural line break past this validator's \n-based line splitting
// using U+2028 (LINE SEPARATOR), U+2029 (PARAGRAPH SEPARATOR), or U+0085 (NEL) — all of
// which real terminals, browsers, and some YAML/JSON consumers DO treat as line breaks.
func normalizeLineBreaks(content string) string {
	replacer := strings.NewReplacer(
		"\r\n", "\n",
		"\r", "\n",
		" ", "\n",
		" ", "\n",
		"", "\n",
	)
	return replacer.Replace(content)
}

// validateFrontmatterIndentation rejects indented continuation lines in the frontmatter
// block.
//
// Threat defended: an indented line under a scalar field is valid YAML block-scalar
// syntax that can smuggle extra content — or a forged key, once re-parsed by a real
// YAML engine downstream — past this validator's simple line-based field parsing.
func validateFrontmatterIndentation(frontmatter string) error {
	for _, line := range strings.Split(frontmatter, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			return fmt.Errorf("agentbuilder: final markdown frontmatter must not contain indented continuation lines")
		}
	}
	return nil
}

// validateNativeFrontmatterKeys rejects any frontmatter key outside
// finalMarkdownAllowedFrontmatterKeys. See that variable's doc comment for the threat
// this defends against.
func validateNativeFrontmatterKeys(frontmatter string) error {
	for _, line := range strings.Split(frontmatter, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("agentbuilder: final markdown frontmatter line %q must be a top-level native Claude agent field", line)
		}
		key := parts[0]
		if key == "" || key != strings.TrimSpace(key) {
			return fmt.Errorf("agentbuilder: final markdown frontmatter line %q must use a valid top-level field name", line)
		}
		if !finalMarkdownAllowedFrontmatterKeys[key] {
			return fmt.Errorf("agentbuilder: final markdown frontmatter field %q is not allowed", key)
		}
	}
	return nil
}

// frontmatterScalarValue extracts and validates a single required top-level scalar
// field from frontmatter.
//
// Threat defended (duplicate-key smuggling): requiring uniqueness stops a hand-edited
// preview from defining the same key twice, where a naive downstream YAML parser might
// silently take the last value while this validator inspected the first (or vice
// versa) — a classic parser-differential injection vector.
func frontmatterScalarValue(frontmatter, field string) (string, error) {
	fieldPrefix := field + ":"
	separatorPrefix := field + ": "
	found := false
	var value string
	for _, line := range strings.Split(frontmatter, "\n") {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		if !strings.HasPrefix(line, fieldPrefix) {
			continue
		}
		if found {
			return "", fmt.Errorf("agentbuilder: final markdown frontmatter field %s must be unique", field)
		}
		found = true
		if !strings.HasPrefix(line, separatorPrefix) {
			rawWithoutSeparator := strings.TrimSpace(strings.TrimPrefix(line, fieldPrefix))
			if rawWithoutSeparator != "" {
				return "", fmt.Errorf("agentbuilder: final markdown frontmatter field %s must use 'field: value' separator", field)
			}
		}
		rawValue := strings.TrimSpace(strings.TrimPrefix(line, fieldPrefix))
		if rawValue == "" {
			return "", fmt.Errorf("agentbuilder: final markdown frontmatter field %s is required", field)
		}
		parsedValue, err := parseFrontmatterScalar(field, rawValue)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(parsedValue) == "" {
			return "", fmt.Errorf("agentbuilder: final markdown frontmatter field %s is required", field)
		}
		// Unicode line separators are folded to \n/\r by normalizeLineBreaks before we
		// ever reach here, so this single check also covers U+2028/U+2029/U+0085 (R1-002).
		if strings.ContainsAny(parsedValue, "\n\r") {
			return "", fmt.Errorf("agentbuilder: agent frontmatter field %s contains a newline", field)
		}
		value = parsedValue
	}
	if found {
		return value, nil
	}
	return "", fmt.Errorf("agentbuilder: final markdown frontmatter field %s is required", field)
}

// parseFrontmatterScalar decodes a single-line YAML-ish scalar into its string value,
// rejecting every scalar form except a safe plain string or an explicitly quoted
// string.
//
// Threat defended: block-scalars (|, >), comments, flow sequences/maps ([...], {...}),
// and implicit non-string types (true/false/null/numbers) are all valid YAML that a
// real downstream YAML parser would interpret differently than this line-based
// validator. Accepting them here would let a hand-edited preview smuggle content past
// validation that a real parser later reinterprets as something else entirely.
func parseFrontmatterScalar(field, rawValue string) (string, error) {
	if strings.HasPrefix(rawValue, "|") || strings.HasPrefix(rawValue, ">") {
		return "", fmt.Errorf("agentbuilder: final markdown frontmatter field %s must be a single-line scalar", field)
	}
	if strings.HasPrefix(rawValue, `"`) {
		value, err := strconv.Unquote(rawValue)
		if err != nil {
			return "", fmt.Errorf("agentbuilder: final markdown frontmatter field %s has invalid quoted scalar", field)
		}
		return value, nil
	}
	if strings.HasPrefix(rawValue, `'`) {
		if !strings.HasSuffix(rawValue, `'`) || len(rawValue) == 1 {
			return "", fmt.Errorf("agentbuilder: final markdown frontmatter field %s has invalid quoted scalar", field)
		}
		inner := strings.TrimSuffix(strings.TrimPrefix(rawValue, `'`), `'`)
		for i := 0; i < len(inner); i++ {
			if inner[i] != '\'' {
				continue
			}
			if i+1 >= len(inner) || inner[i+1] != '\'' {
				return "", fmt.Errorf("agentbuilder: final markdown frontmatter field %s has invalid quoted scalar", field)
			}
			i++
		}
		return strings.ReplaceAll(inner, `''`, `'`), nil
	}
	if strings.HasPrefix(rawValue, "#") || strings.HasPrefix(rawValue, "[") || strings.HasPrefix(rawValue, "{") {
		return "", fmt.Errorf("agentbuilder: final markdown frontmatter field %s must be a string scalar", field)
	}
	if isImplicitNonStringScalar(rawValue) {
		return "", fmt.Errorf("agentbuilder: final markdown frontmatter field %s must be a string scalar; quote the value", field)
	}
	if !isPlainSafeFrontmatterScalar(rawValue) {
		return "", fmt.Errorf("agentbuilder: final markdown frontmatter field %s has unsafe plain scalar; quote the value", field)
	}
	return rawValue, nil
}

// isImplicitNonStringScalar reports whether an unquoted plain scalar would be parsed by
// a real YAML engine as a bool/null/float rather than a string (e.g. `model: true`).
//
// Threat defended: silently accepting these as "the string true" here, while a
// downstream YAML parser reads them as an actual boolean, is a parser-differential that
// could flip a safety-relevant field's type unexpectedly.
func isImplicitNonStringScalar(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "false", "null", "~", ".nan", ".inf", "+.inf", "-.inf":
		return true
	}
	return implicitNumberScalarPattern.MatchString(value)
}

// isPlainSafeFrontmatterScalar allow-lists the character set permitted in an unquoted
// plain scalar.
//
// Threat defended: YAML plain scalars have surprising special characters (e.g. "#"
// starting a comment mid-value); restricting to letters/digits/space/",.-_/" avoids
// ambiguous plain-scalar edge cases entirely rather than trying to replicate a full
// YAML plain-scalar grammar here.
func isPlainSafeFrontmatterScalar(value string) bool {
	for _, r := range value {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			continue
		case r == ' ', r == ',', r == '.', r == '-', r == '_', r == '/':
			continue
		default:
			return false
		}
	}
	return true
}
