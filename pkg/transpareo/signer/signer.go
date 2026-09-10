package signer

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/transpareo/transpareo-cli/pkg/transpareo/signer/internal/base58"
	"github.com/transpareo/transpareo-cli/pkg/transpareo/signer/internal/jcs"
)

// CryptosuiteBaseProof names the request shape that asks for a
// selective-disclosure base proof.
const CryptosuiteBaseProof = "ecdsa-sd-2023"

// p256MultikeyPrefix opens the multikey form of a P-256 public
// key: the varint of 0x1200, followed by the compressed point.
var p256MultikeyPrefix = []byte{0x80, 0x24}

// DocumentsRequest asks for whole-document signatures: one
// Ed25519 proof value per proof configuration of each snapshot.
type DocumentsRequest struct {
	RequestedAt string     `json:"requested_at"`
	Snapshots   []Snapshot `json:"snapshots"`
}

// Snapshot is one canonical document with the proof
// configurations to sign it under.
type Snapshot struct {
	Kind   string            `json:"kind"`
	Body   string            `json:"body"`
	Proofs []json.RawMessage `json:"proofs"`
}

// DocumentsResponse carries the proof values, one entry per
// snapshot in request order.
type DocumentsResponse struct {
	Signatures []Signature `json:"signatures"`
}

// Signature is the proof values of one snapshot, base58btc
// behind a "z", in the order of its proof configurations.
type Signature struct {
	Kind        string   `json:"kind"`
	ProofValues []string `json:"proofValues"`
}

// BaseProofRequest asks for the signed components of an
// ecdsa-sd-2023 base proof.
type BaseProofRequest struct {
	RequestedAt string    `json:"requested_at"`
	Cryptosuite string    `json:"cryptosuite"`
	BaseProof   BaseProof `json:"base_proof"`
}

// BaseProof is what the platform hashed and canonicalised: the
// two 32-byte digests base64, and the non-mandatory N-Quad
// statements as sent.
type BaseProof struct {
	ProofHash     string   `json:"proof_hash"`
	MandatoryHash string   `json:"mandatory_hash"`
	NonMandatory  []string `json:"non_mandatory"`
}

// BaseProofResponse carries the components, every value base64.
type BaseProofResponse struct {
	BaseProof BaseProofComponents `json:"base_proof"`
}

// BaseProofComponents are the multikey public key of the
// proof-scoped key, the base signature by the workspace key,
// and one proof-scoped signature per statement.
type BaseProofComponents struct {
	PublicKey     string   `json:"public_key"`
	BaseSignature string   `json:"base_signature"`
	Signatures    []string `json:"signatures"`
}

// Signer holds the workspace's keys and answers both request
// shapes.
type Signer struct {
	keys *Keys
}

// New returns a Signer over the keys. Either key may be nil
// when a workspace registered only one curve; a request for the
// missing one is refused.
func New(keys *Keys) *Signer {
	return &Signer{keys: keys}
}

// SignDocuments signs SHA-256(JCS(proofConfig)) followed by
// SHA-256(body) with the Ed25519 key, for every proof
// configuration of every snapshot.
func (s *Signer) SignDocuments(req *DocumentsRequest) (*DocumentsResponse,
	error) {
	if s.keys.Ed25519 == nil {
		return nil, errors.New("no Ed25519 key is loaded")
	}
	if len(req.Snapshots) == 0 {
		return nil, errors.New("the request carries no snapshots")
	}
	resp := &DocumentsResponse{}
	for i, snap := range req.Snapshots {
		if len(snap.Proofs) == 0 {
			return nil, fmt.Errorf("snapshot %d carries no proof "+
				"configurations", i)
		}
		bodyDigest := sha256.Sum256([]byte(snap.Body))
		values := make([]string, 0, len(snap.Proofs))
		for j, config := range snap.Proofs {
			canonical, err := jcs.Canonical(config)
			if err != nil {
				return nil, fmt.Errorf("snapshot %d, proof %d: %w", i, j, err)
			}
			values = append(values, ProofValue(s.keys.Ed25519,
				HashData(canonical, bodyDigest[:])))
		}
		resp.Signatures = append(resp.Signatures,
			Signature{Kind: snap.Kind, ProofValues: values})
	}
	return resp, nil
}

// HashData is the eddsa-jcs-2022 message: the digest of the
// canonical proof configuration followed by the digest of the
// document.
func HashData(canonicalConfig, documentDigest []byte) []byte {
	configDigest := sha256.Sum256(canonicalConfig)
	return append(configDigest[:], documentDigest...)
}

// ProofValue signs hashData with the Ed25519 key and encodes
// the signature as the cryptosuite asks: base58btc behind "z".
func ProofValue(key ed25519.PrivateKey, hashData []byte) string {
	return "z" + base58.Encode(ed25519.Sign(key, hashData))
}

// SignBaseProof generates a P-256 key for this one proof, signs
// every non-mandatory statement with it, and signs the tuple of
// proof hash, proof-scoped public key and mandatory hash with
// the workspace's P-256 key.
func (s *Signer) SignBaseProof(req *BaseProofRequest) (*BaseProofResponse,
	error) {
	if s.keys.P256 == nil {
		return nil, errors.New("no P-256 key is loaded")
	}
	if req.Cryptosuite != CryptosuiteBaseProof {
		return nil, fmt.Errorf("unsupported cryptosuite %q", req.Cryptosuite)
	}
	proofHash, err := digest(req.BaseProof.ProofHash, "proof_hash")
	if err != nil {
		return nil, err
	}
	mandatoryHash, err := digest(req.BaseProof.MandatoryHash, "mandatory_hash")
	if err != nil {
		return nil, err
	}
	scoped, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	publicKey := Multikey(&scoped.PublicKey)
	baseSignature, err := SignES256(s.keys.P256, BaseTuple(proofHash,
		publicKey, mandatoryHash))
	if err != nil {
		return nil, err
	}
	signatures := make([]string, 0, len(req.BaseProof.NonMandatory))
	for _, statement := range req.BaseProof.NonMandatory {
		sig, err := SignES256(scoped, []byte(statement))
		if err != nil {
			return nil, err
		}
		signatures = append(signatures, base64.StdEncoding.EncodeToString(sig))
	}
	return &BaseProofResponse{BaseProof: BaseProofComponents{
		PublicKey:     base64.StdEncoding.EncodeToString(publicKey),
		BaseSignature: base64.StdEncoding.EncodeToString(baseSignature),
		Signatures:    signatures,
	}}, nil
}

// BaseTuple is the message the workspace key signs: the proof
// hash, the multikey of the proof-scoped public key and the
// mandatory hash, concatenated.
func BaseTuple(proofHash, publicKey, mandatoryHash []byte) []byte {
	tuple := make([]byte, 0, len(proofHash)+len(publicKey)+len(mandatoryHash))
	tuple = append(tuple, proofHash...)
	tuple = append(tuple, publicKey...)
	return append(tuple, mandatoryHash...)
}

// Multikey renders a P-256 public key as the multicodec prefix
// followed by the compressed point, 35 bytes: 0x02 or 0x03 for
// the parity of y, then x.
func Multikey(pub *ecdsa.PublicKey) []byte {
	uncompressed, err := pub.Bytes()
	if err != nil || len(uncompressed) != 65 {
		// A key made by this package or parsed from a valid PEM
		// always renders; anything else is not a P-256 key.
		panic("signer: not a P-256 public key")
	}
	x, y := uncompressed[1:33], uncompressed[33:]
	out := make([]byte, 0, 35)
	out = append(out, p256MultikeyPrefix...)
	out = append(out, 0x02+y[len(y)-1]&1)
	return append(out, x...)
}

// SignES256 signs data with SHA-256 and a P-256 key and answers
// the raw 64-byte r||s signature the cryptosuite uses.
func SignES256(key *ecdsa.PrivateKey, data []byte) ([]byte, error) {
	sum := sha256.Sum256(data)
	r, s, err := ecdsa.Sign(rand.Reader, key, sum[:])
	if err != nil {
		return nil, err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return sig, nil
}

func digest(encoded, name string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("%s is not base64: %w", name, err)
	}
	if len(raw) != sha256.Size {
		return nil, fmt.Errorf("%s is %d bytes, not %d", name, len(raw),
			sha256.Size)
	}
	return raw, nil
}
