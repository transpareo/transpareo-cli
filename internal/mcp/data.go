package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/transpareo/transpareo-cli/internal/flows"
)

// dataTools are the composite tools of the data group: the flows
// behind imports, exports, tasks and the event feed. They are
// hand-written because each spans several operations.
func (s *Server) addDataTools() {
	if !s.wants(GroupData) {
		return
	}
	s.addTool(GroupData, &sdk.Tool{
		Name: "wait_for_task",
		Description: "Poll the statusUrl a bulk create, export or import " +
			"answered " +
			"until the work is done, and return the final document. " +
			"Permission: " +
			"the one of the work that was started. Data tier: authorised. " +
			"Example: wait_for_task {\"statusUrl\": " +
			"\"https://<host>/api/exports/42\"}",
		InputSchema: schema(map[string]any{
			"statusUrl": map[string]any{"type": "string",
				"description": "The statusUrl of the answer that started the " +
					"work"},
			"timeoutSeconds": map[string]any{"type": "integer",
				"description": "Give up after this many seconds (default 600)"},
		}, "statusUrl"),
		Annotations: &sdk.ToolAnnotations{Title: "Wait for a task",
			ReadOnlyHint: true},
	}, s.waitForTask)

	s.addTool(GroupData, &sdk.Tool{
		Name: "tail_events",
		Description: "Read the workspace's passport events since a cursor or " +
			"a " +
			"time, and the next cursor to keep. Permission: dpp_history. " +
			"Data " +
			"tier: authorised (events carry actor identifiers). Example: " +
			"tail_events {\"since\": \"2026-09-01T00:00:00Z\", \"types\": " +
			"\"published\"}",
		InputSchema: schema(map[string]any{
			"since": map[string]any{"type": "string",
				"description": "The nextCursor of the previous answer, or an " +
					"ISO 8601 time"},
			"types": map[string]any{"type": "string",
				"description": "Event types to keep, comma separated"},
			"dppCode": map[string]any{"type": "string",
				"description": "Confine the feed to one passport"},
			"limit": map[string]any{"type": "integer",
				"description": "Events per answer, up to 500"},
		}),
		Annotations: &sdk.ToolAnnotations{Title: "Tail events",
			ReadOnlyHint: true},
	}, s.tailEvents)

	if s.opts.ReadOnly {
		return
	}
	s.addTool(GroupData, &sdk.Tool{
		Name: "export_catalogue",
		Description: "Start an export of the passport catalogue and wait for " +
			"the " +
			"archive; the answer carries the downloadUrl for the command " +
			"line " +
			"(`transpareo exports create --download`). One export runs at a " +
			"time per consumer. Permission: export_access. Data tier: " +
			"authorised. Example: export_catalogue {\"format\": \"csv\"}",
		InputSchema: schema(map[string]any{
			"format": map[string]any{"type": "string",
				"enum": []string{"jsonld", "csv", "xlsx", "sql"}},
			"normalize":    map[string]any{"type": "boolean"},
			"includeMedia": map[string]any{"type": "boolean"},
			"wait": map[string]any{"type": "boolean",
				"description": "Wait for the archive (default true)"},
		}),
		Annotations: &sdk.ToolAnnotations{Title: "Export the catalogue"},
	}, s.exportCatalogue)

	s.addTool(GroupData, &sdk.Tool{
		Name: "import_spreadsheet",
		Description: "Upload a spreadsheet or JSON file from a path on this " +
			"machine, map its columns, validate and, only with execute true " +
			"after a clean validation, write the records. When columns stay " +
			"unresolved the answer lists them with the platform's " +
			"suggestions " +
			"and the core attributes to target; write the mappings and call " +
			"again. The tool never creates a property type unless a mapping " +
			"says create_new. Permission: import_access plus the write " +
			"permission of the data type. Data tier: authorised. Example: " +
			"import_spreadsheet {\"path\": \"/data/catalogue.xlsx\", " +
			"\"dataType\": \"products\", \"acceptSuggestions\": true}",
		InputSchema: schema(map[string]any{
			"path": map[string]any{"type": "string",
				"description": "File path on this machine"},
			"dataType": map[string]any{"type": "string",
				"enum": []string{"components", "products", "dpps"}},
			"mappings": map[string]any{"type": "object",
				"description": "One action per column, as " +
					"ImportMappingsInput.mappings"},
			"acceptSuggestions": map[string]any{"type": "boolean",
				"description": "Take every exact match of the preview"},
			"execute": map[string]any{"type": "boolean",
				"description": "Write the records after a clean validation"},
			"published": map[string]any{"type": "boolean",
				"description": "Publish the records the import creates"},
		}, "path"),
		Annotations: &sdk.ToolAnnotations{Title: "Import a spreadsheet"},
	}, s.importSpreadsheet)
}

func (s *Server) wants(group string) bool {
	if len(s.opts.Groups) == 0 {
		return true
	}
	for _, g := range s.opts.Groups {
		if g == group {
			return true
		}
	}
	return false
}

// addTool registers a tool with a handler that takes decoded
// arguments.
func (s *Server) addTool(group string, tool *sdk.Tool,
	handler func(context.Context, map[string]any) *sdk.CallToolResult) {
	s.composed = append(s.composed, composedTool{
		Name: tool.Name, Group: group, Tool: tool})
	s.Server.AddTool(tool, func(ctx context.Context,
		req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		args, err := arguments(req)
		if err != nil {
			return failure(err), nil
		}
		return handler(ctx, args), nil
	})
}

func schema(props map[string]any, required ...string) map[string]any {
	out := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

func (s *Server) waitForTask(ctx context.Context,
	args map[string]any) *sdk.CallToolResult {
	statusURL := stringArg(args["statusUrl"])
	if statusURL == "" {
		return failure(errors.New("statusUrl is required"))
	}
	timeout := 600 * time.Second
	if secs, ok := args["timeoutSeconds"].(float64); ok && secs > 0 {
		timeout = time.Duration(secs) * time.Second
	}
	client, err := s.api()
	if err != nil {
		return failure(err)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	task, err := client.WaitForTask(ctx, statusURL, nil)
	if err != nil {
		return failure(err)
	}
	var doc any
	json.Unmarshal(task.Body, &doc)
	text := fmt.Sprintf("task %s (%d%%)", task.Status, task.Progress)
	return &sdk.CallToolResult{IsError: task.Failed(),
		Content:           []sdk.Content{&sdk.TextContent{Text: text}},
		StructuredContent: doc}
}

func (s *Server) tailEvents(ctx context.Context,
	args map[string]any) *sdk.CallToolResult {
	client, err := s.api()
	if err != nil {
		return failure(err)
	}
	opts := flows.FeedOptions{Since: stringArg(args["since"]),
		Types: stringArg(args["types"]), DppCode: stringArg(args["dppCode"])}
	if limit, ok := args["limit"].(float64); ok {
		opts.Limit = int(limit)
	}
	feed, err := flows.ReadFeed(ctx, client, opts)
	if err != nil {
		return failure(err)
	}
	events := make([]any, 0, len(feed.Events))
	for _, raw := range feed.Events {
		var ev any
		json.Unmarshal(raw, &ev)
		events = append(events, ev)
	}
	text := fmt.Sprintf("%d events; continue with since %q", len(events),
		feed.NextCursor)
	return &sdk.CallToolResult{
		Content: []sdk.Content{&sdk.TextContent{Text: text}},
		StructuredContent: map[string]any{"events": events,
			"nextCursor": feed.NextCursor},
	}
}

func (s *Server) exportCatalogue(ctx context.Context,
	args map[string]any) *sdk.CallToolResult {
	client, err := s.api()
	if err != nil {
		return failure(err)
	}
	opts := flows.ExportOptions{Format: stringArg(args["format"]), Wait: true}
	if wait, ok := args["wait"].(bool); ok {
		opts.Wait = wait
	}
	if normalize, ok := args["normalize"].(bool); ok {
		opts.Normalize = &normalize
	}
	if media, ok := args["includeMedia"].(bool); ok {
		opts.IncludeMedia = &media
	}
	exp, err := flows.StartExport(ctx, client, opts)
	if err != nil {
		return failure(err)
	}
	var doc any
	json.Unmarshal(exp.Body, &doc)
	return &sdk.CallToolResult{
		Content:           []sdk.Content{&sdk.TextContent{Text: exp.Summary()}},
		StructuredContent: doc,
	}
}

func (s *Server) importSpreadsheet(ctx context.Context,
	args map[string]any) *sdk.CallToolResult {
	client, err := s.api()
	if err != nil {
		return failure(err)
	}
	path := stringArg(args["path"])
	if path == "" {
		return failure(errors.New("path is required"))
	}
	var mappings map[string]flows.Mapping
	if raw, ok := args["mappings"].(map[string]any); ok {
		data, _ := json.Marshal(raw)
		if err := json.Unmarshal(data, &mappings); err != nil {
			return failure(fmt.Errorf("mappings: %w", err))
		}
	}
	opts := flows.RunOptions{Path: path, DataType: stringArg(args["dataType"]),
		Mappings: mappings}
	opts.AcceptSuggestions, _ = args["acceptSuggestions"].(bool)
	opts.Execute, _ = args["execute"].(bool)
	if published, ok := args["published"].(bool); ok {
		opts.Options = &flows.MappingOptions{Published: &published}
	}
	imp, err := flows.Run(ctx, client, opts)
	var required *flows.ErrMappingRequired
	var failed *flows.ErrValidationFailed
	switch {
	case err == nil:
		var doc any
		json.Unmarshal(imp.Body, &doc)
		text := fmt.Sprintf("import %s %s", imp.ID, imp.Status)
		if !opts.Execute {
			text += "; validation passed, call again with execute true to write"
		}
		return &sdk.CallToolResult{
			Content:           []sdk.Content{&sdk.TextContent{Text: text}},
			StructuredContent: doc,
		}
	case errors.As(err, &required):
		structured := map[string]any{"importId": required.Import.ID,
			"unresolved": required.Unresolved}
		if required.Import.Preview != nil {
			structured["coreAttributes"] = required.Import.Preview.CoreAttributes
			structured["propertyTypes"] = required.Import.Preview.PropertyTypes
		}
		return &sdk.CallToolResult{IsError: true,
			Content: []sdk.Content{&sdk.TextContent{Text: err.Error() +
				"; write mappings for them and call again"}},
			StructuredContent: structured}
	case errors.As(err, &failed):
		var doc any
		json.Unmarshal(failed.Import.Body, &doc)
		return &sdk.CallToolResult{IsError: true,
			Content:           []sdk.Content{&sdk.TextContent{Text: err.Error()}},
			StructuredContent: doc}
	default:
		return failure(err)
	}
}
