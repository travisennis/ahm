package ahm

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatusReportsWorkflowArtifactConsistency(t *testing.T) {
	root := projectRoot(t)
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(); err != nil {
		t.Fatal(err)
	}
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Completed In Active", "Completed", "depends_on: []\n")
	if err := os.Remove(filepath.Join(root, ".ahm", "tasks", "cancelled", "index.md")); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.status(); !errors.Is(err, errValidationFailed) {
		t.Errorf("expected errValidationFailed, got: %v", err)
	}
	got := out.String()
	assertContainsAll(t, got,
		`"code": "task_bucket_mismatch"`,
		`completed task should be in .ahm/tasks/completed`,
		`"code": "generated_index_missing"`,
		`"path": ".ahm/tasks/cancelled/index.md"`,
	)
}

func TestValidateTaskDuplicateIDsReportsError(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	paths := workflowPathsFor(root)

	// Write two task files with the same ID in different buckets.
	writeTaskFile(t, paths.taskFile("active", "042"), "042", "Duplicate Task A", "Pending", "")
	writeTaskFile(t, paths.taskFile("completed", "042"), "042", "Duplicate Task B", "Completed", "depends_on: -\n")

	report, tasks := validateWorkflowScopedForPaths(root, []string{CheckScopeWorkflow}, paths)

	if !hasFinding(report.Errors, "task_duplicate_id") {
		t.Fatalf("expected task_duplicate_id error, got errors: %#v", report.Errors)
	}
	// Verify the error message contains both file paths.
	for _, f := range report.Errors {
		if f.Code == "task_duplicate_id" {
			if f.Path != "" {
				t.Errorf("task_duplicate_id should have empty path, got %q", f.Path)
			}
			if !strings.Contains(f.Message, "042") || !strings.Contains(f.Message, "active/042.md") || !strings.Contains(f.Message, "completed/042.md") {
				t.Errorf("task_duplicate_id message missing expected paths: %q", f.Message)
			}
		}
	}

	// Verify the tasks list still includes both (validation is read-only, no filtering).
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks in list, got %d", len(tasks))
	}
}

func TestValidateTaskDuplicateIDsReportsErrorInReusedState(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	paths := workflowPathsFor(root)

	writeTaskFile(t, paths.taskFile("active", "042"), "042", "Duplicate Task A", "Pending", "")
	writeTaskFile(t, paths.taskFile("completed", "042"), "042", "Duplicate Task B", "Completed", "depends_on: -\n")

	tasks, err := collectTasksForPaths(paths)
	if err != nil {
		t.Fatal(err)
	}
	writes, err := indexWritesForPaths(root, tasks, paths, nil)
	if err != nil {
		t.Fatal(err)
	}

	report := validateWorkflowStateForPaths(root, paths, tasks, writes, nil, nil)
	if !hasFinding(report.Errors, "task_duplicate_id") {
		t.Fatalf("expected task_duplicate_id error in reused state, got errors: %#v", report.Errors)
	}
}

// TestValidateTaskDuplicateTitlesReportsOneWarningPerSharedTitle pins the
// state-level counterpart to the create/edit/import warning: one warning-tier
// finding per duplicated active title, case-insensitive, with completed and
// cancelled records skipped on every side.
func TestValidateTaskDuplicateTitlesReportsOneWarningPerSharedTitle(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	paths := workflowPathsFor(root)

	// One title shared across three active records, case-insensitively equal,
	// plus a completed record that reuses it. Under the command-side rule the
	// completed record is not part of the duplicate set.
	writeTaskFile(t, paths.taskFile("active", "001"), "001", "Shared title", "Pending", "")
	writeTaskFile(t, paths.taskFile("active", "002"), "002", "shared TITLE", "Pending", "")
	writeTaskFile(t, paths.taskFile("active", "010"), "010", "SHARED title", "Pending", "")
	writeTaskFile(t, paths.taskFile("active", "003"), "003", "Unique", "Pending", "")
	writeTaskFile(t, paths.taskFile("completed", "004"), "004", "Shared title", "Completed", "depends_on: -\n")
	// Two non-active records that share a title are both skipped.
	writeTaskFile(t, paths.taskFile("completed", "005"), "005", "Chore", "Completed", "depends_on: -\n")
	writeTaskFile(t, paths.taskFile("cancelled", "006"), "006", "chore", "Cancelled", "depends_on: -\n")

	tasks, err := collectTasksForPaths(paths)
	if err != nil {
		t.Fatal(err)
	}
	var report validationReport
	validateTaskDuplicateTitles(paths, tasks, &report)

	if len(report.Errors) != 0 {
		t.Fatalf("duplicate titles must be warning-tier, got errors: %#v", report.Errors)
	}
	if len(report.Warnings) != 1 {
		t.Fatalf("expected exactly one finding for the shared title, got: %#v", report.Warnings)
	}
	finding := report.Warnings[0]
	if finding.Code != "task_duplicate_title" {
		t.Errorf("code = %q, want task_duplicate_title", finding.Code)
	}
	if finding.Path != "" {
		t.Errorf("task_duplicate_title reuses the duplicate-ID convention of an empty path, got %q", finding.Path)
	}
	assertContainsAll(t, finding.Message, `"Shared title"`, "active/001.md", "active/002.md", "active/010.md")
	assertNotContains(t, finding.Message, "003.md", "004.md", "005.md", "006.md")
}

// TestValidateTaskDuplicateTitlesAreSortedByTitle pins the deterministic order
// of the findings so a multi-title repository reports the same sequence every
// run.
func TestValidateTaskDuplicateTitlesAreSortedByTitle(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	paths := workflowPathsFor(root)

	writeTaskFile(t, paths.taskFile("active", "001"), "001", "Beta", "Pending", "")
	writeTaskFile(t, paths.taskFile("active", "002"), "002", "Beta", "Pending", "")
	writeTaskFile(t, paths.taskFile("active", "003"), "003", "Alpha", "Pending", "")
	writeTaskFile(t, paths.taskFile("active", "004"), "004", "alpha", "Pending", "")

	tasks, err := collectTasksForPaths(paths)
	if err != nil {
		t.Fatal(err)
	}
	var report validationReport
	validateTaskDuplicateTitles(paths, tasks, &report)

	if len(report.Warnings) != 2 {
		t.Fatalf("expected two findings, got: %#v", report.Warnings)
	}
	assertContainsAll(t, report.Warnings[0].Message, `"Alpha"`)
	assertContainsAll(t, report.Warnings[1].Message, `"Beta"`)
}

// TestStatusAndDoctorWarnOnPreExistingDuplicateTitle proves the finding reaches
// the standalone validators as a warning: the pair predates the command, no
// command introduced it, and no command's exit code changes. It also pins the
// scope and rendering: `prime` surfaces it, the `links` check scope does not,
// and in home mode the message carries store-relative display paths.
func TestStatusAndDoctorWarnOnPreExistingDuplicateTitle(t *testing.T) {
	root := projectRoot(t)
	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("init: %s", stderr)
	}
	createTask(t, root, "Duplicate title")
	createTask(t, root, "duplicate TITLE")

	for _, command := range []string{"status", "doctor"} {
		t.Run(command, func(t *testing.T) {
			out, stderr, code := runCLI(t, "--root", root, "--json", command)
			if code != 0 {
				t.Fatalf("%s code=%d stdout=%q stderr=%q", command, code, out, stderr)
			}
			assertContainsAll(t, out,
				`"code": "task_duplicate_title"`,
				"active/001.md", "active/002.md",
				// A duplicate title is a warning, so validation stays ok.
				`"ok": true`,
			)
		})
	}

	t.Run("prime reports the finding", func(t *testing.T) {
		out, stderr, code := runCLI(t, "--root", root, "--json", "prime")
		if code != 0 {
			t.Fatalf("prime code=%d stdout=%q stderr=%q", code, out, stderr)
		}
		assertContainsAll(t, out, `"code": "task_duplicate_title"`, "active/001.md", "active/002.md")
	})

	t.Run("the links scope excludes it", func(t *testing.T) {
		out, stderr, code := runCLI(t, "--root", root, "--json", "--check", "links", "status")
		if code != 0 {
			t.Fatalf("links-scope status code=%d stdout=%q stderr=%q", code, out, stderr)
		}
		assertNotContains(t, out, "task_duplicate_title")
	})

	t.Run("home mode renders store-relative paths", func(t *testing.T) {
		home := initHomeModeRepository(t)
		createTask(t, home, "Shared title")
		createTask(t, home, "shared title")
		out, stderr, code := runCLI(t, "--root", home, "--json", "doctor")
		if code != 0 {
			t.Fatalf("home-mode doctor code=%d stdout=%q stderr=%q", code, out, stderr)
		}
		assertContainsAll(t, out, `"code": "task_duplicate_title"`, "store:tasks/active/001.md", "store:tasks/active/002.md")
	})
}
