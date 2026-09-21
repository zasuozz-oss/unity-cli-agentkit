package initcmd

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// Skills point at each other with `@skill-name` in their Related Skills
// sections. A pointer to a skill the kit does not ship is a dead end for the
// agent reading it: it follows the reference, finds nothing, and falls back to
// guessing. Backticks are what distinguishes a reference from an email address
// or a PHPDoc tag in an example block.
var skillRef = regexp.MustCompile("`@([a-z0-9][a-z0-9-]*)`")

func TestSkillRefsResolveToShippedSkills(t *testing.T) {
	root := filepath.Join("..", "..", "skills")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read skills dir: %v", err)
	}
	shipped := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() {
			shipped[e.Name()] = true
		}
	}
	if len(shipped) == 0 {
		t.Fatal("no skills found — wrong root?")
	}
	for name := range shipped {
		path := filepath.Join(root, name, "SKILL.md")
		body, err := os.ReadFile(path)
		if err != nil {
			continue // not every dir has to be a skill
		}
		for _, m := range skillRef.FindAllSubmatch(body, -1) {
			ref := string(m[1])
			if !shipped[ref] {
				t.Errorf("%s references `@%s`, which the kit does not ship", path, ref)
			}
		}
	}
}
