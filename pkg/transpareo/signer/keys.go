package signer

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
)

// Keys are the two private keys of a workspace: P-256 for the
// passport's selective-disclosure proof, Ed25519 for whole-
// document proofs.
type Keys struct {
	P256    *ecdsa.PrivateKey
	Ed25519 ed25519.PrivateKey
}

// Generate mints both keys.
func Generate() (*Keys, error) {
	p256, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	_, ed, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Keys{P256: p256, Ed25519: ed}, nil
}

// ParsePrivateKey reads a PEM private key: PKCS#8 as keygen
// writes it, or the SEC 1 form of an OpenSSL-made P-256 key.
func ParsePrivateKey(data []byte) (any, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("no PEM block found")
	}
	switch block.Type {
	case "PRIVATE KEY":
		return x509.ParsePKCS8PrivateKey(block.Bytes)
	case "EC PRIVATE KEY":
		return x509.ParseECPrivateKey(block.Bytes)
	}
	return nil, fmt.Errorf("unexpected PEM block %q", block.Type)
}

// LoadP256 reads the P-256 private key at path.
func LoadP256(path string) (*ecdsa.PrivateKey, error) {
	key, err := loadKey(path)
	if err != nil {
		return nil, err
	}
	ec, ok := key.(*ecdsa.PrivateKey)
	if !ok || ec.Curve != elliptic.P256() {
		return nil, fmt.Errorf("%s: not a P-256 key", path)
	}
	return ec, nil
}

// LoadEd25519 reads the Ed25519 private key at path.
func LoadEd25519(path string) (ed25519.PrivateKey, error) {
	key, err := loadKey(path)
	if err != nil {
		return nil, err
	}
	ed, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%s: not an Ed25519 key", path)
	}
	return ed, nil
}

func loadKey(path string) (any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	key, err := ParsePrivateKey(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return key, nil
}

// ParsePublicKey reads a PEM public key (SubjectPublicKeyInfo),
// the form the platform publishes its request-signing key in.
func ParsePublicKey(data []byte) (any, error) {
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "PUBLIC KEY" {
		return nil, errors.New("no PUBLIC KEY block found")
	}
	return x509.ParsePKIXPublicKey(block.Bytes)
}

// ParseEd25519PublicKey reads the PEM of an Ed25519 public key.
func ParseEd25519PublicKey(data []byte) (ed25519.PublicKey, error) {
	key, err := ParsePublicKey(data)
	if err != nil {
		return nil, err
	}
	ed, ok := key.(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("not an Ed25519 public key")
	}
	return ed, nil
}

// PrivatePEM renders a private key as PKCS#8 PEM.
func PrivatePEM(key any) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}),
		nil
}

// PublicPEM renders the public half of a private key as
// SubjectPublicKeyInfo PEM, what the application manager's BYOK
// form takes.
func PublicPEM(key any) ([]byte, error) {
	var pub any
	switch k := key.(type) {
	case *ecdsa.PrivateKey:
		pub = &k.PublicKey
	case ed25519.PrivateKey:
		pub = k.Public()
	default:
		return nil, fmt.Errorf("unsupported key type %T", key)
	}
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}),
		nil
}

// WritePrivateKey writes a key as PKCS#8 PEM readable by its
// owner only, and refuses to replace an existing file unless
// force is set.
func WritePrivateKey(path string, key any, force bool) error {
	data, err := PrivatePEM(key)
	if err != nil {
		return err
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !force {
		flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	}
	f, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
