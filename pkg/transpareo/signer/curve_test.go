package signer

import (
	"crypto/elliptic"
	"math/big"
)

// decompressP256 recovers the point behind a compressed encoding
// (0x02 or 0x03 and the x coordinate), as a verifier of the
// multikey does. y^2 = x^3 - 3x + b over the P-256 field.
func decompressP256(compressed []byte) (*big.Int, *big.Int) {
	if len(compressed) != 33 || (compressed[0] != 2 && compressed[0] != 3) {
		return nil, nil
	}
	params := elliptic.P256().Params()
	x := new(big.Int).SetBytes(compressed[1:])
	rhs := new(big.Int).Exp(x, big.NewInt(3), params.P)
	rhs.Sub(rhs, new(big.Int).Mul(x, big.NewInt(3)))
	rhs.Add(rhs, params.B)
	rhs.Mod(rhs, params.P)
	y := new(big.Int).ModSqrt(rhs, params.P)
	if y == nil {
		return nil, nil
	}
	if y.Bit(0) != uint(compressed[0]-2) {
		y.Sub(params.P, y)
	}
	return x, y
}

func p256Curve() elliptic.Curve { return elliptic.P256() }
