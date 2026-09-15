package spec

import "testing"

func TestCatalogueCarriesTheSpecificationVersion(t *testing.T) {
	version, tools := Catalogue()
	if version != Version() {
		t.Errorf("the catalogue names %s and the specification %s; the two "+
			"are vendored together", version, Version())
	}
	if len(tools) == 0 {
		t.Fatal("the vendored catalogue carries no tools")
	}
}

// A destructive tool without a phrase compares equal to any other
// tool without one, so the comparison would pass on a wording
// change that took the phrase away.
func TestEveryDestructiveCuratedToolStatesItsConfirmPhrase(t *testing.T) {
	_, tools := Catalogue()
	for name, tool := range tools {
		if tool.Operation == "" || !tool.Destructive {
			continue
		}
		if tool.Confirm == "" {
			t.Errorf("%s is destructive and no confirm phrase can be read "+
				"for it; the catalogue states neither x-transpareo.confirm "+
				"nor the `Must be \"...\"` wording", name)
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
