// Package pillar implements the block-producing role on the Zenon network.
//
// # Overview
//
// A pillar listens for ProducerEvent emissions from
// [github.com/zenon-network/go-zenon/consensus] and, when its configured
// coinbase matches the elected pillar for the current tick, authors a
// [github.com/zenon-network/go-zenon/chain/nom.Momentum] over the pending
// account blocks. The momentum is then handed to chain for insertion and to
// [github.com/zenon-network/go-zenon/protocol] for broadcast.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package pillar
