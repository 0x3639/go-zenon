// Package embedded is the dispatcher for Zenon's built-in system
// contracts.
//
// # Overview
//
// embedded maps a system address (Pillar, Sentinel, Stake, Token,
// Plasma, Spork, Swap, Accelerator, HTLC, Bridge, Liquidity) to its
// current implementation, taking into account active sporks. Each
// contract method implements three hooks ([Method.GetPlasma],
// [Method.ValidateSendBlock], [Method.ReceiveBlock]) and is
// resolved through [GetEmbeddedMethod] given a target address and
// the call's 4-byte ABI selector.
//
// # Spork Tiering
//
// Four pre-built dispatch tables are stacked in increasing
// activation order:
//
//   - originEmbedded — genesis-time contracts (Plasma, Pillar,
//     Token, Sentinel, Swap, Stake, Spork) plus donation-only
//     stubs for Liquidity and Accelerator.
//   - acceleratorEmbedded — adds the full Accelerator contract and
//     CollectReward wiring across pillar/sentinel/stake.
//   - bridgeAndLiquidityEmbedded — adds Bridge and the full
//     Liquidity method set.
//   - htlcEmbedded — adds the HTLC contract.
//
// [GetEmbeddedMethod] selects the table based on which sporks the
// caller's [vm_context.AccountVmContext] reports as enforced.
//
// # Sub-packages
//
//   - definition — per-contract ABI definitions and storage record
//     types.
//   - implementation — per-contract method behavior.
//   - tests — embedded-contract integration test suite.
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/vm] — primary consumer;
//     calls [GetEmbeddedMethod] from the per-account-block VM.
//   - [github.com/zenon-network/go-zenon/vm/abi] — supplies the
//     ABI codec used for selector dispatch and storage encoding.
//   - [github.com/zenon-network/go-zenon/vm/constants] — supplies
//     the [constants.PlasmaTable] consumed by [Method.GetPlasma].
//   - [github.com/zenon-network/go-zenon/common/types] — defines
//     the per-contract address constants (PillarContract,
//     SentinelContract, …) the dispatcher keys on.
package embedded
