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
