package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/transpareo/transpareo-cli/internal/registry"
	"github.com/transpareo/transpareo-cli/spec"
)

// Markdown renders the command reference: a table of contents,
// the hand-written commands first, then one section per group of
// generated commands, each command with its usage, summary,
// example, options and the operation with its permission key.
func Markdown(root *cobra.Command) (string, error) {
	reg := registry.Default()
	var b strings.Builder
	fmt.Fprintf(&b, "# Command reference\n\n")
	fmt.Fprintf(&b, "Generated from specification %s by `go generate ./...`; "+
		"do not edit by hand.\n\n", spec.Version())
	fmt.Fprintf(&b, "Every command accepts `--json`, `--jsonl`, `-q`, "+
		"`--fields`, `--profile`, `--read-only` and `--yes`; "+
		"the README says what they do. `--help` on any command "+
		"prints the same example and permission key as this page.\n\n")
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
	for _, name := range names {
		fmt.Fprintf(&b, "- [%s](#%s)\n", name, anchor(name))
	}
	b.WriteString("\n")
	for _, name := range names {
		fmt.Fprintf(&b, "## %s\n\n", name)
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
	path := cmd.CommandPath()
	if args := argsOf(cmd); args != "" {
		path += " " + args
	}
	fmt.Fprintf(b, "### %s\n\n", cmd.CommandPath())
	fmt.Fprintf(b, "%s\n\n```\n%s\n```\n\n",
		strings.TrimSuffix(cmd.Short, "."), path)
	if example := strings.TrimSpace(cmd.Example); example != "" {
		fmt.Fprintf(b, "Example:\n\n```\n%s\n```\n\n",
			strings.ReplaceAll(example, "\n  ", "\n"))
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
		fmt.Fprintf(b, "**API:** `%s %s` (`%s`). ", op.Method, op.Path, op.ID)
		switch {
		case len(op.Permission) > 0:
			fmt.Fprintf(b, "**Permission:** `%s`.",
				strings.Join(op.Permission, "` or `"))
		case op.Public:
			fmt.Fprintf(b, "**Permission:** none, the endpoint is public.")
		default:
			fmt.Fprintf(b, "**Permission:** any consumer token.")
		}
		if op.Destructive {
			fmt.Fprintf(b, " **Cannot be undone**; needs `--yes`.")
		}
		fmt.Fprintf(b, "\n\n")
	}
}
