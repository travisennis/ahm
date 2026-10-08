package ahm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// registerRemoteCheckout returns a scratch Git repository whose origin gives the
// project a stable remote key.
func registerRemoteCheckout(t *testing.T) string {
	t.Helper()
	root := newGitRepo(t)
	git(t, root, "remote", "add", "origin", "git@github.com:travisennis/ahm.git")
	return root
}

// TestStoreUnregisterRemovesTheEntryAndKeepsRecords covers the default mode: it
// removes the current project's registry entry and touches no record and no
// store directory.
func TestStoreUnregisterRemovesTheEntryAndKeepsRecords(t *testing.T) {
	storeHome := setStoreHome(t)
	root := registerRemoteCheckout(t)

	if _, stderr, code := runCLIFromDir(t, root, "store", "path"); code != 0 {
		t.Fatalf("store path: %s", stderr)
	}
	paths, err := resolveStore(root)
	if err != nil {
		t.Fatal(err)
	}
	recordPath := filepath.Join(paths.recordsDir(), "active", "001.md")
	writeFile(t, recordPath, "record content\n")

	stdout, stderr, code := runCLIFromDir(t, root, "store", "unregister")
	if code != 0 {
		t.Fatalf("store unregister exited %d: %s", code, stderr)
	}
	assertContainsAll(t, stdout, "unregister "+paths.Key, "remove path "+paths.resolvedPath)

	reg, err := loadRegistry(storeHome)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Projects[paths.Key]; ok {
		t.Errorf("the registry still holds %s:\n%+v", paths.Key, reg.Projects)
	}
	if _, err := os.Stat(recordPath); err != nil {
		t.Errorf("unregister removed a record: %v", err)
	}
	if _, err := os.Stat(paths.ProjectDir); err != nil {
		t.Errorf("unregister removed the store directory: %v", err)
	}
}

// TestStoreUnregisterJSONReportsTheRemoval pins the structured payload a script
// reads to audit the change.
func TestStoreUnregisterJSONReportsTheRemoval(t *testing.T) {
	setStoreHome(t)
	root := registerRemoteCheckout(t)
	if _, stderr, code := runCLIFromDir(t, root, "store", "path"); code != 0 {
		t.Fatalf("store path: %s", stderr)
	}
	paths, err := resolveStore(root)
	if err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLIFromDir(t, root, "--json", "store", "unregister")
	if code != 0 {
		t.Fatalf("store unregister exited %d: %s", code, stderr)
	}
	var report storeUnregisterReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("unmarshal %q: %v", stdout, err)
	}
	want := storeUnregisterReport{
		Key:          paths.Key,
		Kind:         storeKindRemote,
		EntryRemoved: true,
		PathsRemoved: []string{paths.resolvedPath},
	}
	if report.Key != want.Key || report.EntryRemoved != want.EntryRemoved || !slices.Equal(report.PathsRemoved, want.PathsRemoved) {
		t.Errorf("json report = %+v, want %+v", report, want)
	}
}

// TestStoreUnregisterRemovesOneRecordedPath covers the --path mode: it removes
// one recorded path and leaves the entry and its other paths alone.
func TestStoreUnregisterRemovesOneRecordedPath(t *testing.T) {
	storeHome := setStoreHome(t)
	root := registerRemoteCheckout(t)
	if _, stderr, code := runCLIFromDir(t, root, "store", "path"); code != 0 {
		t.Fatalf("store path: %s", stderr)
	}
	paths, err := resolveStore(root)
	if err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(t.TempDir(), "stale-probe")
	reg, err := loadRegistry(storeHome)
	if err != nil {
		t.Fatal(err)
	}
	entry := reg.Projects[paths.Key]
	entry.Paths = append(entry.Paths, stale)
	reg.Projects[paths.Key] = entry
	writeRegistryFile(t, storeHome, reg)

	stdout, stderr, code := runCLIFromDir(t, root, "store", "unregister", "--path", stale)
	if code != 0 {
		t.Fatalf("store unregister --path exited %d: %s", code, stderr)
	}
	assertContainsAll(t, stdout, "unregister path "+stale, "from "+paths.Key, paths.resolvedPath)

	reg, err = loadRegistry(storeHome)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := reg.Projects[paths.Key]
	if !ok {
		t.Fatalf("--path removed the whole entry:\n%+v", reg.Projects)
	}
	if slices.Contains(entry.Paths, stale) {
		t.Errorf("entry still records the removed path %q: %v", stale, entry.Paths)
	}
	if !slices.Contains(entry.Paths, paths.resolvedPath) {
		t.Errorf("entry lost its other path %q: %v", paths.resolvedPath, entry.Paths)
	}
}

// TestStoreUnregisterReportsAbsentEntryAndPath pins the explicit outcome: an
// unregistered project and an unrecorded path are errors, not silent successes.
func TestStoreUnregisterReportsAbsentEntryAndPath(t *testing.T) {
	t.Run("absent entry", func(t *testing.T) {
		setStoreHome(t)
		root := registerRemoteCheckout(t)
		stdout, stderr, code := runCLIFromDir(t, root, "store", "unregister")
		if code != 1 {
			t.Fatalf("store unregister exited %d, want 1 for an unregistered project\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
		}
		assertContainsAll(t, stderr, "is not registered")
	})

	t.Run("absent path", func(t *testing.T) {
		setStoreHome(t)
		root := registerRemoteCheckout(t)
		if _, stderr, code := runCLIFromDir(t, root, "store", "path"); code != 0 {
			t.Fatalf("store path: %s", stderr)
		}
		missing := filepath.Join(t.TempDir(), "never-recorded")
		stdout, stderr, code := runCLIFromDir(t, root, "store", "unregister", "--path", missing)
		if code != 1 {
			t.Fatalf("store unregister --path exited %d, want 1 for an unrecorded path\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
		}
		assertContainsAll(t, stderr, "is not recorded")
	})
}

// TestStoreUnregisterDryRunWritesNothing checks the preview reports the removal
// without touching the store.
func TestStoreUnregisterDryRunWritesNothing(t *testing.T) {
	storeHome := setStoreHome(t)
	root := registerRemoteCheckout(t)
	if _, stderr, code := runCLIFromDir(t, root, "store", "path"); code != 0 {
		t.Fatalf("store path: %s", stderr)
	}
	before := snapshotTree(t, storeHome)

	stdout, stderr, code := runCLIFromDir(t, root, "--dry-run", "store", "unregister")
	if code != 0 {
		t.Fatalf("store unregister exited %d: %s", code, stderr)
	}
	assertContainsAll(t, stdout, "would unregister", "remove path")
	assertTreeUnchanged(t, storeHome, before)

	paths, err := resolveStore(root)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := loadRegistry(storeHome)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Projects[paths.Key]; !ok {
		t.Error("--dry-run removed the registry entry")
	}
}

// TestStoreUnregisterReRegistersCleanly covers the recoverability the command
// relies on: a later `store path` re-registers the same key and directory.
func TestStoreUnregisterReRegistersCleanly(t *testing.T) {
	storeHome := setStoreHome(t)
	root := registerRemoteCheckout(t)
	if _, stderr, code := runCLIFromDir(t, root, "store", "path"); code != 0 {
		t.Fatalf("store path: %s", stderr)
	}
	before, err := resolveStore(root)
	if err != nil {
		t.Fatal(err)
	}

	if _, stderr, code := runCLIFromDir(t, root, "store", "unregister"); code != 0 {
		t.Fatalf("store unregister: %s", stderr)
	}
	if _, stderr, code := runCLIFromDir(t, root, "store", "path"); code != 0 {
		t.Fatalf("store path after unregister: %s", stderr)
	}

	after, err := resolveStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if after.Key != before.Key || after.ProjectDir != before.ProjectDir {
		t.Errorf("re-registration changed the mapping: key %q -> %q, dir %q -> %q", before.Key, after.Key, before.ProjectDir, after.ProjectDir)
	}
	reg, err := loadRegistry(storeHome)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Projects[before.Key]; !ok {
		t.Errorf("re-registration did not restore the entry:\n%+v", reg.Projects)
	}
}

// TestStoreUnregisterUnderProjectSelector removes an entry from anywhere,
// resolving the project from the registry alone, which is how a stale entry for
// a checkout that no longer exists is cleared.
func TestStoreUnregisterUnderProjectSelector(t *testing.T) {
	home := setStoreHome(t)
	registerStoreProject(t, home, "github.com/travisennis/ahm", "ahm-0242d8b9", "")

	stdout, stderr, code := runCLIFromDir(t, t.TempDir(), "--project", "github.com/travisennis/ahm", "store", "unregister")
	if code != 0 {
		t.Fatalf("store unregister --project exited %d: %s", code, stderr)
	}
	assertContainsAll(t, stdout, "unregister github.com/travisennis/ahm")

	reg, err := loadRegistry(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Projects["github.com/travisennis/ahm"]; ok {
		t.Errorf("the registry still holds the selected project:\n%+v", reg.Projects)
	}
}

// TestStoreUnregisterSerializesWithAnotherStoreWrite pins the store-state lock
// on the new writer: an unregister held open must block a concurrent store
// write, so neither drops the other's change.
func TestStoreUnregisterSerializesWithAnotherStoreWrite(t *testing.T) {
	storeRoot := t.TempDir()
	removed := testStoreProjectPaths(t, storeRoot, "example.com/owner/first")
	added := testStoreProjectPaths(t, storeRoot, "example.com/owner/second")
	if err := recordStoreProject(removed); err != nil {
		t.Fatal(err)
	}

	interleaveStoreStateWriters(t, storeRegistryFileName,
		func() error {
			_, err := unregisterStoreProject(removed, "")
			return err
		},
		func() error { return recordStoreProject(added) },
	)

	reg, err := loadRegistry(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Projects[removed.Key]; ok {
		t.Errorf("the registry still holds the unregistered project %s:\n%+v", removed.Key, reg.Projects)
	}
	if _, ok := reg.Projects[added.Key]; !ok {
		t.Errorf("the concurrent write lost %s:\n%+v", added.Key, reg.Projects)
	}
}
