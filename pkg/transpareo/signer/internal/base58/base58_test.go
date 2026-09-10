package base58

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

// TestMatchesThePlatformEncoder compares the encoding with what
// the platform's own base58 module answered for the same bytes,
// and decodes each back.
func TestMatchesThePlatformEncoder(t *testing.T) {
	data, err := os.ReadFile("testdata/platform_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Hex    string `json:"hex"`
		Base58 string `json:"base58"`
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors) < 10 {
		t.Fatalf("only %d vectors", len(vectors))
	}
	for _, v := range vectors {
		raw, _ := hex.DecodeString(v.Hex)
		if got := Encode(raw); got != v.Base58 {
			t.Errorf("%s: encoded %q, want %q", v.Hex, got, v.Base58)
		}
		back, err := Decode(v.Base58)
		if err != nil || !bytes.Equal(back, raw) {
			t.Errorf("%q: decoded %x (%v), want %s", v.Base58, back, err, v.Hex)
		}
	}
}

func TestDecodeRefusesTheAmbiguousLetters(t *testing.T) {
	for _, s := range []string{"0", "O", "I", "l", "z0"} {
		if _, err := Decode(s); err == nil {
			t.Errorf("%q was accepted", s)
		}
	}
}
