package signer

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// platformRotation is a hand-over statement the platform's own
// code produced, with the key it hands over to. Everything the
// endpoint reads off a rotation is checked against these bytes.
const platformRotation = `{"previous_fingerprint":"56dc51e813d8c301daf2c` +
	`4170c388aa56500f7e4ea58b9710ce706bae5d37644","previous_public_key":` +
	`"-----BEGIN PUBLIC KEY-----\nMCowBQYDK2VwAyEAEXJ3L/LS8sr2oS8hOH18Jt` +
	`alIMUldFP0WyO9GTE5hCk=\n-----END PUBLIC KEY-----\n","fingerprint":"` +
	`850d46463336db8f48486ce14e04f1035857cff0993b0b73be455c404140fd70","r` +
	`otated_at":"2026-09-10T18:16:06Z","signature":"yIq1rAMLkGaEtEU/znEvb` +
	`LwyM6yOMeaYBCyXP7occnfK2nEtxSK8zFpeCU55vK9sufKR/0M+7sxAbfxHYWqMDA=="}`

const platformSuccessorPEM = `-----BEGIN PUBLIC KEY-----
MCowBQYDK2VwAyEAoTVlfitDoY2ncXIhDF+taZ4PcLH2QNYpXXYDB82GNik=
-----END PUBLIC KEY-----
`

func parseRotation(t *testing.T, data string) *Rotation {
	t.Helper()
	rotation, err := ParseRotation([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return rotation
}

func publicKey(t *testing.T, pem string) ed25519.PublicKey {
	t.Helper()
	key, err := ParseEd25519PublicKey([]byte(pem))
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// TestFingerprintMatchesThePlatform checks the fingerprint over
// the SPKI DER against the two the platform wrote into its own
// statement, so the header comparison agrees with the platform
// byte for byte.
func TestFingerprintMatchesThePlatform(t *testing.T) {
	rotation := parseRotation(t, platformRotation)
	previous := publicKey(t, rotation.PreviousPublicKey)
	fingerprint, err := Fingerprint(previous)
	if err != nil {
		t.Fatal(err)
	}
	if fingerprint != rotation.PreviousFingerprint {
		t.Errorf("retired key = %s, statement says %s", fingerprint,
			rotation.PreviousFingerprint)
	}
	successor, err := Fingerprint(publicKey(t, platformSuccessorPEM))
	if err != nil {
		t.Fatal(err)
	}
	if successor != rotation.Fingerprint {
		t.Errorf("successor = %s, statement says %s", successor,
			rotation.Fingerprint)
	}
	if _, err := Fingerprint(nil); err == nil {
		t.Error("a missing key has no fingerprint")
	}
}

// TestPlatformRotationVerifies checks the statement the way the
// endpoint does: signed by the retired key over the four parts,
// naming the key the host serves now.
func TestPlatformRotationVerifies(t *testing.T) {
	rotation := parseRotation(t, platformRotation)
	previous := publicKey(t, rotation.PreviousPublicKey)
	if err := rotation.HandsOverFrom(previous); err != nil {
		t.Fatalf("the platform's own statement: %v", err)
	}
	if err := rotation.Names(publicKey(t, platformSuccessorPEM)); err != nil {
		t.Errorf("the key it hands over to: %v", err)
	}
	if err := rotation.Names(previous); err == nil {
		t.Error("the retired key passed as the successor")
	}

	// Another key's statement is not this endpoint's hand-over.
	other, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := rotation.HandsOverFrom(other); err == nil ||
		!strings.Contains(err.Error(), "not the pinned key") {
		t.Errorf("a statement for another key: %v", err)
	}
}

// TestRotationRefusesTampering proves every part of the signed
// string is covered, and that a truncated statement is refused
// before anything is verified.
func TestRotationRefusesTampering(t *testing.T) {
	previous := publicKey(t,
		parseRotation(t, platformRotation).PreviousPublicKey)
	for _, field := range []string{"fingerprint", "rotated_at",
		"previous_fingerprint", "signature"} {
		t.Run(field, func(t *testing.T) {
			var fields map[string]any
			if err := json.Unmarshal([]byte(platformRotation),
				&fields); err != nil {
				t.Fatal(err)
			}
			fields[field] = tamper(fields[field].(string))
			data, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			rotation, err := ParseRotation(data)
			if err != nil {
				t.Fatal(err)
			}
			if err := rotation.HandsOverFrom(previous); err == nil {
				t.Errorf("a changed %s still verified", field)
			}
		})
	}
	if _, err := ParseRotation([]byte(`{"fingerprint":"a"}`)); err == nil {
		t.Error("an incomplete statement was accepted")
	}
	if _, err := ParseRotation([]byte("not json")); err == nil {
		t.Error("a statement that is not JSON was accepted")
	}
}

// tamper changes the last character of a value, keeping its
// alphabet so base64 and hex still decode.
func tamper(value string) string {
	last := value[len(value)-1]
	replacement := byte('a')
	if last == 'a' {
		replacement = 'b'
	}
	return value[:len(value)-1] + string(replacement)
}

// rotationFixture is a platform that signs with one key and can
// hand over to another, for the verifier's side of a rotation.
type rotationFixture struct {
	t                  *testing.T
	pinned, current    ed25519.PrivateKey
	statement          *Rotation
	fetches, rotations int
	rotationErr        error
	nonce              int
}

func newRotationFixture(t *testing.T) *rotationFixture {
	t.Helper()
	_, pinned, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	_, current, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	f := &rotationFixture{t: t, pinned: pinned, current: current}
	f.statement = f.handOver(pinned, current.Public().(ed25519.PublicKey))
	return f
}

// handOver is the statement a host publishes when from retires
// in favour of to.
func (f *rotationFixture) handOver(from ed25519.PrivateKey,
	to ed25519.PublicKey) *Rotation {
	f.t.Helper()
	previous, err := Fingerprint(from.Public().(ed25519.PublicKey))
	if err != nil {
		f.t.Fatal(err)
	}
	fingerprint, err := Fingerprint(to)
	if err != nil {
		f.t.Fatal(err)
	}
	rotation := &Rotation{PreviousFingerprint: previous,
		Fingerprint: fingerprint, RotatedAt: "2026-09-10T18:16:06Z"}
	rotation.Signature = base64.StdEncoding.EncodeToString(
		ed25519.Sign(from, rotation.signed()))
	return rotation
}

// verifier is pinned to the fixture's first key and reaches the
// platform through the fixture's counted fetches.
func (f *rotationFixture) verifier() *Verifier {
	v := NewVerifier(f.pinned.Public().(ed25519.PublicKey))
	v.Now = func() time.Time {
		return time.Date(2026, 9, 10, 18, 30, 0, 0,
			time.UTC)
	}
	v.Refetch = func() (ed25519.PublicKey, error) {
		f.fetches++
		return f.current.Public().(ed25519.PublicKey), nil
	}
	v.Rotation = func() (*Rotation, error) {
		f.rotations++
		return f.statement, f.rotationErr
	}
	return v
}

// request is signed by key and carries the fingerprint of it,
// the way the platform sends one.
func (f *rotationFixture) request(key ed25519.PrivateKey) (*http.Request,
	[]byte) {
	f.t.Helper()
	f.nonce++
	body := []byte(`{"snapshots":[]}`)
	timestamp := "2026-09-10T18:30:00Z"
	nonce := fmt.Sprintf("nonce-%d", f.nonce)
	r := httptest.NewRequest(http.MethodPost,
		"https://signer.example.com:8443/sign", strings.NewReader(string(body)))
	r.Host = "signer.example.com:8443"
	canonical := Canonical(r.Method, "signer.example.com", "/sign", timestamp,
		nonce, body)
	fingerprint, err := Fingerprint(key.Public().(ed25519.PublicKey))
	if err != nil {
		f.t.Fatal(err)
	}
	r.Header.Set(HeaderTimestamp, timestamp)
	r.Header.Set(HeaderNonce, nonce)
	r.Header.Set(HeaderSignature, base64.StdEncoding.EncodeToString(
		ed25519.Sign(key, canonical)))
	r.Header.Set(HeaderFingerprint, fingerprint)
	return r, body
}

// TestRotationIsFollowedOnlyWhenHandedOver walks the verifier
// through a rotation: the statement of the pinned key carries
// the new one in, and without it nothing changes.
func TestRotationIsFollowedOnlyWhenHandedOver(t *testing.T) {
	t.Run("the pinned key hands over", func(t *testing.T) {
		f := newRotationFixture(t)
		v := f.verifier()
		r, body := f.request(f.current)
		if err := v.Verify(r, body); err != nil {
			t.Fatalf("a rotation the pinned key signed: %v", err)
		}
		if f.fetches != 1 || f.rotations != 1 {
			t.Errorf("fetches = %d, statements = %d", f.fetches, f.rotations)
		}
		// The new key is pinned now, so the next request needs
		// neither fetch.
		r, body = f.request(f.current)
		if err := v.Verify(r, body); err != nil {
			t.Fatalf("after the hand-over: %v", err)
		}
		if f.fetches != 1 || f.rotations != 1 {
			t.Errorf("fetched again: %d, %d", f.fetches, f.rotations)
		}
	})

	t.Run("a host without a statement", func(t *testing.T) {
		f := newRotationFixture(t)
		f.statement, f.rotationErr = nil, ErrNoRotation
		v := f.verifier()
		r, body := f.request(f.current)
		if err := v.Verify(r, body); err != nil {
			t.Fatalf("a node serves no statement: %v", err)
		}
	})

	t.Run("a statement from another key", func(t *testing.T) {
		f := newRotationFixture(t)
		_, stranger, err := ed25519.GenerateKey(nil)
		if err != nil {
			t.Fatal(err)
		}
		f.statement = f.handOver(stranger,
			f.current.Public().(ed25519.PublicKey))
		v := f.verifier()
		r, body := f.request(f.current)
		err = v.Verify(r, body)
		if err == nil || !strings.Contains(err.Error(), "restart the endpoint") {
			t.Fatalf("a statement the pinned key never signed: %v", err)
		}
	})

	t.Run("a statement naming a third key", func(t *testing.T) {
		f := newRotationFixture(t)
		other, _, err := ed25519.GenerateKey(nil)
		if err != nil {
			t.Fatal(err)
		}
		f.statement = f.handOver(f.pinned, other)
		v := f.verifier()
		r, body := f.request(f.current)
		err = v.Verify(r, body)
		if err == nil || !strings.Contains(err.Error(), "hands over to") {
			t.Fatalf("the key served is not the one named: %v", err)
		}
	})

	t.Run("a forgery under the pinned fingerprint", func(t *testing.T) {
		f := newRotationFixture(t)
		v := f.verifier()
		r, body := f.request(f.pinned)
		// The platform says it signed with the pinned key, so a
		// signature that fails is a forgery and not a rotation.
		r.Header.Set(HeaderSignature, base64.StdEncoding.EncodeToString(
			make([]byte, ed25519.SignatureSize)))
		if err := v.Verify(r, body); err == nil {
			t.Fatal("a forged signature verified")
		}
		if f.fetches != 0 || f.rotations != 0 {
			t.Errorf("a forgery reached the platform: %d, %d", f.fetches,
				f.rotations)
		}
	})
}
