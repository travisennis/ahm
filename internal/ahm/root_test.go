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
	if !strings.Contains(err.Error(), "no .git or .ahm/config.json found") {
		t.Errorf("error should mention missing markers: %v", err)
	}
	if !strings.Contains(err.Error(), "--root") {
		t.Errorf("error should mention --root: %v", err)
	}
	if !strings.Contains(err.Error(), "ahm init") {
		t.Errorf("error should mention ahm init: %v", err)
	}
}

// assertDetectedRootEqual compares a detected root against the expected repo
// root after canonicalizing both sides: on Windows the current directory may
// resolve through an 8.3 short name (e.g. RUNNER~1) while EvalSymlinks returns
// the long name, so a raw comparison would spuriously fail.
func assertDetectedRootEqual(t *testing.T, got string, root string) {
	t.Helper()
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatalf("canonicalize detected root %q: %v", got, err)
	}
	if canonical != want {
		t.Errorf("root = %q, want %q", got, want)
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
	assertDetectedRootEqual(t, got, root)
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
	assertDetectedRootEqual(t, got, root)
}

// TestDetectManagedRootFailsOnLegacyLayout covers the one layout this version
// cannot read: a repository whose ahm metadata is still .agents/ahm.json. Root
// detection refuses it and names the final v1 release that can migrate it,
// because initializing over it would leave its records behind.
func TestDetectManagedRootFailsOnLegacyLayout(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".agents", "ahm.json"), "{}\n")

	withWorkingDir(t, root, func() {
		_, err := detectManagedRoot()
		if err == nil {
			t.Fatal("detectManagedRoot accepted a legacy .agents/ahm.json repository")
		}
		assertContainsAll(t, err.Error(), filepath.FromSlash(legacyMetadataRelPath), finalV1Release)
	})
}

// TestDetectManagedRootFailsOnLegacyLayoutInParent covers the same refusal when
// the legacy metadata sits in an ancestor directory.
func TestDetectManagedRootFailsOnLegacyLayoutInParent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".agents", "ahm.json"), "{}\n")
	nested := filepath.Join(root, "nested", "deeper")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	withWorkingDir(t, nested, func() {
		_, err := detectManagedRoot()
		if err == nil {
			t.Fatal("detectManagedRoot accepted a nested path inside a legacy repository")
		}
		assertContainsAll(t, err.Error(), finalV1Release)
	})
}

// TestDetectManagedRootSucceedsWithBothMetadataFiles covers the ordering rule:
// `.ahm/config.json` wins over a leftover `.agents/ahm.json`, because the v1
// migration wrote the config after moving the records.
func TestDetectManagedRootSucceedsWithBothMetadataFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".agents", "ahm.json"), "{}\n")
	writeFile(t, filepath.Join(root, ".ahm", "config.json"), "{}\n")

	withWorkingDir(t, root, func() {
		got, err := detectManagedRoot()
		if err != nil {
			t.Fatalf("detectManagedRoot() = %v, want the managed root", err)
		}
		assertDetectedRootEqual(t, got, root)
	})
}

// withWorkingDir runs f with the process working directory set to dir and
// restores the original directory afterwards.
func withWorkingDir(t *testing.T, dir string, f func()) {
	t.Helper()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if chErr := os.Chdir(origDir); chErr != nil {
			t.Errorf("failed to restore working directory: %v", chErr)
		}
	})
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	f()
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
	assertDetectedRootEqual(t, got, root)
}

func TestStrictCommandsFailOutsideManagedRepository(t *testing.T) {
	root := t.TempDir()
	// Temp dir has no .git and no .ahm/config.json

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
			if !strings.Contains(stderr, "no .git or .ahm/config.json found") {
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
	assertContainsAll(t, stdout, "created:", "  .ahm/config.json", "indexes:")
	assertNotContains(t, stdout, "AGENTS.md", ".agents/TASKS.md")
}

// TestInitIsIdempotentOutsideManagedRepository proves the acceptance criterion
// that init on an up-to-date repository writes nothing: every file ahm owns
// keeps its content and its modification time.
func TestInitIsIdempotentOutsideManagedRepository(t *testing.T) {
	root := t.TempDir()
	if _, stderr, code := runCLIFromDir(t, root, "init"); code != 0 {
		t.Fatalf("first init exit code = %d, stderr = %s", code, stderr)
	}
	before := snapshotTree(t, root)

	stdout, stderr, code := runCLIFromDir(t, root, "init")
	if code != 0 {
		t.Errorf("second init exit code = %d, stderr = %s", code, stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("second init reported work on an up-to-date repository:\n%s", stdout)
	}
	assertTreeUnchanged(t, root, before)
}

func TestStatusSucceedsAfterInitInCleanDir(t *testing.T) {
	root := t.TempDir()
	// Canonicalize up front: the status output echoes the current directory,
	// which on Windows may resolve through an 8.3 short name when the temp
	// directory is reached via its short form.
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	root = canonicalRoot

	// init succeeds outside managed repo
	stdout, stderr, code := runCLIFromDir(t, root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "created:")

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
