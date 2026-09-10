package cli

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/transpareo/transpareo-cli/internal/registry"
)

// Markers around the generated part of the skill.
const (
	ReferenceStart = "<!-- reference:start -->"
	ReferenceEnd   = "<!-- reference:end -->"
)

// FillReference replaces the reference section of the skill text
// with one line per command: the command, its arguments, its
// summary and the permission key.
func FillReference(skill string, root *cobra.Command) (string, error) {
	start := strings.Index(skill, ReferenceStart)
	end := strings.Index(skill, ReferenceEnd)
	if start < 0 || end < 0 || end < start {
		return "", fmt.Errorf("the skill has no reference markers")
	}
	reg := registry.Default()
	var lines []string
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.Runnable() && !cmd.Hidden && cmd.Name() != "help" &&
			cmd.Name() != "completion" && !operatorOnly(cmd) {
			lines = append(lines, referenceLine(cmd, reg))
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
	sort.Strings(lines)
	body := "\n" + strings.Join(lines, "\n") + "\n"
	return skill[:start+len(ReferenceStart)] + body + skill[end:], nil
}

func referenceLine(cmd *cobra.Command, reg *registry.Registry) string {
	path := cmd.CommandPath()
	if args := argsOf(cmd); args != "" {
		path += " " + args
	}
	line := "- `" + path + "`: " + strings.TrimSuffix(cmd.Short, ".")
	id := cmd.Annotations[annotationOperation]
	if id == "" {
		id = operationOf[cmd.CommandPath()]
	}
	if op := reg.Find(id); op != nil && len(op.Permission) > 0 {
		line += " (" + strings.Join(op.Permission, " or ") + ")"
	}
	return line
}

var commandSpan = regexp.MustCompile("`(transpareo [^`]*)`")

// CommandsNamedIn lists the commands the skill text names in code
// spans, as their command words without arguments or options.
// annotationOperator marks a command tree that is for the
// person operating a workspace, not for an assistant: the skill
// leaves it out, so no assistant is told to run it.
const annotationOperator = "operator"

// operatorOnly reports whether cmd or one of its parents carries
// the operator annotation.
func operatorOnly(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Annotations[annotationOperator] != "" {
			return true
		}
	}
	return false
}

func CommandsNamedIn(skill string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range commandSpan.FindAllStringSubmatch(skill, -1) {
		var words []string
		for _, word := range strings.Fields(m[1]) {
			if strings.ContainsAny(word[:1], "-<[$") {
				break
			}
			words = append(words, word)
		}
		name := strings.Join(words, " ")
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}
