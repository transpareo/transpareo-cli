// Package mcp is the Model Context Protocol server: a curated set
// of tools over the API, two discovery tools that reach every
// other operation, and three resources.
package mcp

//go:generate go run ./gen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/transpareo/transpareo-cli/internal/registry"
)

// Group names a set of tools an assistant can be limited to with
// --tools. Identity and discovery are always present.
const (
	GroupIdentity  = "identity"
	GroupDiscovery = "discovery"
	GroupProducts  = "products"
	GroupDpps      = "dpps"
	GroupData      = "data"
	GroupWebhooks  = "webhooks"
)

// Groups lists the groups --tools accepts.
var Groups = []string{GroupProducts, GroupDpps, GroupData, GroupWebhooks}

type kind int

const (
	kindList kind = iota
	kindGet
	kindWrite
	kindRows
)

// String is the name the catalogue declares the kind under.
func (k kind) String() string {
	switch k {
	case kindList:
		return "list"
	case kindGet:
		return "get"
	case kindWrite:
		return "write"
	case kindRows:
		return "rows"
	}
	return "unknown"
}

// Tool describes one curated tool: which operation it calls, how
// its arguments map onto the request, and the phrase a
// destructive one demands.
type Tool struct {
	Name      string
	Group     string
	Operation string
	Kind      kind

	// Confirm is the phrase template of a destructive tool, with
	// the path argument in angle brackets: "void <id>".
	Confirm string

	// Description adds to what the registry says.
	Description string
}

// viaCallAPIOnly lists the consumer-callable operations that have
// no curated tool, each with the reason, so the coverage test can
// tell a decision from an omission.
var viaCallAPIOnly = map[string]string{
	"exchange_token": "the server logs in itself",
	"authorize_client": "a person decides on a consent screen in their " +
		"browser; the answer is a redirect, not data",
	"create_grant": "issues credentials; a person hands them out with the " +
		"command line",
	"get_bulk_task":     "wait_for_task polls the statusUrl",
	"get_dpp_stats":     "reporting, rarely part of a flow",
	"list_dpp_versions": "history, reachable when a flow needs it",
	"list_dpp_events":   "history, reachable when a flow needs it",
	"delete_dpp": "drafts only; voiding is the documented " +
		"way out",
	"correct_dpp": "a passport follows a product an operator " +
		"corrected",
	"get_dpp_private_properties": "restricted tier, read on purpose only",
	"get_dpp_version_private_properties": "restricted tier, read on purpose " +
		"only",
	"list_events":              "tail_events reads the feed",
	"create_export":            "export_catalogue runs the flow",
	"get_export":               "export_catalogue runs the flow",
	"download_export":          "binary download, for the command line",
	"create_import":            "import_spreadsheet runs the flow",
	"get_import":               "import_spreadsheet runs the flow",
	"get_import_example":       "binary download, for the command line",
	"get_import_supplier_form": "binary download, for the command line",
	"map_import":               "import_spreadsheet runs the flow",
	"validate_import":          "import_spreadsheet runs the flow",
	"execute_import":           "import_spreadsheet runs the flow",
	"revert_import":            "import_spreadsheet runs the flow",
	"get_webhook":              "list_webhooks shows every field",
	"update_webhook":           "rare; delete and create is clearer",
	"delete_product": "unpublishing is the safe way; deletion stays a " +
		"deliberate call",
	"delete_component": "unpublishing is the safe way; deletion stays a " +
		"deliberate call",
	"delete_brand": "a brand goes with its products",
	"get_brand":    "list_brands shows every field",
	"update_brand": "a brand is its name; rename in the " +
		"application manager",
	"publish_component":   "components publish with their products",
	"unpublish_component": "components publish with their products",
	"delete_mediafile": "taking a file away stays a deliberate call; " +
		"update_product_mediafiles takes it off a product",
	"list_product_properties":   "reference data, on demand",
	"list_featured_products":    "storefront view, on demand",
	"list_product_categories":   "reference data, on demand",
	"get_config":                "reference data, on demand",
	"list_languages":            "reference data, on demand",
	"get_navigation":            "storefront view, on demand",
	"get_localization":          "storefront view, on demand",
	"lookup_coupon":             "storefront flow",
	"resolve_permalink":         "storefront flow",
	"list_plans":                "storefront flow",
	"get_plan":                  "storefront flow",
	"list_product_gtins":        "reference data, on demand",
	"list_countries":            "reference data, on demand",
	"list_component_names":      "reference data, on demand",
	"list_component_functions":  "reference data, on demand",
	"list_component_types":      "reference data, on demand",
	"list_component_properties": "reference data, on demand",
	"search_catalogue": "list_products and list_components take a " +
		"term",
}

// hostedOnly and localOnly are the tools one catalogue carries
// and the other does not, each with the reason. Only a composed
// tool can appear: a curated one is generated from the row the
// catalogue publishes, so the two sides carry the same set by
// construction. A test compares the two catalogues and refuses
// any difference not listed here.
var hostedOnly = map[string]string{
	"search": "deep research prescribes both this name and its shape; " +
		"a client that finds anything else falls back to no research",
	"fetch": "deep research prescribes both this name and its shape; " +
		"a client that finds anything else falls back to no research",
}

var localOnly = map[string]string{
	"import_spreadsheet": "the flow starts from a path to a file on " +
		"disk, which a hosted assistant has no way to reach",
}

// Tools returns the curated tools, filtered to the groups asked
// for and, in read-only mode, to the ones that change nothing.
func Tools(reg *registry.Registry, groups []string, readOnly bool) []Tool {
	wanted := map[string]bool{GroupIdentity: true, GroupDiscovery: true}
	for _, g := range groups {
		wanted[g] = true
	}
	var out []Tool
	for _, t := range curated {
		if len(groups) > 0 && !wanted[t.Group] {
			continue
		}
		op := reg.Find(t.Operation)
		if op == nil {
			continue
		}
		if readOnly && !op.ReadOnly() {
			continue
		}
		out = append(out, t)
	}
	return out
}

// Find returns the curated tool with the name, or nil.
func Find(name string) *Tool {
	for i := range curated {
		if curated[i].Name == name {
			return &curated[i]
		}
	}
	return nil
}

// tier says which data an operation's answer can carry, from its
// permission keys: the private properties are the restricted
// tier, everything a consumer token reads is the workspace's own
// catalogue, and a public endpoint answers public data.
func tier(op *registry.Operation) string {
	for _, key := range op.Permission {
		if key == "dpp_vault_read" {
			return "restricted (private properties)"
		}
	}
	if strings.Contains(op.Path, "private_properties") {
		return "restricted (private properties)"
	}
	if op.Public && len(op.Permission) == 0 {
		return "public"
	}
	return "authorised (the workspace's own catalogue)"
}

// describe composes the tool description: the summary, the extra
// text, the permission, the irreversibility note, the data tier
// and one example call.
func (t Tool) describe(op *registry.Operation) string {
	var b strings.Builder
	b.WriteString(op.Summary)
	if t.Description != "" {
		b.WriteString(". " + t.Description)
	}
	b.WriteString("\n")
	switch {
	case len(op.Permission) == 1:
		fmt.Fprintf(&b, "Permission: %s.", op.Permission[0])
	case len(op.Permission) > 1:
		fmt.Fprintf(&b, "Permission: one of %s.", strings.Join(op.Permission,
			", "))
	case op.Public:
		b.WriteString("Permission: none, the endpoint is public.")
	default:
		b.WriteString("Permission: none beyond a valid token.")
	}
	if op.Destructive {
		fmt.Fprintf(&b, " Cannot be undone; confirm must be %q.",
			t.confirmPhrase(op))
	}
	fmt.Fprintf(&b, " Data tier: %s.", tier(op))
	fmt.Fprintf(&b, "\nExample: %s", t.example(op))
	return b.String()
}

// confirmPhrase renders the confirm template with the name of
// the path argument.
func (t Tool) confirmPhrase(op *registry.Operation) string {
	phrase := t.Confirm
	for _, p := range op.PathParams {
		phrase = strings.ReplaceAll(phrase, "<id>", "<"+p.Name+">")
	}
	return phrase
}

// example builds one example call from the request example and
// placeholder path arguments. Each value is kept as the document
// spells it, so the fields of a body stay in their declared
// order, which is the order that reads as an explanation.
func (t Tool) example(op *registry.Operation) string {
	args := map[string]json.RawMessage{}
	for _, p := range op.PathParams {
		args[p.Name] = literal("<" + p.Name + ">")
	}
	switch t.Kind {
	case kindList:
		args["per_page"] = literal(20)
	case kindRows:
		args["rows"] = json.RawMessage(`[{"modelIdentifier":"FC-50ML"}]`)
	case kindWrite:
		var body map[string]json.RawMessage
		json.Unmarshal(bodyExample(op), &body)
		for key, value := range body {
			args[key] = value
		}
		if t.Confirm != "" {
			args["confirm"] = literal(t.confirmPhrase(op))
		}
	case kindGet:
		for _, p := range op.QueryParams {
			if p.Name == "productId" {
				args["productId"] = literal("<productId>")
			}
		}
	}
	return t.Name + " " + object(args)
}

// literal renders one value with the angle brackets of a
// placeholder left alone: encoding/json escapes <, > and & by
// default, which would turn <id> into an escape sequence in the
// one line a reader copies from.
func literal(value any) json.RawMessage {
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return json.RawMessage("null")
	}
	return json.RawMessage(bytes.TrimRight(b.Bytes(), "\n"))
}

// object writes the members in name order, each value as it
// stands.
func object(members map[string]json.RawMessage) string {
	names := make([]string, 0, len(members))
	for name := range members {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteByte('{')
	for i, name := range names {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(literal(name))
		b.WriteByte(':')
		b.Write(members[name])
	}
	b.WriteByte('}')
	return b.String()
}

// inputSchema builds the JSON Schema of the arguments: the path
// parameters, the query parameters of a list or a get, the body
// properties of a write, an idempotency key where the operation
// replays one, rows for a bulk call, fields to narrow the answer,
// and confirm for a destructive call.
func (t Tool) inputSchema(op *registry.Operation) map[string]any {
	props := map[string]any{}
	var required []string
	for _, p := range op.PathParams {
		props[p.Name] = map[string]any{"type": "string",
			"description": orDefault(p.Description,
				"The "+p.Name+" in the path")}
		required = append(required, p.Name)
	}
	switch t.Kind {
	case kindList, kindGet:
		for _, p := range op.QueryParams {
			props[p.Name] = paramSchema(p)
		}
		fieldsOf := "of the record"
		if t.Kind == kindList {
			fieldsOf = "of each item"
		}
		props["fields"] = map[string]any{"type": "array",
			"items": map[string]any{"type": "string"},
			"description": "Keep only these fields " + fieldsOf +
				", such as id, name or status"}
	case kindWrite:
		var body struct {
			Properties map[string]any `json:"properties"`
			Required   []string       `json:"required"`
		}
		if declared := op.JSONBody(); declared != nil {
			json.Unmarshal(declared.Schema, &body)
		}
		for name, schema := range body.Properties {
			props[name] = schema
		}
		required = append(required, body.Required...)
		if op.Idempotent {
			props["idempotencyKey"] = map[string]any{"type": "string",
				"description": "Makes the call safe to repeat: the same key " +
					"within a day answers the result of the first call"}
		}
	case kindRows:
		props["rows"] = map[string]any{"type": "array",
			"items":       map[string]any{"type": "object"},
			"description": "One passport per row, as DppBulkRowInput"}
		props["shared"] = map[string]any{"type": "object",
			"description": "Batch-wide defaults every row deep-merges over"}
		required = append(required, "rows")
		if op.ID == "bulk_create_dpps" {
			props["async"] = map[string]any{"type": "boolean",
				"description": "Process in the background and answer a " +
					"statusUrl " +
					"to poll; needed beyond 500 rows"}
		}
	}
	if t.Confirm != "" {
		props["confirm"] = map[string]any{"type": "string",
			"description": fmt.Sprintf("Must be %q", t.confirmPhrase(op))}
		required = append(required, "confirm")
	}
	sort.Strings(required)
	schema := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// bodyExample is the example an assistant is shown: the one of
// the body it would send, which is the JSON body whenever the
// operation declares one.
func bodyExample(op *registry.Operation) json.RawMessage {
	body := op.DefaultBody()
	if body == nil {
		return nil
	}
	return body.Example
}

func paramSchema(p registry.Param) map[string]any {
	schema := map[string]any{}
	json.Unmarshal(p.Schema, &schema)
	if p.Description != "" {
		schema["description"] = p.Description
	}
	if _, ok := schema["type"]; !ok {
		schema["type"] = "string"
	}
	return schema
}

// outputSchema declares the structured content: a page for a
// list, the operation's response schema otherwise.
func (t Tool) outputSchema(op *registry.Operation) any {
	if t.Kind == kindList {
		items := map[string]any{"type": "object"}
		var response struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		json.Unmarshal(op.ResponseSchema, &response)
		for _, schema := range response.Properties {
			var arr struct {
				Type  string          `json:"type"`
				Items json.RawMessage `json:"items"`
			}
			if json.Unmarshal(schema, &arr) == nil && arr.Type == "array" &&
				arr.Items != nil {
				items = map[string]any{}
				json.Unmarshal(arr.Items, &items)
				break
			}
		}
		return map[string]any{"type": "object", "properties": map[string]any{
			"items":    map[string]any{"type": "array", "items": items},
			"page":     map[string]any{"type": "integer"},
			"total":    map[string]any{"type": "integer"},
			"nextPage": map[string]any{"type": "integer"},
		}}
	}
	if len(op.ResponseSchema) == 0 {
		return map[string]any{"type": "object"}
	}
	var schema any
	json.Unmarshal(op.ResponseSchema, &schema)
	return schema
}

func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// descriptionWhenNoJSONBody carries the operation's prose for a
// body call_api cannot send as it stands. The prose is then the
// only place saying what to send instead.
func descriptionWhenNoJSONBody(op *registry.Operation) string {
	if len(op.RequestBodies) == 0 || op.JSONBody() != nil {
		return ""
	}
	return op.Description
}
