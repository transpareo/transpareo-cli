//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/transpareo/transpareo-cli/internal/auth"
	"github.com/transpareo/transpareo-cli/internal/cli"
	"github.com/transpareo/transpareo-cli/pkg/transpareo"
)

type env struct {
	host, clientID, secret, secondHost string
}

func load(t *testing.T) env {
	t.Helper()
	e := env{
		host:       os.Getenv("TRANSPAREO_TEST_HOST"),
		clientID:   os.Getenv("TRANSPAREO_TEST_CLIENT_ID"),
		secret:     os.Getenv("TRANSPAREO_TEST_CLIENT_SECRET"),
		secondHost: os.Getenv("TRANSPAREO_TEST_SECOND_HOST"),
	}
	if e.host == "" || e.clientID == "" || e.secret == "" {
		t.Skip("TRANSPAREO_TEST_HOST, _CLIENT_ID and _CLIENT_SECRET " +
			"are not set")
	}
	return e
}

func (e env) client(t *testing.T) *transpareo.Client {
	t.Helper()
	creds := transpareo.ClientCredentials{ID: e.clientID, Secret: e.secret}
	c, err := transpareo.New(e.host, creds)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func apiError(t *testing.T, err error) *transpareo.Error {
	t.Helper()
	var apiErr *transpareo.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected an API error, got %T: %v", err, err)
	}
	return apiErr
}

func TestTokenExchangeAndMe(t *testing.T) {
	e := load(t)
	ctx := context.Background()
	c := e.client(t)
	token, err := c.TokenSource().Token(ctx)
	if err != nil {
		t.Fatalf("token exchange: %v", err)
	}
	if token.AccessToken == "" {
		t.Fatal("empty token")
	}
	lifetime := time.Until(token.ExpiresAt)
	if lifetime < 50*time.Minute || lifetime > 70*time.Minute {
		t.Errorf("token lifetime %v, want about an hour", lifetime)
	}
	me, err := c.Me(ctx)
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	if me.Key != e.clientID || me.Status != "active" {
		t.Errorf("me = %+v", me)
	}
	if len(me.Permissions) == 0 || len(me.Scope) == 0 {
		t.Errorf("permissions %v scope %v", me.Permissions, me.Scope)
	}
	if me.TokenExpiresAt.Sub(token.ExpiresAt).Abs() > time.Minute {
		t.Errorf("tokenExpiresAt %v differs from expires_in %v",
			me.TokenExpiresAt, token.ExpiresAt)
	}
}

func TestWrongSecretIsInvalidClient(t *testing.T) {
	e := load(t)
	creds := transpareo.ClientCredentials{ID: e.clientID, Secret: "wrong"}
	c, _ := transpareo.New(e.host, creds)
	_, err := c.Me(context.Background())
	apiErr := apiError(t, err)
	if apiErr.Code != "invalid_client" || apiErr.Status != 401 ||
		apiErr.Retryable {
		t.Errorf("error = %+v", apiErr)
	}
}

func TestForgedTokenIsRefused(t *testing.T) {
	e := load(t)
	c, _ := transpareo.NewWithToken(e.host, "not.a.token")
	_, err := c.Me(context.Background())
	apiErr := apiError(t, err)
	if apiErr.Status != 401 || apiErr.Code != "TOKEN_INVALID" {
		t.Errorf("error = %+v", apiErr)
	}
	if apiErr.DocsURL != "" &&
		!strings.Contains(apiErr.DocsURL, "/apidocs/guide#") {
		t.Errorf("docsUrl = %q", apiErr.DocsURL)
	}
}

func TestUnknownOAuthPathIs404(t *testing.T) {
	e := load(t)
	c := e.client(t)
	_, err := c.Do(context.Background(), &transpareo.Request{
		Method: http.MethodGet, Path: "/oauth/nothing"})
	apiErr := apiError(t, err)
	if apiErr.Status != 404 {
		t.Errorf("status = %d, error = %+v", apiErr.Status, apiErr)
	}
}

func TestRateLimitHeaders(t *testing.T) {
	e := load(t)
	c := e.client(t)
	resp, err := c.Do(context.Background(), &transpareo.Request{
		Method: http.MethodGet, Path: "/me"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.RateLimit.Limit <= 0 || resp.RateLimit.Reset.IsZero() {
		t.Errorf("rate limit headers = %+v", resp.RateLimit)
	}
}

type brand struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// createBrand posts one brand and registers its deletion.
func createBrand(t *testing.T, c *transpareo.Client, name,
	key string) (brand, *transpareo.Response) {
	t.Helper()
	var out brand
	body := map[string]any{"brand": map[string]any{"name": name}}
	resp, err := c.JSON(context.Background(), &transpareo.Request{
		Method: http.MethodPost, Path: "/brands", Body: body,
		IdempotencyKey: key}, &out)
	if err != nil {
		t.Fatalf("create brand %q: %v", name, err)
	}
	if out.ID == "" {
		t.Fatalf("create brand %q answered no id: %s", name, resp.Body)
	}
	t.Cleanup(func() {
		_, err := c.Delete(context.Background(), "/brands/"+out.ID, nil)
		if err != nil {
			t.Errorf("delete brand %s: %v", out.ID, err)
		}
	})
	return out, resp
}

func runID() string {
	return fmt.Sprintf("cli-e2e-%d", time.Now().UnixNano())
}

func TestIdempotencyKeyReplays(t *testing.T) {
	e := load(t)
	c := e.client(t)
	name := runID()
	first, resp := createBrand(t, c, name, name)
	if resp.Replayed() {
		t.Error("the first request must not be a replay")
	}
	var again brand
	body := map[string]any{"brand": map[string]any{"name": name}}
	resp, err := c.JSON(context.Background(), &transpareo.Request{
		Method: http.MethodPost, Path: "/brands", Body: body,
		IdempotencyKey: name}, &again)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !resp.Replayed() || again.ID != first.ID {
		t.Errorf("replayed %v, ids %s and %s", resp.Replayed(), first.ID,
			again.ID)
	}
}

type webhook struct {
	ID  json.Number `json:"id"`
	URL string      `json:"url"`
}

// webhookPrefix marks the subscriptions this suite creates, so
// a run that died half way is cleaned up by the next one.
const webhookPrefix = "https://example.com/hooks/cli-e2e-"

func deleteWebhook(t *testing.T, c *transpareo.Client, id json.Number) {
	t.Helper()
	_, err := c.Delete(context.Background(), "/webhooks/"+id.String(), nil)
	if err != nil {
		t.Errorf("delete webhook %s: %v", id, err)
	}
}

// sweepWebhooks removes leftovers of earlier runs.
func sweepWebhooks(t *testing.T, c *transpareo.Client) {
	t.Helper()
	ctx := context.Background()
	for w, err := range transpareo.ListAll[webhook](ctx, c, "/webhooks", nil) {
		if err != nil {
			t.Fatalf("listing webhooks: %v", err)
		}
		if strings.HasPrefix(w.URL, webhookPrefix) {
			deleteWebhook(t, c, w.ID)
		}
	}
}

func createWebhook(t *testing.T, c *transpareo.Client, url string) webhook {
	t.Helper()
	var out webhook
	body := map[string]any{"webhook": map[string]any{
		"url": url, "eventTypes": []string{"published", "voided"}}}
	_, err := c.Post(context.Background(), "/webhooks", body, &out)
	if err != nil {
		t.Fatalf("create webhook %q: %v", url, err)
	}
	if out.ID == "" {
		t.Fatalf("create webhook %q answered no id", url)
	}
	t.Cleanup(func() { deleteWebhook(t, c, out.ID) })
	return out
}

// TestListFollowsLinkHeader pages over the consumer's own webhook
// subscriptions, the one list that shows a consumer's records
// whether or not they are published.
func TestListFollowsLinkHeader(t *testing.T) {
	e := load(t)
	c := e.client(t)
	ctx := context.Background()
	sweepWebhooks(t, c)
	prefix := webhookPrefix + fmt.Sprint(time.Now().UnixNano())
	created := map[string]bool{}
	for i := range 2 {
		w := createWebhook(t, c, fmt.Sprintf("%s-%d", prefix, i))
		created[w.ID.String()] = true
	}
	query := url.Values{"per_page": {"1"}}
	page, err := transpareo.List[webhook](ctx, c, "/webhooks", query)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total < 2 || page.PerPage != 1 || page.NextURL == "" {
		t.Fatalf("page = total %d per page %d next %q", page.Total,
			page.PerPage, page.NextURL)
	}
	seen := 0
	for w, err := range transpareo.ListAll[webhook](ctx, c, "/webhooks", query) {
		if err != nil {
			t.Fatal(err)
		}
		if created[w.ID.String()] {
			seen++
		}
	}
	if seen != 2 {
		t.Errorf("saw %d of the 2 created webhooks while following Link", seen)
	}
}

func TestSecondHostRefusesTheToken(t *testing.T) {
	e := load(t)
	if e.secondHost == "" {
		t.Skip("TRANSPAREO_TEST_SECOND_HOST is not set")
	}
	token, err := e.client(t).TokenSource().Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	other, _ := transpareo.NewWithToken(e.secondHost, token.AccessToken)
	_, err = other.Me(context.Background())
	apiErr := apiError(t, err)
	if apiErr.Status != 401 {
		t.Errorf("a token from %s was accepted on %s: %+v", e.host,
			e.secondHost, apiErr)
	}
}

// run executes the command line in-process with credentials from
// the environment and an empty configuration directory.
func run(t *testing.T, e env, args ...string) (string, string, int) {
	t.Helper()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	dir := t.TempDir()
	values := map[string]string{
		auth.EnvHost:         e.host,
		auth.EnvClientID:     e.clientID,
		auth.EnvClientSecret: e.secret,
	}
	app := &cli.App{
		Stdin:     strings.NewReader(""),
		Stdout:    stdout,
		Stderr:    stderr,
		Getenv:    func(key string) string { return values[key] },
		ConfigDir: dir,
		WorkDir:   dir,
		Stores: &auth.Stores{Dir: dir, Keyring: &auth.MemoryStore{},
			File: auth.NewFileStore(dir)},
	}
	code := cli.Main(context.Background(), app, args)
	return stdout.String(), stderr.String(), code
}

func TestCommandLineMeAndDoctor(t *testing.T) {
	e := load(t)
	out, errOut, code := run(t, e, "me")
	if code != 0 {
		t.Fatalf("me: exit %d: %s%s", code, out, errOut)
	}
	var me map[string]any
	err := json.Unmarshal([]byte(out), &me)
	if err != nil || me["key"] != e.clientID {
		t.Errorf("me = %s", out)
	}

	out, errOut, code = run(t, e, "doctor", "--json")
	if code != 0 {
		t.Fatalf("doctor: exit %d: %s%s", code, out, errOut)
	}
	var report cli.Report
	json.Unmarshal([]byte(out), &report)
	for _, check := range report.Checks {
		if check.Status == "error" {
			t.Errorf("doctor: %s: %s", check.Name, check.Detail)
		}
	}

	out, _, code = run(t, e, "api", "GET", "/me", "--fields", "key,status")
	if code != 0 || !strings.Contains(out, e.clientID) {
		t.Errorf("api GET /me: exit %d: %s", code, out)
	}
}
