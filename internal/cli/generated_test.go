package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/transpareo/transpareo-cli/internal/registry"
)

// generatedHarness extends the fake host with the endpoints the
// generated commands under test call.
func generatedHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	mux := h.server.Config.Handler.(*http.ServeMux)
	record := func(r *http.Request) {
		body, _ := readAll(r)
		h.mu.Lock()
		h.requests = append(h.requests, r)
		h.bodies = append(h.bodies, body)
		h.mu.Unlock()
	}
	mux.HandleFunc("GET /api/dpps/{code}", func(w http.ResponseWriter,
		r *http.Request) {
		record(r)
		writeJSON(w, 200, map[string]any{"code": r.PathValue("code"),
			"status": "draft"})
	})
	mux.HandleFunc("GET /api/exports/{id}/download", func(w http.ResponseWriter,
		r *http.Request) {
		record(r)
		w.Header().Set("Content-Type", "application/gzip")
		w.Write([]byte("GZ-BYTES"))
	})
	mux.HandleFunc("POST /api/dpps/{code}/void", func(w http.ResponseWriter,
		r *http.Request) {
		record(r)
		writeJSON(w, 200, map[string]any{"code": r.PathValue("code"),
			"status": "voided"})
	})
	mux.HandleFunc("POST /api/dpps/bulk", func(w http.ResponseWriter,
		r *http.Request) {
		record(r)
		if r.Header.Get("Prefer") == "respond-async" {
			writeJSON(w, 202, map[string]any{"taskId": "t1",
				"statusUrl": h.server.URL + "/api/dpps/bulk/t1", "rowCount": 2})
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Write([]byte("{\"row\":1}\n{\"row\":2}\n"))
	})
	var polls int
	mux.HandleFunc("GET /api/dpps/bulk/t1", func(w http.ResponseWriter,
		r *http.Request) {
		record(r)
		polls++
		status := "running"
		if polls > 1 {
			status = "completed"
		}
		writeJSON(w, 200, map[string]any{"taskId": "t1", "status": status,
			"progress": polls * 50, "createdCount": 2})
	})
	mux.HandleFunc("POST /api/imports", func(w http.ResponseWriter,
		r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			writeJSON(w, 400, map[string]string{"error": "BAD",
				"message": err.Error()})
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": "NO_FILE",
				"message": err.Error()})
			return
		}
		content, _ := readAllReader(file)
		writeJSON(w, 201, map[string]any{"id": 12, "status": "fresh",
			"filename": header.Filename, "content": content,
			"dataType": r.FormValue("dataType"),
			"mapping":  r.FormValue("mappings[name][action]")})
	})
	mux.HandleFunc("GET /api/brands", func(w http.ResponseWriter,
		r *http.Request) {
		record(r)
		writeJSON(w, 200, map[string]any{"brands": []map[string]any{
			{"id": "b1", "name": "Alpha"}, {"id": "b2", "name": "Beta"}}})
	})
	return h
}

func readAllReader(r interface{ Read([]byte) (int, error) }) (string, error) {
	var out []byte
	buf := make([]byte, 1024)
	for {
		n, err := r.Read(buf)
		out = append(out, buf[:n]...)
		if err != nil {
			if err.Error() == "EOF" {
				return string(out), nil
			}
			return string(out), err
		}
	}
}

func TestGeneratedListUnwrapsForTablesAndIds(t *testing.T) {
	h := generatedHarness(t)
	h.login()
	out, _, code := h.run("brands", "list", "--per-page", "2", "--page", "3")
	if code != 0 {
		t.Fatalf("out = %s", out)
	}
	if q := h.lastRequest().URL.RawQuery; q != "page=3&per_page=2" {
		t.Errorf("query = %q", q)
	}
	var wrapped map[string]any
	if err := json.Unmarshal([]byte(out), &wrapped); err != nil ||
		wrapped["brands"] == nil {
		t.Errorf("--json keeps the wrapper: %s", out)
	}
	out, _, _ = h.run("brands", "list", "-q")
	if out != "b1\nb2\n" {
		t.Errorf("quiet = %q", out)
	}
	out, _, _ = h.run("brands", "list", "--jsonl")
	if lines := strings.Split(strings.TrimSpace(out), "\n"); len(lines) != 2 {
		t.Errorf("jsonl = %q", out)
	}
	h.terminal = true
	out, _, _ = h.run("brands", "list")
	if !strings.HasPrefix(out, "id  name\nb1  Alpha\n") {
		t.Errorf("table = %q", out)
	}
	out, _, _ = h.run("brands", "list", "--fields", "name")
	if !strings.HasPrefix(out, "name\nAlpha\n") {
		t.Errorf("fields = %q", out)
	}
}

func TestGeneratedGetWithPathParamAndBinaryOutput(t *testing.T) {
	h := generatedHarness(t)
	h.login()
	out, _, code := h.run("dpps", "get", "A1B2", "--fields", "code")
	if code != 0 || strings.TrimSpace(out) != `{"code":"A1B2"}` &&
		!strings.Contains(out, `"code": "A1B2"`) {
		t.Errorf("code = %d, out = %q", code, out)
	}
	if h.lastRequest().URL.Path != "/api/dpps/A1B2" {
		t.Errorf("path = %q", h.lastRequest().URL.Path)
	}
	file := filepath.Join(t.TempDir(), "export.tar.gz")
	_, _, code = h.run("exports", "download", "42", "--output", file)
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	data, _ := os.ReadFile(file)
	if string(data) != "GZ-BYTES" {
		t.Errorf("file = %q", data)
	}
	if h.lastRequest().Header.Get("Accept") != "application/gzip" {
		t.Errorf("accept = %q", h.lastRequest().Header.Get("Accept"))
	}
}

func TestGeneratedDestructiveNeedsYesAndSetBuildsBody(t *testing.T) {
	h := generatedHarness(t)
	h.login()
	out, _, code := h.run("dpps", "void", "A1B2", "--set", "reason=recalled")
	if code != 4 || !strings.Contains(out, "CONFIRMATION_REQUIRED") {
		t.Fatalf("code = %d, out = %s", code, out)
	}
	_, _, code = h.run("dpps", "void", "A1B2", "--yes", "--set",
		"reason=recalled",
		"--set", "meta.count=3", "--set", "meta.flag=true", "--set",
		"description=Batch 9")
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	var body map[string]any
	json.Unmarshal([]byte(h.lastBody()), &body)
	meta, _ := body["meta"].(map[string]any)
	if body["reason"] != "recalled" || body["description"] != "Batch 9" ||
		meta["count"] != 3.0 || meta["flag"] != true {
		t.Errorf("body = %s", h.lastBody())
	}
	if _, _, code := h.run("dpps", "void", "A1B2", "--yes"); code != 2 {
		t.Errorf("a required body must be a usage error, got %d", code)
	}
	if _, _, code := h.run("dpps", "void", "A1B2", "--yes", "--read-only",
		"--set", "reason=recalled"); code != 4 {
		t.Errorf("--read-only must refuse, got %d", code)
	}
	if _, _, code := h.run("dpps", "void", "--yes"); code != 2 {
		t.Errorf("a missing path argument must be a usage error, got %d", code)
	}
}

func TestGeneratedFileOptionSendsTheFileContent(t *testing.T) {
	h := generatedHarness(t)
	h.login()
	file := filepath.Join(t.TempDir(), "void.json")
	os.WriteFile(file, []byte("{\n  \"reason\": \"recalled\"\n}\n"), 0o600)
	_, _, code := h.run("dpps", "void", "A1B2", "--yes", "--file", file)
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	if h.lastBody() != "{\n  \"reason\": \"recalled\"\n}\n" {
		t.Errorf("body = %q, want the file's content", h.lastBody())
	}
	if h.lastRequest().Header.Get("Content-Type") != "application/json" {
		t.Errorf("content type = %q", h.lastRequest().Header.Get("Content-Type"))
	}
	_, _, code = h.run("dpps", "void", "A1B2", "--yes", "--file", "/nope.json")
	if code != 2 {
		t.Errorf("a missing file must be a usage error, got %d", code)
	}
}

func TestGeneratedNDJSONBodyHeaderAndWait(t *testing.T) {
	h := generatedHarness(t)
	h.login()
	h.stdin = "{\"modelIdentifier\":\"A\"}\n{\"modelIdentifier\":\"B\"}\n"
	out, _, code := h.run("dpps", "bulk", "create", "--file", "-")
	if code != 0 {
		t.Fatalf("code = %d, out = %s", code, out)
	}
	req := h.lastRequest()
	if req.Header.Get("Content-Type") != "application/x-ndjson" ||
		!strings.Contains(h.lastBody(), "\"B\"") {
		t.Errorf("content type %q body %q", req.Header.Get("Content-Type"),
			h.lastBody())
	}
	if out != "{\"row\":1}\n{\"row\":2}\n" {
		t.Errorf("ndjson answer = %q", out)
	}

	h.stdin = "{\"modelIdentifier\":\"A\"}\n"
	out, errOut, code := h.run("dpps", "bulk", "create", "--file", "-",
		"--prefer", "respond-async", "--wait")
	if code != 0 {
		t.Fatalf("code = %d, out = %s, err = %s", code, out, errOut)
	}
	var task map[string]any
	json.Unmarshal([]byte(out), &task)
	if task["status"] != "completed" || task["createdCount"] != 2.0 {
		t.Errorf("task = %s", out)
	}
	if !strings.Contains(errOut, "running 50%") ||
		!strings.Contains(errOut, "completed 100%") {
		t.Errorf("progress = %q", errOut)
	}
}

func TestGeneratedMultipartUpload(t *testing.T) {
	h := generatedHarness(t)
	h.login()
	file := filepath.Join(t.TempDir(), "catalogue.xlsx")
	os.WriteFile(file, []byte("XLSX"), 0o600)
	out, errOut, code := h.run("imports", "create", "--file", file,
		"--data-type", "products",
		"--mappings", `{"name": {"action": "map_to_attribute"}}`)
	if code != 0 {
		t.Fatalf("code = %d, out = %s, err = %s", code, out, errOut)
	}
	var answer map[string]any
	json.Unmarshal([]byte(out), &answer)
	if answer["filename"] != "catalogue.xlsx" || answer["content"] != "XLSX" ||
		answer["dataType"] != "products" ||
		answer["mapping"] != "map_to_attribute" {
		t.Errorf("answer = %s", out)
	}
	if _, _, code := h.run("imports", "create", "--file",
		"/nope/missing.xlsx"); code != 2 {
		t.Errorf("a missing upload must be a usage error, got %d", code)
	}
}

func TestGeneratedHelpCarriesExampleAndPermission(t *testing.T) {
	h := generatedHarness(t)
	out, _, code := h.run("dpps", "void", "--help")
	if code != 0 {
		t.Fatal(out)
	}
	for _, want := range []string{"Permission: dpp_lifecycle",
		"cannot be undone",
		"transpareo dpps void <id> --file body.json --yes",
		"Operation: void_dpp"} {
		if !strings.Contains(out, want) {
			t.Errorf("help lacks %q:\n%s", want, out)
		}
	}
	out, _, _ = h.run("brands", "list", "--help")
	if !strings.Contains(out, "the endpoint is public") ||
		!strings.Contains(out, "--per-page") {
		t.Errorf("help = %s", out)
	}
}

func TestEveryGeneratedCommandHasAnExampleAndOperation(t *testing.T) {
	h := generatedHarness(t)
	out, _, _ := h.run("commands", "--json")
	var cat Catalogue
	json.Unmarshal([]byte(out), &cat)
	generated := 0
	for _, c := range cat.Commands {
		if strings.HasPrefix(c.Path, "transpareo completion") {
			continue
		}
		if c.Example == "" {
			t.Errorf("%s has no example", c.Path)
		}
		if c.OperationID != "" && !strings.HasPrefix(c.Path, "transpareo auth") &&
			c.Path != "transpareo me" {
			generated++
		}
	}
	if generated < 80 {
		t.Errorf("only %d generated commands", generated)
	}
}

func TestTasksWait(t *testing.T) {
	h := generatedHarness(t)
	h.login()
	out, errOut, code := h.run("tasks", "wait", h.server.URL+"/api/dpps/bulk/t1")
	if code != 0 {
		t.Fatalf("code = %d: %s%s", code, out, errOut)
	}
	var task map[string]any
	json.Unmarshal([]byte(out), &task)
	if task["status"] != "completed" || !strings.Contains(errOut, "running 50%") {
		t.Errorf("task = %s, progress = %q", out, errOut)
	}
	_, _, code = h.run("tasks", "wait", "https://other.example.com/api/x")
	if code != 1 {
		t.Errorf("another host must be refused with 1, got %d", code)
	}
}

// TestCommandBindsTheAttachmentBody keeps the options that take
// a path on an operation accepting a file as an attachment and
// the same bytes inline. A document adding a JSON body beside
// the multipart one must not cost the command its --file and
// --name, nor turn --file into the request body itself.
func TestCommandBindsTheAttachmentBody(t *testing.T) {
	op := &registry.Operation{ID: "upload_thing_image", Group: "things",
		Tag: "Things", Method: "POST", Path: "/things/images",
		RequestBodies: []registry.RequestBody{
			{ContentType: "application/json", Schema: json.RawMessage(
				`{"type":"object","properties":{"image":{"type":"object"}}}`)},
			{ContentType: "multipart/form-data", Schema: json.RawMessage(
				`{"type":"object","properties":{` +
					`"image[file]":{"type":"string","format":"binary"},` +
					`"image[name]":{"type":"string"}}}`)},
		}}
	body := commandBody(op)
	if body == nil || body.ContentType != "multipart/form-data" {
		t.Fatalf("command body = %+v", body)
	}
	app := &App{Getenv: func(string) string { return "" }}
	cmd := app.operationCommand(op, CommandWords(op))
	for _, name := range []string{"file", "name"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("the command lost the option --%s", name)
		}
	}
	if cmd.Flags().Lookup("set") != nil {
		t.Error("--set belongs to a JSON body, not to an attachment one")
	}
	if !strings.Contains(cmd.Example, "--file <path>") {
		t.Errorf("the example must hand over a path: %q", cmd.Example)
	}

	// The assistant side of the same operation reads the other body.
	if json := op.JSONBody(); json == nil ||
		!strings.Contains(string(json.Schema), `"image"`) {
		t.Errorf("json body = %+v", json)
	}
}
