// Package constants defines the VM-wide numeric limits and the
// canonical error values that contract execution returns.
//
// # Overview
//
// constants is a leaf package consumed by everything in the VM and
// embedded-contract layers. It owns four files:
//
//   - consensus.go — the [Consensus] tuning struct and the live
//     [ConsensusConfig].
//   - plasma.go — plasma cost constants ([AccountBlockBasePlasma],
//     [PoWDifficultyPerPlasma], etc.) and the [PlasmaTable] struct
//     contract methods receive in [embedded.Method.GetPlasma].
//   - embedded.go — per-contract tunables: pillar / sentinel / stake
//     amounts, accelerator parameters, token / spork / swap /
//     liquidity / bridge constants, and the network reward
//     schedules ([NetworkZnnRewardConfig], [NetworkQsrRewardConfig])
//     plus the helpers that index into them.
//   - errors.go — every [errors.New] sentinel returned by the VM
//     and the embedded contracts, grouped by contract.
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/vm] — primary consumer of
//     plasma constants and [ErrVmRunPanic].
//   - [github.com/zenon-network/go-zenon/vm/embedded] — consumes
//     per-contract tunables and the [PlasmaTable].
//   - [github.com/zenon-network/go-zenon/consensus] — reads
//     [ConsensusConfig].
//   - [github.com/zenon-network/go-zenon/verifier] — surfaces a
//     subset of these errors to API consumers.
package constants
