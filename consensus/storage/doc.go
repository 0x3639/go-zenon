// Package storage persists consensus state — election results, points,
// epoch snapshots — to LevelDB.
//
// # Overview
//
// Election outputs and per-pillar points are encoded as protobuf messages and
// stored alongside the chain database. storage exposes typed accessors over
// the raw protobuf records.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package storage
