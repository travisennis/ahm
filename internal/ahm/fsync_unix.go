//go:build !windows

package ahm

import "os"

// fsyncDir opens the directory, calls Sync, and closes it. Syncing the parent
// directory after an atomic rename makes the rename itself durable across a
// crash; the temp-file sync alone only guarantees the file content survives.
func fsyncDir(path string) error {
	f, err := os.OpenFile(path, os.O_RDONLY, 0) // #nosec G304 // path is canonical; caller validates non-canonical paths
	if err != nil {
		return err
	}
	err = f.Sync()
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}
