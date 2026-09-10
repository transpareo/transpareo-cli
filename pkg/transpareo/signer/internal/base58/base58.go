// Package base58 encodes bytes in base58btc, the Bitcoin
// alphabet without 0, O, I and l, which the eddsa-jcs-2022
// cryptosuite uses for proof values behind a "z" prefix. The
// prefix is the caller's business.
package base58

import (
	"errors"
	"math/big"
	"strings"
)

const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZ" +
	"abcdefghijkmnopqrstuvwxyz"

var (
	base    = big.NewInt(int64(len(alphabet)))
	indexOf = func() map[rune]int64 {
		m := make(map[rune]int64, len(alphabet))
		for i, c := range alphabet {
			m[c] = int64(i)
		}
		return m
	}()
)

// Encode answers the base58btc text of data. Leading zero bytes
// become leading "1"s, one per byte.
func Encode(data []byte) string {
	zeros := 0
	for zeros < len(data) && data[zeros] == 0 {
		zeros++
	}
	n := new(big.Int).SetBytes(data)
	var out []byte
	mod := new(big.Int)
	for n.Sign() > 0 {
		n.DivMod(n, base, mod)
		out = append(out, alphabet[mod.Int64()])
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return strings.Repeat("1", zeros) + string(out)
}

// Decode answers the bytes behind base58btc text, or an error on
// a character outside the alphabet.
func Decode(s string) ([]byte, error) {
	ones := 0
	for ones < len(s) && s[ones] == '1' {
		ones++
	}
	n := new(big.Int)
	for _, c := range s {
		i, ok := indexOf[c]
		if !ok {
			return nil, errors.New("base58: invalid character " +
				strconv(c))
		}
		n.Mul(n, base)
		n.Add(n, big.NewInt(i))
	}
	body := n.Bytes()
	out := make([]byte, ones+len(body))
	copy(out[ones:], body)
	return out, nil
}

func strconv(c rune) string { return string([]rune{'"', c, '"'}) }
