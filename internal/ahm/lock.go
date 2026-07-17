package ahm

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const workflowRecordLockName = "workflow-records"

var (
	workflowLockRetryDelay = 10 * time.Millisecond
	workflowLockTimeout    = 10 * time.Second
	workflowLockStaleAfter = 30 * time.Minute
)

// withWorkflowRecordLock runs f while holding the repository-scoped workflow
// record-mutation lock. When mutating is false, f is called without a lock
// (used for dry-run and read-only preview paths).
func (a *app) withWorkflowRecordLock(mutating bool, f func() error) error {
	if !mutating {
		return f()
	}
	release, err := acquireWorkflowRecordLock(a.opts.root)
	if err != nil {
		return err
	}
	defer func() {
		if err := release(); err != nil {
			fmt.Fprintln(a.err, err)
		}
	}()
	return f()
}

// workflowRecordLockRoot returns the lock directory for the repository's
// current storage mode. Commands that wait for the lock re-evaluate this on
// each retry so they follow a storage-mode transition (e.g. records migration)
// rather than waiting on an abandoned lock directory.
func workflowRecordLockRoot(root string) string {
	return filepath.Join(root, workflowPathsFor(root).recordsDir, ".lock")
}

// acquireWorkflowRecordLock returns the release function for the single
// repository-scoped workflow record-mutation lock. The lock lives in the
// active storage root (`.agents/.lock` or `.ahm/.lock`) and is re-resolved on
// each retry so waiters migrate with a storage-mode transition.
func acquireWorkflowRecordLock(root string) (func() error, error) {
	deadline := time.Now().Add(workflowLockTimeout)
	for {
		lockRoot := workflowRecordLockRoot(root)
		release, err := tryAcquireWorkflowLock(root, lockRoot, workflowRecordLockName)
		if err == nil {
			return release, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		lockPath := filepath.Join(lockRoot, workflowRecordLockName)
		removeStaleWorkflowLock(lockPath)
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for workflow lock %s", relPath(root, lockPath))
		}
		time.Sleep(workflowLockRetryDelay)
	}
}

// acquireWorkflowRecordMigrationLocks holds the record-mutation lock for both
// the current storage root and the `.ahm` target root during records migration.
// This prevents the lock namespace from splitting while the repository's active
// storage mode changes.
func acquireWorkflowRecordMigrationLocks(root string) (func() error, error) {
	currentRoot := workflowRecordLockRoot(root)
	targetRoot := filepath.Join(root, toolRecordsDirName, ".lock")

	releaseCurrent, err := acquireNamedWorkflowLock(root, currentRoot, workflowRecordLockName)
	if err != nil {
		return nil, err
	}
	if currentRoot == targetRoot {
		return releaseCurrent, nil
	}

	releaseTarget, err := acquireNamedWorkflowLock(root, targetRoot, workflowRecordLockName)
	if err != nil {
		_ = releaseCurrent()
		return nil, err
	}

	return func() error {
		var firstErr error
		if err := releaseTarget(); err != nil && firstErr == nil {
			firstErr = err
		}
		if err := releaseCurrent(); err != nil && firstErr == nil {
			firstErr = err
		}
		return firstErr
	}, nil
}

func acquireWorkflowLock(root string, name string) (func() error, error) {
	lockRoot := filepath.Join(root, workflowPathsFor(root).recordsDir, ".lock")
	return acquireNamedWorkflowLock(root, lockRoot, name)
}

// acquireNamedWorkflowLock waits for the named lock under a fixed lock root,
// cleaning up stale locks and respecting the configured timeout.
func acquireNamedWorkflowLock(root string, lockRoot string, name string) (func() error, error) {
	deadline := time.Now().Add(workflowLockTimeout)
	for {
		release, err := tryAcquireWorkflowLock(root, lockRoot, name)
		if err == nil {
			return release, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		lockPath := filepath.Join(lockRoot, name)
		removeStaleWorkflowLock(lockPath)
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for workflow lock %s", relPath(root, lockPath))
		}
		time.Sleep(workflowLockRetryDelay)
	}
}

// tryAcquireWorkflowLock makes a single attempt to create the named lock
// directory. It returns os.ErrExist when the lock is already held.
func tryAcquireWorkflowLock(root string, lockRoot string, name string) (func() error, error) {
	if err := os.MkdirAll(lockRoot, 0o755); err != nil { // #nosec G301 // workflow lock directories use standard permissions
		return nil, fmt.Errorf("acquire workflow lock: create lock dir: %w", err)
	}

	lockPath := filepath.Join(lockRoot, name)
	err := os.Mkdir(lockPath, 0o755) // #nosec G301 // workflow lock directories use standard permissions
	if err == nil {
		return func() error {
			if err := os.Remove(lockPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("release workflow lock %s: %w", relPath(root, lockPath), err)
			}
			return nil
		}, nil
	}
	if !errors.Is(err, os.ErrExist) {
		return nil, fmt.Errorf("acquire workflow lock %s: %w", relPath(root, lockPath), err)
	}
	return nil, err
}

func removeStaleWorkflowLock(lockPath string) {
	info, err := os.Stat(lockPath)
	if err != nil || !info.IsDir() {
		return
	}
	if time.Since(info.ModTime()) <= workflowLockStaleAfter {
		return
	}
	_ = os.RemoveAll(lockPath)
}
