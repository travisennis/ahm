package ahm

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// saveLockTimeout saves the current workflowLockTimeout and restores it via
// t.Cleanup. It is safe to call with any positive duration.
func saveLockTimeout(t *testing.T) {
	t.Helper()
	orig := workflowLockTimeout
	t.Cleanup(func() { workflowLockTimeout = orig })
}

// saveLockStaleAfter saves the current workflowLockStaleAfter and restores it
// via t.Cleanup. It is safe to call with any positive duration.
func saveLockStaleAfter(t *testing.T) {
	t.Helper()
	orig := workflowLockStaleAfter
	t.Cleanup(func() { workflowLockStaleAfter = orig })
}

func TestAcquireWorkflowLock_AcquireRelease(t *testing.T) {
	dir := t.TempDir()

	// First acquire must succeed.
	release, err := acquireWorkflowLock(dir, "test-a")
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}

	// Release must succeed.
	if err := release(); err != nil {
		t.Fatalf("release failed: %v", err)
	}

	// Acquire again on the same name must succeed.
	release2, err := acquireWorkflowLock(dir, "test-a")
	if err != nil {
		t.Fatalf("second acquire failed: %v", err)
	}
	if err := release2(); err != nil {
		t.Fatalf("second release failed: %v", err)
	}
}

func TestAcquireWorkflowLock_BlocksContention(t *testing.T) {
	dir := t.TempDir()
	saveLockTimeout(t)
	workflowLockTimeout = 50 * time.Millisecond

	// Hold the lock.
	release, err := acquireWorkflowLock(dir, "test-b")
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	defer func() {
		if err := release(); err != nil {
			t.Errorf("release failed: %v", err)
		}
	}()

	// A second acquire on the same name must time out.
	_, err = acquireWorkflowLock(dir, "test-b")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestAcquireWorkflowLock_ConcurrentSerialization(t *testing.T) {
	dir := t.TempDir()
	saveLockTimeout(t)
	workflowLockTimeout = 10 * time.Second // generous; each acquire should be near-instant

	const goroutines = 10
	var counter int64
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := acquireWorkflowLock(dir, "test-c")
			if err != nil {
				t.Errorf("concurrent acquire failed: %v", err)
				return
			}
			atomic.AddInt64(&counter, 1)
			if err := release(); err != nil {
				t.Errorf("concurrent release failed: %v", err)
			}
		}()
	}
	wg.Wait()

	if counter != goroutines {
		t.Errorf("counter = %d, want %d", counter, goroutines)
	}
}

func TestAcquireWorkflowLock_Timeout(t *testing.T) {
	dir := t.TempDir()
	saveLockTimeout(t)
	workflowLockTimeout = 10 * time.Millisecond

	// Hold the lock so the second attempt must wait.
	release, err := acquireWorkflowLock(dir, "test-d")
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	defer func() {
		if err := release(); err != nil {
			t.Errorf("release failed: %v", err)
		}
	}()

	start := time.Now()
	_, err = acquireWorkflowLock(dir, "test-d")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	// The timeout must fire within a reasonable window of the configured value.
	// Allow up to 3x for scheduler variability.
	if elapsed > 3*workflowLockTimeout {
		t.Errorf("timeout took %v, expected ~%v", elapsed, workflowLockTimeout)
	}
}

func TestAcquireWorkflowLock_StaleLockCleanup(t *testing.T) {
	dir := t.TempDir()
	saveLockStaleAfter(t)

	// Make the stale-after threshold very short so we can simulate age without
	// waiting real time.
	workflowLockStaleAfter = 10 * time.Millisecond

	// Manually create a lock directory so it looks like a stale lock from a
	// previous crashed process.
	lockPath := filepath.Join(dir, ".agents", ".lock", "test-e")
	if err := os.MkdirAll(lockPath, 0o755); err != nil {
		t.Fatal(err)
	}

	// Set the directory mod time far enough in the past that
	// removeStaleWorkflowLock considers it stale.
	past := time.Now().Add(-2 * workflowLockStaleAfter)
	if err := os.Chtimes(lockPath, past, past); err != nil {
		t.Fatal(err)
	}

	// Acquire must succeed: the stale lock is detected and cleaned up.
	release, err := acquireWorkflowLock(dir, "test-e")
	if err != nil {
		t.Fatalf("acquire after stale cleanup failed: %v", err)
	}
	defer func() {
		if err := release(); err != nil {
			t.Errorf("release failed: %v", err)
		}
	}()

	// The lock directory must now exist (re-created by the successful acquire).
	if _, err := os.Stat(lockPath); err != nil {
		t.Errorf("lock directory should exist after fresh acquire: %v", err)
	}
}

func TestAcquireWorkflowLock_NonStaleLockIsNotRemoved(t *testing.T) {
	dir := t.TempDir()
	saveLockTimeout(t)
	saveLockStaleAfter(t)

	workflowLockTimeout = 50 * time.Millisecond
	workflowLockStaleAfter = 10 * time.Minute // long enough to not be stale

	// Manually create a recent lock directory.
	lockPath := filepath.Join(dir, ".agents", ".lock", "test-f")
	if err := os.MkdirAll(lockPath, 0o755); err != nil {
		t.Fatal(err)
	}

	// The lock directory modtime is now (fresh).

	// Acquire must fail with timeout because the non-stale lock is held.
	_, err := acquireWorkflowLock(dir, "test-f")
	if err == nil {
		t.Fatal("expected timeout error for non-stale held lock")
	}

	// The original lock directory must still exist (was not removed).
	if _, err := os.Stat(lockPath); err != nil {
		t.Errorf("non-stale lock directory was removed: %v", err)
	}
}
