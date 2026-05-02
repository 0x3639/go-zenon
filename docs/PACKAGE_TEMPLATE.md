# Package Doc Template

Copy this template into a new `doc.go` for each package and fill in. Keep section headers exactly as shown — they render as headings on pkg.go.dev.

```go
// Package PKGNAME ONE-SENTENCE-SUMMARY-ENDING-WITH-PERIOD.
//
// # Overview
//
// Two to six sentences explaining what this package does and why it exists.
// Mention the layer it sits in (ledger, consensus, VM, RPC, …) and what depends
// on it. Avoid restating the summary.
//
// # Key Concepts
//
//   - TermOne — definition. Cross-references the [ARCHITECTURE.md] glossary
//     where applicable.
//   - TermTwo — definition.
//
// # Usage
//
// Typical call sequence for a consumer of this package, with doc-links to the
// entry points:
//
//   1. Create with [Constructor].
//   2. Initialize with [Type.Init].
//   3. Drive via [Type.Method].
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/upstream] — what this package consumes.
//   - [github.com/zenon-network/go-zenon/downstream] — what consumes this package.
package PKGNAME
```

## Notes

- The blank line after each section header is required for godoc to render the heading.
- Lists must be indented with three spaces (`   - item`) for godoc to render bullets.
- Doc-links use `[pkg.Symbol]` (in-module) or `[full/import/path.Symbol]` (cross-module).
- One sentence summary is the line shown in pkg.go.dev indexes — keep it informative and self-contained.

## Minimal Example (small leaf package)

```go
// Package pow generates plasma proof-of-work nonces.
//
// # Overview
//
// Plasma costs may be paid either by fusing QSR or by burning a small
// proof-of-work per account block. This package implements the PoW path.
//
// # Key Concepts
//
//   - Difficulty — target zeros required in the resulting hash.
//   - Nonce — 8-byte value satisfying the difficulty.
//
// # Usage
//
// Call [GetPoWNonce] with the block-derived seed and the target difficulty.
//
// # Related Packages
//
//   - [github.com/zenon-network/go-zenon/chain/nom] — defines the AccountBlock
//     fields (`Difficulty`, `Nonce`) consumed here.
package pow
```

## Larger Example (subsystem entry point)

See [`rpc/server/doc.go`](../rpc/server/doc.go) — it is the existing reference example, ported from go-ethereum.
