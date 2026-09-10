package signer

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// HeaderFingerprint names the key the platform signed a request
// with, so a rotation is seen before a signature fails.
const HeaderFingerprint = "X-Master-Key-Fingerprint"

// rotationContext opens the string a hand-over statement signs.
const rotationContext = "master-key-rotation"

// ErrNoRotation says the host publishes no hand-over statement.
// A host that never rotated answers that way, and so does one
// that is not the master of its cluster.
var ErrNoRotation = errors.New("the host publishes no rotation statement")

// Fingerprint is the lowercase hex SHA-256 over the SPKI DER of
// key: the value a signed request carries in
// X-Master-Key-Fingerprint and a hand-over statement names.
func Fingerprint(key ed25519.PublicKey) (string, error) {
	if len(key) != ed25519.PublicKeySize {
		return "", errors.New("not an Ed25519 public key")
	}
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:]), nil
}

// Rotation is the statement a host publishes when it promotes a
// new request-signing key: the retired key signs the hand-over,
// so an endpoint pinned to that key can take the new one without
// trusting whatever the key URL happens to answer.
type Rotation struct {
	PreviousFingerprint string `json:"previous_fingerprint"`
	PreviousPublicKey   string `json:"previous_public_key"`
	Fingerprint         string `json:"fingerprint"`
	RotatedAt           string `json:"rotated_at"`
	Signature           string `json:"signature"`
}

// ParseRotation reads a statement and checks that it carries
// every part of the hand-over.
func ParseRotation(data []byte) (*Rotation, error) {
	var rotation Rotation
	if err := json.Unmarshal(data, &rotation); err != nil {
		return nil, fmt.Errorf("malformed rotation statement: %w", err)
	}
	if rotation.PreviousFingerprint == "" || rotation.Fingerprint == "" ||
		rotation.RotatedAt == "" || rotation.Signature == "" {
		return nil, errors.New("incomplete rotation statement")
	}
	return &rotation, nil
}

// HandsOverFrom checks that previous retired itself in this
// statement: it names previous as the key it replaces, and
// previous signed it.
func (r *Rotation) HandsOverFrom(previous ed25519.PublicKey) error {
	fingerprint, err := Fingerprint(previous)
	if err != nil {
		return err
	}
	if r.PreviousFingerprint != fingerprint {
		return fmt.Errorf("the statement retires %s, not the pinned key",
			r.PreviousFingerprint)
	}
	signature, err := base64.StdEncoding.DecodeString(r.Signature)
	if err != nil {
		return errors.New("malformed signature on the rotation statement")
	}
	if !ed25519.Verify(previous, r.signed(), signature) {
		return errors.New("the retired key did not sign the rotation statement")
	}
	return nil
}

// Names says whether the statement hands over to key.
func (r *Rotation) Names(key ed25519.PublicKey) error {
	fingerprint, err := Fingerprint(key)
	if err != nil {
		return err
	}
	if fingerprint != r.Fingerprint {
		return fmt.Errorf("the key served is %s, the statement hands over "+
			"to %s", fingerprint, r.Fingerprint)
	}
	return nil
}

// signed is the string the retired key signs: the context and
// the three fields, joined by newlines, with no trailing one.
func (r *Rotation) signed() []byte {
	return []byte(strings.Join([]string{rotationContext,
		r.PreviousFingerprint, r.Fingerprint, r.RotatedAt}, "\n"))
}
