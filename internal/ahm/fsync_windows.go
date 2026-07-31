//go:build windows

package ahm

// fsyncDir is a no-op on Windows. Directory fsync is not possible there:
// opening a directory with O_RDONLY (FlushFileBuffers on a read-only handle)
// fails with ERROR_ACCESS_DENIED, so every atomic write would report an error
// after the rename had already succeeded. Skipping the directory sync is the
// established cross-platform compromise for atomic writes (see ADR 001); the
// rename itself remains atomic, and only the extra crash-durability guarantee
// for the rename is lost.
func fsyncDir(_ string) error {
	return nil
}
