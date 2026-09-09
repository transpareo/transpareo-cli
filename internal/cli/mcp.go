package cli

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/spf13/cobra"

	"github.com/transpareo/transpareo-cli/internal/mcp"
	"github.com/transpareo/transpareo-cli/internal/output"
	"github.com/transpareo/transpareo-cli/internal/version"
	"github.com/transpareo/transpareo-cli/pkg/transpareo"
)

func (a *App) mcpCommand() *cobra.Command {
	var groups []string
	cmd := &cobra.Command{
		Use: "mcp [--tools <group,...>] [--read-only]",
		Short: "Start the Model Context Protocol server over standard " +
			"input and output",
		Long: `Serves the curated tools, search_operations,
			call_api and the guide,
openapi and me resources to an assistant over standard input and
output. Log output goes to standard error only. The credential
comes from the profile, so the assistant's configuration holds no
secret:

  { "command": "transpareo", "args": ["mcp", "--profile", "acme"] }

--tools narrows the tools to groups (` + strings.Join(mcp.Groups, ", ") + `);
--read-only removes every tool that changes data and keeps the
validations.`,
		Example: "  transpareo mcp --profile acme\n" +
			"  transpareo mcp --profile acme --tools dpps,products --read-only",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, g := range groups {
				if !contains(mcp.Groups, g) {
					return output.Exit(output.ExitUsage, fmt.Errorf(
						"unknown tool group %q; use %s", g,
						strings.Join(mcp.Groups, ", ")))
				}
			}
			server := mcp.New(mcp.Options{
				Client: func() (*transpareo.Client, error) {
					client, _, err := a.Client()
					return client, err
				},
				Version:  version.Version,
				Groups:   groups,
				ReadOnly: a.ReadOnly,
				Logger:   slog.New(slog.NewTextHandler(a.Stderr, nil)),
			})
			err := server.Run(cmd.Context())
			if err != nil && !errors.Is(err, cmd.Context().Err()) {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&groups, "tools", nil,
		"tool groups to serve, comma separated (default: all)")
	return cmd
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
