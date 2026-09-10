package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/transpareo/transpareo-cli/internal/auth"
	"github.com/transpareo/transpareo-cli/spec"
)

// harness is a fake workspace host plus the pieces every run of
// the command line needs.
type harness struct {
	t         *testing.T
	server    *httptest.Server
	configDir string
	stores    *auth.Stores
	env       map[string]string
	stdin     string
	terminal  bool

	mu       sync.Mutex
	requests []*http.Request
	bodies   []string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{t: t, configDir: t.TempDir(), env: map[string]string{}}
	mem := &auth.MemoryStore{}
	h.stores = &auth.Stores{Dir: h.configDir, Keyring: mem,
		File:             auth.NewFileStore(h.configDir),
		KeyringAvailable: func() bool { return true }}
	h.server = httptest.NewServer(h.handler())
	t.Cleanup(h.server.Close)
	return h
}

func (h *harness) handler() http.Handler {
	mux := http.NewServeMux()
	record := func(r *http.Request) {
		body, _ := readAll(r)
		h.mu.Lock()
		h.requests = append(h.requests, r)
		h.bodies = append(h.bodies, body)
		h.mu.Unlock()
	}
	mux.HandleFunc("POST /api/oauth/token", func(w http.ResponseWriter,
		r *http.Request) {
		record(r)
		r.PostForm, _ = url.ParseQuery(h.lastBody())
		if r.PostForm.Get("client_secret") != "s3cret" {
			writeJSON(w, 401, map[string]string{"error": "invalid_client",
				"error_description": "Client authentication failed."})
			return
		}
		writeJSON(w, 200, map[string]any{"access_token": "tok",
			"token_type": "Bearer",
			"expires_in": 3600, "scope": r.PostForm.Get("scope")})
	})
	authed := func(next func(http.ResponseWriter,
		*http.Request)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			record(r)
			if r.Header.Get("Authorization") != "Bearer tok" &&
				r.Header.Get("Authorization") != "Bearer env-token" {
				writeJSON(w, 401, map[string]string{"error": "AUTH_REQUIRED",
					"message": "Consumer credentials required"})
				return
			}
			next(w, r)
		}
	}
	mux.HandleFunc("GET /api/me", authed(func(w http.ResponseWriter,
		r *http.Request) {
		writeJSON(w, 200, map[string]any{"name": "ERP", "key": "k",
			"status":         "active",
			"permissions":    []string{"dpp_read", "dpp_write"},
			"scope":          []string{"dpp_read"},
			"tokenExpiresAt": "2026-09-09T13:00:00Z"})
	}))
	mux.HandleFunc("GET /api/dpps", authed(func(w http.ResponseWriter,
		r *http.Request) {
		w.Header().Set("API-Total", "2")
		writeJSON(w, 200, []map[string]any{
			{"id": "1", "code": "AAA", "name": "First", "status": "draft"},
			{"id": "2", "code": "BBB", "name": "Second", "status": "published"},
		})
	}))
	mux.HandleFunc("POST /api/dpps", authed(func(w http.ResponseWriter,
		r *http.Request) {
		writeJSON(w, 201, map[string]any{"id": "3", "code": "CCC"})
	}))
	mux.HandleFunc("POST /api/dpps/validate", authed(func(w http.ResponseWriter,
		r *http.Request) {
		writeJSON(w, 200, map[string]any{"valid": true})
	}))
	mux.HandleFunc("DELETE /api/webhooks/7", authed(func(w http.ResponseWriter,
		r *http.Request) {
		w.WriteHeader(204)
	}))
	mux.HandleFunc("GET /api/dpps/missing", authed(func(w http.ResponseWriter,
		r *http.Request) {
		writeJSON(w, 404, map[string]any{"error": "DPP_NOT_FOUND",
			"message": "DPP not found",
			"hint":    "Check the code.", "docsUrl": "https://x/guide#w"})
	}))
	mux.HandleFunc("POST /api/grant", authed(func(w http.ResponseWriter,
		r *http.Request) {
		writeJSON(w, 201, map[string]any{"key": "child", "secret": "once",
			"expiresAt": "2026-09-10T12:00:00Z",
			"scope":     map[string]string{"code": "A1B2"}})
	}))
	mux.HandleFunc("GET /apidocs/openapi.json", func(w http.ResponseWriter,
		r *http.Request) {
		record(r)
		w.Header().Set("Content-Type", "application/json")
		w.Write(spec.JSON)
	})
	mux.HandleFunc("GET /apidocs/guide.md", func(w http.ResponseWriter,
		r *http.Request) {
		record(r)
		w.Header().Set("Content-Type", "text/markdown")
		w.Write([]byte("# API guide\n"))
	})
	mux.HandleFunc("GET /.well-known/oauth-authorization-server",
		func(w http.ResponseWriter, r *http.Request) {
			record(r)
			writeJSON(w, 200,
				map[string]any{"token_endpoint": h.server.URL + "/api/oauth/token",
					"grant_types_supported": []string{"client_credentials"}})
		})
	return mux
}

func readAll(r *http.Request) (string, error) {
	if r.Body == nil {
		return "", nil
	}
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r.Body)
	return buf.String(), err
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// run executes one command line and returns stdout, stderr and
// the exit code.
func (h *harness) run(args ...string) (string, string, int) {
	h.t.Helper()
	return h.runWith(nil, args...)
}

// runWith lets a test adjust the App before the run.
func (h *harness) runWith(adjust func(*App), args ...string) (string, string, int) {
	h.t.Helper()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	env := map[string]string{"TRANSPAREO_CONFIG_DIR": h.configDir}
	for k, v := range h.env {
		env[k] = v
	}
	app := &App{
		Stdin:      strings.NewReader(h.stdin),
		Stdout:     stdout,
		Stderr:     stderr,
		Getenv:     func(key string) string { return env[key] },
		Terminal:   h.terminal,
		ConfigDir:  h.configDir,
		WorkDir:    h.t.TempDir(),
		Stores:     h.stores,
		HTTPClient: h.server.Client(),
	}
	if adjust != nil {
		adjust(app)
	}
	code := Main(context.Background(), app, args)
	return stdout.String(), stderr.String(), code
}

func (h *harness) login() {
	h.t.Helper()
	h.stdin = "s3cret\n"
	out, errOut, code := h.run("auth", "login", "--host", h.server.URL,
		"--client-id", "id")
	if code != 0 {
		h.t.Fatalf("login failed (%d): %s %s", code, out, errOut)
	}
	h.stdin = ""
}

func (h *harness) lastRequest() *http.Request {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.requests[len(h.requests)-1]
}

func (h *harness) lastBody() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.bodies[len(h.bodies)-1]
}

// errorMessage is the message of a JSON error envelope, taken
// out of the JSON so a path in it carries the separators of the
// host: Windows writes them as escapes that a comparison
// against filepath.Join would never match. Output that is not
// an envelope is answered as it stands.
func errorMessage(t *testing.T, out string) string {
	t.Helper()
	var envelope struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil ||
		envelope.Error.Message == "" {
		return out
	}
	return envelope.Error.Message
}

// TestErrorMessageUnescapesAPath proves the comparison the
// path assertions rest on: a Windows path reaches the envelope
// with its separators escaped, and comes back out whole.
func TestErrorMessageUnescapesAPath(t *testing.T) {
	envelope := `{"ok":false,"error":{"code":"ERROR","message":` +
		`"open C:\keys\p256.pem: The system cannot find the file ` +
		`specified."}}`
	if got := errorMessage(t, envelope); !strings.Contains(got,
		`C:\keys\p256.pem`) {
		t.Errorf("message = %q", got)
	}
	if got := errorMessage(t, "Error: --platform-key is required"); got !=
		"Error: --platform-key is required" {
		t.Errorf("plain output = %q", got)
	}
}

func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		t.Fatalf("not a JSON object: %v: %q", err, s)
	}
	return out
}

func TestLoginStoresProfileAndMeWorks(t *testing.T) {
	h := newHarness(t)
	h.login()
	cfg, err := auth.LoadConfig(h.configDir)
	if err != nil {
		t.Fatal(err)
	}
	name := profileNameFor(h.server.URL)
	profile, ok := cfg.Profiles[name]
	if !ok || profile.Host != h.server.URL || profile.ClientID != "id" ||
		profile.Store != "memory" {
		t.Fatalf("config = %+v", cfg)
	}
	if cfg.DefaultProfile != name {
		t.Errorf("default = %q", cfg.DefaultProfile)
	}
	data, _ := os.ReadFile(filepath.Join(h.configDir, "config.json"))
	if strings.Contains(string(data), "s3cret") {
		t.Error("the secret must not be in config.json")
	}

	out, errOut, code := h.run("me")
	if code != 0 {
		t.Fatalf("me: %d %s", code, errOut)
	}
	if me := decode(t, out); me["name"] != "ERP" {
		t.Errorf("me = %v", me)
	}
	if h.lastRequest().Header.Get("Accept") != "application/vnd.api.v1+json" {
		t.Error("the Accept header is missing")
	}
	if !strings.HasPrefix(h.lastRequest().Header.Get("User-Agent"),
		"transpareo-cli/") {
		t.Errorf("user agent = %q", h.lastRequest().Header.Get("User-Agent"))
	}
}

func TestLoginWithWrongSecretFails(t *testing.T) {
	h := newHarness(t)
	h.stdin = "wrong\n"
	out, _, code := h.run("auth", "login", "--host", h.server.URL,
		"--client-id", "id")
	if code != 1 {
		t.Fatalf("code = %d", code)
	}
	env := decode(t, out)
	if env["ok"] != false ||
		env["error"].(map[string]any)["code"] != "invalid_client" {
		t.Errorf("envelope = %v", env)
	}
	if cfg, _ := auth.LoadConfig(h.configDir); len(cfg.Profiles) != 0 {
		t.Error("a failed login must not store a profile")
	}
}

func TestLoginNeedsASecret(t *testing.T) {
	h := newHarness(t)
	h.stdin = ""
	out, _, code := h.run("auth", "login", "--host", h.server.URL,
		"--client-id", "id")
	if code != 2 || !strings.Contains(out, "no secret") {
		t.Errorf("code = %d, out = %q", code, out)
	}
}

func TestLoginTakesSecretFromEnvironmentAndNamesProfile(t *testing.T) {
	h := newHarness(t)
	h.env["TRANSPAREO_CLIENT_SECRET"] = "s3cret"
	_, _, code := h.run("auth", "login", "--host", h.server.URL, "--client-id",
		"id",
		"--name", "acme", "--scope", "dpp_read,dpp_write")
	if code != 0 {
		t.Fatal("login failed")
	}
	delete(h.env, "TRANSPAREO_CLIENT_SECRET")
	cfg, _ := auth.LoadConfig(h.configDir)
	if p := cfg.Profiles["acme"]; len(p.Scope) != 2 {
		t.Errorf("profile = %+v", p)
	}
	h.run("me", "--profile", "acme")
	h.mu.Lock()
	var tokenForm string
	for i, r := range h.requests {
		if strings.HasSuffix(r.URL.Path, "/oauth/token") {
			tokenForm = h.bodies[i]
		}
	}
	h.mu.Unlock()
	if !strings.Contains(tokenForm, "scope=dpp_read+dpp_write") {
		t.Errorf("token form = %q", tokenForm)
	}
}

func TestStatusOnTerminalPrintsProfileLine(t *testing.T) {
	h := newHarness(t)
	h.login()
	h.terminal = true
	out, errOut, code := h.run("auth", "status")
	if code != 0 {
		t.Fatal(errOut)
	}
	if !strings.Contains(errOut, "Profile ") ||
		!strings.Contains(errOut, "memory store") {
		t.Errorf("stderr = %q", errOut)
	}
	if !strings.Contains(out, "name:") || strings.HasPrefix(out, "{") {
		t.Errorf("stdout = %q", out)
	}
}

func TestLogoutRemovesProfile(t *testing.T) {
	h := newHarness(t)
	h.login()
	if _, _, code := h.run("auth", "logout"); code != 0 {
		t.Fatal("logout failed")
	}
	cfg, _ := auth.LoadConfig(h.configDir)
	if len(cfg.Profiles) != 0 {
		t.Errorf("profiles = %v", cfg.Profiles)
	}
	out, _, code := h.run("me")
	if code != 1 || !strings.Contains(out, "NO_PROFILE") {
		t.Errorf("code = %d, out = %s", code, out)
	}
}

// exchanges counts the token exchanges the harness has seen.
func (h *harness) exchanges() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, r := range h.requests {
		if r.URL.Path == "/api/oauth/token" {
			n++
		}
	}
	return n
}

// TestStoredProfileReusesItsTokenAcrossRuns proves a stored
// profile exchanges the secret at login and then not again for
// as long as the token lives, that `auth token` mints a fresh one
// regardless, and that logout removes the stored token.
func TestStoredProfileReusesItsTokenAcrossRuns(t *testing.T) {
	h := newHarness(t)
	h.login()
	for range 3 {
		if _, errOut, code := h.run("me"); code != 0 {
			t.Fatalf("me: %d %s", code, errOut)
		}
	}
	if h.exchanges() != 1 {
		t.Errorf("exchanges = %d, want the login's one", h.exchanges())
	}
	if _, _, code := h.run("auth", "token"); code != 0 || h.exchanges() != 2 {
		t.Errorf("auth token: code %d, exchanges %d", code, h.exchanges())
	}
	name := profileNameFor(h.server.URL)
	store := h.stores.TokenStore(name, "memory")
	if token, err := store.Load(); err != nil || token == nil ||
		token.AccessToken != "tok" {
		t.Fatalf("stored token = %v, err = %v", token, err)
	}
	if _, _, code := h.run("auth", "logout"); code != 0 {
		t.Fatal("logout failed")
	}
	if token, _ := store.Load(); token != nil {
		t.Error("logout kept the token")
	}
}

// TestExpiredStoredTokenIsReplaced proves a run that finds a
// token about to expire exchanges once and stores the new one.
func TestExpiredStoredTokenIsReplaced(t *testing.T) {
	h := newHarness(t)
	h.login()
	name := profileNameFor(h.server.URL)
	store := h.stores.TokenStore(name, "memory")
	store.Save(&auth.StoredToken{AccessToken: "tok",
		ExpiresAt: time.Now().Add(30 * time.Second)})
	if _, errOut, code := h.run("me"); code != 0 {
		t.Fatalf("me: %d %s", code, errOut)
	}
	token, _ := store.Load()
	if h.exchanges() != 2 || token == nil ||
		time.Until(token.ExpiresAt) < 59*time.Minute {
		t.Errorf("exchanges %d, stored %+v", h.exchanges(), token)
	}
}

// TestEnvironmentCredentialsStoreNoToken proves a run on
// TRANSPAREO_CLIENT_SECRET exchanges every time and keeps nothing.
func TestEnvironmentCredentialsStoreNoToken(t *testing.T) {
	h := newHarness(t)
	h.env["TRANSPAREO_HOST"] = h.server.URL
	h.env["TRANSPAREO_CLIENT_ID"] = "id"
	h.env["TRANSPAREO_CLIENT_SECRET"] = "s3cret"
	for range 2 {
		if _, errOut, code := h.run("me"); code != 0 {
			t.Fatalf("me: %d %s", code, errOut)
		}
	}
	if h.exchanges() != 2 {
		t.Errorf("exchanges = %d, want one per run", h.exchanges())
	}
	if _, err := h.stores.Keyring.Get("/token"); err == nil {
		t.Error("a token was stored without a profile")
	}
}

func TestTokenPrintsBearerToken(t *testing.T) {
	h := newHarness(t)
	h.login()
	out, _, code := h.run("auth", "token", "--scope", "dpp_read")
	if code != 0 || out != "tok\n" {
		t.Errorf("code = %d, out = %q", code, out)
	}
	if !strings.Contains(h.lastBody(), "scope=dpp_read") {
		t.Errorf("token form = %q", h.lastBody())
	}
	out, _, _ = h.run("auth", "token", "--json")
	if tok := decode(t, out); tok["accessToken"] != "tok" ||
		tok["tokenType"] != "Bearer" {
		t.Errorf("token = %v", tok)
	}
}

func TestEnvironmentTokenIsUsedAsIs(t *testing.T) {
	h := newHarness(t)
	h.env["TRANSPAREO_HOST"] = h.server.URL
	h.env["TRANSPAREO_TOKEN"] = "env-token"
	out, _, code := h.run("me")
	if code != 0 || decode(t, out)["name"] != "ERP" {
		t.Errorf("code = %d, out = %s", code, out)
	}
	if h.lastRequest().Header.Get("Authorization") != "Bearer env-token" {
		t.Error("the environment token was not sent")
	}
	out, _, code = h.run("auth", "token")
	if code != 2 || !strings.Contains(out, "TRANSPAREO_TOKEN") {
		t.Errorf("code = %d, out = %q", code, out)
	}
}

func TestGrant(t *testing.T) {
	h := newHarness(t)
	h.login()
	out, _, code := h.run("auth", "grant", "--code", "A1B2")
	if code != 0 {
		t.Fatalf("out = %s", out)
	}
	if grant := decode(t, out); grant["secret"] != "once" {
		t.Errorf("grant = %v", grant)
	}
	if h.lastBody() != `{"dppCode":"A1B2"}` {
		t.Errorf("body = %q", h.lastBody())
	}
	if _, _, code := h.run("auth", "grant"); code != 2 {
		t.Errorf("a grant without a passport must be a usage error, got %d",
			code)
	}
	if _, _, code := h.run("auth", "grant", "--code", "A1B2",
		"--read-only"); code != 4 {
		t.Errorf("a grant under --read-only must exit 4, got %d", code)
	}
}

func TestAPIGetWithQueryAndFields(t *testing.T) {
	h := newHarness(t)
	h.login()
	out, _, code := h.run("api", "GET", "/dpps", "--query", "page=2", "--query",
		"per_page=50",
		"--fields", "code,status")
	if code != 0 {
		t.Fatalf("out = %s", out)
	}
	if got := h.lastRequest().URL.RawQuery; got != "page=2&per_page=50" {
		t.Errorf("query = %q", got)
	}
	var items []map[string]any
	json.Unmarshal([]byte(out), &items)
	if len(items) != 2 || items[0]["code"] != "AAA" || len(items[0]) != 2 {
		t.Errorf("items = %v", items)
	}

	out, _, _ = h.run("api", "get", "/dpps", "-q")
	if out != "1\n2\n" {
		t.Errorf("quiet = %q", out)
	}
	out, _, _ = h.run("api", "GET", "/dpps", "--jsonl")
	if lines := strings.Split(strings.TrimSpace(out), "\n"); len(lines) != 2 {
		t.Errorf("jsonl = %q", out)
	}

	h.terminal = true
	out, _, _ = h.run("api", "GET", "/dpps")
	if !strings.HasPrefix(out, "id  code  name    status\n") {
		t.Errorf("table = %q", out)
	}
}

func TestAPIPostBodies(t *testing.T) {
	h := newHarness(t)
	h.login()
	file := filepath.Join(t.TempDir(), "body.json")
	os.WriteFile(file, []byte(`{"dpp":{"a":1}}`), 0o600)
	out, _, code := h.run("api", "POST", "/dpps", "--body", "@"+file,
		"--idempotency-key", "create-1")
	if code != 0 {
		t.Fatalf("out = %s", out)
	}
	req := h.lastRequest()
	if h.lastBody() != `{"dpp":{"a":1}}` ||
		req.Header.Get("Idempotency-Key") != "create-1" ||
		req.Header.Get("Content-Type") != "application/json" {
		t.Errorf("body = %q, headers = %v", h.lastBody(), req.Header)
	}

	h.stdin = `{"from":"stdin"}`
	h.run("api", "POST", "/dpps", "--body", "-", "--header", "X-Extra: yes")
	if h.lastBody() != `{"from":"stdin"}` ||
		h.lastRequest().Header.Get("X-Extra") != "yes" {
		t.Errorf("body = %q", h.lastBody())
	}
	h.stdin = ""

	h.run("api", "POST", "/dpps", "--body", `{"inline":true}`)
	if h.lastBody() != `{"inline":true}` {
		t.Errorf("body = %q", h.lastBody())
	}
	if key := h.lastRequest().Header.Get("Idempotency-Key"); len(key) != 36 {
		t.Errorf("a POST without a key must get a UUID, got %q", key)
	}

	_, _, code = h.run("api", "POST", "/dpps", "--body",
		"@/nonexistent/file.json")
	if code != 2 {
		t.Errorf("a missing body file must be a usage error, got %d", code)
	}
}

func TestAPIRefusalsAndReadOnly(t *testing.T) {
	h := newHarness(t)
	h.login()
	out, _, code := h.run("api", "DELETE", "/webhooks/7")
	if code != 4 {
		t.Fatalf("a destructive call without --yes must exit 4, got %d: %s",
			code, out)
	}
	if env := decode(t,
		out); env["error"].(map[string]any)["code"] != "CONFIRMATION_REQUIRED" {
		t.Errorf("envelope = %v", env)
	}
	_, errOut, code := h.run("api", "DELETE", "/webhooks/7", "--yes")
	if code != 0 || errOut != "" {
		t.Errorf("with --yes: code %d, stderr %q", code, errOut)
	}

	if _, _, code := h.run("api", "POST", "/dpps", "--read-only"); code != 4 {
		t.Errorf("a write under --read-only must exit 4, got %d", code)
	}
	if _, _, code := h.run("api", "POST", "/dpps/validate", "--read-only",
		"--body", "{}"); code != 0 {
		t.Errorf("a validation under --read-only must pass, got %d", code)
	}
	if _, _, code := h.run("api", "GET", "/dpps", "--read-only"); code != 0 {
		t.Errorf("a read under --read-only must pass, got %d", code)
	}
	if _, _, code := h.run("api", "BREW", "/dpps"); code != 2 {
		t.Errorf("an unknown method must be a usage error, got %d", code)
	}
	if _, _, code := h.run("api", "GET", "/dpps", "--query",
		"novalue"); code != 2 {
		t.Errorf("a malformed query must be a usage error, got %d", code)
	}
}

func TestAPIErrorEnvelopeAndExitCode(t *testing.T) {
	h := newHarness(t)
	h.login()
	out, _, code := h.run("api", "GET", "/dpps/missing")
	if code != 1 {
		t.Fatalf("code = %d", code)
	}
	env := decode(t, out)
	e := env["error"].(map[string]any)
	if e["code"] != "DPP_NOT_FOUND" || e["hint"] != "Check the code." ||
		e["status"] != 404.0 {
		t.Errorf("envelope = %v", env)
	}

	h.terminal = true
	out, errOut, _ := h.run("api", "GET", "/dpps/missing")
	if out != "" ||
		errOut != "DPP_NOT_FOUND: DPP not found\nCheck the code.\n" {
		t.Errorf("stdout = %q, stderr = %q", out, errOut)
	}
}

func TestAPIEmptyResponse(t *testing.T) {
	h := newHarness(t)
	h.login()
	h.terminal = true
	out, errOut, code := h.run("api", "DELETE", "/webhooks/7", "--yes")
	if code != 0 || out != "" || !strings.Contains(errOut, "HTTP 204") {
		t.Errorf("code %d, out %q, err %q", code, out, errOut)
	}
}

func TestCommandsCatalogue(t *testing.T) {
	h := newHarness(t)
	out, _, code := h.run("commands", "--json")
	if code != 0 {
		t.Fatal(out)
	}
	var cat Catalogue
	if err := json.Unmarshal([]byte(out), &cat); err != nil {
		t.Fatal(err)
	}
	if cat.SpecVersion != spec.Version() || len(cat.Operations) < 100 {
		t.Errorf("catalogue = %d operations, spec %s", len(cat.Operations),
			cat.SpecVersion)
	}
	paths := map[string]Command{}
	for _, c := range cat.Commands {
		paths[c.Path] = c
	}
	for _, want := range []string{"transpareo api", "transpareo auth login",
		"transpareo auth status",
		"transpareo auth logout", "transpareo auth token",
		"transpareo auth grant", "transpareo me",
		"transpareo commands", "transpareo schema", "transpareo guide",
		"transpareo doctor",
		"transpareo version", "transpareo completion bash"} {
		if _, ok := paths[want]; !ok {
			t.Errorf("%q is missing from the catalogue", want)
		}
	}
	if paths["transpareo me"].OperationID != "get_me" ||
		paths["transpareo api"].Example == "" {
		t.Errorf("command = %+v", paths["transpareo me"])
	}
	if strings.Contains(out, `\u003c`) {
		t.Error("JSON output must not escape angle brackets")
	}

	h.terminal = true
	out, _, _ = h.run("commands")
	if !strings.Contains(out, "transpareo api") ||
		!strings.Contains(out, "API operations") {
		t.Errorf("listing = %q", out)
	}
}

func TestSchema(t *testing.T) {
	h := newHarness(t)
	h.terminal = true
	out, _, code := h.run("schema", "create_dpp")
	if code != 0 {
		t.Fatal(out)
	}
	schema := decode(t, out)
	if schema["method"] != "POST" || schema["path"] != "/dpps" ||
		schema["requestBody"] == nil ||
		schema["requestExample"] == nil {
		t.Errorf("schema = %v", schema)
	}
	if _, errOut, code := h.run("schema", "nope"); code != 2 ||
		!strings.Contains(errOut, "unknown operation") {
		t.Errorf("code = %d, stderr = %q", code, errOut)
	}
}

func TestGuide(t *testing.T) {
	h := newHarness(t)
	h.env["TRANSPAREO_HOST"] = h.server.URL
	out, _, code := h.run("guide")
	if code != 0 || out != "# API guide\n" {
		t.Errorf("code = %d, out = %q", code, out)
	}
	if h.lastRequest().Header.Get("Authorization") != "" {
		t.Error("the guide needs no credential")
	}
	delete(h.env, "TRANSPAREO_HOST")
	if out, _, code := h.run("guide"); code != 1 ||
		!strings.Contains(out, "NO_PROFILE") {
		t.Errorf("without a host: code %d, out %s", code, out)
	}
}

func TestDoctor(t *testing.T) {
	h := newHarness(t)
	h.login()
	out, _, code := h.run("doctor", "--json")
	if code != 0 {
		t.Fatalf("code = %d: %s", code, out)
	}
	var report Report
	json.Unmarshal([]byte(out), &report)
	status := map[string]string{}
	for _, c := range report.Checks {
		status[c.Name] = c.Status
	}
	want := map[string]string{"binary": "ok", "profile": "ok", "host": "ok",
		"token endpoint": "ok", "credential": "ok", "keyring": "warn"}
	for name, s := range want {
		if status[name] != s {
			t.Errorf("%s = %q, want %q", name, status[name], s)
		}
	}

	h.env["TRANSPAREO_CLIENT_SECRET"] = "wrong"
	out, _, code = h.run("doctor")
	if code != 1 {
		t.Errorf("a failing check must exit 1, got %d: %s", code, out)
	}
	json.Unmarshal([]byte(out), &report)
	if report.OK {
		t.Error("report must not be ok")
	}
}

func TestDoctorWithoutProfile(t *testing.T) {
	h := newHarness(t)
	h.terminal = true
	out, _, code := h.run("doctor")
	if code != 1 || !strings.Contains(out, "none configured") ||
		!strings.Contains(out, "skipped") {
		t.Errorf("code = %d, out = %q", code, out)
	}
}

func TestVersionAndUsageErrors(t *testing.T) {
	h := newHarness(t)
	out, _, code := h.run("version")
	if code != 0 || decode(t, out)["specVersion"] != spec.Version() {
		t.Errorf("version = %s", out)
	}
	h.terminal = true
	out, _, _ = h.run("version")
	if !strings.HasPrefix(out, "transpareo dev") {
		t.Errorf("version = %q", out)
	}
	_, errOut, code := h.run("api", "GET")
	if code != 2 || !strings.Contains(errOut, "Usage:") {
		t.Errorf("code = %d, stderr = %q", code, errOut)
	}
	if _, _, code := h.run("--nope"); code != 2 {
		t.Errorf("unknown flag: code %d", code)
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct{ builtIn, live, want string }{
		{"1.6.0", "1.6.0", statusOK},
		{"1.6.0", "1.6.3", statusOK},
		{"1.6.0", "1.7.0", statusWarn},
		{"1.6.0", "2.0.0", statusError},
		{"2.0.0", "1.9.0", statusWarn},
		{"1.6.0", "x", statusWarn},
	}
	for _, tc := range cases {
		if got, _ := compareVersions(tc.builtIn, tc.live); got != tc.want {
			t.Errorf("%s vs %s: %s, want %s", tc.builtIn, tc.live, got, tc.want)
		}
	}
}

func TestProfileNameFor(t *testing.T) {
	for in, want := range map[string]string{
		"acme.example.com": "acme", "https://acme.example.com/apidocs": "acme",
		"localhost:3000": "localhost", "http://127.0.0.1:8080": "127.0.0.1",
	} {
		if got := profileNameFor(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}
