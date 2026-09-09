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

// Markdown renders the command reference: the hand-written
// commands first, then one section per group of generated
// commands, each command with its summary, example, options and
// permission key.
func Markdown(root *cobra.Command) (string, error) {
	reg := registry.Default()
	var b strings.Builder
	fmt.Fprintf(&b, "# Command reference\n\n")
	fmt.Fprintf(&b, "Generated from specification %s by `go generate ./...`; "+
		"do not edit by hand.\n\n", spec.Version())
	fmt.Fprintf(&b, "Every command accepts `--json`, `--jsonl`, `-q`, "+
		"`--fields`, `--profile`, `--read-only` and `--yes`. "+
		"See the README for what they do.\n\n")
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

func writeCommandDoc(b *strings.Builder, cmd *cobra.Command,
	reg *registry.Registry) {
	path := cmd.CommandPath()
	if args := argsOf(cmd); args != "" {
		path += " " + args
	}
	fmt.Fprintf(b, "### `%s`\n\n%s\n\n", path, cmd.Short)
	if example := strings.TrimSpace(cmd.Example); example != "" {
		fmt.Fprintf(b, "```\n%s\n```\n\n", strings.ReplaceAll(example, "\n  ",
			"\n"))
	}
	var options []string
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == "help" {
			return
		}
		option := "`--" + f.Name + "`"
		if f.Value.Type() != "bool" {
			option += " `<" + f.Value.Type() + ">`"
		}
		if f.Usage != "" {
			option += ": " + strings.ReplaceAll(f.Usage, "\n", " ")
		}
		options = append(options, "- "+option)
	})
	if len(options) > 0 {
		fmt.Fprintf(b, "%s\n\n", strings.Join(options, "\n"))
	}
	id := cmd.Annotations[annotationOperation]
	if id == "" {
		id = operationOf[cmd.CommandPath()]
	}
	if op := reg.Find(id); op != nil {
		fmt.Fprintf(b, "Operation `%s`, `%s %s`. ", op.ID, op.Method, op.Path)
		switch {
		case len(op.Permission) > 0:
			fmt.Fprintf(b, "Permission: `%s`.", strings.Join(op.Permission,
				"` or `"))
		case op.Public:
			fmt.Fprintf(b, "No permission needed; the endpoint is public.")
		default:
			fmt.Fprintf(b, "Any consumer token.")
		}
		if op.Destructive {
			fmt.Fprintf(b, " Cannot be undone; needs `--yes`.")
		}
		fmt.Fprintf(b, "\n\n")
	}
}
