package signer

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"encoding/json"
	"os"
	"testing"
)

// vectors is testdata/platform_vectors.json: keys, a documents
// request with the proof values the platform's own code
// produced for it, a request signed in the cluster's canonical
// form, and a base proof tuple signed by the workspace key.
type vectors struct {
	Ed25519PrivatePEM  string `json:"ed25519_private_pem"`
	Ed25519PublicPEM   string `json:"ed25519_public_pem"`
	P256PrivatePEM     string `json:"p256_private_pem"`
	P256PublicPEM      string `json:"p256_public_pem"`
	PlatformPrivatePEM string `json:"platform_private_pem"`
	PlatformPublicPEM  string `json:"platform_public_pem"`
	Documents          struct {
		Request     string   `json:"request"`
		ProofValues []string `json:"proof_values"`
	} `json:"documents"`
	SignedRequest struct {
		Method, Host, Path, Timestamp, Nonce, Body, Signature string
	} `json:"signed_request"`
	BaseProof struct {
		ProofHash      string   `json:"proof_hash"`
		MandatoryHash  string   `json:"mandatory_hash"`
		NonMandatory   []string `json:"non_mandatory"`
		ScopedMultikey string   `json:"scoped_multikey"`
		BaseSignature  string   `json:"base_signature"`
	} `json:"base_proof"`
}

func loadVectors(t *testing.T) *vectors {
	t.Helper()
	data, err := os.ReadFile("testdata/platform_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v vectors
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	return &v
}

func (v *vectors) keys(t *testing.T) *Keys {
	t.Helper()
	ed, err := ParsePrivateKey([]byte(v.Ed25519PrivatePEM))
	if err != nil {
		t.Fatal(err)
	}
	p256, err := ParsePrivateKey([]byte(v.P256PrivatePEM))
	if err != nil {
		t.Fatal(err)
	}
	return &Keys{Ed25519: ed.(ed25519.PrivateKey),
		P256: p256.(*ecdsa.PrivateKey)}
}

func (v *vectors) platformKey(t *testing.T) ed25519.PublicKey {
	t.Helper()
	key, err := ParseEd25519PublicKey([]byte(v.PlatformPublicPEM))
	if err != nil {
		t.Fatal(err)
	}
	return key
}
