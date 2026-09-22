package ahm

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTaskStatusAndCompleteRoundTripWithCRLF(t *testing.T) {
	root := projectRoot(t)
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(); err != nil {
		t.Fatal(err)
	}

	// Write a task with CRLF line endings.
	path := filepath.Join(root, ".ahm", "tasks", "active", "098.md")
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
		t.Error(err)
	}

	// Verify the task was moved to completed and parsed correctly.
	completedPath := filepath.Join(root, ".ahm", "tasks", "completed", "098.md")
	if _, err := os.Stat(completedPath); err != nil {
		t.Errorf("completed task not found: %v", err)
	}
}

func TestTaskCreateAllowsFlagsAfterTitle(t *testing.T) {
	root := projectRoot(t)
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Smoke", "task", "--description", "Verify task creation", "--priority", "P1")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Errorf("create stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	content := mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"))
	assertContainsAll(t, content,
		"title: Smoke task",
		"priority: P1",
		"created: ",
		"Verify task creation",
	)
}

func TestTaskCreateParallelAllocatesUniqueIDs(t *testing.T) {
	root := projectRoot(t)
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(); err != nil {
		t.Fatal(err)
	}

	const creates = 5
	var wg sync.WaitGroup
	ids := make(chan string, creates)
	errs := make(chan error, creates)
	for i := 0; i < creates; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out strings.Builder
			a := app{opts: options{root: root}, out: &out}
			err := a.taskCreateParsed(taskCreateArgs{
				title:    fmt.Sprintf("Parallel Task %d", i+1),
				priority: "P2",
				effort:   "S",
				labels:   "type:task, area:unknown",
				status:   "Open",
			})
			if err != nil {
				errs <- err
				return
			}
			ids <- strings.TrimSpace(out.String())
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	var got []string
	for id := range ids {
		got = append(got, id)
	}
	sort.Strings(got)
	want := []string{"001", "002", "003", "004", "005"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("parallel task IDs = %v, want %v", got, want)
	}
	for _, id := range want {
		if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "active", id+".md")); err != nil {
			t.Errorf("task %s not written: %v", id, err)
		}
	}
	indexContent := mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "index.md"))
	for i := 0; i < creates; i++ {
		assertContainsAll(t, indexContent, fmt.Sprintf("Parallel Task %d", i+1))
	}
}

func TestTaskCreateWaitsForIDAllocationLock(t *testing.T) {
	oldRetryDelay := workflowLockRetryDelay
	oldTimeout := workflowLockTimeout
	workflowLockRetryDelay = time.Millisecond
	workflowLockTimeout = 2 * time.Second
	defer func() {
		workflowLockRetryDelay = oldRetryDelay
		workflowLockTimeout = oldTimeout
	}()

	root := projectRoot(t)
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(); err != nil {
		t.Fatal(err)
	}

	release, err := acquireWorkflowRecordLock(workflowPathsFor(root))
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	var out strings.Builder
	go func() {
		a := app{opts: options{root: root}, out: &out}
		done <- a.taskCreateParsed(taskCreateArgs{
			title:    "Created After Lock",
			priority: "P2",
			effort:   "S",
			labels:   "type:task, area:unknown",
			status:   "Open",
		})
	}()

	select {
	case err := <-done:
		t.Errorf("task create finished while workflow lock was held: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Existing Task", "Pending", "")
	if err := release(); err != nil {
		t.Error(err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(2 * time.Second):
		t.Error("task create did not finish after workflow lock was released")
	}
	if strings.TrimSpace(out.String()) != "002" {
		t.Errorf("create stdout = %q, want 002", out.String())
	}
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "title: Created After Lock")
}

func TestTaskCreateBodyFile(t *testing.T) {
	root := projectRoot(t)
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	bodyPath := filepath.Join(root, "body.md")
	body := "## Problem\n\nThings are broken.\n\n## Acceptance Notes\n\n- [ ] Fix things\n"
	if err := os.WriteFile(bodyPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Body File Task",
		"--priority", "P1", "--effort", "M", "--labels", "type:feature, area:cli", "--body-file", bodyPath)
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Errorf("create stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}

	content := mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"))
	// Front matter and ID allocation unchanged.
	assertContainsAll(t, content,
		"id: 001",
		"title: Body File Task",
		"status: Open",
		"priority: P1",
		"effort: M",
		"labels: type:feature, area:cli",
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
	indexContent := mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "index.md"))
	assertContainsAll(t, indexContent, "Body File Task")
}

func TestTaskCreateBodyFileFromStdin(t *testing.T) {
	root := projectRoot(t)
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(); err != nil {
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
		t.Error(err)
	}
	if strings.TrimSpace(out.String()) != "001" {
		t.Errorf("create stdout = %q, want 001", out.String())
	}

	content := mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"))
	assertContainsAll(t, content, "# Stdin Body Task", "Piped via stdin.")
	assertNotContains(t, content, "## Summary\n\nTODO.")
}

func TestTaskCreateBodyFileErrors(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	t.Run("unreadable file", func(t *testing.T) {
		missing := filepath.Join(root, "does-not-exist.md")
		_, stderr, code := runCLI(t, "--root", root, "task", "create", "Missing Body", "--body-file", missing)
		if code != 1 {
			t.Errorf("exit code = %d, stderr = %s", code, stderr)
		}
		if !strings.Contains(stderr, "reading task body from") {
			t.Errorf("stderr = %q, want reading task body error", stderr)
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
			t.Errorf("exit code = %d, stderr = %s", code, stderr)
		}
		if !strings.Contains(stderr, "--body-file or --description") {
			t.Errorf("stderr = %q, want conflict error", stderr)
		}
	})

	t.Run("empty file", func(t *testing.T) {
		bodyPath := filepath.Join(root, "empty.md")
		if err := os.WriteFile(bodyPath, []byte("   \n\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, stderr, code := runCLI(t, "--root", root, "task", "create", "Empty Body", "--body-file", bodyPath)
		if code != 2 {
			t.Errorf("exit code = %d, stderr = %s", code, stderr)
		}
		if !strings.Contains(stderr, "is empty") {
			t.Errorf("stderr = %q, want empty body error", stderr)
		}
	})
}

func TestTaskCreateBodyFileStripsDuplicateH1(t *testing.T) {
	root := projectRoot(t)
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Body file includes an H1 that matches the task title.
	// It should be stripped to avoid duplication since the CLI
	// always generates the H1 from front matter.
	bodyPath := filepath.Join(root, "body.md")
	body := "# Dedup Test\n## Problem\n\nBody content.\n"
	if err := os.WriteFile(bodyPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Dedup Test", "--body-file", bodyPath)
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Errorf("create stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}

	content := mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"))

	// Should have exactly one H1 heading (from renderTask).
	// Count occurrences of "# Dedup Test" in the file.
	h1Count := strings.Count(content, "# Dedup Test")
	if h1Count != 1 {
		t.Errorf("expected exactly 1 H1 %q, got %d:\n%s", "# Dedup Test", h1Count, content)
	}

	// Body content after the H1 should still be present.
	assertContainsAll(t, content, "## Problem", "Body content.")
}

func TestTaskCreateBodyFileStripsDuplicateH1WithLeadingBlanks(t *testing.T) {
	root := projectRoot(t)
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Body file has leading blank lines before the matching H1.
	bodyPath := filepath.Join(root, "body.md")
	body := "\n\n\n# Lead Blanks\n## Problem\n\nBody.\n"
	if err := os.WriteFile(bodyPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Lead Blanks", "--body-file", bodyPath)
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Errorf("create stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}

	content := mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"))
	h1Count := strings.Count(content, "# Lead Blanks")
	if h1Count != 1 {
		t.Errorf("expected exactly 1 H1 %q, got %d:\n%s", "# Lead Blanks", h1Count, content)
	}
	assertContainsAll(t, content, "## Problem", "Body.")
}

func TestTaskCreateBodyFilePreservesDifferentH1(t *testing.T) {
	root := projectRoot(t)
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Body file has a different H1 than the task title.
	// This is unusual but should be preserved — it's intentional content.
	bodyPath := filepath.Join(root, "body.md")
	body := "# Different Header\n## Problem\n\nBody.\n"
	if err := os.WriteFile(bodyPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "My Title", "--body-file", bodyPath)
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Errorf("create stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}

	content := mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"))
	// Both H1s should exist: the generated one from renderTask and the one from the body.
	// This is intentional — the body's H1 is different content, not a duplicate.
	assertContainsAll(t, content, "# My Title", "# Different Header", "## Problem", "Body.")
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
				t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
			}

			_, stderr, code = runCLI(t, append([]string{"--root", root, "task", "create"}, tt.args...)...)
			if code != 2 {
				t.Errorf("exit code = %d, stderr = %s", code, stderr)
			}
			if !strings.Contains(stderr, tt.want) {
				t.Errorf("stderr = %q, want %q", stderr, tt.want)
			}
		})
	}
}

func TestTaskCreateRejectsNewlines(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "title with newline",
			args: []string{"Smoke\ntask"},
			want: "task create title must not contain newlines",
		},
		{
			name: "title with CRLF",
			args: []string{"Smoke\r\ntask"},
			want: "task create title must not contain newlines",
		},
		{
			name: "labels with newline",
			args: []string{"Smoke task", "--labels", "type:task\nstatus: Completed"},
			want: "task create labels must not contain newlines",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			stdout, stderr, code := runCLI(t, "--root", root, "init")
			if code != 0 {
				t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
			}

			_, stderr, code = runCLI(t, append([]string{"--root", root, "task", "create"}, tt.args...)...)
			if code != 2 {
				t.Errorf("exit code = %d, stderr = %s", code, stderr)
			}
			if !strings.Contains(stderr, tt.want) {
				t.Errorf("stderr = %q, want %q", stderr, tt.want)
			}
		})
	}
}

func TestTaskCreateRejectsWhitespace(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "title with leading whitespace",
			args: []string{" Smoke task"},
			want: "task create title must not have leading or trailing whitespace",
		},
		{
			name: "title with trailing whitespace",
			args: []string{"Smoke task "},
			want: "task create title must not have leading or trailing whitespace",
		},
		{
			name: "labels with leading whitespace",
			args: []string{"Smoke task", "--labels", " type:task"},
			want: "task create labels must not have leading or trailing whitespace",
		},
		{
			name: "labels with trailing whitespace",
			args: []string{"Smoke task", "--labels", "type:task "},
			want: "task create labels must not have leading or trailing whitespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			stdout, stderr, code := runCLI(t, "--root", root, "init")
			if code != 0 {
				t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
			}

			_, stderr, code = runCLI(t, append([]string{"--root", root, "task", "create"}, tt.args...)...)
			if code != 2 {
				t.Errorf("exit code = %d, stderr = %s", code, stderr)
			}
			if !strings.Contains(stderr, tt.want) {
				t.Errorf("stderr = %q, want %q", stderr, tt.want)
			}
		})
	}
}

func TestTaskCreateCanonicalizesEmptyLabels(t *testing.T) {
	root := projectRoot(t)
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "No Labels", "--labels", "")
	if code != 0 {
		t.Errorf("exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	if strings.TrimSpace(stdout) != "001" {
		t.Errorf("stdout = %q, want 001", stdout)
	}
	content := mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"))
	if !strings.Contains(content, "labels: -") {
		t.Errorf("empty labels did not render as dash sentinel:\n%s", content)
	}
}

func TestTaskCreateSubtask(t *testing.T) {
	root := projectRoot(t)
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Create a parent task (a top-level numeric-only task).
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Parent Task", "--status", "Tracking")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Fatalf("create parent stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}

	// Create a child task under the parent.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Child A", "--parent", "001")
	if code != 0 {
		t.Fatalf("create child stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	childID := strings.TrimSpace(stdout)
	if childID != "001a" {
		t.Errorf("child id = %q, want %q", childID, "001a")
	}

	// Verify the child file exists with correct parent front matter.
	childPath := filepath.Join(root, ".ahm", "tasks", "active", "001a.md")
	content := mustRead(t, childPath)
	assertContainsAll(t, content, "id: 001a", "parent: 001", "title: Child A")

	// Create another child — should get next letter.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Child B", "--parent", "001")
	if code != 0 || strings.TrimSpace(stdout) != "001b" {
		t.Errorf("second child stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}

	// Index should include both children.
	indexContent := mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "index.md"))
	assertContainsAll(t, indexContent, "Parent Task", "Child A", "Child B")
}

func TestTaskCreateSubtaskParentNotFound(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	_, stderr, code = runCLI(t, "--root", root, "task", "create", "Orphan", "--parent", "999")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "not found") {
		t.Errorf("stderr = %q, want parent not found error", stderr)
	}
}

func TestTaskCreateSubtaskParentIsChildRejected(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Create a top-level task.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Parent", "--status", "Tracking")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Fatalf("create parent stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}

	// Create a child.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Child", "--parent", "001")
	if code != 0 || strings.TrimSpace(stdout) != "001a" {
		t.Fatalf("create child stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}

	// Try creating a subtask of the child — should be rejected.
	_, stderr, code = runCLI(t, "--root", root, "task", "create", "Grandchild", "--parent", "001a")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "is a child task") {
		t.Errorf("stderr = %q, want child task rejection", stderr)
	}
}

func TestTaskCreateSubtaskCollisionAvoidance(t *testing.T) {
	root := projectRoot(t)
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(); err != nil {
		t.Fatal(err)
	}

	// Manually create a child with letter 'c' to skip 'a', 'b'.
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001c.md"), "001c", "Existing Child C", "Pending", "parent: 001\n")

	// Also create a completed child with letter 'e' to prove scanning happens across buckets.
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "completed", "001e.md"), "001e", "Completed Child E", "Completed", "parent: 001\n")

	// Collect tasks and call nextChildTaskIDForPaths directly.
	tasks, err := collectTasksForPaths(workflowPathsFor(root))
	if err == nil {
		t.Log("collectTasksForPaths returned no error") // may warn but succeed
	}

	got, err := nextChildTaskIDForPaths(tasks, workflowPathsFor(root), "001")
	if err != nil {
		t.Fatalf("nextChildTaskIDForPaths: %v", err)
	}
	// 'c' and 'e' exist, so first available is 'a'.
	if got != "001a" {
		t.Errorf("nextChildTaskIDForPaths = %q, want %q", got, "001a")
	}
}

func TestTaskCreateSubtaskDryRun(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Create a parent.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Parent")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Fatalf("create parent stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}

	// Dry-run child creation.
	stdout, stderr, code = runCLI(t, "--root", root, "--dry-run", "task", "create", "Dry Child", "--parent", "001")
	if code != 0 {
		t.Fatalf("dry-run child stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	assertContainsAll(t, stdout, "001a")

	// File should not exist.
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "active", "001a.md")); err == nil {
		t.Errorf("dry-run should not create child file")
	}
}

func TestTaskCreateTopLevelUnchangedWithParentFlag(t *testing.T) {
	// Verify that not using --parent still produces top-level IDs.
	root := projectRoot(t)
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Normal Task")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Errorf("create stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}

	content := mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"))
	assertNotContains(t, content, "parent:")
}

func TestTaskCreateDependsOn(t *testing.T) {
	root := projectRoot(t)
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	for _, title := range []string{"Dep One", "Dep Two"} {
		stdout, stderr, code = runCLI(t, "--root", root, "task", "create", title)
		if code != 0 {
			t.Fatalf("create %q: exit code = %d, stdout = %s, stderr = %s", title, code, stdout, stderr)
		}
	}

	// Multiple IDs (comma-separated) land in depends_on front matter, sorted
	// and canonicalized regardless of input order.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Depends Task", "--depends-on", "2,1")
	if code != 0 || strings.TrimSpace(stdout) != "003" {
		t.Fatalf("create with depends-on stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	content := mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "003.md"))
	assertContainsAll(t, content, "id: 003", "depends_on: 001, 002", "title: Depends Task")

	// Duplicate patterns resolve to one dependency entry.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Deduped Deps", "--depends-on", "1,001,2")
	if code != 0 || strings.TrimSpace(stdout) != "004" {
		t.Fatalf("create deduped stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	content = mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "004.md"))
	assertContainsAll(t, content, "depends_on: 001, 002")
	if strings.Contains(content, "001, 001") || strings.Contains(content, "001, 002, 002") {
		t.Errorf("duplicate dependency IDs were not deduped:\n%s", content)
	}

	// A single short-form ID is canonicalized to the zero-padded ID.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Single Dep", "--depends-on", "1")
	if code != 0 || strings.TrimSpace(stdout) != "005" {
		t.Fatalf("create single dep stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	content = mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "005.md"))
	assertContainsAll(t, content, "depends_on: 001")

	// A task with no --depends-on still renders the dash sentinel.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "No Deps")
	if code != 0 || strings.TrimSpace(stdout) != "006" {
		t.Fatalf("create no deps stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	content = mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "006.md"))
	assertContainsAll(t, content, "depends_on: -")

	// Suffixed IDs are deduped by canonical ID too.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Parent Seven", "--status", "Tracking")
	if code != 0 || strings.TrimSpace(stdout) != "007" {
		t.Fatalf("create parent seven stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Child A", "--parent", "007")
	if code != 0 || strings.TrimSpace(stdout) != "007a" {
		t.Fatalf("create child a stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Suffixed Deps", "--depends-on", "7a,007a")
	if code != 0 || strings.TrimSpace(stdout) != "008" {
		t.Fatalf("create suffixed deps stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	content = mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "008.md"))
	assertContainsAll(t, content, "depends_on: 007a")
	if strings.Contains(content, "007a, 007a") {
		t.Errorf("suffixed duplicate dependency IDs were not deduped:\n%s", content)
	}
}

func TestTaskCreateDependsOnResolvesBeforeSelfCycleCheck(t *testing.T) {
	root := projectRoot(t)
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// 001 exists and 002 is the ID about to be allocated. The child 002a
	// (parent 002 missing) makes the short pattern "2" resolvable by prefix
	// matching, so it must resolve instead of being mistaken for a
	// self-reference to the allocated 002.
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Top", "Pending", "depends_on: -\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "002a.md"), "002a", "Child A", "Pending", "depends_on: -\nparent: 002\n")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Resolves", "--depends-on", "2")
	if code != 0 || strings.TrimSpace(stdout) != "002" {
		t.Fatalf("create with resolvable pattern stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	content := mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"))
	assertContainsAll(t, content, "id: 002", "depends_on: 002a")
}

func TestTaskCreateDependsOnRejectsMissing(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	_, stderr, code = runCLI(t, "--root", root, "task", "create", "Missing Dep", "--depends-on", "999")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "not found") {
		t.Errorf("stderr = %q, want missing dependency error", stderr)
	}
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "active", "001.md")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing dependency rejection should not create a task file: %v", err)
	}
}

func TestTaskCreateDependsOnRejectsSelfCycle(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// In a fresh repo the next ID is 001; depending on it is a self-cycle.
	_, stderr, code = runCLI(t, "--root", root, "task", "create", "Self Dep", "--depends-on", "001")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "cannot depend on itself") {
		t.Errorf("stderr = %q, want self-dependency cycle error", stderr)
	}
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "active", "001.md")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("cycle rejection should not create a task file: %v", err)
	}
}

func TestTaskCreateDependsOnRejectsDanglingCycle(t *testing.T) {
	root := projectRoot(t)
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Task 001 (hand-edited) already references the ID about to be allocated
	// (002). Creating 002 with a dependency back on 001 would close a cycle.
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Dangling", "Pending", "depends_on: 002\n")

	_, stderr, code = runCLI(t, "--root", root, "task", "create", "Cycler", "--depends-on", "001")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "would create a cycle") {
		t.Errorf("stderr = %q, want dependency cycle error", stderr)
	}
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "active", "002.md")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("cycle rejection should not create a task file: %v", err)
	}
}

func TestTaskCreateDependsOnRejectsAmbiguous(t *testing.T) {
	root := projectRoot(t)
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Two child tasks with no parent 001 make the short pattern "1" ambiguous,
	// while a top-level task keeps the allocated ID away from 001.
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001a.md"), "001a", "Child A", "Pending", "parent: 001\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001b.md"), "001b", "Child B", "Pending", "parent: 001\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "002", "Top Level", "Pending", "depends_on: -\n")

	_, stderr, code = runCLI(t, "--root", root, "task", "create", "Ambiguous Dep", "--depends-on", "1")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "ambiguous") {
		t.Errorf("stderr = %q, want ambiguous dependency error", stderr)
	}
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "active", "003.md")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("ambiguous rejection should not create a task file: %v", err)
	}
}

func TestTaskCreateDependsOnRejectsAmbiguousNotSelfCycle(t *testing.T) {
	root := projectRoot(t)
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Orphaned 002a/002b (parent 002 missing) make "2" ambiguous, even though
	// 002 is the ID about to be allocated; the ambiguity message must win over
	// the self-cycle fallback.
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Top", "Pending", "depends_on: -\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "002a.md"), "002a", "Child A", "Pending", "depends_on: -\nparent: 002\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "002b.md"), "002b", "Child B", "Pending", "depends_on: -\nparent: 002\n")

	_, stderr, code = runCLI(t, "--root", root, "task", "create", "Ambiguous Dep", "--depends-on", "2")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "ambiguous") {
		t.Errorf("stderr = %q, want ambiguous dependency error", stderr)
	}
	if strings.Contains(stderr, "cannot depend on itself") {
		t.Errorf("stderr = %q, ambiguous pattern mislabeled as self-cycle", stderr)
	}
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "active", "002.md")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("ambiguous rejection should not create a task file: %v", err)
	}
}

func TestTaskCreateDependsOnRejectsDuplicatedBucketID(t *testing.T) {
	root := projectRoot(t)
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// The same ID in two buckets makes the dependency ambiguous at write time.
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Active Dup", "Pending", "depends_on: -\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "completed", "001.md"), "001", "Completed Dup", "Completed", "depends_on: -\n")

	_, stderr, code = runCLI(t, "--root", root, "task", "create", "New Dep", "--depends-on", "001")
	// Repo-state conflict (duplicate ID) is a runtime error, not a usage error.
	if code != 1 {
		t.Errorf("exit code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "duplicated") {
		t.Errorf("stderr = %q, want duplicated dependency error", stderr)
	}
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "active", "002.md")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("duplicate rejection should not create a task file: %v", err)
	}
}

func TestTaskCreateDependsOnRejectsCompleted(t *testing.T) {
	root := projectRoot(t)
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "completed", "001.md"), "001", "Done Dep", "Completed", "depends_on: -\n")

	// The duplicate pattern (001,1) must still be rejected: status checks run
	// before dedupe skips a repeated ID.
	_, stderr, code = runCLI(t, "--root", root, "task", "create", "New Dep", "--depends-on", "001,1")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "cannot depend on completed task 001") {
		t.Errorf("stderr = %q, want completed dependency error", stderr)
	}
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "active", "002.md")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("completed dependency rejection should not create a task file: %v", err)
	}
}

func TestTaskCreateDependsOnRejectsCancelled(t *testing.T) {
	root := projectRoot(t)
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "cancelled", "001.md"), "001", "Cancelled Dep", "Cancelled", "depends_on: -\n")

	_, stderr, code = runCLI(t, "--root", root, "task", "create", "New Dep", "--depends-on", "001")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "cannot depend on cancelled task 001") {
		t.Errorf("stderr = %q, want cancelled dependency error", stderr)
	}
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "active", "002.md")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("cancelled dependency rejection should not create a task file: %v", err)
	}
}

func TestTaskCreateDependsOnRejectsEmptyPart(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	for _, value := range []string{"001,", ",001", "001,,002", " ", ","} {
		_, stderr, code = runCLI(t, "--root", root, "task", "create", "Bad Deps", "--depends-on", value)
		if code != 2 {
			t.Errorf("--depends-on %q: exit code = %d, stderr = %s", value, code, stderr)
		}
		if !strings.Contains(stderr, "comma-separated list of task IDs") {
			t.Errorf("--depends-on %q: stderr = %q, want list format error", value, stderr)
		}
	}
}

func TestTaskCreateDependsOnWithParent(t *testing.T) {
	root := projectRoot(t)
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Parent task (top-level) and a dependency task.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Parent Task", "--status", "Tracking")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Fatalf("create parent stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Dep Task")
	if code != 0 || strings.TrimSpace(stdout) != "002" {
		t.Fatalf("create dep stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}

	// A child task can also depend on other tasks.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Child Dep", "--parent", "001", "--depends-on", "002")
	if code != 0 || strings.TrimSpace(stdout) != "001a" {
		t.Fatalf("create child with dep stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	content := mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "001a.md"))
	assertContainsAll(t, content, "id: 001a", "parent: 001", "depends_on: 002")
}

func TestTaskCreateDependsOnDryRun(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Dep Task")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Fatalf("create dep stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}

	// Dry-run prints the planned path, ID, and depends_on without creating.
	stdout, stderr, code = runCLI(t, "--root", root, "--dry-run", "task", "create", "Dry Deps", "--depends-on", "001")
	if code != 0 {
		t.Fatalf("dry-run stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	assertContainsAll(t, stdout, "002", "001")
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "active", "002.md")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("dry-run should not create task file: %v", err)
	}

	// Dry-run still validates dependencies and creates nothing on failure.
	_, stderr, code = runCLI(t, "--root", root, "--dry-run", "task", "create", "Dry Bad", "--depends-on", "999")
	if code != 2 {
		t.Errorf("dry-run missing dep exit code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "not found") {
		t.Errorf("dry-run missing dep stderr = %q, want not found error", stderr)
	}
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "active", "002.md")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("dry-run validation failure should not create a task file: %v", err)
	}

	// JSON dry-run emits depends_on as an array.
	stdout, stderr, code = runCLI(t, "--root", root, "--json", "--dry-run", "task", "create", "Dry Deps JSON", "--depends-on", "001")
	if code != 0 {
		t.Fatalf("json dry-run stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	assertContainsAll(t, stdout, `"depends_on": [`, `"001"`, `"id": "002"`)
}

func TestTaskStatusPreservesOptionalFrontMatter(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, ".ahm", "tasks", "active", "001.md")
	writeTaskFile(t, path, "001", "Preserve Metadata", "Pending", "depends_on: []\n"+
		"created: 2026-05-01\n"+
		"updated: 2026-05-02\n"+
		"parent: 000\n"+
		"external_ref: gh-123\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatus([]string{"001"}, "Completed"); err != nil {
		t.Error(err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".ahm", "tasks", "completed", "001.md"))
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
			t.Errorf("rewritten task missing %q:\n%s", want, content)
		}
	}
	if !strings.Contains(content, "updated: ") {
		t.Errorf("rewritten task missing updated field:\n%s", content)
	}
	if strings.Contains(content, "2026-05-02") {
		t.Errorf("rewritten task still has old updated value:\n%s", content)
	}
}

func TestTaskStatusPreservesUnknownFrontMatter(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, ".ahm", "tasks", "active", "001.md")
	writeTaskFile(t, path, "001", "Unknown Fields", "Pending",
		"assignee: alice\n"+
			"due: 2026-06-01\n"+
			"tags: bug, urgent\n"+
			"ticket: JIRA-456\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatus([]string{"001"}, "Completed"); err != nil {
		t.Error(err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".ahm", "tasks", "completed", "001.md"))
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
			t.Errorf("rewritten task missing unknown field %q:\n%s", want, content)
		}
	}
}

func TestTaskStatusTransitionsDoNotDuplicateFormattedTitleH1(t *testing.T) {
	tests := []struct {
		name          string
		initial       string
		target        string
		initialBucket string
		targetBucket  string
		reason        string
	}{
		{name: "accept", initial: "Open", target: "Pending", initialBucket: "active", targetBucket: "active"},
		{name: "start", initial: "Pending", target: "In Progress", initialBucket: "active", targetBucket: "active"},
		{name: "complete", initial: "In Progress", target: "Completed", initialBucket: "active", targetBucket: "completed"},
		{name: "cancel", initial: "Pending", target: "Cancelled", initialBucket: "active", targetBucket: "cancelled", reason: "No longer needed"},
		{name: "reopen completed", initial: "Completed", target: "Pending", initialBucket: "completed", targetBucket: "active"},
		{name: "reopen cancelled", initial: "Cancelled", target: "Pending", initialBucket: "cancelled", targetBucket: "active"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := projectRoot(t)
			path := filepath.Join(root, ".ahm", "tasks", tt.initialBucket, "001.md")
			writeFormattedTitleTask(t, path, "001", "Fix ahm task accept", tt.initial)

			var out strings.Builder
			a := app{opts: options{root: root}, out: &out}
			err := a.taskStatusWithArgs(taskStatusArgs{
				ids:    []string{"001"},
				status: tt.target,
				reason: tt.reason,
			})
			if err != nil {
				t.Fatal(err)
			}

			updatedPath := filepath.Join(root, ".ahm", "tasks", tt.targetBucket, "001.md")
			assertTaskHasSinglePlainH1(t, updatedPath, "Fix ahm task accept")
		})
	}
}

func writeFormattedTitleTask(t *testing.T, path string, id string, title string, status string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\n" +
		"id: " + id + "\n" +
		"title: " + title + "\n" +
		"status: " + status + "\n" +
		"priority: P2\n" +
		"effort: S\n" +
		"labels: type:bug, area:tasks\n" +
		"exec_plan: -\n" +
		"depends_on: -\n" +
		"---\n" +
		"# Fix `ahm task accept`\n\n" +
		"## Summary\n\n" +
		"TODO.\n\n" +
		"## Acceptance Notes\n\n" +
		"- [x] Verified.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertTaskHasSinglePlainH1(t *testing.T, path string, title string) {
	t.Helper()
	updated := mustRead(t, path)
	h1Count := 0
	for _, line := range strings.Split(updated, "\n") {
		if strings.HasPrefix(line, "# ") {
			h1Count++
		}
	}
	if h1Count != 1 {
		t.Errorf("H1 heading count = %d, want 1:\n%s", h1Count, updated)
	}
	if got := strings.Count(updated, "# "+title); got != 1 {
		t.Errorf("plain task H1 count = %d, want 1:\n%s", got, updated)
	}
	if strings.Contains(updated, "# Fix `ahm task accept`") {
		t.Errorf("formatted duplicate H1 was preserved:\n%s", updated)
	}
}

func TestTaskStatusNoOp(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, ".ahm", "tasks", "active", "001.md")
	writeTaskFile(t, path, "001", "Already In Progress", "In Progress", "depends_on: -\n")

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatus([]string{"001"}, "In Progress"); err != nil {
		t.Error(err)
	}

	// File should still be in active, content unchanged.
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("file content changed on no-op status update:\nbefore: %s\nafter:  %s", before, after)
	}

	if !strings.Contains(out.String(), "already In Progress") {
		t.Errorf("output missing no-op message: %q", out.String())
	}
}

func TestTaskCompleteRepairsBucketWhenStatusAlreadyMatches(t *testing.T) {
	root := projectRoot(t)
	// Task has Completed status but sits in active bucket.
	oldPath := filepath.Join(root, ".ahm", "tasks", "active", "001.md")
	writeTaskFile(t, oldPath, "001", "Already Completed", "Completed", "depends_on: -\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatus([]string{"001"}, "Completed"); err != nil {
		t.Error(err)
	}

	// Task should be moved to completed bucket.
	completedPath := filepath.Join(root, ".ahm", "tasks", "completed", "001.md")
	if _, err := os.Stat(completedPath); err != nil {
		t.Errorf("completed task not found: %v", err)
	}
	// Old file should be removed.
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("old file should be removed, err = %v", err)
	}
	// Output should use the move message, not the no-op message.
	if !strings.Contains(out.String(), "001 -> Completed") {
		t.Errorf("output missing move message: %q", out.String())
	}
}

func TestTaskCancelRepairsBucketWhenStatusAlreadyMatches(t *testing.T) {
	root := projectRoot(t)
	// Task has Cancelled status but sits in active bucket.
	oldPath := filepath.Join(root, ".ahm", "tasks", "active", "001.md")
	writeTaskFile(t, oldPath, "001", "Already Cancelled", "Cancelled", "depends_on: -\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatusWithArgs(taskStatusArgs{ids: []string{"001"}, status: "Cancelled", reason: "No longer needed"}); err != nil {
		t.Error(err)
	}

	// Task should be moved to cancelled bucket.
	cancelledPath := filepath.Join(root, ".ahm", "tasks", "cancelled", "001.md")
	if _, err := os.Stat(cancelledPath); err != nil {
		t.Errorf("cancelled task not found: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("old file should be removed, err = %v", err)
	}
	if !strings.Contains(out.String(), "001 -> Cancelled") {
		t.Errorf("output missing move message: %q", out.String())
	}
	assertFileContainsAll(t, cancelledPath, "## Cancellation Reason", "No longer needed")
}

func TestTaskCompleteDryRunOnBucketMismatch(t *testing.T) {
	root := projectRoot(t)
	// Task has Completed status but sits in active bucket.
	oldPath := filepath.Join(root, ".ahm", "tasks", "active", "001.md")
	writeTaskFile(t, oldPath, "001", "Already Completed", "Completed", "depends_on: -\n")

	var out strings.Builder
	a := app{opts: options{root: root, dryRun: true}, out: &out}
	if err := a.taskStatus([]string{"001"}, "Completed"); err != nil {
		t.Error(err)
	}

	// File should still be in active (dry run).
	if _, err := os.Stat(oldPath); err != nil {
		t.Errorf("file should still exist after dry run: %v", err)
	}
	completedPath := filepath.Join(root, ".ahm", "tasks", "completed", "001.md")
	if _, err := os.Stat(completedPath); !os.IsNotExist(err) {
		t.Errorf("completed file should not exist after dry run, err = %v", err)
	}
}

func TestTaskStatusNoOpWhenBucketAndStatusMatch(t *testing.T) {
	root := projectRoot(t)
	// Task is Completed and already in completed bucket — true no-op.
	path := filepath.Join(root, ".ahm", "tasks", "completed", "001.md")
	writeTaskFile(t, path, "001", "Truly Completed", "Completed", "depends_on: -\n")

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatus([]string{"001"}, "Completed"); err != nil {
		t.Error(err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("file content changed on no-op status update:\nbefore: %s\nafter:  %s", before, after)
	}
	if !strings.Contains(out.String(), "already Completed") {
		t.Errorf("output missing no-op message: %q", out.String())
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
		t.Errorf("ready = %#v", ready)
	}
	blocked := filterTasks(tasks, "blocked")
	if len(blocked) != 1 || blocked[0].ID != "003" {
		t.Errorf("blocked = %#v", blocked)
	}
}

func TestTaskListFiltersStatus(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Pending Task", "Pending", "")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "completed", "002.md"), "002", "Completed Task", "Completed", "")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "cancelled", "003.md"), "003", "Cancelled Task", "Cancelled", "")

	t.Run("single status", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskList("all", []string{"completed"}, nil, nil, nil); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "002 [Completed] P2 S Completed Task")
		assertNotContains(t, got, "001 [Pending]", "003 [Cancelled]")
	})

	t.Run("multiple statuses", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskList("all", []string{"pending", "cancelled"}, nil, nil, nil); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "001 [Pending] P2 S Pending Task", "003 [Cancelled] P2 S Cancelled Task")
		assertNotContains(t, got, "002 [Completed]")
	})

	t.Run("normalization applies per entry", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskList("all", []string{"PENDING", "CANCELLED"}, nil, nil, nil); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "001 [Pending] P2 S Pending Task", "003 [Cancelled] P2 S Cancelled Task")
		assertNotContains(t, got, "002 [Completed]")
	})

	t.Run("duplicate statuses are deduplicated", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskList("all", []string{"pending", "Pending"}, nil, nil, nil); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "001 [Pending]")
		assertNotContains(t, got, "002 [Completed]", "003 [Cancelled]")
	})

	t.Run("invalid status returns error", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		err := a.taskList("all", []string{"pending", "bogus"}, nil, nil, nil)
		if err == nil {
			t.Error("expected error for invalid status")
		}
		if !strings.Contains(err.Error(), "unsupported task status") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("empty string in comma tokens returns error", func(t *testing.T) {
		// Simulate what happens when --status pending, is used (trailing comma)
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		err := a.taskList("all", []string{"pending", ""}, nil, nil, nil)
		if err == nil {
			t.Error("expected error for empty status")
		}
		if !strings.Contains(err.Error(), "unsupported task status") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestTaskListFiltersLabels(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "CLI Feature", "Pending", "labels: type:feature, area:cli\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "002", "Docs Feature", "Pending", "labels: type:feature, area:docs\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "003.md"), "003", "CLI Bug", "Pending", "labels: type:bug, area:cli\n")

	t.Run("matches all labels", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskList("all", nil, []string{"type:feature", "area:cli"}, nil, nil); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "001 [Pending] P2 S CLI Feature")
		assertNotContains(t, got, "002 [Pending]", "003 [Pending]")
	})

	t.Run("splits comma-separated labels", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskList("all", nil, []string{"type:feature, area:docs"}, nil, nil); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "002 [Pending] P2 S Docs Feature")
		assertNotContains(t, got, "001 [Pending]", "003 [Pending]")
	})

	t.Run("empty label returns usage error", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		err := a.taskList("all", nil, []string{"type:feature,"}, nil, nil)
		if err == nil {
			t.Error("expected error for empty label")
		}
		if !strings.Contains(err.Error(), "task label filter cannot be empty") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestTaskSearch(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Add timeout handling", "Pending", "labels: type:feature, area:cli\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "002", "Document Timeout defaults", "Open", "labels: type:docs, area:docs\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "003.md"), "003", "Unrelated work", "Pending", "labels: type:task, area:cli\n")

	t.Run("matches case-insensitive substring on title", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("timeout", nil, nil); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "001 [Pending] P2 S Add timeout handling", "002 [Open] P2 S Document Timeout defaults")
		assertNotContains(t, got, "003 [Pending]")
	})

	t.Run("composes status filter", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("timeout", []string{"Open"}, nil); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "002 [Open] P2 S Document Timeout defaults")
		assertNotContains(t, got, "001 [Pending]")
	})

	t.Run("composes status and label filters", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("timeout", []string{"Pending"}, []string{"area:cli"}); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "001 [Pending] P2 S Add timeout handling")
		assertNotContains(t, got, "002 [Open]")
	})

	t.Run("empty results print no tasks found", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskSearch("nomatch", nil, nil); err != nil {
			t.Error(err)
		}
		if strings.TrimSpace(out.String()) != "No tasks found." {
			t.Errorf("unexpected output: %q", out.String())
		}
	})

	t.Run("empty results in json mode print empty array", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root, json: true}, out: &out}
		if err := a.taskSearch("nomatch", nil, nil); err != nil {
			t.Error(err)
		}
		if strings.TrimSpace(out.String()) != "[]" {
			t.Errorf("unexpected output: %q", out.String())
		}
	})

	t.Run("blank query returns usage error", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		err := a.taskSearch("   ", nil, nil)
		if err == nil {
			t.Fatal("expected error for blank query")
		}
		if !strings.Contains(err.Error(), "task search requires a query") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestTaskSearchCLINoQuery(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Some task", "Pending", "")
	_, stderr, code := runCLI(t, "--root", root, "task", "search")
	if code != 2 {
		t.Errorf("no-query exit code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "task search requires a query") {
		t.Errorf("unexpected stderr: %s", stderr)
	}
}

func TestTaskListFiltersPriority(t *testing.T) {
	root := projectRoot(t)
	writeTaskFileWithPriority(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "P0 Task", "Pending", "P0", "")
	writeTaskFileWithPriority(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "002", "P1 Task", "Pending", "P1", "")
	writeTaskFileWithPriority(t, filepath.Join(root, ".ahm", "tasks", "active", "003.md"), "003", "P2 Task", "Pending", "P2", "")

	t.Run("single priority filter", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskList("all", nil, nil, []string{"P0"}, nil); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "001 [Pending] P0 S P0 Task")
		assertNotContains(t, got, "002 [Pending]", "003 [Pending]")
	})

	t.Run("multiple priority filter", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskList("all", nil, nil, []string{"P0", "P1"}, nil); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "001 [Pending] P0 S P0 Task", "002 [Pending] P1 S P1 Task")
		assertNotContains(t, got, "003 [Pending]")
	})

	t.Run("priority normalization applies", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskList("all", nil, nil, []string{"p0", "p1"}, nil); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "001 [Pending] P0 S P0 Task", "002 [Pending] P1 S P1 Task")
		assertNotContains(t, got, "003 [Pending]")
	})

	t.Run("invalid priority returns error", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		err := a.taskList("all", nil, nil, []string{"P5"}, nil)
		if err == nil {
			t.Error("expected error for invalid priority")
		}
		if !strings.Contains(err.Error(), "unsupported task priority") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("priority composes with status", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskList("all", []string{"Pending"}, nil, []string{"P1"}, nil); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "002 [Pending] P1 S P1 Task")
		assertNotContains(t, got, "001 [Pending]", "003 [Pending]")
	})
}

func TestTaskListFiltersEffort(t *testing.T) {
	root := projectRoot(t)
	// Write task files with custom effort values via extraFrontMatter override
	writeTaskFileWithPriority(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "XS Task", "Pending", "P2", "effort: XS\n")
	writeTaskFileWithPriority(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "002", "S Task", "Pending", "P2", "effort: S\n")
	writeTaskFileWithPriority(t, filepath.Join(root, ".ahm", "tasks", "active", "003.md"), "003", "M Task", "Pending", "P2", "effort: M\n")

	t.Run("single effort filter", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskList("all", nil, nil, nil, []string{"M"}); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "003 [Pending] P2 M M Task")
		assertNotContains(t, got, "001 [Pending]", "002 [Pending]")
	})

	t.Run("multiple effort filter", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskList("all", nil, nil, nil, []string{"XS", "S"}); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "001 [Pending] P2 XS XS Task", "002 [Pending] P2 S S Task")
		assertNotContains(t, got, "003 [Pending]")
	})

	t.Run("effort normalization applies", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskList("all", nil, nil, nil, []string{"xs", "m"}); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "001 [Pending] P2 XS XS Task", "003 [Pending] P2 M M Task")
		assertNotContains(t, got, "002 [Pending]")
	})

	t.Run("invalid effort returns error", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		err := a.taskList("all", nil, nil, nil, []string{"XXL"})
		if err == nil {
			t.Error("expected error for invalid effort")
		}
		if !strings.Contains(err.Error(), "unsupported task effort") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("effort composes with status and label", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out}
		if err := a.taskList("all", []string{"Pending"}, nil, nil, []string{"M"}); err != nil {
			t.Error(err)
		}
		got := out.String()
		assertContainsAll(t, got, "003 [Pending] P2 M M Task")
		assertNotContains(t, got, "001 [Pending]", "002 [Pending]")
	})
}

func TestTaskListFiltersPriorityEffortJSON(t *testing.T) {
	root := projectRoot(t)
	writeTaskFileWithPriority(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "P0 XS", "Pending", "P0", "effort: XS\n")
	writeTaskFileWithPriority(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "002", "P1 M", "Pending", "P1", "effort: M\n")

	stdout, stderr, code := runCLI(t, "--json", "--root", root, "task", "list", "--priority", "P1", "--effort", "M")
	if code != 0 {
		t.Errorf("exit code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stdout, `"id": "002"`) {
		t.Errorf("expected 002 in JSON output:\n%s", stdout)
	}
	if strings.Contains(stdout, `"id": "001"`) {
		t.Errorf("001 should not appear in JSON output:\n%s", stdout)
	}
}

func TestTaskReadyFiltersLabels(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "completed", "001.md"), "001", "Done", "Completed", "labels: type:task, area:cli\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "002", "CLI Ready", "Pending", "labels: type:feature, area:cli\ndepends_on: 001\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "003.md"), "003", "Docs Ready", "Pending", "labels: type:feature, area:docs\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "004.md"), "004", "CLI Waiting", "Pending", "labels: type:feature, area:cli\ndepends_on: 999\n")

	stdout, stderr, code := runCLI(t, "--root", root, "task", "ready", "--label", "type:feature,area:cli")
	if code != 0 {
		t.Errorf("ready --label exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "002 [Pending] P2 S CLI Ready")
	assertNotContains(t, stdout, "003 [Pending]", "004 [Pending]")
}

func TestTaskReadyIncludesTrackingWithAllChildrenResolved(t *testing.T) {
	root := projectRoot(t)
	// 001 is Tracking with all children Completed or Cancelled — ready.
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Tracker Done", "Tracking", "")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "completed", "001a.md"), "001a", "Child A", "Completed", "parent: 001\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "cancelled", "001b.md"), "001b", "Child B", "Cancelled", "parent: 001\n")
	// 002 is Tracking with a child still open — not ready.
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "002", "Tracker Open", "Tracking", "")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "002a.md"), "002a", "Child Open", "Pending", "parent: 002\n")
	// 003 is Tracking with no children — not ready.
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "003.md"), "003", "Empty Tracker", "Tracking", "")
	// 005 is Tracking with all children Completed but its own dependency is
	// still open — not ready.
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "005.md"), "005", "Tracker Waiting", "Tracking", "depends_on: 006\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "completed", "005a.md"), "005a", "Child Done", "Completed", "parent: 005\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "006.md"), "006", "Open Dep", "Pending", "")
	// 007 is Tracking with all children Completed but depends on a Cancelled
	// task — its own dependency is unsatisfiable, so not ready.
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "007.md"), "007", "Tracker Cancelled Dep", "Tracking", "depends_on: 008\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "completed", "007a.md"), "007a", "Child Done", "Completed", "parent: 007\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "cancelled", "008.md"), "008", "Cancelled Dep", "Cancelled", "")
	// 004 is Pending with no deps — ready as before.
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "004.md"), "004", "Plain Ready", "Pending", "")

	stdout, stderr, code := runCLI(t, "--root", root, "task", "ready")
	if code != 0 {
		t.Fatalf("ready exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"001 [Tracking] P2 S Tracker Done",
		"004 [Pending] P2 S Plain Ready",
	)
	assertNotContains(t, stdout, "002 [Tracking]", "003 [Tracking]", "005 [Tracking]", "007 [Tracking]")
}

func TestTaskCompleteLastChildWarnsTrackerChildrenComplete(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	// Create a tracker and two children through the CLI.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Tracker", "--status", "Tracking")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Fatalf("create tracker stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Child A", "--parent", "001")
	if code != 0 || strings.TrimSpace(stdout) != "001a" {
		t.Fatalf("create child A stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Child B", "--parent", "001")
	if code != 0 || strings.TrimSpace(stdout) != "001b" {
		t.Fatalf("create child B stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}

	// Completing the first child leaves one open, so no tracking warning.
	_, stderr, code = runCLI(t, "--root", root, "task", "complete", "001a")
	if code != 0 {
		t.Fatalf("complete child A exit code = %d, stderr = %s", code, stderr)
	}
	assertNotContains(t, stderr, "task 001 is Tracking")

	// Completing the last child must warn through post-mutation validation.
	_, stderr, code = runCLI(t, "--root", root, "task", "complete", "001b")
	if code != 0 {
		t.Fatalf("complete child B exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stderr, "warning: task 001 is Tracking but all its child tasks are Completed or Cancelled")
}

func TestTaskNextSelectsHighestPriorityReadyTracking(t *testing.T) {
	root := projectRoot(t)
	// P1 tracker with all children Completed beats the P2 pending task.
	writeTaskFileWithPriority(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "P1 Tracker", "Tracking", "P1", "")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "completed", "001a.md"), "001a", "Child A", "Completed", "parent: 001\n")
	writeTaskFileWithPriority(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "002", "P2 Pending", "Pending", "P2", "")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskNext(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	assertContainsAll(t, got, "001 [Tracking] P1 S P1 Tracker")
	assertNotContains(t, got, "002 [Pending]")
}

func TestTaskLabelsListsCounts(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "completed", "001.md"), "001", "Done", "Completed", "labels: type:feature, area:cli\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "002", "Ready", "Pending", "labels: type:feature, area:cli\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "003.md"), "003", "Needs Triage", "Open", "labels: type:bug, area:cli\n")

	stdout, stderr, code := runCLI(t, "--root", root, "task", "labels")
	if code != 0 {
		t.Errorf("labels exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"area:cli total=3 active=2 open=1 ready=1",
		"type:bug total=1 active=1 open=1 ready=0",
		"type:feature total=2 active=1 open=0 ready=1",
	)
	if strings.Index(stdout, "area:cli") > strings.Index(stdout, "type:bug") {
		t.Errorf("labels are not sorted:\n%s", stdout)
	}
}

func TestTaskNextShowsHighestPriorityReadyTask(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "completed", "001.md"), "001", "Done", "Completed", "")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "002", "P2 Ready", "Pending", "depends_on: 001\n")
	writeTaskFileWithPriority(t, filepath.Join(root, ".ahm", "tasks", "active", "003.md"), "003", "P1 Ready", "Pending", "P1", "")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "004.md"), "004", "Blocked", "Pending", "depends_on: 999\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskNext(); err != nil {
		t.Error(err)
	}
	got := out.String()
	assertContainsAll(t, got, "003 [Pending] P1 S P1 Ready")
	assertNotContains(t, got, "002 [Pending]", "004 [Pending]")
}

func TestTaskCommandsResilientToMalformedTasks(t *testing.T) {
	root := projectRoot(t)
	// Valid task
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Valid Task", "Pending", "")
	// Malformed task: invalid enum value "Doing"
	malformedPath := filepath.Join(root, ".ahm", "tasks", "active", "002.md")
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
		if err := a.taskList("all", nil, nil, nil, nil); err != nil {
			t.Error(err)
		}
		got := out.String()
		if !strings.Contains(got, "001 [Pending]") {
			t.Errorf("expected 001 in output:\n%s", got)
		}
		if strings.Contains(got, "002 [Doing]") {
			t.Errorf("malformed task should not appear in list:\n%s", got)
		}
		if !strings.Contains(errBuf.String(), "warning:") {
			t.Errorf("expected stderr warning, got: %q", errBuf.String())
		}
	})

	t.Run("task ready skips malformed task", func(t *testing.T) {
		var out, errBuf strings.Builder
		a := app{opts: options{root: root}, out: &out, err: &errBuf}
		if err := a.taskList("ready", nil, nil, nil, nil); err != nil {
			t.Error(err)
		}
		got := out.String()
		if !strings.Contains(got, "001 [Pending]") {
			t.Errorf("expected 001 in output:\n%s", got)
		}
		if !strings.Contains(errBuf.String(), "warning:") {
			t.Errorf("expected stderr warning, got: %q", errBuf.String())
		}
	})

	t.Run("task next skips malformed task", func(t *testing.T) {
		var out, errBuf strings.Builder
		a := app{opts: options{root: root}, out: &out, err: &errBuf}
		if err := a.taskNext(); err != nil {
			t.Error(err)
		}
		got := out.String()
		if !strings.Contains(got, "001 [Pending]") {
			t.Errorf("expected 001 in output:\n%s", got)
		}
		if !strings.Contains(errBuf.String(), "warning:") {
			t.Errorf("expected stderr warning, got: %q", errBuf.String())
		}
	})

	t.Run("resolveTask finds valid task despite malformed others", func(t *testing.T) {
		var errBuf strings.Builder
		a := app{opts: options{root: root}, err: &errBuf}
		task, err := a.resolveTask("001")
		if err != nil {
			t.Error(err)
		}
		if task.ID != "001" {
			t.Errorf("id = %q", task.ID)
		}
		if !strings.Contains(errBuf.String(), "warning:") {
			t.Errorf("expected stderr warning, got: %q", errBuf.String())
		}
	})

	t.Run("resolveTask returns not-found for malformed task", func(t *testing.T) {
		var errBuf strings.Builder
		a := app{opts: options{root: root}, err: &errBuf}
		_, err := a.resolveTask("002")
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected not-found for malformed task, got: %v", err)
		}
	})

	t.Run("index regenerates despite malformed task", func(t *testing.T) {
		var out, errBuf strings.Builder
		a := app{opts: options{root: root}, out: &out, err: &errBuf}
		if err := a.writeIndexes(); err != nil {
			t.Error(err)
		}
		indexPath := filepath.Join(root, ".ahm", "tasks", "index.md")
		data, err := os.ReadFile(indexPath)
		if err != nil {
			t.Fatal(err)
		}
		got := string(data)
		if !strings.Contains(got, "001.md) | Valid Task") {
			t.Errorf("expected 001 in index:\n%s", got)
		}
	})

	t.Run("task dep tree works with malformed task", func(t *testing.T) {
		writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "005.md"), "005", "Dep Parent", "Pending", "depends_on: 001\n")

		var out, errBuf strings.Builder
		a := app{opts: options{root: root}, out: &out, err: &errBuf}
		if err := a.taskDepTree([]string{"005"}); err != nil {
			t.Error(err)
		}
		got := out.String()
		if !strings.Contains(got, "005 [Pending] Dep Parent") {
			t.Errorf("expected dep tree with 005:\n%s", got)
		}
		if !strings.Contains(got, "001 [Pending] Valid Task") {
			t.Errorf("expected dep tree with 001:\n%s", got)
		}
		// Should print exactly one warning, not two (no double collectTasks call)
		warnCount := strings.Count(errBuf.String(), "warning:")
		if warnCount != 1 {
			t.Errorf("expected exactly 1 warning, got %d: %q", warnCount, errBuf.String())
		}
	})

	t.Run("task dep cycles works with malformed task", func(t *testing.T) {
		// Add cycle between valid tasks
		writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "003.md"), "003", "Cycle A", "Pending", "depends_on: 004\n")
		writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "004.md"), "004", "Cycle B", "Pending", "depends_on: 003\n")

		var out, errBuf strings.Builder
		a := app{opts: options{root: root}, out: &out, err: &errBuf}
		if err := a.taskDepCycles(); err != nil {
			t.Error(err)
		}
		got := out.String()
		if !strings.Contains(got, "003 -> 004 -> 003") {
			t.Errorf("cycle output = %q", got)
		}
		if !strings.Contains(errBuf.String(), "warning:") {
			t.Errorf("expected stderr warning, got: %q", errBuf.String())
		}
	})
}

func TestTaskCreateWithMalformedTaskDeduplicatesWarnings(t *testing.T) {
	root := projectRoot(t)
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Create a valid task first so there's at least one parsed task.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Valid Task", "--status", "Pending")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Fatalf("create valid stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}

	// Write a malformed task file in active/.
	malformed := filepath.Join(root, ".ahm", "tasks", "active", "bad.md")
	if err := os.WriteFile(malformed, []byte("---\ninvalid : key\n---\n# Bad\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Run task create "x" — should print each distinct warning exactly once.
	var createOut, createErr strings.Builder
	a := app{
		opts: options{root: root},
		out:  &createOut,
		err:  &createErr,
	}
	if err := a.taskCreateParsed(taskCreateArgs{title: "x", status: "Pending", priority: "P2", effort: "S", labels: "type:task, area:unknown"}); err != nil {
		t.Fatal(err)
	}
	got := createErr.String()
	warnCount := strings.Count(got, "warning:")
	// With one malformed file, the expected minimum warnings are:
	//   1. "some task files could not be parsed and were skipped"
	//   2. "some task files could not be parsed and were skipped: ..."
	// Plus post-mutation validation adds findings about the missing
	// front matter fields and unsupported status from the bad.md file
	// (its front matter has only "invalid : key" and is missing all
	// required fields).
	if warnCount < 3 {
		t.Errorf("expected at least 3 warning lines (2 parse + validation findings), got %d:\n%s", warnCount, got)
	}
	if !strings.Contains(got, "some task files could not be parsed and were skipped") {
		t.Errorf("missing generic parse warning:\n%s", got)
	}
	if !strings.Contains(got, "bad.md") {
		t.Errorf("expected malformed file reference in stderr:\n%s", got)
	}
	// Post-mutation validation should report the missing front matter fields.
	if !strings.Contains(got, "task front matter is missing") {
		t.Errorf("expected post-mutation validation warning about missing front matter:\n%s", got)
	}
	// Verify the created task exists.
	createdPath := filepath.Join(root, ".ahm", "tasks", "active", "002.md")
	if _, err := os.Stat(createdPath); err != nil {
		t.Errorf("created task not found: %v", err)
	}
}

func TestMainTaskLifecycleAndDependencyIntegration(t *testing.T) {
	root := projectRoot(t)
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// First task: explicitly Pending so lifecycle integration works
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "First Task", "--priority", "P1", "--effort", "M", "--description", "First body", "--status", "Pending")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Errorf("create first stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	// Second task: explicitly Pending so lifecycle integration works
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Second Task", "--status", "Pending")
	if code != 0 || strings.TrimSpace(stdout) != "002" {
		t.Errorf("create second stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "dep", "add", "002", "001")
	if code != 0 {
		t.Errorf("dep add exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "002 depends_on: 001")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "depends_on: 001")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "blocked")
	if code != 0 {
		t.Errorf("blocked exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "002 [Pending]")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "start", "001")
	if code != 0 {
		t.Errorf("start exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "001 -> In Progress")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "complete", "001")
	if code != 0 {
		t.Errorf("complete exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "001 -> Completed")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "completed", "001.md"), "status: Completed")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "ready")
	if code != 0 {
		t.Errorf("ready exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "002 [Pending]")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "reopen", "001")
	if code != 0 {
		t.Errorf("reopen exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "001 -> Pending")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "status: Pending")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "dep", "tree", "002")
	if code != 0 {
		t.Errorf("dep tree exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "002 [Pending] Second Task", "  001 [Pending] First Task")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "dep", "remove", "002", "001")
	if code != 0 {
		t.Errorf("dep remove exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "002 depends_on: -")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "depends_on: -")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "next")
	if code != 0 {
		t.Errorf("next exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "001 [Pending] P1 M First Task")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "cancel", "002", "--reason", "No longer needed")
	if code != 0 {
		t.Errorf("cancel exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "002 -> Cancelled")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "cancelled", "002.md"), "status: Cancelled", "## Cancellation Reason", "No longer needed")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "list", "--status", "cancelled")
	if code != 0 {
		t.Errorf("list --status exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "002 [Cancelled] P2 S Second Task")
	assertNotContains(t, stdout, "001 [Pending]")
}

func TestTaskCompleteRefusesIncompleteDependencies(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Dependency Task", "Pending", "")
	writeTaskFileWithDeps(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "002", "Main Task", "Pending", "001")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	err := a.taskStatus([]string{"002"}, "Completed")
	if err == nil {
		t.Error("expected error from completing task with incomplete dependency")
	}
	if !strings.Contains(err.Error(), "incomplete dependencies: 001") {
		t.Errorf("error message = %q, want incomplete dependencies: 001", err.Error())
	}
	// Task file should not have been moved.
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "completed", "002.md")); !os.IsNotExist(err) {
		t.Error("completed file should not exist after failed completion")
	}
}

func TestTaskCompleteSucceedsWithCompletedDependencies(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "completed", "001.md"), "001", "Completed Dep", "Completed", "")
	writeTaskFileWithDeps(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "002", "Main Task", "Pending", "001")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatus([]string{"002"}, "Completed"); err != nil {
		t.Error(err)
	}
	// Task should have been moved to completed.
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "completed", "002.md")); err != nil {
		t.Errorf("completed file should exist: %v", err)
	}
}

func TestTaskStatusReusesParsedStateAfterLock(t *testing.T) {
	root := projectRoot(t)
	const taskCount = 300
	for i := 1; i <= taskCount; i++ {
		id := fmt.Sprintf("%03d", i)
		writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", id+".md"), id, "Task "+id, "Pending", "")
	}

	originalParseHook := taskParseHook
	originalIndexHook := indexWritesForPathsHook
	originalPreLockHook := taskStatusPreLockHook
	t.Cleanup(func() {
		taskParseHook = originalParseHook
		indexWritesForPathsHook = originalIndexHook
		taskStatusPreLockHook = originalPreLockHook
	})
	parseCounts := map[string]int{}
	indexRenders := 0
	measuring := false
	taskParseHook = func(path string) {
		if measuring {
			parseCounts[path]++
		}
	}
	indexWritesForPathsHook = func() {
		if measuring {
			indexRenders++
		}
	}
	taskStatusPreLockHook = func() {
		parseCounts = map[string]int{}
		indexRenders = 0
		measuring = true
	}

	stdout, stderr, code := runCLI(t, "--root", root, "task", "start", "300")
	measuring = false
	if code != 0 {
		t.Fatalf("task start exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if len(parseCounts) != taskCount {
		t.Fatalf("parsed task files = %d, want %d", len(parseCounts), taskCount)
	}
	for path, count := range parseCounts {
		if count != 1 {
			t.Fatalf("post-lock parse count for %s = %d, want 1", path, count)
		}
	}
	if indexRenders != 1 {
		t.Fatalf("generated index renders = %d, want 1", indexRenders)
	}
}

func TestTaskMutationRefusesDuplicateIDs(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	paths := workflowPathsFor(root)

	// Write two task files with the same ID in different buckets (crash-window state).
	writeTaskFile(t, paths.taskFile("active", "042"), "042", "Duplicate Original", "Pending", "")
	writeTaskFile(t, paths.taskFile("completed", "042"), "042", "Duplicate Copy", "Completed", "depends_on: -\n")

	t.Run("status transition fails", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out, err: &strings.Builder{}}
		err := a.taskStatus([]string{"042"}, "Completed")
		if err == nil {
			t.Fatal("expected error for duplicate task ID, got nil")
		}
		if !strings.Contains(err.Error(), "duplicate") || !strings.Contains(err.Error(), "042") {
			t.Errorf("error should mention duplicate and the ID, got: %v", err)
		}
		if !strings.Contains(err.Error(), "active/042.md") || !strings.Contains(err.Error(), "completed/042.md") {
			t.Errorf("error should name both conflicting paths, got: %v", err)
		}
		if !strings.Contains(err.Error(), "resolve the duplicate manually") {
			t.Errorf("error should include recovery instruction, got: %v", err)
		}
	})

	t.Run("dependency command fails", func(t *testing.T) {
		// Create a second valid task to use as dependency target.
		writeTaskFile(t, paths.taskFile("active", "001"), "001", "Valid Task", "Pending", "")

		var out strings.Builder
		a := app{opts: options{root: root}, out: &out, err: &strings.Builder{}}
		err := a.taskDepUpdate([]string{"001", "042"}, true)
		if err == nil {
			t.Fatal("expected error for duplicate task ID, got nil")
		}
		if !strings.Contains(err.Error(), "duplicate") || !strings.Contains(err.Error(), "042") {
			t.Errorf("error should mention duplicate and the ID, got: %v", err)
		}
		if !strings.Contains(err.Error(), "resolve the duplicate manually") {
			t.Errorf("error should include recovery instruction, got: %v", err)
		}
	})

	t.Run("show command still works (read-only)", func(t *testing.T) {
		var out strings.Builder
		a := app{opts: options{root: root}, out: &out, err: &strings.Builder{}}
		err := a.taskShow([]string{"042"})
		if err != nil {
			t.Errorf("read-only show command should still work with duplicates, got: %v", err)
		}
	})
}

func TestTaskCompleteSucceedsWithNoDependencies(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Standalone Task", "Pending", "")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatus([]string{"001"}, "Completed"); err != nil {
		t.Error(err)
	}
	// Task should have been moved to completed.
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "completed", "001.md")); err != nil {
		t.Errorf("completed file should exist: %v", err)
	}
}

func TestTaskCompleteUnblocksDirectDependents(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Dependency Task", "Pending", "")
	writeTaskFileWithDeps(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "002", "Dependent Task", "Blocked", "001")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatus([]string{"001"}, "Completed"); err != nil {
		t.Error(err)
	}

	assertContainsAll(t, out.String(), "001 -> Completed", "002 -> Pending")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "completed", "001.md"), "status: Completed")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "status: Pending", "depends_on: 001")
}

func TestTaskCompleteLeavesMultiDependencyBlockedUntilAllComplete(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "First Dependency", "Pending", "")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "002", "Second Dependency", "Pending", "")
	writeTaskFileWithDeps(t, filepath.Join(root, ".ahm", "tasks", "active", "003.md"), "003", "Dependent Task", "Blocked", "001, 002")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatus([]string{"001"}, "Completed"); err != nil {
		t.Error(err)
	}

	assertContainsAll(t, out.String(), "001 -> Completed")
	assertNotContains(t, out.String(), "003 -> Pending")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "003.md"), "status: Blocked", "depends_on: 001, 002")
}

func TestTaskCompleteDoesNotUnblockUnrelatedBlockedTasks(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Finished Dependency", "Pending", "")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "completed", "002.md"), "002", "Other Dependency", "Completed", "")
	writeTaskFileWithDeps(t, filepath.Join(root, ".ahm", "tasks", "active", "003.md"), "003", "Unrelated Blocked Task", "Blocked", "002")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatus([]string{"001"}, "Completed"); err != nil {
		t.Error(err)
	}

	assertContainsAll(t, out.String(), "001 -> Completed")
	assertNotContains(t, out.String(), "003 -> Pending")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "003.md"), "status: Blocked", "depends_on: 002")
}

func TestTaskCompleteDryRunReportsUnblockedDependentsWithoutWriting(t *testing.T) {
	root := projectRoot(t)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Dependency Task", "Pending", "")
	writeTaskFileWithDeps(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "002", "Dependent Task", "Blocked", "001")

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "complete", "001")
	if code != 0 {
		t.Errorf("dry-run complete exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	assertContainsAll(t, stdout,
		"move: ", ".ahm/tasks/completed/001.md",
		"status: Completed",
		"unblocked:",
		"id: 002",
		".ahm/tasks/active/002.md",
		"status: Pending",
	)
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "status: Pending")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "status: Blocked")
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "completed", "001.md")); !os.IsNotExist(err) {
		t.Error("completed file should not exist after dry-run completion")
	}
}

func TestTaskCompleteWarnsForIncompleteAcceptanceByDefault(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Needs Acceptance")
	if code != 0 {
		t.Errorf("create exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "complete", "001")
	if code != 0 {
		t.Errorf("complete exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "001 -> Completed")
	assertContainsAll(t, stderr, "warning: task 001 acceptance notes still contain the TODO placeholder")
}

func TestTaskCancelRequiresReason(t *testing.T) {
	root := projectRoot(t)
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "No Reason")
	if code != 0 {
		t.Errorf("create exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "cancel", "001")
	if code == 0 {
		t.Errorf("expected cancel without reason to fail, stdout = %s, stderr = %s", stdout, stderr)
	}
	assertContainsAll(t, stderr, "task cancel requires --reason")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "status: Open")
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "cancelled", "001.md")); !os.IsNotExist(err) {
		t.Error("cancelled file should not exist after missing reason failure")
	}
}

func TestTaskCancelForceDoesNotBypassMissingReason(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "No Reason")
	if code != 0 {
		t.Errorf("create exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "--force", "task", "cancel", "001")
	if code == 0 {
		t.Errorf("expected force cancel without reason to fail, stdout = %s, stderr = %s", stdout, stderr)
	}
	assertContainsAll(t, stderr, "task cancel requires --reason")
}

func TestTaskCancelPersistsReason(t *testing.T) {
	root := projectRoot(t)
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Cancelled Task", "--status", "Pending")
	if code != 0 {
		t.Errorf("create exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "cancel", "001", "--reason", "Superseded by 002")
	if code != 0 {
		t.Errorf("cancel exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "001 -> Cancelled")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "cancelled", "001.md"),
		"status: Cancelled",
		"## Cancellation Reason\n\nSuperseded by 002",
	)
}

func TestTaskCancelDryRunShowsReasonWithoutWriting(t *testing.T) {
	root := projectRoot(t)
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Dry Run Cancel")
	if code != 0 {
		t.Errorf("create exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "--dry-run", "task", "cancel", "001", "--reason", "Superseded")
	if code != 0 {
		t.Errorf("dry-run cancel exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "move: ", ".ahm/tasks/cancelled/001.md", "status: Cancelled", "reason: Superseded")
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "cancelled", "001.md")); !os.IsNotExist(err) {
		t.Error("cancelled file should not exist after dry-run cancellation")
	}
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "status: Open")
}

func TestTaskCancelWarnsForSeededAcceptanceTODO(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Needs Acceptance")
	if code != 0 {
		t.Errorf("create exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "cancel", "001", "--reason", "Obsolete")
	if code != 0 {
		t.Errorf("cancel exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
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
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	meta, err := readMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	meta.StrictAcceptance = true
	writeMetadataFile(t, root, meta)
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Needs Acceptance")
	if code != 0 {
		t.Errorf("create exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "complete", "001")
	if code == 0 {
		t.Errorf("expected strict completion failure, stdout = %s, stderr = %s", stdout, stderr)
	}
	assertContainsAll(t, stderr,
		"warning: task 001 acceptance notes still contain the TODO placeholder",
		"cannot complete task 001: acceptance notes are incomplete; use --force to override",
	)
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "completed", "001.md")); !os.IsNotExist(err) {
		t.Error("completed file should not exist after strict acceptance failure")
	}
}

func TestTaskCompleteForceOverridesStrictAcceptance(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	meta, err := readMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	meta.StrictAcceptance = true
	writeMetadataFile(t, root, meta)
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Needs Acceptance")
	if code != 0 {
		t.Errorf("create exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "--force", "task", "complete", "001")
	if code != 0 {
		t.Errorf("force complete exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "001 -> Completed")
	assertContainsAll(t, stderr, "warning: task 001 acceptance notes still contain the TODO placeholder")
}

func TestTaskCompleteDryRunPreservesPreviewWithAcceptanceWarning(t *testing.T) {
	root := projectRoot(t)
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Needs Acceptance")
	if code != 0 {
		t.Errorf("create exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "--dry-run", "task", "complete", "001")
	if code != 0 {
		t.Errorf("dry-run complete exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "move: ", ".ahm/tasks/completed/001.md", "status: Completed")
	assertContainsAll(t, stderr, "warning: task 001 acceptance notes still contain the TODO placeholder")
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "completed", "001.md")); !os.IsNotExist(err) {
		t.Error("completed file should not exist after dry-run completion")
	}
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "status: Open")
}

func TestTaskCompleteRefusesIncompleteDepsIntegration(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Dependency")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Errorf("create first stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Main")
	if code != 0 || strings.TrimSpace(stdout) != "002" {
		t.Errorf("create second stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}

	// Make 002 depend on 001.
	_, stderr, code = runCLI(t, "--root", root, "task", "dep", "add", "002", "001")
	if code != 0 {
		t.Errorf("dep add exit code = %d, stderr = %s", code, stderr)
	}

	// Try completing 002 while 001 is still pending.
	_, stderr, code = runCLI(t, "--root", root, "task", "complete", "002")
	if code == 0 {
		t.Error("expected non-zero exit from completing task with pending dependency")
	}
	if !strings.Contains(stderr, "incomplete dependencies: 001") {
		t.Errorf("stderr = %q, want incomplete dependencies: 001", stderr)
	}
	// Verify 002 was not moved to completed.
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "completed", "002.md")); !os.IsNotExist(err) {
		t.Error("completed file should not exist after failed completion")
	}

	// Now complete 001 and verify 002 can be completed.
	_, stderr, code = runCLI(t, "--root", root, "task", "complete", "001")
	if code != 0 {
		t.Errorf("complete 001 exit code = %d, stderr = %s", code, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "complete", "002")
	if code != 0 {
		t.Errorf("complete 002 exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "002 -> Completed")
}

func TestTaskAcceptMovesOpenToPending(t *testing.T) {
	root := projectRoot(t)
	_, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stderr = %s", code, stderr)
	}

	// Create an Open task (new default).
	_, stderr, code = runCLI(t, "--root", root, "task", "create", "Needs Triage")
	if code != 0 {
		t.Errorf("create exit code = %d, stderr = %s", code, stderr)
	}
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "status: Open")

	// Accept it.
	stdout, stderr, code := runCLI(t, "--root", root, "task", "accept", "001")
	if code != 0 {
		t.Errorf("accept exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "001 -> Pending")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "status: Pending")
}

func TestTaskAcceptDryRunPreviews(t *testing.T) {
	root := projectRoot(t)
	_, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stderr = %s", code, stderr)
	}

	_, stderr, code = runCLI(t, "--root", root, "task", "create", "Needs Triage")
	if code != 0 {
		t.Errorf("create exit code = %d, stderr = %s", code, stderr)
	}

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "accept", "001")
	if code != 0 {
		t.Errorf("dry-run accept exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "move: ", ".ahm/tasks/active/001.md", "status: Pending")
	// File should remain Open after dry-run.
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "status: Open")
}

func TestTaskAcceptFromBlocked(t *testing.T) {
	root := projectRoot(t)
	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	// Create a Blocked task directly.
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Blocked Task", "Blocked", "")

	if err := a.taskStatus([]string{"001"}, "Pending"); err != nil {
		t.Error(err)
	}
	assertContainsAll(t, out.String(), "001 -> Pending")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "status: Pending")
}

func TestTaskAcceptNoOp(t *testing.T) {
	root := projectRoot(t)
	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Already Pending", "Pending", "")

	if err := a.taskStatus([]string{"001"}, "Pending"); err != nil {
		t.Error(err)
	}
	assertContainsAll(t, out.String(), "001 already Pending")
}

func writeTaskFileWithDeps(t *testing.T, path string, id string, title string, status string, deps string) {
	t.Helper()
	extra := "depends_on: " + deps + "\n"
	writeTaskFile(t, path, id, title, status, extra)
}

func TestTaskCompleteParallelUnblocksDependents(t *testing.T) {
	root := projectRoot(t)
	// Create tasks manually (no install needed — just raw task files).
	// 001 and 002 are dependencies of 003 (Blocked).
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Dependency A", "Pending", "")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "002", "Dependency B", "Pending", "")
	writeTaskFileWithDeps(t, filepath.Join(root, ".ahm", "tasks", "active", "003.md"), "003", "Dependent Task", "Blocked", "001, 002")

	// Verify initial state.
	for _, id := range []string{"001", "002", "003"} {
		p := filepath.Join(root, ".ahm", "tasks", "active", id+".md")
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("missing task %s: %v", id, err)
		}
	}

	const completions = 2
	var wg sync.WaitGroup
	errc := make(chan error, completions)
	ids := []string{"001", "002"}
	for _, id := range ids {
		id := id
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out strings.Builder
			a := app{opts: options{root: root}, out: &out}
			if err := a.taskStatus([]string{id}, "Completed"); err != nil {
				errc <- err
			}
		}()
	}
	wg.Wait()
	close(errc)
	for err := range errc {
		t.Error(err)
	}

	// Task 003 must now be Pending (both dependencies completed).
	data, err := os.ReadFile(filepath.Join(root, ".ahm", "tasks", "active", "003.md"))
	if err != nil {
		t.Errorf("active/003.md: %v", err)
	} else if !strings.Contains(string(data), "status: Pending") {
		t.Errorf("003.md status not Pending, got:\n%s", string(data))
	}
	// Both dependencies should be in completed/.
	for _, id := range []string{"001", "002"} {
		p := filepath.Join(root, ".ahm", "tasks", "completed", id+".md")
		assertFileContainsAll(t, p, "status: Completed")
	}
	// Index must show 003 as Pending (it renders as a table cell, not front matter).
	indexContent := mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "index.md"))
	if !strings.Contains(indexContent, "Dependent Task") {
		t.Errorf("index missing Dependent Task:\n%s", indexContent)
	}
	// The table shows | [id](path) | Title | Status | ... so look for the task row with Pending status.
	if !strings.Contains(indexContent, "003.md) | Dependent Task | Pending") {
		t.Errorf("index does not show 003 as Pending:\n%s", indexContent)
	}
}

func TestTaskCompleteWaitsForStatusLock(t *testing.T) {
	saveLockTimeout(t)
	workflowLockTimeout = 2 * time.Second
	workflowLockRetryDelay = time.Millisecond

	root := projectRoot(t)
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(); err != nil {
		t.Fatal(err)
	}
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Locked Task", "Pending", "")

	release, err := acquireWorkflowRecordLock(workflowPathsFor(root))
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	var out strings.Builder
	go func() {
		a := app{opts: options{root: root}, out: &out}
		done <- a.taskStatus([]string{"001"}, "Completed")
	}()

	select {
	case err := <-done:
		t.Errorf("task complete finished while workflow lock was held: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	if err := release(); err != nil {
		t.Error(err)
	}

	select {
	case err := <-done:
		// May fail with incomplete acceptance notes (created via writeTaskFile
		// has no acceptance section). The important thing is that it didn't hang.
		_ = err
	case <-time.After(2 * time.Second):
		t.Error("task complete did not finish after workflow lock was released")
	}
}

func TestTaskStatusReResolvesTargetUnderLock(t *testing.T) {
	saveLockTimeout(t)
	workflowLockTimeout = 2 * time.Second
	workflowLockRetryDelay = time.Millisecond

	root := projectRoot(t)
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(); err != nil {
		t.Fatal(err)
	}
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Original Title", "Pending", "")

	release, err := acquireWorkflowRecordLock(workflowPathsFor(root))
	if err != nil {
		t.Fatal(err)
	}

	oldHook := taskStatusPreLockHook
	preLock := make(chan struct{})
	taskStatusPreLockHook = func() { close(preLock) }
	defer func() { taskStatusPreLockHook = oldHook }()

	done := make(chan error, 1)
	var out strings.Builder
	go func() {
		a := app{opts: options{root: root, force: true}, out: &out}
		done <- a.taskStatus([]string{"001"}, "Completed")
	}()

	select {
	case <-preLock:
	case <-time.After(2 * time.Second):
		t.Fatal("task status did not reach pre-lock hook")
	}

	// Simulate a concurrent update that landed after the goroutine resolved the
	// task but before it could acquire the mutation lock.
	activePath := filepath.Join(root, ".ahm", "tasks", "active", "001.md")
	content := mustRead(t, activePath)
	updated := strings.Replace(content, "Original Title", "Updated Title", 1)
	if err := os.WriteFile(activePath, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := release(); err != nil {
		t.Error(err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("task complete returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("task complete did not finish after workflow lock was released")
	}

	// The completed task must preserve the concurrent update, proving that the
	// status transition re-resolved the target under the lock.
	completedPath := filepath.Join(root, ".ahm", "tasks", "completed", "001.md")
	assertFileContainsAll(t, completedPath, "title: Updated Title", "# Updated Title")
}

func TestTaskCommentAndCompleteSerialized(t *testing.T) {
	saveLockTimeout(t)
	workflowLockTimeout = 5 * time.Second
	workflowLockRetryDelay = time.Millisecond

	for i := 0; i < 10; i++ {
		root := projectRoot(t)
		var installOut strings.Builder
		installer := app{opts: options{root: root}, out: &installOut}
		if err := installer.install(); err != nil {
			t.Fatal(err)
		}

		var createOut strings.Builder
		creator := app{opts: options{root: root}, out: &createOut}
		if err := creator.taskCreateParsed(taskCreateArgs{
			title:    "Race Task",
			priority: "P2",
			effort:   "S",
			status:   "Open",
		}); err != nil {
			t.Fatalf("create failed: %v", err)
		}

		var wg sync.WaitGroup
		errc := make(chan error, 2)

		wg.Add(1)
		go func() {
			defer wg.Done()
			a := app{opts: options{root: root, force: true}, out: io.Discard}
			errc <- a.taskStatus([]string{"001"}, "Completed")
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			a := app{opts: options{root: root}, out: io.Discard}
			errc <- a.taskComment(taskCommentArgs{id: "001", text: "concurrent note"})
		}()

		wg.Wait()
		close(errc)
		for err := range errc {
			if err != nil {
				t.Fatalf("iteration %d: concurrent command failed: %v", i, err)
			}
		}

		activePath := filepath.Join(root, ".ahm", "tasks", "active", "001.md")
		completedPath := filepath.Join(root, ".ahm", "tasks", "completed", "001.md")
		_, err := os.Stat(activePath)
		activeExists := !os.IsNotExist(err)
		_, err = os.Stat(completedPath)
		completedExists := !os.IsNotExist(err)

		if activeExists && completedExists {
			t.Fatalf("iteration %d: duplicate active and completed 001.md", i)
		}
		if !activeExists && !completedExists {
			t.Fatalf("iteration %d: 001.md missing from both buckets", i)
		}

		finalPath := activePath
		if completedExists {
			finalPath = completedPath
		}
		content := mustRead(t, finalPath)
		if !strings.Contains(content, "concurrent note") {
			t.Fatalf("iteration %d: comment missing from final task:\n%s", i, content)
		}
		if completedExists && !strings.Contains(content, "status: Completed") {
			t.Fatalf("iteration %d: completed file missing status: Completed", i)
		}
	}
}

func TestTaskCompleteWarnsOnCorruptMetadata(t *testing.T) {
	root := projectRoot(t)
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	// Create a task with acceptance notes.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Test Task")
	if code != 0 {
		t.Errorf("create exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Corrupt the metadata file.
	metaPath := filepath.Join(root, ".ahm", "config.json")
	if err := os.WriteFile(metaPath, []byte("{invalid json}"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Task complete should warn about corrupt metadata but still succeed
	// (strict acceptance can't be determined).
	stdout, stderr, code = runCLI(t, "--root", root, "task", "complete", "001")
	if code != 0 {
		t.Errorf("complete exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stderr, "corrupt workflow metadata .ahm/config.json", "strict acceptance disabled")
	assertContainsAll(t, stdout, "001 -> Completed")
}

func TestTaskCompleteRespectsStrictAcceptanceWithValidMetadata(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	// Enable strict acceptance.
	meta, err := readMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	meta.StrictAcceptance = true
	writeMetadataFile(t, root, meta)
	// Create a task.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Needs Strict")
	if code != 0 {
		t.Errorf("create exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Task complete should block due to strict acceptance.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "complete", "001")
	if code == 0 {
		t.Errorf("expected strict completion failure, code=%d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stderr, "cannot complete task 001: acceptance notes are incomplete")
}

func TestFormatComment(t *testing.T) {
	ts := "2026-06-24T12:00:00Z"

	got := formatComment(ts, "", "Found the root cause")
	want := "**2026-06-24T12:00:00Z** — Found the root cause"
	if got != want {
		t.Errorf("formatComment = %q, want %q", got, want)
	}

	got = formatComment(ts, "Travis", "Need to revisit")
	want = "**2026-06-24T12:00:00Z** — _Travis_: Need to revisit"
	if got != want {
		t.Errorf("formatComment with author = %q, want %q", got, want)
	}
}

func TestAppendComment(t *testing.T) {
	ts := "**2026-06-24T12:00:00Z** — Test comment"

	t.Run("creates section when missing", func(t *testing.T) {
		body := "# My Task\n\n## Summary\n\nBody content.\n"
		got := appendComment(body, ts)
		assertContainsAll(t, got, "## Comments", ts, "## Summary", "Body content.")
	})

	t.Run("appends to existing section", func(t *testing.T) {
		body := "# My Task\n\n## Comments\n\n**old** — first comment\n\n## Other\n"
		got := appendComment(body, ts)
		assertContainsAll(t, got, "## Comments", ts, "first comment", "## Other")
		assertContainsAll(t, got, "first comment\n\n"+ts)
	})

	t.Run("handles empty section", func(t *testing.T) {
		body := "# My Task\n\n## Comments\n\n## Other"
		got := appendComment(body, ts)
		assertContainsAll(t, got, "## Comments", ts, "## Other")
	})

	t.Run("appends to body-only task", func(t *testing.T) {
		body := "# My Task\n\nJust some text.\n"
		got := appendComment(body, ts)
		assertContainsAll(t, got, "## Comments", ts, "Just some text.")
	})

	t.Run("section heading is case-insensitive", func(t *testing.T) {
		body := "# My Task\n\n## comments\n\n**old** — existing\n"
		got := appendComment(body, ts)
		assertContainsAll(t, got, "## comments", ts)
	})
}

func TestTaskCommentCLI(t *testing.T) {
	root := projectRoot(t)
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Create a task.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Test Task")
	if code != 0 {
		t.Fatalf("create exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	t.Run("appends comment to active task", func(t *testing.T) {
		stdout, stderr, code = runCLI(t, "--root", root, "task", "comment", "001", "Found the root cause")
		if code != 0 {
			t.Fatalf("comment exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
		}
		if strings.TrimSpace(stdout) != "001" {
			t.Errorf("stdout = %q, want %q", stdout, "001")
		}
		content := mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"))
		assertContainsAll(t, content, "## Comments", "Found the root cause")
	})

	t.Run("appends multiple comments", func(t *testing.T) {
		stdout, stderr, code = runCLI(t, "--root", root, "task", "comment", "001", "--author", "Travis", "Second observation")
		if code != 0 {
			t.Fatalf("second comment exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
		}
		content := mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"))
		assertContainsAll(t, content, "_Travis_: Second observation")
		// Both comments should be present.
		assertContainsAll(t, content, "Found the root cause")
	})

	t.Run("dry-run does not mutate", func(t *testing.T) {
		contentBefore := mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"))

		stdout, stderr, code = runCLI(t, "--root", root, "--dry-run", "task", "comment", "001", "Dry run comment")
		if code != 0 {
			t.Fatalf("dry-run comment exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
		}
		assertContainsAll(t, stdout, "001")

		contentAfter := mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"))
		if contentBefore != contentAfter {
			t.Errorf("dry-run modified task file")
		}
	})

	t.Run("usage error on missing text", func(t *testing.T) {
		stdout, stderr, code = runCLI(t, "--root", root, "task", "comment", "001")
		if code != 2 {
			t.Errorf("expected exit code 2, got %d; stderr = %s", code, stderr)
		}
	})

	t.Run("works on completed task", func(t *testing.T) {
		// Complete the task first, bypassing strict acceptance because it has TODO placeholder.
		// The task has default acceptance notes with TODO, but --force skips strict check.
		stdout, stderr, code = runCLI(t, "--root", root, "task", "complete", "001")
		if code != 0 {
			t.Fatalf("complete exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
		}

		stdout, stderr, code = runCLI(t, "--root", root, "task", "comment", "001", "Post-completion note")
		if code != 0 {
			t.Fatalf("comment on completed task exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
		}
		content := mustRead(t, filepath.Join(root, ".ahm", "tasks", "completed", "001.md"))
		assertContainsAll(t, content, "Post-completion note")
	})

	t.Run("fails on missing task", func(t *testing.T) {
		stdout, stderr, code = runCLI(t, "--root", root, "task", "comment", "999", "Nope")
		if code == 0 {
			t.Errorf("expected failure for missing task, code=%d, stdout=%s", code, stdout)
		}
	})
}

func TestTaskShowSingleID(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init failed: stdout=%q stderr=%q", stdout, stderr)
	}

	// Create a task to show.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Test Show", "--description", "Body text")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Fatalf("create failed: stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}

	// Show single ID (text mode).
	stdout, stderr, code = runCLI(t, "--root", root, "task", "show", "001")
	if code != 0 {
		t.Errorf("show exit code = %d, stderr = %q", code, stderr)
	}
	assertContainsAll(t, stdout, "title: Test Show", "## Summary", "Body text")
}

func TestTaskShowSingleIDJSON(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init failed: stdout=%q stderr=%q", stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "JSON Show", "--description", "Body json")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Fatalf("create failed: stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}

	// Show single ID with --json yields a single object, not an array.
	stdout, stderr, code = runCLI(t, "--root", root, "--json", "task", "show", "001")
	if code != 0 {
		t.Errorf("show exit code = %d, stderr = %q", code, stderr)
	}
	// Should start with "{" not "["
	if !strings.HasPrefix(strings.TrimSpace(stdout), "{") {
		t.Errorf("single ID --json should emit object, got:\n%s", stdout)
	}
	assertContainsAll(t, stdout, `"id": "001"`, `"title": "JSON Show"`)
}

func TestTaskShowMultipleIDsText(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init failed: stdout=%q stderr=%q", stdout, stderr)
	}

	// Create two tasks.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "First", "--description", "Alpha")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Fatalf("create first failed: stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Second", "--description", "Beta")
	if code != 0 || strings.TrimSpace(stdout) != "002" {
		t.Fatalf("create second failed: stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}

	// Show both IDs.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "show", "001", "002")
	if code != 0 {
		t.Errorf("show exit code = %d, stderr = %q", code, stderr)
	}
	// Both titles should be present.
	assertContainsAll(t, stdout, "title: First", "title: Second")
	// Separator should appear.
	if !strings.Contains(stdout, "\n---\n") {
		t.Errorf("multi-task text should contain separator, got:\n%s", stdout)
	}
}

func TestTaskShowMultipleIDsJSON(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init failed: stdout=%q stderr=%q", stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Alpha", "--description", "AA")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Fatalf("create first failed: stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Beta", "--description", "BB")
	if code != 0 || strings.TrimSpace(stdout) != "002" {
		t.Fatalf("create second failed: stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}

	// Show both IDs with --json yields an array.
	stdout, stderr, code = runCLI(t, "--root", root, "--json", "task", "show", "001", "002")
	if code != 0 {
		t.Errorf("show exit code = %d, stderr = %q", code, stderr)
	}
	if !strings.HasPrefix(strings.TrimSpace(stdout), "[") {
		t.Errorf("multi ID --json should emit array, got:\n%s", stdout)
	}
	assertContainsAll(t, stdout, `"id": "001"`, `"id": "002"`)
}

func TestTaskShowNoArgs(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init failed: stdout=%q stderr=%q", stdout, stderr)
	}

	_, stderr, code = runCLI(t, "--root", root, "task", "show")
	if code == 0 {
		t.Errorf("expected non-zero exit code, got 0")
	}
	if !strings.Contains(stderr, "task show requires at least one id") {
		t.Errorf("expected usage error in stderr, got: %s", stderr)
	}
}

func TestTaskShowNonExistentID(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init failed: stdout=%q stderr=%q", stdout, stderr)
	}

	_, stderr, code = runCLI(t, "--root", root, "task", "show", "999")
	if code == 0 {
		t.Errorf("expected non-zero exit code, got 0")
	}
	if !strings.Contains(stderr, `task "999" not found`) {
		t.Errorf("expected not-found error, got: %s", stderr)
	}
}

func TestTaskShowPartialFailure(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init failed: stdout=%q stderr=%q", stdout, stderr)
	}

	// Create one task.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Partial", "--description", "Partial test")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Fatalf("create failed: stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}

	// Show one valid and one invalid ID.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "show", "001", "999")
	if code == 0 {
		t.Errorf("expected non-zero exit code for partial failure, got 0")
	}
	// Should contain the valid task output.
	assertContainsAll(t, stdout, "title: Partial")
	// Should contain the error for the invalid ID.
	if !strings.Contains(stderr, `task "999" not found`) {
		t.Errorf("expected not-found error, got: %s", stderr)
	}
}

func TestTrimTrailingBlankLines(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{"no trailing blanks", []string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{"trailing blanks", []string{"a", "b", "", "", ""}, []string{"a", "b"}},
		{"all blanks", []string{"", "", ""}, []string{}},
		{"empty input", []string{}, []string{}},
		{"blanks in middle preserved", []string{"a", "", "b", ""}, []string{"a", "", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trimTrailingBlankLines(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d; got %v", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
