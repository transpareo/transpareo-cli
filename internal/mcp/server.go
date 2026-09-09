package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/transpareo/transpareo-cli/internal/output"
	"github.com/transpareo/transpareo-cli/internal/registry"
	"github.com/transpareo/transpareo-cli/pkg/transpareo"
	"github.com/transpareo/transpareo-cli/spec"
)

// Options configure a server.
type Options struct {
	// Client builds the API client on first use, so the server
	// starts before any credential is read and reports a missing
	// one in the tool result.
	Client func() (*transpareo.Client, error)

	// Version is the binary's version, sent to the assistant.
	Version string

	// Groups limits the tools to these groups; empty means all.
	Groups []string

	// ReadOnly removes every tool that changes data; validations
	// stay.
	ReadOnly bool

	// Logger receives the server's log; nil logs to stderr.
	Logger *slog.Logger
}

// instructions is what the assistant reads when it connects.
const instructions = `Transpareo holds a workspace's products,
	components and Digital
Product Passports. Start with me to learn what the credential
allows. For a new product call product_property_types before
create_product. For passports of an existing product call
dpp_requirements, then validate_dpp before create_dpp. publish_dpp
signs the passport and cannot be undone. void_dpp and
supersede_dpp need the confirm argument with the stated phrase.
Anything without a tool: search_operations, then call_api. Lists
are paged; follow nextPage.`

// Server wraps the SDK server with the client and the registry.
type Server struct {
	*sdk.Server
	opts   Options
	reg    *registry.Registry
	tools  []Tool
	extra  []string
	logger *slog.Logger

	once   sync.Once
	client *transpareo.Client
	err    error
}

// New builds the server with its tools and resources.
func New(opts Options) *Server {
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	s := &Server{
		opts:   opts,
		reg:    registry.Default(),
		logger: logger,
	}
	s.Server = sdk.NewServer(
		&sdk.Implementation{Name: "transpareo", Version: opts.Version},
		&sdk.ServerOptions{Instructions: instructions, Logger: logger})
	s.tools = Tools(s.reg, opts.Groups, opts.ReadOnly)
	for _, t := range s.tools {
		s.addCurated(t)
	}
	s.addDiscovery()
	s.addDataTools()
	s.addResources()
	return s
}

func (s *Server) api() (*transpareo.Client, error) {
	s.once.Do(func() {
		if s.opts.Client == nil {
			s.err = fmt.Errorf("no client configured")
			return
		}
		s.client, s.err = s.opts.Client()
	})
	return s.client, s.err
}

// ToolNames lists the registered tools, for tests and doctor.
func (s *Server) ToolNames() []string {
	names := []string{"search_operations", "call_api"}
	for _, t := range s.tools {
		names = append(names, t.Name)
	}
	names = append(names, s.extra...)
	sort.Strings(names)
	return names
}

func (s *Server) addCurated(t Tool) {
	op := s.reg.Find(t.Operation)
	schema := t.inputSchema(op)
	tool := &sdk.Tool{
		Name:         t.Name,
		Description:  t.describe(op, schema),
		InputSchema:  schema,
		OutputSchema: t.outputSchema(op),
		Annotations:  annotations(op),
	}
	s.Server.AddTool(tool, func(ctx context.Context,
		req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		args, err := arguments(req)
		if err != nil {
			return failure(err), nil
		}
		return s.callCurated(ctx, t, op, args), nil
	})
}

// annotations derive the protocol hints from the registry.
func annotations(op *registry.Operation) *sdk.ToolAnnotations {
	readOnly := op.ReadOnly()
	destructive := op.Destructive
	return &sdk.ToolAnnotations{
		Title:           op.Summary,
		ReadOnlyHint:    readOnly,
		DestructiveHint: &destructive,
		IdempotentHint:  op.Method == http.MethodPut || readOnly,
	}
}

func arguments(req *sdk.CallToolRequest) (map[string]any, error) {
	args := map[string]any{}
	if len(req.Params.Arguments) == 0 {
		return args, nil
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("arguments must be a JSON object: %w", err)
	}
	return args, nil
}

// callCurated maps the arguments onto the request and shapes the
// answer.
func (s *Server) callCurated(ctx context.Context, t Tool,
	op *registry.Operation,
	args map[string]any) *sdk.CallToolResult {
	if t.Confirm != "" {
		want := t.confirmPhrase(op)
		for _, p := range op.PathParams {
			want = strings.ReplaceAll(want, "<"+p.Name+">",
				stringArg(args[p.Name]))
		}
		if stringArg(args["confirm"]) != want {
			return failure(fmt.Errorf("%s cannot be undone; pass confirm: %q",
				t.Name, want))
		}
	}
	req, err := s.buildRequest(t, op, args)
	if err != nil {
		return failure(err)
	}
	client, err := s.api()
	if err != nil {
		return failure(err)
	}
	resp, err := client.Do(ctx, req)
	if err != nil {
		return failure(err)
	}
	fields := stringList(args["fields"])
	return s.shape(t, op, resp, fields)
}

// buildRequest fills the path, the query and the body from the
// arguments.
func (s *Server) buildRequest(t Tool, op *registry.Operation,
	args map[string]any) (*transpareo.Request, error) {
	path := op.Path
	used := map[string]bool{"fields": true, "confirm": true}
	for _, p := range op.PathParams {
		value := stringArg(args[p.Name])
		if value == "" {
			return nil, fmt.Errorf("%s is required", p.Name)
		}
		path = strings.ReplaceAll(path, "{"+p.Name+"}", url.PathEscape(value))
		used[p.Name] = true
	}
	req := &transpareo.Request{Method: op.Method, Path: path,
		Query:  url.Values{},
		Header: http.Header{}}
	switch t.Kind {
	case kindList, kindGet:
		for _, p := range op.QueryParams {
			if value, ok := args[p.Name]; ok && value != nil {
				req.Query.Set(p.Name, stringArg(value))
			}
		}
	case kindWrite:
		if op.RequestContentType == "" {
			break
		}
		body := map[string]any{}
		for key, value := range args {
			if !used[key] {
				body[key] = value
			}
		}
		req.Body = body
	case kindRows:
		rows, _ := args["rows"].([]any)
		if len(rows) == 0 {
			return nil, fmt.Errorf("rows must carry at least one passport")
		}
		var lines []string
		if shared, ok := args["shared"].(map[string]any); ok {
			line, _ := json.Marshal(map[string]any{"shared": shared})
			lines = append(lines, string(line))
		}
		for _, row := range rows {
			line, err := json.Marshal(row)
			if err != nil {
				return nil, err
			}
			lines = append(lines, string(line))
		}
		req.Body = []byte(strings.Join(lines, "\n") + "\n")
		req.ContentType = "application/x-ndjson"
		if async, _ := args["async"].(bool); async {
			req.Header.Set("Prefer", "respond-async")
		}
	}
	return req, nil
}

// shape turns the answer into a one-line summary plus structured
// content.
func (s *Server) shape(t Tool, op *registry.Operation,
	resp *transpareo.Response,
	fields []string) *sdk.CallToolResult {
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "ndjson") {
		return ndjsonResult(resp.Body)
	}
	var value any
	if len(resp.Body) > 0 {
		if err := json.Unmarshal(resp.Body, &value); err != nil {
			return &sdk.CallToolResult{Content: []sdk.Content{
				&sdk.TextContent{Text: string(resp.Body)}}}
		}
	}
	if t.Kind == kindList {
		return listResult(resp, value, fields)
	}
	value = output.Project(value, fields)
	return &sdk.CallToolResult{
		Content: []sdk.Content{&sdk.TextContent{Text: summarise(op,
			value)}},
		StructuredContent: value,
	}
}

func listResult(resp *transpareo.Response, value any,
	fields []string) *sdk.CallToolResult {
	items := []any{}
	if obj, ok := value.(map[string]any); ok {
		keys := make([]string, 0, len(obj))
		for key := range obj {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if list, ok := obj[key].([]any); ok {
				items = list
				break
			}
		}
	} else if list, ok := value.([]any); ok {
		items = list
	}
	page := transpareoPage(resp)
	if page.Page == 0 {
		page.Page = 1
	}
	next := 0
	if page.NextURL != "" {
		next = page.Page + 1
	}
	projected := output.Project(items, fields)
	structured := map[string]any{"items": projected, "page": page.Page,
		"total": page.Total, "nextPage": next}
	text := fmt.Sprintf("%d items on page %d", len(items), page.Page)
	if page.Total > 0 {
		text += fmt.Sprintf(" of %d in total", page.Total)
	}
	if next > 0 {
		text += fmt.Sprintf("; continue with page %d", next)
	}
	return &sdk.CallToolResult{
		Content:           []sdk.Content{&sdk.TextContent{Text: text}},
		StructuredContent: structured,
	}
}

func transpareoPage(resp *transpareo.Response) transpareo.Page[any] {
	header := func(name string) int {
		var n int
		fmt.Sscan(resp.Header.Get(name), &n)
		return n
	}
	page := transpareo.Page[any]{Total: header("API-Total"),
		Page:    header("API-Page"),
		PerPage: header("API-Per-Page")}
	for _, link := range resp.Header.Values("Link") {
		if strings.Contains(link, `rel="next"`) {
			page.NextURL = link
		}
	}
	return page
}

func ndjsonResult(body []byte) *sdk.CallToolResult {
	var rows []any
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var row any
		if err := json.Unmarshal([]byte(line), &row); err == nil {
			rows = append(rows, row)
		}
	}
	return &sdk.CallToolResult{
		Content: []sdk.Content{&sdk.TextContent{
			Text: fmt.Sprintf("%d result rows", len(rows))}},
		StructuredContent: map[string]any{"rows": rows},
	}
}

// summarise writes the one line a person or a model reads first.
func summarise(op *registry.Operation, value any) string {
	obj, ok := value.(map[string]any)
	if !ok {
		return op.Summary
	}
	var parts []string
	for _, key := range []string{"code", "id", "name", "status", "valid",
		"taskId", "statusUrl"} {
		if v, ok := obj[key]; ok && v != nil {
			parts = append(parts, fmt.Sprintf("%s %v", key, v))
		}
	}
	if len(parts) == 0 {
		return op.Summary
	}
	return op.Summary + ": " + strings.Join(parts, ", ")
}

// failure reports an error inside the result, so the model sees
// the code, the message and the hint.
func failure(err error) *sdk.CallToolResult {
	return &sdk.CallToolResult{
		IsError: true,
		Content: []sdk.Content{&sdk.TextContent{Text: err.Error()}},
	}
}

func stringArg(v any) string {
	switch value := v.(type) {
	case nil:
		return ""
	case string:
		return value
	case float64:
		if value == float64(int64(value)) {
			return fmt.Sprintf("%d", int64(value))
		}
		return fmt.Sprintf("%v", value)
	case bool:
		return fmt.Sprintf("%t", value)
	default:
		data, _ := json.Marshal(value)
		return string(data)
	}
}

func stringList(v any) []string {
	list, _ := v.([]any)
	var out []string
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// addDiscovery registers search_operations and call_api, the two
// tools that reach every operation without a curated tool.
func (s *Server) addDiscovery() {
	s.Server.AddTool(&sdk.Tool{
		Name: "search_operations",
		Description: "Find API operations by words in their id, summary, " +
			"description or path. Each match carries the operationId, the " +
			"method and path, the parameters, the permission and an example " +
			"body for call_api. Permission: none. Data tier: public. " +
			"Example: search_operations {\"query\": \"export\"}",
		InputSchema: map[string]any{"type": "object",
			"properties": map[string]any{"query": map[string]any{"type": "string",
				"description": "Words to look for"}},
			"required": []string{"query"}},
		Annotations: &sdk.ToolAnnotations{Title: "Search operations",
			ReadOnlyHint: true},
	}, func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult,
		error) {
		args, err := arguments(req)
		if err != nil {
			return failure(err), nil
		}
		matches := s.search(stringArg(args["query"]))
		return &sdk.CallToolResult{
			Content: []sdk.Content{&sdk.TextContent{
				Text: fmt.Sprintf("%d operations match", len(matches))}},
			StructuredContent: map[string]any{"operations": matches},
		}, nil
	})

	description := "Execute any API operation by operationId, with path " +
		"and query parameters and a body. Operations that cannot be undone " +
		"need confirm equal to \"<operationId> <path value>\". "
	if s.opts.ReadOnly {
		description += "Read-only mode: only reads and validations are " +
			"allowed. "
	}
	description += "Permission: the operation's, see search_operations. " +
		"Data tier: authorised. Example: call_api {\"operationId\": " +
		"\"get_dpp_stats\", \"path\": {\"id\": \"A1B2C3D4E\"}}"
	s.Server.AddTool(&sdk.Tool{
		Name:        "call_api",
		Description: description,
		InputSchema: map[string]any{"type": "object",
			"properties": map[string]any{
				"operationId": map[string]any{"type": "string"},
				"path": map[string]any{"type": "object",
					"description": "Path parameters"},
				"query": map[string]any{"type": "object",
					"description": "Query parameters"},
				"body":    map[string]any{"description": "Request body"},
				"confirm": map[string]any{"type": "string"},
			},
			"required": []string{"operationId"}},
		Annotations: &sdk.ToolAnnotations{Title: "Call the API",
			ReadOnlyHint: s.opts.ReadOnly},
	}, func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult,
		error) {
		args, err := arguments(req)
		if err != nil {
			return failure(err), nil
		}
		return s.callAPI(ctx, args), nil
	})
}

// Match is one answer of search_operations.
type Match struct {
	OperationID string           `json:"operationId"`
	Method      string           `json:"method"`
	Path        string           `json:"path"`
	Summary     string           `json:"summary"`
	Permission  []string         `json:"permission"`
	Destructive bool             `json:"destructive,omitempty"`
	PathParams  []registry.Param `json:"pathParams,omitempty"`
	QueryParams []registry.Param `json:"queryParams,omitempty"`
	Example     json.RawMessage  `json:"example,omitempty"`
	Tool        string           `json:"tool,omitempty"`
}

// search scores every callable operation by the query words.
func (s *Server) search(query string) []Match {
	words := strings.Fields(strings.ToLower(query))
	type scored struct {
		match Match
		score int
	}
	var results []scored
	for i := range s.reg.Operations {
		op := &s.reg.Operations[i]
		if !callable(op) {
			continue
		}
		haystack := strings.ToLower(op.ID + " " + op.Summary + "" +
			"" + op.Path + " " +
			op.Description + " " + op.Tag)
		score := 0
		for _, word := range words {
			switch {
			case strings.Contains(op.ID, word):
				score += 3
			case strings.Contains(strings.ToLower(op.Summary+op.Path), word):
				score += 2
			case strings.Contains(haystack, word):
				score++
			}
		}
		if score == 0 && len(words) > 0 {
			continue
		}
		results = append(results, scored{score: score, match: Match{
			OperationID: op.ID, Method: op.Method, Path: op.Path,
			Summary: op.Summary, Permission: op.Permission,
			Destructive: op.Destructive, PathParams: op.PathParams,
			QueryParams: op.QueryParams, Example: op.RequestExample,
			Tool: toolFor(op.ID),
		}})
	}
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})
	matches := make([]Match, 0, len(results))
	for _, r := range results {
		matches = append(matches, r.match)
	}
	return matches
}

// callable reports whether a consumer token can call the
// operation, or it is a public read.
func callable(op *registry.Operation) bool {
	if op.UserOnly || op.ID == "exchange_token" {
		return false
	}
	for _, scheme := range op.Security {
		if scheme == "oauth2" {
			return true
		}
	}
	return op.Public && op.Method == http.MethodGet
}

func toolFor(operationID string) string {
	for _, t := range curated {
		if t.Operation == operationID {
			return t.Name
		}
	}
	return ""
}

func (s *Server) callAPI(ctx context.Context,
	args map[string]any) *sdk.CallToolResult {
	id := stringArg(args["operationId"])
	op := s.reg.Find(id)
	if op == nil || !callable(op) {
		return failure(fmt.Errorf("unknown operation %q; use search_operations",
			id))
	}
	if s.opts.ReadOnly && !op.ReadOnly() {
		return failure(fmt.Errorf("%s changes data and the server runs"+
			"read-only", id))
	}
	pathArgs, _ := args["path"].(map[string]any)
	if op.Destructive {
		want := id
		for _, p := range op.PathParams {
			want += " " + stringArg(pathArgs[p.Name])
		}
		if stringArg(args["confirm"]) != want {
			return failure(fmt.Errorf("%s cannot be undone; pass confirm: %q",
				id, want))
		}
	}
	path := op.Path
	for _, p := range op.PathParams {
		value := stringArg(pathArgs[p.Name])
		if value == "" {
			return failure(fmt.Errorf("path.%s is required", p.Name))
		}
		path = strings.ReplaceAll(path, "{"+p.Name+"}", url.PathEscape(value))
	}
	req := &transpareo.Request{Method: op.Method, Path: path,
		Query: url.Values{}}
	if query, ok := args["query"].(map[string]any); ok {
		for key, value := range query {
			req.Query.Set(key, stringArg(value))
		}
	}
	if body, ok := args["body"]; ok && body != nil {
		req.Body = body
		if op.RequestContentType != "" &&
			op.RequestContentType != "multipart/form-data" {
			req.ContentType = op.RequestContentType
		}
		if op.NDJSON || op.RequestContentType == "application/x-ndjson" {
			if rows, ok := body.([]any); ok {
				var lines []string
				for _, row := range rows {
					line, _ := json.Marshal(row)
					lines = append(lines, string(line))
				}
				req.Body = []byte(strings.Join(lines, "\n") + "\n")
			}
		}
	}
	client, err := s.api()
	if err != nil {
		return failure(err)
	}
	resp, err := client.Do(ctx, req)
	if err != nil {
		return failure(err)
	}
	kind := kindGet
	if strings.HasPrefix(op.ID, "list_") {
		kind = kindList
	}
	return s.shape(Tool{Name: id, Kind: kind}, op, resp, nil)
}

// addResources registers the guide, the specification and the
// identity as resources.
func (s *Server) addResources() {
	s.Server.AddResource(&sdk.Resource{
		URI: "transpareo://guide", Name: "guide", MIMEType: "text/markdown",
		Description: "The workspace's API guide: credentials, permissions, " +
			"pagination, idempotency, the passport flows, the event feed, " +
			"webhooks, rate limits and the error format.",
	}, func(ctx context.Context,
		req *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
		client, err := s.api()
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(ctx, &transpareo.Request{Method: http.MethodGet,
			Path: client.Host() + "/apidocs/guide.md", Accept: "text/markdown"})
		if err != nil {
			return nil, err
		}
		return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{{
			URI: req.Params.URI, MIMEType: "text/markdown",
			Text: string(resp.Body)}}}, nil
	})
	s.Server.AddResource(&sdk.Resource{
		URI: "transpareo://openapi", Name: "openapi",
		MIMEType: "application/json",
		Description: "The OpenAPI specification the server was built with, " +
			"version " + spec.Version() + ".",
	}, func(ctx context.Context,
		req *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
		return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{{
			URI: req.Params.URI, MIMEType: "application/json",
			Text: string(spec.JSON)}}}, nil
	})
	s.Server.AddResource(&sdk.Resource{
		URI: "transpareo://me", Name: "me", MIMEType: "application/json",
		Description: "What the credential allows: permissions, scope, " +
			"rate limit and token expiry.",
	}, func(ctx context.Context,
		req *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
		client, err := s.api()
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(ctx, &transpareo.Request{Method: http.MethodGet,
			Path: "/me"})
		if err != nil {
			return nil, err
		}
		return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{{
			URI: req.Params.URI, MIMEType: "application/json",
			Text: string(resp.Body)}}}, nil
	})
}

// Run serves the protocol over standard input and output until
// the client disconnects or ctx ends.
func (s *Server) Run(ctx context.Context) error {
	return s.Server.Run(ctx, &sdk.StdioTransport{})
}
