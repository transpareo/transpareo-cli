package transpareo

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Error codes the client itself produces when no API response
// exists to take one from.
const (
	// CodeNetwork is set when the request never reached the host
	// or the connection failed before a response arrived.
	CodeNetwork = "NETWORK_ERROR"

	// CodeTimeout is set when the request exceeded its deadline.
	CodeTimeout = "TIMEOUT"

	// CodeInvalidResponse is set when the host answered with a
	// body that is not the documented error envelope.
	CodeInvalidResponse = "INVALID_RESPONSE"

	// CodeHostMismatch is set when a URL taken from a response
	// (a Link header, a statusUrl) points at a different host
	// than the client's. The bearer token is never sent there.
	CodeHostMismatch = "HOST_MISMATCH"
)

// Error is the error a failed call returns. It carries the
// documented error envelope of the API plus the rate-limit
// headers of the response. Its text is "CODE: message" with the
// hint on the next line, which is what a person or a model
// should read first.
type Error struct {
	// Code is the stable machine-readable code (PRODUCT_NOT_FOUND,
	// TOKEN_EXPIRED). Token endpoint errors carry the RFC 6749
	// code (invalid_client). Client-side failures carry one of
	// the Code* constants.
	Code string

	// Status is the HTTP status of the response, or zero when no
	// response arrived.
	Status int

	// Message is the human-readable text the platform sent.
	Message string

	// Hint says what to do next, when the platform has advice.
	Hint string

	// DocsURL is the guide section that explains the refusal.
	DocsURL string

	// Fields carries one entry per attribute on a validation
	// failure.
	Fields map[string]FieldError

	// Retryable is true for rate limits, server errors and
	// transport failures, false for everything the caller has to
	// change first.
	Retryable bool

	// RateLimit mirrors the X-RateLimit-* headers of the response.
	RateLimit RateLimit

	// RetryAfter is the wait a 429 asked for, or zero.
	RetryAfter time.Duration

	cause error
}

// FieldError is one entry of Error.Fields. Whatever the
// validation recorded besides its message rides along.
type FieldError struct {
	Error       string   `json:"error,omitempty"`
	Message     string   `json:"message,omitempty"`
	FullMessage string   `json:"fullMessage,omitempty"`
	Field       string   `json:"field,omitempty"`
	Missing     []string `json:"missing,omitempty"`
}

// UnmarshalJSON accepts the documented object, a bare string and
// a list of strings, so an older endpoint that answers messages
// only still yields a usable entry.
func (f *FieldError) UnmarshalJSON(data []byte) error {
	type plain FieldError
	var obj plain
	if err := json.Unmarshal(data, &obj); err == nil {
		*f = FieldError(obj)
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*f = FieldError{Message: text}
		return nil
	}
	var list []string
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	*f = FieldError{Message: strings.Join(list, ", ")}
	return nil
}

// RateLimit mirrors the rate-limit headers of a response. Reset
// is the end of the current window.
type RateLimit struct {
	Limit     int
	Remaining int
	Reset     time.Time
}

func (e *Error) Error() string {
	text := e.Code
	if e.Message != "" {
		text += ": " + e.Message
	}
	if e.Hint != "" {
		text += "\n" + e.Hint
	}
	return text
}

// Unwrap returns the transport error behind a client-side
// failure, so errors.Is works against context and net errors.
func (e *Error) Unwrap() error {
	return e.cause
}

// IsCode reports whether err is an *Error carrying code.
func IsCode(err error, code string) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.Code == code
}

// errorEnvelope is the documented error body.
type errorEnvelope struct {
	Error   string                `json:"error"`
	Message string                `json:"message"`
	Hint    string                `json:"hint"`
	DocsURL string                `json:"docsUrl"`
	Fields  map[string]FieldError `json:"fields"`

	// The token endpoint answers the RFC 6749 shape instead.
	ErrorDescription string `json:"error_description"`
}

// errorFromResponse builds the *Error for a non-2xx response.
func errorFromResponse(status int, header http.Header, body []byte) *Error {
	e := &Error{
		Status:     status,
		Retryable:  status == http.StatusTooManyRequests || status >= 500,
		RateLimit:  parseRateLimit(header),
		RetryAfter: parseRetryAfter(header),
	}
	var env errorEnvelope
	if err := json.Unmarshal(body, &env); err != nil || env.Error == "" {
		e.Code = CodeInvalidResponse
		e.Message = fmt.Sprintf("HTTP %d %s", status,
			http.StatusText(status))
		if len(body) > 0 && len(body) < 200 && err != nil {
			e.Message += ": " + strings.TrimSpace(string(body))
		}
		return e
	}
	e.Code = env.Error
	e.Message = env.Message
	if e.Message == "" {
		e.Message = env.ErrorDescription
	}
	e.Hint = env.Hint
	e.DocsURL = env.DocsURL
	e.Fields = env.Fields
	return e
}

func parseRateLimit(h http.Header) RateLimit {
	limit, _ := strconv.Atoi(h.Get("X-RateLimit-Limit"))
	remaining, _ := strconv.Atoi(h.Get("X-RateLimit-Remaining"))
	var reset time.Time
	unix, err := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64)
	if err == nil {
		reset = time.Unix(unix, 0).UTC()
	}
	return RateLimit{Limit: limit, Remaining: remaining, Reset: reset}
}

// parseRetryAfter reads the header in seconds, the only form the
// platform sends; an HTTP date is ignored.
func parseRetryAfter(h http.Header) time.Duration {
	seconds, err := strconv.Atoi(h.Get("Retry-After"))
	if err != nil || seconds < 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}
