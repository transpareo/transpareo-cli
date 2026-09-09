package transpareo

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/transpareo/transpareo-cli/pkg/transpareo/api"
)

func TestTypedClientGoesThroughTheTransport(t *testing.T) {
	ts := newTokenServer(t)
	var calls atomic.Int32
	var got http.Header
	ts.Mux.HandleFunc("GET /api/brands", func(w http.ResponseWriter,
		r *http.Request) {
		got = r.Header.Clone()
		if calls.Add(1) == 1 {
			writeJSON(w, 503, map[string]string{"error": "DOWN",
				"message": "x"})
			return
		}
		w.Header().Set("API-Total", "1")
		writeJSON(w, 200, map[string]any{"brands": []map[string]any{
			{"id": "b1", "name": "Alpha"}}})
	})
	c := newTestClient(t, ts, newFakeClock(), WithUserAgent("typed/1"))
	resp, err := c.API().ListBrandsWithResponse(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode() != 200 || resp.JSON200 == nil ||
		resp.JSON200.Brands == nil {
		t.Fatalf("response = %d %s", resp.StatusCode(), resp.Body)
	}
	brands := *resp.JSON200.Brands
	if len(brands) != 1 || brands[0].Name == nil || *brands[0].Name != "Alpha" {
		t.Errorf("brands = %+v", brands)
	}
	if got.Get("Authorization") != "Bearer tok1" ||
		got.Get("User-Agent") != "typed/1" {
		t.Errorf("headers = %v", got)
	}
	if got.Get("Accept") == "" {
		t.Error("the Accept header is missing")
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want a retry after the 503", calls.Load())
	}
}

func TestTypedClientSendsIdempotencyKeyAndBody(t *testing.T) {
	ts := newTokenServer(t)
	var key, body string
	ts.Mux.HandleFunc("POST /api/brands", func(w http.ResponseWriter,
		r *http.Request) {
		key = r.Header.Get("Idempotency-Key")
		b, _ := readBody(r)
		body = b
		writeJSON(w, 201, map[string]any{"id": "b2"})
	})
	c := newTestClient(t, ts, newFakeClock())
	name := "Natura"
	in := api.BrandInput{Brand: struct {
		Name string `json:"name"`
	}{Name: name}}
	resp, err := c.API().CreateBrandWithResponse(context.Background(), nil, in)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode() != 201 || len(key) != 36 {
		t.Errorf("status %d key %q", resp.StatusCode(), key)
	}
	if body != `{"brand":{"name":"Natura"}}` {
		t.Errorf("body = %s", body)
	}
}

func TestTypedClientReturnsRefusalsAsResponses(t *testing.T) {
	ts := newTokenServer(t)
	ts.Mux.HandleFunc("GET /api/brands/x", func(w http.ResponseWriter,
		r *http.Request) {
		writeJSON(w, 404, map[string]string{"error": "BRAND_NOT_FOUND",
			"message": "no"})
	})
	c := newTestClient(t, ts, newFakeClock())
	resp, err := c.API().GetBrandWithResponse(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode() != 404 || resp.JSON404 == nil ||
		*resp.JSON404.Error != "BRAND_NOT_FOUND" {
		t.Errorf("response = %d %s", resp.StatusCode(), resp.Body)
	}
}

func TestTransportRefusesOtherHosts(t *testing.T) {
	ts := newTokenServer(t)
	c := newTestClient(t, ts, newFakeClock())
	req, _ := http.NewRequest(http.MethodGet,
		"https://other.example.com/api/me", nil)
	_, err := c.HTTPClient().Do(req)
	if !IsCode(err, CodeHostMismatch) {
		t.Errorf("err = %v", err)
	}
}
