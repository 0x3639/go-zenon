// Package mock provides a test harness that builds an in-memory
// Zenon node suitable for unit and integration tests.
//
// # Overview
//
// mock wires a fully working but ephemeral chain, consensus, and VM
// supervisor against an in-memory leveldb plus the embedded
// [github.com/zenon-network/go-zenon/chain/genesis/mock] genesis.
// Tests use the [MockZenon] interface to drive scenarios end to end
// — issue send blocks, force receives, advance momentums — without
// standing up a real network or pinning real time.
//
// # Key Capabilities
//
// Beyond the standard [zenon.Zenon] surface, [MockZenon] exposes:
//
//   - [MockZenon.InsertNewMomentum] / [MockZenon.InsertMomentumsTo]
//     — drive consensus forward by one momentum (or up to a target
//     height) without waiting for wall-clock time. Time is
//     virtualised via [mockClock], which sources its "now" from the
//     frontier momentum's timestamp.
//   - [MockZenon.CallContract] — submit an embedded-contract send
//     and verify the contract's receive-side outcome via the late
//     caller pattern (the assertion fires after the next
//     InsertNewMomentum cements the receive).
//   - [MockZenon.InsertSendBlock] / [MockZenon.InsertReceiveBlock]
//     — finer-grained block insertion with expected-error and
//     expected-VM-changes assertions.
//   - [MockZenon.ExpectBalance] / [MockZenon.SaveLogs] — common
//     assertions used in embedded-contract tests.
//
// Producer logs from the pillar manager are intercepted by
// [ProducerLogSaver]; embedded-block error strings are looked up by
// send-block hash so [MockZenon.CallContract] can attribute them
// to the originating test step ([MockContractCaller]).
//
// # Limitations
//
// Verifier, Protocol, Producer, and Config accessors return nil —
// tests that need those subsystems should use a real Zenon instead.
// Broadcaster is wired to the mock itself so that block-publishing
// flows (e.g. the RPC PublishRawTransaction path) round-trip
// through CreateAccountBlock / CreateMomentum.
//
// # Generated Files
//
// None. Files are Zenon-specific (no upstream header).
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/zenon] — production
//     orchestrator; [MockZenon] satisfies its [Zenon] interface.
//   - [github.com/zenon-network/go-zenon/chain/genesis/mock] —
//     keypair set and embedded genesis used by the harness.
//   - [github.com/zenon-network/go-zenon/vm/embedded/tests] —
//     primary consumer of this package.
package mock
