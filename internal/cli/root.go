package cli

import (
	"github.com/spf13/cobra"
)

// Root builds the command tree.
func (a *App) Root() *cobra.Command {
	root := &cobra.Command{
		Use:   "transpareo",
		Short: "Drive a Transpareo workspace from the terminal",
		Long: `transpareo talks to the Transpareo API of one workspace: it
logs in with client credentials, reaches every endpoint, and
prints JSON that scripts and assistants can read.

Start with:

  transpareo auth login --host acme.example.com --client-id <key>
  transpareo me`,
		SilenceUsage:  true,
		SilenceErrors: true,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: false,
		},
	}
	a.addOutputFlags(root)
	root.AddCommand(
		a.authCommand(),
		a.meCommand(),
		a.apiCommand(),
		a.commandsCommand(),
		a.schemaCommand(),
		a.guideCommand(),
		a.doctorCommand(),
		a.versionCommand(),
		a.mcpCommand(),
		a.setupCommand(),
		a.upgradeCommand(),
		a.tasksCommand(),
		a.signerCommand(),
	)
	a.addGeneratedCommands(root)
	a.addCompositeCommands(root)
	return root
}
