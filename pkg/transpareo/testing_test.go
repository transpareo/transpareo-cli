package transpareo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// fakeClock is a settable time source plus a sleep that records
// every wait instead of waiting.
type fakeClock struct {
	mu    sync.Mutex
	now   time.Time
	waits []time.Duration
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)}
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	f.now = f.now.Add(d)
	f.mu.Unlock()
}

func (f *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	f.mu.Lock()
	f.waits = append(f.waits, d)
	f.now = f.now.Add(d)
	f.mu.Unlock()
	return ctx.Err()
}

// tokenServer is a test server with a token endpoint that hands
// out numbered tokens, and a mux for further routes.
type tokenServer struct {
	*httptest.Server
	Mux       *http.ServeMux
	mu        sync.Mutex
	exchanges int
	lastForm  map[string]string
}

func newTokenServer(t *testing.T) *tokenServer {
	t.Helper()
	ts := &tokenServer{Mux: http.NewServeMux()}
	ts.Mux.HandleFunc("POST /api/oauth/token", func(w http.ResponseWriter,
		r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("token form: %v", err)
		}
		ts.mu.Lock()
		ts.exchanges++
		n := ts.exchanges
		ts.lastForm = map[string]string{}
		for k := range r.PostForm {
			ts.lastForm[k] = r.PostForm.Get(k)
		}
		ts.mu.Unlock()
		if r.PostForm.Get("client_secret") != "s3cret" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{
				"error":             "invalid_client",
				"error_description": "Client authentication failed."})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok" + string(rune('0'+n)),
			"token_type":   "Bearer",
			"expires_in":   3600,
			"scope":        r.PostForm.Get("scope"),
		})
	})
	ts.Server = httptest.NewServer(ts.Mux)
	t.Cleanup(ts.Close)
	return ts
}

func (ts *tokenServer) Exchanges() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.exchanges
}

// newTestClient wires a client to the server with the fake clock.
func newTestClient(t *testing.T, ts *tokenServer, clock *fakeClock,
	opts ...Option) *Client {
	t.Helper()
	creds := ClientCredentials{ID: "id", Secret: "s3cret"}
	c, err := New(ts.URL, creds, opts...)
	if err != nil {
		t.Fatal(err)
	}
	c.now = clock.Now
	c.sleep = clock.Sleep
	if src, ok := c.tokens.(*credentialsSource); ok {
		src.now = clock.Now
	}
	return c
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
