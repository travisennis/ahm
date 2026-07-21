package ahm

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const workflowRecordLockName = "workflow-records"

var errWorkflowLockOwnershipLost = errors.New("workflow lock ownership lost")

var (
	workflowLockRetryDelay = 10 * time.Millisecond
	workflowLockTimeout    = 10 * time.Second
	workflowLockStaleAfter = 30 * time.Minute
)

// withWorkflowRecordLock runs f while holding the repository-scoped workflow
// record-mutation lock. When mutating is false, f is called without a lock
// (used for dry-run and read-only preview paths).
func (a *app) withWorkflowRecordLock(mutating bool, f func() error) (resultErr error) {
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
	defer func() { resultErr = errors.Join(resultErr, release()) }()
	return f()
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
		_ = removeStaleWorkflowLock(lockPath)
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
		_ = removeStaleWorkflowLock(lockPath)
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
		owner, statErr := os.Stat(lockPath)
		if statErr != nil {
			return nil, fmt.Errorf("acquire workflow lock %s: inspect ownership: %w", relPath(root, lockPath), statErr)
		}
		return func() error {
			if err := removeWorkflowLockIfOwned(lockPath, owner); err != nil {
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

func removeStaleWorkflowLock(lockPath string) error {
	return removeStaleWorkflowLockAfterObservation(lockPath, nil)
}

// removeStaleWorkflowLockAfterObservation exists so tests can deterministically
// replace a lock after its age is observed but before reclamation claims it.
func removeStaleWorkflowLockAfterObservation(lockPath string, afterObservation func()) error {
	info, err := os.Stat(lockPath)
	if err != nil || !info.IsDir() {
		return nil
	}
	if time.Since(info.ModTime()) <= workflowLockStaleAfter {
		return nil
	}
	if afterObservation != nil {
		afterObservation()
	}
	return removeWorkflowLockIfOwned(lockPath, info)
}

// removeWorkflowLockIfOwned atomically moves a lock into a unique quarantine
// before deleting it. Identity checks on both sides of the rename prevent a
// stale observer or former owner from deleting a replacement lock.
func removeWorkflowLockIfOwned(lockPath string, owner os.FileInfo) error {
	current, err := os.Stat(lockPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errWorkflowLockOwnershipLost
		}
		return err
	}
	if !os.SameFile(owner, current) {
		return errWorkflowLockOwnershipLost
	}

	quarantine, err := os.MkdirTemp(filepath.Dir(lockPath), "."+filepath.Base(lockPath)+".reclaim-")
	if err != nil {
		return fmt.Errorf("create lock reclamation directory: %w", err)
	}
	claimedPath := filepath.Join(quarantine, filepath.Base(lockPath))
	if err := os.Rename(lockPath, claimedPath); err != nil {
		_ = os.Remove(quarantine)
		if errors.Is(err, os.ErrNotExist) {
			return errWorkflowLockOwnershipLost
		}
		return fmt.Errorf("claim lock for removal: %w", err)
	}

	claimed, err := os.Stat(claimedPath)
	if err != nil {
		return fmt.Errorf("%w: inspect claimed lock %s: %v", errWorkflowLockOwnershipLost, claimedPath, err)
	}
	if !os.SameFile(owner, claimed) {
		return fmt.Errorf("%w: replacement moved to %s", errWorkflowLockOwnershipLost, claimedPath)
	}
	if err := os.RemoveAll(quarantine); err != nil {
		return fmt.Errorf("remove claimed lock: %w", err)
	}
	return nil
}
