package ahm

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskStatusAndCompleteRoundTripWithCRLF(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	// Write a task with CRLF line endings.
	path := filepath.Join(root, ".agents", ".tasks", "active", "098.md")
	content := "---\r\n" +
		"id: 098\r\n" +
		"title: CRLF Completer\r\n" +
		"status: Pending\r\n" +
		"priority: P3\r\n" +
		"effort: XS\r\n" +
		"labels: type:test, area:workflow\r\n" +
		"exec_plan: -\r\n" +
		"depends_on: -\r\n" +
		"---\r\n" +
		"# CRLF Completer\r\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Run task status (which reads and parses the task).
	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatus([]string{"098"}, "Completed"); err != nil {
		t.Fatal(err)
	}

	// Verify the task was moved to completed and parsed correctly.
	completedPath := filepath.Join(root, ".agents", ".tasks", "completed", "098.md")
	if _, err := os.Stat(completedPath); err != nil {
		t.Fatalf("completed task not found: %v", err)
	}
}

func TestTaskCreateAllowsFlagsAfterTitle(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Smoke", "task", "--description", "Verify task creation", "--priority", "P1")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Fatalf("create stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	content := mustRead(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"))
	assertContainsAll(t, content,
		"title: Smoke task",
		"priority: P1",
		"created: ",
		"Verify task creation",
	)
}

func TestTaskCreateBodyFile(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	bodyPath := filepath.Join(root, "body.md")
	body := "## Problem\n\nThings are broken.\n\n## Acceptance Notes\n\n- [ ] Fix things\n"
	if err := os.WriteFile(bodyPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Body File Task",
		"--priority", "P1", "--effort", "M", "--labels", "type:feature, area:cli", "--body-file", bodyPath)
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Fatalf("create stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}

	content := mustRead(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"))
	// Front matter and ID allocation unchanged.
	assertContainsAll(t, content,
		"id: 001",
		"title: Body File Task",
		"status: Open",
		"priority: P1",
		"effort: M",
		"labels: type:feature, area:cli",
		"exec_plan: -",
		"depends_on: -",
		"created: ",
		"# Body File Task",
		"## Problem",
		"Things are broken.",
		"- [ ] Fix things",
	)
	// The default placeholder body should be replaced.
	assertNotContains(t, content, "## Summary\n\nTODO.")

	// Index regenerated to include the new task.
	indexContent := mustRead(t, filepath.Join(root, ".agents", ".tasks", "active", "index.md"))
	assertContainsAll(t, indexContent, "Body File Task")
}

func TestTaskCreateBodyFileFromStdin(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	body := "## Problem\n\nPiped via stdin.\n"
	var out strings.Builder
	a := app{opts: options{root: root}, out: &out, in: strings.NewReader(body)}
	parsed := taskCreateArgs{
		title:    "Stdin Body Task",
		priority: "P2",
		effort:   "S",
		labels:   "type:task, area:unknown",
		status:   "Open",
		bodyFile: "-",
	}
	if err := a.taskCreateParsed(parsed); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "001" {
		t.Fatalf("create stdout = %q, want 001", out.String())
	}

	content := mustRead(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"))
	assertContainsAll(t, content, "# Stdin Body Task", "Piped via stdin.")
	assertNotContains(t, content, "## Summary\n\nTODO.")
}

func TestTaskCreateBodyFileErrors(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	t.Run("unreadable file", func(t *testing.T) {
		missing := filepath.Join(root, "does-not-exist.md")
		_, stderr, code := runCLI(t, "--root", root, "task", "create", "Missing Body", "--body-file", missing)
		if code != 1 {
			t.Fatalf("exit code = %d, stderr = %s", code, stderr)
		}
		if !strings.Contains(stderr, "reading task body from") {
			t.Fatalf("stderr = %q, want reading task body error", stderr)
		}
	})

	t.Run("conflict with description", func(t *testing.T) {
		bodyPath := filepath.Join(root, "conflict.md")
		if err := os.WriteFile(bodyPath, []byte("body\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, stderr, code := runCLI(t, "--root", root, "task", "create", "Conflict",
			"--description", "summary", "--body-file", bodyPath)
		if code != 2 {
			t.Fatalf("exit code = %d, stderr = %s", code, stderr)
		}
		if !strings.Contains(stderr, "--body-file or --description") {
			t.Fatalf("stderr = %q, want conflict error", stderr)
		}
	})

	t.Run("empty file", func(t *testing.T) {
		bodyPath := filepath.Join(root, "empty.md")
		if err := os.WriteFile(bodyPath, []byte("   \n\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, stderr, code := runCLI(t, "--root", root, "task", "create", "Empty Body", "--body-file", bodyPath)
		if code != 2 {
			t.Fatalf("exit code = %d, stderr = %s", code, stderr)
		}
		if !strings.Contains(stderr, "is empty") {
			t.Fatalf("stderr = %q, want empty body error", stderr)
		}
	})
}

func TestTaskCreateRejectsUnsupportedEnums(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "status",
			args: []string{"Smoke task", "--status", "Doing"},
			want: `unsupported task status "Doing"`,
		},
		{
			name: "priority",
			args: []string{"Smoke task", "--priority", "P5"},
			want: `unsupported task priority "P5"`,
		},
		{
			name: "effort",
			args: []string{"Smoke task", "--effort", "XXL"},
			want: `unsupported task effort "XXL"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			stdout, stderr, code := runCLI(t, "--root", root, "init")
			if code != 0 {
				t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
			}

			_, stderr, code = runCLI(t, append([]string{"--root", root, "task", "create"}, tt.args...)...)
			if code != 2 {
				t.Fatalf("exit code = %d, stderr = %s", code, stderr)
			}
			if !strings.Contains(stderr, tt.want) {
				t.Fatalf("stderr = %q, want %q", stderr, tt.want)
			}
		})
	}
}

func TestTaskStatusPreservesOptionalFrontMatter(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	writeTaskFile(t, path, "001", "Preserve Metadata", "Pending", "depends_on: []\n"+
		"created: 2026-05-01\n"+
		"updated: 2026-05-02\n"+
		"parent: 000\n"+
		"external_ref: gh-123\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatus([]string{"001"}, "Completed"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".agents", ".tasks", "completed", "001.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	// created is preserved; updated is overwritten with current timestamp.
	for _, want := range []string{
		"created: 2026-05-01",
		"parent: 000",
		"external_ref: gh-123",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("rewritten task missing %q:\n%s", want, content)
		}
	}
	if !strings.Contains(content, "updated: ") {
		t.Fatalf("rewritten task missing updated field:\n%s", content)
	}
	if strings.Contains(content, "2026-05-02") {
		t.Fatalf("rewritten task still has old updated value:\n%s", content)
	}
}

func TestTaskStatusPreservesUnknownFrontMatter(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	writeTaskFile(t, path, "001", "Unknown Fields", "Pending",
		"assignee: alice\n"+
			"due: 2026-06-01\n"+
			"tags: bug, urgent\n"+
			"ticket: JIRA-456\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatus([]string{"001"}, "Completed"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".agents", ".tasks", "completed", "001.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"assignee: alice",
		"due: 2026-06-01",
		"tags: bug, urgent",
		"ticket: JIRA-456",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("rewritten task missing unknown field %q:\n%s", want, content)
		}
	}
}

func TestTaskStatusNoOp(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	writeTaskFile(t, path, "001", "Already In Progress", "In Progress", "depends_on: -\n")

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatus([]string{"001"}, "In Progress"); err != nil {
		t.Fatal(err)
	}

	// File should still be in active, content unchanged.
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("file content changed on no-op status update:\nbefore: %s\nafter:  %s", before, after)
	}

	if !strings.Contains(out.String(), "already In Progress") {
		t.Fatalf("output missing no-op message: %q", out.String())
	}
}

func TestTaskCompleteRepairsBucketWhenStatusAlreadyMatches(t *testing.T) {
	root := t.TempDir()
	// Task has Completed status but sits in active bucket.
	oldPath := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	writeTaskFile(t, oldPath, "001", "Already Completed", "Completed", "depends_on: -\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatus([]string{"001"}, "Completed"); err != nil {
		t.Fatal(err)
	}

	// Task should be moved to completed bucket.
	completedPath := filepath.Join(root, ".agents", ".tasks", "completed", "001.md")
	if _, err := os.Stat(completedPath); err != nil {
		t.Fatalf("completed task not found: %v", err)
	}
	// Old file should be removed.
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old file should be removed, err = %v", err)
	}
	// Output should use the move message, not the no-op message.
	if !strings.Contains(out.String(), "001 -> Completed") {
		t.Fatalf("output missing move message: %q", out.String())
	}
}

func TestTaskCancelRepairsBucketWhenStatusAlreadyMatches(t *testing.T) {
	root := t.TempDir()
	// Task has Cancelled status but sits in active bucket.
	oldPath := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	writeTaskFile(t, oldPath, "001", "Already Cancelled", "Cancelled", "depends_on: -\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatusWithArgs(taskStatusArgs{ids: []string{"001"}, status: "Cancelled", reason: "No longer needed"}); err != nil {
		t.Fatal(err)
	}

	// Task should be moved to cancelled bucket.
	cancelledPath := filepath.Join(root, ".agents", ".tasks", "cancelled", "001.md")
	if _, err := os.Stat(cancelledPath); err != nil {
		t.Fatalf("cancelled task not found: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old file should be removed, err = %v", err)
	}
	if !strings.Contains(out.String(), "001 -> Cancelled") {
		t.Fatalf("output missing move message: %q", out.String())
	}
	assertFileContainsAll(t, cancelledPath, "## Cancellation Reason", "No longer needed")
}

func TestTaskCompleteDryRunOnBucketMismatch(t *testing.T) {
	root := t.TempDir()
	// Task has Completed status but sits in active bucket.
	oldPath := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	writeTaskFile(t, oldPath, "001", "Already Completed", "Completed", "depends_on: -\n")

	var out strings.Builder
	a := app{opts: options{root: root, dryRun: true}, out: &out}
	if err := a.taskStatus([]string{"001"}, "Completed"); err != nil {
		t.Fatal(err)
	}

	// File should still be in active (dry run).
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("file should still exist after dry run: %v", err)
	}
	completedPath := filepath.Join(root, ".agents", ".tasks", "completed", "001.md")
	if _, err := os.Stat(completedPath); !os.IsNotExist(err) {
		t.Fatalf("completed file should not exist after dry run, err = %v", err)
	}
}

func TestTaskStatusNoOpWhenBucketAndStatusMatch(t *testing.T) {
	root := t.TempDir()
	// Task is Completed and already in completed bucket — true no-op.
	path := filepath.Join(root, ".agents", ".tasks", "completed", "001.md")
	writeTaskFile(t, path, "001", "Truly Completed", "Completed", "depends_on: -\n")

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatus([]string{"001"}, "Completed"); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("file content changed on no-op status update:\nbefore: %s\nafter:  %s", before, after)
	}
	if !strings.Contains(out.String(), "already Completed") {
		t.Fatalf("output missing no-op message: %q", out.String())
	}
}

func TestFilterReadyAndBlockedTasks(t *testing.T) {
	tasks := []Task{
		{ID: "001", Status: "Completed", Priority: "P1"},
		{ID: "002", Status: "Pending", Priority: "P0", DependsOn: []string{"001"}},
		{ID: "003", Status: "Pending", Priority: "P2", DependsOn: []string{"004"}},
	}
	ready := filterTasks(tasks, "ready")
	if len(ready) != 1 || ready[0].ID != "002" {
		t.Fatalf("ready = %#v", ready)
	}
	blocked := filterTasks(tasks, "blocked")
	if len(blocked) != 1 || blocked[0].ID != "003" {
		t.Fatalf("blocked = %#v", blocked)
	}
}

func TestTaskListFiltersStatus(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Pending Task", "Pending", "")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "completed", "002.md"), "002", "Completed Task", "Completed", "")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "cancelled", "003.md"), "003", "Cancelled Task", "Cancelled", "")

	t.Run("single status", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskList("all", []string{"completed"}); err != nil {
			t.Fatal(err)
		}
		got := out.String()
		assertContainsAll(t, got, "002 [Completed] P2 S Completed Task")
		assertNotContains(t, got, "001 [Pending]", "003 [Cancelled]")
	})

	t.Run("multiple statuses", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskList("all", []string{"pending", "cancelled"}); err != nil {
			t.Fatal(err)
		}
		got := out.String()
		assertContainsAll(t, got, "001 [Pending] P2 S Pending Task", "003 [Cancelled] P2 S Cancelled Task")
		assertNotContains(t, got, "002 [Completed]")
	})

	t.Run("normalization applies per entry", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskList("all", []string{"PENDING", "CANCELLED"}); err != nil {
			t.Fatal(err)
		}
		got := out.String()
		assertContainsAll(t, got, "001 [Pending] P2 S Pending Task", "003 [Cancelled] P2 S Cancelled Task")
		assertNotContains(t, got, "002 [Completed]")
	})

	t.Run("duplicate statuses are deduplicated", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskList("all", []string{"pending", "Pending"}); err != nil {
			t.Fatal(err)
		}
		got := out.String()
		assertContainsAll(t, got, "001 [Pending]")
		assertNotContains(t, got, "002 [Completed]", "003 [Cancelled]")
	})

	t.Run("invalid status returns error", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		err := a.taskList("all", []string{"pending", "bogus"})
		if err == nil {
			t.Fatal("expected error for invalid status")
		}
		if !strings.Contains(err.Error(), "unsupported task status") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty string in comma tokens returns error", func(t *testing.T) {
		// Simulate what happens when --status pending, is used (trailing comma)
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		err := a.taskList("all", []string{"pending", ""})
		if err == nil {
			t.Fatal("expected error for empty status")
		}
		if !strings.Contains(err.Error(), "unsupported task status") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestTaskNextShowsHighestPriorityReadyTask(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "completed", "001.md"), "001", "Done", "Completed", "")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "P2 Ready", "Pending", "depends_on: 001\n")
	writeTaskFileWithPriority(t, filepath.Join(root, ".agents", ".tasks", "active", "003.md"), "003", "P1 Ready", "Pending", "P1", "")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "004.md"), "004", "Blocked", "Pending", "depends_on: 999\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskNext(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	assertContainsAll(t, got, "003 [Pending] P1 S P1 Ready")
	assertNotContains(t, got, "002 [Pending]", "004 [Pending]")
}

func TestTaskCommandsResilientToMalformedTasks(t *testing.T) {
	root := t.TempDir()
	// Valid task
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Valid Task", "Pending", "")
	// Malformed task: invalid enum value "Doing"
	malformedPath := filepath.Join(root, ".agents", ".tasks", "active", "002.md")
	malformedContent := "---\n" +
		"id: 002\n" +
		"title: Bad Task\n" +
		"status: Doing\n" +
		"priority: P2\n" +
		"effort: S\n" +
		"labels: type:bug\n" +
		"exec_plan: -\n" +
		"---\n" +
		"# Bad Task\n"
	if err := os.MkdirAll(filepath.Dir(malformedPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(malformedPath, []byte(malformedContent), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("task list skips malformed task with warning", func(t *testing.T) {
		var out, errBuf strings.Builder
		a := app{opts: options{root: root}, out: &out, err: &errBuf}
		if err := a.taskList("all", nil); err != nil {
			t.Fatal(err)
		}
		got := out.String()
		if !strings.Contains(got, "001 [Pending]") {
			t.Fatalf("expected 001 in output:\n%s", got)
		}
		if strings.Contains(got, "002 [Doing]") {
			t.Fatalf("malformed task should not appear in list:\n%s", got)
		}
		if !strings.Contains(errBuf.String(), "warning:") {
			t.Fatalf("expected stderr warning, got: %q", errBuf.String())
		}
	})

	t.Run("task ready skips malformed task", func(t *testing.T) {
		var out, errBuf strings.Builder
		a := app{opts: options{root: root}, out: &out, err: &errBuf}
		if err := a.taskList("ready", nil); err != nil {
			t.Fatal(err)
		}
		got := out.String()
		if !strings.Contains(got, "001 [Pending]") {
			t.Fatalf("expected 001 in output:\n%s", got)
		}
		if !strings.Contains(errBuf.String(), "warning:") {
			t.Fatalf("expected stderr warning, got: %q", errBuf.String())
		}
	})

	t.Run("task next skips malformed task", func(t *testing.T) {
		var out, errBuf strings.Builder
		a := app{opts: options{root: root}, out: &out, err: &errBuf}
		if err := a.taskNext(); err != nil {
			t.Fatal(err)
		}
		got := out.String()
		if !strings.Contains(got, "001 [Pending]") {
			t.Fatalf("expected 001 in output:\n%s", got)
		}
		if !strings.Contains(errBuf.String(), "warning:") {
			t.Fatalf("expected stderr warning, got: %q", errBuf.String())
		}
	})

	t.Run("resolveTask finds valid task despite malformed others", func(t *testing.T) {
		var errBuf strings.Builder
		a := app{opts: options{root: root}, err: &errBuf}
		task, err := a.resolveTask("001")
		if err != nil {
			t.Fatal(err)
		}
		if task.ID != "001" {
			t.Fatalf("id = %q", task.ID)
		}
		if !strings.Contains(errBuf.String(), "warning:") {
			t.Fatalf("expected stderr warning, got: %q", errBuf.String())
		}
	})

	t.Run("resolveTask returns not-found for malformed task", func(t *testing.T) {
		var errBuf strings.Builder
		a := app{opts: options{root: root}, err: &errBuf}
		_, err := a.resolveTask("002")
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("expected not-found for malformed task, got: %v", err)
		}
	})

	t.Run("index regenerates despite malformed task", func(t *testing.T) {
		var out, errBuf strings.Builder
		a := app{opts: options{root: root}, out: &out, err: &errBuf}
		if err := a.writeIndexes(); err != nil {
			t.Fatal(err)
		}
		indexPath := filepath.Join(root, ".agents", ".tasks", "index.md")
		data, err := os.ReadFile(indexPath)
		if err != nil {
			t.Fatal(err)
		}
		got := string(data)
		if !strings.Contains(got, "001.md) | Valid Task") {
			t.Fatalf("expected 001 in index:\n%s", got)
		}
	})

	t.Run("task dep tree works with malformed task", func(t *testing.T) {
		writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "005.md"), "005", "Dep Parent", "Pending", "depends_on: 001\n")

		var out, errBuf strings.Builder
		a := app{opts: options{root: root}, out: &out, err: &errBuf}
		if err := a.taskDepTree([]string{"005"}); err != nil {
			t.Fatal(err)
		}
		got := out.String()
		if !strings.Contains(got, "005 [Pending] Dep Parent") {
			t.Fatalf("expected dep tree with 005:\n%s", got)
		}
		if !strings.Contains(got, "001 [Pending] Valid Task") {
			t.Fatalf("expected dep tree with 001:\n%s", got)
		}
		// Should print exactly one warning, not two (no double collectTasks call)
		warnCount := strings.Count(errBuf.String(), "warning:")
		if warnCount != 1 {
			t.Fatalf("expected exactly 1 warning, got %d: %q", warnCount, errBuf.String())
		}
	})

	t.Run("task dep cycles works with malformed task", func(t *testing.T) {
		// Add cycle between valid tasks
		writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "003.md"), "003", "Cycle A", "Pending", "depends_on: 004\n")
		writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "004.md"), "004", "Cycle B", "Pending", "depends_on: 003\n")

		var out, errBuf strings.Builder
		a := app{opts: options{root: root}, out: &out, err: &errBuf}
		if err := a.taskDepCycles(); err != nil {
			t.Fatal(err)
		}
		got := out.String()
		if !strings.Contains(got, "003 -> 004 -> 003") {
			t.Fatalf("cycle output = %q", got)
		}
		if !strings.Contains(errBuf.String(), "warning:") {
			t.Fatalf("expected stderr warning, got: %q", errBuf.String())
		}
	})
}

func TestMainTaskLifecycleAndDependencyIntegration(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// First task: explicitly Pending so lifecycle integration works
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "First Task", "--priority", "P1", "--effort", "M", "--description", "First body", "--status", "Pending")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Fatalf("create first stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	// Second task: explicitly Pending so lifecycle integration works
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Second Task", "--status", "Pending")
	if code != 0 || strings.TrimSpace(stdout) != "002" {
		t.Fatalf("create second stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "dep", "add", "002", "001")
	if code != 0 {
		t.Fatalf("dep add exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "002 depends_on: 001")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "depends_on: 001")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "blocked")
	if code != 0 {
		t.Fatalf("blocked exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "002 [Pending]")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "start", "001")
	if code != 0 {
		t.Fatalf("start exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "001 -> In Progress")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "complete", "001")
	if code != 0 {
		t.Fatalf("complete exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "001 -> Completed")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "completed", "001.md"), "status: Completed")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "ready")
	if code != 0 {
		t.Fatalf("ready exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "002 [Pending]")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "reopen", "001")
	if code != 0 {
		t.Fatalf("reopen exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "001 -> Pending")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "status: Pending")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "dep", "tree", "002")
	if code != 0 {
		t.Fatalf("dep tree exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "002 [Pending] Second Task", "  001 [Pending] First Task")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "dep", "remove", "002", "001")
	if code != 0 {
		t.Fatalf("dep remove exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "002 depends_on: -")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "depends_on: -")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "next")
	if code != 0 {
		t.Fatalf("next exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "001 [Pending] P1 M First Task")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "cancel", "002", "--reason", "No longer needed")
	if code != 0 {
		t.Fatalf("cancel exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "002 -> Cancelled")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "cancelled", "002.md"), "status: Cancelled", "## Cancellation Reason", "No longer needed")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "list", "--status", "cancelled")
	if code != 0 {
		t.Fatalf("list --status exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "002 [Cancelled] P2 S Second Task")
	assertNotContains(t, stdout, "001 [Pending]")
}

func TestTaskCompleteRefusesIncompleteDependencies(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Dependency Task", "Pending", "")
	writeTaskFileWithDeps(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Main Task", "Pending", "001")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	err := a.taskStatus([]string{"002"}, "Completed")
	if err == nil {
		t.Fatal("expected error from completing task with incomplete dependency")
	}
	if !strings.Contains(err.Error(), "incomplete dependencies: 001") {
		t.Fatalf("error message = %q, want incomplete dependencies: 001", err.Error())
	}
	// Task file should not have been moved.
	if _, err := os.Stat(filepath.Join(root, ".agents", ".tasks", "completed", "002.md")); !os.IsNotExist(err) {
		t.Fatal("completed file should not exist after failed completion")
	}
}

func TestTaskCompleteSucceedsWithCompletedDependencies(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "completed", "001.md"), "001", "Completed Dep", "Completed", "")
	writeTaskFileWithDeps(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Main Task", "Pending", "001")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatus([]string{"002"}, "Completed"); err != nil {
		t.Fatal(err)
	}
	// Task should have been moved to completed.
	if _, err := os.Stat(filepath.Join(root, ".agents", ".tasks", "completed", "002.md")); err != nil {
		t.Fatalf("completed file should exist: %v", err)
	}
}

func TestTaskCompleteSucceedsWithNoDependencies(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Standalone Task", "Pending", "")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatus([]string{"001"}, "Completed"); err != nil {
		t.Fatal(err)
	}
	// Task should have been moved to completed.
	if _, err := os.Stat(filepath.Join(root, ".agents", ".tasks", "completed", "001.md")); err != nil {
		t.Fatalf("completed file should exist: %v", err)
	}
}

func TestTaskCompleteWarnsForIncompleteAcceptanceByDefault(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Needs Acceptance")
	if code != 0 {
		t.Fatalf("create exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "complete", "001")
	if code != 0 {
		t.Fatalf("complete exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "001 -> Completed")
	assertContainsAll(t, stderr, "warning: task 001 acceptance notes still contain the TODO placeholder")
}

func TestTaskCancelRequiresReason(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "No Reason")
	if code != 0 {
		t.Fatalf("create exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "cancel", "001")
	if code == 0 {
		t.Fatalf("expected cancel without reason to fail, stdout = %s, stderr = %s", stdout, stderr)
	}
	assertContainsAll(t, stderr, "task cancel requires --reason")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "status: Open")
	if _, err := os.Stat(filepath.Join(root, ".agents", ".tasks", "cancelled", "001.md")); !os.IsNotExist(err) {
		t.Fatal("cancelled file should not exist after missing reason failure")
	}
}

func TestTaskCancelForceDoesNotBypassMissingReason(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "No Reason")
	if code != 0 {
		t.Fatalf("create exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "--force", "task", "cancel", "001")
	if code == 0 {
		t.Fatalf("expected force cancel without reason to fail, stdout = %s, stderr = %s", stdout, stderr)
	}
	assertContainsAll(t, stderr, "task cancel requires --reason")
}

func TestTaskCancelPersistsReason(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Cancelled Task", "--status", "Pending")
	if code != 0 {
		t.Fatalf("create exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "cancel", "001", "--reason", "Superseded by 002")
	if code != 0 {
		t.Fatalf("cancel exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "001 -> Cancelled")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "cancelled", "001.md"),
		"status: Cancelled",
		"## Cancellation Reason\n\nSuperseded by 002",
	)
}

func TestTaskCancelDryRunShowsReasonWithoutWriting(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Dry Run Cancel")
	if code != 0 {
		t.Fatalf("create exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "--dry-run", "task", "cancel", "001", "--reason", "Superseded")
	if code != 0 {
		t.Fatalf("dry-run cancel exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "move: ", ".agents/.tasks/cancelled/001.md", "status: Cancelled", "reason: Superseded")
	if _, err := os.Stat(filepath.Join(root, ".agents", ".tasks", "cancelled", "001.md")); !os.IsNotExist(err) {
		t.Fatal("cancelled file should not exist after dry-run cancellation")
	}
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "status: Open")
}

func TestTaskCancelWarnsForSeededAcceptanceTODO(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Needs Acceptance")
	if code != 0 {
		t.Fatalf("create exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "cancel", "001", "--reason", "Obsolete")
	if code != 0 {
		t.Fatalf("cancel exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stderr, "warning: task 001 acceptance notes still contain the TODO placeholder")
}

func TestTaskCancelReplacesExistingReason(t *testing.T) {
	body := "## Summary\n\nOld work.\n\n## Cancellation Reason\n\nOld reason.\n\n## Notes\n\nKeep this."
	got := upsertCancellationReason(body, "New reason.")
	assertContainsAll(t, got,
		"## Summary\n\nOld work.",
		"## Cancellation Reason\n\nNew reason.\n\n## Notes",
		"## Notes\n\nKeep this.",
	)
	assertNotContains(t, got, "Old reason.")
}

func TestTaskCompleteStrictAcceptanceBlocksIncompleteNotes(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	meta, err := readMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	meta.StrictAcceptance = true
	if err := writeMetadata(root, meta); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Needs Acceptance")
	if code != 0 {
		t.Fatalf("create exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "complete", "001")
	if code == 0 {
		t.Fatalf("expected strict completion failure, stdout = %s, stderr = %s", stdout, stderr)
	}
	assertContainsAll(t, stderr,
		"warning: task 001 acceptance notes still contain the TODO placeholder",
		"cannot complete task 001: acceptance notes are incomplete; use --force to override",
	)
	if _, err := os.Stat(filepath.Join(root, ".agents", ".tasks", "completed", "001.md")); !os.IsNotExist(err) {
		t.Fatal("completed file should not exist after strict acceptance failure")
	}
}

func TestTaskCompleteForceOverridesStrictAcceptance(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	meta, err := readMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	meta.StrictAcceptance = true
	if err := writeMetadata(root, meta); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Needs Acceptance")
	if code != 0 {
		t.Fatalf("create exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "--force", "task", "complete", "001")
	if code != 0 {
		t.Fatalf("force complete exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "001 -> Completed")
	assertContainsAll(t, stderr, "warning: task 001 acceptance notes still contain the TODO placeholder")
}

func TestTaskCompleteDryRunPreservesPreviewWithAcceptanceWarning(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Needs Acceptance")
	if code != 0 {
		t.Fatalf("create exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "--dry-run", "task", "complete", "001")
	if code != 0 {
		t.Fatalf("dry-run complete exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "move: ", ".agents/.tasks/completed/001.md", "status: Completed")
	assertContainsAll(t, stderr, "warning: task 001 acceptance notes still contain the TODO placeholder")
	if _, err := os.Stat(filepath.Join(root, ".agents", ".tasks", "completed", "001.md")); !os.IsNotExist(err) {
		t.Fatal("completed file should not exist after dry-run completion")
	}
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "status: Open")
}

func TestTaskCompleteRefusesIncompleteDepsIntegration(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Dependency")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Fatalf("create first stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Main")
	if code != 0 || strings.TrimSpace(stdout) != "002" {
		t.Fatalf("create second stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}

	// Make 002 depend on 001.
	_, stderr, code = runCLI(t, "--root", root, "task", "dep", "add", "002", "001")
	if code != 0 {
		t.Fatalf("dep add exit code = %d, stderr = %s", code, stderr)
	}

	// Try completing 002 while 001 is still pending.
	_, stderr, code = runCLI(t, "--root", root, "task", "complete", "002")
	if code == 0 {
		t.Fatal("expected non-zero exit from completing task with pending dependency")
	}
	if !strings.Contains(stderr, "incomplete dependencies: 001") {
		t.Fatalf("stderr = %q, want incomplete dependencies: 001", stderr)
	}
	// Verify 002 was not moved to completed.
	if _, err := os.Stat(filepath.Join(root, ".agents", ".tasks", "completed", "002.md")); !os.IsNotExist(err) {
		t.Fatal("completed file should not exist after failed completion")
	}

	// Now complete 001 and verify 002 can be completed.
	_, stderr, code = runCLI(t, "--root", root, "task", "complete", "001")
	if code != 0 {
		t.Fatalf("complete 001 exit code = %d, stderr = %s", code, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "complete", "002")
	if code != 0 {
		t.Fatalf("complete 002 exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "002 -> Completed")
}

func TestTaskWorkDefaultsToCakeAndMarksPendingInProgress(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Workable Task", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/" + executable, nil
	})
	var captured taskWorkCapture
	stubTaskWorkRunner(t, captured.runner)

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001")
	if code != 0 {
		t.Fatalf("task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	if captured.root != root {
		t.Fatalf("runner root = %q, want %q", captured.root, root)
	}
	if captured.executable != "/stub/cake" {
		t.Fatalf("runner executable = %q, want /stub/cake", captured.executable)
	}
	if len(captured.args) != 3 || captured.args[0] != "--output-format" || captured.args[1] != "stream-json" {
		t.Fatalf("cake args = %#v", captured.args)
	}
	assertContainsAll(t, captured.args[2], "Work on task 001.", ".agents/.tasks/active/001.md", "Do not commit or push")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "status: In Progress")
}

func TestTaskWorkAgentConfigAndFlagPrecedence(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeMetadata(root, metadata{DefaultWorkAgent: "codex"}); err != nil {
		t.Fatal(err)
	}
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Configured Task", "In Progress", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/" + executable, nil
	})

	var configured taskWorkCapture
	stubTaskWorkRunner(t, configured.runner)
	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001")
	if code != 0 {
		t.Fatalf("configured task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	if configured.executable != "/stub/codex" || len(configured.args) != 2 || configured.args[0] != "exec" {
		t.Fatalf("configured invocation executable=%q args=%#v", configured.executable, configured.args)
	}

	var flagged taskWorkCapture
	stubTaskWorkRunner(t, flagged.runner)
	stdout, stderr, code = runCLI(t, "--root", root, "task", "work", "001", "--agent", "cursor")
	if code != 0 {
		t.Fatalf("flagged task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	if flagged.executable != "/stub/cursor-agent" {
		t.Fatalf("flagged executable = %q, want /stub/cursor-agent", flagged.executable)
	}
	if len(flagged.args) != 4 || flagged.args[0] != "-p" || flagged.args[1] != "--output-format" || flagged.args[2] != "text" {
		t.Fatalf("cursor args = %#v", flagged.args)
	}
}

func TestTaskWorkUnsupportedAgentIsUsageError(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Workable Task", "Pending", "")

	_, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--agent", "unknown")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2; stderr = %s", code, stderr)
	}
	assertContainsAll(t, stderr, `unsupported task work agent "unknown"`, "supported: cake, codex, cursor")
}

func TestTaskWorkUnsupportedConfiguredAgentIsUsageError(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeMetadata(root, metadata{DefaultWorkAgent: "unknown"}); err != nil {
		t.Fatal(err)
	}
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Workable Task", "Pending", "")

	_, stderr, code := runCLI(t, "--root", root, "task", "work", "001")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2; stderr = %s", code, stderr)
	}
	assertContainsAll(t, stderr, `unsupported task work agent "unknown"`, "supported: cake, codex, cursor")
}

func TestTaskWorkDryRunPreviewsWithoutMutatingOrInvoking(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Workable Task", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/" + executable, nil
	})
	stubTaskWorkRunner(t, func(root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		t.Fatal("runner should not be called during dry-run")
		return nil
	})

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "work", "001", "--agent", "codex")
	if code != 0 {
		t.Fatalf("dry-run task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "agent: codex", "executable: /stub/codex", "status: In Progress", "task: 001", "exec", "Work on task 001.")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "status: Pending")
}

func TestTaskWorkRepairsBucketForPendingTaskInWrongBucket(t *testing.T) {
	root := t.TempDir()
	// Task has Pending status but sits in completed bucket.
	oldPath := filepath.Join(root, ".agents", ".tasks", "completed", "001.md")
	writeTaskFile(t, oldPath, "001", "Misplaced Pending", "Pending", "depends_on: -\n")

	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/" + executable, nil
	})
	var captured taskWorkCapture
	stubTaskWorkRunner(t, captured.runner)

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001")
	if code != 0 {
		t.Fatalf("task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Task should be in active bucket now.
	activePath := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	if _, err := os.Stat(activePath); err != nil {
		t.Fatalf("active task not found after work: %v", err)
	}
	assertFileContainsAll(t, activePath, "status: In Progress")

	// Old file should be removed.
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old file should be removed, err = %v", err)
	}
}

func TestTaskWorkDryRunOnBucketMismatch(t *testing.T) {
	root := t.TempDir()
	// Task has Pending status but sits in completed bucket.
	oldPath := filepath.Join(root, ".agents", ".tasks", "completed", "001.md")
	writeTaskFile(t, oldPath, "001", "Misplaced Pending", "Pending", "depends_on: -\n")

	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/" + executable, nil
	})
	stubTaskWorkRunner(t, func(root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		t.Fatal("runner should not be called during dry-run")
		return nil
	})

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "work", "001", "--agent", "codex")
	if code != 0 {
		t.Fatalf("dry-run task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// File should still be in completed (dry run).
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("file should still exist after dry run: %v", err)
	}
	activePath := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	if _, err := os.Stat(activePath); !os.IsNotExist(err) {
		t.Fatalf("active file should not exist after dry run, err = %v", err)
	}
}

func TestTaskWorkRefusesCompletedAndCancelledTasks(t *testing.T) {
	for _, tt := range []struct {
		status string
		bucket string
	}{
		{status: "Completed", bucket: "completed"},
		{status: "Cancelled", bucket: "cancelled"},
	} {
		t.Run(tt.status, func(t *testing.T) {
			root := t.TempDir()
			writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", tt.bucket, "001.md"), "001", "Closed Task", tt.status, "")
			stubTaskWorkLookPath(t, func(executable string) (string, error) {
				t.Fatalf("LookPath should not be called for %s task", tt.status)
				return "", nil
			})

			_, stderr, code := runCLI(t, "--root", root, "task", "work", "001")
			if code == 0 {
				t.Fatalf("expected failure for %s task", tt.status)
			}
			assertContainsAll(t, stderr, "cannot work task 001: status is "+tt.status)
		})
	}
}

func TestTaskWorkRefusesIncompleteDependencies(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Dependency", "Pending", "")
	writeTaskFileWithDeps(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Main", "Pending", "001")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		t.Fatal("LookPath should not be called for incomplete dependencies")
		return "", nil
	})

	_, stderr, code := runCLI(t, "--root", root, "task", "work", "002")
	if code == 0 {
		t.Fatal("expected dependency failure")
	}
	assertContainsAll(t, stderr, "cannot work task 002: incomplete dependencies: 001")
}

func TestTaskWorkMissingExecutableLeavesPendingTaskUnchanged(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Workable Task", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "", errors.New("missing")
	})

	_, stderr, code := runCLI(t, "--root", root, "task", "work", "001")
	if code == 0 {
		t.Fatal("expected missing executable failure")
	}
	assertContainsAll(t, stderr, `cannot work task 001 with cake: executable "cake" not found on PATH`)
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "status: Pending")
}

func TestTaskWorkAgentInvocations(t *testing.T) {
	for _, tt := range []struct {
		name             string
		executable       string
		prefix           []string
		supportsSessions bool
		supportsReview   bool
	}{
		{name: "cake", executable: "cake", prefix: []string{"--output-format", "stream-json"}, supportsSessions: true, supportsReview: true},
		{name: "codex", executable: "codex", prefix: []string{"exec"}},
		{name: "cursor", executable: "cursor-agent", prefix: []string{"-p", "--output-format", "text"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			agent, err := parseTaskWorkAgent(tt.name)
			if err != nil {
				t.Fatal(err)
			}
			if agent.executable != tt.executable {
				t.Fatalf("executable = %q, want %q", agent.executable, tt.executable)
			}
			if agent.supportsSessions != tt.supportsSessions {
				t.Fatalf("supportsSessions = %v, want %v", agent.supportsSessions, tt.supportsSessions)
			}
			if agent.supportsReview != tt.supportsReview {
				t.Fatalf("supportsReview = %v, want %v", agent.supportsReview, tt.supportsReview)
			}
			args := agent.args("prompt")
			for i, want := range tt.prefix {
				if args[i] != want {
					t.Fatalf("args = %#v, want prefix %#v", args, tt.prefix)
				}
			}
			if args[len(args)-1] != "prompt" {
				t.Fatalf("args = %#v, final arg should be prompt", args)
			}
			// Verify session methods are set only for session-capable agents.
			if agent.supportsSessions {
				if agent.resumeArgs == nil {
					t.Fatal("session-capable agent must have resumeArgs")
				}
				if agent.parseSessionID == nil {
					t.Fatal("session-capable agent must have parseSessionID")
				}
			}
			// Verify review methods are set only for review-capable agents.
			if agent.supportsReview {
				if agent.reviewArgs == nil {
					t.Fatal("review-capable agent must have reviewArgs")
				}
				if agent.parseReviewFeedback == nil {
					t.Fatal("review-capable agent must have parseReviewFeedback")
				}
			}
		})
	}
}

type taskWorkCapture struct {
	root       string
	executable string
	args       []string
}

func (c *taskWorkCapture) runner(root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	c.root = root
	c.executable = executable
	c.args = append([]string(nil), args...)
	return nil
}

func stubTaskWorkLookPath(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	orig := taskWorkLookPath
	taskWorkLookPath = fn
	t.Cleanup(func() {
		taskWorkLookPath = orig
	})
}

func stubTaskWorkRunner(t *testing.T, fn func(string, string, []string, io.Reader, io.Writer, io.Writer) error) {
	t.Helper()
	orig := taskWorkRunCommand
	taskWorkRunCommand = fn
	t.Cleanup(func() {
		taskWorkRunCommand = orig
	})
}

func TestTaskWorkCakeSessionCapture(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Task With Session", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})
	// Stub the runner to produce valid cake stream-json output.
	stubTaskWorkRunner(t, func(root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		if len(args) < 3 || args[0] != "--output-format" || args[1] != "stream-json" {
			t.Fatalf("unexpected args = %#v", args)
		}
		fmt.Fprintln(stdout, `{"event":"task_start","session_id":"sess_abc123","task_id":"tsk_xyz"}`)
		fmt.Fprintln(stdout, `{"event":"message","content":"Working..."}`)
		fmt.Fprint(stdout, `{"event":"task_complete","result":"Work completed.","session_id":"sess_abc123"}`)
		return nil
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001")
	if code != 0 {
		t.Fatalf("task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	// The result text should be printed to stdout (through MultiWriter).
	assertContainsAll(t, stdout, "session_id", "sess_abc123", "event", "task_start")
	// The session started message should appear on stderr.
	assertContainsAll(t, stderr, "cake session started: sess_abc")
	// The session warning should not appear.
	assertNotContains(t, stderr, "could not capture session ID")
	assertNotContains(t, stderr, "no session ID returned")
	// Task should be marked In Progress.
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "status: In Progress")
}

func TestTaskWorkCakeSessionParseInvalidJSON(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Session Parse Failure", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})
	// Stub the runner to produce invalid JSON (non-JSON line).
	stubTaskWorkRunner(t, func(root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		_, err := fmt.Fprint(stdout, `not json`)
		return err
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001")
	if code != 0 {
		t.Fatalf("task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	// Non-JSON lines are silently tolerated; no session ID means a warning.
	assertContainsAll(t, stderr, "warning: no session ID returned by cake")
}

func TestTaskWorkCakeSessionMissingID(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "No Session ID", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})
	// Stub the runner to produce stream-json without a task_start event.
	stubTaskWorkRunner(t, func(root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		fmt.Fprintln(stdout, `{"event":"message","content":"Done."}`)
		fmt.Fprint(stdout, `{"event":"task_complete","result":"Done.","session_id":""}`)
		return nil
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001")
	if code != 0 {
		t.Fatalf("task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	// Should warn about missing session ID but not fail.
	assertContainsAll(t, stderr, "warning: no session ID returned by cake")
}

func TestCakeSessionIDParsing(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		wantID    string
		wantError bool
	}{
		{name: "task_start first", output: `{"event":"task_start","session_id":"sess_xyz","task_id":"tsk_1"}
{"event":"task_complete","result":"ok"}`, wantID: "sess_xyz"},
		{name: "task_start second line", output: `{"event":"message","content":"hi"}
{"event":"task_start","session_id":"sess_abc","task_id":"tsk_2"}`, wantID: "sess_abc"},
		{name: "empty session", output: `{"event":"task_start","session_id":""}`, wantID: ""},
		{name: "non-task_start", output: `{"event":"message","content":"ok"}`, wantID: ""},
		{name: "non-JSON line", output: `not json`, wantID: ""},
		{name: "empty output", output: ``, wantID: ""},
		{name: "no event field", output: `{"session_id":"sess_xyz"}`, wantID: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := parseCakeSessionID([]byte(tt.output))
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if id != tt.wantID {
				t.Fatalf("sessionID = %q, want %q", id, tt.wantID)
			}
		})
	}
}

func TestCakeResumeArgs(t *testing.T) {
	args := cakeResumeArgs("sess_abc", "Continue working")
	want := []string{"--resume", "sess_abc", "--output-format", "stream-json", "Continue working"}
	if len(args) != len(want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}

func TestTaskWorkNonSessionAgentStreamsDirectly(t *testing.T) {
	// Non-session agents (codex, cursor) should stream stdout directly
	// rather than going through the session capture path.
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Non-Session Work", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/codex", nil
	})
	// Capture that the runner is called directly (not wrapped by session capture).
	var captured taskWorkCapture
	stubTaskWorkRunner(t, captured.runner)

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--agent", "codex")
	if code != 0 {
		t.Fatalf("task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	if captured.executable != "/stub/codex" {
		t.Fatalf("executable = %q, want /stub/codex", captured.executable)
	}
}

func TestParseCakeReviewFeedback(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		wantResult string
		wantError  bool
	}{
		{name: "task_complete with result", output: `{"event":"message","content":"checking..."}
{"event":"task_complete","result":"Found 3 issues.","session_id":"sess_abc"}`, wantResult: "Found 3 issues."},
		{name: "empty result", output: `{"event":"task_complete","result":""}`, wantResult: ""},
		{name: "no task_complete", output: `{"event":"message","content":"ok"}`, wantResult: ""},
		{name: "non-JSON line", output: `not json`, wantResult: ""},
		{name: "empty output", output: ``, wantResult: ""},
		{name: "last task_complete wins", output: `{"event":"task_complete","result":"First"}
{"event":"task_complete","result":"Final"}`, wantResult: "Final"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseCakeReviewFeedback([]byte(tt.output))
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.wantResult {
				t.Fatalf("result = %q, want %q", result, tt.wantResult)
			}
		})
	}
}

func TestTaskWorkReviewArgs(t *testing.T) {
	agent, err := parseTaskWorkAgent("cake")
	if err != nil {
		t.Fatal(err)
	}
	if !agent.supportsReview {
		t.Fatal("cake should support review")
	}
	args := agent.reviewArgs("Review the changes.")
	want := []string{"--no-session", "--skills", "deslop", "--output-format", "stream-json", "Review the changes."}
	if len(args) != len(want) {
		t.Fatalf("reviewArgs = %#v, want %#v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("reviewArgs[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}

func TestTaskWorkReviewOrchestration(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Reviewable", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})

	// Record all invocations of the runner so we can verify order.
	type invocation struct {
		root       string
		executable string
		args       []string
		hasStdin   bool
	}
	var invocations []invocation
	stubTaskWorkRunner(t, func(root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		// On the first call (session work), produce valid session JSON.
		if len(invocations) == 0 {
			fmt.Fprintln(stdout, `{"event":"task_start","session_id":"sess_review123","task_id":"tsk_xyz"}`)
			_, err := fmt.Fprint(stdout, `{"event":"task_complete","result":"Work done."}`)
			invocations = append(invocations, invocation{root, executable, args, stdin != nil})
			return err
		}
		// On the second call (review), produce review feedback JSON.
		if len(invocations) == 1 {
			_, err := fmt.Fprint(stdout, `{"event":"task_complete","result":"Found 2 style issues and 1 missing test."}`)
			invocations = append(invocations, invocation{root, executable, args, stdin != nil})
			return err
		}
		// On the third call (resume with feedback), return success.
		_, err := fmt.Fprint(stdout, `{"event":"task_complete","result":"Addressed feedback."}`)
		invocations = append(invocations, invocation{root, executable, args, stdin != nil})
		return err
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--review")
	if code != 0 {
		t.Fatalf("task work with review exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Should have 3 invocations: session work, review, resume.
	if len(invocations) != 3 {
		t.Fatalf("expected 3 invocations (session, review, resume), got %d", len(invocations))
	}

	// First invocation: session work with --output-format stream-json.
	if invocations[0].args[0] != "--output-format" || invocations[0].args[1] != "stream-json" {
		t.Fatalf("first invocation args = %#v, want session work prefix", invocations[0].args)
	}
	// First invocation should have stdin (user input passed through).
	if !invocations[0].hasStdin {
		t.Fatal("first invocation should have stdin connected")
	}

	// Second invocation: review with --no-session --skills deslop.
	if invocations[1].args[0] != "--no-session" || invocations[1].args[1] != "--skills" || invocations[1].args[2] != "deslop" {
		t.Fatalf("second invocation args = %#v, want review prefix", invocations[1].args)
	}
	// Review invocation should have no stdin (independent run).
	if invocations[1].hasStdin {
		t.Fatal("review invocation should have no stdin")
	}

	// Third invocation: resume with the session ID.
	if len(invocations[2].args) < 4 || invocations[2].args[0] != "--resume" || invocations[2].args[1] != "sess_review123" {
		t.Fatalf("third invocation args = %#v, want resume prefix with session ID", invocations[2].args)
	}
	// Resume should contain the feedback in the prompt.
	lastArg := invocations[2].args[len(invocations[2].args)-1]
	if !strings.Contains(lastArg, "Found 2 style issues and 1 missing test.") {
		t.Fatalf("resume prompt should contain review feedback, got %q", lastArg)
	}

	// Review status messages should appear on stderr.
	assertContainsAll(t, stderr, "--- Running review ---")
	assertContainsAll(t, stderr, "Review produced feedback, applying to session")

	// Task should be marked In Progress.
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "status: In Progress")
}

func TestTaskWorkReviewEmptyFeedback(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Empty Review", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})

	var invocationCount int
	stubTaskWorkRunner(t, func(root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		invocationCount++
		if invocationCount == 1 {
			// Session work.
			fmt.Fprintln(stdout, `{"event":"task_start","session_id":"sess_empty","task_id":"tsk_xyz"}`)
			_, err := fmt.Fprint(stdout, `{"event":"task_complete","result":"Done."}`)
			return err
		}
		// Review produces empty feedback.
		_, err := fmt.Fprint(stdout, `{"event":"task_complete","result":""}`)
		return err
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--review")
	if code != 0 {
		t.Fatalf("task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Should have exactly 2 invocations: session work and review (no resume).
	if invocationCount != 2 {
		t.Fatalf("expected 2 invocations (session, review), got %d", invocationCount)
	}

	assertContainsAll(t, stderr, "--- Running review ---")
	assertContainsAll(t, stderr, "No review feedback to address, skipping feedback step.")
}

func TestTaskWorkReviewFails(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Failing Review", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})

	var invocationCount int
	stubTaskWorkRunner(t, func(root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		invocationCount++
		if invocationCount == 1 {
			fmt.Fprintln(stdout, `{"event":"task_start","session_id":"sess_fail","task_id":"tsk_xyz"}`)
			_, err := fmt.Fprint(stdout, `{"event":"task_complete","result":"Work."}`)
			return err
		}
		// Review fails with a non-zero exit.
		return fmt.Errorf("review command exited with status 1")
	})

	_, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--review")
	if code == 0 {
		t.Fatal("expected task work to fail when review fails")
	}

	assertContainsAll(t, stderr, "review failed:")
	assertContainsAll(t, stderr, "review command exited with status 1")
}

func TestTaskWorkReviewOnNonSessionAgent(t *testing.T) {
	// Non-session-capable agents (codex) should not get review orchestration
	// even when --review is set, because they don't have session capture.
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Codex No Review", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/codex", nil
	})

	var invocationCount int
	stubTaskWorkRunner(t, func(root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		invocationCount++
		return nil
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--agent", "codex", "--review")
	if code != 0 {
		t.Fatalf("task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Should have exactly 1 invocation (session work not supported, so direct stream).
	if invocationCount != 1 {
		t.Fatalf("expected 1 invocation (direct stream), got %d", invocationCount)
	}

	// Should not contain review messages.
	assertNotContains(t, stderr, "--- Running review ---")
	// But should warn that --review is unsupported.
	assertContainsAll(t, stderr, "warning: --review is not supported by agent codex")
}

func TestTaskWorkReviewParseError(t *testing.T) {
	// When the review command succeeds but produces non-JSON output,
	// the tolerant parser treats it as empty feedback and skips the
	// feedback-resume step (graceful degradation).
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Bad Review Output", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})

	var invocationCount int
	stubTaskWorkRunner(t, func(root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		invocationCount++
		if invocationCount == 1 {
			// Session work succeeds.
			fmt.Fprintln(stdout, `{"event":"task_start","session_id":"sess_parse","task_id":"tsk_xyz"}`)
			_, err := fmt.Fprint(stdout, `{"event":"task_complete","result":"Done."}`)
			return err
		}
		// Review produces non-JSON output (tolerated, treated as empty feedback).
		_, err := fmt.Fprint(stdout, `not valid json`)
		return err
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--review")
	if code != 0 {
		t.Fatalf("task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Should have 2 invocations: session work and review (no resume after empty feedback).
	if invocationCount != 2 {
		t.Fatalf("expected 2 invocations (session, review), got %d", invocationCount)
	}

	assertContainsAll(t, stderr, "--- Running review ---")
	assertContainsAll(t, stderr, "No review feedback to address, skipping feedback step.")
}

func TestTaskWorkReviewWithoutFlag(t *testing.T) {
	// Without --review, review should not run even for review-capable agents.
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "No Review", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})

	var invocationCount int
	stubTaskWorkRunner(t, func(root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		invocationCount++
		if invocationCount == 1 {
			fmt.Fprintln(stdout, `{"event":"task_start","session_id":"sess_noreview","task_id":"tsk_xyz"}`)
			_, err := fmt.Fprint(stdout, `{"event":"task_complete","result":"Done."}`)
			return err
		}
		return nil
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001")
	if code != 0 {
		t.Fatalf("task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Should have exactly 1 invocation (no review step).
	if invocationCount != 1 {
		t.Fatalf("expected 1 invocation (session only), got %d", invocationCount)
	}

	assertNotContains(t, stderr, "--- Running review ---")
}

func TestTaskWorkCompletionHandoff(t *testing.T) {
	// --complete should resume the session with a completion prompt after work.
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Completable", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})

	var invocations []struct {
		root       string
		executable string
		args       []string
	}
	stubTaskWorkRunner(t, func(root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		invocations = append(invocations, struct {
			root       string
			executable string
			args       []string
		}{root, executable, args})
		// First invocation: session work.
		if len(invocations) == 1 {
			fmt.Fprintln(stdout, `{"event":"task_start","session_id":"sess_complete123","task_id":"tsk_xyz"}`)
			_, err := fmt.Fprint(stdout, `{"event":"task_complete","result":"Work done."}`)
			return err
		}
		// Second invocation: resume with completion prompt.
		_, err := fmt.Fprint(stdout, `{"event":"task_complete","result":"Completed task."}`)
		return err
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--complete")
	if code != 0 {
		t.Fatalf("task work with --complete exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Should have 2 invocations: session work, completion resume.
	if len(invocations) != 2 {
		t.Fatalf("expected 2 invocations (session, completion), got %d", len(invocations))
	}

	// First invocation: session work args.
	if len(invocations[0].args) < 3 || invocations[0].args[0] != "--output-format" || invocations[0].args[1] != "stream-json" {
		t.Fatalf("first invocation args = %#v, want session work prefix", invocations[0].args)
	}

	// Second invocation: resume with session ID and completion prompt.
	args := invocations[1].args
	if len(args) < 4 || args[0] != "--resume" || args[1] != "sess_complete123" {
		t.Fatalf("second invocation args = %#v, want resume with session ID", args)
	}
	lastArg := args[len(args)-1]
	if !strings.Contains(lastArg, "Complete task 001") {
		t.Fatalf("completion prompt should mention task ID, got %q", lastArg)
	}
	if !strings.Contains(lastArg, "ahm task complete 001") {
		t.Fatalf("completion prompt should mention ahm task complete, got %q", lastArg)
	}

	// Status messages should appear on stderr.
	assertContainsAll(t, stderr, "--- Running completion handoff ---")

	// Task should be marked In Progress.
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "status: In Progress")
}

func TestTaskWorkCompletionOnNonSessionAgent(t *testing.T) {
	// Non-session-capable agents (codex) should get a warning with --complete.
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Codex No Complete", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/codex", nil
	})

	var invocationCount int
	stubTaskWorkRunner(t, func(root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		invocationCount++
		return nil
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--agent", "codex", "--complete")
	if code != 0 {
		t.Fatalf("task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Should have exactly 1 invocation (direct stream, no completion).
	if invocationCount != 1 {
		t.Fatalf("expected 1 invocation (direct stream), got %d", invocationCount)
	}

	// Should warn that --complete is unsupported.
	assertContainsAll(t, stderr, "warning: --complete is not supported by agent codex")
	// Should not contain completion messages.
	assertNotContains(t, stderr, "--- Running completion handoff ---")
}

func TestTaskWorkCompletionWithReview(t *testing.T) {
	// --review and --complete should run review first, then completion.
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Both Flags", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})

	type invocation struct {
		args     []string
		hasStdin bool
	}
	var invocations []invocation
	stubTaskWorkRunner(t, func(root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		inv := invocation{args: append([]string(nil), args...), hasStdin: stdin != nil}
		invocations = append(invocations, inv)
		switch len(invocations) {
		case 1:
			// Session work.
			fmt.Fprintln(stdout, `{"event":"task_start","session_id":"sess_both456","task_id":"tsk_xyz"}`)
			_, err := fmt.Fprint(stdout, `{"event":"task_complete","result":"Work."}`)
			return err
		case 2:
			// Review.
			_, err := fmt.Fprint(stdout, `{"event":"task_complete","result":"Minor issues found."}`)
			return err
		case 3:
			// Resume with review feedback.
			_, err := fmt.Fprint(stdout, `{"event":"task_complete","result":"Addressed."}`)
			return err
		default:
			// Resume with completion prompt.
			_, err := fmt.Fprint(stdout, `{"event":"task_complete","result":"Completed."}`)
			return err
		}
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--review", "--complete")
	if code != 0 {
		t.Fatalf("task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Should have 4 invocations: session, review, resume-feedback, completion.
	if len(invocations) != 4 {
		t.Fatalf("expected 4 invocations (session, review, resume, completion), got %d", len(invocations))
	}

	// First: session work.
	if len(invocations[0].args) < 3 || invocations[0].args[0] != "--output-format" || invocations[0].args[1] != "stream-json" {
		t.Fatalf("first invocation args = %#v, want session work", invocations[0].args)
	}
	if !invocations[0].hasStdin {
		t.Fatal("first invocation should have stdin")
	}

	// Second: review (no stdin).
	if invocations[1].args[0] != "--no-session" || invocations[1].args[1] != "--skills" || invocations[1].args[2] != "deslop" {
		t.Fatalf("second invocation args = %#v, want review prefix", invocations[1].args)
	}
	if invocations[1].hasStdin {
		t.Fatal("review invocation should have no stdin")
	}

	// Third: resume with feedback.
	if len(invocations[2].args) < 4 || invocations[2].args[0] != "--resume" || invocations[2].args[1] != "sess_both456" {
		t.Fatalf("third invocation args = %#v, want resume with session", invocations[2].args)
	}

	// Fourth: resume with completion prompt.
	if len(invocations[3].args) < 4 || invocations[3].args[0] != "--resume" || invocations[3].args[1] != "sess_both456" {
		t.Fatalf("fourth invocation args = %#v, want resume with session", invocations[3].args)
	}
	lastArg := invocations[3].args[len(invocations[3].args)-1]
	if !strings.Contains(lastArg, "Complete task 001") {
		t.Fatalf("completion prompt should mention task ID, got %q", lastArg)
	}

	assertContainsAll(t, stderr, "--- Running review ---")
	assertContainsAll(t, stderr, "--- Running completion handoff ---")
}

func TestTaskWorkCompletionMissingSession(t *testing.T) {
	// When no session ID is captured, completion handoff should warn and skip.
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "No Session", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})

	var invocationCount int
	stubTaskWorkRunner(t, func(root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		invocationCount++
		if invocationCount == 1 {
			// Session work returns no session_id.
			_, err := fmt.Fprint(stdout, `{"result":"Done."}`)
			return err
		}
		t.Fatal("should not have additional invocations without a session ID")
		return nil
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--complete")
	if code != 0 {
		t.Fatalf("task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Only the session work invocation should happen.
	if invocationCount != 1 {
		t.Fatalf("expected 1 invocation (session only), got %d", invocationCount)
	}

	// Should warn about missing session ID, not about completion.
	assertContainsAll(t, stderr, "warning: no session ID returned by cake")
	assertNotContains(t, stderr, "--- Running completion handoff ---")
}

func TestTaskWorkCompletionFails(t *testing.T) {
	// When the completion handoff command fails, the error should be wrapped
	// with a descriptive prefix, matching the runReview error pattern.
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Failing Complete", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})

	var invocationCount int
	stubTaskWorkRunner(t, func(root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		invocationCount++
		if invocationCount == 1 {
			// Session work succeeds.
			fmt.Fprintln(stdout, `{"event":"task_start","session_id":"sess_failcomplete","task_id":"tsk_xyz"}`)
			_, err := fmt.Fprint(stdout, `{"event":"task_complete","result":"Work."}`)
			return err
		}
		// Completion handoff fails.
		return fmt.Errorf("exit status 1")
	})

	_, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--complete")
	if code == 0 {
		t.Fatal("expected task work to fail when completion handoff fails")
	}

	assertContainsAll(t, stderr, "completion handoff failed:")
	assertContainsAll(t, stderr, "exit status 1")
}

func TestTaskWorkCommitHandoff(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Committable", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})

	var invocations []struct {
		args []string
	}
	stubTaskWorkRunner(t, func(root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		invocations = append(invocations, struct {
			args []string
		}{append([]string(nil), args...)})
		if len(invocations) == 1 {
			fmt.Fprintln(stdout, `{"event":"task_start","session_id":"sess_commit123","task_id":"tsk_xyz"}`)
			_, err := fmt.Fprint(stdout, `{"event":"task_complete","result":"Work done."}`)
			return err
		}
		_, err := fmt.Fprint(stdout, `{"event":"task_complete","result":"Committed."}`)
		return err
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--commit")
	if code != 0 {
		t.Fatalf("task work with --commit exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	if len(invocations) != 2 {
		t.Fatalf("expected 2 invocations (session, commit), got %d", len(invocations))
	}
	args := invocations[1].args
	if len(args) < 4 || args[0] != "--resume" || args[1] != "sess_commit123" {
		t.Fatalf("second invocation args = %#v, want resume with session ID", args)
	}
	prompt := args[len(args)-1]
	assertContainsAll(t, prompt,
		"Commit the completed work for task 001",
		"Make sure the task is marked completed before committing",
		"Include both task files and project source files in a single commit",
		"Do not push or open a pull request",
	)
	assertContainsAll(t, stderr, "--- Running commit handoff ---")
}

func TestTaskWorkCommitWithReviewRunsLast(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Review Then Commit", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})

	var prompts []string
	stubTaskWorkRunner(t, func(root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		prompts = append(prompts, args[len(args)-1])
		switch len(prompts) {
		case 1:
			fmt.Fprintln(stdout, `{"event":"task_start","session_id":"sess_reviewcommit","task_id":"tsk_xyz"}`)
			_, err := fmt.Fprint(stdout, `{"event":"task_complete","result":"Work."}`)
			return err
		case 2:
			_, err := fmt.Fprint(stdout, `{"event":"task_complete","result":"Fix the docs."}`)
			return err
		case 3:
			_, err := fmt.Fprint(stdout, `{"event":"task_complete","result":"Feedback addressed."}`)
			return err
		default:
			_, err := fmt.Fprint(stdout, `{"event":"task_complete","result":"Committed."}`)
			return err
		}
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--review", "--commit")
	if code != 0 {
		t.Fatalf("task work with review+commit exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	if len(prompts) != 4 {
		t.Fatalf("expected 4 invocations (session, review, feedback, commit), got %d", len(prompts))
	}
	assertContainsAll(t, prompts[2], "Please address the following review feedback", "Fix the docs.")
	assertContainsAll(t, prompts[3], "Commit the completed work for task 001")
	assertContainsAll(t, stderr, "--- Running review ---", "--- Running commit handoff ---")
}

func TestTaskWorkCommitFails(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Failing Commit", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})

	var invocationCount int
	stubTaskWorkRunner(t, func(root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		invocationCount++
		if invocationCount == 1 {
			fmt.Fprintln(stdout, `{"event":"task_start","session_id":"sess_failcommit","task_id":"tsk_xyz"}`)
			_, err := fmt.Fprint(stdout, `{"event":"task_complete","result":"Work."}`)
			return err
		}
		return fmt.Errorf("exit status 1")
	})

	_, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--commit")
	if code == 0 {
		t.Fatal("expected task work to fail when commit handoff fails")
	}
	assertContainsAll(t, stderr, "commit handoff failed:", "exit status 1")
}

func TestTaskWorkCommitMissingSessionFails(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "No Commit Session", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})

	var invocationCount int
	stubTaskWorkRunner(t, func(root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		invocationCount++
		if invocationCount == 1 {
			_, err := fmt.Fprint(stdout, `{"result":"Work."}`)
			return err
		}
		t.Fatal("commit handoff should not run without a session ID")
		return nil
	})

	_, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--commit")
	if code == 0 {
		t.Fatal("expected task work to fail when commit handoff lacks a session ID")
	}
	assertContainsAll(t, stderr, "cannot run commit handoff: no session ID returned by cake")
}

func TestTaskWorkCompletionDryRun(t *testing.T) {
	// Dry run with --complete should preview the completion flag.
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Dry Complete", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})
	stubTaskWorkRunner(t, func(root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		t.Fatal("runner should not be called during dry-run")
		return nil
	})

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "work", "001", "--complete")
	if code != 0 {
		t.Fatalf("dry-run exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "complete: true")
	assertNotContains(t, stderr, "--- Running completion handoff ---")
	// Task should remain Pending.
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "status: Pending")
}

func TestTaskWorkCommitDryRun(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Dry Commit", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})
	stubTaskWorkRunner(t, func(root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		t.Fatal("runner should not be called during dry-run")
		return nil
	})

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "work", "001", "--review", "--commit")
	if code != 0 {
		t.Fatalf("dry-run exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "commit: true", "review: true")
	assertNotContains(t, stderr, "--- Running commit handoff ---")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "status: Pending")
}

func TestTaskWorkCompletionPromptContent(t *testing.T) {
	// Verify the completion prompt structure and content.
	agent, err := parseTaskWorkAgent("cake")
	if err != nil {
		t.Fatal(err)
	}
	if !agent.supportsSessions {
		t.Fatal("cake should support sessions")
	}

	// Build a minimal app to generate the prompt.
	a := &app{opts: options{root: "/tmp"}}
	prompt := a.buildTaskWorkCompletionPrompt("042")

	assertContainsAll(t, prompt,
		"Complete task 042",
		"Fill the task Acceptance Notes",
		"run the required verification",
		"ahm task complete 042",
		"Do not commit or push",
	)
}

func TestTaskWorkCommitPromptContent(t *testing.T) {
	a := &app{opts: options{root: "/tmp"}}
	prompt := a.buildTaskWorkCommitPrompt("042")

	assertContainsAll(t, prompt,
		"Commit the completed work for task 042",
		"Make sure the task is marked completed before committing",
		"Include both task files and project source files in a single commit",
		"Do not push or open a pull request",
	)
	assertNotContains(t, prompt, "Conventional Commit")
}

func TestTaskWorkCompletionDryRunWithReview(t *testing.T) {
	// Dry run with both --review and --complete previews both flags.
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Both Dry", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})
	stubTaskWorkRunner(t, func(root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		t.Fatal("runner should not be called during dry-run")
		return nil
	})

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "work", "001", "--review", "--complete")
	if code != 0 {
		t.Fatalf("dry-run exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "complete: true", "review: true")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "status: Pending")
}

func TestTaskAcceptMovesOpenToPending(t *testing.T) {
	root := t.TempDir()
	_, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", code, stderr)
	}

	// Create an Open task (new default).
	_, stderr, code = runCLI(t, "--root", root, "task", "create", "Needs Triage")
	if code != 0 {
		t.Fatalf("create exit code = %d, stderr = %s", code, stderr)
	}
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "status: Open")

	// Accept it.
	stdout, stderr, code := runCLI(t, "--root", root, "task", "accept", "001")
	if code != 0 {
		t.Fatalf("accept exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "001 -> Pending")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "status: Pending")
}

func TestTaskAcceptDryRunPreviews(t *testing.T) {
	root := t.TempDir()
	_, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", code, stderr)
	}

	_, stderr, code = runCLI(t, "--root", root, "task", "create", "Needs Triage")
	if code != 0 {
		t.Fatalf("create exit code = %d, stderr = %s", code, stderr)
	}

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "accept", "001")
	if code != 0 {
		t.Fatalf("dry-run accept exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "move: ", ".agents/.tasks/active/001.md", "status: Pending")
	// File should remain Open after dry-run.
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "status: Open")
}

func TestTaskAcceptFromBlocked(t *testing.T) {
	root := t.TempDir()
	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	// Create a Blocked task directly.
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Blocked Task", "Blocked", "")

	if err := a.taskStatus([]string{"001"}, "Pending"); err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, out.String(), "001 -> Pending")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "status: Pending")
}

func TestTaskAcceptNoOp(t *testing.T) {
	root := t.TempDir()
	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Already Pending", "Pending", "")

	if err := a.taskStatus([]string{"001"}, "Pending"); err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, out.String(), "001 already Pending")
}

func writeTaskFileWithDeps(t *testing.T, path string, id string, title string, status string, deps string) {
	t.Helper()
	extra := "depends_on: " + deps + "\n"
	writeTaskFile(t, path, id, title, status, extra)
}

func TestTaskCompleteWarnsOnCorruptMetadata(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	// Create a task with acceptance notes.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Test Task")
	if code != 0 {
		t.Fatalf("create exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Corrupt the metadata file.
	metaPath := filepath.Join(root, ".agents", "ahm.json")
	if err := os.WriteFile(metaPath, []byte("{invalid json}"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Task complete should warn about corrupt metadata but still succeed
	// (strict acceptance can't be determined).
	stdout, stderr, code = runCLI(t, "--root", root, "task", "complete", "001")
	if code != 0 {
		t.Fatalf("complete exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stderr, "corrupt workflow metadata .agents/ahm.json", "strict acceptance disabled")
	assertContainsAll(t, stdout, "001 -> Completed")
}

func TestTaskWorkWarnsOnCorruptMetadata(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	// Set a default work agent.
	meta, err := readMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	meta.DefaultWorkAgent = "codex"
	if err := writeMetadata(root, meta); err != nil {
		t.Fatal(err)
	}

	// Create a task.
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Workable", "Pending", "")

	// Corrupt the metadata file.
	metaPath := filepath.Join(root, ".agents", "ahm.json")
	if err := os.WriteFile(metaPath, []byte("{invalid json}"), 0o644); err != nil {
		t.Fatal(err)
	}

	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/" + executable, nil
	})

	var captured taskWorkCapture
	stubTaskWorkRunner(t, captured.runner)

	// Task work should warn about corrupt metadata but still use the default agent.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "work", "001")
	if code != 0 {
		t.Fatalf("task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stderr, "corrupt workflow metadata .agents/ahm.json", "using default agent")
	// Falls back to cake.
	if captured.executable != "/stub/cake" {
		t.Fatalf("executable = %q, want /stub/cake", captured.executable)
	}
}

func TestTaskCompleteRespectsStrictAcceptanceWithValidMetadata(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	// Enable strict acceptance.
	meta, err := readMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	meta.StrictAcceptance = true
	if err := writeMetadata(root, meta); err != nil {
		t.Fatal(err)
	}
	// Create a task.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Needs Strict")
	if code != 0 {
		t.Fatalf("create exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Task complete should block due to strict acceptance.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "complete", "001")
	if code == 0 {
		t.Fatalf("expected strict completion failure, code=%d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stderr, "cannot complete task 001: acceptance notes are incomplete")
}
