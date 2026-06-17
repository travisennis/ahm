package ahm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestCleanupStaleTemps_NoAgentsDir(t *testing.T) {
	dir := t.TempDir()

	// No .agents directory — should not error.
	if err := cleanupStaleTemps(dir); err != nil {
		t.Errorf("cleanupStaleTemps on dir without .agents: %v", err)
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
