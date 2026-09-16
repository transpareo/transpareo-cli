package mcp

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/transpareo/transpareo-cli/internal/registry"
	"github.com/transpareo/transpareo-cli/spec"
)

// TestGeneratedToolsMatchTheCatalogue fails when tools.gen.go
// lags the vendored catalogue, naming what to run. The table and
// the instructions are derived, so this is the check that they
// were derived from the document that is committed beside them.
func TestGeneratedToolsMatchTheCatalogue(t *testing.T) {
	doc := spec.Catalogue()
	if instructions != doc.Instructions {
		t.Errorf("the instructions differ from the vendored catalogue; "+
			"run go generate ./internal/mcp/\n here  %q\n there %q",
			instructions, doc.Instructions)
	}
	names := doc.CuratedNames()
	if len(names) != len(curated) {
		t.Fatalf("the catalogue curates %d tools and the generated table "+
			"carries %d; run go generate ./internal/mcp/", len(names),
			len(curated))
	}
	for i, name := range names {
		want, got := doc.Tools[name], curated[i]
		if got.Name != want.Name || got.Group != want.Group ||
			got.Operation != want.Operation || got.Kind.String() != want.Kind ||
			got.Confirm != want.Confirm || got.Description != want.Sentence {
			t.Errorf("%s was generated as %+v, and the catalogue declares "+
				"{Group:%s Operation:%s Kind:%s Confirm:%s Sentence:%s}; run "+
				"go generate ./internal/mcp/", name, got, want.Group,
				want.Operation, want.Kind, want.Confirm, want.Sentence)
		}
	}
}

// TestCatalogueMatchesTheHostedOne compares this server's tools
// with the catalogue the hosted assistant server publishes.
//
// A curated tool is no longer compared on what it is: both sides
// read that from the same row of the same document, and this side
// generates its table from it. What is compared is what each side
// built out of that row, because the building is done twice, once
// in Ruby and once here. The description an assistant reads and
// both schemas it fills in have to come out the same.
//
// A composed tool is still written twice by hand, so its absence
// on one side is allowed when hostedOnly or localOnly gives the
// reason, and one both sides carry must agree on its group and
// its flags.
func TestCatalogueMatchesTheHostedOne(t *testing.T) {
	doc := spec.Catalogue()
	if doc.Version != spec.Version() {
		t.Fatalf("the vendored catalogue is %s and the specification %s; "+
			"re-vendor both, they move together", doc.Version, spec.Version())
	}

	reg := registry.Default()
	for _, tool := range curated {
		op := reg.Find(tool.Operation)
		if op == nil {
			t.Errorf("%s is curated over %s, which the registry does not "+
				"carry; the two documents disagree about the API", tool.Name,
				tool.Operation)
			continue
		}
		hosted := doc.Tools[tool.Name]
		for _, difference := range renderedDifferences(tool, op, hosted) {
			t.Errorf("curated tool %s: %s", tool.Name, difference)
		}
	}

	for _, tool := range New(Options{}).composedTools() {
		if tool.Tool.Annotations == nil {
			t.Errorf("composed tool %s carries no annotations, so its "+
				"flags cannot be compared", tool.Name)
			continue
		}
		got, ok := doc.Tools[tool.Name]
		if !ok {
			if localOnly[tool.Name] == "" {
				t.Errorf("%s is a tool here and not in the hosted "+
					"catalogue, with no reason in localOnly", tool.Name)
			}
			continue
		}
		if got.Operation != "" {
			t.Errorf("composed tool %s answers operation %s in the hosted "+
				"catalogue, so it is curated there and written by hand here",
				tool.Name, got.Operation)
		}
		if got.Confirm != "" {
			t.Errorf("composed tool %s states the confirm phrase %q in the "+
				"hosted catalogue; this server composes one per operation",
				tool.Name, got.Confirm)
		}
		if got.Group != tool.Group {
			t.Errorf("composed tool %s is in group %s here and %s there",
				tool.Name, tool.Group, got.Group)
		}
		destructive := hint(tool.Tool.Annotations.DestructiveHint)
		if got.Destructive != destructive {
			t.Errorf("composed tool %s is destructive %t here and %t there",
				tool.Name, destructive, got.Destructive)
		}
		if got.Safe != tool.Tool.Annotations.ReadOnlyHint {
			t.Errorf("composed tool %s is safe %t here and %t there",
				tool.Name, tool.Tool.Annotations.ReadOnlyHint, got.Safe)
		}
	}

	for name, tool := range doc.Tools {
		if tool.Operation != "" || serves(name) {
			continue
		}
		if hostedOnly[name] == "" {
			t.Errorf("%s is a hosted tool this server does not carry, "+
				"with no reason in hostedOnly", name)
		}
	}
}

// renderedDifferences names every way the two renderings of one
// curated tool disagree, in words: a schema runs to tens of
// kilobytes and reading two of them side by side is not how the
// difference is found.
func renderedDifferences(tool Tool, op *registry.Operation,
	hosted spec.CatalogueTool) []string {
	var out []string
	if hosted.Destructive != op.Destructive {
		out = append(out, fmt.Sprintf("destructive is %t here and %t there",
			op.Destructive, hosted.Destructive))
	}
	if hosted.Safe != op.ReadOnly() {
		out = append(out, fmt.Sprintf("safe is %t here and %t there",
			op.ReadOnly(), hosted.Safe))
	}
	out = append(out, describedDifferently(tool, op, hosted)...)
	for _, schema := range []struct {
		what   string
		local  any
		hosted json.RawMessage
	}{
		{"input schema", tool.inputSchema(op), hosted.InputSchema},
		{"output schema", tool.outputSchema(op), hosted.OutputSchema},
	} {
		local, err := json.Marshal(schema.local)
		if err != nil {
			out = append(out, "the "+schema.what+" here will not marshal: "+
				err.Error())
			continue
		}
		for _, difference := range documentDifferences(schema.what, local,
			schema.hosted) {
			out = append(out, "the "+difference)
		}
	}
	return out
}

// describedDifferently compares the two descriptions. The example
// is compared as the call it is: the hosted server writes a body's
// fields in the order the document declares them and this one
// writes them in name order, because Go marshals a map that way.
// The two differ in spelling while telling an assistant to send
// the same call.
func describedDifferently(tool Tool, op *registry.Operation,
	hosted spec.CatalogueTool) []string {
	localProse, localExample := splitExample(tool.describe(op))
	hostedProse, hostedExample := splitExample(hosted.Description)
	var out []string
	if localProse != hostedProse {
		out = append(out, fmt.Sprintf("the description is %q here and %q "+
			"there", localProse, hostedProse))
	}
	if !sameCall(localExample, hostedExample) {
		out = append(out, fmt.Sprintf("the example is %q here and %q there",
			localExample, hostedExample))
	}
	return out
}

// splitExample cuts a description into the prose above the
// example line and the example itself.
func splitExample(description string) (string, string) {
	const marker = "\nExample: "
	if i := strings.Index(description, marker); i >= 0 {
		return description[:i], description[i+len(marker):]
	}
	return description, ""
}

// sameCall reports whether two example lines name the same tool
// and carry the same arguments, whatever order they are written
// in.
func sameCall(local, hosted string) bool {
	if local == hosted {
		return true
	}
	localName, localArgs, _ := strings.Cut(local, " ")
	hostedName, hostedArgs, _ := strings.Cut(hosted, " ")
	if localName != hostedName {
		return false
	}
	var localValue, hostedValue any
	if json.Unmarshal([]byte(localArgs), &localValue) != nil ||
		json.Unmarshal([]byte(hostedArgs), &hostedValue) != nil {
		return false
	}
	return reflect.DeepEqual(localValue, hostedValue)
}

// documentDifferences names the paths at which two JSON documents
// disagree, at most eight of them, so a failure says which
// property moved.
func documentDifferences(what string, local, hosted json.RawMessage) []string {
	var localValue, hostedValue any
	if err := json.Unmarshal(local, &localValue); err != nil {
		return []string{what + " here is not JSON: " + err.Error()}
	}
	if err := json.Unmarshal(hosted, &hostedValue); err != nil {
		return []string{what + " there is not JSON: " + err.Error()}
	}
	found := walk(what, localValue, hostedValue)
	if len(found) > 8 {
		found = append(found[:8], fmt.Sprintf("%s: and %d more", what,
			len(found)-8))
	}
	return found
}

func walk(path string, local, hosted any) []string {
	localObject, isObject := local.(map[string]any)
	hostedObject, alsoObject := hosted.(map[string]any)
	if !isObject || !alsoObject {
		if reflect.DeepEqual(local, hosted) {
			return nil
		}
		return []string{fmt.Sprintf("%s is %s here and %s there", path,
			brief(local), brief(hosted))}
	}
	names := map[string]bool{}
	for name := range localObject {
		names[name] = true
	}
	for name := range hostedObject {
		names[name] = true
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	var out []string
	for _, name := range ordered {
		localChild, here := localObject[name]
		hostedChild, there := hostedObject[name]
		switch {
		case !there:
			out = append(out, fmt.Sprintf("%s.%s is only here", path, name))
		case !here:
			out = append(out, fmt.Sprintf("%s.%s is only there", path, name))
		default:
			out = append(out, walk(path+"."+name, localChild, hostedChild)...)
		}
	}
	return out
}

// brief renders a value short enough to read in a failure line.
func brief(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "unreadable"
	}
	if len(data) > 120 {
		return string(data[:120]) + "..."
	}
	return string(data)
}

// TestAllowancesNameARealDifference keeps the two lists honest:
// an entry that no longer describes a difference is noise, and a
// reason is the whole point of the entry.
func TestAllowancesNameARealDifference(t *testing.T) {
	doc := spec.Catalogue()
	for name, reason := range hostedOnly {
		if _, ok := doc.Tools[name]; !ok {
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
		if _, ok := doc.Tools[name]; ok {
			t.Errorf("localOnly names %s, which the hosted catalogue "+
				"carries too", name)
		}
		if reason == "" {
			t.Errorf("localOnly gives no reason for %s", name)
		}
	}
}

// hint reads an annotation the SDK models as an optional flag.
func hint(value *bool) bool {
	return value != nil && *value
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
