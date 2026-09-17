package flows

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/transpareo/transpareo-cli/pkg/transpareo"
)

type host struct {
	*httptest.Server
	mux      *http.ServeMux
	mu       sync.Mutex
	requests []*http.Request
	bodies   []string
}

func newHost(t *testing.T) (*host, *transpareo.Client) {
	t.Helper()
	h := &host{mux: http.NewServeMux()}
	h.mux.HandleFunc("POST /api/oauth/token", func(w http.ResponseWriter,
		r *http.Request) {
		writeJSON(w, 200, map[string]any{"access_token": "tok",
			"token_type": "Bearer",
			"expires_in": 3600})
	})
	h.Server = httptest.NewServer(h.mux)
	t.Cleanup(h.Close)
	c, err := transpareo.New(h.URL, transpareo.ClientCredentials{ID: "i",
		Secret: "s"},
		transpareo.WithHTTPClient(h.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return h, c
}

func (h *host) record(r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	h.mu.Lock()
	h.requests = append(h.requests, r)
	h.bodies = append(h.bodies, string(body))
	h.mu.Unlock()
}

func (h *host) last() (*http.Request, string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.requests[len(h.requests)-1], h.bodies[len(h.bodies)-1]
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

var preview = Preview{
	Columns: []Column{
		{Header: "Artikelname", Column: "artikelname",
			SuggestedAction: "map_to_attribute",
			CoreAttribute:   "name", MatchType: "attribute"},
		{Header: "Gewicht", Column: "gewicht", SuggestedAction: "use_existing",
			TypeID: "6650", TypeName: "Weight", MatchType: "exact"},
		{Header: "Farbe", Column: "farbe", SuggestedAction: "create_new",
			TypeName: "Farbe", MatchType: "none",
			SampleValues: []string{"rot"}},
		{Header: "Intern", Column: "intern", SuggestedAction: "use_existing",
			TypeID: "7", TypeName: "Internal", MatchType: "fuzzy",
			Similarity: 0.6},
	},
	CoreAttributes:     []string{"name", "gtin"},
	RequiredAttributes: []string{"name"},
	PropertyTypes: []PropertyType{{ID: "6650", Name: "Weight"}, {ID: "9",
		Name: "Colour"}},
}

func TestParseMapSpec(t *testing.T) {
	cases := map[string]Mapping{
		"Artikelname=name": {Action: "map_to_attribute",
			CoreAttribute: "name"},
		"Gewicht=property:Weight": {Action: "use_existing",
			TypeName: "Weight"},
		"Farbe=new:Colour":         {Action: "create_new", TypeName: "Colour"},
		"Intern=skip":              {Action: "skip"},
		" Spaced = property:Width": {Action: "use_existing", TypeName: "Width"},
	}
	for spec, want := range cases {
		column, got, err := ParseMapSpec(spec)
		if err != nil || got != want || column == "" {
			t.Errorf("%q: %q %+v %v", spec, column, got, err)
		}
	}
	for _, bad := range []string{"novalue", "=x", "a="} {
		if _, _, err := ParseMapSpec(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}

func TestResolveMappingsAcceptsSafeSuggestionsOnly(t *testing.T) {
	imp := &Import{Status: "fresh", Preview: &preview}
	resolved, err := ResolveMappings(imp, nil, true)
	var required *ErrMappingRequired
	if !errors.As(err, &required) {
		t.Fatalf("err = %v", err)
	}
	if resolved["artikelname"].CoreAttribute != "name" ||
		resolved["gewicht"].TypeID != "6650" {
		t.Errorf("resolved = %+v", resolved)
	}
	if len(required.Unresolved) != 2 ||
		required.Unresolved[0].Header != "Farbe" ||
		required.Unresolved[1].MatchType != "fuzzy" {
		t.Errorf("unresolved = %+v", required.Unresolved)
	}
	if required.Unresolved[0].Suggestion.Action != "create_new" {
		t.Error("the suggestion is reported, not applied")
	}
	if !strings.Contains(err.Error(), "Farbe, Intern") {
		t.Errorf("message = %q", err)
	}

	explicit := map[string]Mapping{
		"Farbe":  {Action: "use_existing", TypeName: "Colour"},
		"intern": {Action: "skip"},
	}
	resolved, err = ResolveMappings(imp, explicit, true)
	if err != nil {
		t.Fatal(err)
	}
	if resolved["farbe"].TypeID != "9" || resolved["farbe"].TypeName != "" ||
		resolved["intern"].Action != "skip" || len(resolved) != 4 {
		t.Errorf("resolved = %+v", resolved)
	}

	_, err = ResolveMappings(imp, map[string]Mapping{"Nope": {Action: "skip"}},
		false)
	if err == nil || !strings.Contains(err.Error(), "no column") {
		t.Errorf("unknown column: %v", err)
	}
	_, err = ResolveMappings(imp,
		map[string]Mapping{"Farbe": {Action: "use_existing",
			TypeName: "Nothing"}}, false)
	if err == nil || !strings.Contains(err.Error(), "no property type") {
		t.Errorf("unknown type: %v", err)
	}
	if _, err := ResolveMappings(imp, nil, false); !IsMappingRequired(err) {
		t.Error("without suggestions every column is unresolved")
	}
}

func TestRunImportFlow(t *testing.T) {
	h, c := newHost(t)
	var polls int
	h.mux.HandleFunc("POST /api/imports", func(w http.ResponseWriter,
		r *http.Request) {
		r.ParseMultipartForm(1 << 20)
		_, header, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": "NO_FILE",
				"message": err.Error()})
			return
		}
		h.record(r)
		writeJSON(w, 201, map[string]any{"id": 12, "status": "fresh",
			"dataType":         r.FormValue("dataType"),
			"originalFilename": header.Filename,
			"preview":          preview, "statusUrl": h.URL + "/api/imports/12"})
	})
	h.mux.HandleFunc("PUT /api/imports/12/mappings", func(w http.ResponseWriter,
		r *http.Request) {
		h.record(r)
		writeJSON(w, 200, map[string]any{"id": 12, "status": "mapped"})
	})
	h.mux.HandleFunc("POST /api/imports/12/validate",
		func(w http.ResponseWriter, r *http.Request) {
			h.record(r)
			writeJSON(w, 202, map[string]any{"id": 12, "status": "validating",
				"statusUrl": h.URL + "/api/imports/12"})
		})
	h.mux.HandleFunc("GET /api/imports/12", func(w http.ResponseWriter,
		r *http.Request) {
		polls++
		status := "validating"
		if polls >= 2 {
			status = "validated"
		}
		writeJSON(w, 200, map[string]any{"id": 12, "status": status,
			"progress":  polls * 50,
			"rowErrors": []any{}, "failedCount": 0, "totalEntries": 240})
	})
	h.mux.HandleFunc("POST /api/imports/12/execute", func(w http.ResponseWriter,
		r *http.Request) {
		h.record(r)
		polls = 1
		writeJSON(w, 202, map[string]any{"id": 12, "status": "importing",
			"statusUrl": h.URL + "/api/imports/12/done"})
	})
	h.mux.HandleFunc("GET /api/imports/12/done", func(w http.ResponseWriter,
		r *http.Request) {
		writeJSON(w, 200, map[string]any{"id": 12, "status": "completed",
			"createdCount": 240})
	})
	file := filepath.Join(t.TempDir(), "catalogue.xlsx")
	os.WriteFile(file, []byte("XLSX"), 0o600)
	fast := func(*transpareo.Task) {}
	_ = fast
	ctx := context.Background()

	_, err := Run(ctx, c, RunOptions{Path: file, DataType: "products",
		AcceptSuggestions: true})
	if !IsMappingRequired(err) {
		t.Fatalf("expected a mapping request, got %v", err)
	}

	published := true
	imp, err := Run(ctx, c, RunOptions{Path: file, DataType: "products",
		AcceptSuggestions: true, Execute: true,
		Mappings: map[string]Mapping{"Farbe": {Action: "use_existing",
			TypeName: "Colour"},
			"Intern": {Action: "skip"}},
		Options: &MappingOptions{Published: &published}})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if imp.Status != "completed" {
		t.Errorf("status = %s", imp.Status)
	}
	var sentMapping, sentExecute string
	h.mu.Lock()
	for i, r := range h.requests {
		if r.Method == "PUT" {
			sentMapping = h.bodies[i]
		}
		if strings.HasSuffix(r.URL.Path, "/execute") {
			sentExecute = h.bodies[i]
		}
	}
	h.mu.Unlock()
	if !strings.Contains(sentMapping,
		`"farbe":{"action":"use_existing","typeId":"9"}`) ||
		!strings.Contains(sentMapping, `"options":{"published":true}`) {
		t.Errorf("mapping body = %s", sentMapping)
	}
	if sentExecute != `{"options":{"published":true}}` {
		t.Errorf("execute body = %s", sentExecute)
	}
}

// A sheet a caller has already read goes up as a JSON file,
// because the extension is what picks the reader on the other
// side.
func TestRunUploadsRowsAsAJSONFile(t *testing.T) {
	h, c := newHost(t)
	var sent string
	h.mux.HandleFunc("POST /api/imports", func(w http.ResponseWriter,
		r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		sent = string(body)
		writeJSON(w, 201, map[string]any{"id": 12, "status": "mapped"})
	})
	h.mux.HandleFunc("POST /api/imports/12/validate",
		func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, 202, map[string]any{"id": 12, "status": "validating",
				"statusUrl": h.URL + "/api/imports/12"})
		})
	h.mux.HandleFunc("GET /api/imports/12", func(w http.ResponseWriter,
		r *http.Request) {
		writeJSON(w, 200, map[string]any{"id": 12, "status": "validated",
			"rowErrors": []any{}, "failedCount": 0})
	})
	rows := []map[string]any{{"Name": "Aqua", "Origin": "Germany"}}
	imp, err := Run(context.Background(), c, RunOptions{Rows: rows,
		DataType: "products"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if imp.Status != "validated" {
		t.Errorf("status = %s", imp.Status)
	}
	if !strings.Contains(sent, `filename="rows.json"`) {
		t.Errorf("upload = %s", sent)
	}
	if !strings.Contains(sent, `[{"Name":"Aqua","Origin":"Germany"}]`) {
		t.Errorf("upload = %s", sent)
	}
	if !strings.Contains(sent, "products") {
		t.Errorf("upload carries no dataType: %s", sent)
	}
}

func TestValidateReportsRowErrors(t *testing.T) {
	h, c := newHost(t)
	h.mux.HandleFunc("POST /api/imports/5/validate", func(w http.ResponseWriter,
		r *http.Request) {
		writeJSON(w, 202, map[string]any{"id": 5, "status": "validating"})
	})
	h.mux.HandleFunc("GET /api/imports/5", func(w http.ResponseWriter,
		r *http.Request) {
		writeJSON(w, 200, map[string]any{"id": 5, "status": "validated",
			"failedCount": 1,
			"rowErrors": []map[string]any{{"row": 3, "code": "validation_name",
				"field": "name", "message": "is required"}}})
	})
	imp, err := Validate(context.Background(), c, "5", nil)
	var failed *ErrValidationFailed
	if !errors.As(err, &failed) || imp == nil ||
		len(failed.Import.RowErrors) != 1 {
		t.Fatalf("err = %v imp = %+v", err, imp)
	}
	if failed.Import.RowErrors[0].Row != 3 {
		t.Errorf("row error = %+v", failed.Import.RowErrors[0])
	}
}

func TestExportStartWaitAndDownload(t *testing.T) {
	h, c := newHost(t)
	var polls int
	h.mux.HandleFunc("POST /api/exports", func(w http.ResponseWriter,
		r *http.Request) {
		h.record(r)
		writeJSON(w, 202, map[string]any{"id": 42, "status": "pending",
			"statusUrl": h.URL + "/api/exports/42"})
	})
	h.mux.HandleFunc("GET /api/exports/42", func(w http.ResponseWriter,
		r *http.Request) {
		polls++
		if polls < 2 {
			writeJSON(w, 200, map[string]any{"id": 42, "status": "running",
				"progress": 40})
			return
		}
		writeJSON(w, 200, map[string]any{"id": 42, "status": "completed",
			"filename":    "export.tar.gz",
			"downloadUrl": h.URL + "/api/exports/42/download"})
	})
	h.mux.HandleFunc("GET /api/exports/42/download", func(w http.ResponseWriter,
		r *http.Request) {
		h.record(r)
		w.Header().Set("Content-Type", "application/gzip")
		w.Write([]byte("GZIP-BYTES"))
	})
	normalize := true
	exp, err := StartExport(context.Background(), c,
		ExportOptions{Format: "csv",
			Normalize: &normalize, Wait: true})
	if err != nil {
		t.Fatal(err)
	}
	_, body := h.requests[0], h.bodies[0]
	if body != `{"export":{"format":"csv","normalize":true}}` {
		t.Errorf("body = %s", body)
	}
	if exp.Status != "completed" || exp.Filename != "export.tar.gz" ||
		exp.Summary() != "export 42 completed (export.tar.gz)" {
		t.Errorf("export = %+v", exp)
	}
	var out strings.Builder
	n, err := Download(context.Background(), c, exp, &out)
	if err != nil || n != 10 || out.String() != "GZIP-BYTES" {
		t.Errorf("download = %d %q %v", n, out.String(), err)
	}
	req, _ := h.last()
	if req.Header.Get("Authorization") != "Bearer tok" {
		t.Error("the download must carry the token")
	}
}

func TestExportFailure(t *testing.T) {
	h, c := newHost(t)
	h.mux.HandleFunc("POST /api/exports", func(w http.ResponseWriter,
		r *http.Request) {
		writeJSON(w, 202, map[string]any{"id": 43, "status": "pending"})
	})
	h.mux.HandleFunc("GET /api/exports/43", func(w http.ResponseWriter,
		r *http.Request) {
		writeJSON(w, 200, map[string]any{"id": 43, "status": "failed",
			"failure": "disk full"})
	})
	_, err := StartExport(context.Background(), c, ExportOptions{Wait: true})
	if !transpareo.IsCode(err, "EXPORT_FAILED") ||
		!strings.Contains(err.Error(), "disk full") {
		t.Errorf("err = %v", err)
	}
}

func TestFollowFeedAdvancesTheCursor(t *testing.T) {
	h, c := newHost(t)
	var calls int
	h.mux.HandleFunc("GET /api/events", func(w http.ResponseWriter,
		r *http.Request) {
		h.record(r)
		calls++
		switch calls {
		case 1:
			writeJSON(w, 200,
				map[string]any{"events": []map[string]any{{"id": "e1"},
					{"id": "e2"}}, "nextCursor": "e2"})
		case 2:
			writeJSON(w, 200, map[string]any{"events": []any{},
				"nextCursor": "e2"})
		default:
			writeJSON(w, 200,
				map[string]any{"events": []map[string]any{{"id": "e3"}},
					"nextCursor": "e3"})
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	var waits []time.Duration
	sleep := func(ctx context.Context, d time.Duration) error {
		waits = append(waits, d)
		if len(waits) == 3 {
			cancel()
		}
		return ctx.Err()
	}
	var seen []string
	err := Follow(ctx, c, FeedOptions{Since: "start", Types: "published",
		Limit: 50},
		5*time.Second, sleep, func(feed *Feed) error {
			for _, ev := range feed.Events {
				seen = append(seen, string(ev))
			}
			return nil
		})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if strings.Join(seen, ",") != `{"id":"e1"},{"id":"e2"},{"id":"e3"}` {
		t.Errorf("seen = %v", seen)
	}
	req, _ := h.last()
	if req.URL.Query().Get("since") != "e2" ||
		req.URL.Query().Get("types") != "published" ||
		req.URL.Query().Get("limit") != "50" {
		t.Errorf("last query = %s", req.URL.RawQuery)
	}
	if len(waits) != 3 || waits[0] != 5*time.Second {
		t.Errorf("waits = %v", waits)
	}
}
