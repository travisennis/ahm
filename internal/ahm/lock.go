package ahm

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var (
	workflowLockRetryDelay = 10 * time.Millisecond
	workflowLockTimeout    = 10 * time.Second
	workflowLockStaleAfter = 30 * time.Minute
)

func acquireWorkflowLock(root string, name string) (func() error, error) {
	lockRoot := filepath.Join(root, ".agents", ".lock")
	if err := os.MkdirAll(lockRoot, 0o755); err != nil { // #nosec G301 // workflow lock directories use standard permissions
		return nil, fmt.Errorf("acquire workflow lock: create lock dir: %w", err)
	}

	lockPath := filepath.Join(lockRoot, name)
	deadline := time.Now().Add(workflowLockTimeout)
	for {
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
		removeStaleWorkflowLock(lockPath)
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for workflow lock %s", relPath(root, lockPath))
		}
		time.Sleep(workflowLockRetryDelay)
	}
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
