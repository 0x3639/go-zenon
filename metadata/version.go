//go:build !libznn

package metadata

// Version is the human-readable version string baked into the standalone
// `znnd` binary. The libznn build defines its own [Version] constant in
// version_libznn.go; the build tags ensure exactly one is present per
// build.
const (
	Version = "v0.0.8"
)
