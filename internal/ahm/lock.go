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
	firstResolution := true
	release, err := acquireWorkflowRecordLockWithResolver(a.opts.root, func() workflowPaths {
		if !firstResolution {
			a.invalidateWorkflowPaths()
		}
		firstResolution = false
		return a.workflowPaths()
	})
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

// acquireWorkflowRecordLock returns the release function for the single
// repository-scoped workflow record-mutation lock. The lock lives in the
// active record root (`.agents/.lock` or `.ahm/.lock`) and is re-resolved on
// each retry so waiters migrate with a layout transition.
func acquireWorkflowRecordLock(root string) (func() error, error) {
	return acquireWorkflowRecordLockWithResolver(root, func() workflowPaths {
		return workflowPathsFor(root)
	})
}

func acquireWorkflowRecordLockWithResolver(root string, resolve func() workflowPaths) (func() error, error) {
	deadline := time.Now().Add(workflowLockTimeout)
	for {
		lockRoot := filepath.Join(root, resolve().recordsDir, ".lock")
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
// the current record root and the `.ahm` target root during records migration.
// This prevents the lock namespace from splitting while the repository's record
// layout changes.
func acquireWorkflowRecordMigrationLocksForPaths(root string, paths workflowPaths) (func() error, error) {
	currentRoot := filepath.Join(root, paths.recordsDir, ".lock")
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
