package ahm

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const workflowRecordLockName = "workflow-records"

// workflowLockOwnerFile is the name of the file inside a freshly acquired lock
// directory that holds the owner token identifying that lock instance.
const workflowLockOwnerFile = "owner"

var errWorkflowLockOwnershipLost = errors.New("workflow lock ownership lost")

var (
	workflowLockRetryDelay        = 10 * time.Millisecond
	workflowLockTimeout           = 10 * time.Second
	workflowLockStaleAfter        = 30 * time.Minute
	workflowLockHeartbeatInterval = workflowLockStaleAfter / 6 // 5 minutes in practice
	workflowLockStaleRecheckDelay = 1 * time.Second
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
		token, tokenErr := writeWorkflowLockToken(lockPath)
		if tokenErr != nil {
			_ = os.RemoveAll(lockPath)
			return nil, fmt.Errorf("acquire workflow lock %s: write owner token: %w", relPath(root, lockPath), tokenErr)
		}

		heartbeatDone := make(chan struct{})
		go heartbeatLock(lockPath, workflowLockHeartbeatInterval, heartbeatDone)

		return func() error {
			close(heartbeatDone)
			if err := removeWorkflowLockIfOwned(lockPath, token); err != nil {
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

// heartbeatLock periodically updates the lock directory's modification time so
// that the stale-lock reclamation does not reclaim a live lock. It exits when
// done is closed.
func heartbeatLock(lockPath string, interval time.Duration, done <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := time.Now()
			_ = os.Chtimes(lockPath, now, now)
		case <-done:
			return
		}
	}
}

func removeStaleWorkflowLock(lockPath string) error {
	return removeStaleWorkflowLockAfterObservation(lockPath, nil)
}

// removeStaleWorkflowLockAfterObservation exists so tests can deterministically
// replace a lock after its age is observed but before reclamation claims it.
//
// To avoid reclaiming a live lock whose owner is heartbeating, it uses a
// two-check pattern: it observes the modification time, waits a short delay,
// observes again, and only reclaims when both observations are past the stale
// threshold and the modification time is unchanged. The owner token is read
// only once the lock is genuinely stale, before the claim (which re-reads it)
// runs, so a replacement lock acquired between the read and the claim is
// detected. Reading the token for fresh locks is deliberately avoided: on
// Windows, os.ReadFile opens the token file without FILE_SHARE_DELETE, and a
// concurrent rename of the lock directory fails with ACCESS_DENIED while such
// a handle is open.
func removeStaleWorkflowLockAfterObservation(lockPath string, afterObservation func()) error {
	info, err := os.Stat(lockPath)
	if err != nil || !info.IsDir() {
		return nil
	}
	if time.Since(info.ModTime()) <= workflowLockStaleAfter {
		return nil
	}

	// Re-observe after a short delay. A live heartbeat would update the
	// modification time between observations, preventing reclamation.
	time.Sleep(workflowLockStaleRecheckDelay)

	info2, err := os.Stat(lockPath)
	if err != nil || !info2.IsDir() {
		return nil
	}
	if time.Since(info2.ModTime()) <= workflowLockStaleAfter {
		return nil
	}
	if !info2.ModTime().Equal(info.ModTime()) {
		// Modification time changed between observations; owner is heartbeating.
		return nil
	}

	token, hasToken := workflowLockToken(lockPath)

	if afterObservation != nil {
		afterObservation()
	}
	if hasToken {
		return removeWorkflowLockIfOwned(lockPath, token)
	}
	// Legacy lock without an owner token (left by a pre-token ahm version that
	// crashed): fall back to the filesystem-identity claim, passing the second
	// observation so a replacement in the observe-to-claim window is detected.
	// The two-check modtime observation remains the primary guard against
	// reclaiming a live lock; newly acquired locks always carry a token and
	// use token identity.
	return reclaimLegacyWorkflowLock(lockPath, info2)
}

// writeWorkflowLockToken writes a fresh unique owner token into the lock
// directory and returns it. The token lets release and stale reclamation verify
// they are acting on the exact lock instance they acquired or observed.
func writeWorkflowLockToken(lockPath string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b[:])
	if err := os.WriteFile(filepath.Join(lockPath, workflowLockOwnerFile), []byte(token), 0o600); err != nil {
		return "", err
	}
	return token, nil
}

// workflowLockToken reads the owner token from the lock directory. The second
// return value reports whether the lock carries a token; locks created by
// pre-token versions of ahm have none.
func workflowLockToken(lockPath string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(lockPath, workflowLockOwnerFile)) // #nosec G304 // lockPath is built from the repository root by acquire/reclaim callers
	if err != nil {
		return "", false
	}
	return string(data), true
}

// workflowLockOwned reports whether the directory at lockPath belongs to the
// holder of token, i.e. it still contains exactly that owner token. A
// replacement lock written by a different acquire carries a different token,
// so identity checks work on every platform: filesystem identity via
// os.SameFile is defeated by inode reuse on Unix and by Windows' path-based
// file-ID resolution.
func workflowLockOwned(lockPath string, token string) bool {
	got, ok := workflowLockToken(lockPath)
	return ok && got == token
}

// removeWorkflowLockIfOwned atomically moves a lock into a unique quarantine
// before deleting it. Ownership is verified by comparing the owner token on
// both sides of the rename, so a stale observer or former owner never deletes
// a replacement lock.
func removeWorkflowLockIfOwned(lockPath string, token string) error {
	if !workflowLockOwned(lockPath, token) {
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

	if !workflowLockOwned(claimedPath, token) {
		return fmt.Errorf("%w: replacement moved to %s", errWorkflowLockOwnershipLost, claimedPath)
	}
	if err := os.RemoveAll(quarantine); err != nil {
		return fmt.Errorf("remove claimed lock: %w", err)
	}
	return nil
}

// reclaimLegacyWorkflowLock removes a token-less lock directory using
// filesystem-identity checks. It exists to reclaim stale locks left by
// pre-token versions of ahm; newly acquired locks always carry an owner token
// and are reclaimed through removeWorkflowLockIfOwned. observed is the
// FileInfo captured by the second modtime observation, so a replacement lock
// appearing between the observations and the claim is not deleted.
func reclaimLegacyWorkflowLock(lockPath string, observed os.FileInfo) error {
	current, err := os.Stat(lockPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errWorkflowLockOwnershipLost
		}
		return err
	}
	// A replacement is a fresh acquire with a recent modification time; the
	// os.SameFile check below cannot distinguish one on Windows (file IDs are
	// resolved by path at comparison time), so re-verify staleness at claim.
	if time.Since(current.ModTime()) <= workflowLockStaleAfter {
		return nil
	}
	if !os.SameFile(observed, current) {
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
	if !os.SameFile(current, claimed) {
		return fmt.Errorf("%w: replacement moved to %s", errWorkflowLockOwnershipLost, claimedPath)
	}
	if err := os.RemoveAll(quarantine); err != nil {
		return fmt.Errorf("remove claimed lock: %w", err)
	}
	return nil
}
