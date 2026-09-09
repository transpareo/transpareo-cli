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

// Command describes one hand-written command in the catalogue.
type Command struct {
	Path        string   `json:"path"`
	Summary     string   `json:"summary"`
	Example     string   `json:"example,omitempty"`
	Flags       []Flag   `json:"flags,omitempty"`
	Args        string   `json:"args,omitempty"`
	OperationID string   `json:"operationId,omitempty"`
	Permission  []string `json:"permission,omitempty"`
}

// Flag is one option of a command.
type Flag struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// Catalogue is what `transpareo commands --json` prints: every
// command of the binary and every operation of the specification
// it was built with.
type Catalogue struct {
	SpecVersion string               `json:"specVersion"`
	Commands    []Command            `json:"commands"`
	Operations  []registry.Operation `json:"operations"`
}

// operationOf maps hand-written commands to the operation they
// call, so the catalogue carries the permission key.
var operationOf = map[string]string{
	"transpareo auth status": "get_me",
	"transpareo me":          "get_me",
	"transpareo auth grant":  "create_grant",
	"transpareo auth login":  "exchange_token",
	"transpareo auth token":  "exchange_token",
}

func (a *App) commandsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "commands",
		Short: "List every command and every API operation, for agents",
		Long: `Prints the catalogue of the binary: the commands with their
options and examples, and the operations of the API specification
it was built with. Use --json to read it from a program.`,
		Example: "  transpareo commands --json | jq '.operations[] | .operationId'",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			catalogue, err := a.catalogue(cmd.Root())
			if err != nil {
				return err
			}
			printer := a.Printer()
			if printer.JSONMode() {
				return printer.Print(catalogue)
			}
			for _, c := range catalogue.Commands {
				fmt.Fprintf(a.Stdout, "%-36s %s\n", c.Path, c.Summary)
			}
			fmt.Fprintf(a.Stdout, "\n%d API operations in specification %s; "+
				"see `transpareo commands --json` and `transpareo schema <operationId>`.\n",
				len(catalogue.Operations), catalogue.SpecVersion)
			return nil
		},
	}
}

func (a *App) catalogue(root *cobra.Command) (*Catalogue, error) {
	reg, err := a.Registry()
	if err != nil {
		return nil, err
	}
	catalogue := &Catalogue{
		SpecVersion: spec.Version(),
		Operations:  reg.Operations,
	}
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.Runnable() && !cmd.Hidden && cmd.Name() != "help" {
			catalogue.Commands = append(catalogue.Commands, describeCommand(cmd, reg))
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
	sort.Slice(catalogue.Commands, func(i, j int) bool {
		return catalogue.Commands[i].Path < catalogue.Commands[j].Path
	})
	return catalogue, nil
}

func describeCommand(cmd *cobra.Command, reg *registry.Registry) Command {
	c := Command{
		Path:    cmd.CommandPath(),
		Summary: cmd.Short,
		Example: strings.TrimSpace(cmd.Example),
	}
	use := strings.TrimPrefix(cmd.Use, cmd.Name())
	if use = strings.TrimSpace(use); use != "" {
		c.Args = use
	}
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		c.Flags = append(c.Flags, Flag{Name: f.Name, Type: f.Value.Type(),
			Description: f.Usage})
	})
	if id, ok := operationOf[c.Path]; ok {
		c.OperationID = id
		if op := reg.Find(id); op != nil {
			c.Permission = op.Permission
		}
	}
	return c
}
