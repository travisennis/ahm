package ahm

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
)

// initHomeModeRepository initializes a scratch repository whose records live in
// the home store and returns the project root.
func initHomeModeRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeHomeModeConfig(t, root)
	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("init: %s", stderr)
	}
	return root
}

// createTask runs `ahm task create` and returns the allocated ID.
func createTask(t *testing.T, root string, args ...string) string {
	t.Helper()
	stdout, stderr, code := runCLI(t, append([]string{"--root", root, "task", "create"}, args...)...)
	if code != 0 {
		t.Fatalf("task create %v: stdout=%q stderr=%q code=%d", args, stdout, stderr, code)
	}
	return strings.TrimSpace(stdout)
}

// storeTaskIDCounter reads the counter the store persists for the project.
func storeTaskIDCounter(t *testing.T, root string) int {
	t.Helper()
	state, err := readProjectState(storePathsOf(t, root))
	if err != nil {
		t.Fatalf("reading the store state: %v", err)
	}
	return state.NextID
}

// storePathsOf resolves the store location of a test repository.
func storePathsOf(t *testing.T, root string) storePaths {
	t.Helper()
	store, err := resolveStore(root)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// TestHomeModeTaskIDsAreNotReissuedAfterTheNewestRecordIsDeleted is the
// milestone's acceptance test. A store has no Git history to prove that a
// deleted ID was used, so the persisted counter is what keeps the number from
// returning to the pool.
func TestHomeModeTaskIDsAreNotReissuedAfterTheNewestRecordIsDeleted(t *testing.T) {
	setStoreHome(t)
	root := initHomeModeRepository(t)

	if got := createTask(t, root, "First"); got != "001" {
		t.Fatalf("first create = %q, want 001", got)
	}
	record := storeTaskFile(t, root, "active", "001")
	if err := os.Remove(record); err != nil {
		t.Fatal(err)
	}

	if got := createTask(t, root, "Second"); got != "002" {
		t.Errorf("create after the deletion = %q, want 002: the store reissued a deleted ID", got)
	}
	if got := storeTaskIDCounter(t, root); got != 3 {
		t.Errorf("persisted counter = %d, want 3", got)
	}
	if _, err := os.Stat(record); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the deleted record came back: %v", err)
	}
}

// TestHomeModeDryRunTaskCreateDoesNotConsumeAnID keeps the dry-run contract:
// the preview names the ID a real create would allocate and leaves the counter,
// and therefore the next real allocation, untouched.
func TestHomeModeDryRunTaskCreateDoesNotConsumeAnID(t *testing.T) {
	home := setStoreHome(t)
	root := initHomeModeRepository(t)
	if got := createTask(t, root, "First"); got != "001" {
		t.Fatalf("first create = %q, want 001", got)
	}

	before := snapshotTree(t, home)
	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "create", "Preview")
	if code != 0 {
		t.Fatalf("dry-run create: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout, "id: 002", "store:tasks/active/002.md")
	if _, err := os.Stat(storeTaskFile(t, root, "active", "002")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the preview created a record: %v", err)
	}
	// The counter file survives the preview byte-for-byte and unrewritten.
	assertTreeUnchanged(t, home, before)

	if got := createTask(t, root, "Real"); got != "002" {
		t.Errorf("create after the preview = %q, want 002", got)
	}
}

// TestParentIDIsNotReissuedWhileItsChildRemains covers the case the numeric scan
// cannot see: every remaining record is a child, so its number is ignored by the
// scan. Only the counter remembers that the parent's number was spent.
func TestParentIDIsNotReissuedWhileItsChildRemains(t *testing.T) {
	setStoreHome(t)
	root := initHomeModeRepository(t)
	if got := createTask(t, root, "Parent"); got != "001" {
		t.Fatalf("parent create = %q, want 001", got)
	}
	if got := createTask(t, root, "Child", "--parent", "001"); got != "001a" {
		t.Fatalf("child create = %q, want 001a", got)
	}
	if err := os.Remove(storeTaskFile(t, root, "active", "001")); err != nil {
		t.Fatal(err)
	}

	if got := createTask(t, root, "Next"); got != "002" {
		t.Errorf("create = %q, want 002: the number of the deleted parent was reissued", got)
	}
}

// TestInitSeedsTheTaskIDCounterFromTheRecordsPresent covers a store whose state
// file holds no counter - one written before this milestone, or one an earlier
// command lost. init records the high-water mark the records imply, so the
// number of a record that is deleted afterwards is still spent.
func TestInitSeedsTheTaskIDCounterFromTheRecordsPresent(t *testing.T) {
	setStoreHome(t)
	root := initHomeModeRepository(t)
	createTask(t, root, "First")
	createTask(t, root, "Second")

	store := storePathsOf(t, root)
	if err := os.Remove(store.statePath()); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("second init: %s", stderr)
	}
	if got := storeTaskIDCounter(t, root); got != 3 {
		t.Fatalf("init recorded counter %d, want 3", got)
	}

	if err := os.Remove(storeTaskFile(t, root, "active", "002")); err != nil {
		t.Fatal(err)
	}
	if got := createTask(t, root, "Third"); got != "003" {
		t.Errorf("create = %q, want 003: init did not seed the counter", got)
	}
}

// TestInitDryRunDoesNotSeedTheTaskIDCounter keeps --dry-run from writing the
// store, including the counter.
func TestInitDryRunDoesNotSeedTheTaskIDCounter(t *testing.T) {
	home := setStoreHome(t)
	root := t.TempDir()
	writeHomeModeConfig(t, root)
	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("init: %s", stderr)
	}
	if err := os.Remove(storePathsOf(t, root).statePath()); err != nil {
		t.Fatal(err)
	}

	before := snapshotTree(t, home)
	if _, stderr, code := runCLI(t, "--root", root, "--dry-run", "init"); code != 0 {
		t.Fatalf("dry-run init: %s", stderr)
	}
	assertTreeUnchanged(t, home, before)
}

// TestTaskIDCounterOnlyMovesUp pins the property the counter exists for: no
// command, and no stale observation from another command, can roll it back.
func TestTaskIDCounterOnlyMovesUp(t *testing.T) {
	paths := workflowPathsForStore(t.TempDir(), testStorePaths(t))
	path, ok := paths.taskIDCounterPath()
	if !ok {
		t.Fatal("home mode reported no counter path")
	}

	if err := writeTaskIDCounter(paths, 2); err != nil {
		t.Fatal(err)
	}
	if got, err := readTaskIDCounter(paths); err != nil || got != 2 {
		t.Fatalf("readTaskIDCounter = %d, %v, want 2", got, err)
	}
	written := mustRead(t, path)

	// A repeated observation writes nothing, and a lower one never lowers the
	// stored value.
	if err := writeTaskIDCounter(paths, 2); err != nil {
		t.Fatal(err)
	}
	if err := writeTaskIDCounter(paths, 1); err != nil {
		t.Fatal(err)
	}
	if got := mustRead(t, path); got != written {
		t.Errorf("a repeated or lower observation rewrote the counter:\ngot:\n%s\nwant:\n%s", got, written)
	}

	if err := writeTaskIDCounter(paths, 9); err != nil {
		t.Fatal(err)
	}
	// The state observation another command records carries the counter it read,
	// so a stale write must not undo a later one.
	if err := writeProjectState(paths.store, projectState{Version: storeFormatVersion, NextID: 4}); err != nil {
		t.Fatal(err)
	}
	if got, err := readTaskIDCounter(paths); err != nil || got != 9 {
		t.Errorf("readTaskIDCounter after a stale state write = %d, %v, want 9", got, err)
	}
}

// TestPersistTaskIDCounterIgnoresChildIDs records the decision that the counter
// tracks promised numbers, not the letters under one of them: a child carries
// its parent's number, and the parent record is present by construction.
func TestPersistTaskIDCounterIgnoresChildIDs(t *testing.T) {
	paths := workflowPathsForStore(t.TempDir(), testStorePaths(t))
	if err := writeTaskIDCounter(paths, 4); err != nil {
		t.Fatal(err)
	}
	if err := persistTaskIDCounter(paths, "267a"); err != nil {
		t.Fatal(err)
	}
	if got, err := readTaskIDCounter(paths); err != nil || got != 4 {
		t.Errorf("readTaskIDCounter after a child allocation = %d, %v, want 4", got, err)
	}
	if err := persistTaskIDCounter(paths, "267"); err != nil {
		t.Fatal(err)
	}
	if got, err := readTaskIDCounter(paths); err != nil || got != 268 {
		t.Errorf("readTaskIDCounter after a top-level allocation = %d, %v, want 268", got, err)
	}
}

// TestTaskIDCounterPathIsInsideAnOwnedRoot ties the counter write to the
// containment rule: the store's project directory is an owned root, so
// writeOwned accepts the state file it lives in.
func TestTaskIDCounterPathIsInsideAnOwnedRoot(t *testing.T) {
	paths := workflowPathsForStore(t.TempDir(), testStorePaths(t))
	path, ok := paths.taskIDCounterPath()
	if !ok {
		t.Fatal("home mode reported no counter path")
	}
	owned := false
	for _, root := range paths.ownedRoots() {
		if pathWithin(root, path) {
			owned = true
		}
	}
	if !owned {
		t.Errorf("the counter %s is outside the owned roots %v, so writeOwned would refuse it", path, paths.ownedRoots())
	}
}

// TestProjectModeWritesNoTaskIDCounter keeps project mode unchanged: Git
// history there proves which IDs were spent, and no counter file exists to
// write.
func TestProjectModeWritesNoTaskIDCounter(t *testing.T) {
	root := t.TempDir()
	paths := workflowPathsFor(root)
	if _, ok := paths.taskIDCounterPath(); ok {
		t.Error("project mode reported a counter path")
	}
	if err := writeTaskIDCounter(paths, 5); err != nil {
		t.Fatal(err)
	}
	if err := persistTaskIDCounter(paths, "007"); err != nil {
		t.Fatal(err)
	}
	if got, err := readTaskIDCounter(paths); err != nil || got != 0 {
		t.Errorf("readTaskIDCounter = %d, %v, want 0", got, err)
	}
	if got := relativeTreePaths(t, root); len(got) != 0 {
		t.Errorf("project mode wrote %v", got)
	}
}

// TestTaskIDAllocationHealsFromTheRecordsPresent covers a counter that lags the
// records - a store written before the counter, or one whose state file was
// restored from a copy. Allocation takes the records into account, so a number
// already in use is never handed out, and it persists the healed value.
func TestTaskIDAllocationHealsFromTheRecordsPresent(t *testing.T) {
	setStoreHome(t)
	root := initHomeModeRepository(t)
	createTask(t, root, "First")
	createTask(t, root, "Second")

	store := storePathsOf(t, root)
	writeFile(t, store.statePath(), `{"version": 1, "next_id": 1}`)
	if got := createTask(t, root, "Third"); got != "003" {
		t.Errorf("create with a lagging counter = %q, want 003", got)
	}
	if err := os.Remove(store.statePath()); err != nil {
		t.Fatal(err)
	}
	if got := createTask(t, root, "Fourth"); got != "004" {
		t.Errorf("create with no counter = %q, want 004", got)
	}
	if got := storeTaskIDCounter(t, root); got != 5 {
		t.Errorf("persisted counter = %d, want 5: the healed value was not recorded", got)
	}
}

// TestInitDoesNotLowerTheTaskIDCounter covers an init that finds a counter ahead
// of the records: raising it is the only direction init may move it.
func TestInitDoesNotLowerTheTaskIDCounter(t *testing.T) {
	setStoreHome(t)
	root := initHomeModeRepository(t)
	createTask(t, root, "First")
	createTask(t, root, "Second")

	store := storePathsOf(t, root)
	writeFile(t, store.statePath(), `{"version": 1, "next_id": 9}`)
	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("second init: %s", stderr)
	}
	if got := storeTaskIDCounter(t, root); got != 9 {
		t.Fatalf("init lowered the counter to %d, want 9", got)
	}
	if got := createTask(t, root, "Third"); got != "009" {
		t.Errorf("create = %q, want 009", got)
	}
}

// TestHomeModeTaskCreateFailsOnAnUnreadableTaskIDCounter keeps the counter a
// hard dependency: a state file this ahm cannot read is an error rather than a
// silent reissue of a number it cannot see.
func TestHomeModeTaskCreateFailsOnAnUnreadableTaskIDCounter(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state string
		want  string
	}{
		{name: "newer store format", state: `{"version": 99}`, want: "version 99"},
		{name: "corrupt state", state: "not json", want: "corrupt store state"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setStoreHome(t)
			root := initHomeModeRepository(t)
			createTask(t, root, "First")
			writeFile(t, storePathsOf(t, root).statePath(), tc.state)

			stdout, stderr, code := runCLI(t, "--root", root, "task", "create", "Second")
			if code != 1 {
				t.Fatalf("create with an unreadable counter: stdout=%q stderr=%q code=%d, want 1", stdout, stderr, code)
			}
			assertContainsAll(t, stderr, tc.want)
		})
	}
}
