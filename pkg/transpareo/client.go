package transpareo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Accept is the media type that selects version 1 of the API.
const Accept = "application/vnd.api.v1+json"

// maxBodyBytes bounds what the client reads of a response body.
const maxBodyBytes = 64 << 20

// defaultUserAgent identifies the Go package; the command-line
// tool replaces it with its own name and version.
const defaultUserAgent = "transpareo-go"

// RetryPolicy says how often and how long the client retries a
// request that failed with a transport error, a 429 or a server
// error. The wait doubles per attempt, varies randomly by up to
// half its length, and never exceeds MaxWait; Retry-After on a
// 429 replaces it.
type RetryPolicy struct {
	Attempts int
	BaseWait time.Duration
	MaxWait  time.Duration
}

// DefaultRetryPolicy is three attempts starting at half a second.
var DefaultRetryPolicy = RetryPolicy{
	Attempts: 3,
	BaseWait: 500 * time.Millisecond,
	MaxWait:  8 * time.Second,
}

// Client talks to one workspace host.
type Client struct {
	base      *url.URL
	http      *http.Client
	tokens    TokenSource
	userAgent string
	retry     RetryPolicy
	logger    *slog.Logger

	now    func() time.Time
	sleep  func(context.Context, time.Duration) error
	newKey func() string
	random *rand.Rand

	apiState
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient replaces the HTTP client, for a custom transport
// or timeout.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.http = h }
}

// WithUserAgent sets the User-Agent header.
func WithUserAgent(ua string) Option {
	return func(c *Client) { c.userAgent = ua }
}

// WithRetryPolicy replaces DefaultRetryPolicy. Attempts of one
// disables retries.
func WithRetryPolicy(p RetryPolicy) Option {
	return func(c *Client) { c.retry = p }
}

// WithTokenSource replaces the token source built by New.
func WithTokenSource(ts TokenSource) Option {
	return func(c *Client) { c.tokens = ts }
}

// WithLogger reports every attempt the client repeats, at Warn:
// the method and path, the attempt number, how long the failed
// attempt took and the error it ended with. Nothing is logged
// otherwise, so a stalled request that a retry hid stays visible.
func WithLogger(logger *slog.Logger) Option {
	return func(c *Client) { c.logger = logger }
}

// New returns a client for host that authenticates with the
// client credentials grant. host is a workspace host name or
// URL, such as "acme.example.com" or "https://acme.example.com".
func New(host string, creds ClientCredentials,
	opts ...Option) (*Client, error) {
	c, err := newClient(host, opts)
	if err != nil {
		return nil, err
	}
	if c.tokens == nil {
		c.tokens, err = NewClientCredentialsSource(host, creds, c.http)
		if err != nil {
			return nil, err
		}
	}
	return c, nil
}

// NewWithToken returns a client for host that sends a token
// issued elsewhere and never refreshes it.
func NewWithToken(host, token string, opts ...Option) (*Client, error) {
	c, err := newClient(host, opts)
	if err != nil {
		return nil, err
	}
	if c.tokens == nil {
		c.tokens = StaticToken(token)
	}
	return c, nil
}

func newClient(host string, opts []Option) (*Client, error) {
	base, err := parseHost(host)
	if err != nil {
		return nil, err
	}
	c := &Client{
		base:      base,
		userAgent: defaultUserAgent,
		retry:     DefaultRetryPolicy,
		now:       time.Now,
		sleep:     sleepContext,
		newKey:    newUUID,
		random:    rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())),
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.http == nil {
		c.http = defaultHTTPClient()
	}
	return c, nil
}

// Host returns the origin the client talks to, without the /api
// path, such as "https://acme.example.com".
func (c *Client) Host() string {
	return c.base.Scheme + "://" + c.base.Host
}

// BaseURL returns the API root, such as
// "https://acme.example.com/api".
func (c *Client) BaseURL() string {
	return c.base.String()
}

// TokenSource returns the source the client authenticates with,
// for callers that need a token of their own, such as a script
// that hands one to a subprocess.
func (c *Client) TokenSource() TokenSource {
	return c.tokens
}

// parseHost turns a host name or URL into the API base URL.
// A bare name gets https; a path is dropped so that a pasted
// address of any page on the host works.
func parseHost(host string) (*url.URL, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, &Error{Code: "HOST_MISSING",
			Message: "no workspace host was given"}
	}
	if !strings.Contains(host, "://") {
		host = "https://" + host
	}
	u, err := url.Parse(host)
	if err != nil || u.Host == "" {
		return nil, &Error{Code: "HOST_INVALID",
			Message: fmt.Sprintf("%q is not a host name or URL", host)}
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, &Error{Code: "HOST_INVALID",
			Message: fmt.Sprintf("unsupported scheme %q", u.Scheme)}
	}
	return &url.URL{Scheme: u.Scheme, Host: u.Host, Path: "/api"}, nil
}

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 60 * time.Second}
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Request is one call to the API.
type Request struct {
	// Method is the HTTP method.
	Method string

	// Path is relative to the API root ("/dpps/ABC/publish") or
	// an absolute URL on the same host, as the API hands them out
	// in Link headers and statusUrl fields.
	Path string

	// Query carries the query parameters, in the snake_case the
	// API expects.
	Query url.Values

	// Body is sent as JSON unless it is nil, a []byte, a
	// json.RawMessage or an io.Reader, which are sent as they
	// are.
	Body any

	// ContentType overrides application/json for a raw Body.
	ContentType string

	// Accept overrides the API media type, for downloads.
	Accept string

	// IdempotencyKey is sent on a POST; a random UUID when empty.
	IdempotencyKey string

	// Header carries further headers.
	Header http.Header
}

// Response is what a call answered.
type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	RateLimit  RateLimit
}

// Replayed reports whether the answer is the stored response of
// an earlier request with the same Idempotency-Key.
func (r *Response) Replayed() bool {
	return r.Header.Get("Idempotent-Replayed") == "true"
}

// Decode unmarshals the body into out.
func (r *Response) Decode(out any) error {
	if out == nil || len(r.Body) == 0 {
		return nil
	}
	if err := json.Unmarshal(r.Body, out); err != nil {
		return &Error{Code: CodeInvalidResponse, Status: r.StatusCode,
			Message: "the response is not the JSON the client expected: " +
				err.Error(), cause: err}
	}
	return nil
}

// Do sends the request, retrying as the policy says, and returns
// the response of a 2xx status. Any other status becomes an
// *Error.
func (c *Client) Do(ctx context.Context, req *Request) (*Response, error) {
	target, err := c.resolve(req.Path, req.Query)
	if err != nil {
		return nil, err
	}
	body, contentType, err := encodeBody(req)
	if err != nil {
		return nil, err
	}
	key := req.IdempotencyKey
	if key == "" && req.Method == http.MethodPost {
		key = c.newKey()
	}
	resp, apiErr := c.exchange(ctx, req.Method, target, body,
		func(httpReq *http.Request, token *Token) {
			c.setHeaders(httpReq, req, token, key, contentType)
		})
	if apiErr != nil {
		return nil, apiErr
	}
	if resp.StatusCode >= 300 {
		return nil, errorFromResponse(resp.StatusCode, resp.Header, resp.Body)
	}
	return resp, nil
}

// exchange runs the attempt loop: a token per attempt, one
// refresh on an expired token, retries on transport errors, 429
// and server errors. It returns the last response whatever its
// status, or the error that ended the attempts.
func (c *Client) exchange(ctx context.Context, method, target string,
	body []byte, set func(*http.Request, *Token)) (*Response, *Error) {
	refreshed := false
	attempts := max(c.retry.Attempts, 1)
	for attempt := 1; ; attempt++ {
		started := c.now()
		resp, apiErr, step := c.attempt(ctx, method, target, body, set)
		if apiErr == nil && resp.StatusCode < 300 {
			return resp, nil
		}
		if apiErr == nil {
			apiErr = errorFromResponse(resp.StatusCode, resp.Header, resp.Body)
		}
		if apiErr.Status == http.StatusUnauthorized && !refreshed &&
			c.invalidate(apiErr) {
			refreshed = true
			continue
		}
		if !apiErr.Retryable || attempt >= attempts {
			return settle(resp, apiErr)
		}
		wait := c.backoff(attempt, apiErr)
		c.logRetry(method, target, step, attempt, c.now().Sub(started), wait,
			apiErr)
		if err := c.sleep(ctx, wait); err != nil {
			return settle(resp, apiErr)
		}
	}
}

// logRetry records the attempt that is about to be repeated: the
// request, the step that failed when it was the token exchange
// rather than the request itself, how long the attempt took, the
// error, and the wait before the next one.
func (c *Client) logRetry(method, target, step string, attempt int,
	elapsed, wait time.Duration, apiErr *Error) {
	if c.logger == nil {
		return
	}
	path := target
	if u, err := url.Parse(target); err == nil {
		path = u.Path
	}
	attrs := []any{"method", method, "path", path}
	if step != "" {
		attrs = append(attrs, "during", step)
	}
	attrs = append(attrs, "attempt", attempt,
		"elapsed", elapsed.Round(time.Millisecond), "code", apiErr.Code,
		"status", apiErr.Status, "wait", wait.Round(time.Millisecond))
	c.logger.Warn("retrying request", attrs...)
}

// settle ends the attempts: a refused status travels back as the
// response itself, an error without a response as the error.
func settle(resp *Response, apiErr *Error) (*Response, *Error) {
	if resp != nil {
		return resp, nil
	}
	return nil, apiErr
}

// stepTokenExchange names the token exchange in a retry log, so a
// refusal of the exchange is not read as one of the request.
const stepTokenExchange = "token exchange"

// attempt fetches a token and sends the request once. A token
// endpoint failure is returned like any other, so a transient
// one is retried by the same policy; the step then says where
// the failure came from.
func (c *Client) attempt(ctx context.Context, method, target string,
	body []byte, set func(*http.Request, *Token)) (*Response, *Error,
	string) {
	token, err := c.tokens.Token(ctx)
	if err != nil {
		var apiErr *Error
		if errors.As(err, &apiErr) {
			return nil, apiErr, stepTokenExchange
		}
		return nil, &Error{Code: "TOKEN_ERROR", Message: err.Error(),
			cause: err}, stepTokenExchange
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, target,
		bytes.NewReader(body))
	if err != nil {
		return nil, &Error{Code: "REQUEST_INVALID", Message: err.Error(),
			cause: err}, ""
	}
	set(httpReq, token)
	resp, apiErr := c.send(httpReq)
	return resp, apiErr, ""
}

// invalidate discards the cached token when the API reports it
// expired and the source can mint another. It reports whether a
// retry makes sense.
func (c *Client) invalidate(apiErr *Error) bool {
	if apiErr.Code != "TOKEN_EXPIRED" {
		return false
	}
	inv, ok := c.tokens.(invalidator)
	if !ok {
		return false
	}
	inv.Invalidate()
	return true
}

func (c *Client) setHeaders(httpReq *http.Request, req *Request,
	token *Token, key, contentType string) {
	httpReq.Header.Set("Accept", Accept)
	httpReq.Header.Set("Authorization", "Bearer "+token.AccessToken)
	httpReq.Header.Set("User-Agent", c.userAgent)
	if req.Accept != "" {
		httpReq.Header.Set("Accept", req.Accept)
	}
	if contentType != "" {
		httpReq.Header.Set("Content-Type", contentType)
	}
	if key != "" {
		httpReq.Header.Set("Idempotency-Key", key)
	}
	for name, values := range req.Header {
		httpReq.Header[http.CanonicalHeaderKey(name)] = values
	}
}

func (c *Client) send(httpReq *http.Request) (*Response, *Error) {
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, transportError(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, transportError(err)
	}
	return &Response{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       body,
		RateLimit:  parseRateLimit(resp.Header),
	}, nil
}

// backoff is the wait before the next attempt: Retry-After when
// the API sent one, else the policy's growing, jittered wait.
func (c *Client) backoff(attempt int, apiErr *Error) time.Duration {
	if apiErr.RetryAfter > 0 {
		return apiErr.RetryAfter
	}
	wait := c.retry.BaseWait << (attempt - 1)
	if wait > c.retry.MaxWait || wait <= 0 {
		wait = c.retry.MaxWait
	}
	jitter := 0.5 + c.random.Float64()
	return time.Duration(float64(wait) * jitter)
}

// resolve joins a relative path onto the API root, or accepts an
// absolute URL on the client's own host.
func (c *Client) resolve(path string, query url.Values) (string, error) {
	var u *url.URL
	if strings.Contains(path, "://") {
		parsed, err := url.Parse(path)
		if err != nil {
			return "", &Error{Code: "REQUEST_INVALID", Message: err.Error()}
		}
		if parsed.Scheme != c.base.Scheme || parsed.Host != c.base.Host {
			return "", &Error{Code: CodeHostMismatch,
				Message: fmt.Sprintf("%s is not on %s", path, c.Host()),
				Hint: "The client only sends its token to the host it was " +
					"created for."}
		}
		u = parsed
	} else {
		path, rawQuery, _ := strings.Cut(path, "?")
		u = c.base.JoinPath(strings.TrimPrefix(path, "/api/"))
		u.RawQuery = rawQuery
	}
	if len(query) > 0 {
		existing := u.Query()
		for name, values := range query {
			existing[name] = values
		}
		u.RawQuery = existing.Encode()
	}
	return u.String(), nil
}

func encodeBody(req *Request) ([]byte, string, error) {
	contentType := req.ContentType
	switch body := req.Body.(type) {
	case nil:
		return nil, "", nil
	case []byte:
		return body, orJSON(contentType), nil
	case json.RawMessage:
		return body, orJSON(contentType), nil
	case io.Reader:
		data, err := io.ReadAll(body)
		if err != nil {
			return nil, "", &Error{Code: "REQUEST_INVALID",
				Message: "reading the request body: " + err.Error(), cause: err}
		}
		return data, orJSON(contentType), nil
	default:
		data, err := json.Marshal(body)
		if err != nil {
			return nil, "", &Error{Code: "REQUEST_INVALID",
				Message: "encoding the request body: " + err.Error(),
				cause:   err}
		}
		return data, "application/json", nil
	}
}

func orJSON(contentType string) string {
	if contentType == "" {
		return "application/json"
	}
	return contentType
}

// transportError classifies an error from the HTTP client.
func transportError(err error) *Error {
	code := CodeNetwork
	message := err.Error()
	var netErr net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		code = CodeTimeout
		message = "the request exceeded its deadline"
	case errors.As(err, &netErr) && netErr.Timeout():
		code = CodeTimeout
		message = "the request timed out"
	case errors.Is(err, context.Canceled):
		message = "the request was cancelled"
	}
	return &Error{
		Code:      code,
		Message:   message,
		Retryable: !errors.Is(err, context.Canceled),
		cause:     err,
	}
}

// Get sends a GET and decodes the answer into out.
func (c *Client) Get(ctx context.Context, path string, query url.Values,
	out any) (*Response, error) {
	return c.JSON(ctx, &Request{Method: http.MethodGet, Path: path,
		Query: query}, out)
}

// Post sends a POST with body as JSON and decodes the answer.
func (c *Client) Post(ctx context.Context, path string, body,
	out any) (*Response, error) {
	return c.JSON(ctx, &Request{Method: http.MethodPost, Path: path,
		Body: body}, out)
}

// Put sends a PUT with body as JSON and decodes the answer.
func (c *Client) Put(ctx context.Context, path string, body,
	out any) (*Response, error) {
	return c.JSON(ctx, &Request{Method: http.MethodPut, Path: path,
		Body: body}, out)
}

// Patch sends a PATCH with body as JSON and decodes the answer.
func (c *Client) Patch(ctx context.Context, path string, body,
	out any) (*Response, error) {
	return c.JSON(ctx, &Request{Method: http.MethodPatch, Path: path,
		Body: body}, out)
}

// Delete sends a DELETE and decodes the answer, if any.
func (c *Client) Delete(ctx context.Context, path string,
	out any) (*Response, error) {
	return c.JSON(ctx, &Request{Method: http.MethodDelete, Path: path}, out)
}

// JSON sends the request and decodes a JSON answer into out,
// which may be nil.
func (c *Client) JSON(ctx context.Context, req *Request,
	out any) (*Response, error) {
	resp, err := c.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := resp.Decode(out); err != nil {
		return resp, err
	}
	return resp, nil
}

// Download streams the answer of a GET to w, for archives that
// should not sit in memory. It authenticates like every request
// and refuses a different host, but does not retry once bytes
// have flowed.
func (c *Client) Download(ctx context.Context, path string, w io.Writer) (int64,
	error) {
	target, err := c.resolve(path, nil)
	if err != nil {
		return 0, err
	}
	token, err := c.tokens.Token(ctx)
	if err != nil {
		return 0, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, &Error{Code: "REQUEST_INVALID", Message: err.Error(),
			cause: err}
	}
	httpReq.Header.Set("Authorization", "Bearer "+token.AccessToken)
	httpReq.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return 0, transportError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return 0, errorFromResponse(resp.StatusCode, resp.Header, body)
	}
	n, err := io.Copy(w, resp.Body)
	if err != nil {
		return n, transportError(err)
	}
	return n, nil
}
