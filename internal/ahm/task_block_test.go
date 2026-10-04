package ahm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskBlockRecordsReasonAndRef(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Blocked Work", "Pending", "")

	stdout, stderr, code := runCLI(t, "--root", root, "task", "block", "001",
		"--reason", "Waiting on the storage decision", "--ref", "https://example.com/issues/1")
	if code != 0 {
		t.Fatalf("block exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "001 -> Blocked")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"),
		"status: Blocked",
		"blocked_reason: Waiting on the storage decision",
		"blocked_ref: https://example.com/issues/1",
	)
}

func TestTaskBlockFromOpenStatus(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Untriaged", "Open", "")

	stdout, stderr, code := runCLI(t, "--root", root, "task", "block", "001", "--reason", "Needs a design decision")
	if code != 0 {
		t.Fatalf("block exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "001 -> Blocked")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"),
		"status: Blocked", "blocked_reason: Needs a design decision")
}

func TestTaskBlockRequiresReason(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Blocked Work", "Pending", "")

	stdout, stderr, code := runCLI(t, "--root", root, "task", "block", "001")
	if code != 2 {
		t.Fatalf("block without reason exit code = %d, want 2, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stderr, "task block requires --reason")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "status: Pending")
}

func TestTaskBlockForceDoesNotBypassMissingReason(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Blocked Work", "Pending", "")

	stdout, stderr, code := runCLI(t, "--root", root, "--force", "task", "block", "001")
	if code != 2 {
		t.Fatalf("force block without reason exit code = %d, want 2, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stderr, "task block requires --reason")
}

func TestTaskBlockRefusesTerminalStatus(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "completed", "001.md"), "001", "Done", "Completed", "")

	stdout, stderr, code := runCLI(t, "--root", root, "task", "block", "001", "--reason", "Nope")
	if code != 2 {
		t.Fatalf("block completed exit code = %d, want 2, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stderr, "cannot block task 001")
}

func TestTaskBlockRewritesReasonWhenAlreadyBlocked(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Blocked Work", "Blocked", "blocked_reason: Old reason\n")

	stdout, stderr, code := runCLI(t, "--root", root, "task", "block", "001", "--reason", "New reason")
	if code != 0 {
		t.Fatalf("re-block exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "001 -> Blocked")
	data, err := os.ReadFile(filepath.Join(root, ".ahm", "tasks", "active", "001.md"))
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, string(data), "blocked_reason: New reason")
	assertNotContains(t, string(data), "Old reason")
}

func TestTaskBlockRefusesNewlineInReason(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Blocked Work", "Pending", "")

	stdout, stderr, code := runCLI(t, "--root", root, "task", "block", "001", "--reason", "line one\nline two")
	if code != 2 {
		t.Fatalf("block with newline reason exit code = %d, want 2, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stderr, "must not contain a newline")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "status: Pending")
}

func TestTaskBlockRefusesNewlineInRef(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Blocked Work", "Pending", "")

	stdout, stderr, code := runCLI(t, "--root", root, "task", "block", "001", "--reason", "ok", "--ref", "a\nb")
	if code != 2 {
		t.Fatalf("block with newline ref exit code = %d, want 2, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stderr, "must not contain a newline")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "status: Pending")
}

func TestTaskBlockFromInProgress(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "WIP", "In Progress", "")

	stdout, stderr, code := runCLI(t, "--root", root, "task", "block", "001", "--reason", "Paused for a decision")
	if code != 0 {
		t.Fatalf("block exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "001 -> Blocked")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"),
		"status: Blocked", "blocked_reason: Paused for a decision")
}

func TestTaskBlockedListShowsReason(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Stuck", "Blocked",
		"blocked_reason: Waiting on the decision\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "002", "Dep", "Pending", "")
	writeTaskFileWithDeps(t, filepath.Join(root, ".ahm", "tasks", "active", "003.md"), "003", "Dep Blocked", "Pending", "002")
	// 004 is Blocked with no recorded reason (written by hand or an older
	// release), so the list shows the fallback reason text.
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "004.md"), "004", "No Reason", "Blocked", "")

	stdout, stderr, code := runCLI(t, "--root", root, "task", "blocked")
	if code != 0 {
		t.Fatalf("task blocked exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"001 [Blocked] P2 S Stuck",
		"reason: Waiting on the decision",
		"003 [Pending] P2 S Dep Blocked",
		"reason: waiting on 002",
		"004 [Blocked] P2 S No Reason",
		"reason: no reason recorded",
	)
}

func TestTaskBlockReasonSurvivesPartialDependencyCompletion(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Dep A", "Pending", "")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "002", "Dep B", "Pending", "")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "003.md"), "003", "Multi", "Blocked",
		"depends_on: 001, 002\nblocked_reason: Waiting on deps\n")

	stdout, stderr, code := runCLI(t, "--root", root, "task", "complete", "001", "--force")
	if code != 0 {
		t.Fatalf("complete exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "001 -> Completed")
	assertNotContains(t, stdout, "003 -> Pending")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "003.md"),
		"status: Blocked", "blocked_reason: Waiting on deps")
}

func TestTaskBlockDryRunReportsReasonWithoutWriting(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Blocked Work", "Pending", "")

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "block", "001",
		"--reason", "Waiting", "--ref", "https://example.com/1")
	if code != 0 {
		t.Fatalf("dry-run block exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "move: ", "status: Blocked", "reason: Waiting", "ref: https://example.com/1")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "status: Pending")
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "active", "001.md")); err != nil {
		t.Fatal(err)
	}
}

func TestTaskUnblockClearsReason(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Blocked Work", "Blocked",
		"blocked_reason: Waiting on the decision\nblocked_ref: https://example.com/1\n")

	stdout, stderr, code := runCLI(t, "--root", root, "task", "unblock", "001")
	if code != 0 {
		t.Fatalf("unblock exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "001 -> Pending")
	data, err := os.ReadFile(filepath.Join(root, ".ahm", "tasks", "active", "001.md"))
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, string(data), "status: Pending")
	assertNotContains(t, string(data), "blocked_reason", "blocked_ref")
}

func TestTaskUnblockRequiresBlocked(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Pending Work", "Pending", "")

	stdout, stderr, code := runCLI(t, "--root", root, "task", "unblock", "001")
	if code != 2 {
		t.Fatalf("unblock pending exit code = %d, want 2, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stderr, "cannot unblock task 001", "not Blocked")
}

func TestStatusTransitionFromBlockedClearsReason(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Blocked Work", "Blocked",
		"blocked_reason: Waiting\n")

	stdout, stderr, code := runCLI(t, "--root", root, "task", "accept", "001")
	if code != 0 {
		t.Fatalf("accept exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "001 -> Pending")
	data, err := os.ReadFile(filepath.Join(root, ".ahm", "tasks", "active", "001.md"))
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, string(data), "status: Pending")
	assertNotContains(t, string(data), "blocked_reason")
}

func TestTaskCompleteAutoUnblockClearsReason(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Dependency", "Pending", "")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "002", "Dependent", "Blocked",
		"depends_on: 001\nblocked_reason: Waiting on 001\n")

	stdout, stderr, code := runCLI(t, "--root", root, "task", "complete", "001", "--force")
	if code != 0 {
		t.Fatalf("complete exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "001 -> Completed", "002 -> Pending")
	data, err := os.ReadFile(filepath.Join(root, ".ahm", "tasks", "active", "002.md"))
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, string(data), "status: Pending")
	assertNotContains(t, string(data), "blocked_reason")
}

func TestValidationReportsBlockedMissingReason(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "No Reason", "Blocked", "")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "002", "With Reason", "Blocked",
		"blocked_reason: Waiting on a decision\n")

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	_ = a.doctor()
	got := out.String()
	assertContainsAll(t, got,
		`"code": "task_blocked_missing_reason"`,
		"task 001 is Blocked with no blocked_reason",
	)
	// Task 002 records a reason, so it must not warn.
	assertNotContains(t, got, "task 002 is Blocked with no blocked_reason")
}

func TestPrimeNamesBlockedTasksWithReasons(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Dep", "Pending", "")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "002", "Stuck", "Blocked",
		"blocked_reason: Waiting on the storage decision\nblocked_ref: https://example.com/1\n")
	writeTaskFileWithDeps(t, filepath.Join(root, ".ahm", "tasks", "active", "003.md"), "003", "Dep Blocked", "Pending", "001")
	indexer := app{opts: options{root: root}, out: &strings.Builder{}}
	if err := indexer.writeIndexes(); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "--root", root, "prime")
	if code != 0 {
		t.Fatalf("prime exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"## Blocked",
		"002 [Blocked] P2 S Stuck",
		"reason: Waiting on the storage decision (https://example.com/1)",
		"003 [Pending] P2 S Dep Blocked",
		"reason: waiting on 001",
	)

	jsonOut, stderr, code := runCLI(t, "--root", root, "--json", "prime")
	if code != 0 {
		t.Fatalf("prime --json exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, jsonOut,
		`"blocked_tasks":`,
		`"reason": "Waiting on the storage decision (https://example.com/1)"`,
		`"reason": "waiting on 001"`,
	)
}
