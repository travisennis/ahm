package ahm

import (
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
