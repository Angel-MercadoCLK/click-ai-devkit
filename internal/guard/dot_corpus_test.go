package guard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"
)

// dotCorpusPath is the golden corpus Click Dot also runs against its JavaScript conversion of these
// patterns, so both engines must agree on every case. All corpus data is synthetic.
var dotCorpusPath = filepath.Join("..", "..", "dot", "test", "fixtures", "guard-corpus.json")

type dotCorpus struct {
	Cases []struct {
		Text  string   `json:"text"`
		Rules []string `json:"rules"`
	} `json:"cases"`
}

func TestEmbeddedPatternsMatchClickDotCorpus(t *testing.T) {
	raw, err := os.ReadFile(dotCorpusPath)
	if err != nil {
		t.Fatalf("read Click Dot corpus: %v", err)
	}
	var corpus dotCorpus
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatalf("parse Click Dot corpus: %v", err)
	}
	if len(corpus.Cases) == 0 {
		t.Fatal("Click Dot corpus has no cases")
	}

	var cfg patternConfig
	if err := yaml.Unmarshal(embeddedPatterns, &cfg); err != nil {
		t.Fatalf("parse embedded patterns: %v", err)
	}
	type idRule struct {
		id string
		re *regexp.Regexp
	}
	var rules []idRule
	for _, category := range cfg.Categories {
		for _, rule := range category.Rules {
			rules = append(rules, idRule{id: rule.ID, re: regexp.MustCompile(rule.Pattern)})
		}
	}

	for _, tc := range corpus.Cases {
		var got []string
		for _, rule := range rules {
			if rule.re.MatchString(tc.Text) {
				got = append(got, rule.id)
			}
		}
		want := slices.Clone(tc.Rules)
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("corpus case %q: Go rules %v, corpus expects %v", tc.Text, got, want)
		}
	}
}
