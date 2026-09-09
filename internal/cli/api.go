package cli

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/transpareo/transpareo-cli/internal/output"
	"github.com/transpareo/transpareo-cli/pkg/transpareo"
)

func (a *App) apiCommand() *cobra.Command {
	var body string
	var queries, headers []string
	var idempotencyKey string
	cmd := &cobra.Command{
		Use:   "api <METHOD> <path>",
		Short: "Send an authenticated request to any endpoint",
		Long: `Sends one request with the current credential and prints the
answer. The path is relative to the API root. The body comes from
--body: inline JSON, @<file>, or - for standard input.

Operations that cannot be undone need --yes; --read-only refuses
everything but reads and validations.`,
		Example: `  transpareo api GET /dpps --query page=2 --query per_page=50
  transpareo api POST /dpps/validate --body @passport.json
  transpareo api PUT /products/42 --body '{"product": {"name": "New name"}}'
  transpareo api DELETE /webhooks/7 --yes`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.api(cmd, strings.ToUpper(args[0]), args[1], body,
				queries, headers, idempotencyKey)
		},
	}
	f := cmd.Flags()
	f.StringVar(&body, "body", "",
		"request body: JSON, @<file>, or - for stdin")
	f.StringArrayVar(&queries, "query", nil,
		"query parameter as name=value (repeatable)")
	f.StringArrayVar(&headers, "header", nil,
		"extra header as Name: value (repeatable)")
	f.StringVar(&idempotencyKey, "idempotency-key", "",
		"Idempotency-Key for a POST (default: random)")
	return cmd
}

var methods = map[string]bool{
	http.MethodGet: true, http.MethodPost: true, http.MethodPut: true,
	http.MethodPatch: true, http.MethodDelete: true,
}

func (a *App) api(cmd *cobra.Command, method, path, body string,
	queries, headers []string, idempotencyKey string) error {
	if !methods[method] {
		return output.Exit(output.ExitUsage,
			fmt.Errorf("unsupported method %q", method))
	}
	reg, err := a.Registry()
	if err != nil {
		return err
	}
	if err := a.refuseUnlessAllowed(reg.Match(method, path), method); err != nil {
		return err
	}
	query, err := parsePairs(queries, "=")
	if err != nil {
		return err
	}
	header, err := parsePairs(headers, ":")
	if err != nil {
		return err
	}
	data, err := a.readBody(body)
	if err != nil {
		return err
	}
	client, _, err := a.Client()
	if err != nil {
		return err
	}
	req := &transpareo.Request{
		Method:         method,
		Path:           path,
		Query:          url.Values(query),
		Header:         http.Header(header),
		IdempotencyKey: idempotencyKey,
	}
	if data != nil {
		req.Body = data
	}
	resp, err := client.Do(cmd.Context(), req)
	if err != nil {
		return err
	}
	return a.printResponse(resp)
}

// printResponse prints a JSON body through the printer and any
// other body as it is.
func (a *App) printResponse(resp *transpareo.Response) error {
	printer := a.Printer()
	contentType := resp.Header.Get("Content-Type")
	if len(resp.Body) == 0 {
		if !printer.JSONMode() {
			printer.Message("HTTP %d, no content.", resp.StatusCode)
		}
		return nil
	}
	isJSON := strings.Contains(contentType, "json") &&
		!strings.Contains(contentType, "ndjson")
	if isJSON {
		return printer.Print(resp.Body)
	}
	return printer.PrintRaw(resp.Body)
}

// readBody resolves the --body option: empty, inline JSON, a
// file, or standard input.
func (a *App) readBody(body string) ([]byte, error) {
	switch {
	case body == "":
		return nil, nil
	case body == "-":
		return io.ReadAll(a.Stdin)
	case strings.HasPrefix(body, "@"):
		data, err := os.ReadFile(body[1:])
		if err != nil {
			return nil, output.Exit(output.ExitUsage, err)
		}
		return data, nil
	default:
		return []byte(body), nil
	}
}

// parsePairs splits repeatable "name<sep>value" options.
func parsePairs(pairs []string, sep string) (map[string][]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	out := map[string][]string{}
	for _, pair := range pairs {
		name, value, ok := strings.Cut(pair, sep)
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, output.Exit(output.ExitUsage,
				fmt.Errorf("%q is not name%svalue", pair, sep))
		}
		out[name] = append(out[name], strings.TrimSpace(value))
	}
	return out, nil
}
