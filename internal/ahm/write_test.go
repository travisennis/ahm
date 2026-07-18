package ahm

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWriteFileAtomic_WritesContentCorrectly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	if err := writeFileAtomic(path, []byte("hello world"), 0o644); err != nil {
		t.Errorf("writeFileAtomic failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("read back failed: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("content = %q, want %q", string(data), "hello world")
	}
}

func TestWriteFileAtomic_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "test.txt")

	if err := writeFileAtomic(path, []byte("nested"), 0o644); err != nil {
		t.Errorf("writeFileAtomic failed: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("stat nested file: %v", err)
	}
}

func TestWriteFileAtomic_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := writeFileAtomic(path, []byte("updated"), 0o644); err != nil {
		t.Errorf("writeFileAtomic failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("read back failed: %v", err)
	}
	if string(data) != "updated" {
		t.Errorf("content = %q, want %q", string(data), "updated")
	}
}

func TestWriteFileAtomic_RejectsEmptyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	// Write original content.
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Use an empty path to trigger canonical-path validation failure.
	// This also verifies that a pre-existing file is left intact.
	err := writeFileAtomic("", []byte("should fail"), 0o644)
	if err == nil {
		t.Error("expected error for empty path")
	}

	// Original file must be unchanged.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("read original failed: %v", err)
	}
	if string(data) != "original" {
		t.Errorf("original content = %q, want %q", string(data), "original")
	}
}

func TestWriteFileAtomic_SucceedsWithStaleFixedTmp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	stalePath := path + ".tmp"

	// Create a stale fixed-name .tmp file left by a pre-unique-naming crash.
	if err := os.WriteFile(stalePath, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Perform a fresh atomic write — must succeed even with the stale .tmp present.
	if err := writeFileAtomic(path, []byte("fresh"), 0o644); err != nil {
		t.Errorf("writeFileAtomic failed: %v", err)
	}

	// Content should be the new write.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("read failed: %v", err)
	}
	if string(data) != "fresh" {
		t.Errorf("content = %q, want %q", string(data), "fresh")
	}

	// The stale .tmp file should still exist (writeFileAtomic no longer
	// removes fixed-name .tmp files). cleanupStaleTemps handles that.
	if _, err := os.Stat(stalePath); err != nil {
		t.Errorf("stale .tmp was removed, but cleanupStaleTemps is responsible for that: %v", err)
	}
}

func TestWriteFileAtomic_StaleTmpCleanedByCleanupStaleTemps(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, ".agents")
	path := filepath.Join(agentsDir, "test.txt")
	stalePath := path + ".tmp"

	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a real file.
	if err := os.WriteFile(path, []byte("real"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a stale .tmp alongside it (crash that succeeded past rename).
	if err := os.WriteFile(stalePath, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Age the .tmp file so cleanupStaleTemps considers it stale.
	past := time.Now().Add(-2 * cleanupStaleTempMaxAge)
	if err := os.Chtimes(stalePath, past, past); err != nil {
		t.Fatal(err)
	}

	// cleanupStaleTemps must remove the stale .tmp.
	if err := cleanupStaleTemps(dir); err != nil {
		t.Errorf("cleanupStaleTemps: %v", err)
	}
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Errorf("stale .tmp file was not removed by cleanupStaleTemps")
	}

	// Real file must still exist.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("real file was removed: %v", err)
	}
}

func TestWriteFileAtomic_RejectsNonCanonicalPath(t *testing.T) {
	err := writeFileAtomic("/tmp/../etc/passwd", []byte("bad"), 0o644)
	if err == nil {
		t.Error("expected error for non-canonical path")
	}
	if !strings.Contains(err.Error(), "non-canonical path") {
		t.Errorf("error = %q, want 'non-canonical path'", err)
	}
}

func TestWriteFileAtomic_AcceptsCanonicalAbsolutePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absolute.txt")

	if err := writeFileAtomic(path, []byte("absolute"), 0o644); err != nil {
		t.Fatalf("writeFileAtomic failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back failed: %v", err)
	}
	if string(data) != "absolute" {
		t.Errorf("content = %q, want %q", string(data), "absolute")
	}
}

func TestWriteFileAtomic_AcceptsCanonicalParentTraversal(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(child)

	path := filepath.Join("..", "parent.txt")
	if err := writeFileAtomic(path, []byte("parent"), 0o644); err != nil {
		t.Fatalf("writeFileAtomic failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "parent.txt"))
	if err != nil {
		t.Fatalf("read back failed: %v", err)
	}
	if string(data) != "parent" {
		t.Errorf("content = %q, want %q", string(data), "parent")
	}
}

func TestCleanupStaleTemps(t *testing.T) {
	dir := t.TempDir()

	// Create .agents directory.
	agentsDir := filepath.Join(dir, ".agents")
	taskDir := filepath.Join(agentsDir, ".tasks", "active")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a real file.
	realFile := filepath.Join(taskDir, "001.md")
	if err := os.WriteFile(realFile, []byte("task"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create an orphaned .tmp file (no corresponding real file).
	orphanTmp := filepath.Join(taskDir, "orphan.md.tmp")
	if err := os.WriteFile(orphanTmp, []byte("orphan"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a stale .tmp file alongside a real file (simulating a crash
	// where the rename succeeded but the .tmp wasn't cleaned up).
	staleTmp := filepath.Join(taskDir, "001.md.tmp")
	if err := os.WriteFile(staleTmp, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Age the stale .tmp files so cleanupStaleTemps considers them stale.
	past := time.Now().Add(-2 * cleanupStaleTempMaxAge)
	if err := os.Chtimes(orphanTmp, past, past); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(staleTmp, past, past); err != nil {
		t.Fatal(err)
	}

	// Run cleanup.
	if err := cleanupStaleTemps(dir); err != nil {
		t.Errorf("cleanupStaleTemps: %v", err)
	}

	// Orphan .tmp should be gone.
	if _, err := os.Stat(orphanTmp); !os.IsNotExist(err) {
		t.Errorf("orphan .tmp file was not removed")
	}

	// Stale .tmp alongside real file should be gone.
	if _, err := os.Stat(staleTmp); !os.IsNotExist(err) {
		t.Errorf("stale .tmp file was not removed")
	}

	// Real file must still exist.
	if _, err := os.Stat(realFile); err != nil {
		t.Errorf("real file was removed: %v", err)
	}
}

func TestCleanupStaleTemps_ContinuesPastRemoveFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test relies on filesystem permissions; root bypasses them")
	}
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, ".agents")

	// A read-only subdirectory holds a .tmp file that cannot be removed:
	// on Unix, removing a file requires write permission on its parent dir.
	// WalkDir visits directories in lexical order, so "locked" is walked
	// before "writable" — proving the walk continues past the failure.
	lockedDir := filepath.Join(agentsDir, "locked")
	if err := os.MkdirAll(lockedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stuckTmp := filepath.Join(lockedDir, "stuck.md.tmp")
	if err := os.WriteFile(stuckTmp, []byte("stuck"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A removable orphan .tmp in a separate writable directory.
	writableDir := filepath.Join(agentsDir, "writable")
	if err := os.MkdirAll(writableDir, 0o755); err != nil {
		t.Fatal(err)
	}
	removableTmp := filepath.Join(writableDir, "orphan.md.tmp")
	if err := os.WriteFile(removableTmp, []byte("orphan"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Age both .tmp files so cleanupStaleTemps considers them stale. The stuck
	// file must be old enough to be eligible for removal; that eligibility is
	// what triggers the permission-denied failure below.
	past := time.Now().Add(-2 * cleanupStaleTempMaxAge)
	if err := os.Chtimes(stuckTmp, past, past); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(removableTmp, past, past); err != nil {
		t.Fatal(err)
	}

	// Make the locked directory read-only so its .tmp cannot be removed.
	if err := os.Chmod(lockedDir, 0o555); err != nil {
		t.Fatal(err)
	}
	// Restore write permission so t.TempDir cleanup can remove the file.
	t.Cleanup(func() { _ = os.Chmod(lockedDir, 0o755) })

	err := cleanupStaleTemps(dir)
	if err == nil {
		t.Fatal("expected a non-fatal cleanup error for the unremovable .tmp")
	}

	// The removable .tmp in the other directory must still be cleaned up,
	// proving the walk continued past the failed os.Remove.
	if _, statErr := os.Stat(removableTmp); !os.IsNotExist(statErr) {
		t.Errorf("removable .tmp was not cleaned up after an earlier remove failure")
	}

	// The unremovable .tmp must still be present.
	if _, statErr := os.Stat(stuckTmp); statErr != nil {
		t.Errorf("unremovable .tmp unexpectedly gone: %v", statErr)
	}
}

func TestCleanupStaleTemps_NoAgentsDir(t *testing.T) {
	dir := t.TempDir()

	// No .agents directory — should not error.
	if err := cleanupStaleTemps(dir); err != nil {
		t.Errorf("cleanupStaleTemps on dir without .agents: %v", err)
	}
}

func TestCleanupStaleTemps_SkipsFreshTemp(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, ".agents")
	freshTmp := filepath.Join(agentsDir, "fresh.md.tmp")

	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(freshTmp, []byte("fresh"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The fresh .tmp file must be preserved.
	if err := cleanupStaleTemps(dir); err != nil {
		t.Errorf("cleanupStaleTemps: %v", err)
	}
	if _, err := os.Stat(freshTmp); err != nil {
		t.Errorf("fresh .tmp file was removed: %v", err)
	}
}

func TestCleanupStaleTemps_RemovesOldOrphanTemp(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, ".agents")
	orphanTmp := filepath.Join(agentsDir, "orphan.md.tmp")

	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orphanTmp, []byte("orphan"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Age the orphan .tmp file so cleanupStaleTemps considers it stale.
	past := time.Now().Add(-2 * cleanupStaleTempMaxAge)
	if err := os.Chtimes(orphanTmp, past, past); err != nil {
		t.Fatal(err)
	}

	if err := cleanupStaleTemps(dir); err != nil {
		t.Errorf("cleanupStaleTemps: %v", err)
	}
	if _, err := os.Stat(orphanTmp); !os.IsNotExist(err) {
		t.Errorf("old orphan .tmp file was not removed")
	}
}

func TestCleanupStaleTemps_RaceWithActiveWriter(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, ".agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	var activeTmp string
	ready := make(chan struct{})
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		f, err := os.CreateTemp(agentsDir, "active.md.*.tmp")
		if err != nil {
			t.Errorf("create active temp: %v", err)
			close(ready)
			return
		}
		activeTmp = f.Name()
		close(ready)
		<-done
		_ = f.Close()
	}()

	<-ready
	if activeTmp == "" {
		t.Fatal("active temp path was not set")
	}

	// Run cleanup while the goroutine still holds the temp file open.
	if err := cleanupStaleTemps(dir); err != nil {
		t.Errorf("cleanupStaleTemps: %v", err)
	}
	close(done)
	wg.Wait()

	// The active temp file must still exist because it is fresh.
	if _, err := os.Stat(activeTmp); err != nil {
		t.Errorf("active .tmp file was removed during an active write: %v", err)
	}

	// Once the writer is finished, the file should still be removable after
	// aging it past the threshold.
	past := time.Now().Add(-2 * cleanupStaleTempMaxAge)
	if err := os.Chtimes(activeTmp, past, past); err != nil {
		t.Fatal(err)
	}
	if err := cleanupStaleTemps(dir); err != nil {
		t.Errorf("cleanupStaleTemps after aging: %v", err)
	}
	if _, err := os.Stat(activeTmp); !os.IsNotExist(err) {
		t.Errorf("aged .tmp file was not removed after the active writer finished")
	}
}

func TestWriteFileAtomic_Permissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exec.sh")

	if err := writeFileAtomic(path, []byte("#!/bin/sh\necho hi"), 0o755); err != nil {
		t.Errorf("writeFileAtomic failed: %v", err)
	}

	stat, err := os.Stat(path)
	if err != nil {
		t.Errorf("stat failed: %v", err)
	}
	// Check that the execute bit is set (mode 0o755 has 0100).
	if stat.Mode().Perm()&0o100 == 0 {
		t.Errorf("file is not executable: mode = %o", stat.Mode().Perm())
	}
}
