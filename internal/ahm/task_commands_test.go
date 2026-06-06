package ahm

import (
	"errors"
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
		"status: Pending",
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
		labels:   "type:task, area:cli",
		status:   "Pending",
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

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskList("all", "completed"); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	assertContainsAll(t, got, "002 [Completed] P2 S Completed Task")
	assertNotContains(t, got, "001 [Pending]", "003 [Cancelled]")
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
		if err := a.taskList("all", ""); err != nil {
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
		if err := a.taskList("ready", ""); err != nil {
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

	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "First Task", "--priority", "P1", "--effort", "M", "--description", "First body")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Fatalf("create first stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Second Task")
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

	stdout, stderr, code = runCLI(t, "--root", root, "task", "cancel", "002")
	if code != 0 {
		t.Fatalf("cancel exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "002 -> Cancelled")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "cancelled", "002.md"), "status: Cancelled")

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
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "status: Pending")
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
	if len(captured.args) != 3 || captured.args[0] != "--output-format" || captured.args[1] != "text" {
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
		name       string
		executable string
		prefix     []string
	}{
		{name: "cake", executable: "cake", prefix: []string{"--output-format", "text"}},
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
			args := agent.args("prompt")
			for i, want := range tt.prefix {
				if args[i] != want {
					t.Fatalf("args = %#v, want prefix %#v", args, tt.prefix)
				}
			}
			if args[len(args)-1] != "prompt" {
				t.Fatalf("args = %#v, final arg should be prompt", args)
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

func writeTaskFileWithDeps(t *testing.T, path string, id string, title string, status string, deps string) {
	t.Helper()
	extra := "depends_on: " + deps + "\n"
	writeTaskFile(t, path, id, title, status, extra)
}
