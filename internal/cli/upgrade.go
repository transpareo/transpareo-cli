package cli

import (
	"github.com/spf13/cobra"

	"github.com/transpareo/transpareo-cli/internal/upgrade"
	"github.com/transpareo/transpareo-cli/internal/version"
)

func (a *App) upgradeCommand() *cobra.Command {
	var pin string
	var check, verify bool
	cmd := &cobra.Command{
		Use:   "upgrade [--version <x.y.z>] [--check]",
		Short: "Replace this binary with a verified release from GitHub",
		Long: `Downloads the release for this platform from GitHub, verifies the
Sigstore signature of its checksum file against the release
workflow's identity and the embedded Sigstore trusted root,
checks the archive's checksum, and replaces this binary. Nothing
is installed when either check fails. This is the only network
call the tool makes besides the workspace host.

--verify checks this binary instead of replacing it: it fetches
the release it came from, verifies the signature and the checksum,
and compares the binary byte for byte with the one in the archive.`,
		Example: "  transpareo upgrade\n  transpareo upgrade --check\n" +
			"  transpareo upgrade --version 1.2.0\n  transpareo upgrade --verify",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			printer := a.Printer()
			opts := upgrade.Options{Version: pin, Current: version.Version,
				HTTP:     a.httpClient(),
				Progress: func(s string) { printer.Message("%s", s) }}
			if verify {
				result, err := upgrade.Verify(cmd.Context(), opts)
				if result != nil {
					if printErr := printer.Print(result); printErr != nil {
						return printErr
					}
				}
				return err
			}
			if check {
				release, err := upgrade.Check(cmd.Context(), opts)
				if err != nil {
					return err
				}
				return printer.Print(map[string]any{"current": version.Version,
					"available": release.Version})
			}
			result, err := upgrade.Run(cmd.Context(), opts)
			if err != nil {
				return err
			}
			return printer.Print(result)
		},
	}
	cmd.Flags().StringVar(&pin, "version", "",
		"install this version instead of the latest")
	cmd.Flags().BoolVar(&check, "check", false,
		"report the latest version without installing")
	cmd.Flags().BoolVar(&verify, "verify", false,
		"verify this binary against its release")
	return cmd
}
