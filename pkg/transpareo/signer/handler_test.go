package signer

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serve(t *testing.T, v *vectors) (*httptest.Server, *bytes.Buffer) {
	t.Helper()
	ver := NewVerifier(v.platformKey(t))
	ver.AllowUnsigned = true
	var log bytes.Buffer
	h := Handler(New(v.keys(t)), ver, HandlerOptions{Path: "/sign",
		Logger: slog.New(slog.NewTextHandler(&log, nil))})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, &log
}

func post(t *testing.T, url string, body string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var out map[string]any
	json.Unmarshal(data, &out)
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("content type = %q", ct)
	}
	return resp.StatusCode, out
}

func TestHandlerServesBothShapes(t *testing.T) {
	v := loadVectors(t)
	srv, log := serve(t, v)
	status, out := post(t, srv.URL+"/sign", v.Documents.Request)
	if status != 200 {
		t.Fatalf("documents: %d %v", status, out)
	}
	sigs, _ := out["signatures"].([]any)
	first, _ := sigs[0].(map[string]any)
	values, _ := first["proofValues"].([]any)
	if len(values) != 2 || values[0] != v.Documents.ProofValues[0] {
		t.Errorf("proof values = %v", values)
	}
	base, _ := json.Marshal(map[string]any{"cryptosuite": CryptosuiteBaseProof,
		"base_proof": v.BaseProof2()})
	status, out = post(t, srv.URL+"/sign", string(base))
	bp, _ := out["base_proof"].(map[string]any)
	if status != 200 || bp["public_key"] == nil || bp["base_signature"] == nil {
		t.Errorf("base proof: %d %v", status, out)
	}
	if !strings.Contains(log.String(), "kind=documents") ||
		!strings.Contains(log.String(), `kind="base proof"`) {
		t.Errorf("log = %s", log.String())
	}
}

func TestHandlerRefusals(t *testing.T) {
	v := loadVectors(t)
	srv, _ := serve(t, v)
	cases := []struct {
		name, method, path, body string
		status                   int
	}{
		{"route", "POST", "/other", "{}", 404},
		{"method", "GET", "/sign", "", 405},
		{"not json", "POST", "/sign", "nope", 422},
		{"neither shape", "POST", "/sign", `{"x":1}`, 422},
		{"unknown suite", "POST", "/sign", `{"cryptosuite":"x"}`, 422},
		{"bad base proof", "POST", "/sign",
			`{"cryptosuite":"ecdsa-sd-2023","base_proof":{}}`, 422},
		{"too large", "POST", "/sign",
			`{"snapshots":[{"body":"` + strings.Repeat("x", MaxBodyBytes) +
				`"}]}`, 413},
	}
	for _, c := range cases {
		req, _ := http.NewRequest(c.method, srv.URL+c.path,
			strings.NewReader(c.body))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != c.status || !strings.Contains(string(data),
			`"error"`) {
			t.Errorf("%s: %d %s", c.name, resp.StatusCode, data)
		}
	}
}

func TestHandlerRequiresTheSignature(t *testing.T) {
	v := loadVectors(t)
	h := Handler(New(v.keys(t)), NewVerifier(v.platformKey(t)),
		HandlerOptions{Path: "/sign"})
	srv := httptest.NewServer(h)
	defer srv.Close()
	status, out := post(t, srv.URL+"/sign", v.Documents.Request)
	if status != 401 || !strings.Contains(out["error"].(string), "signature") {
		t.Errorf("unsigned: %d %v", status, out)
	}
}
