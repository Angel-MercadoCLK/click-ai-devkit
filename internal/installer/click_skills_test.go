package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestClickSkills_SkillNameMatchesDirectory guards the domain-prefix invariant for the vendored
// click-skills plugin: every plugins/click-skills/skills/<dir>/SKILL.md must exist and its `name:`
// frontmatter value must equal its own directory name. A future re-vendoring that forgets to
// rename the frontmatter would otherwise produce a skill name collision the moment Claude Code
// tries to load two skills sharing the same `name:` (e.g. two upstream "security" skills vendored
// without their domain prefix) — this test catches that at build time instead of at runtime inside
// a developer's Claude Code session.
func TestClickSkills_SkillNameMatchesDirectory(t *testing.T) {
	skillsDir := filepath.Join("..", "..", "plugins", "click-skills", "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", skillsDir, err)
	}

	seenNames := map[string]string{} // name -> first directory that declared it
	dirCount := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirCount++
		dir := entry.Name()
		skillPath := filepath.Join(skillsDir, dir, "SKILL.md")
		data, err := os.ReadFile(skillPath)
		if err != nil {
			t.Errorf("dir %q: expected %s to exist: %v", dir, skillPath, err)
			continue
		}
		name, ok := frontmatterName(string(data))
		if !ok {
			t.Errorf("dir %q: %s has no `name:` frontmatter line", dir, skillPath)
			continue
		}
		if name != dir {
			t.Errorf("dir %q: SKILL.md name: %q, want it to equal the directory name %q", dir, name, dir)
		}
		if prevDir, exists := seenNames[name]; exists {
			t.Errorf("skill name %q declared by both %q and %q — skill names must be unique across click-skills", name, prevDir, dir)
		} else {
			seenNames[name] = dir
		}
	}
	if dirCount == 0 {
		t.Fatal("no skill directories found under plugins/click-skills/skills — expected the vendored content to be populated")
	}
}

// TestClickSkills_LicenseAndNoticePresent guards that a future re-vendoring of click-skills cannot
// silently drop upstream attribution: plugins/click-skills/LICENSE and NOTICE must both exist and
// be non-empty.
func TestClickSkills_LicenseAndNoticePresent(t *testing.T) {
	for _, name := range []string{"LICENSE", "NOTICE"} {
		path := filepath.Join("..", "..", "plugins", "click-skills", name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("expected %s to exist: %v", path, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("%s is empty, want attribution content", path)
		}
	}
}

// frontmatterName extracts the value of the `name:` key from a SKILL.md's YAML frontmatter (the
// block between the first two `---` lines), without pulling in a YAML parser dependency just for
// this one field.
func frontmatterName(content string) (string, bool) {
	lines := strings.Split(content, "\n")
	inFrontmatter := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			if inFrontmatter {
				break
			}
			inFrontmatter = true
			continue
		}
		if !inFrontmatter {
			continue
		}
		if rest, ok := strings.CutPrefix(trimmed, "name:"); ok {
			return strings.TrimSpace(rest), true
		}
	}
	return "", false
}
