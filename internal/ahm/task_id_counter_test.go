package ahm

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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

// storeChildSuffixMark reads the child suffix mark the store persists for one
// parent.
func storeChildSuffixMark(t *testing.T, root string, parentID string) string {
	t.Helper()
	state, err := readProjectState(storePathsOf(t, root))
	if err != nil {
		t.Fatalf("reading the store state: %v", err)
	}
	return state.ChildSuffixMarks[parentID]
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

// TestHomeModeChildTaskIDsAreNotReissuedAfterTheChildRecordIsDeleted is the
// task's acceptance test: the top-level counter cannot cover a child letter,
// so the store's per-parent suffix mark is what keeps a deleted child's letter
// from returning to the pool.
func TestHomeModeChildTaskIDsAreNotReissuedAfterTheChildRecordIsDeleted(t *testing.T) {
	setStoreHome(t)
	root := initHomeModeRepository(t)
	if got := createTask(t, root, "Parent"); got != "001" {
		t.Fatalf("parent create = %q, want 001", got)
	}
	if got := createTask(t, root, "Child", "--parent", "001"); got != "001a" {
		t.Fatalf("child create = %q, want 001a", got)
	}
	record := storeTaskFile(t, root, "active", "001a")
	if err := os.Remove(record); err != nil {
		t.Fatal(err)
	}

	if got := createTask(t, root, "Second Child", "--parent", "001"); got != "001b" {
		t.Errorf("child create after the deletion = %q, want 001b: the store reissued a deleted child ID", got)
	}
	if got := storeChildSuffixMark(t, root, "001"); got != "b" {
		t.Errorf("persisted child suffix mark = %q, want \"b\"", got)
	}
	if _, err := os.Stat(record); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the deleted child record came back: %v", err)
	}
}

// TestHomeModeChildAllocationSkipsALetterBelowTheHighestUsed pins the high-water
// semantics: deleting a child whose letter sits below the highest used one does
// not return that letter, because the store cannot prove nothing referenced it.
func TestHomeModeChildAllocationSkipsALetterBelowTheHighestUsed(t *testing.T) {
	setStoreHome(t)
	root := initHomeModeRepository(t)
	createTask(t, root, "Parent")
	createTask(t, root, "Child A", "--parent", "001")
	createTask(t, root, "Child B", "--parent", "001")
	if err := os.Remove(storeTaskFile(t, root, "active", "001a")); err != nil {
		t.Fatal(err)
	}

	if got := createTask(t, root, "Child C", "--parent", "001"); got != "001c" {
		t.Errorf("child create after a middle deletion = %q, want 001c", got)
	}
}

// TestHomeModeChildSuffixMarkSelfHealsFromTheChildrenPresent covers a store
// whose state file holds no mark - one written before this change, or one an
// earlier command lost. Allocation records the high-water mark the children
// present imply, so a child deleted afterwards is still spent.
func TestHomeModeChildSuffixMarkSelfHealsFromTheChildrenPresent(t *testing.T) {
	setStoreHome(t)
	root := initHomeModeRepository(t)
	createTask(t, root, "Parent")
	createTask(t, root, "Child A", "--parent", "001")
	createTask(t, root, "Child B", "--parent", "001")

	store := storePathsOf(t, root)
	writeFile(t, store.statePath(), `{"version": 1, "next_id": 2}`)
	if got := createTask(t, root, "Child C", "--parent", "001"); got != "001c" {
		t.Errorf("child create with no mark = %q, want 001c", got)
	}
	if got := storeChildSuffixMark(t, root, "001"); got != "c" {
		t.Errorf("persisted child suffix mark = %q, want \"c\": the healed value was not recorded", got)
	}

	// A store with no state file at all heals the same way.
	if err := os.Remove(store.statePath()); err != nil {
		t.Fatal(err)
	}
	if got := createTask(t, root, "Child D", "--parent", "001"); got != "001d" {
		t.Errorf("child create with no state file = %q, want 001d", got)
	}
	if got := storeChildSuffixMark(t, root, "001"); got != "d" {
		t.Errorf("persisted child suffix mark = %q, want \"d\"", got)
	}
}

// TestHomeModeChildSuffixMarksArePerParent keeps the marks scoped: letters
// spent under one parent say nothing about another parent's letters.
func TestHomeModeChildSuffixMarksArePerParent(t *testing.T) {
	setStoreHome(t)
	root := initHomeModeRepository(t)
	createTask(t, root, "Parent One")
	createTask(t, root, "Parent Two")
	createTask(t, root, "Child A", "--parent", "001")
	createTask(t, root, "Child B", "--parent", "001")

	if got := createTask(t, root, "Child", "--parent", "002"); got != "002a" {
		t.Errorf("first child under a second parent = %q, want 002a", got)
	}
	if got := storeChildSuffixMark(t, root, "002"); got != "a" {
		t.Errorf("persisted child suffix mark for 002 = %q, want \"a\"", got)
	}
}

// TestHomeModeChildAllocationKeepsTheTwentySixLimit pins the unchanged limit
// and error message, now that the mark decides allocation.
func TestHomeModeChildAllocationKeepsTheTwentySixLimit(t *testing.T) {
	setStoreHome(t)
	root := initHomeModeRepository(t)
	createTask(t, root, "Parent")
	for ch := 'a'; ch <= 'z'; ch++ {
		if got := createTask(t, root, "Child", "--parent", "001"); got != "001"+string(ch) {
			t.Fatalf("child create = %q, want 001%c", got, ch)
		}
	}
	stdout, stderr, code := runCLI(t, "--root", root, "task", "create", "Overflow", "--parent", "001")
	if code != 1 {
		t.Fatalf("27th child create: stdout=%q stderr=%q code=%d, want 1", stdout, stderr, code)
	}
	assertContainsAll(t, stderr, `all 26 child task slots used for parent "001"`)
	if got := storeChildSuffixMark(t, root, "001"); got != "z" {
		t.Errorf("persisted child suffix mark = %q, want \"z\"", got)
	}
}

// TestHomeModeDryRunChildCreateDoesNotConsumeALetter keeps the dry-run
// contract for child allocation: the preview names the child ID a real create
// would allocate and leaves the mark, and therefore the next real allocation,
// untouched.
func TestHomeModeDryRunChildCreateDoesNotConsumeALetter(t *testing.T) {
	home := setStoreHome(t)
	root := initHomeModeRepository(t)
	createTask(t, root, "Parent")
	createTask(t, root, "Child", "--parent", "001")

	before := snapshotTree(t, home)
	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "create", "Preview", "--parent", "001")
	if code != 0 {
		t.Fatalf("dry-run child create: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout, "id: 001b", "store:tasks/active/001b.md")
	assertTreeUnchanged(t, home, before)

	if got := createTask(t, root, "Real", "--parent", "001"); got != "001b" {
		t.Errorf("child create after the preview = %q, want 001b", got)
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

// TestInitSeedsChildSuffixMarksFromTheChildrenPresent covers a store whose
// state file holds no mark - one written before this change, or one an earlier
// command lost. init records the high-water mark the children present imply, so
// a child deleted before the next allocation is still spent.
func TestInitSeedsChildSuffixMarksFromTheChildrenPresent(t *testing.T) {
	setStoreHome(t)
	root := initHomeModeRepository(t)
	createTask(t, root, "Parent")
	createTask(t, root, "Child A", "--parent", "001")
	createTask(t, root, "Child B", "--parent", "001")

	store := storePathsOf(t, root)
	if err := os.Remove(store.statePath()); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("second init: %s", stderr)
	}
	if got := storeChildSuffixMark(t, root, "001"); got != "b" {
		t.Fatalf("init recorded child suffix mark %q, want \"b\"", got)
	}

	if err := os.Remove(storeTaskFile(t, root, "active", "001b")); err != nil {
		t.Fatal(err)
	}
	if got := createTask(t, root, "Child C", "--parent", "001"); got != "001c" {
		t.Errorf("child create = %q, want 001c: init did not seed the mark", got)
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
	path, ok := paths.storeStatePath()
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

// TestChildSuffixMarkOnlyMovesUp pins the property the child mark exists for:
// no command, and no stale observation from another command, can roll it back.
func TestChildSuffixMarkOnlyMovesUp(t *testing.T) {
	paths := workflowPathsForStore(t.TempDir(), testStorePaths(t))
	path, ok := paths.storeStatePath()
	if !ok {
		t.Fatal("home mode reported no store state path")
	}

	if err := writeChildSuffixMark(paths, "001", "b"); err != nil {
		t.Fatal(err)
	}
	if got, err := readChildSuffixMark(paths, "001"); err != nil || got != "b" {
		t.Fatalf("readChildSuffixMark = %q, %v, want \"b\"", got, err)
	}
	written := mustRead(t, path)

	// A repeated observation writes nothing, and a lower one never lowers the
	// stored value.
	if err := writeChildSuffixMark(paths, "001", "b"); err != nil {
		t.Fatal(err)
	}
	if err := writeChildSuffixMark(paths, "001", "a"); err != nil {
		t.Fatal(err)
	}
	if got := mustRead(t, path); got != written {
		t.Errorf("a repeated or lower observation rewrote the mark:\ngot:\n%s\nwant:\n%s", got, written)
	}

	if err := writeChildSuffixMark(paths, "001", "f"); err != nil {
		t.Fatal(err)
	}
	// The state observation another command records carries the mark it read,
	// so a stale write must not undo a later one.
	if err := writeProjectState(paths.store, projectState{Version: storeFormatVersion, ChildSuffixMarks: map[string]string{"001": "c"}}); err != nil {
		t.Fatal(err)
	}
	if got, err := readChildSuffixMark(paths, "001"); err != nil || got != "f" {
		t.Errorf("readChildSuffixMark after a stale state write = %q, %v, want \"f\"", got, err)
	}
}

// TestPersistTaskIDAllocationRaisesTheMatchingMark records the shape decision:
// a top-level allocation advances next_id, and a child allocation advances its
// parent's suffix mark without touching the counter. The mark is the store's
// only evidence that the letter was spent.
func TestPersistTaskIDAllocationRaisesTheMatchingMark(t *testing.T) {
	paths := workflowPathsForStore(t.TempDir(), testStorePaths(t))
	if err := writeTaskIDCounter(paths, 4); err != nil {
		t.Fatal(err)
	}
	if err := persistTaskIDAllocation(paths, "267a"); err != nil {
		t.Fatal(err)
	}
	if got, err := readTaskIDCounter(paths); err != nil || got != 4 {
		t.Errorf("readTaskIDCounter after a child allocation = %d, %v, want 4", got, err)
	}
	if got, err := readChildSuffixMark(paths, "267"); err != nil || got != "a" {
		t.Errorf("readChildSuffixMark after a child allocation = %q, %v, want \"a\"", got, err)
	}
	if err := persistTaskIDAllocation(paths, "267"); err != nil {
		t.Fatal(err)
	}
	if got, err := readTaskIDCounter(paths); err != nil || got != 268 {
		t.Errorf("readTaskIDCounter after a top-level allocation = %d, %v, want 268", got, err)
	}
	if got, err := readChildSuffixMark(paths, "267"); err != nil || got != "a" {
		t.Errorf("readChildSuffixMark after a top-level allocation = %q, %v, want \"a\"", got, err)
	}
}

// TestStoreStatePathIsInsideAnOwnedRoot ties the state write to the
// containment rule: the store's project directory is an owned root, so
// writeOwned accepts the state file it lives in.
func TestStoreStatePathIsInsideAnOwnedRoot(t *testing.T) {
	paths := workflowPathsForStore(t.TempDir(), testStorePaths(t))
	path, ok := paths.storeStatePath()
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

// TestProjectModeWritesNoTaskIDMarks keeps project mode unchanged: Git
// history there proves which IDs were spent, and no state file exists to
// write.
func TestProjectModeWritesNoTaskIDMarks(t *testing.T) {
	root := t.TempDir()
	paths := workflowPathsFor(root)
	if _, ok := paths.storeStatePath(); ok {
		t.Error("project mode reported a store state path")
	}
	if err := writeTaskIDCounter(paths, 5); err != nil {
		t.Fatal(err)
	}
	if err := writeChildSuffixMark(paths, "001", "c"); err != nil {
		t.Fatal(err)
	}
	if err := persistTaskIDAllocation(paths, "007"); err != nil {
		t.Fatal(err)
	}
	if err := persistTaskIDAllocation(paths, "007a"); err != nil {
		t.Fatal(err)
	}
	if got, err := readTaskIDCounter(paths); err != nil || got != 0 {
		t.Errorf("readTaskIDCounter = %d, %v, want 0", got, err)
	}
	if got, err := readChildSuffixMark(paths, "007"); err != nil || got != "" {
		t.Errorf("readChildSuffixMark = %q, %v, want \"\"", got, err)
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

// TestHomeModeChildCreateFailsOnAnUnreadableChildSuffixMark keeps the mark a
// hard dependency, like the counter: a state file this ahm cannot read is an
// error rather than a silent reissue of a letter it cannot see.
func TestHomeModeChildCreateFailsOnAnUnreadableChildSuffixMark(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state string
		want  string
	}{
		{name: "newer store format", state: `{"version": 99}`, want: "version 99"},
		{name: "corrupt state", state: "not json", want: "corrupt store state"},
		{name: "invalid mark", state: `{"version": 1, "next_id": 2, "child_suffix_marks": {"001": "ab"}}`, want: `child suffix mark for parent 001 is "ab"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setStoreHome(t)
			root := initHomeModeRepository(t)
			createTask(t, root, "Parent")
			writeFile(t, storePathsOf(t, root).statePath(), tc.state)

			stdout, stderr, code := runCLI(t, "--root", root, "task", "create", "Child", "--parent", "001")
			if code != 1 {
				t.Fatalf("child create with an unreadable mark: stdout=%q stderr=%q code=%d, want 1", stdout, stderr, code)
			}
			assertContainsAll(t, stderr, tc.want)
		})
	}
}

// TestStoreStateLockKeepsStorePathFromLoweringTheCounter is the interleaving the
// store-state lock closes: a `store path` observation computes its state bytes
// from a counter that a concurrent `task create` then raises. The observation
// is held at the point where its bytes are about to become stale, so the
// interleaving is deterministic: without the lock the observation writes its
// stale counter over the raise, and with it the two writers serialize.
func TestStoreStateLockKeepsStorePathFromLoweringTheCounter(t *testing.T) {
	setStoreHome(t)
	root := initHomeModeRepository(t)

	var observeOut strings.Builder
	observer := app{opts: options{root: root}, out: &observeOut}
	var createOut strings.Builder
	creator := app{opts: options{root: root}, out: &createOut}
	interleaveStoreStateWriters(t, storeStateFileName,
		observer.storePath,
		func() error {
			return creator.taskCreateParsed(taskCreateArgs{
				title:    "Created While The Store Was Observed",
				priority: "P2",
				effort:   "S",
				labels:   "type:task, area:unknown",
				status:   "Open",
			})
		},
	)

	if got := storeTaskIDCounter(t, root); got != 2 {
		t.Errorf("the persisted next_id = %d after the interleaving, want 2", got)
	}
	if got := strings.TrimSpace(createOut.String()); got != "001" {
		t.Errorf("task create allocated %q, want 001", got)
	}
	if _, err := os.Stat(filepath.Join(storePathsOf(t, root).recordsDir(), "active", "001.md")); err != nil {
		t.Errorf("the created record is not in the store: %v", err)
	}
}
