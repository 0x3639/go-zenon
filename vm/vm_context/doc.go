// Package vm_context provides the per-block and per-momentum execution
// contexts handed to embedded-contract methods and to the VM
// supervisor.
//
// # Overview
//
// vm_context owns two interfaces:
//
//   - [AccountVmContext] — per-account-block surface. Bundles chain
//     and consensus reads, balance helpers, lifecycle hooks
//     (Save/Reset/Done for contract-receive rollback), and
//     spork-status checks.
//   - [MomentumVMContext] — per-momentum surface, equivalent to
//     [store.Momentum] for callers that only need to admit
//     account-block transactions into a momentum view.
//
// # Key Concepts
//
//   - Working view vs snapshot — [accountVmContext.Save] parks the
//     current account view and installs a fresh snapshot for
//     contract execution. [accountVmContext.Reset] discards the
//     snapshot's writes; [accountVmContext.Done] applies them onto
//     the parked view.
//   - Genesis variants — [NewGenesisAccountContext] (per-contract
//     seeding) and [NewGenesisMomentumVMContext] (in-memory genesis
//     momentum context) are used during genesis assembly only.
//
// # Concurrency
//
// Contexts are not goroutine-safe; one is constructed per executed
// block / momentum and not shared.
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/vm] — primary consumer;
//     constructs contexts via the supervisor and hands them to
//     contract methods.
//   - [github.com/zenon-network/go-zenon/vm/embedded] — contract
//     methods read and mutate state through this context.
//   - [github.com/zenon-network/go-zenon/chain/store] — the
//     underlying [store.Account] / [store.Momentum] views the
//     contexts wrap.
//   - [github.com/zenon-network/go-zenon/consensus/api] — supplies
//     the [api.PillarReader] embedded in [AccountVmContext].
package vm_context
