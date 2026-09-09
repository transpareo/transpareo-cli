package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/transpareo/transpareo-cli/internal/registry"
)

// TestContractEveryGeneratedCommandAgainstTheExamples runs each
// generated command against a server that answers the response
// example of its operation, and checks that the command sends the
// right method and path and prints the example back. Operations
// without an example are counted and reported.
func TestContractEveryGeneratedCommandAgainstTheExamples(t *testing.T) {
	reg := registry.Default()
	var served *registry.Operation
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		if r.URL.Path == "/api/oauth/token" {
			writeJSON(w, 200, map[string]any{"access_token": "tok",
				"token_type": "Bearer", "expires_in": 3600})
			return
		}
		op := reg.Match(r.Method, r.URL.Path)
		if op == nil || served == nil || op.ID != served.ID {
			writeJSON(w, 404, map[string]string{"error": "NOT_FOUND",
				"message": r.Method + " " + r.URL.Path})
			return
		}
		status := 200
		if n, err := parseStatus(op.ResponseStatus); err == nil {
			status = n
		}
		w.Header().Set("Content-Type", op.ResponseContentType)
		w.WriteHeader(status)
		w.Write(op.ResponseExample)
	}))
	defer server.Close()

	dir := t.TempDir()
	upload := filepath.Join(dir, "upload.bin")
	os.WriteFile(upload, []byte("bytes"), 0o600)
	skipped := 0
	for _, op := range exposedOperations(reg) {
		if len(op.ResponseExample) == 0 {
			skipped++
			continue
		}
		served = op
		args := contractArgs(t, op, dir, upload)
		stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
		env := map[string]string{"TRANSPAREO_HOST": server.URL,
			"TRANSPAREO_CLIENT_ID": "id", "TRANSPAREO_CLIENT_SECRET": "s",
			"TRANSPAREO_CONFIG_DIR": dir}
		app := &App{Stdin: strings.NewReader(""), Stdout: stdout,
			Stderr:    stderr,
			Getenv:    func(key string) string { return env[key] },
			ConfigDir: dir, WorkDir: dir, HTTPClient: server.Client()}
		if code := Main(t.Context(), app, args); code != 0 {
			t.Errorf("%s (%v): exit %d\n%s%s", op.ID, args, code, stdout,
				stderr)
			continue
		}
		if strings.Contains(op.ResponseContentType, "json") {
			var want, got any
			json.Unmarshal(op.ResponseExample, &want)
			if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
				t.Errorf("%s: output is not JSON: %s", op.ID, stdout)
				continue
			}
			wantJSON, _ := json.Marshal(want)
			gotJSON, _ := json.Marshal(got)
			if string(wantJSON) != string(gotJSON) {
				t.Errorf("%s: printed %s, want %s", op.ID, gotJSON, wantJSON)
			}
		}
	}
	t.Logf("%d exposed operations have no response example and were skipped",
		skipped)
}

func parseStatus(s string) (int, error) {
	return strconv.Atoi(s)
}

// contractArgs builds a command line for an operation from its
// registry entry: placeholder path arguments, the request
// example as the body, and --yes for a destructive one.
func contractArgs(t *testing.T, op *registry.Operation, dir,
	upload string) []string {
	t.Helper()
	args := CommandWords(op)
	for range op.PathParams {
		args = append(args, "x")
	}
	switch {
	case op.RequestContentType == "multipart/form-data":
		for _, field := range multipartFields(op) {
			if field.binary {
				args = append(args, "--"+field.flag, upload)
			}
		}
	case op.RequestContentType != "":
		body := op.RequestExample
		if len(body) == 0 {
			body = []byte("{}")
		}
		if op.RequestContentType == "application/x-ndjson" {
			var compact bytes.Buffer
			json.Compact(&compact, body)
			body = append(compact.Bytes(), '\n')
		}
		path := filepath.Join(dir, op.ID+".body")
		os.WriteFile(path, body, 0o600)
		args = append(args, "--file", path)
	}
	if op.Destructive {
		args = append(args, "--yes")
	}
	return append(args, "--json")
}
