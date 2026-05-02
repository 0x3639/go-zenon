// Package vm_context provides the per-block execution context handed to
// embedded-contract methods.
//
// # Overview
//
// A vm_context exposes read access to the current chain state, the momentum
// being produced, the account block under execution, the configured plasma
// budget, and the helpers that contracts use to emit descendant sends. It is
// the only API surface a contract sees during execution.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package vm_context
