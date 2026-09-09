package transpareo

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestTokenExchangeSendsFormFields(t *testing.T) {
	ts := newTokenServer(t)
	clock := newFakeClock()
	src, err := NewClientCredentialsSource(ts.URL,
		ClientCredentials{ID: "id", Secret: "s3cret",
			Scope: []string{"dpp_read", "dpp_write"}},
		ts.Client())
	if err != nil {
		t.Fatal(err)
	}
	src.(*credentialsSource).now = clock.Now
	token, err := src.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "tok1" {
		t.Errorf("token = %q", token.AccessToken)
	}
	want := clock.Now().Add(time.Hour)
	if !token.ExpiresAt.Equal(want) {
		t.Errorf("expires at %v, want %v", token.ExpiresAt, want)
	}
	if len(token.Scope) != 2 || token.Scope[1] != "dpp_write" {
		t.Errorf("scope = %v", token.Scope)
	}
	form := ts.lastForm
	if form["grant_type"] != "client_credentials" ||
		form["client_id"] != "id" ||
		form["client_secret"] != "s3cret" ||
		form["scope"] != "dpp_read dpp_write" {
		t.Errorf("form = %v", form)
	}
}

func TestTokenIsCachedUntilSixtySecondsBeforeExpiry(t *testing.T) {
	ts := newTokenServer(t)
	clock := newFakeClock()
	c := newTestClient(t, ts, clock)
	ctx := context.Background()
	for range 3 {
		if _, err := c.tokens.Token(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if ts.Exchanges() != 1 {
		t.Fatalf("exchanges = %d, want 1", ts.Exchanges())
	}
	clock.Advance(59*time.Minute - time.Second)
	token, _ := c.tokens.Token(ctx)
	if token.AccessToken != "tok1" || ts.Exchanges() != 1 {
		t.Fatalf("refreshed too early: %q after %d exchanges",
			token.AccessToken, ts.Exchanges())
	}
	clock.Advance(2 * time.Second)
	token, _ = c.tokens.Token(ctx)
	if token.AccessToken != "tok2" || ts.Exchanges() != 2 {
		t.Fatalf("not refreshed: %q after %d exchanges",
			token.AccessToken, ts.Exchanges())
	}
}

func TestTokenExchangeErrorIsRFC6749Shaped(t *testing.T) {
	ts := newTokenServer(t)
	src, _ := NewClientCredentialsSource(ts.URL,
		ClientCredentials{ID: "id", Secret: "wrong"}, ts.Client())
	_, err := src.Token(context.Background())
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %T %v", err, err)
	}
	if apiErr.Code != "invalid_client" ||
		apiErr.Status != http.StatusUnauthorized {
		t.Errorf("error = %+v", apiErr)
	}
	if apiErr.Message != "Client authentication failed." {
		t.Errorf("message = %q", apiErr.Message)
	}
	if apiErr.Retryable {
		t.Error("a wrong secret must not be retryable")
	}
}

func TestStaticTokenNeverRefreshes(t *testing.T) {
	src := StaticToken("abc")
	token, err := src.Token(context.Background())
	if err != nil || token.AccessToken != "abc" || !token.ExpiresAt.IsZero() {
		t.Errorf("token = %+v, err = %v", token, err)
	}
	if _, err := StaticToken("").Token(context.Background()); err == nil {
		t.Error("an empty token must be refused")
	}
	if _, ok := any(src).(invalidator); ok {
		t.Error("a static token cannot be invalidated")
	}
}

func TestTokenValid(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name  string
		token *Token
		want  bool
	}{
		{"nil", nil, false},
		{"empty", &Token{}, false},
		{"no expiry", &Token{AccessToken: "x"}, true},
		{"fresh", &Token{AccessToken: "x", ExpiresAt: now.Add(time.Hour)},
			true},
		{"within margin", &Token{AccessToken: "x",
			ExpiresAt: now.Add(30 * time.Second)}, false},
		{"expired", &Token{AccessToken: "x", ExpiresAt: now.Add(-time.Second)},
			false},
	}
	for _, tc := range cases {
		if got := tc.token.Valid(now); got != tc.want {
			t.Errorf("%s: valid = %v, want %v", tc.name, got, tc.want)
		}
	}
}
