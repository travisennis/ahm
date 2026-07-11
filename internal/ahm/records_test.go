package ahm

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCollectRecordFilesSelectsAhmSourceRecords(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "# Task\n")
	writeFile(t, filepath.Join(root, ".ahm", "tasks", "active", "index.md"), "# Generated\n")
	writeFile(t, filepath.Join(root, ".ahm", "research", "topics", "storage.md"), "# Research\n")
	writeFile(t, filepath.Join(root, ".ahm", "research", "index.md"), "# Generated\n")
	writeFile(t, filepath.Join(root, ".ahm", "exec-plans", "active", "plan.md"), "# Plan\n")
	writeFile(t, filepath.Join(root, ".ahm", "exec-plans", "active", "index.md"), "# Generated\n")
	writeFile(t, filepath.Join(root, ".ahm", "config.json"), "{}\n")
	writeFile(t, filepath.Join(root, ".agents", ".tasks", "active", "legacy.md"), "# Legacy\n")

	files, err := collectRecordFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, file := range files {
		got = append(got, file.RelPath)
	}
	want := []string{
		".ahm/exec-plans/active/plan.md",
		".ahm/research/topics/storage.md",
		".ahm/tasks/active/001.md",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("record files = %#v, want %#v", got, want)
	}
}

func TestCollectRecordFilesRejectsSymlinkedMarkdownRecords(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "outside.md"), "# Outside\n")
	if err := os.MkdirAll(filepath.Join(root, ".ahm", "tasks", "active"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "outside.md"), filepath.Join(root, ".ahm", "tasks", "active", "001.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "outside.md"), filepath.Join(root, ".ahm", "tasks", "active", "ignored.txt")); err != nil {
		t.Fatal(err)
	}

	_, err := collectRecordFiles(root)
	if err == nil {
		t.Fatal("expected symlinked markdown record to be rejected")
	}
	assertContainsAll(t, err.Error(), "record file symlinks are not supported", ".ahm/tasks/active/001.md")
}

// newGitRepo creates a new git repository for testing.
func newGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			t.Skip("git not available")
		}
		t.Fatal(err)
	}
	root := t.TempDir()
	git(t, root, "init", "-q", "-b", "main")
	git(t, root, "config", "user.name", "Test User")
	git(t, root, "config", "user.email", "test@example.com")
	return root
}

// writeAHMConfig writes a minimal .ahm/config.json so workflow paths resolve
// to the migrated .ahm/ layout.
func writeAHMConfig(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, ".ahm", "config.json"), `{
  "version": "test",
  "strict_acceptance": false,
  "files": {}
}
`)
}

func git(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmdArgs := append([]string{"-C", root}, args...)
	cmd := exec.Command("git", cmdArgs...) // #nosec G204 // tests run git with explicit test arguments in a temp repo.
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}
