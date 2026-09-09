//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/transpareo/transpareo-cli/internal/flows"
	"github.com/transpareo/transpareo-cli/internal/mcp"
	"github.com/transpareo/transpareo-cli/pkg/transpareo"
)

func jsonOf(t *testing.T, out string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("not JSON: %v: %s", err, out)
	}
	return m
}

func TestAuthTokenWithScope(t *testing.T) {
	e := load(t)
	out, errOut, code := run(t, e, "auth", "token", "--scope", "dpp_read",
		"--json")
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errOut)
	}
	tok := jsonOf(t, out)
	scope, _ := tok["scope"].([]any)
	if len(scope) != 1 || scope[0] != "dpp_read" {
		t.Errorf("scope = %v", scope)
	}
	other, _ := transpareo.NewWithToken(e.host, tok["accessToken"].(string))
	me, err := other.Me(context.Background())
	if err != nil || len(me.Scope) != 1 || me.Scope[0] != "dpp_read" {
		t.Errorf("me with the scoped token: %+v %v", me, err)
	}
}

// throwawayProduct creates a product for the run and deletes it
// at the end. It skips when the workspace demands mandatory
// properties the suite does not know.
func throwawayProduct(t *testing.T, c *transpareo.Client) json.Number {
	t.Helper()
	ctx := context.Background()
	var template map[string]any
	if _, err := c.Get(ctx, "/products/new", nil, &template); err != nil {
		t.Fatalf("products new: %v", err)
	}
	name := runID()
	var created struct {
		ID json.Number `json:"id"`
	}
	// One brand for every run, since a brand created through a
	// product outlives the product.
	body := map[string]any{
		"product": map[string]any{"name": name,
			"componentsInput": []map[string]any{{"name": "cli e2e component"}}},
		"brand": map[string]any{"name": "CLI end-to-end checks"},
	}
	_, err := c.Post(ctx, "/products", body, &created)
	if transpareo.IsCode(err, "PRODUCT_MISSING_PROPERTIES") {
		t.Skipf("the workspace demands mandatory properties: %v", err)
	}
	if err != nil {
		var apiErr *transpareo.Error
		if errors.As(err, &apiErr) {
			t.Fatalf("create product: %v\nfields: %+v", err, apiErr.Fields)
		}
		t.Fatalf("create product: %v", err)
	}
	t.Cleanup(func() {
		if _, err := c.Delete(ctx, "/products/"+created.ID.String(),
			nil); err != nil {
			t.Errorf("delete product %s: %v", created.ID, err)
		}
	})
	return created.ID
}

func TestPassportFlowOnAThrowawayProduct(t *testing.T) {
	e := load(t)
	c := e.client(t)
	ctx := context.Background()
	productID := throwawayProduct(t, c)

	var requirements struct {
		Templates struct {
			Create map[string]any `json:"create"`
		} `json:"templates"`
		Outlook struct {
			PublishBlocked bool `json:"publishBlocked"`
		} `json:"outlook"`
	}
	_, err := c.Get(ctx, "/dpps/requirements",
		map[string][]string{"productId": {productID.String()},
			"granularity": {"item"}},
		&requirements)
	if err != nil {
		t.Fatalf("requirements: %v", err)
	}
	dppTemplate, _ := requirements.Templates.Create["dpp"].(map[string]any)
	if dppTemplate == nil {
		t.Fatalf("requirements carry no create template: %+v", requirements)
	}
	dppTemplate["modelIdentifier"] = "CLI-E2E"
	dppTemplate["batchIdentifier"] = "L-e2e"
	dppTemplate["serialIdentifier"] = fmt.Sprintf("%d",
		time.Now().UnixNano()%1000000)
	dppTemplate["description"] = "cli e2e"
	dpp := map[string]any{"dpp": dppTemplate}
	// valid is false both for a model error, which fields names,
	// and for a publish rule the workspace's templates impose; only
	// the former stops the flow.
	var validation struct {
		Valid          bool           `json:"valid"`
		Fields         map[string]any `json:"fields"`
		PublishBlocked bool           `json:"publishBlocked"`
	}
	if _, err := c.Post(ctx, "/dpps/validate", dpp, &validation); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if len(validation.Fields) > 0 {
		t.Fatalf("the passport from the template has model errors: %v",
			validation.Fields)
	}
	if !validation.Valid && !validation.PublishBlocked {
		t.Fatalf("valid false without fields or a publish block")
	}
	var created struct {
		ID   json.Number `json:"id"`
		Code string      `json:"code"`
	}
	if _, err := c.Post(ctx, "/dpps", dpp, &created); err != nil {
		t.Fatalf("create dpp: %v", err)
	}
	if created.Code == "" || created.ID == "" {
		t.Fatalf("created passport lacks an id or a code: %+v", created)
	}
	// A workspace whose templates block publishing answers a
	// documented refusal; the flow up to here is what the suite
	// proves on such a tenant.
	_, err = c.Post(ctx, "/dpps/"+created.Code+"/publish",
		map[string]any{"reason": "edit"}, nil)
	switch {
	case err == nil:
	case (requirements.Outlook.PublishBlocked || validation.PublishBlocked) &&
		strings.HasPrefix(errorCode(err), "DPP_PUBLISH_"):
		t.Logf("publish blocked by the workspace's templates: %v", err)
	default:
		t.Fatalf("publish: %v", err)
	}
	// get_dpp takes the record id; the public code names the
	// publish and private-properties paths.
	var fetched map[string]any
	_, err = c.Get(ctx, "/dpps/"+created.ID.String(), nil, &fetched)
	if err != nil {
		t.Fatalf("get dpp: %v", err)
	}
	if fetched["code"] != created.Code {
		t.Errorf("fetched = %v", fetched)
	}

	rows := "{\"modelIdentifier\": \"" + runID() + "\", \"unlockableId\": " +
		"" + productID.String() + "}\n"
	resp, err := c.Do(ctx, &transpareo.Request{Method: http.MethodPost,
		Path: "/dpps/bulk/validate",
		Body: []byte(rows), ContentType: "application/x-ndjson"})
	if err != nil {
		t.Fatalf("bulk validate: %v", err)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "ndjson") ||
		len(resp.Body) == 0 {
		t.Errorf("bulk validate answered %q: %s",
			resp.Header.Get("Content-Type"), resp.Body)
	}

	feed, err := flows.ReadFeed(ctx, c, flows.FeedOptions{DppCode: created.Code,
		Limit: 10})
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if feed.NextCursor == "" {
		t.Error("the feed answered no cursor")
	}
}

func TestImportValidateWithTheTemplate(t *testing.T) {
	e := load(t)
	c := e.client(t)
	ctx := context.Background()
	resp, err := c.Do(ctx, &transpareo.Request{Method: http.MethodGet,
		Path:   "/imports/example",
		Query:  map[string][]string{"dataType": {"products"}},
		Accept: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"})
	if err != nil {
		t.Fatalf("import example: %v", err)
	}
	path := filepath.Join(t.TempDir(), "template.xlsx")
	os.WriteFile(path, resp.Body, 0o600)
	imp, err := flows.Run(ctx, c, flows.RunOptions{Path: path,
		DataType:          "products",
		AcceptSuggestions: true})
	if flows.IsMappingRequired(err) {
		t.Fatalf("the template must map on its own: %v", err)
	}
	if err != nil && imp == nil {
		t.Fatalf("import run: %v", err)
	}
	if imp.Status != "validated" && imp.Status != "mapped" {
		t.Errorf("import status = %s (%v)", imp.Status, err)
	}
}

func TestExportCreateAndDownload(t *testing.T) {
	e := load(t)
	c := e.client(t)
	ctx := context.Background()
	exp, err := flows.StartExport(ctx, c, flows.ExportOptions{Format: "jsonld",
		Wait: true})
	if transpareo.IsCode(err, "EXPORT_IN_PROGRESS") {
		t.Skip("another export is running for this consumer")
	}
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var buf bytes.Buffer
	n, err := flows.Download(ctx, c, exp, &buf)
	if err != nil || n == 0 ||
		!bytes.HasPrefix(buf.Bytes(), []byte{0x1f, 0x8b}) {
		t.Errorf("download: %d bytes, err %v, gzip header %v", n, err,
			buf.Bytes()[:min(2, buf.Len())])
	}
}

func TestWebhookCreateTestDelete(t *testing.T) {
	e := load(t)
	c := e.client(t)
	ctx := context.Background()
	sweepWebhooks(t, c)
	w := createWebhook(t, c, webhookPrefix+fmt.Sprint(time.Now().UnixNano()))
	var result struct {
		OK      bool   `json:"ok"`
		Status  *int   `json:"status"`
		Failure string `json:"failure"`
	}
	if _, err := c.Post(ctx, "/webhooks/"+w.ID.String()+"/test", nil,
		&result); err != nil {
		t.Fatalf("test webhook: %v", err)
	}
	if result.Status == nil && result.Failure == "" {
		t.Errorf("test answered neither a status nor a failure: %+v", result)
	}
}

func TestMCPServerAgainstTheHost(t *testing.T) {
	e := load(t)
	server := mcp.New(mcp.Options{Client: func() (*transpareo.Client, error) {
		return e.client(t), nil
	}, Version: "e2e", Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	session, err := sdk.NewClient(&sdk.Implementation{Name: "e2e",
		Version: "0"}, nil).
		Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	count := 0
	for _, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatal(err)
		}
		count++
	}
	if count < 30 {
		t.Errorf("only %d tools", count)
	}
	me, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "me"})
	if err != nil || me.IsError {
		t.Fatalf("me: %v %v", err, me)
	}
	search, err := session.CallTool(ctx,
		&sdk.CallToolParams{Name: "search_operations",
			Arguments: map[string]any{"query": "webhook"}})
	if err != nil || search.IsError {
		t.Fatalf("search_operations: %v", err)
	}
	if text, ok := search.Content[0].(*sdk.TextContent); !ok ||
		!strings.Contains(text.Text, "operations match") {
		t.Errorf("search text = %v", search.Content)
	}
}

func TestReadOnlyConsumerIsRefusedAWrite(t *testing.T) {
	e := load(t)
	id, secret := os.Getenv("TRANSPAREO_TEST_READONLY_CLIENT_ID"),
		os.Getenv("TRANSPAREO_TEST_READONLY_CLIENT_SECRET")
	if id == "" || secret == "" {
		t.Skip("TRANSPAREO_TEST_READONLY_CLIENT_ID and _SECRET are not set")
	}
	c, err := transpareo.New(e.host, transpareo.ClientCredentials{ID: id,
		Secret: secret})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Me(context.Background()); err != nil {
		t.Fatalf("me: %v", err)
	}
	_, err = c.Post(context.Background(), "/brands",
		map[string]any{"brand": map[string]any{
			"name": runID()}}, nil)
	var apiErr *transpareo.Error
	if !errors.As(err, &apiErr) || apiErr.Status != 403 {
		t.Errorf("a read-only consumer wrote a brand: %v", err)
	}
}

func errorCode(err error) string {
	var apiErr *transpareo.Error
	if errors.As(err, &apiErr) {
		return apiErr.Code
	}
	return ""
}
