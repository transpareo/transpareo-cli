package mcp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/transpareo/transpareo-cli/internal/registry"
	"github.com/transpareo/transpareo-cli/pkg/transpareo"
)

type fakeHost struct {
	*httptest.Server
	mu       sync.Mutex
	requests []*http.Request
	bodies   []string
}

func newFakeHost(t *testing.T) *fakeHost {
	t.Helper()
	h := &fakeHost{}
	mux := http.NewServeMux()
	record := func(r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		h.mu.Lock()
		h.requests = append(h.requests, r)
		h.bodies = append(h.bodies, string(body))
		h.mu.Unlock()
	}
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("POST /api/oauth/token", func(w http.ResponseWriter,
		r *http.Request) {
		writeJSON(w, 200, map[string]any{"access_token": "tok",
			"token_type": "Bearer",
			"expires_in": 3600})
	})
	mux.HandleFunc("GET /api/me", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		writeJSON(w, 200, map[string]any{"name": "ERP", "status": "active",
			"permissions": []string{"dpp_read"}})
	})
	mux.HandleFunc("GET /api/products", func(w http.ResponseWriter,
		r *http.Request) {
		record(r)
		w.Header().Set("API-Total", "3")
		w.Header().Set("API-Page", "1")
		w.Header().Set("Link", `<`+h.URL+`/api/products?page=2>; rel="next"`)
		writeJSON(w, 200, map[string]any{"products": []map[string]any{
			{"id": 1, "name": "Cream", "gtin": "4006381333931"},
			{"id": 2, "name": "Soap", "gtin": "4006381333932"}}})
	})
	mux.HandleFunc("GET /api/dpps/{id}", func(w http.ResponseWriter,
		r *http.Request) {
		record(r)
		writeJSON(w, 200, map[string]any{"code": r.PathValue("id"),
			"status":      "draft",
			"description": "Cream"})
	})
	mux.HandleFunc("POST /api/dpps/validate", func(w http.ResponseWriter,
		r *http.Request) {
		record(r)
		writeJSON(w, 200, map[string]any{"valid": true})
	})
	mux.HandleFunc("POST /api/dpps/{id}/void", func(w http.ResponseWriter,
		r *http.Request) {
		record(r)
		writeJSON(w, 200, map[string]any{"code": r.PathValue("id"),
			"status": "voided"})
	})
	mux.HandleFunc("POST /api/dpps/bulk/validate", func(w http.ResponseWriter,
		r *http.Request) {
		record(r)
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Write([]byte("{\"row\":1,\"status\":\"ok\"}\n" +
			"{\"row\":2,\"status\":\"error\"}\n"))
	})
	mux.HandleFunc("GET /api/dpps/{id}/stats", func(w http.ResponseWriter,
		r *http.Request) {
		record(r)
		writeJSON(w, 200, map[string]any{"scans": 7})
	})
	mux.HandleFunc("POST /api/products", func(w http.ResponseWriter,
		r *http.Request) {
		record(r)
		writeJSON(w, 422, map[string]any{"error": "PRODUCT_INVALID",
			"message": "Product invalid", "hint": "Fix the fields.",
			"fields": map[string]any{
				"brand": map[string]any{"fullMessage": "Brand is required"},
				"product.componentsInput": map[string]any{
					"message": "missing"}}})
	})
	mux.HandleFunc("GET /api/dpps/missing", func(w http.ResponseWriter,
		r *http.Request) {
		writeJSON(w, 404, map[string]any{"error": "DPP_NOT_FOUND",
			"message": "no",
			"hint":    "Check the code."})
	})
	mux.HandleFunc("GET /apidocs/guide.md", func(w http.ResponseWriter,
		r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		w.Write([]byte("# API guide\n"))
	})
	h.Server = httptest.NewServer(mux)
	t.Cleanup(h.Close)
	return h
}

func (h *fakeHost) last() (*http.Request, string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.requests[len(h.requests)-1], h.bodies[len(h.bodies)-1]
}

// connect starts a server with the options and returns a client
// session talking to it in memory.
func connect(t *testing.T, host *fakeHost, opts Options) *sdk.ClientSession {
	t.Helper()
	opts.Client = func() (*transpareo.Client, error) {
		return transpareo.New(host.URL, transpareo.ClientCredentials{ID: "id",
			Secret: "s"},
			transpareo.WithHTTPClient(host.Client()))
	}
	opts.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	server := New(opts)
	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"},
		nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func call(t *testing.T, session *sdk.ClientSession, name string,
	args map[string]any) *sdk.CallToolResult {
	t.Helper()
	result, err := session.CallTool(context.Background(), &sdk.CallToolParams{
		Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return result
}

func text(result *sdk.CallToolResult) string {
	var parts []string
	for _, c := range result.Content {
		if tc, ok := c.(*sdk.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func structured(t *testing.T, result *sdk.CallToolResult) map[string]any {
	t.Helper()
	data, _ := json.Marshal(result.StructuredContent)
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("structured content: %v: %s", err, data)
	}
	return out
}

func toolNames(t *testing.T, session *sdk.ClientSession) map[string]*sdk.Tool {
	t.Helper()
	tools := map[string]*sdk.Tool{}
	for tool, err := range session.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		tools[tool.Name] = tool
	}
	return tools
}

func TestInstructionsAndToolList(t *testing.T) {
	host := newFakeHost(t)
	session := connect(t, host, Options{Version: "test"})
	if !strings.Contains(session.InitializeResult().Instructions,
		"validate_dpp before create_dpp") {
		t.Errorf("instructions = %q", session.InitializeResult().Instructions)
	}
	tools := toolNames(t, session)
	for _, want := range []string{"me", "search_operations", "call_api",
		"list_products",
		"product_property_types", "dpp_requirements", "validate_dpp",
		"create_dpp",
		"publish_dpp", "void_dpp", "bulk_create_dpps", "create_webhook",
		"delete_webhook"} {
		if _, ok := tools[want]; !ok {
			t.Errorf("tool %s is missing", want)
		}
	}
	void := tools["void_dpp"]
	if !strings.Contains(void.Description, "Permission: dpp_lifecycle") ||
		!strings.Contains(void.Description, `confirm must be "void <id>"`) ||
		!strings.Contains(void.Description, "Data tier") ||
		!strings.Contains(void.Description, "Example: void_dpp {") {
		t.Errorf("void description = %q", void.Description)
	}
	if void.Annotations == nil || !*void.Annotations.DestructiveHint ||
		void.Annotations.ReadOnlyHint {
		t.Errorf("void annotations = %+v", void.Annotations)
	}
	if !tools["list_products"].Annotations.ReadOnlyHint ||
		!tools["validate_dpp"].Annotations.ReadOnlyHint {
		t.Error("reads and validations must be read-only")
	}
	if !tools["update_product"].Annotations.IdempotentHint {
		t.Error("a PUT must be idempotent")
	}
	schema, _ := json.Marshal(tools["void_dpp"].InputSchema)
	if !strings.Contains(string(schema), `"confirm"`) ||
		!strings.Contains(string(schema), `"reason"`) {
		t.Errorf("void input schema = %s", schema)
	}
	schema, _ = json.Marshal(tools["list_products"].OutputSchema)
	if !strings.Contains(string(schema), `"nextPage"`) {
		t.Errorf("list output schema = %s", schema)
	}
}

func TestMeAndList(t *testing.T) {
	host := newFakeHost(t)
	session := connect(t, host, Options{})
	result := call(t, session, "me", nil)
	if result.IsError || !strings.Contains(text(result), "name ERP") {
		t.Errorf("me = %q (error %v)", text(result), result.IsError)
	}
	if structured(t, result)["status"] != "active" {
		t.Errorf("me structured = %v", structured(t, result))
	}

	result = call(t, session, "list_products", map[string]any{"per_page": 2,
		"term":   "cream",
		"fields": []string{"name"}})
	req, _ := host.last()
	if req.URL.RawQuery != "per_page=2&term=cream" {
		t.Errorf("query = %q", req.URL.RawQuery)
	}
	page := structured(t, result)
	items, _ := page["items"].([]any)
	if len(items) != 2 || page["total"] != 3.0 || page["nextPage"] != 2.0 ||
		page["page"] != 1.0 {
		t.Errorf("page = %v", page)
	}
	if first, _ := items[0].(map[string]any); len(first) != 1 ||
		first["name"] != "Cream" {
		t.Errorf("fields projection = %v", items[0])
	}
	if !strings.Contains(text(result),
		"2 items on page 1 of 3 in total; continue with page 2") {
		t.Errorf("summary = %q", text(result))
	}
}

func TestWriteToolsAndConfirm(t *testing.T) {
	host := newFakeHost(t)
	session := connect(t, host, Options{})
	result := call(t, session, "validate_dpp", map[string]any{
		"dpp": map[string]any{"unlockableId": 5, "granularity": "serial"}})
	if result.IsError {
		t.Fatalf("validate: %s", text(result))
	}
	_, body := host.last()
	if body != `{"dpp":{"granularity":"serial","unlockableId":5}}` {
		t.Errorf("body = %s", body)
	}

	result = call(t, session, "void_dpp", map[string]any{"id": "A1B2",
		"reason": "recalled"})
	if !result.IsError ||
		!strings.Contains(text(result), `confirm: "void A1B2"`) {
		t.Errorf("void without confirm = %q error %v", text(result),
			result.IsError)
	}
	result = call(t, session, "void_dpp", map[string]any{"id": "A1B2",
		"reason":  "recalled",
		"confirm": "void A1B2"})
	if result.IsError {
		t.Fatalf("void: %s", text(result))
	}
	req, body := host.last()
	if req.URL.Path != "/api/dpps/A1B2/void" ||
		body != `{"reason":"recalled"}` {
		t.Errorf("void request = %s %s", req.URL.Path, body)
	}
	if req.Header.Get("Idempotency-Key") == "" {
		t.Error("a POST must carry an Idempotency-Key")
	}
	if !strings.Contains(text(result), "status voided") {
		t.Errorf("summary = %q", text(result))
	}
}

func TestRowsTool(t *testing.T) {
	host := newFakeHost(t)
	session := connect(t, host, Options{})
	result := call(t, session, "bulk_validate_dpps", map[string]any{
		"shared": map[string]any{"granularity": "model"},
		"rows": []any{map[string]any{"modelIdentifier": "A"},
			map[string]any{"modelIdentifier": "B"}},
	})
	if result.IsError {
		t.Fatalf("bulk validate: %s", text(result))
	}
	req, body := host.last()
	if req.Header.Get("Content-Type") != "application/x-ndjson" ||
		body != "{\"shared\":{\"granularity\":\"model\"}}\n"+
			"{\"modelIdentifier\":\"A\"}\n{\"modelIdentifier\":\"B\"}\n" {
		t.Errorf("request = %q %q", req.Header.Get("Content-Type"), body)
	}
	rows, _ := structured(t, result)["rows"].([]any)
	if len(rows) != 2 || !strings.Contains(text(result), "2 result rows") {
		t.Errorf("rows = %v, text %q", rows, text(result))
	}
	result = call(t, session, "bulk_validate_dpps",
		map[string]any{"rows": []any{}})
	if !result.IsError {
		t.Error("empty rows must be refused")
	}
}

func TestErrorsCarryCodeAndHint(t *testing.T) {
	host := newFakeHost(t)
	session := connect(t, host, Options{})
	result := call(t, session, "get_dpp", map[string]any{"id": "missing"})
	if !result.IsError || text(result) != "DPP_NOT_FOUND: no\nCheck the code." {
		t.Errorf("error = %q (%v)", text(result), result.IsError)
	}
	if structured(t, result)["error"] != "DPP_NOT_FOUND" {
		t.Errorf("structured error = %v", structured(t, result))
	}
	result = call(t, session, "create_product", map[string]any{
		"product": map[string]any{"name": "x"}})
	want := "PRODUCT_INVALID: Product invalid\nFix the fields.\n" +
		"  brand: Brand is required\n  product.componentsInput: missing"
	if !result.IsError || text(result) != want {
		t.Errorf("validation error = %q", text(result))
	}
	result = call(t, session, "get_dpp", map[string]any{})
	if !result.IsError || !strings.Contains(text(result), "id is required") {
		t.Errorf("missing id = %q", text(result))
	}
}

func TestSearchAndCallAPI(t *testing.T) {
	host := newFakeHost(t)
	session := connect(t, host, Options{})
	result := call(t, session, "search_operations",
		map[string]any{"query": "dpp stats"})
	ops, _ := structured(t, result)["operations"].([]any)
	if len(ops) == 0 {
		t.Fatal("no matches")
	}
	first, _ := ops[0].(map[string]any)
	if first["operationId"] != "get_dpp_stats" {
		t.Errorf("first match = %v", first)
	}
	for _, op := range ops {
		if op.(map[string]any)["operationId"] == "login_session" {
			t.Error("user-only operations must not be listed")
		}
	}

	result = call(t, session, "call_api",
		map[string]any{"operationId": "get_dpp_stats",
			"path":  map[string]any{"id": "A1B2"},
			"query": map[string]any{"format": "json"}})
	if result.IsError {
		t.Fatalf("call_api: %s", text(result))
	}
	req, _ := host.last()
	if req.URL.Path != "/api/dpps/A1B2/stats" ||
		req.URL.RawQuery != "format=json" {
		t.Errorf("call_api request = %s", req.URL)
	}
	if structured(t, result)["scans"] != 7.0 {
		t.Errorf("structured = %v", structured(t, result))
	}

	result = call(t, session, "call_api",
		map[string]any{"operationId": "void_dpp",
			"path": map[string]any{"id": "A1B2"},
			"body": map[string]any{"reason": "other"}})
	if !result.IsError || !strings.Contains(text(result), `"void_dpp A1B2"`) {
		t.Errorf("destructive call_api without confirm = %q", text(result))
	}
	result = call(t, session, "call_api",
		map[string]any{"operationId": "void_dpp",
			"path":    map[string]any{"id": "A1B2"},
			"body":    map[string]any{"reason": "other"},
			"confirm": "void_dpp A1B2"})
	if result.IsError {
		t.Errorf("confirmed call_api failed: %s", text(result))
	}
	result = call(t, session, "call_api",
		map[string]any{"operationId": "login_session"})
	if !result.IsError {
		t.Error("a user-only operation must be refused")
	}
}

func TestReadOnlyAndGroups(t *testing.T) {
	host := newFakeHost(t)
	session := connect(t, host, Options{ReadOnly: true,
		Groups: []string{GroupDpps}})
	tools := toolNames(t, session)
	for name := range tools {
		if strings.HasPrefix(name, "create_") || name == "void_dpp" ||
			name == "publish_dpp" {
			t.Errorf("%s must not be served read-only", name)
		}
	}
	for _, want := range []string{"me", "search_operations", "call_api",
		"list_dpps",
		"validate_dpp", "bulk_validate_dpps", "dpp_requirements"} {
		if _, ok := tools[want]; !ok {
			t.Errorf("%s missing in read-only dpps mode", want)
		}
	}
	if _, ok := tools["list_products"]; ok {
		t.Error("--tools dpps must not serve product tools")
	}
	result := call(t, session, "call_api",
		map[string]any{"operationId": "create_dpp",
			"body": map[string]any{}})
	if !result.IsError || !strings.Contains(text(result), "read-only") {
		t.Errorf("call_api write in read-only = %q", text(result))
	}
	result = call(t, session, "call_api",
		map[string]any{"operationId": "validate_dpp",
			"body": map[string]any{"dpp": map[string]any{}}})
	if result.IsError {
		t.Errorf("validation must pass in read-only: %s", text(result))
	}
}

func TestResources(t *testing.T) {
	host := newFakeHost(t)
	session := connect(t, host, Options{})
	uris := map[string]bool{}
	for res, err := range session.Resources(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		uris[res.URI] = true
	}
	for _, want := range []string{"transpareo://guide", "transpareo://openapi",
		"transpareo://me"} {
		if !uris[want] {
			t.Errorf("resource %s missing", want)
		}
	}
	read := func(uri string) string {
		result, err := session.ReadResource(context.Background(),
			&sdk.ReadResourceParams{URI: uri})
		if err != nil {
			t.Fatalf("%s: %v", uri, err)
		}
		return result.Contents[0].Text
	}
	if read("transpareo://guide") != "# API guide\n" {
		t.Error("guide not served")
	}
	if !strings.Contains(read("transpareo://openapi"), `"openapi"`) {
		t.Error("openapi not served")
	}
	if !strings.Contains(read("transpareo://me"), `"ERP"`) {
		t.Error("me not served")
	}
}

// TestEveryOperationIsClassified keeps the curated set complete:
// each operation is behind a tool, listed in viaCallAPIOnly with
// a reason, hidden from consumers, or user-only.
func TestEveryOperationIsClassified(t *testing.T) {
	reg := registry.Default()
	covered := map[string]bool{}
	for _, tool := range curated {
		op := reg.Find(tool.Operation)
		if op == nil {
			t.Errorf("tool %s names unknown operation %s", tool.Name,
				tool.Operation)
			continue
		}
		if covered[tool.Operation] {
			t.Errorf("operation %s has two tools", tool.Operation)
		}
		covered[tool.Operation] = true
		if tool.Confirm == "" && op.Destructive {
			t.Errorf("tool %s wraps a destructive operation without confirm",
				tool.Name)
		}
		if tool.Confirm != "" && !op.Destructive {
			t.Errorf("tool %s demands confirm for a reversible operation",
				tool.Name)
		}
	}
	for id, reason := range viaCallAPIOnly {
		if reg.Find(id) == nil {
			t.Errorf("viaCallAPIOnly names unknown operation %s", id)
		}
		if covered[id] {
			t.Errorf("%s has a tool and is listed viaCallAPIOnly", id)
		}
		if strings.TrimSpace(reason) == "" {
			t.Errorf("%s has no reason", id)
		}
	}
	for _, op := range reg.Operations {
		switch {
		case covered[op.ID], op.UserOnly:
		case !callable(&op):
			// A storefront flow no consumer token can use.
		case viaCallAPIOnly[op.ID] != "":
		default:
			t.Errorf("%s is neither a tool nor classified in viaCallAPIOnly",
				op.ID)
		}
	}
}

func TestDataTools(t *testing.T) {
	host := newFakeHost(t)
	mux := host.Config.Handler.(*http.ServeMux)
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(v)
	}
	polls := 0
	mux.HandleFunc("GET /api/exports/42", func(w http.ResponseWriter,
		r *http.Request) {
		polls++
		status := "running"
		if polls > 1 {
			status = "completed"
		}
		writeJSON(w, 200, map[string]any{"id": 42, "status": status,
			"progress":    50 * polls,
			"filename":    "x.tar.gz",
			"downloadUrl": host.URL + "/api/exports/42/download"})
	})
	mux.HandleFunc("POST /api/exports", func(w http.ResponseWriter,
		r *http.Request) {
		polls = 0
		writeJSON(w, 202, map[string]any{"id": 42, "status": "pending",
			"statusUrl": host.URL + "/api/exports/42"})
	})
	mux.HandleFunc("GET /api/events", func(w http.ResponseWriter,
		r *http.Request) {
		writeJSON(w, 200,
			map[string]any{"events": []map[string]any{{"id": "e1"}},
				"nextCursor": "e1"})
	})
	mux.HandleFunc("POST /api/imports", func(w http.ResponseWriter,
		r *http.Request) {
		r.ParseMultipartForm(1 << 20)
		writeJSON(w, 201, map[string]any{"id": 12, "status": "fresh",
			"preview": map[string]any{"columns": []map[string]any{
				{"header": "Farbe", "column": "farbe",
					"suggestedAction": "create_new",
					"typeName":        "Farbe", "matchType": "none"}},
				"coreAttributes": []string{"name"}}})
	})
	session := connect(t, host, Options{})
	tools := toolNames(t, session)
	for _, want := range []string{"wait_for_task", "tail_events",
		"export_catalogue",
		"import_spreadsheet"} {
		if _, ok := tools[want]; !ok {
			t.Errorf("%s missing", want)
		}
	}

	result := call(t, session, "wait_for_task",
		map[string]any{"statusUrl": host.URL + "/api/exports/42"})
	if result.IsError || structured(t, result)["status"] != "completed" {
		t.Errorf("wait_for_task = %q %v", text(result), structured(t, result))
	}
	result = call(t, session, "export_catalogue",
		map[string]any{"format": "csv"})
	if result.IsError ||
		!strings.Contains(text(result), "export 42 completed") {
		t.Errorf("export_catalogue = %q", text(result))
	}
	result = call(t, session, "tail_events", map[string]any{"since": "x"})
	if result.IsError || structured(t, result)["nextCursor"] != "e1" {
		t.Errorf("tail_events = %v", structured(t, result))
	}
	file := t.TempDir() + "/c.xlsx"
	os.WriteFile(file, []byte("X"), 0o600)
	result = call(t, session, "import_spreadsheet", map[string]any{"path": file,
		"dataType": "products", "acceptSuggestions": true})
	if !result.IsError ||
		!strings.Contains(text(result), "1 columns need a mapping") {
		t.Errorf("import_spreadsheet = %q", text(result))
	}
	unresolved, _ := structured(t, result)["unresolved"].([]any)
	if len(unresolved) != 1 {
		t.Errorf("unresolved = %v", structured(t, result))
	}

	readOnly := connect(t, host, Options{ReadOnly: true})
	tools = toolNames(t, readOnly)
	if _, ok := tools["export_catalogue"]; ok {
		t.Error("export_catalogue must not be served read-only")
	}
	if _, ok := tools["tail_events"]; !ok {
		t.Error("tail_events must stay in read-only mode")
	}
	limited := connect(t, host, Options{Groups: []string{GroupDpps}})
	if _, ok := toolNames(t, limited)["wait_for_task"]; ok {
		t.Error("--tools dpps must not serve the data tools")
	}
}

// TestWriteToolsHaveAJSONBody keeps a write tool on an operation
// whose body it can actually send. A tool's arguments travel as
// JSON, so an operation declaring only a file attachment has no
// tool until the document carries the bytes inline as well.
func TestWriteToolsHaveAJSONBody(t *testing.T) {
	reg := registry.Default()
	for _, tool := range curated {
		if tool.Kind != kindWrite {
			continue
		}
		op := reg.Find(tool.Operation)
		if op == nil || len(op.RequestBodies) == 0 {
			continue
		}
		if op.JSONBody() == nil {
			t.Errorf("%s writes %s, which declares only %s", tool.Name, op.ID,
				op.DefaultBody().ContentType)
		}
	}
}

// TestSearchReportsEveryBody keeps the bodies an operation
// accepts visible to an assistant. call_api sends JSON, so one
// declaring only a file attachment must say so and carry the
// prose that names what to send instead.
func TestSearchReportsEveryBody(t *testing.T) {
	s := New(Options{Version: "test"})
	reg := registry.Default()
	seen := 0
	for _, m := range s.search("create product mediafile import") {
		op := reg.Find(m.OperationID)
		if len(op.RequestBodies) == 0 {
			continue
		}
		seen++
		if len(m.ContentTypes) != len(op.RequestBodies) {
			t.Errorf("%s reports %v of %d bodies", m.OperationID,
				m.ContentTypes, len(op.RequestBodies))
		}
		if op.JSONBody() != nil && m.Description != "" {
			t.Errorf("%s takes JSON and needs no prose", m.OperationID)
		}
		if op.JSONBody() == nil && m.Description == "" {
			t.Errorf("%s takes no body call_api can send and carries no prose",
				m.OperationID)
		}
	}
	if seen == 0 {
		t.Fatal("the search matched no operation taking a body")
	}
}

// TestCreateMediafileWaitsForItsBody fails when the vendored
// document gains an upload body a tool can send. The reason the
// operation is reachable through call_api alone is spent then,
// and it becomes a curated tool the assistant can call.
func TestCreateMediafileWaitsForItsBody(t *testing.T) {
	op := registry.Default().Find("create_mediafile")
	if op == nil {
		t.Skip("the document no longer declares create_mediafile")
	}
	if op.JSONBody() != nil {
		t.Error("create_mediafile now takes a JSON body: give it a curated " +
			"tool and drop it from viaCallAPIOnly")
	}
}
