package ahm

import (
	"fmt"
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
// path under it. Tests keep a store root, and accept a root ahm resolved, only
// when they can prove it is a scratch directory. An empty path is never inside.
// Both sides are canonicalized first, because one directory has more than one
// spelling: macOS reaches the temporary directory as /var/... and
// /private/var/..., and Windows resolves a current directory through an 8.3
// short name. t.TempDir, os.Getwd, and EvalSymlinks do not agree on which
// spelling they return, so comparing the literal paths would call a temporary
// path external.
func withinTempDir(path string) bool {
	if path == "" {
		return false
	}
	return pathWithin(canonicalPath(os.TempDir()), canonicalPath(path))
}

// canonicalPath resolves the symbolic links in path's existing prefix and
// leaves the rest in place, so a path that does not exist yet — a store
// directory a test is about to create — still compares by the directory it
// would land in.
func canonicalPath(path string) string {
	clean := filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		return resolved
	}
	parent := filepath.Dir(clean)
	if parent == clean {
		return clean
	}
	return filepath.Join(canonicalPath(parent), filepath.Base(clean))
}

// assertResolvedRootWithinTempDir is the guard the test binary installs on
// resolvedRootHook. It fails the run when ahm resolves a root outside the
// temporary directory the tests run in, because every workflow path ahm
// derives comes from such a root: a resolved root outside the sandbox is a path
// a mutating test could reach the developer's real workflow records through.
// The panic names the offending path, and stopping the run is the point — a
// suite that continues past it writes through a root the test does not own.
func assertResolvedRootWithinTempDir(path string) {
	if withinTempDir(path) {
		return
	}
	panic(fmt.Sprintf(
		"ahm resolved %s, which is outside the test temporary directory %s: build fixtures with t.TempDir (projectRoot, setupAhmRepo, newGitRepo), run commands through the helpers in test_helpers_test.go, and point the store at a scratch root with setStoreHome or a temporary AHM_HOME",
		path, os.TempDir()))
}

// testStorePaths returns a resolved store location for a scratch store, so a
// test can build the home-mode workflow paths without running identity
// resolution against a real repository.
func testStorePaths(t *testing.T) storePaths {
	t.Helper()
	root := t.TempDir()
	return storePaths{
		Root:       root,
		Key:        "example.com/owner/repo",
		Kind:       "remote",
		ProjectDir: filepath.Join(root, storeProjectsDirName, "repo-3f9ac4d1"),
	}
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
// environment the test set, including AHM_HOME, and replaces no store root of
// its own. Tests that exercise an unusual AHM_HOME use it; the scratch store
// root the test binary pins at startup is what an unset one resolves to.
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

// projectRoot returns a temporary directory prepared as a repository that
// predates the home store: its committed configuration carries no
// tasks_location key, so its records stay in the project. A repository with no
// configuration at all is a new project and defaults to the store, so a test
// that exercises the in-project layout asks for this fixture instead of a bare
// temporary directory.
func projectRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeMetadataFile(t, root, metadata{Files: map[string]string{}})
	return root
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

// writeTaskFileWithBody writes a task whose Markdown body is caller-supplied,
// so tests can exercise body-text search independently of the title.
func writeTaskFileWithBody(t *testing.T, path string, id string, title string, status string, priority string, labels string, body string) {
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
		"labels: " + labels + "\n" +
		"---\n" +
		"# " + title + "\n\n" +
		body + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
