// Package pow generates and verifies the proof-of-work nonces that satisfy
// the plasma difficulty for an account block.
//
// # Overview
//
// Plasma may be paid either by fusing QSR (steady yield through the Plasma
// embedded contract) or by burning a small proof of work per block. pow
// implements the second path. The PoW seed is `Hash(address ||
// previousHash)` — deliberately decoupled from per-block payload so a
// caller can pre-compute the nonce while the rest of the block is still
// being assembled.
//
// # Key Concepts
//
//   - Difficulty — a [big.Int] target. Higher values demand more hashes;
//     [GetThresholdByDifficulty] converts the target to the 64-bit
//     threshold the resulting hash must reach (the comparison in
//     [greaterDifficulty] returns true on equal values, so the rule is
//     "≥ threshold").
//   - Nonce — 8 random bytes that, when hashed with the seed, yield a
//     value at or above the threshold. Stored in
//     [github.com/zenon-network/go-zenon/chain/nom.AccountBlock.Nonce].
//   - Seed — `Hash(address || previousHash)`. Computed by
//     [GetAccountBlockHash].
//
// # Usage
//
// Search for a nonce when constructing an account block that pays plasma
// via PoW:
//
//	seed := pow.GetAccountBlockHash(block)
//	block.Nonce = pow.GetPoWNonce(big.NewInt(int64(block.Difficulty)), seed)
//
// Verify a nonce on an inbound block (used by the verifier):
//
//	if !pow.CheckPoWNonce(block) { /* reject */ }
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/chain/nom] — defines the
//     `Difficulty`/`Nonce` fields on [chain/nom.AccountBlock] consumed
//     here.
//   - [github.com/zenon-network/go-zenon/common/crypto] — supplies the
//     hash function used to score nonces.
//   - [github.com/zenon-network/go-zenon/wallet] — provides the
//     cryptographically-secure entropy source for nonce search seeds.
//   - [github.com/zenon-network/go-zenon/verifier] — calls [CheckPoWNonce]
//     during stateless block validation.
package pow
