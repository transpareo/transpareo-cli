package signer

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func signedRequest(t *testing.T, v *vectors) (*http.Request, []byte) {
	t.Helper()
	s := v.SignedRequest
	r := httptest.NewRequest(s.Method, "https://"+s.Host+":8443"+s.Path,
		strings.NewReader(s.Body))
	r.Host = s.Host + ":8443"
	r.Header.Set(HeaderTimestamp, s.Timestamp)
	r.Header.Set(HeaderNonce, s.Nonce)
	r.Header.Set(HeaderSignature, s.Signature)
	return r, []byte(s.Body)
}

func clockAt(stamp string) func() time.Time {
	at, _ := time.Parse(time.RFC3339, stamp)
	return func() time.Time { return at.Add(30 * time.Second) }
}

// TestVerifiesThePlatformsSignedRequest checks the signature the
// platform's canonical form produced, with the port stripped
// from the Host header and the query kept in the path.
func TestVerifiesThePlatformsSignedRequest(t *testing.T) {
	v := loadVectors(t)
	ver := NewVerifier(v.platformKey(t))
	ver.Now = clockAt(v.SignedRequest.Timestamp)
	r, body := signedRequest(t, v)
	if err := ver.Verify(r, body); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := ver.Verify(r, body); err == nil ||
		!strings.Contains(err.Error(), "replayed") {
		t.Errorf("the second use of the nonce: %v", err)
	}
}

func TestRefusals(t *testing.T) {
	v := loadVectors(t)
	fresh := func() (*Verifier, *http.Request, []byte) {
		ver := NewVerifier(v.platformKey(t))
		ver.Now = clockAt(v.SignedRequest.Timestamp)
		r, body := signedRequest(t, v)
		return ver, r, body
	}
	t.Run("tampered body", func(t *testing.T) {
		ver, r, body := fresh()
		body = append(body, ' ')
		if err := ver.Verify(r, body); err == nil ||
			!strings.Contains(err.Error(), "invalid signature") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("other host", func(t *testing.T) {
		ver, r, body := fresh()
		r.Host = "other.example.com"
		if err := ver.Verify(r, body); err == nil {
			t.Error("a request signed for another host was accepted")
		}
	})
	t.Run("stale timestamp", func(t *testing.T) {
		ver, r, body := fresh()
		ver.Now = func() time.Time {
			return clockAt(v.SignedRequest.Timestamp)().Add(6 * time.Minute)
		}
		if err := ver.Verify(r, body); err == nil ||
			!strings.Contains(err.Error(), "replay window") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("missing header", func(t *testing.T) {
		ver, r, body := fresh()
		r.Header.Del(HeaderNonce)
		if err := ver.Verify(r, body); err == nil ||
			!strings.Contains(err.Error(), "incomplete") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("unsigned", func(t *testing.T) {
		ver, r, body := fresh()
		r.Header.Del(HeaderNonce)
		r.Header.Del(HeaderTimestamp)
		r.Header.Del(HeaderSignature)
		if err := ver.Verify(r, body); err != ErrUnsigned {
			t.Errorf("err = %v", err)
		}
		ver.AllowUnsigned = true
		if err := ver.Verify(r, body); err != nil {
			t.Errorf("with AllowUnsigned: %v", err)
		}
	})
	t.Run("malformed", func(t *testing.T) {
		ver, r, body := fresh()
		r.Header.Set(HeaderSignature, "%%%")
		if err := ver.Verify(r, body); err == nil {
			t.Error("a malformed signature was accepted")
		}
		r.Header.Set(HeaderTimestamp, "yesterday")
		if err := ver.Verify(r, body); err == nil {
			t.Error("a malformed timestamp was accepted")
		}
	})
}

// TestRotatedKeyIsFetchedOnce proves a request signed by a key
// the verifier does not hold triggers one refetch, is accepted
// when the fetched key verifies it, and that a forgery does not
// cause a refetch storm.
func TestRotatedKeyIsFetchedOnce(t *testing.T) {
	v := loadVectors(t)
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	ver := NewVerifier(v.platformKey(t))
	ver.Now = clockAt(v.SignedRequest.Timestamp)
	fetches := 0
	ver.Refetch = func() (ed25519.PublicKey, error) {
		fetches++
		return pub, nil
	}
	s := v.SignedRequest
	r, body := signedRequest(t, v)
	r.Header.Set(HeaderNonce, "0000000000000000000000000000000a")
	canonical := Canonical(s.Method, s.Host, s.Path, s.Timestamp,
		"0000000000000000000000000000000a", body)
	r.Header.Set(HeaderSignature,
		base64.StdEncoding.EncodeToString(ed25519.Sign(priv, canonical)))
	if err := ver.Verify(r, body); err != nil || fetches != 1 {
		t.Fatalf("after a rotation: err %v, fetches %d", err, fetches)
	}
	// The old key no longer verifies, and a second failure inside
	// the minute does not fetch again.
	old, oldBody := signedRequest(t, v)
	if err := ver.Verify(old, oldBody); err == nil || fetches != 1 {
		t.Errorf("old key: err %v, fetches %d", err, fetches)
	}
}
