package ahm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeErrorsExitCode1(t *testing.T) {
	// Running outside a managed project should produce a runtime error (exit 1).
	dir := t.TempDir()
	_, stderr, code := runCLIFromDir(t, dir, "status")
	if code != 1 {
		t.Errorf("exit code = %d, want 1; stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "error:") {
		t.Errorf("stderr missing 'error:' prefix:\n%s", stderr)
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
				t.Errorf("exit code = %d, want 1; stderr = %s", code, stderr)
			}
			if !strings.Contains(stderr, "error:") {
				t.Errorf("stderr missing 'error:' prefix:\n%s", stderr)
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
		t.Error("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no .git, .ahm/config.json, or .agents/ahm.json found") {
		t.Errorf("error should mention missing markers: %v", err)
	}
	if !strings.Contains(err.Error(), "--root") {
		t.Errorf("error should mention --root: %v", err)
	}
	if !strings.Contains(err.Error(), "ahm init") {
		t.Errorf("error should mention ahm init: %v", err)
	}
}

func TestDetectManagedRootSucceedsWithDotGitFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: /other/worktree/.git\n"), 0o644); err != nil {
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
		t.Errorf("unexpected error: %v", err)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("root = %q, want %q", got, want)
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
		t.Errorf("unexpected error: %v", err)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("root = %q, want %q", got, want)
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
		t.Errorf("unexpected error: %v", err)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("root = %q, want %q", got, want)
	}
}

func TestDetectManagedRootSucceedsWithAhmConfig(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, ".ahm")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte("{}"), 0o644); err != nil {
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
		t.Errorf("unexpected error: %v", err)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("root = %q, want %q", got, want)
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
				t.Errorf("exit code = %d, want 1; stderr = %s", code, stderr)
			}
			if !strings.Contains(stderr, "no .git, .ahm/config.json, or .agents/ahm.json found") {
				t.Errorf("stderr should mention missing root markers: %s", stderr)
			}
		})
	}
}

func TestInitSucceedsOutsideManagedRepository(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLIFromDir(t, root, "init")
	if code != 0 {
		t.Errorf("exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "metadata:", "  .ahm/config.json", "indexes:")
	assertNotContains(t, stdout, "AGENTS.md", ".agents/TASKS.md")
}

func TestUpgradeSucceedsOutsideManagedRepository(t *testing.T) {
	root := t.TempDir()

	// upgrade without prior install: lenient, acts like init
	stdout, stderr, code := runCLIFromDir(t, root, "upgrade")
	if code != 0 {
		t.Errorf("upgrade exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "metadata:", "  .agents/ahm.json", "indexes:")
	assertNotContains(t, stdout, "AGENTS.md", ".agents/TASKS.md")

	// upgrade again: should skip unchanged files
	stdout, stderr, code = runCLIFromDir(t, root, "upgrade")
	if code != 0 {
		t.Errorf("second upgrade exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "metadata:", "  .agents/ahm.json", "indexes:")
}

func TestStatusSucceedsAfterInitInCleanDir(t *testing.T) {
	root := t.TempDir()

	// init succeeds outside managed repo
	stdout, stderr, code := runCLIFromDir(t, root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "metadata:", "indexes:")

	// status now succeeds because .ahm/config.json exists
	stdout, stderr, code = runCLIFromDir(t, root, "status")
	if code != 0 {
		t.Errorf("status exit code = %d, stderr = %s", code, stderr)
	}
	// root in output is symlink-resolved; compare with evaluated path
	evalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, evalRoot) {
		t.Errorf("status output missing root %q:\n%s", evalRoot, stdout)
	}
}
