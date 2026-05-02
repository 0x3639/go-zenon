// Package nom defines the on-chain block model: account blocks, momentums,
// and the transaction wrappers that bind blocks to their atomic state changes.
//
// # Overview
//
// nom is the schema layer everything else depends on. It owns block types
// (`UserSend`, `UserReceive`, `ContractSend`, `ContractReceive`,
// `GenesisReceive`), canonical hashing, Ed25519 signature attachment, and the
// `*Transaction` types that pair a block with its [common/db.Patch].
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package nom
