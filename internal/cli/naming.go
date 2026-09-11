package cli

import (
	"sort"
	"strings"

	"github.com/transpareo/transpareo-cli/internal/registry"
)

// verbs are the leading words of operation ids, longest first
// so that bulk_create wins over create.
var verbs = []string{
	"bulk_create", "list", "get", "create", "update", "delete", "publish",
	"unpublish", "void", "supersede", "reissue", "validate", "append",
	"assign", "search", "login", "logout", "refresh", "exchange", "confirm",
	"cancel", "upgrade", "choose", "start", "reset", "prepare", "register",
	"lookup", "resolve", "download", "regenerate", "test", "execute",
	"revert", "map",
}

// handWritten lists operations a hand-written command already
// covers, so no generated command duplicates them.
var handWritten = map[string]bool{
	"exchange_token": true,
	"get_me":         true,
}

// commandOverrides names the command words of operations whose
// derived name would read badly.
var commandOverrides = map[string]string{
	"list_product_categories": "categories list",
	"append_dpp_event":        "dpps events append",
}

// Exposed reports whether the command line offers an operation:
// every operation a consumer token can call, plus the public
// reads, minus the ones a hand-written command covers.
func Exposed(op *registry.Operation) bool {
	if handWritten[op.ID] || op.UserOnly {
		return false
	}
	// Nothing but a redirect or an error comes back, so there is
	// no answer a command could print. The browser half of OAuth
	// is what lands here: a person decides on a consent screen.
	if op.ResponseStatus == "" {
		return false
	}
	for _, scheme := range op.Security {
		if scheme == "oauth2" {
			return true
		}
	}
	return op.Public && op.Method == "GET"
}

// CommandWords derives the words after "transpareo" for an
// operation: the group from the tag, then the qualifier the id
// carries beyond the group's noun, then the verb.
//
//	list_dpps            dpps list
//	bulk_create_dpps     dpps bulk create
//	get_dpp_requirements dpps requirements
//	get_new_product      products new
func CommandWords(op *registry.Operation) []string {
	if words, ok := commandOverrides[op.ID]; ok {
		return strings.Fields(words)
	}
	verb, rest := splitVerb(op.ID)
	qualifier := qualifierWords(op.Group, rest)
	words := []string{op.Group}
	if len(qualifier) > 0 {
		words = append(words, strings.Join(qualifier, "-"))
	}
	sameAsGroup := verb == op.Group || verb == singular(op.Group)
	if (verb == "get" || sameAsGroup) && len(qualifier) > 0 {
		return words
	}
	// An id that opens with none of the known verbs carries its
	// whole name in the qualifier; appending the empty verb would
	// leave a word that is a space.
	if verb == "" {
		return words
	}
	return append(words, strings.Split(verb, "_")...)
}

func splitVerb(id string) (string, string) {
	for _, verb := range verbs {
		if strings.HasPrefix(id, verb+"_") {
			return verb, strings.TrimPrefix(id, verb+"_")
		}
	}
	return "", id
}

// qualifierWords drops the group's noun, in singular and plural,
// from the words after the verb.
func qualifierWords(group, rest string) []string {
	nouns := map[string]bool{}
	for _, form := range strings.Split(group, "-") {
		nouns[form] = true
		nouns[singular(form)] = true
	}
	var out []string
	for _, word := range strings.Split(rest, "_") {
		if word == "" || nouns[word] {
			continue
		}
		out = append(out, strings.ReplaceAll(word, "_", "-"))
	}
	return out
}

func singular(word string) string {
	switch {
	case strings.HasSuffix(word, "ies"):
		return strings.TrimSuffix(word, "ies") + "y"
	case strings.HasSuffix(word, "s"):
		return strings.TrimSuffix(word, "s")
	default:
		return word
	}
}

// exposedOperations returns the operations the command line
// offers, in a stable order.
func exposedOperations(reg *registry.Registry) []*registry.Operation {
	var ops []*registry.Operation
	for i := range reg.Operations {
		op := &reg.Operations[i]
		if Exposed(op) {
			ops = append(ops, op)
		}
	}
	sort.SliceStable(ops, func(i, j int) bool {
		return strings.Join(CommandWords(ops[i]), " ") <
			strings.Join(CommandWords(ops[j]), " ")
	})
	return ops
}

// flagName turns a parameter name into an option name: per_page
// and productId both become kebab-case.
func flagName(param string) string {
	var b strings.Builder
	for i, r := range param {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r + 'a' - 'A')
			continue
		}
		if r == '_' {
			b.WriteByte('-')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// usage strips the backticks of a description, which cobra would
// otherwise read as the placeholder of the option's value.
func usage(description string) string {
	return strings.ReplaceAll(description, "`", "'")
}
