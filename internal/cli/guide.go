package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/transpareo/transpareo-cli/pkg/transpareo"
)

func (a *App) guideCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "guide",
		Short: "Print the workspace's API guide as Markdown",
		Long: `Fetches /apidocs/guide.md from the workspace host: credentials,
permissions, pagination, idempotency, the passport flows, the
event feed, webhooks, rate limits and the error format.`,
		Example: "  transpareo guide | less",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			host, err := a.Resolver().Host(a.Profile)
			if err != nil {
				return noProfileHint(err)
			}
			data, err := a.fetchPublic(cmd.Context(), host,
				"/apidocs/guide.md", "text/markdown")
			if err != nil {
				return err
			}
			return a.Printer().PrintRaw(data)
		},
	}
}

// fetchPublic reads a document that needs no credential from the
// workspace host, such as the guide or the specification.
func (a *App) fetchPublic(ctx context.Context, host, path,
	accept string) ([]byte, error) {
	client, err := transpareo.NewWithToken(host, "public", a.clientOptions()...)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		client.Host()+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "transpareo-cli")
	resp, err := a.httpClient().Do(req)
	if err != nil {
		return nil, &transpareo.Error{Code: transpareo.CodeNetwork,
			Message: err.Error(), Retryable: true}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &transpareo.Error{Code: transpareo.CodeInvalidResponse,
			Status: resp.StatusCode,
			Message: fmt.Sprintf("%s answered HTTP %d for %s", client.Host(),
				resp.StatusCode, path)}
	}
	return data, nil
}
