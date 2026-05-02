package common

import (
	"math/big"
)

// Pre-allocated [big.Int] constants used throughout the codebase. These
// avoid allocating fresh big.Ints in hot paths — VM execution, plasma
// arithmetic, and the embedded contracts all reuse them. Values:
//   - Big0..Big256: small constants.
//   - BigP255 = 2^255, BigP255m1 = 2^255 - 1.
//   - BigP256 = 2^256, BigP256m1 = 2^256 - 1 (the unsigned 256-bit max).
var (
	Big0      = big.NewInt(0)
	Big1      = big.NewInt(1)
	Big2      = big.NewInt(2)
	Big32     = big.NewInt(32)
	Big64     = big.NewInt(64)
	Big100    = big.NewInt(100)
	Big255    = big.NewInt(255)
	Big256    = big.NewInt(256)
	BigP255   = new(big.Int).Exp(Big2, Big255, nil)
	BigP255m1 = new(big.Int).Sub(BigP255, big.NewInt(1))
	BigP256   = new(big.Int).Exp(Big2, Big256, nil)
	BigP256m1 = new(big.Int).Sub(BigP256, big.NewInt(1))
)

// MinInt64 returns the smaller of x and y.
func MinInt64(x, y int64) int64 {
	if x < y {
		return x
	}
	return y
}

// MaxInt64 returns the larger of x and y.
func MaxInt64(x, y int64) int64 {
	if x > y {
		return x
	}
	return y
}
