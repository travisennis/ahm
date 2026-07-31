package ahm

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

// saveLockHeartbeatInterval saves the current workflowLockHeartbeatInterval and
// restores it via t.Cleanup.
func saveLockHeartbeatInterval(t *testing.T) {
	t.Helper()
	orig := workflowLockHeartbeatInterval
	t.Cleanup(func() { workflowLockHeartbeatInterval = orig })
}

// saveLockStaleRecheckDelay saves the current workflowLockStaleRecheckDelay and
// restores it via t.Cleanup.
func saveLockStaleRecheckDelay(t *testing.T) {
	t.Helper()
	orig := workflowLockStaleRecheckDelay
	t.Cleanup(func() { workflowLockStaleRecheckDelay = orig })
}

func TestAcquireWorkflowLock_AcquireRelease(t *testing.T) {
	dir := t.TempDir()
	lockRoot := filepath.Join(dir, workflowPathsFor(dir).recordsDir, ".lock")

	// First acquire must succeed.
	release, err := acquireNamedWorkflowLock(dir, lockRoot, "test-a")
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}

	// Release must succeed.
	if err := release(); err != nil {
		t.Fatalf("release failed: %v", err)
	}

	// Acquire again on the same name must succeed.
	release2, err := acquireNamedWorkflowLock(dir, lockRoot, "test-a")
	if err != nil {
		t.Fatalf("second acquire failed: %v", err)
	}
	if err := release2(); err != nil {
		t.Fatalf("second release failed: %v", err)
	}
}

func TestAcquireWorkflowLock_BlocksContention(t *testing.T) {
	dir := t.TempDir()
	lockRoot := filepath.Join(dir, workflowPathsFor(dir).recordsDir, ".lock")
	saveLockTimeout(t)
	workflowLockTimeout = 50 * time.Millisecond

	// Hold the lock.
	release, err := acquireNamedWorkflowLock(dir, lockRoot, "test-b")
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	defer func() {
		if err := release(); err != nil {
			t.Errorf("release failed: %v", err)
		}
	}()

	// A second acquire on the same name must time out.
	_, err = acquireNamedWorkflowLock(dir, lockRoot, "test-b")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestAcquireWorkflowLock_ConcurrentSerialization(t *testing.T) {
	dir := t.TempDir()
	lockRoot := filepath.Join(dir, workflowPathsFor(dir).recordsDir, ".lock")
	saveLockTimeout(t)
	workflowLockTimeout = 10 * time.Second // generous; each acquire should be near-instant

	const goroutines = 10
	var counter int64
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := acquireNamedWorkflowLock(dir, lockRoot, "test-c")
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
	lockRoot := filepath.Join(dir, workflowPathsFor(dir).recordsDir, ".lock")
	saveLockTimeout(t)
	workflowLockTimeout = 10 * time.Millisecond

	// Hold the lock so the second attempt must wait.
	release, err := acquireNamedWorkflowLock(dir, lockRoot, "test-d")
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	defer func() {
		if err := release(); err != nil {
			t.Errorf("release failed: %v", err)
		}
	}()

	start := time.Now()
	_, err = acquireNamedWorkflowLock(dir, lockRoot, "test-d")
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
	lockRoot := filepath.Join(dir, workflowPathsFor(dir).recordsDir, ".lock")
	saveLockStaleAfter(t)
	saveLockStaleRecheckDelay(t)

	// Make the stale-after threshold very short so we can simulate age without
	// waiting real time.
	workflowLockStaleAfter = 10 * time.Millisecond
	workflowLockStaleRecheckDelay = 1 * time.Millisecond // short so the test doesn't block

	// Manually create a lock directory so it looks like a stale lock from a
	// previous crashed process. It carries no owner token (pre-token layout),
	// exercising the legacy reclamation fallback.
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
	release, err := acquireNamedWorkflowLock(dir, lockRoot, "test-e")
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

// writeTestLockToken writes an owner token into a manually created lock
// directory, simulating the token a crashed acquire would have left behind.
func writeTestLockToken(t *testing.T, lockPath string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(lockPath, workflowLockOwnerFile), []byte("test-owner"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveStaleWorkflowLock_DoesNotRemoveReplacement(t *testing.T) {
	dir := t.TempDir()
	saveLockStaleAfter(t)
	saveLockStaleRecheckDelay(t)
	workflowLockStaleAfter = 10 * time.Millisecond
	workflowLockStaleRecheckDelay = 1 * time.Millisecond // short so the test doesn't block

	lockRoot := filepath.Join(dir, ".agents", ".lock")
	lockPath := filepath.Join(lockRoot, "test-replacement")
	if err := os.MkdirAll(lockPath, 0o755); err != nil {
		t.Fatal(err)
	}
	// Simulate a crashed acquire: the stale lock carries an owner token.
	writeTestLockToken(t, lockPath)
	past := time.Now().Add(-2 * workflowLockStaleAfter)
	if err := os.Chtimes(lockPath, past, past); err != nil {
		t.Fatal(err)
	}

	var replacementRelease func() error
	err := removeStaleWorkflowLockAfterObservation(lockPath, func() {
		if err := os.RemoveAll(lockPath); err != nil {
			t.Fatalf("remove observed stale lock: %v", err)
		}
		var err error
		replacementRelease, err = tryAcquireWorkflowLock(dir, lockRoot, "test-replacement")
		if err != nil {
			t.Fatalf("acquire replacement lock: %v", err)
		}
	})
	if !errors.Is(err, errWorkflowLockOwnershipLost) {
		t.Fatalf("remove stale lock error = %v, want ownership lost", err)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("replacement lock was removed: %v", err)
	}
	if err := replacementRelease(); err != nil {
		t.Fatalf("release replacement lock: %v", err)
	}
}

// TestRemoveStaleWorkflowLock_DoesNotRemoveLegacyReplacement exercises the
// token-less legacy fallback: a fresh replacement must not be claimed even
// though the filesystem-identity check cannot distinguish it on Windows (file
// IDs are resolved by path at comparison time). The claim-time staleness
// re-check abandons the reclamation instead.
func TestRemoveStaleWorkflowLock_DoesNotRemoveLegacyReplacement(t *testing.T) {
	dir := t.TempDir()
	saveLockStaleAfter(t)
	saveLockStaleRecheckDelay(t)
	workflowLockStaleAfter = 10 * time.Millisecond
	workflowLockStaleRecheckDelay = 1 * time.Millisecond // short so the test doesn't block

	lockRoot := filepath.Join(dir, ".agents", ".lock")
	lockPath := filepath.Join(lockRoot, "test-legacy-replacement")
	if err := os.MkdirAll(lockPath, 0o755); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * workflowLockStaleAfter)
	if err := os.Chtimes(lockPath, past, past); err != nil {
		t.Fatal(err)
	}

	// Replace the observed stale lock with a fresh token-less directory before
	// the claim runs.
	err := removeStaleWorkflowLockAfterObservation(lockPath, func() {
		if err := os.RemoveAll(lockPath); err != nil {
			t.Fatalf("remove observed stale lock: %v", err)
		}
		if err := os.Mkdir(lockPath, 0o755); err != nil {
			t.Fatalf("create replacement lock: %v", err)
		}
	})
	if err != nil {
		t.Fatalf("reclamation should abandon a fresh replacement, got %v", err)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("replacement lock was removed: %v", err)
	}
}

func TestAcquireWorkflowLock_ReleaseRejectsReplacement(t *testing.T) {
	dir := t.TempDir()
	lockRoot := filepath.Join(dir, workflowPathsFor(dir).recordsDir, ".lock")
	lockPath := filepath.Join(lockRoot, "test-release-replacement")

	release, err := acquireNamedWorkflowLock(dir, lockRoot, "test-release-replacement")
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	if err := os.RemoveAll(lockPath); err != nil {
		t.Fatalf("remove acquired lock: %v", err)
	}
	// A replacement lock has no owner token, so release must detect that it
	// no longer owns the lock instead of deleting the replacement. Token
	// identity is deterministic across platforms, unlike filesystem identity:
	// inode reuse on Unix and Windows' path-based file-ID resolution both
	// defeat os.SameFile for a remove-and-recreate at the same path.
	if err := os.Mkdir(lockPath, 0o755); err != nil {
		t.Fatalf("create replacement lock: %v", err)
	}

	if err := release(); !errors.Is(err, errWorkflowLockOwnershipLost) {
		t.Fatalf("release error = %v, want ownership lost", err)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("replacement lock was removed: %v", err)
	}
}

func TestAcquireWorkflowLock_ReleaseRejectsMissingLock(t *testing.T) {
	dir := t.TempDir()
	lockRoot := filepath.Join(dir, workflowPathsFor(dir).recordsDir, ".lock")
	lockPath := filepath.Join(lockRoot, "test-release-missing")

	release, err := acquireNamedWorkflowLock(dir, lockRoot, "test-release-missing")
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	if err := os.RemoveAll(lockPath); err != nil {
		t.Fatalf("remove acquired lock: %v", err)
	}

	if err := release(); !errors.Is(err, errWorkflowLockOwnershipLost) {
		t.Fatalf("release error = %v, want ownership lost", err)
	}
}

func TestWithWorkflowRecordLock_ReturnsReleaseOwnershipLoss(t *testing.T) {
	dir := t.TempDir()
	a := app{opts: options{root: dir}}
	lockPath := filepath.Join(dir, ".agents", ".lock", workflowRecordLockName)

	err := a.withWorkflowRecordLock(true, func() error {
		return os.RemoveAll(lockPath)
	})
	if !errors.Is(err, errWorkflowLockOwnershipLost) {
		t.Fatalf("withWorkflowRecordLock error = %v, want ownership lost", err)
	}
}

func TestAcquireWorkflowLock_NonStaleLockIsNotRemoved(t *testing.T) {
	dir := t.TempDir()
	lockRoot := filepath.Join(dir, workflowPathsFor(dir).recordsDir, ".lock")
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
	_, err := acquireNamedWorkflowLock(dir, lockRoot, "test-f")
	if err == nil {
		t.Fatal("expected timeout error for non-stale held lock")
	}

	// The original lock directory must still exist (was not removed).
	if _, err := os.Stat(lockPath); err != nil {
		t.Errorf("non-stale lock directory was removed: %v", err)
	}
}

func TestHeartbeatPreventsStaleReclamation(t *testing.T) {
	dir := t.TempDir()
	lockRoot := filepath.Join(dir, workflowPathsFor(dir).recordsDir, ".lock")
	lockPath := filepath.Join(lockRoot, "test-heartbeat")

	// Create a lock directory with a past modification time.
	if err := os.MkdirAll(lockPath, 0o755); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(lockPath, past, past); err != nil {
		t.Fatal(err)
	}

	saveLockStaleAfter(t)
	saveLockStaleRecheckDelay(t)
	saveLockHeartbeatInterval(t)

	// Short intervals so the test is fast.
	workflowLockStaleAfter = 50 * time.Millisecond
	workflowLockStaleRecheckDelay = 30 * time.Millisecond
	workflowLockHeartbeatInterval = 10 * time.Millisecond

	// Start a heartbeat to simulate a live lock owner.
	heartbeatDone := make(chan struct{})
	go heartbeatLock(lockPath, workflowLockHeartbeatInterval, heartbeatDone, os.Chtimes)
	defer close(heartbeatDone)

	// Wait long enough for the stale threshold to pass and for the heartbeat
	// to fire at least once after the first observation.
	time.Sleep(100 * time.Millisecond)

	// The stale reclamation must not reclaim the lock: the heartbeat refreshes
	// ModTime between observations.
	err := removeStaleWorkflowLockAfterObservation(lockPath, nil)
	if err != nil {
		t.Fatalf("removeStaleWorkflowLockAfterObservation should not reclaim a heartbeating lock: %v", err)
	}

	// The lock directory must still exist.
	if _, err := os.Stat(lockPath); err != nil {
		t.Errorf("heartbeating lock directory was removed: %v", err)
	}
}

func TestHeartbeatStopsAndLockBecomesReclaimable(t *testing.T) {
	dir := t.TempDir()
	lockRoot := filepath.Join(dir, workflowPathsFor(dir).recordsDir, ".lock")
	lockPath := filepath.Join(lockRoot, "test-heartbeat-stop")

	saveLockStaleAfter(t)
	saveLockStaleRecheckDelay(t)
	saveLockHeartbeatInterval(t)

	workflowLockStaleAfter = 50 * time.Millisecond
	workflowLockStaleRecheckDelay = 10 * time.Millisecond
	workflowLockHeartbeatInterval = 10 * time.Millisecond

	// Create a lock manually and give it an old modtime.
	if err := os.MkdirAll(lockPath, 0o755); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(lockPath, past, past); err != nil {
		t.Fatal(err)
	}

	// Run the heartbeat briefly so that ModTime gets refreshed, then stop it.
	heartbeatDone := make(chan struct{})
	go heartbeatLock(lockPath, workflowLockHeartbeatInterval, heartbeatDone, os.Chtimes)
	time.Sleep(30 * time.Millisecond) // let heartbeat fire at least once
	close(heartbeatDone)
	time.Sleep(10 * time.Millisecond) // let goroutine exit

	// Reset ModTime to past so the lock appears stale.
	past = time.Now().Add(-2 * workflowLockStaleAfter)
	if err := os.Chtimes(lockPath, past, past); err != nil {
		t.Fatal(err)
	}

	// Now the lock should be reclaimable (no heartbeat).
	err := removeStaleWorkflowLockAfterObservation(lockPath, nil)
	if err != nil {
		t.Fatalf("should reclaim lock after heartbeat stops: %v", err)
	}

	// The lock directory should be gone.
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Errorf("expected lock directory to be removed, stat: %v", err)
	}
}

// TestAcquireWorkflowLock_TokenWriteFailureRemovesCreatedLock covers the
// partial-acquire path: the lock directory is created, then writing the owner
// token fails. The created lock must be removed so the repository is not left
// locked with no live owner. This is the equivalent seam for the stat-failure
// path that predated owner tokens: the token write is the only step between
// lock creation and returning a release closure.
func TestAcquireWorkflowLock_TokenWriteFailureRemovesCreatedLock(t *testing.T) {
	dir := t.TempDir()
	lockRoot := filepath.Join(dir, workflowPathsFor(dir).recordsDir, ".lock")
	lockPath := filepath.Join(lockRoot, "test-token-failure")

	orig := workflowLockTokenWriter
	tokenErr := errors.New("injected token write failure")
	workflowLockTokenWriter = func(string) (string, error) { return "", tokenErr }
	t.Cleanup(func() { workflowLockTokenWriter = orig })

	_, err := tryAcquireWorkflowLock(dir, lockRoot, "test-token-failure")
	if !errors.Is(err, tokenErr) {
		t.Fatalf("acquire error = %v, want injected token write failure", err)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Errorf("created lock directory was not removed after token write failure: %v", err)
	}
}

// TestRemoveWorkflowLockIfOwned_PostRenameMismatchCleansQuarantine drives the
// ownership-lost return after the rename succeeds in removeWorkflowLockIfOwned:
// the owner token inside the quarantine is replaced between the two ownership
// checks, so the post-rename check fails. The quarantine directory must not
// survive the error return.
func TestRemoveWorkflowLockIfOwned_PostRenameMismatchCleansQuarantine(t *testing.T) {
	dir := t.TempDir()
	lockRoot := filepath.Join(dir, ".agents", ".lock")
	lockPath := filepath.Join(lockRoot, "test-post-rename-token")
	if err := os.MkdirAll(lockPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lockPath, workflowLockOwnerFile), []byte("original-token"), 0o600); err != nil {
		t.Fatal(err)
	}

	orig := workflowLockClaimHook
	workflowLockClaimHook = func(claimedPath string) {
		// Simulate a replacement lock taking over the quarantined path: its
		// owner token differs, so the post-rename identity check fails.
		if err := os.WriteFile(filepath.Join(claimedPath, workflowLockOwnerFile), []byte("replacement-token"), 0o600); err != nil {
			t.Errorf("rewrite owner token in quarantine: %v", err)
		}
	}
	t.Cleanup(func() { workflowLockClaimHook = orig })

	err := removeWorkflowLockIfOwned(lockPath, "original-token")
	if !errors.Is(err, errWorkflowLockOwnershipLost) {
		t.Fatalf("remove error = %v, want ownership lost", err)
	}
	assertNoReclaimDirs(t, lockRoot)
}

// TestReclaimLegacyLock_StatFailureAfterRenameCleansQuarantine drives the
// ownership-lost return in reclaimLegacyWorkflowLock where the quarantined
// lock disappears before it can be re-inspected. The quarantine directory must
// not survive the error return.
func TestReclaimLegacyLock_StatFailureAfterRenameCleansQuarantine(t *testing.T) {
	dir := t.TempDir()
	lockRoot := filepath.Join(dir, ".agents", ".lock")
	lockPath := filepath.Join(lockRoot, "test-legacy-stat-failure")
	if err := os.MkdirAll(lockPath, 0o755); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(lockPath, past, past); err != nil {
		t.Fatal(err)
	}

	saveLockStaleAfter(t)
	workflowLockStaleAfter = 10 * time.Millisecond
	observed, err := os.Stat(lockPath)
	if err != nil {
		t.Fatal(err)
	}

	orig := workflowLockClaimHook
	workflowLockClaimHook = func(claimedPath string) {
		// Simulate the quarantined lock disappearing before inspection.
		if err := os.RemoveAll(claimedPath); err != nil {
			t.Errorf("remove quarantined lock: %v", err)
		}
	}
	t.Cleanup(func() { workflowLockClaimHook = orig })

	err = reclaimLegacyWorkflowLock(lockPath, observed)
	if !errors.Is(err, errWorkflowLockOwnershipLost) {
		t.Fatalf("reclaim error = %v, want ownership lost", err)
	}
	assertNoReclaimDirs(t, lockRoot)
}

// TestReclaimLegacyLock_IdentityMismatchAfterRenameCleansQuarantine drives the
// ownership-lost return in reclaimLegacyWorkflowLock where the quarantined
// lock is replaced by a different directory before the post-rename identity
// check. On Unix the fresh directory has a distinct identity and reclamation
// aborts with ownership lost; on Windows the identity check is path-based and
// may or may not distinguish the replacement. Either way the quarantine
// directory must not survive.
func TestReclaimLegacyLock_IdentityMismatchAfterRenameCleansQuarantine(t *testing.T) {
	dir := t.TempDir()
	lockRoot := filepath.Join(dir, ".agents", ".lock")
	lockPath := filepath.Join(lockRoot, "test-legacy-identity")
	if err := os.MkdirAll(lockPath, 0o755); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(lockPath, past, past); err != nil {
		t.Fatal(err)
	}

	saveLockStaleAfter(t)
	workflowLockStaleAfter = 10 * time.Millisecond
	observed, err := os.Stat(lockPath)
	if err != nil {
		t.Fatal(err)
	}

	orig := workflowLockClaimHook
	workflowLockClaimHook = func(claimedPath string) {
		// Replace the quarantined lock with a fresh directory so the
		// post-rename identity check sees a different file.
		if err := os.RemoveAll(claimedPath); err != nil {
			t.Errorf("remove quarantined lock: %v", err)
		}
		if err := os.Mkdir(claimedPath, 0o755); err != nil {
			t.Errorf("create replacement quarantined lock: %v", err)
		}
	}
	t.Cleanup(func() { workflowLockClaimHook = orig })

	err = reclaimLegacyWorkflowLock(lockPath, observed)
	if err != nil && !errors.Is(err, errWorkflowLockOwnershipLost) {
		t.Fatalf("reclaim error = %v, want ownership lost or success", err)
	}
	assertNoReclaimDirs(t, lockRoot)
}

// TestAcquireWorkflowLock_ReleaseJoinsHeartbeat proves the release closure
// joins the heartbeat goroutine: release must not return while an os.Chtimes
// is in flight, so no heartbeat write can touch the lock path (or a successor's
// lock at that path) after the owner has released it.
func TestAcquireWorkflowLock_ReleaseJoinsHeartbeat(t *testing.T) {
	dir := t.TempDir()
	lockRoot := filepath.Join(dir, workflowPathsFor(dir).recordsDir, ".lock")
	saveLockHeartbeatInterval(t)
	workflowLockHeartbeatInterval = 1 * time.Millisecond

	origChtimes := workflowLockChtimes
	releaseChtimes := make(chan struct{})
	var closeOnce sync.Once
	closeChtimes := func() { closeOnce.Do(func() { close(releaseChtimes) }) }
	defer closeChtimes()

	var inFlight, completed atomic.Int64
	workflowLockChtimes = func(path string, atime, mtime time.Time) error {
		inFlight.Add(1)
		<-releaseChtimes // hold the call open so release can race it
		completed.Add(1)
		return nil
	}
	t.Cleanup(func() { workflowLockChtimes = origChtimes })

	release, err := acquireNamedWorkflowLock(dir, lockRoot, "test-join-heartbeat")
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}

	// Wait until the heartbeat is inside os.Chtimes.
	deadline := time.Now().Add(2 * time.Second)
	for inFlight.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("heartbeat never fired before release")
		}
		time.Sleep(time.Millisecond)
	}

	released := make(chan error, 1)
	go func() { released <- release() }()

	// Release must block until the in-flight heartbeat completes. A release
	// that returns here would let a later Chtimes touch a successor's lock.
	select {
	case err := <-released:
		t.Fatalf("release returned while heartbeat Chtimes was in flight: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	closeChtimes() // let the in-flight heartbeat finish

	if err := <-released; err != nil {
		t.Fatalf("release failed: %v", err)
	}

	// The heartbeat goroutine is joined: no Chtimes may complete after release
	// has returned, no matter how many intervals pass.
	afterRelease := completed.Load()
	time.Sleep(3 * workflowLockHeartbeatInterval)
	if got := completed.Load(); got != afterRelease {
		t.Errorf("heartbeat completed %d Chtimes after release returned", got-afterRelease)
	}
}

// assertNoReclaimDirs fails the test if any quarantine directory from an
// unfinished lock reclamation survives under root.
func assertNoReclaimDirs(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read lock root %s: %v", root, err)
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), ".") && strings.Contains(e.Name(), ".reclaim-") {
			t.Errorf("quarantine directory %s survived the ownership-lost return", filepath.Join(root, e.Name()))
		}
	}
}
