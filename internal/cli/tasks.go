package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"github.com/transpareo/transpareo-cli/internal/output"
	"github.com/transpareo/transpareo-cli/pkg/transpareo"
)

func (a *App) tasksCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tasks",
		Short: "Work colleagues hand each other, and waiting on background work",
	}
	cmd.AddCommand(a.tasksWaitCommand())
	return cmd
}

func (a *App) tasksWaitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "wait <statusUrl>",
		Short: "Poll a status URL until the work is done",
		Long: `Polls the statusUrl that a bulk create, an export or an import
answered, with waits growing from one to thirty seconds, until the
status is final. Progress goes to standard error, the final
document to standard output. A task that failed exits with 1.
The URL may be absolute, as the API hands it out, or relative to
the API root.`,
		Example: "  transpareo tasks wait https://acme.example.com/api/exports/42\n" +
			"  transpareo tasks wait /dpps/bulk/507f1f77bcf86cd799439099",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			statusURL := strings.TrimSpace(args[0])
			if statusURL == "" {
				return output.Exit(output.ExitUsage, errors.New("a status URL is required"))
			}
			client, _, err := a.Client()
			if err != nil {
				return err
			}
			printer := a.Printer()
			task, err := client.WaitForTask(cmd.Context(), statusURL,
				&transpareo.WaitOptions{OnPoll: func(t *transpareo.Task) {
					printer.Message("%s %d%%", t.Status, t.Progress)
				}})
			if err != nil {
				return err
			}
			if err := printer.Print(task.Body); err != nil {
				return err
			}
			if task.Failed() {
				return output.Exit(output.ExitAPI, nil)
			}
			return nil
		},
	}
}
