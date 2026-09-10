package signer

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Header names of the platform's request signature.
const (
	HeaderTimestamp = "X-Master-Timestamp"
	HeaderNonce     = "X-Master-Nonce"
	HeaderSignature = "X-Master-Signature"
)

// ReplayWindow is how far a request's timestamp may sit from
// the verifier's clock, and how long a nonce is remembered.
const ReplayWindow = 5 * time.Minute

// refetchInterval bounds how often a signature that fails
// triggers a fetch of the platform's current key, so a flood of
// forged requests cannot turn the verifier into a client of
// the platform.
const refetchInterval = time.Minute

// ErrUnsigned is the refusal of a request without the platform
// headers.
var ErrUnsigned = errors.New("the request carries no platform signature")

// Verifier checks the platform's signature on a request: the
// Ed25519 signature over the canonical string, the timestamp
// within the replay window and a nonce not seen before.
type Verifier struct {
	// Refetch answers the platform's current request-signing
	// key, for a rotation that happened since the key was
	// pinned. Called at most once a minute, only after a
	// signature failed, and nil when the key came from a file.
	Refetch func() (ed25519.PublicKey, error)

	// AllowUnsigned accepts a request without the platform
	// headers, for a development platform whose request signing
	// is not seeded. Never set it for a production endpoint.
	AllowUnsigned bool

	// Now replaces the clock, in tests.
	Now func() time.Time

	mu          sync.Mutex
	key         ed25519.PublicKey
	nonces      map[string]time.Time
	lastRefetch time.Time
}

// NewVerifier returns a Verifier pinned to key.
func NewVerifier(key ed25519.PublicKey) *Verifier {
	return &Verifier{key: key, nonces: map[string]time.Time{}, Now: time.Now}
}

// Canonical is the string the platform signs: method, host,
// path with query, timestamp, nonce and body, joined by
// newlines, the method upper case and the host lower case.
func Canonical(method, host, path, timestamp, nonce string,
	body []byte) []byte {
	return []byte(strings.Join([]string{strings.ToUpper(method),
		strings.ToLower(host), path, timestamp, nonce, string(body)}, "\n"))
}

// Verify checks the request with the body already read. The
// host is the Host header without its port, which is how the
// platform names the target it signs for.
func (v *Verifier) Verify(r *http.Request, body []byte) error {
	timestamp := r.Header.Get(HeaderTimestamp)
	nonce := r.Header.Get(HeaderNonce)
	signature := r.Header.Get(HeaderSignature)
	if timestamp == "" && nonce == "" && signature == "" {
		if v.AllowUnsigned {
			return nil
		}
		return ErrUnsigned
	}
	if timestamp == "" || nonce == "" || signature == "" {
		return errors.New("the platform signature headers are incomplete")
	}
	now := v.Now()
	at, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return fmt.Errorf("malformed timestamp %q", timestamp)
	}
	if at.Sub(now).Abs() > ReplayWindow {
		return errors.New("timestamp outside the replay window")
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return errors.New("malformed signature")
	}
	canonical := Canonical(r.Method, hostOf(r), r.URL.RequestURI(), timestamp,
		nonce, body)
	if !v.verifyWithRefetch(canonical, sig, now) {
		return errors.New("invalid signature")
	}
	// The nonce is spent only once the signature is proven, so
	// an unauthenticated caller cannot burn nonces.
	return v.spend(nonce, now)
}

func hostOf(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.Host); err == nil {
		return host
	}
	return r.Host
}

// verifyWithRefetch checks the signature against the pinned key
// and, when that fails and a refetch is possible, once more
// against the platform's current key.
func (v *Verifier) verifyWithRefetch(canonical, sig []byte,
	now time.Time) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if ed25519.Verify(v.key, canonical, sig) {
		return true
	}
	if v.Refetch == nil || now.Sub(v.lastRefetch) < refetchInterval {
		return false
	}
	v.lastRefetch = now
	key, err := v.Refetch()
	if err != nil || !ed25519.Verify(key, canonical, sig) {
		return false
	}
	v.key = key
	return true
}

func (v *Verifier) spend(nonce string, now time.Time) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	for seen, at := range v.nonces {
		if now.Sub(at) > ReplayWindow {
			delete(v.nonces, seen)
		}
	}
	if _, replayed := v.nonces[nonce]; replayed {
		return errors.New("replayed nonce")
	}
	v.nonces[nonce] = now
	return nil
}
