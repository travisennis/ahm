package ahm

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// relativeTreePaths lists the entries under dir, relative and sorted, so a test
// can assert exactly what a command created.
func relativeTreePaths(t *testing.T, dir string) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == dir {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	return paths
}

// writeHomeModeConfig writes the committed configuration of a repository whose
// records live in the home store.
func writeHomeModeConfig(t *testing.T, root string) {
	t.Helper()
	writeMetadataFile(t, root, metadata{
		TasksLocation: string(locationHome),
		Files:         map[string]string{},
	})
}

// storeTaskFile is the store record path of one task, resolved from root.
func storeTaskFile(t *testing.T, root string, bucket string, id string) string {
	t.Helper()
	store, err := resolveStore(root)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(store.recordsDir(), bucket, id+".md")
}

func TestResolveTaskLocation(t *testing.T) {
	cases := []struct {
		name          string
		tasksLocation string
		configExists  bool
		want          taskLocation
	}{
		{name: "explicit home", tasksLocation: "home", configExists: true, want: locationHome},
		{name: "explicit project", tasksLocation: "project", configExists: true, want: locationProject},
		{name: "missing key", tasksLocation: "", configExists: true, want: locationProject},
		{name: "unrecognized value", tasksLocation: "elsewhere", configExists: true, want: locationProject},
		// A repository with no configuration at all is a new project. Milestone
		// 267f turns on the home default for it; until then every configuration
		// without a key keeps its records in the project, which is what protects
		// existing repositories.
		{name: "no configuration", tasksLocation: "", configExists: false, want: locationProject},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			meta := metadata{TasksLocation: tc.tasksLocation}
			if got := resolveTaskLocation(meta, tc.configExists); got != tc.want {
				t.Errorf("resolveTaskLocation(%+v, %v) = %q, want %q", meta, tc.configExists, got, tc.want)
			}
		})
	}
}

func TestTaskLocationRoundTripsThroughMetadata(t *testing.T) {
	data, err := marshalMetadata(metadata{
		StrictAcceptance: true,
		TasksLocation:    string(locationHome),
		Files:            map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"tasks_location": "home"`) {
		t.Errorf("metadata omitted the tasks_location key:\n%s", data)
	}
	var meta metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.TasksLocation != string(locationHome) {
		t.Errorf("TasksLocation = %q, want %q", meta.TasksLocation, locationHome)
	}
	// The key is ahm-owned: it is never preserved as an unknown field, so the
	// next metadata write cannot duplicate it.
	if _, ok := meta.Extra["tasks_location"]; ok {
		t.Errorf("tasks_location was preserved as an unknown field: %v", meta.Extra)
	}

	// A configuration without the key omits it, so project-mode configuration
	// bytes are unchanged by this milestone.
	data, err = marshalMetadata(metadata{Files: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "tasks_location") {
		t.Errorf("metadata wrote an empty tasks_location key:\n%s", data)
	}
}

func TestExplicitProjectLocationKeepsRecordsInTheProject(t *testing.T) {
	home := setStoreHome(t)
	root := t.TempDir()
	writeMetadataFile(t, root, metadata{
		TasksLocation: string(locationProject),
		Files:         map[string]string{},
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "create", "Project task")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Fatalf("task create: stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "active", "001.md")); err != nil {
		t.Errorf("explicit project mode did not write the record in the project: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, storeProjectsDirName)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("explicit project mode touched the store: %v", err)
	}
}

// TestHomeModeKeepsTaskRecordsInTheStore is the milestone's core behavior: with
// tasks_location home, the whole task lifecycle reads and writes the store and
// writes nothing under .ahm/tasks/.
func TestHomeModeKeepsTaskRecordsInTheStore(t *testing.T) {
	home := setStoreHome(t)
	root := t.TempDir()
	writeHomeModeConfig(t, root)

	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout,
		"created:",
		"  store:.gitignore",
		"directories:\n  store:tasks/active\n  store:tasks/completed\n  store:tasks/cancelled\n  docs/adr\n",
		"indexes:",
		"  store:tasks/index.md",
		"  docs/adr/index.md",
	)
	projectRecords := filepath.Join(root, ".ahm", "tasks")
	if _, err := os.Stat(projectRecords); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("home-mode init created %s: %v", projectRecords, err)
	}
	if _, err := os.Stat(filepath.Join(home, storeProjectsDirName)); err != nil {
		t.Errorf("home-mode init did not create the store project directory: %v", err)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Store task", "--priority", "P1")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Fatalf("task create: stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	record := storeTaskFile(t, root, "active", "001")
	assertFileContainsAll(t, record, "title: Store task", "priority: P1")
	if _, err := os.Stat(projectRecords); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("task create wrote under the project records directory: %v", err)
	}
	// The generated task indexes moved with the records and keep their links.
	assertFileContainsAll(t, filepath.Join(filepath.Dir(record), "index.md"), "Store task", "(001.md)")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "list")
	if code != 0 {
		t.Fatalf("task list: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout, "001 [Open] P1 S Store task")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "show", "001")
	if code != 0 {
		t.Fatalf("task show: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout, "title: Store task")

	stdout, stderr, code = runCLI(t, "--root", root, "--json", "task", "show", "001")
	if code != 0 {
		t.Fatalf("task show --json: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout, `"path": "store:tasks/active/001.md"`)
	assertNotContains(t, stdout, home)

	stdout, stderr, code = runCLI(t, "--root", root, "--json", "task", "list")
	if code != 0 {
		t.Fatalf("task list --json: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout, `"path": "store:tasks/active/001.md"`)
	assertNotContains(t, stdout, home)

	stdout, stderr, code = runCLI(t, "--root", root, "--json", "task", "search", "Store")
	if code != 0 {
		t.Fatalf("task search --json: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout, `"path": "store:tasks/active/001.md"`)
	assertNotContains(t, stdout, home)

	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Dependent task", "--depends-on", "001", "--status", "Blocked")
	if code != 0 || strings.TrimSpace(stdout) != "002" {
		t.Fatalf("second create: stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "--dry-run", "task", "complete", "001")
	if code != 0 {
		t.Fatalf("dry-run complete: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout, "move: store:tasks/completed/001.md", "unblocked:", "store:tasks/active/002.md")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "complete", "001", "--force")
	if code != 0 {
		t.Fatalf("task complete: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout, "001 -> Completed", "002 -> Pending")
	if _, err := os.Stat(storeTaskFile(t, root, "completed", "001")); err != nil {
		t.Errorf("completed record is not in the store: %v", err)
	}
	if _, err := os.Stat(record); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("completion left the record in the active bucket: %v", err)
	}
	assertFileContainsAll(t, filepath.Join(filepath.Dir(filepath.Dir(record)), "index.md"), "Completed: 1")

	// The whole run left the project's records directory absent and .ahm/ holding
	// nothing ahm did not have to put there.
	if _, err := os.Stat(projectRecords); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the task lifecycle created the project records directory: %v", err)
	}
	if got := relativeTreePaths(t, filepath.Join(root, ".ahm")); !slices.Equal(got, []string{"config.json"}) {
		t.Errorf("home mode left %v under .ahm/, want only config.json", got)
	}
	// The store's managed .gitignore covers the generated indexes, the lock, the
	// store state file, and temp files.
	storeGitignore := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(record))), gitignoreFileName)
	assertFileContainsAll(t, storeGitignore, "tasks/index.md", ".lock/", "*.tmp", storeStateFileName)
	assertNotContains(t, mustRead(t, storeGitignore), "config.json remain committed")
}

func TestHomeModeStatusReportsTheStore(t *testing.T) {
	home := setStoreHome(t)
	root := t.TempDir()
	writeHomeModeConfig(t, root)
	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("init failed: %s", stderr)
	}
	store, err := resolveStore(root)
	if err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "--root", root, "--json", "status")
	if code != 0 {
		t.Fatalf("status: stdout=%q stderr=%q", stdout, stderr)
	}
	var report struct {
		Store map[string]string `json:"store"`
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("unmarshal status %q: %v", stdout, err)
	}
	want := map[string]string{
		"root":     home,
		"key":      store.Key,
		"kind":     store.Kind,
		"location": string(locationHome),
	}
	if len(report.Store) != len(want) {
		t.Fatalf("status store = %v, want %v", report.Store, want)
	}
	for key, value := range want {
		if report.Store[key] != value {
			t.Errorf("status store[%q] = %q, want %q", key, report.Store[key], value)
		}
	}

	stdout, stderr, code = runCLI(t, "--root", root, "prime")
	if code != 0 {
		t.Fatalf("prime: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout,
		"store:",
		"  root: "+home,
		"  key: "+store.Key,
		"  kind: "+store.Kind,
		"  location: home",
	)

	stdout, stderr, code = runCLI(t, "--root", root, "--json", "prime")
	if code != 0 {
		t.Fatalf("prime --json: stdout=%q stderr=%q", stdout, stderr)
	}
	var briefing struct {
		Store map[string]string `json:"store"`
	}
	if err := json.Unmarshal([]byte(stdout), &briefing); err != nil {
		t.Fatalf("unmarshal prime %q: %v", stdout, err)
	}
	if briefing.Store["kind"] != store.Kind || briefing.Store["location"] != string(locationHome) {
		t.Errorf("prime store = %v, want kind %q and location %q", briefing.Store, store.Kind, locationHome)
	}
}

// TestProjectModeOutputHasNoStoreField keeps the guard from ADR 023: a
// repository whose configuration has no tasks_location key reports no store,
// because its output must stay byte-identical to the output before the home
// store existed.
func TestProjectModeOutputHasNoStoreField(t *testing.T) {
	setStoreHome(t)
	root := t.TempDir()
	writeMetadataFile(t, root, metadata{Files: map[string]string{}})
	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("init failed: %s", stderr)
	}
	if _, stderr, code := runCLI(t, "--root", root, "task", "create", "Project task"); code != 0 {
		t.Fatalf("create failed: %s", stderr)
	}

	for _, args := range [][]string{
		{"--root", root, "--json", "status"},
		{"--root", root, "--json", "prime"},
		{"--root", root, "--json", "doctor"},
	} {
		stdout, stderr, code := runCLI(t, args...)
		if code != 0 {
			t.Fatalf("%v: stdout=%q stderr=%q code=%d", args, stdout, stderr, code)
		}
		assertNotContains(t, stdout, `"store"`, `"location"`)
	}

	// Structured payloads keep the record's own absolute path in project mode,
	// which is what keeps project-mode output byte-identical: displayPath would
	// have made them repository-relative.
	record := filepath.Join(root, ".ahm", "tasks", "active", "001.md")
	stdout, stderr, code := runCLI(t, "--root", root, "--json", "task", "list")
	if code != 0 {
		t.Fatalf("task list --json: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout, `"path": "`+filepath.ToSlash(record)+`"`)

	stdout, stderr, code = runCLI(t, "--root", root, "--json", "task", "show", "001")
	if code != 0 {
		t.Fatalf("task show --json: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout, `"path": "`+filepath.ToSlash(record)+`"`)

	stdout, stderr, code = runCLI(t, "--root", root, "--dry-run", "task", "complete", "001")
	if code != 0 {
		t.Fatalf("dry-run complete: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout, "move: "+filepath.ToSlash(filepath.Join(root, ".ahm", "tasks", "completed", "001.md")))
}

func TestHomeModeInitIsIdempotentAndDryRunWritesNothing(t *testing.T) {
	setStoreHome(t)
	root := t.TempDir()
	writeHomeModeConfig(t, root)

	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("first init failed: %s", stderr)
	}
	store, err := resolveStore(root)
	if err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, store.ProjectDir)

	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("second init failed: %s", stderr)
	}
	assertNotContains(t, stdout, "created:", "updated:", "directories:", "indexes:")
	assertTreeUnchanged(t, store.ProjectDir, before)

	// A dry run creates no store directory at all, even for a home-mode
	// project whose store does not exist yet.
	dryRoot := t.TempDir()
	writeHomeModeConfig(t, dryRoot)
	dryStore, err := resolveStore(dryRoot)
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code = runCLI(t, "--root", dryRoot, "--dry-run", "init")
	if code != 0 {
		t.Fatalf("dry-run init: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout,
		"created:",
		"  store:.gitignore",
		"directories:\n  store:tasks/active\n  store:tasks/completed\n  store:tasks/cancelled\n  docs/adr\n",
		"indexes:",
		"  store:tasks/index.md",
	)
	if _, err := os.Stat(dryStore.ProjectDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("dry-run init created the store project directory: %v", err)
	}
}

func TestDoctorReportsTaskRecordsLeftInTheProject(t *testing.T) {
	setStoreHome(t)
	root := t.TempDir()
	writeHomeModeConfig(t, root)
	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("init failed: %s", stderr)
	}
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "900.md"), "900", "Left behind", "Pending", "")

	stdout, stderr, code := runCLI(t, "--root", root, "--json", "doctor")
	if code != 1 {
		t.Fatalf("doctor code = %d, want 1: stdout=%q stderr=%q", code, stdout, stderr)
	}
	assertContainsAll(t, stdout,
		`"ok": false`,
		`"code": "task_records_in_project"`,
		`"path": ".ahm/tasks"`,
		"a task record remains in the project while tasks_location is home",
	)

	// The finding is read-only: the record stays where it is.
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "active", "900.md")); err != nil {
		t.Errorf("validation moved a record: %v", err)
	}

	// Leftover generated indexes in the project are not drift: they are derived,
	// ignored by Git, and regenerated in the store.
	if err := os.Remove(filepath.Join(root, ".ahm", "tasks", "active", "900.md")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".ahm", "tasks", "active", "index.md"), "# Active Tasks\n")
	writeFile(t, filepath.Join(root, ".ahm", "tasks", "index.md"), "# Task Index\n")
	stdout, stderr, code = runCLI(t, "--root", root, "--json", "doctor")
	if code != 0 {
		t.Fatalf("doctor code = %d, want 0 for leftover indexes: stdout=%q stderr=%q", code, stdout, stderr)
	}
	assertNotContains(t, stdout, "task_records_in_project")
}

// TestHomeModeFindingsRenderStorePaths covers the labels and messages that
// describe a record directory: they name the store's location, never an
// absolute machine path.
func TestHomeModeFindingsRenderStorePaths(t *testing.T) {
	setStoreHome(t)
	root := t.TempDir()
	writeHomeModeConfig(t, root)
	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	// A Completed record in the store's active bucket is a bucket mismatch.
	writeTaskFile(t, storeTaskFile(t, root, "active", "002"), "002", "Wrong bucket", "Completed", "depends_on: -\n")
	stdout, stderr, code := runCLI(t, "--root", root, "--json", "status")
	if code != 0 {
		t.Fatalf("status code = %d, want 0 for warning-tier findings: stdout=%q stderr=%q", code, stdout, stderr)
	}
	assertContainsAll(t, stdout,
		`"code": "task_bucket_mismatch"`,
		"completed task should be in store:tasks/completed",
		`"path": "store:tasks/active/002.md"`,
	)

	// An unreadable store bucket names the records root the same way.
	activeDir := filepath.Dir(storeTaskFile(t, root, "active", "002"))
	if err := os.RemoveAll(activeDir); err != nil {
		t.Fatal(err)
	}
	writeFile(t, activeDir, "not a directory")
	stdout, stderr, code = runCLI(t, "--root", root, "--json", "doctor")
	if code != 1 {
		t.Fatalf("doctor code = %d, want 1: stdout=%q stderr=%q", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, `"code": "task_dir_unreadable"`, `"path": "store:tasks"`)
}

func TestValidationReportsAnUnreadableStoreRecordsDirectory(t *testing.T) {
	setStoreHome(t)
	root := t.TempDir()
	writeHomeModeConfig(t, root)
	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("init failed: %s", stderr)
	}
	store, err := resolveStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(store.recordsDir()); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "--root", root, "--json", "doctor")
	if code != 1 {
		t.Fatalf("doctor code = %d, want 1: stdout=%q stderr=%q", code, stdout, stderr)
	}
	assertContainsAll(t, stdout,
		`"code": "store_dir_unreadable"`,
		`"path": "store:tasks"`,
		"the store's task records directory is missing",
	)
	assertNotContains(t, stdout, store.recordsDir())

	// A project-mode repository reports no such finding: its records root is
	// the project's own .ahm/tasks.
	projectRoot := t.TempDir()
	writeMetadataFile(t, projectRoot, metadata{Files: map[string]string{}})
	stdout, stderr, code = runCLI(t, "--root", projectRoot, "--json", "doctor")
	if code != 1 {
		t.Fatalf("project-mode doctor code = %d, want 1: stdout=%q stderr=%q", code, stdout, stderr)
	}
	assertNotContains(t, stdout, "store_dir_unreadable")
}

// TestStoreRecordLinksResolveAgainstTheProjectLayout covers the ADR 023
// resolution order: a relative link resolves against the record's own directory
// first, and against the record's logical in-project directory when that target
// does not exist, so records written before the move keep their links.
func TestStoreRecordLinksResolveAgainstTheProjectLayout(t *testing.T) {
	setStoreHome(t)
	root := t.TempDir()
	writeHomeModeConfig(t, root)
	writeFile(t, filepath.Join(root, "docs", "adr", "001-probe.md"),
		"---\nstatus: accepted\ndate: 2026-09-21\n---\n# Probe decision\n\nBody.\n")
	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	body := filepath.Join(t.TempDir(), "body.md")
	writeFile(t, body, "## Summary\n\n"+
		"Links to [ADR 001](../../../docs/adr/001-probe.md), to a sibling\n"+
		"[sibling](002.md), and to a missing [target](nope.md).\n")
	if _, stderr, code := runCLI(t, "--root", root, "task", "create", "Linker", "--body-file", body); code != 0 {
		t.Fatalf("create failed: %s", stderr)
	}
	if _, stderr, code := runCLI(t, "--root", root, "task", "create", "Sibling"); code != 0 {
		t.Fatalf("create failed: %s", stderr)
	}

	stdout, stderr, code := runCLI(t, "--root", root, "--json", "--check", "links", "status")
	if code != 0 {
		t.Fatalf("status: stdout=%q stderr=%q", stdout, stderr)
	}
	assertContainsAll(t, stdout, `"code": "markdown_link_missing"`, "nope.md")
	assertNotContains(t, stdout, "001-probe.md", "002.md")
}
