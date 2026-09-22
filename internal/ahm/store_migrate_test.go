package ahm

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// commitEverything commits the working tree, so a migration's record deletions
// have committed content behind them, which is what the move protects.
func commitEverything(t *testing.T, root string, message string) {
	t.Helper()
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", message)
}

// migratedProject returns a scratch Git repository with the project-mode
// workflow installed and one committed task record.
func migratedProject(t *testing.T) string {
	t.Helper()
	root := newGitRepo(t)
	writeAHMConfig(t, root)
	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("init: %s", stderr)
	}
	if _, stderr, code := runCLI(t, "--root", root, "task", "create", "First"); code != 0 {
		t.Fatalf("task create: %s", stderr)
	}
	commitEverything(t, root, "install workflow")
	return root
}

func TestStoreMigrateRequiresTheDestinationFlag(t *testing.T) {
	root := t.TempDir()
	writeAHMConfig(t, root)

	_, stderr, code := runCLI(t, "--root", root, "store", "migrate")
	if code != 2 {
		t.Fatalf("missing --to: code = %d, want 2 (stderr=%q)", code, stderr)
	}
	assertContainsAll(t, stderr, "store migrate requires --to", "ahm store migrate --to home|project")

	_, stderr, code = runCLI(t, "--root", root, "store", "migrate", "--to", "elsewhere")
	if code != 2 {
		t.Fatalf("unknown --to: code = %d, want 2 (stderr=%q)", code, stderr)
	}
	assertContainsAll(t, stderr, `unknown --to value "elsewhere"`, "valid: home, project")

	_, stderr, code = runCLI(t, "--root", root, "store", "migrate", "--to", "home", "extra")
	if code != 2 {
		t.Fatalf("extra argument: code = %d, want 2 (stderr=%q)", code, stderr)
	}
}

// TestStoreMigrateResolvesItsOwedWritesBeforeMovingRecords covers the one
// precondition failure that must not cost the records their home: a
// configuration the move cannot read is reported while every record is still in
// the project, so the command leaves no half-move for a re-run to be unable to
// finish.
func TestStoreMigrateResolvesItsOwedWritesBeforeMovingRecords(t *testing.T) {
	home := setStoreHome(t)
	root := migratedProject(t)
	writeFile(t, filepath.Join(root, ".ahm", "config.json"), "{oops")
	commitEverything(t, root, "corrupt config")

	stdout, stderr, code := runCLI(t, "--root", root, "store", "migrate", "--to", "home")
	if code != 1 {
		t.Fatalf("migrate with a corrupt config: code = %d, want 1 (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	assertContainsAll(t, stderr, "reading workflow metadata .ahm/config.json")
	// The record never left the project, and the move created no store to hold a
	// destination copy of it.
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "title: First")
	assertStoreAbsent(t, home)
}

// TestStoreMigrateDryRunPreviewsWithoutWriting is the milestone's central
// no-write guarantee: the preview names every move, write, removal, and commit,
// and leaves both the project and the (uncreated) store untouched.
func TestStoreMigrateDryRunPreviewsWithoutWriting(t *testing.T) {
	home := setStoreHome(t)
	root := migratedProject(t)
	before := snapshotTree(t, filepath.Join(root, ".ahm"))
	adrBefore := snapshotTree(t, filepath.Join(root, "docs"))

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "store", "migrate", "--to", "home")
	if code != 0 {
		t.Fatalf("dry run: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout,
		"would move 1 record(s) to the home store",
		"  .ahm/tasks/active/001.md -> store:tasks/active/001.md",
		"would write .ahm/config.json",
		"would write .ahm/.gitignore",
		"would write store:.gitignore",
		"would write store:tasks/index.md",
		"would remove .ahm/tasks/index.md",
		"deletions to commit after the move:",
		"  .ahm/tasks/active/001.md",
	)
	assertTreeUnchanged(t, filepath.Join(root, ".ahm"), before)
	assertTreeUnchanged(t, filepath.Join(root, "docs"), adrBefore)
	assertStoreAbsent(t, home)
	if status := git(t, root, "status", "--short"); status != "" {
		t.Errorf("the dry run changed the working tree:\n%s", status)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "--json", "--dry-run", "store", "migrate", "--to", "home")
	if code != 0 {
		t.Fatalf("json dry run: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout,
		`"dry_run": true`,
		`"to": "home"`,
		`"from": ".ahm/tasks/active/001.md"`,
		`"to": "store:tasks/active/001.md"`,
		`"deletions": [`,
	)
	assertNotContains(t, stdout, home)
	assertTreeUnchanged(t, filepath.Join(root, ".ahm"), before)
	assertStoreAbsent(t, home)
}

// assertStoreAbsent fails when a command created the store root's contents.
func assertStoreAbsent(t *testing.T, home string) {
	t.Helper()
	for _, name := range []string{storeProjectsDirName, storeRegistryFileName} {
		if _, err := os.Stat(filepath.Join(home, name)); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the store gained %s: %v", name, err)
		}
	}
}

// TestStoreMigrateToHomeMovesTheRecords covers the real move: the records, their
// indexes, the lock, and the managed .gitignore follow the records into the
// store; the committed configuration names the new location; the project keeps
// the ADR index and prints the deletions it owes Git.
func TestStoreMigrateToHomeMovesTheRecords(t *testing.T) {
	home := setStoreHome(t)
	root := migratedProject(t)
	if _, stderr, code := runCLI(t, "--root", root, "task", "create", "Second"); code != 0 {
		t.Fatalf("task create: %s", stderr)
	}
	if _, stderr, code := runCLI(t, "--root", root, "task", "complete", "002", "--force"); code != 0 {
		t.Fatalf("task complete: %s", stderr)
	}
	commitEverything(t, root, "second task")

	stdout, stderr, code := runCLI(t, "--root", root, "store", "migrate", "--to", "home")
	if code != 0 {
		t.Fatalf("migrate: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout,
		"move 2 record(s) to the home store",
		"  .ahm/tasks/active/001.md -> store:tasks/active/001.md",
		"  .ahm/tasks/completed/002.md -> store:tasks/completed/002.md",
		"write .ahm/config.json",
		"write .ahm/.gitignore",
		"deletions to commit:",
		"  .ahm/tasks/active/001.md",
		"  .ahm/tasks/completed/002.md",
	)
	assertNotContains(t, stdout, home)

	// The records and their generated indexes live in the store, and the
	// project's records tree is gone.
	assertFileContainsAll(t, storeTaskFile(t, root, "active", "001"), "title: First")
	assertFileContainsAll(t, storeTaskFile(t, root, "completed", "002"), "title: Second")
	assertNoProjectRecords(t, root)
	// .ahm/ no longer holds the record tree, and the managed .gitignore keeps
	// only the temp-file pattern, because the atomic config write is the one
	// write ahm still makes there.
	if got := relativeTreePaths(t, filepath.Join(root, ".ahm")); slices.Contains(got, "tasks") {
		t.Errorf(".ahm still holds the records tree: %v", got)
	}
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "config.json"), `"tasks_location": "home"`)
	projectGitignore := mustRead(t, filepath.Join(root, ".ahm", ".gitignore"))
	assertContainsAll(t, projectGitignore, "*.tmp", "the task records live in the user-level store")
	assertNotContains(t, projectGitignore, ".lock/", "tasks/index.md")

	// The ADR index stays committed in the project, and the task indexes moved
	// into the store.
	if _, err := os.Stat(filepath.Join(root, "docs", "adr", "index.md")); err != nil {
		t.Errorf("the move removed the committed ADR index: %v", err)
	}
	store, err := resolveStore(root)
	if err != nil {
		t.Fatal(err)
	}
	assertFileContainsAll(t, filepath.Join(store.recordsDir(), "index.md"), "First", "Second")
	storeGitignore := filepath.Join(store.ProjectDir, gitignoreFileName)
	assertFileContainsAll(t, storeGitignore, "tasks/index.md", ".lock/", "*.tmp", storeStateFileName)
	// The registry records where the records came from, so a store listing says
	// which layout a project's records were moved out of.
	assertFileContainsAll(t, filepath.Join(home, storeRegistryFileName), `"migrated_from": "project"`)

	// The store's lock now lives beside the store's records: a home-mode
	// mutation serializes there, not in the project.
	if _, stderr, code := runCLI(t, "--root", root, "task", "create", "Third"); code != 0 {
		t.Fatalf("task create after the move: %s", stderr)
	}
	if _, err := os.Stat(filepath.Join(store.ProjectDir, lockDirName)); err != nil {
		t.Errorf("the moved records do not use the store's lock: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a home-mode mutation wrote into the project: %v", err)
	}

	// The deletions are left in the working tree for the user to commit.
	status := git(t, root, "status", "--short")
	assertContainsAll(t, status,
		" D .ahm/tasks/active/001.md",
		" D .ahm/tasks/completed/002.md",
		" M .ahm/config.json",
	)

	// The migrated layout validates clean, including the drift finding the
	// milestone's status and doctor output must surface.
	stdout, stderr, code = runCLI(t, "--root", root, "--json", "doctor")
	if code != 0 {
		t.Fatalf("doctor after the move: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout, `"ok": true`)
	assertNotContains(t, stdout, "task_records_in_project")

	// A repeated command reports no work and rewrites nothing on either side.
	before := snapshotTree(t, filepath.Join(root, ".ahm"))
	storeBefore := snapshotTree(t, home)
	stdout, stderr, code = runCLI(t, "--root", root, "store", "migrate", "--to", "home")
	if code != 0 {
		t.Fatalf("repeated migrate: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout, "no work: task records already live in the home store")
	assertTreeUnchanged(t, filepath.Join(root, ".ahm"), before)
	assertTreeUnchanged(t, home, storeBefore)
}

// assertNoProjectRecords fails when the project still holds a task records tree.
func assertNoProjectRecords(t *testing.T, root string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the project still holds its records tree: %v", err)
	}
}

// TestStoreMigrateRoundTripRestoresTheProjectLayout moves the records into the
// store and back, and requires the original layout, the committed
// configuration, the managed .gitignore, and the store's ID counter to survive
// the round trip.
func TestStoreMigrateRoundTripRestoresTheProjectLayout(t *testing.T) {
	home := setStoreHome(t)
	root := migratedProject(t)
	if _, stderr, code := runCLI(t, "--root", root, "task", "create", "Second"); code != 0 {
		t.Fatalf("task create: %s", stderr)
	}
	commitEverything(t, root, "second task")
	if _, stderr, code := runCLI(t, "--root", root, "store", "migrate", "--to", "home"); code != 0 {
		t.Fatalf("migrate to home: %s", stderr)
	}
	store, err := resolveStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if state, err := readProjectState(store); err != nil || state.NextID != 3 {
		t.Errorf("store counter after the move = %+v (err %v), want next_id 3", state, err)
	}

	stdout, stderr, code := runCLI(t, "--root", root, "store", "migrate", "--to", "project")
	if code != 0 {
		t.Fatalf("migrate to project: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout,
		"move 2 record(s) to the project",
		"  store:tasks/active/001.md -> .ahm/tasks/active/001.md",
		"  store:tasks/active/002.md -> .ahm/tasks/active/002.md",
		"write .ahm/.gitignore",
		"remove store:tasks/index.md",
		"additions to commit:",
		"  .ahm/tasks/active/001.md",
	)
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "title: First")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "title: Second")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "config.json"), `"tasks_location": "project"`)
	assertFileContainsAll(t, filepath.Join(root, ".ahm", ".gitignore"),
		"tasks/index.md", "tasks/*/index.md", ".lock/", "*.tmp")
	assertFileContainsAll(t, filepath.Join(home, storeRegistryFileName), `"migrated_from": "home"`)
	if _, err := os.Stat(store.recordsDir()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the move back left the store's records directory behind: %v", err)
	}
	// The store keeps its state, so a later move into it cannot reissue an ID.
	if state, err := readProjectState(store); err != nil || state.NextID != 3 {
		t.Errorf("store counter after the move back = %+v (err %v), want next_id 3", state, err)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "store", "migrate", "--to", "project")
	if code != 0 {
		t.Fatalf("repeated migrate: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout, "no work: task records already live in the project")

	// The project layout works again: a new task lands in the project.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Third")
	if code != 0 || strings.TrimSpace(stdout) != "003" {
		t.Fatalf("task create after the round trip: stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "active", "003.md")); err != nil {
		t.Errorf("the project no longer holds new records: %v", err)
	}
}

// TestStoreMigrateSeedsTheTaskIDCounter covers the requirement that a migration
// initializes the store's counter from the records present: a record deleted by
// hand from the store after the move must never have its number reissued.
func TestStoreMigrateSeedsTheTaskIDCounter(t *testing.T) {
	setStoreHome(t)
	root := migratedProject(t)
	if _, stderr, code := runCLI(t, "--root", root, "task", "create", "Second"); code != 0 {
		t.Fatalf("task create: %s", stderr)
	}
	commitEverything(t, root, "second task")
	if _, stderr, code := runCLI(t, "--root", root, "store", "migrate", "--to", "home"); code != 0 {
		t.Fatalf("migrate: %s", stderr)
	}

	// Deleting the newest record by hand is the case the counter exists for: in
	// the store no Git history proves the ID was used.
	if err := os.Remove(storeTaskFile(t, root, "active", "002")); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runCLI(t, "--root", root, "task", "create", "Third")
	if code != 0 {
		t.Fatalf("task create: stdout=%q stderr=%q", stdout, stderr)
	}
	if got := strings.TrimSpace(stdout); got != "003" {
		t.Errorf("the store reissued an ID after the move: got %q, want 003", got)
	}
}

// TestStoreMigrateResumesAfterAnInterruptedMove interrupts a move after its
// first record and requires a re-run to finish it without rewriting the record
// that already arrived.
func TestStoreMigrateResumesAfterAnInterruptedMove(t *testing.T) {
	setStoreHome(t)
	root := migratedProject(t)
	if _, stderr, code := runCLI(t, "--root", root, "task", "create", "Second"); code != 0 {
		t.Fatalf("task create: %s", stderr)
	}
	commitEverything(t, root, "second task")
	store, err := resolveStore(root)
	if err != nil {
		t.Fatal(err)
	}

	interrupted := errors.New("interrupted")
	storeMigrateRecordHook = func(moved int) error {
		if moved == 1 {
			return interrupted
		}
		return nil
	}
	t.Cleanup(func() { storeMigrateRecordHook = nil })

	_, stderr, code := runCLI(t, "--root", root, "store", "migrate", "--to", "home")
	if code != 1 {
		t.Fatalf("interrupted migrate: code = %d, want 1 (stderr=%q)", code, stderr)
	}
	assertContainsAll(t, stderr, "interrupted")
	// The first record arrived; the second has not moved; and the configuration
	// still names the layout that held the records when the move started.
	assertFileContainsAll(t, storeTaskFile(t, root, "active", "001"), "title: First")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "title: Second")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "config.json"), `"files"`)
	assertNotContains(t, mustRead(t, filepath.Join(root, ".ahm", "config.json")), `"tasks_location"`)

	// The arrived record keeps its bytes and its modification time across the
	// re-run, which removes only its source.
	arrived := storeTaskFile(t, root, "active", "001")
	before, err := os.Stat(arrived)
	if err != nil {
		t.Fatal(err)
	}
	storeMigrateRecordHook = nil
	stdout, stderr, code := runCLI(t, "--root", root, "store", "migrate", "--to", "home")
	if code != 0 {
		t.Fatalf("resumed migrate: stdout=%q stderr=%q", stdout, stderr)
	}
	// The record that already arrived is not moved again, and only the record
	// still in the project is left for the user to commit.
	assertContainsAll(t, stdout,
		"move 1 record(s) to the home store",
		"  .ahm/tasks/active/002.md -> store:tasks/active/002.md",
		"deletions to commit:",
		"  .ahm/tasks/active/002.md",
	)
	assertFileContainsAll(t, storeTaskFile(t, root, "active", "002"), "title: Second")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "config.json"), `"tasks_location": "home"`)
	after, err := os.Stat(arrived)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("the re-run rewrote a record that had already arrived")
	}
	if _, err := os.Stat(store.recordsDir()); err != nil {
		t.Errorf("the resumed move did not finish: %v", err)
	}
	assertNoProjectRecords(t, root)
}

// TestStoreMigrateRefusesUncommittedRecords covers the move-out safety check:
// uncommitted record content has no history behind it, so the move refuses and
// names the paths until --force is given.
func TestStoreMigrateRefusesUncommittedRecords(t *testing.T) {
	home := setStoreHome(t)
	root := migratedProject(t)
	record := filepath.Join(root, ".ahm", "tasks", "active", "001.md")
	writeFile(t, record, mustRead(t, record)+"\n## Notes\n\nuncommitted\n")

	stdout, stderr, code := runCLI(t, "--root", root, "store", "migrate", "--to", "home")
	if code != 1 {
		t.Fatalf("migrate with uncommitted records: code = %d, want 1 (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	assertContainsAll(t, stderr,
		"refusing to move task records: .ahm/tasks has uncommitted changes",
		"  .ahm/tasks/active/001.md",
		"--force",
	)
	// Nothing moved, and the store was not created.
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "active", "001.md")); err != nil {
		t.Errorf("the refusal moved a record: %v", err)
	}
	assertStoreAbsent(t, home)
	assertNotContains(t, mustRead(t, filepath.Join(root, ".ahm", "config.json")), `"tasks_location"`)

	// A record that was never committed is uncommitted content too.
	if _, stderr, code := runCLI(t, "--root", root, "task", "create", "Untracked"); code != 0 {
		t.Fatalf("task create: %s", stderr)
	}
	_, stderr, code = runCLI(t, "--root", root, "store", "migrate", "--to", "home")
	if code != 1 {
		t.Fatalf("migrate with an untracked record: code = %d, want 1 (stderr=%q)", code, stderr)
	}
	assertContainsAll(t, stderr, "  .ahm/tasks/active/002.md")

	// --force is the documented override.
	stdout, stderr, code = runCLI(t, "--root", root, "store", "migrate", "--to", "home", "--force")
	if code != 0 {
		t.Fatalf("forced migrate: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout, "move 2 record(s) to the home store", "deletions to commit:")
	assertFileContainsAll(t, storeTaskFile(t, root, "active", "001"), "uncommitted")
}

// TestStoreMigrateRefusesADivergentDestinationRecord covers the destination
// states that are not a resume: a record that already exists with different
// bytes, and a record that already wears the arriving record's ID in another
// bucket. The move refuses to leave two divergent copies of one record, because
// a store copy has no history to recover from; --force is the documented
// override.
func TestStoreMigrateRefusesADivergentDestinationRecord(t *testing.T) {
	setStoreHome(t)
	root := migratedProject(t)
	divergent := storeTaskFile(t, root, "active", "001")
	writeFile(t, divergent, "---\nid: 001\ntitle: Store Version\nstatus: Pending\n---\n")

	// The refusal is what a preview reports too, so a dry run names the conflict
	// before a real run would meet it.
	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "store", "migrate", "--to", "home")
	if code != 1 {
		t.Fatalf("dry run with a divergent destination: code = %d, want 1 (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	assertContainsAll(t, stderr,
		"refusing to move .ahm/tasks/active/001.md",
		"store:tasks/active/001.md already holds a different record",
		"--force",
	)
	stdout, stderr, code = runCLI(t, "--root", root, "store", "migrate", "--to", "home")
	if code != 1 {
		t.Fatalf("migrate with a divergent destination: code = %d, want 1 (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	// Neither side moved: the store copy is the one that differs, and the
	// project record is still the committed one.
	assertFileContainsAll(t, divergent, "Store Version")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "title: First")

	stdout, stderr, code = runCLI(t, "--root", root, "store", "migrate", "--to", "home", "--force")
	if code != 0 {
		t.Fatalf("forced migrate: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout, "move 1 record(s) to the home store")
	assertFileContainsAll(t, divergent, "title: First")

	// The refusal holds in the other direction too: the project record is the
	// destination then, and the store's record is the one arriving.
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Project Version", "Pending", "")
	commitEverything(t, root, "a divergent project record")
	stdout, stderr, code = runCLI(t, "--root", root, "store", "migrate", "--to", "project")
	if code != 1 {
		t.Fatalf("migrate over a divergent project record: code = %d, want 1 (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	assertContainsAll(t, stderr,
		"refusing to move store:tasks/active/001.md",
		".ahm/tasks/active/001.md already holds a different record",
	)
}

// TestStoreMigrateRefusesADestinationRecordWithTheSameID covers the other
// destination conflict: a record wearing the arriving record's ID in a different
// bucket would survive the move beside it, and two records with one ID is a
// state no command can resolve.
func TestStoreMigrateRefusesADestinationRecordWithTheSameID(t *testing.T) {
	setStoreHome(t)
	root := migratedProject(t)
	// A copy of the record under another bucket, as an interrupted move or a
	// second clone can leave behind.
	writeTaskFile(t, storeTaskFile(t, root, "completed", "001"), "001", "First (completed)", "Completed", "")

	stdout, stderr, code := runCLI(t, "--root", root, "store", "migrate", "--to", "home")
	if code != 1 {
		t.Fatalf("migrate with a duplicate destination ID: code = %d, want 1 (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	assertContainsAll(t, stderr,
		"refusing to move .ahm/tasks/active/001.md",
		"store:tasks/completed/001.md already holds a record with ID 001",
		"--force",
	)
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "title: First")

	// --force moves it anyway, and the two records then exist side by side for
	// the user to resolve.
	stdout, stderr, code = runCLI(t, "--root", root, "store", "migrate", "--to", "home", "--force")
	if code != 0 {
		t.Fatalf("forced migrate: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout, "move 1 record(s) to the home store")
	assertFileContainsAll(t, storeTaskFile(t, root, "completed", "001"), "First (completed)")
}

// TestStoreMigrateWithoutGitMetadataSkipsTheCommitCheck covers a root managed by
// its configuration alone: there is no project Git history to protect, so the
// move proceeds without Git.
func TestStoreMigrateWithoutGitMetadataSkipsTheCommitCheck(t *testing.T) {
	setStoreHome(t)
	root := t.TempDir()
	writeAHMConfig(t, root)
	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("init: %s", stderr)
	}
	if _, stderr, code := runCLI(t, "--root", root, "task", "create", "First"); code != 0 {
		t.Fatalf("task create: %s", stderr)
	}

	stdout, stderr, code := runCLI(t, "--root", root, "store", "migrate", "--to", "home")
	if code != 0 {
		t.Fatalf("migrate: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout, "move 1 record(s) to the home store", "  .ahm/tasks/active/001.md -> store:tasks/active/001.md")
	assertFileContainsAll(t, storeTaskFile(t, root, "active", "001"), "title: First")
}

// TestStoreMigrateRefusesAStoreOfAnotherKey covers the destination check: a
// store directory registered to a different project key never receives this
// project's records, and the refusal names both paths.
func TestStoreMigrateRefusesAStoreOfAnotherKey(t *testing.T) {
	home := setStoreHome(t)
	root := migratedProject(t)
	store, err := resolveStore(root)
	if err != nil {
		t.Fatal(err)
	}
	otherKey := "example.com/other/project"
	writeFile(t, filepath.Join(home, storeRegistryFileName), `{
  "version": 1,
  "projects": {
    "`+otherKey+`": {
      "key": "`+otherKey+`",
      "kind": "remote",
      "dir": "`+store.dirName()+`",
      "created": "2026-01-01T00:00:00Z"
    }
  }
}
`)

	stdout, stderr, code := runCLI(t, "--root", root, "store", "migrate", "--to", "home")
	if code != 1 {
		t.Fatalf("migrate into another key's store: code = %d, want 1 (stdout=%q stderr=%q)", code, stdout, stderr)
	}
	assertContainsAll(t, stderr,
		"refusing to move task records for "+root,
		"into "+store.ProjectDir,
		otherKey,
		store.Key,
	)
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "active", "001.md")); err != nil {
		t.Errorf("the refusal moved a record: %v", err)
	}
}

// TestStoreMigrateWithoutRecordsRewritesTheMode covers a move whose source is
// empty: the configuration, the managed .gitignore, and the destination's
// indexes follow the requested mode even when no record moves.
func TestStoreMigrateWithoutRecordsRewritesTheMode(t *testing.T) {
	setStoreHome(t)
	root := newGitRepo(t)
	writeAHMConfig(t, root)
	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("init: %s", stderr)
	}
	commitEverything(t, root, "install workflow")

	stdout, stderr, code := runCLI(t, "--root", root, "store", "migrate", "--to", "home")
	if code != 0 {
		t.Fatalf("migrate: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout, "move no records", "write .ahm/config.json", "write .ahm/.gitignore")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "config.json"), `"tasks_location": "home"`)
	assertFileContainsAll(t, filepath.Join(root, ".ahm", ".gitignore"), "*.tmp")

	stdout, stderr, code = runCLI(t, "--root", root, "store", "migrate", "--to", "project")
	if code != 0 {
		t.Fatalf("migrate back: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout, "move no records", `write .ahm/config.json`)
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "config.json"), `"tasks_location": "project"`)
	assertFileContainsAll(t, filepath.Join(root, ".ahm", ".gitignore"), "tasks/index.md", ".lock/")
}

// TestStoreMigrateWithoutWorkRemovesTheSourceLeftovers pins the recovery path of
// a run that stopped before it cleaned up its source. The committed .gitignore
// stops ignoring the source's generated indexes, so a repeated command — which
// has no records left to move — is what removes them, and the run that left them
// behind reports no work on every re-run.
func TestStoreMigrateWithoutWorkRemovesTheSourceLeftovers(t *testing.T) {
	setStoreHome(t)
	root := migratedProject(t)
	if _, stderr, code := runCLI(t, "--root", root, "store", "migrate", "--to", "home"); code != 0 {
		t.Fatalf("migrate: %s", stderr)
	}
	commitEverything(t, root, "migrate to home")
	leftover := filepath.Join(root, ".ahm", "tasks", "index.md")
	writeFile(t, leftover, "# Task Index\n")

	stdout, stderr, code := runCLI(t, "--root", root, "store", "migrate", "--to", "home")
	if code != 0 {
		t.Fatalf("repeated migrate: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout, "remove .ahm/tasks/index.md")
	if _, err := os.Stat(leftover); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the leftover index survived a repeated run: %v", err)
	}
	assertNotContains(t, stdout, "move 1 record")

	// With nothing left to clean up, the command reports no work again.
	stdout, stderr, code = runCLI(t, "--root", root, "store", "migrate", "--to", "home")
	if code != 0 {
		t.Fatalf("clean repeated migrate: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout, "no work: task records already live in the home store")
}

// TestStatusReportsTaskRecordsLeftInTheProject pins the drift finding in
// status, which is what makes a half-finished move visible. doctor shares the
// same validation path and is covered in store_records_test.go.
func TestStatusReportsTaskRecordsLeftInTheProject(t *testing.T) {
	setStoreHome(t)
	root := migratedProject(t)
	if _, stderr, code := runCLI(t, "--root", root, "store", "migrate", "--to", "home"); code != 0 {
		t.Fatalf("migrate: %s", stderr)
	}
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "900.md"), "900", "Left behind", "Pending", "")

	stdout, stderr, code := runCLI(t, "--root", root, "--json", "status")
	if code != 1 {
		t.Fatalf("status code = %d, want 1: stdout=%q stderr=%q", code, stdout, stderr)
	}
	assertContainsAll(t, stdout,
		`"code": "task_records_in_project"`,
		`"path": ".ahm/tasks"`,
		"a task record remains in the project while tasks_location is home",
	)
}

// TestStoreMigrateToHomeOnAnUninitializedRepository pins the direction a
// repository with no configuration takes: it names no layout, so the move runs
// instead of reporting that the records already live in the store, and the mode
// --to asks for is written.
func TestStoreMigrateToHomeOnAnUninitializedRepository(t *testing.T) {
	home := setStoreHome(t)
	root := newGitRepo(t)

	stdout, stderr, code := runCLI(t, "--root", root, "store", "migrate", "--to", "home")
	if code != 0 {
		t.Fatalf("migrate: stdout=%q stderr=%q", stdout, stderr)
	}
	assertNotContains(t, stdout, "no work")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "config.json"), `"tasks_location": "home"`)
	store, err := resolveStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.recordsDir()); err != nil {
		t.Errorf("the move did not create the store's records directory: %v", err)
	}
	assertFileContainsAll(t, filepath.Join(home, storeRegistryFileName), store.Key)
}

// TestStoreMigrateToProjectOnAnUninitializedRepository pins the other direction:
// a repository with no configuration can keep its records in the project without
// an init first, and the move records nothing in a store that holds nothing.
func TestStoreMigrateToProjectOnAnUninitializedRepository(t *testing.T) {
	home := setStoreHome(t)
	root := newGitRepo(t)

	stdout, stderr, code := runCLI(t, "--root", root, "store", "migrate", "--to", "project")
	if code != 0 {
		t.Fatalf("migrate: stdout=%q stderr=%q", stdout, stderr)
	}
	assertNotContains(t, stdout, "no work")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "config.json"), `"tasks_location": "project"`)
	for _, bucket := range []string{"active", "completed", "cancelled"} {
		if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", bucket)); err != nil {
			t.Errorf("the move did not create .ahm/tasks/%s: %v", bucket, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, storeRegistryFileName)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the move recorded a registry entry for a store that holds nothing: %v", err)
	}
}

// TestStoreMigrateReportsTheFindingsIndexReports pins the post-mutation findings
// a move emits: a record the scan could not parse is reported with the same
// disk-derived finding ahm index reports, not only the aggregate warning that
// names the file.
func TestStoreMigrateReportsTheFindingsIndexReports(t *testing.T) {
	setStoreHome(t)
	root := migratedProject(t)
	writeFile(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"),
		"---\nid: 002\ntitle: Malformed\nstatus: Pending\ndepends_on:\n- 001\n---\n")
	commitEverything(t, root, "malformed record")

	_, indexStderr, code := runCLI(t, "--root", root, "index")
	if code != 0 {
		t.Fatalf("index: code = %d (stderr=%q)", code, indexStderr)
	}
	_, migrateStderr, code := runCLI(t, "--root", root, "store", "migrate", "--to", "home")
	if code != 0 {
		t.Fatalf("migrate: code = %d (stderr=%q)", code, migrateStderr)
	}
	const finding = "unsupported block list syntax in front matter"
	assertContainsAll(t, indexStderr, finding)
	assertContainsAll(t, migrateStderr, "some task files could not be parsed and were skipped")
	// The finding has to stand alone on its own warning line: the aggregate
	// warning above already carries the same sentence as the parse error's text,
	// so only a separate line proves the disk-reading validation ran.
	assertContainsAll(t, migrateStderr, "\nwarning: "+finding)
}

// TestStoreMigratePreviewLeavesTheGitIndexAlone pins the read-only Git guarantee
// the preview claims: `git status` refreshes Git's own index stat cache, so
// without GIT_OPTIONAL_LOCKS=0 a --dry-run run rewrites .git/index in the
// repository it promises to leave alone.
func TestStoreMigratePreviewLeavesTheGitIndexAlone(t *testing.T) {
	setStoreHome(t)
	root := migratedProject(t)
	// Make the index's stat cache stale, so a Git command that takes the
	// optional lock has something to refresh.
	record := filepath.Join(root, ".ahm", "tasks", "active", "001.md")
	info, err := os.Stat(record)
	if err != nil {
		t.Fatal(err)
	}
	stale := info.ModTime().Add(time.Hour)
	if err := os.Chtimes(record, stale, stale); err != nil {
		t.Fatal(err)
	}

	index := filepath.Join(root, ".git", "index")
	before := mustRead(t, index)
	if _, stderr, code := runCLI(t, "--root", root, "--dry-run", "store", "migrate", "--to", "home"); code != 0 {
		t.Fatalf("dry run: %s", stderr)
	}
	if after := mustRead(t, index); after != before {
		t.Errorf("the preview rewrote .git/index")
	}
}

// TestStoreMigrateCommitsLast pins the move's commit point: every other write
// lands before the committed configuration, so until that last write a re-run
// finds the records where they now are and finishes the move rather than
// reporting it done.
func TestStoreMigrateCommitsLast(t *testing.T) {
	storeHome := setStoreHome(t)
	root := migratedProject(t)
	store, err := resolveStore(root)
	if err != nil {
		t.Fatal(err)
	}

	interrupted := errors.New("interrupted before the commit point")
	storeMigrateCommitHook = func() error { return interrupted }
	t.Cleanup(func() { storeMigrateCommitHook = nil })

	_, stderr, code := runCLI(t, "--root", root, "store", "migrate", "--to", "home")
	if code != 1 {
		t.Fatalf("interrupted migrate: code = %d, want 1 (stderr=%q)", code, stderr)
	}
	assertContainsAll(t, stderr, "interrupted before the commit point")
	// Everything except the committed configuration landed: the records, the
	// store's indexes and state, and both .gitignore files are in place, the
	// source is cleaned up, and the configuration still names the project.
	assertFileContainsAll(t, storeTaskFile(t, root, "active", "001"), "title: First")
	assertFileContainsAll(t, filepath.Join(store.recordsDir(), "index.md"), "First")
	assertFileContainsAll(t, filepath.Join(store.ProjectDir, storeStateFileName), "next_id")
	assertFileContainsAll(t, filepath.Join(store.ProjectDir, gitignoreFileName), storeStateFileName)
	assertFileContainsAll(t, filepath.Join(root, ".ahm", ".gitignore"), gitignoreTempPattern)
	assertNoProjectRecords(t, root)
	assertNotContains(t, mustRead(t, filepath.Join(root, ".ahm", "config.json")), `"tasks_location"`)

	// The re-run has nothing left to do but the commit point, and the store's own
	// bookkeeping is not rewritten.
	storeMigrateCommitHook = nil
	storeBefore := snapshotTree(t, filepath.Join(storeHome, storeProjectsDirName))
	stdout, stderr, code := runCLI(t, "--root", root, "store", "migrate", "--to", "home")
	if code != 0 {
		t.Fatalf("resumed migrate: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout, "move no records", "write .ahm/config.json")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "config.json"), `"tasks_location": "home"`)
	assertTreeUnchanged(t, filepath.Join(storeHome, storeProjectsDirName), storeBefore)
}

// TestStoreMigrateToProjectDoesNotCreateAStore covers a repository whose
// configuration names the store while the store is not on this machine, as a
// fresh clone of a home-mode project is. Moving the records back into the
// project has nothing to record in a store that holds nothing, so it creates no
// registry entry and no state file. The lock the move holds lives in the store's
// project directory, so that directory — and the .lock directory inside it — is
// the one thing that exists there afterwards, exactly as any other home-mode
// mutation leaves it.
func TestStoreMigrateToProjectDoesNotCreateAStore(t *testing.T) {
	home := setStoreHome(t)
	root := migratedProject(t)
	if _, stderr, code := runCLI(t, "--root", root, "store", "migrate", "--to", "home"); code != 0 {
		t.Fatalf("migrate: %s", stderr)
	}
	if err := os.RemoveAll(home); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "--root", root, "store", "migrate", "--to", "project")
	if code != 0 {
		t.Fatalf("migrate to project: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout, "write .ahm/config.json")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "config.json"), `"tasks_location": "project"`)
	if _, err := os.Stat(filepath.Join(home, storeRegistryFileName)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the move recorded a registry entry for a store that holds nothing: %v", err)
	}
	var stateFiles []string
	if err := filepath.WalkDir(home, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && entry.Name() == storeStateFileName {
			stateFiles = append(stateFiles, path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(stateFiles) > 0 {
		t.Errorf("the move recorded store state for a store that holds nothing: %v", stateFiles)
	}
}

func TestMoveTaskRecordsMovesEveryBucket(t *testing.T) {
	root := t.TempDir()
	from := workflowPathsFor(root)
	to := workflowPathsForStore(root, testStorePaths(t))
	for bucket, id := range map[string]string{"active": "001", "completed": "002", "cancelled": "003"} {
		writeTaskFile(t, from.taskFile(bucket, id), id, "Task "+id, "Pending", "")
	}
	// A generated index is not a record and stays where it is.
	writeFile(t, filepath.Join(from.tasksBucketDir("active"), "index.md"), "# Active Tasks\n")

	moved, err := moveTaskRecords(from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(moved) != 3 {
		t.Errorf("moveTaskRecords moved %d records, want 3: %v", len(moved), moved)
	}
	for bucket, id := range map[string]string{"active": "001", "completed": "002", "cancelled": "003"} {
		if _, err := os.Stat(from.taskFile(bucket, id)); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the source record %s/%s.md survived the move: %v", bucket, id, err)
		}
		assertFileContainsAll(t, to.taskFile(bucket, id), "title: Task "+id)
	}
	if _, err := os.Stat(filepath.Join(from.tasksBucketDir("active"), "index.md")); err != nil {
		t.Errorf("the move removed a generated index: %v", err)
	}

	// A second move has nothing to do, and an identical destination record keeps
	// its modification time when the source is restored.
	if moved, err := moveTaskRecords(from, to); err != nil || len(moved) != 0 {
		t.Errorf("a repeated move = %v (err %v), want no records", moved, err)
	}
	arrived := to.taskFile("active", "001")
	writeTaskFile(t, from.taskFile("active", "001"), "001", "Task 001", "Pending", "")
	before, err := os.Stat(arrived)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := moveTaskRecords(from, to); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(arrived)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("the move rewrote an identical destination record")
	}
}

func TestMigratedRecordPathKeepsTheBucketAndName(t *testing.T) {
	root := t.TempDir()
	from := workflowPathsFor(root)
	to := workflowPathsForStore(root, testStorePaths(t))
	src := from.taskFile("completed", "137a")
	got, err := migratedRecordPath(from, to, src)
	if err != nil {
		t.Fatal(err)
	}
	if want := to.taskFile("completed", "137a"); got != want {
		t.Errorf("migratedRecordPath(%s) = %s, want %s", src, got, want)
	}
}

// TestRecordPathsOnlyKeepsRecordPaths pins the filter the uncommitted-record
// refusal applies to Git's output: it keeps exactly the files the task scan
// reads, so a generated index, a file nested under a bucket, a file in the
// records root, and a non-markdown file never reach the user as record content
// the move would delete.
func TestRecordPathsOnlyKeepsRecordPaths(t *testing.T) {
	paths := workflowPathsFor("/project")
	entries := []string{
		".ahm/tasks/active/001.md",
		".ahm/tasks/completed/002.md",
		".ahm/tasks/cancelled/003.md",
		".ahm/tasks/index.md",
		".ahm/tasks/active/index.md",
		".ahm/tasks/notes.md",
		".ahm/tasks/active/sub/nested.md",
		".ahm/tasks/tracking/004.md",
		".ahm/tasks/active/005.txt",
		".ahm/config.json",
	}

	got := recordPathsOnly(paths, entries)
	want := []string{
		".ahm/tasks/active/001.md",
		".ahm/tasks/completed/002.md",
		".ahm/tasks/cancelled/003.md",
	}
	if !slices.Equal(got, want) {
		t.Errorf("recordPathsOnly = %v, want %v", got, want)
	}
}

func TestParsePorcelainPaths(t *testing.T) {
	out := strings.Join([]string{
		" M .ahm/tasks/active/001.md",
		"?? .ahm/tasks/active/002.md",
		"D  .ahm/tasks/cancelled/003.md",
		" D .ahm/tasks/cancelled/006.md",
		"R  .ahm/tasks/active/004.md -> .ahm/tasks/active/005.md",
		"",
	}, "\n")
	got := parsePorcelainPaths(out)
	want := []string{
		".ahm/tasks/active/001.md",
		".ahm/tasks/active/002.md",
		".ahm/tasks/active/005.md",
	}
	if !slices.Equal(got, want) {
		t.Errorf("parsePorcelainPaths = %v, want %v", got, want)
	}
}
