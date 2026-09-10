package transpareo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseHost(t *testing.T) {
	cases := []struct {
		in, want, code string
	}{
		{"acme.example.com", "https://acme.example.com/api", ""},
		{"https://acme.example.com", "https://acme.example.com/api", ""},
		{"https://acme.example.com/", "https://acme.example.com/api", ""},
		{"https://acme.example.com/apidocs", "https://acme.example.com/api",
			""},
		{"http://localhost:3000", "http://localhost:3000/api", ""},
		{" acme.example.com ", "https://acme.example.com/api", ""},
		{"", "", "HOST_MISSING"},
		{"ftp://acme.example.com", "", "HOST_INVALID"},
		{"https://", "", "HOST_INVALID"},
	}
	for _, tc := range cases {
		u, err := parseHost(tc.in)
		if tc.code != "" {
			if !IsCode(err, tc.code) {
				t.Errorf("%q: err = %v, want code %s", tc.in, err, tc.code)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", tc.in, err)
			continue
		}
		if u.String() != tc.want {
			t.Errorf("%q: got %s, want %s", tc.in, u, tc.want)
		}
	}
}

func TestDoSetsHeaders(t *testing.T) {
	ts := newTokenServer(t)
	var got http.Header
	ts.Mux.HandleFunc("/api/dpps", func(w http.ResponseWriter,
		r *http.Request) {
		got = r.Header.Clone()
		got.Set("X-Method", r.Method)
		writeJSON(w, 201, map[string]string{"code": "X"})
	})
	c := newTestClient(t, ts, newFakeClock(), WithUserAgent("test/1"))
	ctx := context.Background()

	resp, err := c.Post(ctx, "/dpps", map[string]string{"a": "b"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		t.Errorf("status = %d", resp.StatusCode)
	}
	if got.Get("Accept") != Accept {
		t.Errorf("Accept = %q", got.Get("Accept"))
	}
	if got.Get("Authorization") != "Bearer tok1" {
		t.Errorf("Authorization = %q", got.Get("Authorization"))
	}
	if got.Get("User-Agent") != "test/1" {
		t.Errorf("User-Agent = %q", got.Get("User-Agent"))
	}
	if got.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", got.Get("Content-Type"))
	}
	key := got.Get("Idempotency-Key")
	if len(key) != 36 || strings.Count(key, "-") != 4 {
		t.Errorf("Idempotency-Key = %q, want a UUID", key)
	}

	if _, err := c.Get(ctx, "/dpps", nil, nil); err != nil {
		t.Fatal(err)
	}
	if got.Get("Idempotency-Key") != "" {
		t.Error("a GET must not carry an Idempotency-Key")
	}

	_, err = c.Do(ctx, &Request{Method: "POST", Path: "/dpps",
		IdempotencyKey: "create-000412"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Get("Idempotency-Key") != "create-000412" {
		t.Errorf("a caller's key was replaced: %q", got.Get("Idempotency-Key"))
	}
}

func TestDoRetriesServerErrorsWithGrowingWaits(t *testing.T) {
	ts := newTokenServer(t)
	var calls atomic.Int32
	ts.Mux.HandleFunc("/api/dpps", func(w http.ResponseWriter,
		r *http.Request) {
		if calls.Add(1) < 3 {
			writeJSON(w, 503, map[string]string{"error": "UPSTREAM",
				"message": "down"})
			return
		}
		writeJSON(w, 200, []any{})
	})
	clock := newFakeClock()
	c := newTestClient(t, ts, clock)
	if _, err := c.Post(context.Background(), "/dpps", nil, nil); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Errorf("calls = %d, want 3", calls.Load())
	}
	if len(clock.waits) != 2 {
		t.Fatalf("waits = %v, want two", clock.waits)
	}
	within := func(d, base time.Duration) bool {
		return d >= base/2 && d < base*3/2
	}
	if !within(clock.waits[0], 500*time.Millisecond) ||
		!within(clock.waits[1], time.Second) {
		t.Errorf("waits = %v, want jittered 500ms then 1s", clock.waits)
	}
}

func TestDoGivesUpAfterThreeAttempts(t *testing.T) {
	ts := newTokenServer(t)
	var calls atomic.Int32
	ts.Mux.HandleFunc("/api/dpps", func(w http.ResponseWriter,
		r *http.Request) {
		calls.Add(1)
		writeJSON(w, 502, map[string]string{"error": "BAD_GATEWAY",
			"message": "no"})
	})
	c := newTestClient(t, ts, newFakeClock())
	_, err := c.Get(context.Background(), "/dpps", nil, nil)
	if !IsCode(err, "BAD_GATEWAY") {
		t.Fatalf("err = %v", err)
	}
	if calls.Load() != 3 {
		t.Errorf("calls = %d, want 3", calls.Load())
	}
	var apiErr *Error
	errors.As(err, &apiErr)
	if !apiErr.Retryable || apiErr.Status != 502 {
		t.Errorf("error = %+v", apiErr)
	}
}

// TestDoLogsTheAttemptItRepeats proves the logger names the
// call, the attempt, how long it took and why it is repeated,
// and stays quiet on the attempt that succeeds.
func TestDoLogsTheAttemptItRepeats(t *testing.T) {
	ts := newTokenServer(t)
	clock := newFakeClock()
	var calls atomic.Int32
	ts.Mux.HandleFunc("/api/products", func(w http.ResponseWriter,
		r *http.Request) {
		if calls.Add(1) == 1 {
			clock.Advance(61 * time.Second)
			writeJSON(w, 503, map[string]string{"error": "UPSTREAM",
				"message": "down"})
			return
		}
		writeJSON(w, 201, map[string]any{"id": 1})
	})
	var log bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&log, nil))
	c := newTestClient(t, ts, clock, WithLogger(logger))
	if _, err := c.Post(context.Background(), "/products", nil, nil); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(log.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("log = %q, want one line", log.String())
	}
	for _, want := range []string{"level=WARN", "method=POST",
		"path=/api/products", "attempt=1", "elapsed=1m1s", "code=UPSTREAM",
		"status=503", "wait="} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("log %q lacks %q", lines[0], want)
		}
	}
	if strings.Contains(lines[0], "during=") {
		t.Errorf("log %q names a step for a failure of the request itself",
			lines[0])
	}
}

// TestDoLogsAThrottledTokenExchange proves a 429 from the token
// endpoint is logged as the exchange, with the Retry-After it
// waits, and not as a refusal of the request that needed it.
func TestDoLogsAThrottledTokenExchange(t *testing.T) {
	ts := newTokenServer(t)
	throttle := http.NewServeMux()
	var exchanges atomic.Int32
	throttle.HandleFunc("POST /api/oauth/token", func(w http.ResponseWriter,
		r *http.Request) {
		if exchanges.Add(1) == 1 {
			w.Header().Set("Retry-After", "60")
			writeJSON(w, 429, map[string]string{"error": "TOO_MANY_REQUESTS",
				"message": "slow down"})
			return
		}
		ts.Mux.ServeHTTP(w, r)
	})
	throttle.Handle("/", ts.Mux)
	ts.Config.Handler = throttle
	ts.Mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"key": "id"})
	})
	var log bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&log, nil))
	clock := newFakeClock()
	c := newTestClient(t, ts, clock, WithLogger(logger))
	if _, err := c.Get(context.Background(), "/me", nil, nil); err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(log.String())
	for _, want := range []string{"method=GET", "path=/api/me",
		`during="token exchange"`, "code=TOO_MANY_REQUESTS", "status=429",
		"wait=1m0s"} {
		if !strings.Contains(line, want) {
			t.Errorf("log %q lacks %q", line, want)
		}
	}
	if len(clock.waits) != 1 || clock.waits[0] != time.Minute {
		t.Errorf("waits = %v, want the Retry-After minute", clock.waits)
	}
}

func TestDoHonoursRetryAfterOn429(t *testing.T) {
	ts := newTokenServer(t)
	var calls atomic.Int32
	ts.Mux.HandleFunc("/api/dpps", func(w http.ResponseWriter,
		r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "7")
			w.Header().Set("X-RateLimit-Limit", "1000")
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", "1788944000")
			writeJSON(w, 429,
				map[string]string{"error": "CONSUMER_RATE_LIMITED",
					"message": "Rate limit exceeded.", "hint": "Wait."})
			return
		}
		writeJSON(w, 200, []any{})
	})
	clock := newFakeClock()
	c := newTestClient(t, ts, clock)
	if _, err := c.Get(context.Background(), "/dpps", nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(clock.waits) != 1 || clock.waits[0] != 7*time.Second {
		t.Errorf("waits = %v, want [7s]", clock.waits)
	}
}

func TestDoDoesNotRetryClientErrors(t *testing.T) {
	ts := newTokenServer(t)
	var calls atomic.Int32
	ts.Mux.HandleFunc("/api/dpps", func(w http.ResponseWriter,
		r *http.Request) {
		calls.Add(1)
		w.Header().Set("X-RateLimit-Limit", "1000")
		w.Header().Set("X-RateLimit-Remaining", "998")
		w.Header().Set("X-RateLimit-Reset", "1788944000")
		writeJSON(w, 422, map[string]any{
			"error":   "DPP_INVALID",
			"message": "Validation failed",
			"hint":    "Fix the fields and try again.",
			"docsUrl": "https://acme.example.com/apidocs/guide#writing-a-passport",
			"fields": map[string]any{
				"unlockableId": map[string]any{"error": "blank",
					"message":     "is required",
					"fullMessage": "Product is required"},
				"properties": map[string]any{"missing": []string{"Weight"}},
				"legacy":     "is odd",
				"older":      []string{"one", "two"},
			},
		})
	})
	c := newTestClient(t, ts, newFakeClock())
	_, err := c.Post(context.Background(), "/dpps", map[string]any{}, nil)
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %T", err)
	}
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want 1", calls.Load())
	}
	if apiErr.Retryable || apiErr.Status != 422 ||
		apiErr.Code != "DPP_INVALID" {
		t.Errorf("error = %+v", apiErr)
	}
	if apiErr.Hint != "Fix the fields and try again." {
		t.Errorf("hint = %q", apiErr.Hint)
	}
	if !strings.HasSuffix(apiErr.DocsURL, "#writing-a-passport") {
		t.Errorf("docsUrl = %q", apiErr.DocsURL)
	}
	if apiErr.Fields["unlockableId"].FullMessage != "Product is required" {
		t.Errorf("fields = %+v", apiErr.Fields)
	}
	if apiErr.Fields["properties"].Missing[0] != "Weight" {
		t.Errorf("missing = %v", apiErr.Fields["properties"].Missing)
	}
	if apiErr.Fields["legacy"].Message != "is odd" ||
		apiErr.Fields["older"].Message != "one, two" {
		t.Errorf("lenient fields = %+v", apiErr.Fields)
	}
	if apiErr.RateLimit.Limit != 1000 || apiErr.RateLimit.Remaining != 998 ||
		apiErr.RateLimit.Reset.Unix() != 1788944000 {
		t.Errorf("rate limit = %+v", apiErr.RateLimit)
	}
	want := "DPP_INVALID: Validation failed\nFix the fields and try again."
	if apiErr.Error() != want {
		t.Errorf("text = %q", apiErr.Error())
	}
}

func TestDoRefreshesOnceOnExpiredToken(t *testing.T) {
	ts := newTokenServer(t)
	var seen []string
	ts.Mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		seen = append(seen, auth)
		if auth == "Bearer tok1" {
			writeJSON(w, 401, map[string]string{"error": "TOKEN_EXPIRED",
				"message": "Token expired"})
			return
		}
		writeJSON(w, 200, map[string]string{"name": "ERP"})
	})
	c := newTestClient(t, ts, newFakeClock())
	me, err := c.Me(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if me.Name != "ERP" {
		t.Errorf("me = %+v", me)
	}
	if len(seen) != 2 || seen[1] != "Bearer tok2" {
		t.Errorf("authorizations = %v", seen)
	}
	if ts.Exchanges() != 2 {
		t.Errorf("exchanges = %d, want 2", ts.Exchanges())
	}
}

func TestDoRefreshesOnlyOnce(t *testing.T) {
	ts := newTokenServer(t)
	var calls atomic.Int32
	ts.Mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeJSON(w, 401, map[string]string{"error": "TOKEN_EXPIRED",
			"message": "Token expired"})
	})
	c := newTestClient(t, ts, newFakeClock())
	_, err := c.Me(context.Background())
	if !IsCode(err, "TOKEN_EXPIRED") {
		t.Fatalf("err = %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want 2", calls.Load())
	}
}

func TestDoDoesNotRefreshAStaticToken(t *testing.T) {
	ts := newTokenServer(t)
	var calls atomic.Int32
	ts.Mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeJSON(w, 401, map[string]string{"error": "TOKEN_EXPIRED",
			"message": "Token expired"})
	})
	c, err := NewWithToken(ts.URL, "old")
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Me(context.Background())
	if !IsCode(err, "TOKEN_EXPIRED") || calls.Load() != 1 {
		t.Errorf("err = %v, calls = %d", err, calls.Load())
	}
}

func TestDoRefusesOtherHosts(t *testing.T) {
	ts := newTokenServer(t)
	c := newTestClient(t, ts, newFakeClock())
	_, err := c.Get(context.Background(),
		"https://other.example.com/api/exports/1", nil, nil)
	if !IsCode(err, CodeHostMismatch) {
		t.Fatalf("err = %v", err)
	}
}

func TestDoAcceptsAbsoluteURLsOnOwnHost(t *testing.T) {
	ts := newTokenServer(t)
	var path string
	ts.Mux.HandleFunc("/api/exports/1", func(w http.ResponseWriter,
		r *http.Request) {
		path = r.URL.RequestURI()
		writeJSON(w, 200, map[string]string{"status": "completed"})
	})
	c := newTestClient(t, ts, newFakeClock())
	_, err := c.Get(context.Background(), ts.URL+"/api/exports/1?x=1",
		map[string][]string{"reload": {"1"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if path != "/api/exports/1?reload=1&x=1" {
		t.Errorf("path = %q", path)
	}
}

func TestDoKeepsAQueryWrittenIntoThePath(t *testing.T) {
	ts := newTokenServer(t)
	var uri string
	ts.Mux.HandleFunc("/api/dpps", func(w http.ResponseWriter, r *http.Request) {
		uri = r.URL.RequestURI()
		writeJSON(w, 200, []any{})
	})
	c := newTestClient(t, ts, newFakeClock())
	query := map[string][]string{"per_page": {"5"}}
	_, err := c.Get(context.Background(), "/dpps?page=2", query, nil)
	if err != nil {
		t.Fatal(err)
	}
	if uri != "/api/dpps?page=2&per_page=5" {
		t.Errorf("uri = %q", uri)
	}
}

func TestDoWrapsNonJSONErrors(t *testing.T) {
	ts := newTokenServer(t)
	ts.Mux.HandleFunc("/api/dpps", func(w http.ResponseWriter,
		r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte("<html>gone</html>"))
	})
	c := newTestClient(t, ts, newFakeClock())
	_, err := c.Get(context.Background(), "/dpps", nil, nil)
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != CodeInvalidResponse ||
		apiErr.Status != 404 {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(apiErr.Message, "HTTP 404") {
		t.Errorf("message = %q", apiErr.Message)
	}
}

func TestDoClassifiesTransportErrors(t *testing.T) {
	ts := newTokenServer(t)
	ts.Mux.HandleFunc("/api/slow", func(w http.ResponseWriter,
		r *http.Request) {
		<-r.Context().Done()
	})
	clock := newFakeClock()
	c := newTestClient(t, ts, clock, WithRetryPolicy(RetryPolicy{Attempts: 1}))
	ctx, cancel := context.WithTimeout(context.Background(),
		50*time.Millisecond)
	defer cancel()
	_, err := c.Get(ctx, "/slow", nil, nil)
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != CodeTimeout {
		t.Fatalf("err = %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Error("the deadline error must be unwrappable")
	}

	ts.Close()
	_, err = c.Get(context.Background(), "/dpps", nil, nil)
	if !errors.As(err, &apiErr) || apiErr.Code != CodeNetwork ||
		!apiErr.Retryable {
		t.Fatalf("err = %v", err)
	}
}

func TestDoRetriesATransientTokenExchange(t *testing.T) {
	ts := newTokenServer(t)
	var tokenCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/oauth/token", func(w http.ResponseWriter,
		r *http.Request) {
		if tokenCalls.Add(1) == 1 {
			w.Header().Set("Retry-After", "2")
			writeJSON(w, 429, map[string]string{"error": "slow_down",
				"error_description": "Rate limit exceeded."})
			return
		}
		writeJSON(w, 200, map[string]any{"access_token": "t",
			"token_type": "Bearer",
			"expires_in": 3600})
	})
	mux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"name": "ok"})
	})
	ts.Server.Config.Handler = mux
	clock := newFakeClock()
	c := newTestClient(t, ts, clock)
	if _, err := c.Me(context.Background()); err != nil {
		t.Fatal(err)
	}
	if tokenCalls.Load() != 2 || len(clock.waits) != 1 ||
		clock.waits[0] != 2*time.Second {
		t.Errorf("token calls = %d, waits = %v", tokenCalls.Load(), clock.waits)
	}
}

func TestResponseReplayedAndDecode(t *testing.T) {
	ts := newTokenServer(t)
	ts.Mux.HandleFunc("/api/dpps", func(w http.ResponseWriter,
		r *http.Request) {
		w.Header().Set("Idempotent-Replayed", "true")
		writeJSON(w, 201, map[string]string{"code": "ABC"})
	})
	c := newTestClient(t, ts, newFakeClock())
	var out struct {
		Code string `json:"code"`
	}
	resp, err := c.Post(context.Background(), "/dpps", nil, &out)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Replayed() || out.Code != "ABC" {
		t.Errorf("replayed = %v, out = %+v", resp.Replayed(), out)
	}
	var wrong int
	if err := resp.Decode(&wrong); !IsCode(err, CodeInvalidResponse) {
		t.Errorf("decode into the wrong type: %v", err)
	}
}

func TestEncodeBodyVariants(t *testing.T) {
	raw := json.RawMessage(`{"a":1}`)
	cases := []struct {
		name        string
		req         *Request
		wantBody    string
		contentType string
	}{
		{"nil", &Request{}, "", ""},
		{"bytes", &Request{Body: []byte("x=1"), ContentType: "text/plain"},
			"x=1", "text/plain"},
		{"raw", &Request{Body: raw}, `{"a":1}`, "application/json"},
		{"reader", &Request{Body: strings.NewReader("{}")}, "{}",
			"application/json"},
		{"value", &Request{Body: map[string]int{"n": 1}}, `{"n":1}`,
			"application/json"},
	}
	for _, tc := range cases {
		body, ct, err := encodeBody(tc.req)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if string(body) != tc.wantBody || ct != tc.contentType {
			t.Errorf("%s: body %q type %q", tc.name, body, ct)
		}
	}
	if _, _, err := encodeBody(&Request{Body: make(chan int)}); err == nil {
		t.Error("an unencodable body must fail")
	}
}
