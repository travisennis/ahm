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
