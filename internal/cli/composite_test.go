package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var testPreview = map[string]any{
	"columns": []map[string]any{
		{"header": "Artikelname", "column": "artikelname",
			"suggestedAction": "map_to_attribute",
			"coreAttribute":   "name", "matchType": "attribute"},
		{"header": "Farbe", "column": "farbe", "suggestedAction": "create_new",
			"typeName": "Farbe", "matchType": "none",
			"sampleValues": []string{"rot"}},
	},
	"coreAttributes": []string{"name", "gtin"},
	"propertyTypes":  []map[string]any{{"id": "9", "name": "Colour"}},
}

// compositeHarness adds the import, export and event endpoints.
func compositeHarness(t *testing.T) *harness {
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
	state := "fresh"
	rowErrors := []map[string]any{}
	mux.HandleFunc("POST /api/imports", func(w http.ResponseWriter,
		r *http.Request) {
		record(r)
		// With auto in the options the platform maps and writes
		// from the upload itself, and the answer is the run.
		if strings.Contains(h.lastBody(),
			"name=\"options[auto]\"\r\n\r\ntrue") {
			state = "completed"
			writeJSON(w, 201, map[string]any{"id": 12, "status": "validating",
				"statusUrl": h.server.URL + "/api/imports/12"})
			return
		}
		state = "fresh"
		writeJSON(w, 201, map[string]any{"id": 12, "status": "fresh",
			"preview":   testPreview,
			"statusUrl": h.server.URL + "/api/imports/12"})
	})
	mux.HandleFunc("GET /api/imports/12", func(w http.ResponseWriter,
		r *http.Request) {
		doc := map[string]any{"id": 12, "status": state, "rowErrors": rowErrors,
			"failedCount": len(rowErrors)}
		if state == "fresh" {
			doc["preview"] = testPreview
		}
		writeJSON(w, 200, doc)
	})
	mux.HandleFunc("PUT /api/imports/12/mappings", func(w http.ResponseWriter,
		r *http.Request) {
		record(r)
		state = "mapped"
		writeJSON(w, 200, map[string]any{"id": 12, "status": "mapped"})
	})
	mux.HandleFunc("POST /api/imports/12/validate", func(w http.ResponseWriter,
		r *http.Request) {
		record(r)
		state = "validated"
		if strings.Contains(r.Header.Get("X-Test"), "fail") {
			rowErrors = []map[string]any{{"row": 2, "code": "validation_name",
				"field": "name", "message": "is required"}}
		}
		writeJSON(w, 202, map[string]any{"id": 12, "status": "validating",
			"statusUrl": h.server.URL + "/api/imports/12"})
	})
	mux.HandleFunc("POST /api/imports/12/execute", func(w http.ResponseWriter,
		r *http.Request) {
		record(r)
		state = "completed"
		writeJSON(w, 202, map[string]any{"id": 12, "status": "importing",
			"statusUrl": h.server.URL + "/api/imports/12"})
	})
	mux.HandleFunc("POST /api/exports", func(w http.ResponseWriter,
		r *http.Request) {
		record(r)
		writeJSON(w, 202, map[string]any{"id": 42, "status": "pending",
			"statusUrl": h.server.URL + "/api/exports/42"})
	})
	mux.HandleFunc("GET /api/exports/42", func(w http.ResponseWriter,
		r *http.Request) {
		writeJSON(w, 200, map[string]any{"id": 42, "status": "completed",
			"filename":    "x.tar.gz",
			"downloadUrl": h.server.URL + "/api/exports/42/download"})
	})
	mux.HandleFunc("GET /api/exports/42/download", func(w http.ResponseWriter,
		r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.Write([]byte("GZ"))
	})
	mux.HandleFunc("GET /api/events", func(w http.ResponseWriter,
		r *http.Request) {
		record(r)
		if r.URL.Query().Get("since") != "e1" {
			writeJSON(w, 200, map[string]any{"events": []map[string]any{
				{"id": "e1", "eventType": "published"}}, "nextCursor": "e1"})
			return
		}
		writeJSON(w, 200, map[string]any{"events": []any{}, "nextCursor": "e1"})
	})
	return h
}

func TestImportsMapExitsFiveOnUnresolvedColumns(t *testing.T) {
	h := compositeHarness(t)
	h.login()
	out, _, code := h.run("imports", "map", "12", "--accept-suggestions")
	if code != 5 {
		t.Fatalf("code = %d, out = %s", code, out)
	}
	var report map[string]any
	json.Unmarshal([]byte(out), &report)
	unresolved, _ := report["unresolved"].([]any)
	if len(unresolved) != 1 ||
		unresolved[0].(map[string]any)["header"] != "Farbe" {
		t.Errorf("report = %s", out)
	}
	out, _, code = h.run("imports", "map", "12", "--accept-suggestions",
		"--map", "Farbe=property:Colour", "--published")
	if code != 0 {
		t.Fatalf("code = %d, out = %s", code, out)
	}
	if body := h.lastBody(); !strings.Contains(body,
		`"farbe":{"action":"use_existing","typeId":"9"}`) ||
		!strings.Contains(body,
			`"artikelname":{"action":"map_to_attribute","coreAttribute":"name"}`) ||
		!strings.Contains(body, `"options":{"published":true}`) {
		t.Errorf("mapping body = %s", body)
	}
	if _, _, code := h.run("imports", "map", "12", "--map",
		"nonsense"); code != 2 {
		t.Errorf("a bad --map must be a usage error, got %d", code)
	}
	if _, _, code := h.run("imports", "map", "12", "--read-only", "--map",
		"Farbe=skip"); code != 4 {
		t.Errorf("--read-only must refuse, got %d", code)
	}
}

func TestImportsRunValidatesAndExecutes(t *testing.T) {
	h := compositeHarness(t)
	h.login()
	file := filepath.Join(t.TempDir(), "catalogue.xlsx")
	os.WriteFile(file, []byte("XLSX"), 0o600)
	out, _, code := h.run("imports", "run", "--file", file, "--type",
		"products",
		"--accept-suggestions")
	if code != 5 {
		t.Fatalf("unresolved columns must exit 5, got %d: %s", code, out)
	}
	out, errOut, code := h.run("imports", "run", "--file", file, "--type",
		"products",
		"--accept-suggestions", "--map", "Farbe=skip", "--execute")
	if code != 0 {
		t.Fatalf("code = %d, out = %s, err = %s", code, out, errOut)
	}
	var imp map[string]any
	json.Unmarshal([]byte(out), &imp)
	if imp["status"] != "completed" {
		t.Errorf("import = %s", out)
	}
	if !strings.Contains(errOut, "validated") {
		t.Errorf("progress = %q", errOut)
	}
	if _, _, code := h.run("imports", "run", "--type", "products"); code != 2 {
		t.Errorf("--file is required, got %d", code)
	}
}

// The flag names what to skip and the API field names what to
// take, so a run started with --skip-backup asks for backup
// false. Without the flag the body carries no backup at all and
// the platform takes one.
func TestImportsRunSkipBackupAsksForNoBackup(t *testing.T) {
	h := compositeHarness(t)
	h.login()
	file := filepath.Join(t.TempDir(), "catalogue.xlsx")
	os.WriteFile(file, []byte("XLSX"), 0o600)
	out, errOut, code := h.run("imports", "run", "--file", file, "--type",
		"products", "--accept-suggestions", "--map", "Farbe=skip",
		"--execute", "--skip-backup")
	if code != 0 {
		t.Fatalf("code = %d, out = %s, err = %s", code, out, errOut)
	}
	var sent string
	h.mu.Lock()
	for i, r := range h.requests {
		if strings.HasSuffix(r.URL.Path, "/execute") {
			sent = h.bodies[i]
		}
	}
	h.mu.Unlock()
	if sent != `{"options":{"backup":false}}` {
		t.Errorf("execute body = %s", sent)
	}
}

// --auto writes the rows from the upload, so the command sends
// the option, skips the mapping and execute calls, and is a write
// that --read-only refuses.
func TestImportsRunAuto(t *testing.T) {
	h := compositeHarness(t)
	h.login()
	mux := h.server.Config.Handler.(*http.ServeMux)
	mux.HandleFunc("POST /api/imports/12/mappings", func(w http.ResponseWriter,
		r *http.Request) {
		t.Error("an automatic run sent a mapping")
	})
	file := filepath.Join(t.TempDir(), "catalogue.xlsx")
	os.WriteFile(file, []byte("XLSX"), 0o600)
	out, errOut, code := h.run("imports", "run", "--file", file, "--type",
		"products", "--auto")
	if code != 0 {
		t.Fatalf("code = %d, out = %s, err = %s", code, out, errOut)
	}
	var sent string
	h.mu.Lock()
	for i, r := range h.requests {
		if r.URL.Path == "/api/imports" {
			sent = h.bodies[i]
		}
	}
	h.mu.Unlock()
	if !strings.Contains(sent, "name=\"options[auto]\"\r\n\r\ntrue") {
		t.Errorf("the upload carries no options[auto]: %s", sent)
	}
	if _, _, code := h.run("imports", "run", "--file", file, "--type",
		"products", "--auto", "--read-only"); code != 4 {
		t.Errorf("--read-only must refuse an automatic run, got %d", code)
	}
}

func TestImportsRunExitsThreeOnRowErrors(t *testing.T) {
	h := compositeHarness(t)
	h.login()
	file := filepath.Join(t.TempDir(), "catalogue.xlsx")
	os.WriteFile(file, []byte("XLSX"), 0o600)
	out, _, code := h.runWith(func(app *App) {
		app.HTTPClient = &http.Client{Transport: headerTransport{name: "X-Test",
			value: "fail",
			next:  h.server.Client().Transport}}
	}, "imports", "run", "--file", file, "--type", "products",
		"--map", "Artikelname=name", "--map", "Farbe=skip", "--execute")
	if code != 3 {
		t.Fatalf("row errors must exit 3, got %d: %s", code, out)
	}
	var imp map[string]any
	json.Unmarshal([]byte(out), &imp)
	if rows, _ := imp["rowErrors"].([]any); len(rows) != 1 {
		t.Errorf("row errors = %s", out)
	}
	for _, r := range h.requests {
		if strings.HasSuffix(r.URL.Path, "/execute") {
			t.Error("a failed validation must not execute")
		}
	}
}

// headerTransport adds a header to every request, for the fake
// host to branch on.
type headerTransport struct {
	name, value string
	next        http.RoundTripper
}

func (t headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set(t.name, t.value)
	return t.next.RoundTrip(r)
}

func TestExportsCreateWaitsAndDownloads(t *testing.T) {
	h := compositeHarness(t)
	h.login()
	target := filepath.Join(t.TempDir(), "catalogue.tar.gz")
	out, errOut, code := h.run("exports", "create", "--format", "csv",
		"--normalize",
		"--download", target)
	if code != 0 {
		t.Fatalf("code = %d, out = %s, err = %s", code, out, errOut)
	}
	body := h.bodies[len(h.bodies)-1]
	if body != `{"export":{"format":"csv","normalize":true}}` {
		t.Errorf("body = %s", body)
	}
	data, _ := os.ReadFile(target)
	if string(data) != "GZ" || !strings.Contains(errOut, "Wrote 2 bytes") {
		t.Errorf("download = %q, stderr = %q", data, errOut)
	}
	var exp map[string]any
	json.Unmarshal([]byte(out), &exp)
	if exp["status"] != "completed" {
		t.Errorf("export = %s", out)
	}
	if _, _, code := h.run("exports", "create", "--read-only"); code != 4 {
		t.Errorf("--read-only must refuse, got %d", code)
	}
}

func TestEventsTail(t *testing.T) {
	h := compositeHarness(t)
	h.login()
	out, errOut, code := h.run("events", "tail", "--since", "start", "--types",
		"published")
	if code != 0 {
		t.Fatalf("code = %d: %s", code, errOut)
	}
	var feed map[string]any
	json.Unmarshal([]byte(out), &feed)
	if feed["nextCursor"] != "e1" || len(feed["events"].([]any)) != 1 {
		t.Errorf("feed = %s", out)
	}
	if q := h.lastRequest().URL.RawQuery; q != "since=start&types=published" {
		t.Errorf("query = %q", q)
	}
	out, errOut, code = h.run("events", "tail", "--jsonl")
	if code != 0 ||
		!strings.HasPrefix(out, `{"eventType":"published","id":"e1"}`) ||
		!strings.Contains(errOut, "cursor: e1") {
		t.Errorf("jsonl: out %q err %q", out, errOut)
	}
}

func TestCompositeCommandsReplaceGeneratedOnes(t *testing.T) {
	h := compositeHarness(t)
	out, _, _ := h.run("commands", "--json")
	var cat Catalogue
	json.Unmarshal([]byte(out), &cat)
	seen := map[string]int{}
	for _, c := range cat.Commands {
		seen[c.Path]++
	}
	for _, path := range []string{"transpareo imports map",
		"transpareo imports run",
		"transpareo exports create", "transpareo events tail"} {
		if seen[path] != 1 {
			t.Errorf("%s appears %d times", path, seen[path])
		}
	}
	if seen["transpareo imports create"] != 1 ||
		seen["transpareo events list"] != 1 {
		t.Error("the generated imports create and events list must stay")
	}
}
