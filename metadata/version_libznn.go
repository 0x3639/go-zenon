//go:build libznn

package metadata

// Version is the human-readable version string baked into the libznn
// (C-shared library) build. Suffixed with `-libznn` so consumers can tell
// it apart from the standalone `znnd` binary at runtime.
const (
	Version = "v0.0.8-libznn"
)
