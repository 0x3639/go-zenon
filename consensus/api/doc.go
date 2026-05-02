// Package api exposes the read-only consensus query surface used by
// the RPC layer and by other in-process subsystems.
//
// # Overview
//
// api separates queries (election lookups, points queries, pillar
// weights) from mutations so that callers reading consensus state
// cannot accidentally reach into the scheduler. The
// [github.com/zenon-network/go-zenon/consensus] package's [API] struct
// implements this interface; consumers receive a [PillarReader] from
// either [consensus.Consensus.FrontierPillarReader] (live view) or
// [consensus.Consensus.FixedPillarReader] (historical view).
//
// # Key Concepts
//
//   - PillarReader — the read-only handle. Bound to a momentum store
//     and the points / election subsystems behind the scenes.
//   - EpochStats — aggregated per-epoch view across every pillar.
//   - EpochPillarStats — per-pillar, per-epoch breakdown of produced
//     vs expected blocks plus weight.
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/consensus] — implements the
//     [PillarReader] interface.
//   - [github.com/zenon-network/go-zenon/rpc/api/embedded] —
//     primary consumer; surfaces these stats over JSON-RPC.
package api
