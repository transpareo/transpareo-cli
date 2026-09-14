package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/transpareo/transpareo-cli/internal/registry"
	"github.com/transpareo/transpareo-cli/spec"
)

// globalOptions are the options every command accepts, for the
// table at the top of the reference.
var globalOptions = [][2]string{
	{"`--json`", "print JSON; the default when the output is not a terminal"},
	{"`--jsonl`", "print lists as one JSON object per line"},
	{"`-q`, `--quiet`", "print ids only, one per line"},
	{"`--fields a,b`", "keep only these fields of the answer"},
	{"`--profile <name>`", "the workspace to use, as stored by `auth login`"},
	{"`--read-only`", "refuse every operation that changes data"},
	{"`--yes`", "confirm an operation that cannot be undone"},
}

// Markdown renders the command reference: the global options, a
// line of links to the groups, then one section per group with
// the tag's description and one entry per command: summary,
// usage and example in one block, options as a table, and the
// operation with its permission on a closing line.
func (a *App) Markdown(root *cobra.Command) (string, error) {
	reg := a.operations()
	tags := tagDescriptions()
	var b strings.Builder
	fmt.Fprintf(&b, "# Command reference\n\n")
	fmt.Fprintf(&b, "Every command of `transpareo`, generated from API "+
		"specification %s. `--help` on any command prints the same "+
		"example and permission key.\n\n", spec.Version())
	fmt.Fprintf(&b, "| Option on every command | What it does |\n|---|---|\n")
	for _, opt := range globalOptions {
		fmt.Fprintf(&b, "| %s | %s |\n", opt[0], opt[1])
	}
	b.WriteString("\n")
	sections := map[string][]*cobra.Command{}
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.Runnable() && !cmd.Hidden && cmd.Name() != "help" {
			section := "General"
			if id := cmd.Annotations[annotationOperation]; id != "" {
				if op := reg.Find(id); op != nil {
					section = op.Tag
				}
			}
			sections[section] = append(sections[section], cmd)
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
	names := make([]string, 0, len(sections))
	for name := range sections {
		if name != "General" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	names = append([]string{"General"}, names...)
	links := make([]string, len(names))
	for i, name := range names {
		links[i] = fmt.Sprintf("[%s](#%s)", name, anchor(name))
	}
	fmt.Fprintf(&b, "%s\n\n", strings.Join(links, " · "))
	for _, name := range names {
		fmt.Fprintf(&b, "## %s\n\n", name)
		if description := tags[name]; description != "" {
			fmt.Fprintf(&b, "%s\n\n", description)
		} else if name == "General" {
			fmt.Fprintf(&b, "Logging in, reaching any endpoint, discovering "+
				"the surface, the assistant setup and the binary itself.\n\n")
		}
		cmds := sections[name]
		sort.Slice(cmds, func(i, j int) bool {
			return cmds[i].CommandPath() < cmds[j].CommandPath()
		})
		for _, cmd := range cmds {
			writeCommandDoc(&b, cmd, reg)
		}
	}
	return b.String(), nil
}

// tagDescriptions reads the one-line description of every tag
// from the embedded specification.
func tagDescriptions() map[string]string {
	var doc struct {
		Tags []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"tags"`
	}
	json.Unmarshal(spec.JSON, &doc)
	out := map[string]string{}
	for _, tag := range doc.Tags {
		out[tag.Name] = strings.TrimSuffix(tag.Description, ".") + "."
	}
	return out
}

// anchor is the heading anchor GitHub derives from a section
// name.
func anchor(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, " ", "-"))
}

// escape keeps angle brackets visible in Markdown prose, where a
// bare <file> would be read as a tag.
func escape(s string) string {
	return strings.NewReplacer("<", "\\<", ">", "\\>", "|", "\\|").Replace(s)
}

// placeholder names the value an option takes, from its pflag
// type.
func placeholder(f *pflag.Flag) string {
	switch f.Value.Type() {
	case "bool":
		return ""
	case "int", "int64":
		return " \\<n\\>"
	case "stringArray", "stringSlice":
		return " \\<value\\>..."
	case "duration":
		return " \\<duration\\>"
	default:
		return " \\<value\\>"
	}
}

func writeCommandDoc(b *strings.Builder, cmd *cobra.Command,
	reg *registry.Registry) {
	fmt.Fprintf(b, "### %s\n\n%s.\n\n", cmd.CommandPath(),
		strings.TrimSuffix(cmd.Short, "."))
	var block []string
	if args := argsOf(cmd); args != "" {
		block = append(block, cmd.CommandPath()+" "+args)
	}
	if example := strings.TrimSpace(cmd.Example); example != "" {
		block = append(block, strings.ReplaceAll(example, "\n  ", "\n"))
	}
	if len(block) > 0 {
		fmt.Fprintf(b, "```sh\n%s\n```\n\n", strings.Join(block, "\n"))
	}
	var rows []string
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == "help" {
			return
		}
		name := "`--" + f.Name + "`"
		if f.Shorthand != "" {
			name = "`-" + f.Shorthand + "`, " + name
		}
		rows = append(rows, "| "+name+placeholder(f)+" | "+
			escape(strings.ReplaceAll(f.Usage, "\n", " "))+" |")
	})
	if len(rows) > 0 {
		fmt.Fprintf(b, "| Option | What it does |\n|---|---|\n%s\n\n",
			strings.Join(rows, "\n"))
	}
	id := cmd.Annotations[annotationOperation]
	if id == "" {
		id = operationOf[cmd.CommandPath()]
	}
	if op := reg.Find(id); op != nil {
		parts := []string{fmt.Sprintf("`%s %s`", op.Method, op.Path),
			fmt.Sprintf("operation `%s`", op.ID)}
		switch {
		case len(op.Permission) > 0:
			parts = append(parts, "permission `"+
				strings.Join(op.Permission, "` or `")+"`")
		case op.Public:
			parts = append(parts, "no permission needed, the endpoint is public")
		default:
			parts = append(parts, "any consumer token")
		}
		if op.Destructive {
			parts = append(parts, "cannot be undone, needs `--yes`")
		}
		fmt.Fprintf(b, "%s\n\n", strings.Join(parts, " · "))
	}
}
