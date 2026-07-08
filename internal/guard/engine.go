package guard

import (
	_ "embed"
	"fmt"
	"regexp"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed patterns.yaml
var embeddedPatterns []byte

// Decision is the deterministic outcome of scanning a mem_save payload.
type Decision struct {
	Blocked  bool
	Category string
	Reason   string
}

type patternConfig struct {
	Categories []patternCategory `yaml:"categories"`
}

type patternCategory struct {
	Name   string        `yaml:"name"`
	Reason string        `yaml:"reason"`
	Rules  []patternRule `yaml:"rules"`
}

type patternRule struct {
	ID          string `yaml:"id"`
	Description string `yaml:"description"`
	Pattern     string `yaml:"pattern"`
}

type compiledRule struct {
	category string
	reason   string
	re       *regexp.Regexp
}

var (
	loadRulesOnce sync.Once
	loadRules     []compiledRule
	loadErr       error
)

// Scan is the pure v0.1 decision engine: block the payload when a forbidden pattern matches.
// If the underlying pattern set cannot be loaded, Scan fails closed by returning a blocked decision.
func Scan(payload string) Decision {
	decision, err := ScanWithError(payload)
	if err != nil {
		return Decision{Blocked: true, Category: "internal", Reason: "blocked: memory guard unavailable"}
	}
	return decision
}

// ScanWithError returns the decision and preserves internal loader/compiler errors for callers that
// must fail closed with a specific process exit code (the PreToolUse hook command).
func ScanWithError(payload string) (Decision, error) {
	rules, err := compiledRules()
	if err != nil {
		return Decision{}, err
	}
	for _, rule := range rules {
		if rule.re.MatchString(payload) {
			return Decision{Blocked: true, Category: rule.category, Reason: rule.reason}, nil
		}
	}
	return Decision{}, nil
}

func compiledRules() ([]compiledRule, error) {
	loadRulesOnce.Do(func() {
		var cfg patternConfig
		if err := yaml.Unmarshal(embeddedPatterns, &cfg); err != nil {
			loadErr = fmt.Errorf("guard: parse patterns: %w", err)
			return
		}

		var compiled []compiledRule
		for _, category := range cfg.Categories {
			for _, rule := range category.Rules {
				re, err := regexp.Compile(rule.Pattern)
				if err != nil {
					loadErr = fmt.Errorf("guard: compile %s: %w", rule.ID, err)
					return
				}
				compiled = append(compiled, compiledRule{
					category: category.Name,
					reason:   category.Reason,
					re:       re,
				})
			}
		}
		loadRules = compiled
	})
	return loadRules, loadErr
}
