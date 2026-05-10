package node

import (
	"syscall"

	"github.com/pkg/errors"
)

var (
	// ErrDataDirUsed — the configured DataPath is already locked by
	// another znnd instance.
	ErrDataDirUsed = errors.New("dataDir already used by another process")
	// ErrNodeStopped — Stop or one of its subsystem teardown
	// helpers was called against a Node that has not been Started
	// (or has already been Stopped). The error string reads "node
	// not started" because the same sentinel covers both states.
	ErrNodeStopped = errors.New("node not started")
	// datadirInUseErrnos maps the platform-specific flock errnos
	// (EAGAIN=11 on Linux, EAGAIN=35 on Darwin, ERROR_LOCK_VIOLATION=32
	// on Windows) to the friendlier [ErrDataDirUsed].
	datadirInUseErrnos = map[uint]bool{11: true, 32: true, 35: true}
)

// convertFileLockError translates a flock errno into [ErrDataDirUsed]
// where applicable. Other errors pass through unchanged.
func convertFileLockError(err error) error {
	if errno, ok := err.(syscall.Errno); ok && datadirInUseErrnos[uint(errno)] {
		return ErrDataDirUsed
	}
	return err
}
