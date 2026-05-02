package metadata

import "runtime/debug"

// GitCommit is the VCS revision recorded by the Go toolchain at build time
// (the value of the `vcs.revision` setting in [debug.BuildInfo]). Empty if
// the binary was built outside a VCS-aware context.
//
// It is consumed by RPC `utility` calls and by structured log lines so a
// running node can be traced back to a specific commit.
var GitCommit = func() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				return setting.Value
			}
		}
	}
	return ""
}()
