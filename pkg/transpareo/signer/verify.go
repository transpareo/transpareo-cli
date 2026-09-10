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

// errInvalidSignature is the refusal of a request whose
// signature does not verify under any key this endpoint trusts.
var errInvalidSignature = errors.New("invalid signature")

// Verifier checks the platform's signature on a request: the
// Ed25519 signature over the canonical string, the timestamp
// within the replay window and a nonce not seen before.
type Verifier struct {
	// Refetch answers the platform's current request-signing
	// key, for a rotation that happened since the key was
	// pinned. Called at most once a minute, only after a
	// signature failed, and nil when the key came from a file.
	Refetch func() (ed25519.PublicKey, error)

	// Rotation answers the hand-over statement the platform
	// published for its current key, ErrNoRotation from a host
	// that publishes none. It gates every key Refetch brings:
	// without it a host that answers the key URL decides what
	// this endpoint trusts. Nil skips the check.
	Rotation func() (*Rotation, error)

	// AllowUnsigned accepts a request without the platform
	// headers, for a development platform whose request signing
	// is not seeded. Never set it for a production endpoint.
	AllowUnsigned bool

	// Host is the host name the platform signs for: the host of
	// the registered endpoint URL. Set it when a reverse proxy
	// rewrites the Host header on the way in; empty takes the
	// request's own Host header without its port.
	Host string

	// Now replaces the clock, in tests.
	Now func() time.Time

	mu             sync.Mutex
	key            ed25519.PublicKey
	keyFingerprint string
	nonces         map[string]time.Time
	lastRefetch    time.Time
}

// NewVerifier returns a Verifier pinned to key.
func NewVerifier(key ed25519.PublicKey) *Verifier {
	fingerprint, _ := Fingerprint(key)
	return &Verifier{key: key, keyFingerprint: fingerprint,
		nonces: map[string]time.Time{}, Now: time.Now}
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
	host := v.Host
	if host == "" {
		host = hostOf(r)
	}
	canonical := Canonical(r.Method, host, r.URL.RequestURI(), timestamp,
		nonce, body)
	if err := v.verifyWithRotation(canonical, sig,
		r.Header.Get(HeaderFingerprint), now); err != nil {
		return err
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

// verifyWithRotation checks the signature against the pinned
// key. When that fails and the platform names a key other than
// the pinned one, its current key is fetched and taken only if
// the pinned key handed over to it. The fetches run outside the
// lock, so a slow platform holds up this one request and not
// every other.
func (v *Verifier) verifyWithRotation(canonical, sig []byte,
	fingerprint string, now time.Time) error {
	v.mu.Lock()
	key, pinned := v.key, v.keyFingerprint
	v.mu.Unlock()
	if ed25519.Verify(key, canonical, sig) {
		return nil
	}
	// The platform signed with the key already pinned, so the
	// signature is wrong and there is nothing to fetch.
	if fingerprint != "" && fingerprint == pinned {
		return errInvalidSignature
	}
	if !v.mayFetch(now) {
		return errInvalidSignature
	}
	fetched, err := v.Refetch()
	if err != nil || !ed25519.Verify(fetched, canonical, sig) {
		return errInvalidSignature
	}
	if err := v.handedOver(key, fetched); err != nil {
		return err
	}
	v.adopt(fetched)
	return nil
}

// handedOver says whether the key just fetched may replace the
// pinned one. A host that publishes a hand-over statement has to
// prove the rotation with it: the pinned key signed the
// statement, and the statement names the fetched key. A host
// that publishes none is taken at its key URL, which is what a
// node in a cluster answers today.
func (v *Verifier) handedOver(pinned, fetched ed25519.PublicKey) error {
	if v.Rotation == nil {
		return nil
	}
	statement, err := v.Rotation()
	if errors.Is(err, ErrNoRotation) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading the rotation statement: %w", err)
	}
	if err := statement.HandsOverFrom(pinned); err != nil {
		return fmt.Errorf("%w; restart the endpoint to pin the key the "+
			"platform signs with now", err)
	}
	return statement.Names(fetched)
}

// mayFetch takes the budget for one look at the platform, which
// a failed signature and a key this endpoint does not know are
// worth at most once a minute. A request that verifies never
// spends it, so a rotation is followed by the first request
// that arrives under the new key.
func (v *Verifier) mayFetch(now time.Time) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.Refetch == nil || now.Sub(v.lastRefetch) < refetchInterval {
		return false
	}
	v.lastRefetch = now
	return true
}

// adopt pins the key the platform handed over to.
func (v *Verifier) adopt(key ed25519.PublicKey) {
	fingerprint, _ := Fingerprint(key)
	v.mu.Lock()
	v.key, v.keyFingerprint = key, fingerprint
	v.mu.Unlock()
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
