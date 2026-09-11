//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
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

// tokenSources holds one source per consumer for the whole run,
// so the suite exchanges a token once per consumer the way a
// client in use does and stays clear of the token endpoint's
// limit of ten exchanges a minute. Only the checks that are about
// the exchange itself call the endpoint again.
var (
	tokenSourcesMu sync.Mutex
	tokenSources   = map[string]transpareo.TokenSource{}
)

func tokenSource(t *testing.T, host, id, secret string) transpareo.TokenSource {
	t.Helper()
	tokenSourcesMu.Lock()
	defer tokenSourcesMu.Unlock()
	key := host + " " + id
	if ts, ok := tokenSources[key]; ok {
		return ts
	}
	creds := transpareo.ClientCredentials{ID: id, Secret: secret}
	ts, err := transpareo.NewClientCredentialsSource(host, creds, nil)
	if err != nil {
		t.Fatal(err)
	}
	tokenSources[key] = ts
	return ts
}

// logger writes the attempts the client repeats into the test
// output, so a stalled request that a retry hid is named there.
var logger = slog.New(slog.NewTextHandler(os.Stderr, nil))

// client answers a client on the shared token of the writing
// consumer.
func (e env) client(t *testing.T) *transpareo.Client {
	t.Helper()
	return sharedClient(t, e.host, e.clientID, e.secret)
}

func sharedClient(t *testing.T, host, id, secret string) *transpareo.Client {
	t.Helper()
	creds := transpareo.ClientCredentials{ID: id, Secret: secret}
	c, err := transpareo.New(host, creds,
		transpareo.WithTokenSource(tokenSource(t, host, id, secret)),
		transpareo.WithLogger(logger))
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

// TestTokenExchangeAndMe is the check about the exchange, so it
// builds a client that exchanges on its own.
func TestTokenExchangeAndMe(t *testing.T) {
	e := load(t)
	ctx := context.Background()
	creds := transpareo.ClientCredentials{ID: e.clientID, Secret: e.secret}
	c, err := transpareo.New(e.host, creds)
	if err != nil {
		t.Fatal(err)
	}
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

// brand takes the id as a number because the platform answers
// integer ids for brands.
type brand struct {
	ID   json.Number `json:"id"`
	Name string      `json:"name"`
}

func (b brand) identifier() json.Number { return b.ID }
func (b brand) named() string           { return b.Name }

// product is what a sweep of earlier runs needs of one.
type product struct {
	ID   json.Number `json:"id"`
	Name string      `json:"name"`
}

func (p product) identifier() json.Number { return p.ID }
func (p product) named() string           { return p.Name }

// createBrand posts one brand and registers its deletion.
func createBrand(t *testing.T, c *transpareo.Client, name,
	key string) (brand, *transpareo.Response) {
	t.Helper()
	sweepLeftovers(t, c)
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
		path := "/brands/" + out.ID.String()
		if _, err := c.Delete(context.Background(), path, nil); err != nil {
			t.Errorf("delete brand %s: %v", out.ID, err)
		}
	})
	return out, resp
}

// runPrefix marks every record this suite names, so a run that
// died before its cleanup is recognisable to the next one.
const runPrefix = "cli-e2e-"

// staleAfter is how old a leftover has to be before a sweep
// takes it, so a run beside this one keeps its own records.
const staleAfter = time.Hour

func runID() string {
	return fmt.Sprintf("%s%d", runPrefix, time.Now().UnixNano())
}

// staleRunID says whether a name carries a run id of this suite
// older than staleAfter. The platform capitalises a name it
// stores, so the comparison is made in lower case.
func staleRunID(name string, now time.Time) bool {
	digits, found := strings.CutPrefix(strings.ToLower(name), runPrefix)
	if !found {
		return false
	}
	nanos, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return false
	}
	return now.Sub(time.Unix(0, nanos)) > staleAfter
}

// sweptOnce keeps the sweep to one per run, whichever check
// creates the first record.
var sweptOnce sync.Once

// sweepLeftovers removes the products and brands of runs that
// died before their own cleanup could delete them. A record it
// cannot delete, a product a passport was published against for
// instance, is reported and left: this is tidying, and it never
// fails a run of its own accord.
func sweepLeftovers(t *testing.T, c *transpareo.Client) {
	t.Helper()
	sweptOnce.Do(func() {
		now := time.Now()
		sweepLeftover[product](t, c, "/products", now)
		sweepLeftover[brand](t, c, "/brands", now)
	})
}

// named is a record this suite can recognise by its name and
// delete by its id.
type named interface {
	identifier() json.Number
	named() string
}

func sweepLeftover[T named](t *testing.T, c *transpareo.Client, path string,
	now time.Time) {
	t.Helper()
	ctx := context.Background()
	for record, err := range transpareo.ListAll[T](ctx, c, path, nil) {
		if err != nil {
			t.Logf("listing %s to sweep: %v", path, err)
			return
		}
		if !staleRunID(record.named(), now) {
			continue
		}
		id := record.identifier().String()
		if _, err := c.Delete(ctx, path+"/"+id, nil); err != nil {
			t.Logf("leftover %s/%s (%s) stays: %v", path, id,
				record.named(), err)
			continue
		}
		t.Logf("swept %s/%s (%s) of an earlier run", path, id, record.named())
	}
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
	for w, err := range transpareo.ListAll[webhook](ctx, c, "/webhooks",
		query) {
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

// run executes the command line in-process on the shared token,
// the way a script with TRANSPAREO_TOKEN set runs it, in an empty
// configuration directory.
func run(t *testing.T, e env, args ...string) (string, string, int) {
	t.Helper()
	token, err := e.client(t).TokenSource().Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return runWith(t, map[string]string{auth.EnvHost: e.host,
		auth.EnvToken: token.AccessToken}, args...)
}

// runWithCredentials lets the command line exchange a token on
// its own, for the checks that are about the exchange.
func runWithCredentials(t *testing.T, e env, args ...string) (string,
	string, int) {
	t.Helper()
	return runWith(t, map[string]string{auth.EnvHost: e.host,
		auth.EnvClientID: e.clientID, auth.EnvClientSecret: e.secret},
		args...)
}

func runWith(t *testing.T, values map[string]string,
	args ...string) (string, string, int) {
	t.Helper()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	dir := t.TempDir()
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
