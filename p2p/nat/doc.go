// Package nat handles NAT traversal so a node behind a home router can
// be reached by external peers.
//
// # Overview
//
// nat is a port of the go-ethereum NAT helper. It abstracts over three
// strategies behind a single [Interface]:
//
//   - [UPnP] — multicast SSDP discovery + SOAP control of an Internet
//     Gateway Device (IGDv1 or IGDv2).
//   - [PMP] — NAT-PMP RPC against the LAN gateway.
//   - [ExtIP] — a no-op stub used when the operator manually forwards
//     ports and tells us the external address.
//
// [Any] races UPnP and NAT-PMP in parallel and returns whichever
// answers first. [Parse] turns a config string ("upnp", "pmp:1.2.3.4",
// "extip:5.6.7.8", "any", "none") into the corresponding interface.
//
// # Mapping Lifecycle
//
// [Map] is the long-running entry point: it adds a port mapping with
// [mapTimeout]=20 minutes lifetime, then refreshes it every
// [mapUpdateInterval]=15 minutes until the supplied close channel is
// signalled, at which point it deletes the mapping. Callers ([p2p.Server]
// and the discovery transport) launch one Map goroutine per protocol
// (TCP for RLPx, UDP for discovery).
//
// # Concurrency
//
// [Interface] implementations are safe for concurrent use. The
// [autodisc] wrapper used by [Any] / [UPnP] / [PMP] (with auto-detected
// gateway) lazily blocks the first method call until detection
// completes, then memoises the resolved backend.
//
// # Generated Files
//
// None. The .go files in this package carry the original go-ethereum
// LGPL-3.0+ headers.
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/p2p] — uses [Map] from
//     Server.startListening to expose the TCP listen port.
//   - [github.com/zenon-network/go-zenon/p2p/discover] — uses [Map] to
//     expose the UDP discovery port.
package nat
