package pow

import (
	"encoding/binary"
	"math/big"

	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common"
	"github.com/zenon-network/go-zenon/common/crypto"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/wallet"
)

// GetAccountBlockHash derives the seed an account block's PoW operates on:
// the hash of the sender's address concatenated with the previous-block
// hash. The seed deliberately omits per-block payload so the PoW can be
// pre-computed before the rest of the block is finalized.
func GetAccountBlockHash(block *nom.AccountBlock) types.Hash {
	return types.NewHash(append(block.Address.Bytes(), block.PreviousHash.Bytes()...))
}

// CheckPoWNonce verifies that block.Nonce satisfies block.Difficulty. The
// verifier calls this whenever a block claims plasma via PoW.
func CheckPoWNonce(block *nom.AccountBlock) bool {
	dataHash := GetAccountBlockHash(block)
	target := getTargetByDifficulty(block.Difficulty)
	calc := hashWithNonce(dataHash, block.Nonce.Serialize())
	return greaterDifficulty(calc, target[:])
}

// GetPoWNonce searches for a nonce that satisfies difficulty against the
// supplied dataHash. The search starts at a cryptographically random
// 8-byte seed and increments until the resulting hash crosses the
// difficulty threshold.
//
// Concurrency: each call uses fresh entropy and is safe to run in parallel.
func GetPoWNonce(difficulty *big.Int, dataHash types.Hash) []byte {
	rng := wallet.GetEntropyCSPRNG(8)
	calc, target := getTarget(difficulty, dataHash, rng)
	for {
		if greaterDifficulty(crypto.Hash(calc), target[:]) {
			break
		}
		calc = quickInc(calc)
	}
	var arr [8]byte
	copy(arr[:], calc[:8])
	return arr[:]
}

// getTarget assembles the 40-byte hash input (`nonce || dataHash`) and
// the 8-byte little-endian difficulty target the search compares against.
func getTarget(difficulty *big.Int, data types.Hash, nonce []byte) ([]byte, [8]byte) {
	threshold := GetThresholdByDifficulty(difficulty)
	calc := make([]byte, 40)
	l := copy(calc, nonce[:])
	copy(calc[l:], data[:])
	target := Uint64ToByteArray(threshold)
	return calc, target
}

// Uint64ToByteArray encodes i as 8 little-endian bytes. Little-endian here
// (as opposed to the big-endian helpers in [common]) matches the on-chain
// PoW byte order.
func Uint64ToByteArray(i uint64) [8]byte {
	var n [8]byte
	binary.LittleEndian.PutUint64(n[:], i)
	return n
}

// quickInc increments x in place as a little-endian counter with byte-level
// carry. Returns x for chaining; the underlying buffer is mutated.
func quickInc(x []byte) []byte {
	for i := 0; i < len(x); i++ {
		x[i] = x[i] + 1
		if x[i] != 0 {
			return x
		}
	}
	return x
}

// GetThresholdByDifficulty returns the unsigned 64-bit threshold a hash
// must exceed to satisfy difficulty: `2^64 - 2^64/difficulty`. Higher
// difficulty pushes the threshold closer to 2^64, leaving a smaller fraction
// of nonces valid.
//
// Panics if difficulty is nil — every call site must supply a positive
// difficulty.
func GetThresholdByDifficulty(difficulty *big.Int) uint64 {
	if difficulty != nil {
		x := big.NewInt(2).Exp(big.NewInt(2), big.NewInt(64), nil)
		y := big.NewInt(0).Quo(x, difficulty)
		x.Sub(x, y)
		return x.Uint64()
	}
	panic("No difficulty supplied to compute PoW")
}

// hashWithNonce returns the first 8 bytes of `Hash(nonce || dataHash)`.
// Verifier-side mirror of the search loop in [GetPoWNonce].
func hashWithNonce(dataHash types.Hash, nonce []byte) []byte {
	calc := make([]byte, 40)
	l := copy(calc, nonce[:])
	copy(calc[l:], dataHash[:])
	return crypto.Hash(calc)[:8]
}

// getTargetByDifficulty is the uint64 form of [GetThresholdByDifficulty]
// returning the 8-byte little-endian encoding directly. A zero difficulty
// produces an all-zero target — every hash satisfies it (no PoW required).
func getTargetByDifficulty(difficulty uint64) [8]byte {
	if difficulty == 0 {
		return [8]byte{}
	}
	// 2^64 - (2^64 / difficulty)
	x := new(big.Int).Exp(common.Big2, common.Big64, nil)
	y := big.NewInt(0).Quo(x, big.NewInt(int64(difficulty)))
	x.Sub(x, y)
	var target [8]byte
	binary.LittleEndian.PutUint64(target[:], x.Uint64())
	return target
}

// greaterDifficulty reports whether the little-endian-encoded x is
// numerically greater than (or equal to) y. Equal values return true so a
// zero-difficulty target is trivially satisfied.
func greaterDifficulty(x, y []byte) bool {
	// Bytes are stored in LittleEndian ordering.
	for i := 7; i >= 0; i-- {
		if x[i] > y[i] {
			return true
		}
		if x[i] < y[i] {
			return false
		}
		if x[i] == y[i] {
			continue
		}
	}
	return true
}
