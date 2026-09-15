package spec

import (
	"encoding/json"
	"testing"
)

func TestVersion(t *testing.T) {
	v := Version()
	if v == "" {
		t.Fatal("embedded specification has no info.version")
	}
}

func TestJSONIsValidOpenAPI(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(JSON, &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, key := range []string{"openapi", "info", "paths", "components"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("missing top-level key %q", key)
		}
	}
}

func TestCompare(t *testing.T) {
	for _, tc := range []struct {
		builtIn, live string
		want          Drift
		ok            bool
	}{
		{"1.18.0", "1.18.0", Drift{}, true},
		{"1.18.0", "1.18.4", Drift{}, true},
		{"1.14.0", "1.18.0", Drift{Minor: 4}, true},
		{"1.18.0", "1.14.0", Drift{Minor: -4}, true},
		{"1.18.0", "2.0.0", Drift{Major: 1, Minor: -18}, true},
		{"2.0.0", "1.18.0", Drift{Major: -1, Minor: 18}, true},
		{"v1.18.0", "1.19.0", Drift{Minor: 1}, true},
		{"1.18", "1.19", Drift{Minor: 1}, true},
		{"1.18.0", "unreleased", Drift{}, false},
		{"", "1.18.0", Drift{}, false},
		{"1", "1.18.0", Drift{}, false},
	} {
		got, ok := Compare(tc.builtIn, tc.live)
		if ok != tc.ok || got != tc.want {
			t.Errorf("Compare(%q, %q) = %+v, %t; want %+v, %t",
				tc.builtIn, tc.live, got, ok, tc.want, tc.ok)
		}
	}
}
