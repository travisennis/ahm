package ahm

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newRefBackedWorkflowRepo builds a repository with valid workflow records
// and opts them into ref-backed storage directly (without using records
// migrate, which no longer produces ref state). This helper will be removed
// together with the ref-backed tests in task 172f.
func newRefBackedWorkflowRepo(t *testing.T) string {
	t.Helper()
	root := newGitRepo(t)

	// Set up workflow records under .ahm/ paths (migrated layout).
	writeFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), `---
id: 001
title: First Task
status: Pending
priority: P2
effort: S
labels: type:task
exec_plan: .agents/exec-plans/active/plan.md
depends_on: -
---
# First Task

## Summary

TODO.

## Acceptance Notes

- [x] Done.
`)
	writeFile(t, filepath.Join(root, ".ahm", "research", "topics", "note.md"), "# Ref Note\n\nBody.\n")
	writeFile(t, filepath.Join(root, ".ahm", "exec-plans", "active", "plan.md"), `# Plan

## Progress

- [x] Step one.

## Surprises & Discoveries

None.

## Decision Log

None.

## Outcomes & Retrospective
`)

	// Write a config with ref mode so the ref-backed commands work.
	writeFile(t, filepath.Join(root, ".ahm", "config.json"), `{
  "version": "test",
  "strict_acceptance": false,
  "store_mode": "ref",
  "records_ref": "refs/ahm/records",
  "records_remote": "origin",
  "files": {}
}`+"\n")

	// Write a gitignore that ignores source records (old ref-backed layout).
	writeFile(t, filepath.Join(root, ".ahm", ".gitignore"), `# Managed by ahm. Workflow records and generated indexes stay local-only;
# config.json remains committed.
/tasks/
/research/
/exec-plans/
`)

	// Run index to regenerate the indexes.
	if _, stderr, code := runCLI(t, "--root", root, "index"); code != 0 {
		t.Fatalf("index exit code = %d, stderr = %s", code, stderr)
	}

	// Seed refs/ahm/records with the current .ahm/ records.
	cfg := recordsStorageConfig{
		Mode:   recordStoreModeRef,
		Ref:    defaultRecordsRef,
		Remote: defaultRecordsRemote,
	}
	ctx := testContext(t)
	if _, err := snapshotRecordsRef(ctx, root, cfg, "Seed ref for tests"); err != nil {
		t.Fatalf("snapshotRecordsRef: %v", err)
	}

	// Commit the .ahm/ state so the repo has a clean base to work from.
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "add workflow records with ref-backed layout")
	return root
}

func recordsRefCommitCount(t *testing.T, root string) string {
	t.Helper()
	return strings.TrimSpace(git(t, root, "rev-list", "--count", defaultRecordsRef))
}

func TestDryRunTaskCreateInRefModeWritesNothing(t *testing.T) {
	root := newRefBackedWorkflowRepo(t)
	countBefore := recordsRefCommitCount(t, root)

	_, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "create", "Preview Task")
	if code != 0 {
		t.Fatalf("dry-run task create exit code = %d, stderr = %s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "active", "002.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run created a task record: %v", err)
	}
	if got := recordsRefCommitCount(t, root); got != countBefore {
		t.Fatalf("dry-run changed records ref commits from %s to %s", countBefore, got)
	}
}

func TestStatusInRefModeReadsAhmRecordsAndLegacyExecPlanRefs(t *testing.T) {
	root := newRefBackedWorkflowRepo(t)

	stdout, stderr, code := runCLI(t, "--root", root, "--json", "status")
	if code != 0 {
		t.Fatalf("status exit code = %d, stderr = %s\n%s", code, stderr, stdout)
	}
	assertContainsAll(t, stdout, `"installed": true`)
	// Task, index, and ExecPlan reads follow the migrated .ahm/ paths, and a
	// legacy .agents/exec-plans reference in migrated task front matter still
	// resolves to the moved plan.
	assertNotContains(t, stdout,
		"task_dir_unreadable",
		"task_exec_plan_missing",
		"generated_index_missing",
		"generated_index_stale",
		"task_bucket_mismatch",
	)
}

func TestBuildTaskWorkCommitPromptIsSameForBothLayouts(t *testing.T) {
	// Both legacy (.agents/) and migrated (.ahm/) layouts keep source records
	// committed, so the commit prompt is the same: include both task files and
	// project source files.
	legacyRoot := t.TempDir()
	writeFile(t, filepath.Join(legacyRoot, ".agents", "ahm.json"), `{"version": "test", "strict_acceptance": false, "files": {}}`)
	legacyApp := app{opts: options{root: legacyRoot}}
	legacyPrompt := legacyApp.buildTaskWorkCommitPrompt("007")
	assertContainsAll(t, legacyPrompt, "Include both task files and project source files")

	migratedRoot := newRefBackedWorkflowRepo(t)
	migratedApp := app{opts: options{root: migratedRoot}}
	migratedPrompt := migratedApp.buildTaskWorkCommitPrompt("007")
	assertContainsAll(t, migratedPrompt, "Include both task files and project source files")
}
