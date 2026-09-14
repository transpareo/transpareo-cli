package cli

// These three render the vendored document, not the compiled
// operation table, so it does not matter that `go generate ./...`
// reaches this package before internal/registry.
//
//go:generate go run ./gensurface
//go:generate go run ./gendocs
//go:generate go run ./genskill

import (
	"bytes"
	"encoding/json"
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// SurfaceJSON renders the snapshot as it is committed.
func SurfaceJSON(root *cobra.Command) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(Surface(root)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// SurfaceCommand is one command of the snapshot in surface.json:
// what a script may rely on, without the prose that may change.
type SurfaceCommand struct {
	Path        string   `json:"path"`
	Args        string   `json:"args,omitempty"`
	Flags       []string `json:"flags,omitempty"`
	OperationID string   `json:"operationId,omitempty"`
}

// Surface lists every runnable command of the tree with its
// arguments and option names, sorted by path.
func Surface(root *cobra.Command) []SurfaceCommand {
	var commands []SurfaceCommand
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.Runnable() && !cmd.Hidden && cmd.Name() != "help" {
			commands = append(commands, surfaceCommand(cmd))
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
	sort.Slice(commands, func(i, j int) bool {
		return commands[i].Path < commands[j].Path
	})
	return commands
}

func surfaceCommand(cmd *cobra.Command) SurfaceCommand {
	c := SurfaceCommand{Path: cmd.CommandPath(),
		OperationID: cmd.Annotations[annotationOperation]}
	if use := argsOf(cmd); use != "" {
		c.Args = use
	}
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if !f.Hidden && f.Name != "help" {
			c.Flags = append(c.Flags, f.Name)
		}
	})
	sort.Strings(c.Flags)
	return c
}
