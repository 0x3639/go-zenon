// Package consensus runs the pillar-election scheduler that determines which
// pillar produces each momentum.
//
// # Overview
//
// Time is divided into ticks; each tick maps deterministically to one elected
// pillar through a weighted shuffle of the registered pillar set. consensus
// owns the tick scheduler, the points system that adjusts election weight by
// historical performance, and the ProducerEvent emission that
// [github.com/zenon-network/go-zenon/pillar] reacts to.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package consensus
