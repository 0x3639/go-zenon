//go:build libznn && !detached

// Command libznn builds the C-shared library variant of the node, exporting
// the znnd functionality through a stable C ABI.
//
// # Overview
//
// libznn is built with `make libznn` (which invokes `go build -buildmode=c-shared
// -tags libznn ./cmd/libznn`). Other languages link against the resulting
// `libznn.dll`/`.dylib`/`.so` to embed a Zenon node.
//
// Per-command documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package main
