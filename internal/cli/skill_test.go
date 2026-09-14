package cli

import (
	"os"
	"strings"
	"testing"
)

const skillPath = "../../skills/transpareo/SKILL.md"

// TestSkillNamesOnlyExistingCommands asserts every command the
// skill names is in the tree, and that its reference section is
// current.
func TestSkillNamesOnlyExistingCommands(t *testing.T) {
	data, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	skill := unixLines(data)
	app := &App{Getenv: func(string) string { return "" }}
	root := app.Root()
	known := map[string]bool{}
	for _, c := range Surface(root) {
		known[c.Path] = true
		for path := c.Path; strings.Contains(path, " "); {
			path = path[:strings.LastIndex(path, " ")]
			known[path] = true
		}
	}
	for _, name := range CommandsNamedIn(skill) {
		if !known[name] {
			t.Errorf("the skill names %q, which is not a command", name)
		}
	}
	filled, err := app.FillReference(skill, root)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(filled) != strings.TrimSpace(skill) {
		t.Error("the skill's reference section is stale; run go generate ./...")
	}
	if !strings.HasPrefix(skill, "---\nname: transpareo\n") {
		t.Error("the skill needs its frontmatter")
	}
}

func TestCommandsNamedIn(t *testing.T) {
	got := CommandsNamedIn("Run `transpareo dpps validate --file x` then " +
		"`transpareo dpps publish <code>` and `transpareo me`; not `curl`.")
	want := "transpareo dpps validate|transpareo dpps publish|transpareo me"
	if strings.Join(got, "|") != want {
		t.Errorf("got %v", got)
	}
}
