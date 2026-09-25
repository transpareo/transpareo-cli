package cli

import (
	"strings"
	"testing"

	"github.com/transpareo/transpareo-cli/internal/registry"
)

func TestCommandWords(t *testing.T) {
	reg := registry.Default()
	cases := map[string]string{
		"list_dpps":                          "dpps list",
		"get_dpp":                            "dpps get",
		"create_dpp":                         "dpps create",
		"bulk_create_dpps":                   "dpps bulk create",
		"validate_dpps_bulk":                 "dpps bulk validate",
		"get_bulk_task":                      "dpps bulk-task",
		"get_dpp_requirements":               "dpps requirements",
		"get_dpp_stats":                      "dpps stats",
		"list_dpp_versions":                  "dpps versions list",
		"list_dpp_events":                    "dpps events list",
		"append_dpp_event":                   "dpps events append",
		"update_dpp_dynamic_data":            "dpps dynamic-data update",
		"void_dpp":                           "dpps void",
		"publish_dpp":                        "dpps publish",
		"get_dpp_private_properties":         "dpps private-properties",
		"get_dpp_version_private_properties": "dpps version-private-properties",
		"get_new_product":                    "products new",
		"list_featured_products":             "products featured list",
		"list_product_properties":            "products properties list",
		"update_product_mediafiles":          "products mediafiles update",
		"list_product_categories":            "categories list",
		"list_help":                          "articles list",
		"get_help_article":                   "articles get",
		"regenerate_webhook_secret":          "webhooks secret regenerate",
		"test_webhook":                       "webhooks test",
		"create_export":                      "exports create",
		"download_export":                    "exports download",
		"list_events":                        "events list",
		"map_import":                         "imports map",
		"get_import_supplier_form":           "imports supplier-form",
		"create_grant":                       "grants create",
		"lookup_coupon":                      "coupons lookup",
		"resolve_permalink":                  "permalinks resolve",
		"search_catalogue":                   "search catalogue",
		"get_config":                         "configuration config",
		"list_languages":                     "configuration languages list",
		"list_countries":                     "reference-data countries list",
		"list_component_names": "reference-data " +
			"component-names list",
		"create_mediafile": "mediafiles create",
	}
	for id, want := range cases {
		op := reg.Find(id)
		if op == nil {
			t.Errorf("%s: not in the registry", id)
			continue
		}
		if got := strings.Join(CommandWords(op), " "); got != want {
			t.Errorf("%s: %q, want %q", id, got, want)
		}
	}
}

func TestCommandWordsAreUnique(t *testing.T) {
	reg := registry.Default()
	seen := map[string]string{}
	for _, op := range exposedOperations(reg) {
		words := strings.Join(CommandWords(op), " ")
		if other, dup := seen[words]; dup {
			t.Errorf("%q is used by %s and %s", words, other, op.ID)
		}
		seen[words] = op.ID
		for _, word := range CommandWords(op) {
			if word == "" || strings.ContainsAny(word, "_ ") {
				t.Errorf("%s: bad word %q", op.ID, word)
			}
		}
	}
}

func TestExposed(t *testing.T) {
	reg := registry.Default()
	cases := map[string]bool{
		"list_dpps":       true,
		"list_brands":     true,
		"get_config":      true,
		"create_grant":    true,
		"get_me":          false,
		"exchange_token":  false,
		"login_session":   false,
		"register_user":   false,
		"create_feedback": false,
		"list_favorites":  false,
		"assign_products": false,
		"choose_plan":     false,
	}
	for id, want := range cases {
		if got := Exposed(reg.Find(id)); got != want {
			t.Errorf("%s: exposed %v, want %v", id, got, want)
		}
	}
}

func TestFlagName(t *testing.T) {
	for in, want := range map[string]string{
		"per_page": "per-page", "productId": "product-id",
		"dppCode": "dpp-code",
		"Prefer":  "prefer", "term": "term", "sheetLocale": "sheet-locale",
	} {
		if got := flagName(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

// The work tasks take the product's word, which the hand-written
// `tasks wait` already carries, so both live under one command.
func TestGeneratedGroupJoinsTheHandWrittenCommand(t *testing.T) {
	root := (&App{}).Root()
	var tasks []string
	found := 0
	for _, cmd := range root.Commands() {
		if cmd.Name() == "tasks" {
			found++
			for _, child := range cmd.Commands() {
				tasks = append(tasks, child.Name())
			}
		}
	}
	if found != 1 {
		t.Fatalf("root carries %d commands named tasks, want 1", found)
	}
	joined := " " + strings.Join(tasks, " ") + " "
	for _, want := range []string{"wait", "list", "claim", "complete"} {
		if !strings.Contains(joined, " "+want+" ") {
			t.Errorf("tasks carries %v, missing %q", tasks, want)
		}
	}
}
