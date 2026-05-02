// Package subscribe implements the RPC subscription channel for momentum,
// account-block, and event notifications.
//
// # Overview
//
// subscribe runs a server that fans out chain events to connected RPC clients
// over WebSocket. It registers as a chain listener, buffers events per
// subscription, and tears each subscription down when its client disconnects
// or unsubscribes.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package subscribe
