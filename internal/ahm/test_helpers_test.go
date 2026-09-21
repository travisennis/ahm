package ahm

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setStoreHome points the home store at a temporary directory and returns it.
// Tests that need several commands to share one store call it first; the CLI
// helpers below keep whatever value is already set.
func setStoreHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv(storeHomeEnvVar, home)
	return home
}

// useTemporaryStoreHome points the store at a temporary directory unless the
// test chose one under the system temporary directory itself. A store root
// outside it — the developer's real ~/.ahm, or any AHM_HOME a shell exports at
// such a path — is replaced. A value under the system temporary directory is
// kept, because that is how a test pre-populates a store before running a
// command.
func useTemporaryStoreHome(t *testing.T) {
	t.Helper()
	if home, ok := os.LookupEnv(storeHomeEnvVar); ok && withinTempDir(home) {
		return
	}
	setStoreHome(t)
}

// withinTempDir reports whether path is the system temporary directory or a
// path under it. Tests only keep a store root they can prove is a scratch
// directory.
func withinTempDir(path string) bool {
	tmp := filepath.Clean(os.TempDir())
	clean := filepath.Clean(path)
	return clean == tmp || strings.HasPrefix(clean, tmp+string(filepath.Separator))
}

func runCLI(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	useTemporaryStoreHome(t)
	var stdout strings.Builder
	var stderr strings.Builder
	code := Main(args, &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}

func runCLIFromDir(t *testing.T, dir string, args ...string) (string, string, int) {
	t.Helper()
	useTemporaryStoreHome(t)
	return runCLIFromDirKeepingEnv(t, dir, args...)
}

// runCLIFromDirKeepingEnv runs the CLI in process from dir with exactly the
// environment the test set, including AHM_HOME, and installs no scratch store
// root. Tests that exercise an unusual AHM_HOME use it.
func runCLIFromDirKeepingEnv(t *testing.T, dir string, args ...string) (string, string, int) {
	t.Helper()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	var stdout strings.Builder
	var stderr strings.Builder
	code := Main(args, &stdout, &stderr)
	if chErr := os.Chdir(origDir); chErr != nil {
		t.Errorf("failed to restore working directory: %v", chErr)
	}
	return stdout.String(), stderr.String(), code
}

// assertSingleTrailingNewline fails when content does not end with exactly one
// newline. Files under docs/adr are linted, and a trailing blank line there is
// markdownlint MD012.
func assertSingleTrailingNewline(t *testing.T, content string) {
	t.Helper()
	if strings.HasSuffix(content, "\n") && !strings.HasSuffix(content, "\n\n") {
		return
	}
	tail := content
	if len(tail) > 16 {
		tail = tail[len(tail)-16:]
	}
	t.Fatalf("expected exactly one trailing newline, got trailing bytes %q", tail)
}

func assertContainsAll(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
}

func assertNotContains(t *testing.T, got string, unwanted ...string) {
	t.Helper()
	for _, item := range unwanted {
		if strings.Contains(got, item) {
			t.Errorf("output unexpectedly contains %q:\n%s", item, got)
		}
	}
}

func assertFileContainsAll(t *testing.T, path string, wants ...string) {
	t.Helper()
	assertContainsAll(t, mustRead(t, path), wants...)
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// setupAhmRepo creates minimal .ahm/ workflow state in root: .ahm/config.json
// and the directory structure ahm installs.
func setupAhmRepo(t *testing.T, root string) {
	t.Helper()
	for _, dir := range []string{
		".ahm/tasks/active",
		".ahm/tasks/completed",
		".ahm/tasks/cancelled",
		"docs/adr",
	} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".ahm", "config.json"), []byte(`{"version":"0.0.0"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeMetadataFile writes meta to .ahm/config.json in the same form install
// writes it.
func writeMetadataFile(t *testing.T, root string, meta metadata) {
	t.Helper()
	if meta.Files == nil {
		meta.Files = map[string]string{}
	}
	data, err := marshalMetadata(meta)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".ahm", "config.json"), string(data))
}

// treeEntry records what an idempotence check needs to detect a rewrite.
type treeEntry struct {
	modTime time.Time
	content string
}

// snapshotTree records every file under root with its content and modification
// time, so a test can prove a command rewrote nothing.
func snapshotTree(t *testing.T, root string) map[string]treeEntry {
	t.Helper()
	entries := map[string]treeEntry{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		entries[relPath(root, path)] = treeEntry{modTime: info.ModTime(), content: string(content)}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

// assertTreeUnchanged fails when root gained, lost, or rewrote any file
// relative to before.
func assertTreeUnchanged(t *testing.T, root string, before map[string]treeEntry) {
	t.Helper()
	after := snapshotTree(t, root)
	for rel, want := range before {
		got, ok := after[rel]
		if !ok {
			t.Errorf("%s was removed", rel)
			continue
		}
		if got.content != want.content {
			t.Errorf("%s content changed:\nbefore: %q\nafter:  %q", rel, want.content, got.content)
		}
		if !got.modTime.Equal(want.modTime) {
			t.Errorf("%s was rewritten without being changed (modification time moved)", rel)
		}
	}
	for rel := range after {
		if _, ok := before[rel]; !ok {
			t.Errorf("%s was created", rel)
		}
	}
}

func writeTaskFile(t *testing.T, path string, id string, title string, status string, extraFrontMatter string) {
	t.Helper()
	writeTaskFileWithPriority(t, path, id, title, status, "P2", extraFrontMatter)
}

func writeTaskFileWithPriority(t *testing.T, path string, id string, title string, status string, priority string, extraFrontMatter string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\n" +
		"id: " + id + "\n" +
		"title: " + title + "\n" +
		"status: " + status + "\n" +
		"priority: " + priority + "\n" +
		"effort: S\n" +
		"labels: type:task\n" +
		extraFrontMatter +
		"---\n" +
		"# " + title + "\n\n" +
		"## Summary\n\nTODO.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
