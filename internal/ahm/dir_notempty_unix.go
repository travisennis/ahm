//go:build !windows

package ahm

import (
	"errors"
	"syscall"
)

// isDirNotEmpty reports whether err indicates that removing a directory failed
// because the directory still contains entries.
func isDirNotEmpty(err error) bool {
	return errors.Is(err, syscall.ENOTEMPTY)
}
