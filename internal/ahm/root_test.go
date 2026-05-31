package ahm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/travisennis/ahm/internal/templates"
)

func TestRuntimeErrorsExitCode1(t *testing.T) {
	// Running outside a managed project should produce a runtime error (exit 1).
	dir := t.TempDir()
	_, stderr, code := runCLIFromDir(t, dir, "status")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "error:") {
		t.Fatalf("stderr missing 'error:' prefix:\n%s", stderr)
	}
}

func TestRuntimeErrorsOnTaskCommandOutsideRepo(t *testing.T) {
	// Task commands outside a managed repo should exit 1 with an error message.
	dir := t.TempDir()
	tests := []string{"list", "ready", "next"}
	for _, cmd := range tests {
		t.Run(cmd, func(t *testing.T) {
			_, stderr, code := runCLIFromDir(t, dir, "task", cmd)
			if code != 1 {
				t.Fatalf("exit code = %d, want 1; stderr = %s", code, stderr)
			}
			if !strings.Contains(stderr, "error:") {
				t.Fatalf("stderr missing 'error:' prefix:\n%s", stderr)
			}
		})
	}
}

func TestDetectManagedRootFailsWithoutGitOrMetadata(t *testing.T) {
	root := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if chErr := os.Chdir(origDir); chErr != nil {
			t.Errorf("failed to restore working directory: %v", chErr)
		}
	}()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	_, err = detectManagedRoot()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no .git or .agents/ahm.json found") {
		t.Fatalf("error should mention missing markers: %v", err)
	}
	if !strings.Contains(err.Error(), "--root") {
		t.Fatalf("error should mention --root: %v", err)
	}
	if !strings.Contains(err.Error(), "ahm init") {
		t.Fatalf("error should mention ahm init: %v", err)
	}
}

func TestDetectManagedRootSucceedsWithDotGit(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if chErr := os.Chdir(origDir); chErr != nil {
			t.Errorf("failed to restore working directory: %v", chErr)
		}
	}()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	got, err := detectManagedRoot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("root = %q, want %q", got, want)
	}
}

func TestDetectManagedRootSucceedsWithMetadata(t *testing.T) {
	root := t.TempDir()
	metaDir := filepath.Join(root, ".agents")
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metaDir, "ahm.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if chErr := os.Chdir(origDir); chErr != nil {
			t.Errorf("failed to restore working directory: %v", chErr)
		}
	}()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	got, err := detectManagedRoot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("root = %q, want %q", got, want)
	}
}

func TestStrictCommandsFailOutsideManagedRepository(t *testing.T) {
	root := t.TempDir()
	// Temp dir has no .git and no .agents/ahm.json

	for _, args := range [][]string{
		{"status"},
		{"doctor"},
		{"index"},
		{"task", "list"},
		{"task", "next"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, stderr, code := runCLIFromDir(t, root, args...)
			if code != 1 {
				t.Fatalf("exit code = %d, want 1; stderr = %s", code, stderr)
			}
			if !strings.Contains(stderr, "no .git or .agents/ahm.json found") {
				t.Fatalf("stderr should mention no .git or .agents/ahm.json: %s", stderr)
			}
		})
	}
}

func TestInitSucceedsOutsideManagedRepository(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLIFromDir(t, root, "init")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "created:", "  AGENTS.md")
}

func TestUpgradeSucceedsOutsideManagedRepository(t *testing.T) {
	root := t.TempDir()

	// upgrade without prior install: lenient, acts like init
	stdout, stderr, code := runCLIFromDir(t, root, "upgrade")
	if code != 0 {
		t.Fatalf("upgrade exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "created:", "  AGENTS.md")

	// upgrade again: should skip unchanged files
	stdout, stderr, code = runCLIFromDir(t, root, "upgrade")
	if code != 0 {
		t.Fatalf("second upgrade exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "skipped:")
}

func TestStatusSucceedsAfterInitInCleanDir(t *testing.T) {
	root := t.TempDir()

	// init succeeds outside managed repo
	stdout, stderr, code := runCLIFromDir(t, root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "created:")

	// status now succeeds because .agents/ahm.json exists
	stdout, stderr, code = runCLIFromDir(t, root, "status")
	if code != 0 {
		t.Fatalf("status exit code = %d, stderr = %s", code, stderr)
	}
	// root in output is symlink-resolved; compare with evaluated path
	evalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, evalRoot) {
		t.Fatalf("status output missing root %q:\n%s", evalRoot, stdout)
	}
	assertContainsAll(t, stdout, templates.Version)
}
