package ahm

import (
	"os"
	"path/filepath"
	"testing"
)

// TestIsDirNotEmpty_NonEmptyDir verifies the cross-platform helper recognizes
// the platform's directory-not-empty error, which is syscall.ENOTEMPTY on Unix
// and ERROR_DIR_NOT_EMPTY on Windows. Producing the error through a real
// os.Remove keeps the test portable.
func TestIsDirNotEmpty_NonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := os.Remove(sub)
	if err == nil {
		t.Fatal("expected removal of a non-empty directory to fail")
	}
	if !isDirNotEmpty(err) {
		t.Errorf("isDirNotEmpty(%v) = false, want true", err)
	}
}

func TestIsDirNotEmpty_EmptyDirRemovalSucceeds(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(sub); err != nil {
		t.Fatalf("removing an empty directory: %v", err)
	}
	if isDirNotEmpty(nil) {
		t.Error("isDirNotEmpty(nil) = true, want false")
	}
}

func TestIsDirNotEmpty_MissingDir(t *testing.T) {
	dir := t.TempDir()
	err := os.Remove(filepath.Join(dir, "missing"))
	if err == nil {
		t.Fatal("expected removal of a missing directory to fail")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("missing dir error = %v, want os.ErrNotExist", err)
	}
	if isDirNotEmpty(err) {
		t.Errorf("isDirNotEmpty(%v) = true, want false", err)
	}
}

func TestIsDirNotEmpty_UnrelatedError(t *testing.T) {
	// Errors that are not directory-not-empty (here, a permission error) must
	// not be treated as a tolerated condition; callers return them verbatim.
	if isDirNotEmpty(os.ErrPermission) {
		t.Error("isDirNotEmpty(os.ErrPermission) = true, want false")
	}
}
