package mcp

import (
	"testing"

	"github.com/transpareo/transpareo-cli/internal/registry"
	"github.com/transpareo/transpareo-cli/spec"
)

// TestCatalogueMatchesTheHostedOne compares this server's tools
// with the catalogue the hosted assistant server publishes. A
// curated tool must agree field for field, because both sides
// derive it from the same specification. A composed tool is
// written twice by hand, so a difference is allowed, but only
// when hostedOnly or localOnly gives the reason.
func TestCatalogueMatchesTheHostedOne(t *testing.T) {
	version, hosted := spec.Catalogue()
	if version != spec.Version() {
		t.Fatalf("the vendored catalogue is %s and the specification %s; "+
			"re-vendor both, they move together", version, spec.Version())
	}

	reg := registry.Default()
	local := map[string]spec.CatalogueTool{}
	for _, tool := range curated {
		op := reg.Find(tool.Operation)
		if op == nil {
			continue
		}
		local[tool.Name] = spec.CatalogueTool{
			Name:        tool.Name,
			Group:       tool.Group,
			Operation:   tool.Operation,
			Confirm:     tool.confirmPhrase(op),
			Destructive: op.Destructive,
			Safe:        op.ReadOnly(),
		}
	}

	for name, want := range local {
		got, ok := hosted[name]
		if !ok {
			t.Errorf("curated tool %s is missing from the hosted "+
				"catalogue; a curated difference is a derivation bug, not "+
				"something hostedOnly or localOnly may excuse", name)
			continue
		}
		if got != want {
			t.Errorf("curated tool %s differs:\n hosted %+v\n  local %+v",
				name, got, want)
		}
	}

	for _, name := range New(Options{}).ToolNames() {
		if _, curated := local[name]; curated {
			continue
		}
		if _, ok := hosted[name]; ok {
			continue
		}
		if localOnly[name] == "" {
			t.Errorf("%s is a tool here and not in the hosted catalogue, "+
				"with no reason in localOnly", name)
		}
	}

	for name, tool := range hosted {
		if _, ok := local[name]; ok {
			continue
		}
		if serves(name) {
			continue
		}
		if tool.Operation != "" {
			t.Errorf("the hosted catalogue curates %s over %s and this "+
				"server does not; a curated difference is a derivation bug",
				name, tool.Operation)
			continue
		}
		if hostedOnly[name] == "" {
			t.Errorf("%s is a hosted tool this server does not carry, "+
				"with no reason in hostedOnly", name)
		}
	}
}

// TestAllowancesNameARealDifference keeps the two lists honest:
// an entry that no longer describes a difference is noise, and a
// reason is the whole point of the entry.
func TestAllowancesNameARealDifference(t *testing.T) {
	_, hosted := spec.Catalogue()
	for name, reason := range hostedOnly {
		if _, ok := hosted[name]; !ok {
			t.Errorf("hostedOnly names %s, which the hosted catalogue no "+
				"longer carries", name)
		}
		if serves(name) {
			t.Errorf("hostedOnly names %s, which this server carries too",
				name)
		}
		if reason == "" {
			t.Errorf("hostedOnly gives no reason for %s", name)
		}
	}
	for name, reason := range localOnly {
		if !serves(name) {
			t.Errorf("localOnly names %s, which this server does not carry",
				name)
		}
		if _, ok := hosted[name]; ok {
			t.Errorf("localOnly names %s, which the hosted catalogue "+
				"carries too", name)
		}
		if reason == "" {
			t.Errorf("localOnly gives no reason for %s", name)
		}
	}
}

// serves reports whether a server with every group carries the
// tool.
func serves(name string) bool {
	for _, got := range New(Options{}).ToolNames() {
		if got == name {
			return true
		}
	}
	return false
}
