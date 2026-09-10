package signer

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	"github.com/transpareo/transpareo-cli/pkg/transpareo/signer/internal/base58"
)

// TestDocumentsMatchThePlatformByteForByte proves the Ed25519
// proof values equal what the platform's own code produced for
// the same keys, body and proof configurations; Ed25519 is
// deterministic, so a differing hash data or encoding would
// show as a different value.
func TestDocumentsMatchThePlatformByteForByte(t *testing.T) {
	v := loadVectors(t)
	var req DocumentsRequest
	if err := json.Unmarshal([]byte(v.Documents.Request), &req); err != nil {
		t.Fatal(err)
	}
	resp, err := New(v.keys(t)).SignDocuments(&req)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Signatures) != 1 || resp.Signatures[0].Kind != "test" {
		t.Fatalf("signatures = %+v", resp.Signatures)
	}
	got := resp.Signatures[0].ProofValues
	if len(got) != len(v.Documents.ProofValues) {
		t.Fatalf("%d proof values, want %d", len(got),
			len(v.Documents.ProofValues))
	}
	for i := range got {
		if got[i] != v.Documents.ProofValues[i] {
			t.Errorf("proof value %d:\n got %s\nwant %s", i, got[i],
				v.Documents.ProofValues[i])
		}
	}
}

func TestDocumentsRefuseEmptyRequests(t *testing.T) {
	s := New(loadVectors(t).keys(t))
	if _, err := s.SignDocuments(&DocumentsRequest{}); err == nil {
		t.Error("no snapshots was accepted")
	}
	_, err := s.SignDocuments(&DocumentsRequest{Snapshots: []Snapshot{{
		Kind: "x", Body: "{}"}}})
	if err == nil || !strings.Contains(err.Error(), "proof configurations") {
		t.Errorf("err = %v", err)
	}
	_, err = s.SignDocuments(&DocumentsRequest{Snapshots: []Snapshot{{
		Kind: "x", Body: "{}", Proofs: []json.RawMessage{[]byte(`nope`)}}}})
	if err == nil {
		t.Error("a proof configuration that is not JSON was accepted")
	}
	if _, err := New(&Keys{}).SignDocuments(&DocumentsRequest{}); err == nil ||
		!strings.Contains(err.Error(), "Ed25519") {
		t.Errorf("without a key: %v", err)
	}
}

// TestBaseTupleLayoutMatchesThePlatform verifies the base
// signature the platform's code made over its own tuple with
// the layout this package builds, so the 99 bytes agree.
func TestBaseTupleLayoutMatchesThePlatform(t *testing.T) {
	v := loadVectors(t)
	keys := v.keys(t)
	proofHash, _ := base64.StdEncoding.DecodeString(v.BaseProof.ProofHash)
	mandatoryHash, _ := base64.StdEncoding.DecodeString(
		v.BaseProof.MandatoryHash)
	multikey, _ := base64.StdEncoding.DecodeString(v.BaseProof.ScopedMultikey)
	sig, _ := base64.StdEncoding.DecodeString(v.BaseProof.BaseSignature)
	tuple := BaseTuple(proofHash, multikey, mandatoryHash)
	if len(tuple) != 99 || !verifyES256(&keys.P256.PublicKey, tuple, sig) {
		t.Errorf("the platform's base signature does not verify over the "+
			"tuple (%d bytes)", len(tuple))
	}
	if len(multikey) != 35 || multikey[0] != 0x80 || multikey[1] != 0x24 {
		t.Errorf("multikey = %x", multikey)
	}
}

// TestBaseProofRoundTrip signs a base proof and verifies every
// component the way the platform does: the base signature with
// the workspace key over the tuple, each statement with the
// proof-scoped key taken from the multikey.
func TestBaseProofRoundTrip(t *testing.T) {
	v := loadVectors(t)
	keys := v.keys(t)
	req := &BaseProofRequest{Cryptosuite: CryptosuiteBaseProof,
		BaseProof: v.BaseProof2()}
	resp, err := New(keys).SignBaseProof(req)
	if err != nil {
		t.Fatal(err)
	}
	c := resp.BaseProof
	publicKey, _ := base64.StdEncoding.DecodeString(c.PublicKey)
	baseSig, _ := base64.StdEncoding.DecodeString(c.BaseSignature)
	proofHash, _ := base64.StdEncoding.DecodeString(v.BaseProof.ProofHash)
	mandatoryHash, _ := base64.StdEncoding.DecodeString(
		v.BaseProof.MandatoryHash)
	if len(publicKey) != 35 || publicKey[0] != 0x80 || publicKey[1] != 0x24 {
		t.Fatalf("public key = %x", publicKey)
	}
	if !verifyES256(&keys.P256.PublicKey,
		BaseTuple(proofHash, publicKey, mandatoryHash), baseSig) {
		t.Error("the base signature does not verify with the workspace key")
	}
	scoped := publicKeyFromMultikey(t, publicKey)
	if len(c.Signatures) != len(v.BaseProof.NonMandatory) {
		t.Fatalf("%d signatures for %d statements", len(c.Signatures),
			len(v.BaseProof.NonMandatory))
	}
	for i, statement := range v.BaseProof.NonMandatory {
		sig, _ := base64.StdEncoding.DecodeString(c.Signatures[i])
		if !verifyES256(scoped, []byte(statement), sig) {
			t.Errorf("statement %d does not verify with the scoped key", i)
		}
	}
	// A second proof gets a key of its own.
	again, _ := New(keys).SignBaseProof(req)
	if again.BaseProof.PublicKey == c.PublicKey {
		t.Error("the proof-scoped key was reused")
	}
}

func TestBaseProofRefusals(t *testing.T) {
	v := loadVectors(t)
	s := New(v.keys(t))
	cases := map[string]*BaseProofRequest{
		"cryptosuite": {Cryptosuite: "eddsa-jcs-2022",
			BaseProof: v.BaseProof2()},
		"proof hash": {Cryptosuite: CryptosuiteBaseProof,
			BaseProof: BaseProof{ProofHash: "AAAA",
				MandatoryHash: v.BaseProof.MandatoryHash}},
		"base64": {Cryptosuite: CryptosuiteBaseProof,
			BaseProof: BaseProof{ProofHash: "%%%",
				MandatoryHash: v.BaseProof.MandatoryHash}},
	}
	for name, req := range cases {
		if _, err := s.SignBaseProof(req); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := New(&Keys{}).SignBaseProof(cases["cryptosuite"]); err == nil ||
		!strings.Contains(err.Error(), "P-256") {
		t.Errorf("without a key: %v", err)
	}
}

func TestProofValueEncoding(t *testing.T) {
	keys := loadVectors(t).keys(t)
	value := ProofValue(keys.Ed25519, HashData([]byte("{}"),
		make([]byte, sha256.Size)))
	if !strings.HasPrefix(value, "z") {
		t.Fatalf("value = %s", value)
	}
	raw, err := base58.Decode(value[1:])
	if err != nil || len(raw) != 64 {
		t.Errorf("decoded %d bytes, err %v", len(raw), err)
	}
}

// BaseProof2 answers the vector's base proof as a request body.
func (v *vectors) BaseProof2() BaseProof {
	return BaseProof{ProofHash: v.BaseProof.ProofHash,
		MandatoryHash: v.BaseProof.MandatoryHash,
		NonMandatory:  v.BaseProof.NonMandatory}
}

func verifyES256(pub *ecdsa.PublicKey, data, sig []byte) bool {
	if len(sig) != 64 {
		return false
	}
	sum := sha256.Sum256(data)
	return ecdsa.Verify(pub, sum[:], new(big.Int).SetBytes(sig[:32]),
		new(big.Int).SetBytes(sig[32:]))
}

func publicKeyFromMultikey(t *testing.T, multikey []byte) *ecdsa.PublicKey {
	t.Helper()
	x, y := decompressP256(multikey[2:])
	if x == nil {
		t.Fatal("the multikey does not decompress to a P-256 point")
	}
	uncompressed := append([]byte{0x04}, x.FillBytes(make([]byte, 32))...)
	uncompressed = append(uncompressed, y.FillBytes(make([]byte, 32))...)
	pub, err := ecdsa.ParseUncompressedPublicKey(p256Curve(), uncompressed)
	if err != nil {
		t.Fatal(err)
	}
	return pub
}
