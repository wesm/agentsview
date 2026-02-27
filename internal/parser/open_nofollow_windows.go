//go:build windows

package parser

import "os"

// openNoFollow opens a file for reading. On Windows,
// O_NOFOLLOW is not available so we fall back to a regular
// open. The discovery-phase containment checks provide the
// primary defense on this platform.
func openNoFollow(path string) (*os.File, error) {
	return os.Open(path)
}
