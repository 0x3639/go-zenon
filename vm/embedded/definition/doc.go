// Package definition holds the ABI definitions and Go type wrappers for each
// embedded contract.
//
// # Overview
//
// Every embedded contract has a paired definition file here that declares its
// methods, events, and storage record types. Implementations under
// [github.com/zenon-network/go-zenon/vm/embedded/implementation] consume
// these definitions to decode call data and to read or write contract state.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package definition
