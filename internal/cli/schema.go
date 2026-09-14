package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/transpareo/transpareo-cli/internal/output"
	"github.com/transpareo/transpareo-cli/internal/registry"
)

// Schema is what `transpareo schema` prints for one operation.
type Schema struct {
	OperationID    string           `json:"operationId"`
	Method         string           `json:"method"`
	Path           string           `json:"path"`
	Summary        string           `json:"summary,omitempty"`
	Description    string           `json:"description,omitempty"`
	Permission     []string         `json:"permission"`
	Destructive    bool             `json:"destructive"`
	Safe           bool             `json:"safe"`
	UserOnly       bool             `json:"userOnly"`
	PathParams     []registry.Param `json:"pathParams,omitempty"`
	QueryParams    []registry.Param `json:"queryParams,omitempty"`
	RequestBody    json.RawMessage  `json:"requestBody,omitempty"`
	RequestExample json.RawMessage  `json:"requestExample,omitempty"`

	// RequestBodies names every media type when the operation
	// takes more than one, so no body stays hidden behind the
	// default.
	RequestBodies  []string        `json:"requestBodies,omitempty"`
	ResponseSchema json.RawMessage `json:"responseSchema,omitempty"`
	ResponseStatus string          `json:"responseStatus,omitempty"`
}

// completeOperationIDs offers the operation ids for shell
// completion of the first argument.
func (a *App) completeOperationIDs(cmd *cobra.Command, args []string,
	toComplete string) ([]string, cobra.ShellCompDirective) {
	reg, err := a.Registry()
	if err != nil || len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var ids []string
	for _, op := range reg.Operations {
		if strings.HasPrefix(op.ID, toComplete) {
			ids = append(ids, op.ID)
		}
	}
	return ids, cobra.ShellCompDirectiveNoFileComp
}

func (a *App) schemaCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "schema <operationId>",
		Short: "Print the request schema and example of an operation",
		Long: `Prints the JSON Schema of the request body, the parameters and the
example of one operation, with every reference resolved. Building
a payload starts here. Operation ids come from
` + "`transpareo commands --json`.",
		Example: "  transpareo schema create_dpp\n" +
			"  transpareo schema list_dpps --fields queryParams",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: a.completeOperationIDs,
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, err := a.Registry()
			if err != nil {
				return err
			}
			op := reg.Find(args[0])
			if op == nil {
				return output.Exit(output.ExitUsage, fmt.Errorf(
					"unknown operation %q; list them with `transpareo commands --json`",
					args[0]))
			}
			printer := a.Printer()
			printer.JSON = true
			var schema, example json.RawMessage
			if body := op.DefaultBody(); body != nil {
				schema, example = body.Schema, body.Example
			}

			// A single body is already the one printed.
			var bodies []string
			if len(op.RequestBodies) > 1 {
				bodies = op.ContentTypes()
			}
			return printer.Print(Schema{
				OperationID:    op.ID,
				Method:         op.Method,
				Path:           op.Path,
				Summary:        op.Summary,
				Description:    op.Description,
				Permission:     op.Permission,
				Destructive:    op.Destructive,
				Safe:           op.Safe,
				UserOnly:       op.UserOnly,
				PathParams:     op.PathParams,
				QueryParams:    op.QueryParams,
				RequestBody:    schema,
				RequestExample: example,
				RequestBodies:  bodies,
				ResponseSchema: op.ResponseSchema,
				ResponseStatus: op.ResponseStatus,
			})
		},
	}
}
