package ahm

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// writeFileAtomic writes data to the specified path atomically.
//
// It creates parent directories if needed. The write strategy is:
//  1. Create a unique sibling temp file in the same directory.
//  2. Write data and sync to disk.
//  3. Rename the temp file to the target path (atomic on Unix when
//     source and destination are on the same filesystem).
//  4. Sync the parent directory to ensure the rename is durable.
//
// Using a unique temp file name per invocation avoids races when multiple
// processes write the same path concurrently (the previous fixed-name .tmp
// strategy allowed one process's "remove stale tmp" to delete another
// process's temp file before the rename).
//
// If any step before the rename fails, the original file is left intact
// and the temp file is cleaned up.
func writeFileAtomic(path string, data []byte, perm fs.FileMode) error {
	// Reject path traversal that would escape the intended directory tree.
	clean := filepath.Clean(path)
	if clean != path {
		return fmt.Errorf("atomic write: non-canonical path %q", path)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil { // #nosec G301 // 0755 is the standard directory permission for workflow files
		return fmt.Errorf("atomic write: create dir %s: %w", dir, err)
	}

	// Create a unique temp file in the target directory. Using a unique name
	// prevents races when multiple processes write the same file concurrently.
	f, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("atomic write: create temp in %s: %w", dir, err)
	}
	tmpPath := f.Name()

	// Set the requested permissions. os.CreateTemp creates with 0o600 by default.
	if err := f.Chmod(perm); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("atomic write: chmod temp %s: %w", tmpPath, err)
	}

	// Write the data.
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("atomic write: write tmp %s: %w", tmpPath, err)
	}

	// Sync the temp file.
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("atomic write: sync tmp %s: %w", tmpPath, err)
	}

	// Close the file before rename (Windows compatibility, good practice).
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("atomic write: close tmp %s: %w", tmpPath, err)
	}

	// Atomic rename.
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("atomic write: rename %s -> %s: %w", tmpPath, path, err)
	}

	// Sync the parent directory so the rename survives a crash.
	if err := fsyncDir(dir); err != nil {
		return fmt.Errorf("atomic write: sync dir %s: %w", dir, err)
	}

	return nil
}

// fsyncDir opens the directory, calls Sync, and closes it.
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

// cleanupStaleTemps scans the .agents directory inside root for orphaned
// .tmp files — files ending in ".tmp" whose corresponding non-.tmp path
// does not exist or is not a regular file. Such files can be left behind
// by a crash during an atomic write.
//
// Only regular files under .agents/ are considered; files in subdirectories
// like .git/ are not scanned.
func cleanupStaleTemps(root string) error {
	agentsDir := filepath.Join(root, ".agents")
	stat, err := os.Stat(agentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !stat.IsDir() {
		return nil
	}

	return filepath.WalkDir(agentsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".tmp") {
			return nil
		}

		// Resolve the path to a clean absolute form to avoid
		// symlink-TOC-TOU attacks (gosec G122).
		cleanPath := filepath.Clean(path)
		if !strings.HasPrefix(cleanPath, filepath.Clean(agentsDir)) {
			return nil
		}

		// Check if the corresponding non-.tmp path exists as a regular file.
		origPath := strings.TrimSuffix(cleanPath, ".tmp")
		origStat, origErr := os.Stat(origPath)
		if origErr == nil && origStat.Mode().IsRegular() {
			// The real file exists; the .tmp file is stale from a crash
			// that succeeded past the rename, or from a write that was
			// interrupted before the rename. Either way, it's safe to remove.
			return os.Remove(cleanPath) //nolint:gosec // best-effort cleanup of .tmp files within .agents/
		}
		if os.IsNotExist(origErr) {
			// The real file doesn't exist; the .tmp is an orphan.
			return os.Remove(cleanPath) //nolint:gosec // best-effort cleanup of .tmp files within .agents/
		}
		// If we can't stat the original, skip (don't remove anything risky).
		return nil
	})
}
