package transpareo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Token is a bearer token and what the token endpoint said
// about it.
type Token struct {
	AccessToken string
	Scope       []string
	ExpiresAt   time.Time
}

// Valid reports whether the token can still be sent at the
// given time, with the refresh margin the client applies.
func (t *Token) Valid(now time.Time) bool {
	if t == nil || t.AccessToken == "" {
		return false
	}
	if t.ExpiresAt.IsZero() {
		return true
	}
	return now.Add(refreshMargin).Before(t.ExpiresAt)
}

// refreshMargin is how long before expiry the client fetches a
// new token instead of sending the old one.
const refreshMargin = 60 * time.Second

// TokenSource hands the client a token for each request. The
// client credentials grant is the one source today; a device
// flow or an authorization code flow would be further sources.
type TokenSource interface {
	Token(ctx context.Context) (*Token, error)
}

// invalidator is implemented by sources that can discard a
// cached token, which lets the client retry once when the API
// reports an expired token.
type invalidator interface {
	Invalidate()
}

// StaticToken is a TokenSource for a token issued elsewhere, for
// example by `transpareo auth token`. It never refreshes.
type StaticToken string

// Token implements TokenSource.
func (s StaticToken) Token(context.Context) (*Token, error) {
	if s == "" {
		return nil, &Error{Code: "TOKEN_MISSING",
			Message: "no bearer token was given"}
	}
	return &Token{AccessToken: string(s)}, nil
}

// ClientCredentials is the client id and secret of an API
// consumer, plus the optional scope to narrow tokens to.
type ClientCredentials struct {
	ID     string
	Secret string
	Scope  []string
}

// credentialsSource exchanges client credentials at the token
// endpoint and caches the result until shortly before expiry.
type credentialsSource struct {
	tokenURL string
	creds    ClientCredentials
	http     *http.Client
	now      func() time.Time

	mu    sync.Mutex
	token *Token
}

// NewClientCredentialsSource returns a TokenSource that exchanges
// the credentials at the token endpoint of host. httpClient may
// be nil.
func NewClientCredentialsSource(host string, creds ClientCredentials,
	httpClient *http.Client) (TokenSource, error) {
	base, err := parseHost(host)
	if err != nil {
		return nil, err
	}
	if httpClient == nil {
		httpClient = defaultHTTPClient()
	}
	return &credentialsSource{
		tokenURL: base.JoinPath("oauth", "token").String(),
		creds:    creds,
		http:     httpClient,
		now:      time.Now,
	}, nil
}

func (s *credentialsSource) Token(ctx context.Context) (*Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token.Valid(s.now()) {
		return s.token, nil
	}
	token, err := s.exchange(ctx)
	if err != nil {
		return nil, err
	}
	s.token = token
	return token, nil
}

func (s *credentialsSource) Invalidate() {
	s.mu.Lock()
	s.token = nil
	s.mu.Unlock()
}

// tokenResponse is the RFC 6749 token response.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
}

func (s *credentialsSource) exchange(ctx context.Context) (*Token, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", s.creds.ID)
	form.Set("client_secret", s.creds.Secret)
	if len(s.creds.Scope) > 0 {
		form.Set("scope", strings.Join(s.creds.Scope, " "))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, transportError(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, transportError(err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errorFromResponse(resp.StatusCode, resp.Header, body)
	}
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil || tr.AccessToken == "" {
		return nil, &Error{Code: CodeInvalidResponse, Status: resp.StatusCode,
			Message: "the token endpoint did not answer a token"}
	}
	if !strings.EqualFold(tr.TokenType, "Bearer") {
		return nil, &Error{Code: CodeInvalidResponse, Status: resp.StatusCode,
			Message: fmt.Sprintf("unsupported token type %q", tr.TokenType)}
	}
	token := &Token{AccessToken: tr.AccessToken}
	if tr.Scope != "" {
		token.Scope = strings.Fields(tr.Scope)
	}
	if tr.ExpiresIn > 0 {
		token.ExpiresAt = s.now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	return token, nil
}
