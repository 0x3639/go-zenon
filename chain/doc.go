// Package chain is the ledger orchestration layer of the Network of Momentum.
//
// # Overview
//
// chain owns the dual-ledger primitives — an account pool of per-account block
// chains and a momentum pool of consensus blocks — and serializes all
// mutations through a single insert lock. All inserts pass through here:
// account blocks from RPC and from contract execution, and momentums from the
// elected pillar or from peer sync.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package chain
