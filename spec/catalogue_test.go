package spec

import "testing"

func TestCatalogueCarriesTheSpecificationVersion(t *testing.T) {
	doc := Catalogue()
	if doc.Version != Version() {
		t.Errorf("the catalogue names %s and the specification %s; the two "+
			"are vendored together", doc.Version, Version())
	}
	if len(doc.Tools) == 0 {
		t.Fatal("the vendored catalogue carries no tools")
	}
	if doc.Instructions == "" {
		t.Error("the vendored catalogue carries no instructions; the MCP " +
			"server's are generated from them")
	}
}

// The declaration is what both servers build a tool from, so a
// tool the catalogue curates over an operation has to state all
// of it. A missing kind would leave this side unable to turn
// arguments into a request; a missing confirm phrase on a
// destructive tool would let two empty strings compare equal.
func TestEveryCuratedToolStatesItsDeclaration(t *testing.T) {
	kinds := map[string]bool{"list": true, "get": true, "write": true,
		"rows": true}
	for name, tool := range Catalogue().Tools {
		if tool.Operation == "" {
			continue
		}
		if !kinds[tool.Kind] {
			t.Errorf("%s declares kind %q", name, tool.Kind)
		}
		if tool.Group == "" {
			t.Errorf("%s declares no group", name)
		}
		if tool.Destructive && tool.Confirm == "" {
			t.Errorf("%s is destructive and no confirm phrase can be read "+
				"for it; the catalogue states neither x-transpareo.confirm "+
				"nor the `Must be \"...\"` wording", name)
		}
	}
}

func TestCuratedNamesAreTheToolsOverAnOperation(t *testing.T) {
	doc := Catalogue()
	names := doc.CuratedNames()
	if len(names) == 0 {
		t.Fatal("no curated tools")
	}
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			t.Errorf("%s is listed twice", name)
		}
		seen[name] = true
		if doc.Tools[name].Operation == "" {
			t.Errorf("%s has no operation and is listed as curated", name)
		}
	}
	for name, tool := range doc.Tools {
		if tool.Operation != "" && !seen[name] {
			t.Errorf("%s is curated over %s and is not listed", name,
				tool.Operation)
		}
	}
}

func TestConfirmPhrasePrefersTheStatedValue(t *testing.T) {
	for _, tc := range []struct {
		name        string
		stated      string
		description string
		want        string
	}{
		{"stated", "void <id>", "", "void <id>"},
		{"stated over prose", "void <id>", `Must be "delete <id>"`,
			"void <id>"},
		{"read from the prose", "", `Must be "delete <id>"`, "delete <id>"},
		{"neither", "", "Pass the phrase shown above", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := confirmPhrase(tc.stated, tc.description); got != tc.want {
				t.Errorf("confirmPhrase = %q, want %q", got, tc.want)
			}
		})
	}
}
