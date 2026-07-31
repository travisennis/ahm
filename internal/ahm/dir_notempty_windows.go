//go:build windows

package ahm

import (
	"errors"
	"syscall"
)

// isDirNotEmpty reports whether err indicates that removing a directory failed
// because the directory still contains entries. Windows reports this condition
// as ERROR_DIR_NOT_EMPTY rather than the POSIX ENOTEMPTY errno, so the raw
// Unix comparison would not match and tolerated non-empty-directory removals
// would surface as hard errors.
func isDirNotEmpty(err error) bool {
	return errors.Is(err, syscall.ERROR_DIR_NOT_EMPTY)
}
