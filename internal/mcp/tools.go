// Package mcp is the Model Context Protocol server: a curated set
// of tools over the API, two discovery tools that reach every
// other operation, and three resources.
package mcp

import (
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

// Tool describes one curated tool: which operation it calls, how
// its arguments map onto the request, and the phrase a
// destructive one demands.
type Tool struct {
	Name      string
	Group     string
	Operation string
	Kind      kind

	// Confirm is the phrase template of a destructive tool, with
	// the path argument in angle brackets: "void <code>".
	Confirm string

	// Description adds to what the registry says.
	Description string
}

// curated is the tool table. Every operation of the registry is
// either here, in viaCallAPIOnly, hidden from consumers, or a
// user-only operation; a test enforces it.
var curated = []Tool{
	{Name: "me", Group: GroupIdentity, Operation: "get_me", Kind: kindGet,
		Description: "Start here: what the credential allows, its scope and " +
			"rate limit."},

	{Name: "list_products", Group: GroupProducts, Operation: "list_products",
		Kind: kindList},
	{Name: "get_product", Group: GroupProducts, Operation: "get_product",
		Kind: kindGet},
	{Name: "create_product", Group: GroupProducts, Operation: "create_product",
		Kind: kindWrite,
		Description: "Call product_property_types first: a product missing a " +
			"mandatory property is refused."},
	{Name: "update_product", Group: GroupProducts, Operation: "update_product",
		Kind: kindWrite},
	{Name: "publish_product", Group: GroupProducts,
		Operation: "publish_product", Kind: kindWrite},
	{Name: "unpublish_product", Group: GroupProducts,
		Operation: "unpublish_product", Kind: kindWrite},
	{Name: "product_property_types", Group: GroupProducts,
		Operation: "get_new_product", Kind: kindGet,
		Description: "The property types a new product can carry, with the " +
			"mandatory ones flagged: the body create_product needs."},
	{Name: "list_components", Group: GroupProducts,
		Operation: "list_components", Kind: kindList},
	{Name: "get_component", Group: GroupProducts, Operation: "get_component",
		Kind: kindGet},
	{Name: "create_component", Group: GroupProducts,
		Operation: "create_component", Kind: kindWrite},
	{Name: "update_component", Group: GroupProducts,
		Operation: "update_component", Kind: kindWrite},
	{Name: "list_brands", Group: GroupProducts, Operation: "list_brands",
		Kind: kindList},
	{Name: "create_brand", Group: GroupProducts, Operation: "create_brand",
		Kind: kindWrite},

	{Name: "dpp_requirements", Group: GroupDpps,
		Operation: "get_dpp_requirements", Kind: kindGet,
		Description: "What a passport of a product still needs, with a body " +
			"ready to fill for validate_dpp and create_dpp."},
	{Name: "list_dpps", Group: GroupDpps, Operation: "list_dpps",
		Kind: kindList},
	{Name: "get_dpp", Group: GroupDpps, Operation: "get_dpp", Kind: kindGet},
	{Name: "validate_dpp", Group: GroupDpps, Operation: "validate_dpp",
		Kind: kindWrite,
		Description: "Runs the checks a publish runs, writes nothing. Call" +
			"it " +
			"before create_dpp."},
	{Name: "create_dpp", Group: GroupDpps, Operation: "create_dpp",
		Kind: kindWrite},
	{Name: "update_dpp", Group: GroupDpps, Operation: "update_dpp",
		Kind: kindWrite},
	{Name: "publish_dpp", Group: GroupDpps, Operation: "publish_dpp",
		Kind: kindWrite,
		Description: "Signs a snapshot into the ten-year archive. Publishing " +
			"cannot be undone."},
	{Name: "append_dpp_event", Group: GroupDpps, Operation: "append_dpp_event",
		Kind: kindWrite},
	{Name: "update_dynamic_data", Group: GroupDpps,
		Operation: "update_dpp_dynamic_data", Kind: kindWrite},
	{Name: "void_dpp", Group: GroupDpps, Operation: "void_dpp", Kind: kindWrite,
		Confirm: "void <id>"},
	{Name: "supersede_dpp", Group: GroupDpps, Operation: "supersede_dpp",
		Kind:    kindWrite,
		Confirm: "supersede <id>"},
	{Name: "reissue_dpp", Group: GroupDpps, Operation: "reissue_dpp",
		Kind: kindWrite},
	{Name: "bulk_validate_dpps", Group: GroupDpps,
		Operation: "validate_dpps_bulk", Kind: kindRows},
	{Name: "bulk_create_dpps", Group: GroupDpps, Operation: "bulk_create_dpps",
		Kind: kindRows,
		Description: "Rows are deduplicated by their identifiers, so " +
			"resubmitting after a timeout creates nothing twice."},

	{Name: "list_webhooks", Group: GroupWebhooks, Operation: "list_webhooks",
		Kind: kindList},
	{Name: "create_webhook", Group: GroupWebhooks, Operation: "create_webhook",
		Kind:        kindWrite,
		Description: "The answer carries the signing secret, shown only here."},
	{Name: "test_webhook", Group: GroupWebhooks, Operation: "test_webhook",
		Kind: kindWrite},
	{Name: "regenerate_webhook_secret", Group: GroupWebhooks,
		Operation: "regenerate_webhook_secret", Kind: kindWrite,
		Confirm: "regenerate <id>",
		Description: "The old secret stops working at once; the answer " +
			"carries the new one, shown only here."},
	{Name: "delete_webhook", Group: GroupWebhooks, Operation: "delete_webhook",
		Kind:    kindWrite,
		Confirm: "delete <id>"},
}

// viaCallAPIOnly lists the consumer-callable operations that have
// no curated tool, each with the reason, so the coverage test can
// tell a decision from an omission.
var viaCallAPIOnly = map[string]string{
	"exchange_token": "the server logs in itself",
	"create_grant": "issues credentials; a person" +
		"hands them out with the command line",
	"get_bulk_task": "polled by wait_for_task once the" +
		"data tools exist",
	"get_dpp_stats": "reporting, rarely part of a flow",
	"list_dpp_versions": "history, reachable when a flow" +
		"needs it",
	"list_dpp_events": "history, reachable when a flow" +
		"needs it",
	"delete_dpp": "drafts only; voiding is the" +
		"documented way out",
	"get_dpp_private_properties": "restricted tier, read on purpose" +
		"only",
	"get_dpp_version_private_properties": "restricted tier, read on purpose" +
		"only",
	"list_events": "covered by tail_events once the" +
		"data tools exist",
	"create_export": "covered by export_catalogue once" +
		"the data tools exist",
	"get_export": "covered by export_catalogue once" +
		"the data tools exist",
	"download_export": "binary download, for the command" +
		"line",
	"create_import": "covered by import_spreadsheet" +
		"once the data tools exist",
	"get_import": "covered by import_spreadsheet" +
		"once the data tools exist",
	"get_import_example": "binary download, for the command" +
		"line",
	"get_import_supplier_form": "binary download, for the command" +
		"line",
	"map_import": "covered by import_spreadsheet" +
		"once the data tools exist",
	"validate_import": "covered by import_spreadsheet" +
		"once the data tools exist",
	"execute_import": "covered by import_spreadsheet" +
		"once the data tools exist",
	"revert_import": "covered by import_spreadsheet" +
		"once the data tools exist",
	"get_webhook":    "list_webhooks shows every field",
	"update_webhook": "rare; delete and create is clearer",
	"delete_product": "unpublishing is the safe way;" +
		"deletion stays a deliberate call",
	"delete_component": "unpublishing is the safe way;" +
		"deletion stays a deliberate call",
	"delete_brand": "a brand goes with its products",
	"get_brand":    "list_brands shows every field",
	"update_brand": "a brand is its name; rename in" +
		"the application manager",
	"publish_component": "components publish with their" +
		"products",
	"unpublish_component": "components publish with their" +
		"products",
	"update_product_mediafiles": "media handling stays with the" +
		"command line",
	"list_mediafiles": "media handling stays with the" +
		"command line",
	"create_mediafile": "media handling stays with the" +
		"command line",
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
	"search_catalogue": "list_products and list_components" +
		"take a term",
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
func (t Tool) describe(op *registry.Operation, schema map[string]any) string {
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
	default:
		b.WriteString("Permission: none, the endpoint is public.")
	}
	if op.Destructive {
		fmt.Fprintf(&b, " Cannot be undone; confirm must be %q.",
			t.confirmPhrase(op))
	}
	fmt.Fprintf(&b, " Data tier: %s.", tier(op))
	fmt.Fprintf(&b, "\nExample: %s", t.example(op, schema))
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
// placeholder path arguments.
func (t Tool) example(op *registry.Operation, schema map[string]any) string {
	args := map[string]any{}
	for _, p := range op.PathParams {
		args[p.Name] = "<" + p.Name + ">"
	}
	switch t.Kind {
	case kindList:
		args["per_page"] = 20
	case kindRows:
		args["rows"] = []any{json.RawMessage(`{"modelIdentifier": "FC-50ML"}`)}
	case kindWrite:
		var body map[string]any
		json.Unmarshal(op.RequestExample, &body)
		for key, value := range body {
			args[key] = value
		}
		if t.Confirm != "" {
			args["confirm"] = t.confirmPhrase(op)
		}
	case kindGet:
		for _, p := range op.QueryParams {
			if p.Name == "productId" {
				args["productId"] = "<productId>"
			}
		}
	}
	data, _ := json.Marshal(args)
	return t.Name + " " + string(data)
}

// inputSchema builds the JSON Schema of the arguments: the path
// parameters, the query parameters of a list or a get, the body
// properties of a write, rows for a bulk call, fields to narrow
// the answer, and confirm for a destructive call.
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
		props["fields"] = map[string]any{"type": "array",
			"items":       map[string]any{"type": "string"},
			"description": "Keep only these fields of the answer"}
	case kindWrite:
		var body struct {
			Properties map[string]any `json:"properties"`
			Required   []string       `json:"required"`
		}
		json.Unmarshal(op.RequestBody, &body)
		for name, schema := range body.Properties {
			props[name] = schema
		}
		required = append(required, body.Required...)
	case kindRows:
		props["rows"] = map[string]any{"type": "array",
			"items":       map[string]any{"type": "object"},
			"description": "One passport per row, as DppBulkRowInput"}
		props["shared"] = map[string]any{"type": "object",
			"description": "Batch-wide defaults every row deep-merges over"}
		required = append(required, "rows")
		if op.ID == "bulk_create_dpps" {
			props["async"] = map[string]any{"type": "boolean",
				"description": "Process in the background and answer a" +
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
