package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestSurfaceSnapshotIsCurrent fails when the command tree and
// surface.json differ. Run `go generate ./...` to refresh the
// snapshot; a removed or renamed command is a breaking change and
// needs a major version.
func TestSurfaceSnapshotIsCurrent(t *testing.T) {
	data, err := os.ReadFile("../../surface.json")
	if err != nil {
		t.Fatalf("surface.json: %v (run go generate ./...)", err)
	}
	var snapshot []SurfaceCommand
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	app := &App{Getenv: func(string) string { return "" }}
	current := Surface(app.Root())
	byPath := map[string]SurfaceCommand{}
	for _, c := range current {
		byPath[c.Path] = c
	}
	for _, c := range snapshot {
		live, ok := byPath[c.Path]
		if !ok {
			t.Errorf("%q is in surface.json but not in the tree: removing a "+
				"command is a breaking change", c.Path)
			continue
		}
		if c.Args != live.Args {
			t.Errorf("%q: args %q in surface.json, %q in the tree", c.Path,
				c.Args, live.Args)
		}
		for _, flag := range c.Flags {
			if !contains(live.Flags, flag) {
				t.Errorf("%q lost the option --%s", c.Path, flag)
			}
		}
	}
	want, _ := SurfaceJSON(app.Root())
	if strings.TrimSpace(string(want)) != strings.TrimSpace(unixLines(data)) {
		t.Error("surface.json is stale; run go generate ./...")
	}
}

// unixLines undoes the CRLF conversion a Windows checkout applies
// to text files.
func unixLines(data []byte) string {
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}

func TestMarkdownReference(t *testing.T) {
	app := &App{Getenv: func(string) string { return "" }}
	doc, err := app.Markdown(app.Root())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## General", "## DPPs",
		"### transpareo dpps void", "transpareo dpps void <id>",
		"permission `dpp_lifecycle`", "cannot be undone, needs `--yes`",
		"### transpareo api", "### transpareo me", "operation `get_me`",
		"| `--per-page` \\<n\\> |", "[DPPs](#dpps)",
		"| Option on every command |"} {
		if !strings.Contains(doc, want) {
			t.Errorf("reference lacks %q", want)
		}
	}
	current, _ := os.ReadFile("../../docs/cli.md")
	if strings.TrimSpace(unixLines(current)) != strings.TrimSpace(doc) {
		t.Error("docs/cli.md is stale; run go generate ./...")
	}
}

// A command whose example is the usage line prints it once. The
// two are built from different halves of the specification and
// land on the same string often enough that the reference used to
// show a doubled call under a third of the get commands.
func TestMarkdownReferencePrintsACallOnce(t *testing.T) {
	app := &App{Getenv: func(string) string { return "" }}
	doc, err := app.Markdown(app.Root())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc, "```sh\ntranspareo imports get <id>\n```") {
		t.Error("transpareo imports get does not print its one call alone")
	}
	var previous string
	for _, line := range strings.Split(doc, "\n") {
		if strings.HasPrefix(line, "transpareo ") && line == previous {
			t.Errorf("the reference prints %q twice in a row", line)
		}
		previous = line
	}
}
