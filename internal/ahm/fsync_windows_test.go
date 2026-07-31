//go:build windows

package ahm

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFsyncDir_NoopOnWindows pins the portability compromise: directory fsync
// is skipped on Windows because a read-only directory handle cannot be synced
// (FlushFileBuffers fails with ERROR_ACCESS_DENIED), which would make every
// atomic write report an error after the rename already succeeded.
func TestFsyncDir_NoopOnWindows(t *testing.T) {
	dir := t.TempDir()
	if err := fsyncDir(dir); err != nil {
		t.Fatalf("fsyncDir(%q) = %v, want nil", dir, err)
	}

	// A full atomic write must also succeed, proving writeFileAtomic does not
	// surface the directory-sync failure.
	path := filepath.Join(dir, "test.txt")
	if err := writeFileAtomic(path, []byte("windows"), 0o644); err != nil {
		t.Fatalf("writeFileAtomic on Windows: %v", err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "windows" {
		t.Fatalf("read back = %q, %v; want %q", string(data), err, "windows")
	}
}
