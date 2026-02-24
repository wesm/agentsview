//go:build !windows

package parser

import (
	"os"
	"syscall"
)

// openNoFollow opens a file for reading without following
// symlinks at the final path component. On Unix systems this
// uses O_NOFOLLOW which causes the open to fail with ELOOP if
// the target is a symlink, closing the TOCTOU window between
// discovery validation and file read.
func openNoFollow(path string) (*os.File, error) {
	return os.OpenFile(
		path, os.O_RDONLY|syscall.O_NOFOLLOW, 0,
	)
}
