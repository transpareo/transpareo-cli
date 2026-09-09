package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/transpareo/transpareo-cli/internal/version"
	"github.com/transpareo/transpareo-cli/spec"
)

func (a *App) versionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version of the binary and of its API specification",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			printer := a.Printer()
			if printer.JSONMode() {
				return printer.Print(map[string]string{
					"version":     version.Version,
					"commit":      version.Commit,
					"date":        version.Date,
					"specVersion": spec.Version(),
					"go":          runtime.Version(),
					"os":          runtime.GOOS,
					"arch":        runtime.GOARCH,
				})
			}
			_, err := fmt.Fprintf(a.Stdout,
				"transpareo %s\nspecification %s\n%s %s/%s\n",
				version.String(), spec.Version(), runtime.Version(),
				runtime.GOOS, runtime.GOARCH)
			return err
		},
	}
}
