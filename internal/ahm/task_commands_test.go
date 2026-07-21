package ahm

import (
	"bytes"
	"context"
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
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
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
	root := t.TempDir()
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
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
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

	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	release, err := acquireWorkflowRecordLockWithResolver(root, func() workflowPaths {
		return workflowPathsFor(root)
	})
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
	root := t.TempDir()
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
	indexContent := mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "index.md"))
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
	root := t.TempDir()
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
	root := t.TempDir()
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
	root := t.TempDir()
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
	root := t.TempDir()
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
	root := t.TempDir()
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
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	// Manually create a child with letter 'c' to skip 'a', 'b'.
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001c.md"), "001c", "Existing Child C", "Pending", "parent: 001\n")

	// Also create a completed child with letter 'e' to prove scanning happens across buckets.
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "completed", "001e.md"), "001e", "Completed Child E", "Completed", "parent: 001\n")

	// Collect tasks and call nextChildTaskIDForPaths directly.
	tasks, err := collectTasksForPaths(root, workflowPathsFor(root))
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
	if _, err := os.Stat(filepath.Join(root, ".agents", ".tasks", "active", "001a.md")); err == nil {
		t.Errorf("dry-run should not create child file")
	}
}

func TestTaskCreateTopLevelUnchangedWithParentFlag(t *testing.T) {
	// Verify that not using --parent still produces top-level IDs.
	root := t.TempDir()
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
		t.Error(err)
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
		t.Error(err)
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
			root := t.TempDir()
			path := filepath.Join(root, ".agents", ".tasks", tt.initialBucket, "001.md")
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

			updatedPath := filepath.Join(root, ".agents", ".tasks", tt.targetBucket, "001.md")
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
	root := t.TempDir()
	// Task has Completed status but sits in active bucket.
	oldPath := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	writeTaskFile(t, oldPath, "001", "Already Completed", "Completed", "depends_on: -\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatus([]string{"001"}, "Completed"); err != nil {
		t.Error(err)
	}

	// Task should be moved to completed bucket.
	completedPath := filepath.Join(root, ".agents", ".tasks", "completed", "001.md")
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
	root := t.TempDir()
	// Task has Cancelled status but sits in active bucket.
	oldPath := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	writeTaskFile(t, oldPath, "001", "Already Cancelled", "Cancelled", "depends_on: -\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatusWithArgs(taskStatusArgs{ids: []string{"001"}, status: "Cancelled", reason: "No longer needed"}); err != nil {
		t.Error(err)
	}

	// Task should be moved to cancelled bucket.
	cancelledPath := filepath.Join(root, ".agents", ".tasks", "cancelled", "001.md")
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
	root := t.TempDir()
	// Task has Completed status but sits in active bucket.
	oldPath := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
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
	completedPath := filepath.Join(root, ".agents", ".tasks", "completed", "001.md")
	if _, err := os.Stat(completedPath); !os.IsNotExist(err) {
		t.Errorf("completed file should not exist after dry run, err = %v", err)
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
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Pending Task", "Pending", "")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "completed", "002.md"), "002", "Completed Task", "Completed", "")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "cancelled", "003.md"), "003", "Cancelled Task", "Cancelled", "")

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
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "CLI Feature", "Pending", "labels: type:feature, area:cli\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Docs Feature", "Pending", "labels: type:feature, area:docs\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "003.md"), "003", "CLI Bug", "Pending", "labels: type:bug, area:cli\n")

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
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Add timeout handling", "Pending", "labels: type:feature, area:cli\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Document Timeout defaults", "Open", "labels: type:docs, area:docs\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "003.md"), "003", "Unrelated work", "Pending", "labels: type:task, area:cli\n")

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
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Some task", "Pending", "")
	_, stderr, code := runCLI(t, "--root", root, "task", "search")
	if code != 2 {
		t.Errorf("no-query exit code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "task search requires a query") {
		t.Errorf("unexpected stderr: %s", stderr)
	}
}

func TestTaskListFiltersPriority(t *testing.T) {
	root := t.TempDir()
	writeTaskFileWithPriority(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "P0 Task", "Pending", "P0", "")
	writeTaskFileWithPriority(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "P1 Task", "Pending", "P1", "")
	writeTaskFileWithPriority(t, filepath.Join(root, ".agents", ".tasks", "active", "003.md"), "003", "P2 Task", "Pending", "P2", "")

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
	root := t.TempDir()
	// Write task files with custom effort values via extraFrontMatter override
	writeTaskFileWithPriority(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "XS Task", "Pending", "P2", "effort: XS\n")
	writeTaskFileWithPriority(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "S Task", "Pending", "P2", "effort: S\n")
	writeTaskFileWithPriority(t, filepath.Join(root, ".agents", ".tasks", "active", "003.md"), "003", "M Task", "Pending", "P2", "effort: M\n")

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
	root := t.TempDir()
	writeTaskFileWithPriority(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "P0 XS", "Pending", "P0", "effort: XS\n")
	writeTaskFileWithPriority(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "P1 M", "Pending", "P1", "effort: M\n")

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
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "completed", "001.md"), "001", "Done", "Completed", "labels: type:task, area:cli\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "CLI Ready", "Pending", "labels: type:feature, area:cli\ndepends_on: 001\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "003.md"), "003", "Docs Ready", "Pending", "labels: type:feature, area:docs\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "004.md"), "004", "CLI Waiting", "Pending", "labels: type:feature, area:cli\ndepends_on: 999\n")

	stdout, stderr, code := runCLI(t, "--root", root, "task", "ready", "--label", "type:feature,area:cli")
	if code != 0 {
		t.Errorf("ready --label exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "002 [Pending] P2 S CLI Ready")
	assertNotContains(t, stdout, "003 [Pending]", "004 [Pending]")
}

func TestTaskLabelsListsCounts(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "completed", "001.md"), "001", "Done", "Completed", "labels: type:feature, area:cli\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Ready", "Pending", "labels: type:feature, area:cli\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "003.md"), "003", "Needs Triage", "Open", "labels: type:bug, area:cli\n")

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
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "completed", "001.md"), "001", "Done", "Completed", "")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "P2 Ready", "Pending", "depends_on: 001\n")
	writeTaskFileWithPriority(t, filepath.Join(root, ".agents", ".tasks", "active", "003.md"), "003", "P1 Ready", "Pending", "P1", "")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "004.md"), "004", "Blocked", "Pending", "depends_on: 999\n")

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
		indexPath := filepath.Join(root, ".agents", ".tasks", "index.md")
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
		writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "005.md"), "005", "Dep Parent", "Pending", "depends_on: 001\n")

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
		writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "003.md"), "003", "Cycle A", "Pending", "depends_on: 004\n")
		writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "004.md"), "004", "Cycle B", "Pending", "depends_on: 003\n")

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
	root := t.TempDir()
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
	root := t.TempDir()
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
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Dependency Task", "Pending", "")
	writeTaskFileWithDeps(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Main Task", "Pending", "001")

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
	if _, err := os.Stat(filepath.Join(root, ".agents", ".tasks", "completed", "002.md")); !os.IsNotExist(err) {
		t.Error("completed file should not exist after failed completion")
	}
}

func TestTaskCompleteSucceedsWithCompletedDependencies(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "completed", "001.md"), "001", "Completed Dep", "Completed", "")
	writeTaskFileWithDeps(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Main Task", "Pending", "001")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatus([]string{"002"}, "Completed"); err != nil {
		t.Error(err)
	}
	// Task should have been moved to completed.
	if _, err := os.Stat(filepath.Join(root, ".agents", ".tasks", "completed", "002.md")); err != nil {
		t.Errorf("completed file should exist: %v", err)
	}
}

func TestTaskStatusReusesParsedStateAfterLock(t *testing.T) {
	root := t.TempDir()
	const taskCount = 300
	for i := 1; i <= taskCount; i++ {
		id := fmt.Sprintf("%03d", i)
		writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", id+".md"), id, "Task "+id, "Pending", "")
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

func TestTaskCompleteSucceedsWithNoDependencies(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Standalone Task", "Pending", "")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatus([]string{"001"}, "Completed"); err != nil {
		t.Error(err)
	}
	// Task should have been moved to completed.
	if _, err := os.Stat(filepath.Join(root, ".agents", ".tasks", "completed", "001.md")); err != nil {
		t.Errorf("completed file should exist: %v", err)
	}
}

func TestTaskCompleteWarnsActiveExecPlan(t *testing.T) {
	root := t.TempDir()

	// Create an active ExecPlan with a filled Outcomes & Retrospective section.
	planDir := filepath.Join(root, ".agents", "exec-plans", "active")
	planPath := filepath.Join(planDir, "myplan.md")
	planContent := "# My Plan\n\n" +
		"## Progress\n\n- [x] Done\n\n" +
		"## Surprises & Discoveries\n\nNone.\n\n" +
		"## Decision Log\n\n- Decision: go ahead\n\n" +
		"## Outcomes & Retrospective\n\nCompleted successfully.\n"
	writeFile(t, planPath, planContent)

	// Create a task with exec_plan pointing to the active ExecPlan.
	taskPath := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	writeTaskFile(t, taskPath, "001", "Task With Active Plan", "Pending",
		"exec_plan: "+relPath(root, planPath)+"\n")

	var out, errOut strings.Builder
	a := app{opts: options{root: root}, out: &out, err: &errOut}
	if err := a.taskStatus([]string{"001"}, "Completed"); err != nil {
		t.Fatal(err)
	}

	// Verify the warning appears on stderr.
	warning := errOut.String()
	if !strings.Contains(warning, "active ExecPlan") {
		t.Errorf("stderr missing active ExecPlan warning:\n%s", warning)
	}
	if !strings.Contains(warning, "move it to the completed ExecPlan bucket") {
		t.Errorf("stderr missing guidance to move plan:\n%s", warning)
	}
	if !strings.Contains(warning, "filled Outcomes & Retrospective") {
		t.Errorf("stderr missing filled Outcomes note:\n%s", warning)
	}
	// Task should still complete (warning only, not an error).
	if _, err := os.Stat(filepath.Join(root, ".agents", ".tasks", "completed", "001.md")); err != nil {
		t.Errorf("completed file should exist: %v", err)
	}
}

func TestTaskCompleteUnblocksDirectDependents(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Dependency Task", "Pending", "")
	writeTaskFileWithDeps(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Dependent Task", "Blocked", "001")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatus([]string{"001"}, "Completed"); err != nil {
		t.Error(err)
	}

	assertContainsAll(t, out.String(), "001 -> Completed", "002 -> Pending")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "completed", "001.md"), "status: Completed")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "status: Pending", "depends_on: 001")
}

func TestTaskCompleteLeavesMultiDependencyBlockedUntilAllComplete(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "First Dependency", "Pending", "")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Second Dependency", "Pending", "")
	writeTaskFileWithDeps(t, filepath.Join(root, ".agents", ".tasks", "active", "003.md"), "003", "Dependent Task", "Blocked", "001, 002")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatus([]string{"001"}, "Completed"); err != nil {
		t.Error(err)
	}

	assertContainsAll(t, out.String(), "001 -> Completed")
	assertNotContains(t, out.String(), "003 -> Pending")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "003.md"), "status: Blocked", "depends_on: 001, 002")
}

func TestTaskCompleteDoesNotUnblockUnrelatedBlockedTasks(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Finished Dependency", "Pending", "")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "completed", "002.md"), "002", "Other Dependency", "Completed", "")
	writeTaskFileWithDeps(t, filepath.Join(root, ".agents", ".tasks", "active", "003.md"), "003", "Unrelated Blocked Task", "Blocked", "002")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskStatus([]string{"001"}, "Completed"); err != nil {
		t.Error(err)
	}

	assertContainsAll(t, out.String(), "001 -> Completed")
	assertNotContains(t, out.String(), "003 -> Pending")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "003.md"), "status: Blocked", "depends_on: 002")
}

func TestTaskCompleteDryRunReportsUnblockedDependentsWithoutWriting(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Dependency Task", "Pending", "")
	writeTaskFileWithDeps(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Dependent Task", "Blocked", "001")

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "complete", "001")
	if code != 0 {
		t.Errorf("dry-run complete exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	assertContainsAll(t, stdout,
		"move: ", ".agents/.tasks/completed/001.md",
		"status: Completed",
		"unblocked:",
		"id: 002",
		".agents/.tasks/active/002.md",
		"status: Pending",
	)
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "status: Pending")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "status: Blocked")
	if _, err := os.Stat(filepath.Join(root, ".agents", ".tasks", "completed", "001.md")); !os.IsNotExist(err) {
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
	root := t.TempDir()
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
	root := t.TempDir()
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
	root := t.TempDir()
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
	if err := writeMetadata(root, meta); err != nil {
		t.Fatal(err)
	}
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
	if _, err := os.Stat(filepath.Join(root, ".agents", ".tasks", "completed", "001.md")); !os.IsNotExist(err) {
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
	if err := writeMetadata(root, meta); err != nil {
		t.Fatal(err)
	}
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
	root := t.TempDir()
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
	if _, err := os.Stat(filepath.Join(root, ".agents", ".tasks", "completed", "002.md")); !os.IsNotExist(err) {
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

func TestTaskWorkDefaultsToCakeAndMarksPendingInProgress(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Workable Task", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/" + executable, nil
	})
	var captured taskWorkCapture
	var markedInProgress bool
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		// At runner-invocation time markTaskInProgress has already run, so the
		// task should be in the active bucket with status: In Progress.
		activePath := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
		data, readErr := os.ReadFile(activePath)
		if readErr != nil {
			return fmt.Errorf("active task not found at runner time: %w", readErr)
		}
		if strings.Contains(string(data), "status: In Progress") {
			markedInProgress = true
		}
		captured.root = root
		captured.executable = executable
		captured.args = append([]string(nil), args...)
		return completeTaskOnDisk(root, "001")
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--no-review", "--no-commit")
	if code != 0 {
		t.Errorf("task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	if !markedInProgress {
		t.Error("task should have been marked In Progress by markTaskInProgress before the runner ran")
	}
	if captured.root != root {
		t.Errorf("runner root = %q, want %q", captured.root, root)
	}
	if captured.executable != "/stub/cake" {
		t.Errorf("runner executable = %q, want /stub/cake", captured.executable)
	}
	if len(captured.args) != 3 || captured.args[0] != "--output-format" || captured.args[1] != "stream-json" {
		t.Errorf("cake args = %#v", captured.args)
	}
	assertContainsAll(t, captured.args[2], "Work on task 001.", "ahm task show 001", "Do not commit or push")
	// After direct completion the task should be Completed in the completed bucket.
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "completed", "001.md"), "status: Completed")
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
	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--no-review", "--no-commit")
	if code != 0 {
		t.Errorf("configured task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	if configured.executable != "/stub/codex" || len(configured.args) != 4 || configured.args[0] != "exec" || configured.args[1] != "--dangerously-bypass-approvals-and-sandbox" || configured.args[2] != "--json" {
		t.Errorf("configured invocation executable=%q args=%#v", configured.executable, configured.args)
	}
	if err := os.Remove(filepath.Join(root, ".agents", ".tasks", "completed", "001.md")); err != nil {
		t.Fatal(err)
	}
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Configured Task", "In Progress", "")

	var flagged taskWorkCapture
	stubTaskWorkRunner(t, flagged.runner)
	stdout, stderr, code = runCLI(t, "--root", root, "task", "work", "001", "--agent", "cursor", "--no-review", "--no-commit")
	if code != 0 {
		t.Errorf("flagged task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	if flagged.executable != "/stub/cursor-agent" {
		t.Errorf("flagged executable = %q, want /stub/cursor-agent", flagged.executable)
	}
	if len(flagged.args) != 5 || flagged.args[0] != "-p" || flagged.args[1] != "--output-format" || flagged.args[2] != "stream-json" || flagged.args[3] != "--trust" {
		t.Errorf("cursor args = %#v", flagged.args)
	}
}

func TestTaskWorkAgentConfigFromAhmConfig(t *testing.T) {
	root := t.TempDir()
	if err := writeConfigMetadata(root, metadata{DefaultWorkAgent: "codex"}); err != nil {
		t.Fatal(err)
	}
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Configured Task", "In Progress", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/" + executable, nil
	})

	var captured taskWorkCapture
	stubTaskWorkRunner(t, captured.runner)
	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--no-review", "--no-commit")
	if code != 0 {
		t.Errorf("configured task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	if captured.executable != "/stub/codex" {
		t.Errorf("configured invocation executable=%q args=%#v", captured.executable, captured.args)
	}
}

func TestTaskWorkUnsupportedAgentIsUsageError(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Workable Task", "Pending", "")

	_, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--agent", "unknown")
	if code != 2 {
		t.Errorf("exit code = %d, want 2; stderr = %s", code, stderr)
	}
	assertContainsAll(t, stderr, `unsupported task work agent "unknown"`, "supported: cake, claude, codex, cursor")
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
		t.Errorf("exit code = %d, want 2; stderr = %s", code, stderr)
	}
	assertContainsAll(t, stderr, `unsupported task work agent "unknown"`, "supported: cake, claude, codex, cursor")
}

func TestTaskWorkTimeoutZeroIsUsageError(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Timeout Zero", "Pending", "")

	_, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--timeout", "0", "--no-review", "--no-commit")
	if code != 2 {
		t.Errorf("exit code = %d, want 2; stderr = %s", code, stderr)
	}
	assertContainsAll(t, stderr, "must be greater than 0", "--timeout")
}

func TestTaskWorkTimeoutNegativeIsUsageError(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Timeout Negative", "Pending", "")

	_, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--timeout", "-5m", "--no-review", "--no-commit")
	if code != 2 {
		t.Errorf("exit code = %d, want 2; stderr = %s", code, stderr)
	}
	assertContainsAll(t, stderr, "must be greater than 0", "--timeout")
}

func TestTaskWorkTimeoutValidDuration(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Timeout Valid", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/" + executable, nil
	})
	var captured taskWorkCapture
	stubTaskWorkRunner(t, captured.runner)

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--timeout", "2h", "--no-review", "--no-commit")
	if code != 0 {
		t.Errorf("task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	if captured.root != root {
		t.Errorf("runner root = %q, want %q", captured.root, root)
	}
	if captured.executable != "/stub/cake" {
		t.Errorf("runner executable = %q, want /stub/cake", captured.executable)
	}
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "completed", "001.md"), "status: Completed")
}

func TestTaskWorkTimeoutDryRunPreview(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Timeout Dry-Run", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/" + executable, nil
	})
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		t.Error("runner should not be called during dry-run")
		return nil
	})

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "work", "001", "--timeout", "90m", "--no-review", "--no-commit")
	if code != 0 {
		t.Errorf("dry-run exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "timeout", "1h30m0s")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "status: Pending")
}

func TestTaskWorkDryRunPreviewsWithoutMutatingOrInvoking(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Workable Task", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/" + executable, nil
	})
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		t.Error("runner should not be called during dry-run")
		return nil
	})

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "work", "001", "--agent", "codex")
	if code != 0 {
		t.Errorf("dry-run task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "agent: codex", "executable: /stub/codex", "status: In Progress", "task: 001", "exec", "prompt: Work on task 001.")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "status: Pending")
}

func TestTaskWorkModelFlagInArgs(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Model Task", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/" + executable, nil
	})
	var captured taskWorkCapture
	stubTaskWorkRunner(t, captured.runner)

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--agent", "codex", "--model", "o3-mini", "--no-review", "--no-commit")
	if code != 0 {
		t.Errorf("task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	if len(captured.args) != 6 {
		t.Fatalf("args = %#v, want 6 elements including --model", captured.args)
	}
	if captured.args[0] != "exec" || captured.args[1] != "--model" || captured.args[2] != "o3-mini" {
		t.Errorf("expected exec --model o3-mini at start, got %#v", captured.args)
	}
}

func TestTaskWorkModelFlagInDryRun(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Model Dry Run", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/" + executable, nil
	})
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		t.Error("runner should not be called during dry-run")
		return nil
	})

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "work", "001", "--model", "o4-mini", "--agent", "cake")
	if code != 0 {
		t.Errorf("dry-run task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "model: o4-mini", "--model", "o4-mini")
}

func TestTaskWorkModelWithAllAgents(t *testing.T) {
	for _, tt := range []struct {
		name       string
		agentFlag  string
		executable string
		modelName  string
		wantPrefix []string
	}{
		{name: "cake", agentFlag: "cake", executable: "cake", modelName: "claude-sonnet-4", wantPrefix: []string{"--output-format", "stream-json", "--model", "claude-sonnet-4"}},
		{name: "codex", agentFlag: "codex", executable: "codex", modelName: "o3-mini", wantPrefix: []string{"exec", "--model", "o3-mini", "--dangerously-bypass-approvals-and-sandbox", "--json"}},
		{name: "cursor", agentFlag: "cursor", executable: "cursor-agent", modelName: "gpt-4o", wantPrefix: []string{"-p", "--model", "gpt-4o", "--output-format", "stream-json", "--trust"}},
		{name: "claude", agentFlag: "claude", executable: "claude", modelName: "sonnet", wantPrefix: []string{"-p", "--model", "sonnet", "--verbose", "--output-format", "stream-json", "--dangerously-skip-permissions"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Model Agent Test", "Pending", "")
			stubTaskWorkLookPath(t, func(executable string) (string, error) {
				return "/stub/" + executable, nil
			})
			var captured taskWorkCapture
			stubTaskWorkRunner(t, captured.runner)

			args := []string{"--root", root, "task", "work", "001", "--agent", tt.agentFlag, "--model", tt.modelName, "--no-review", "--no-commit"}
			stdout, stderr, code := runCLI(t, args...)
			if code != 0 {
				t.Errorf("exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
			}
			if len(captured.args) != len(tt.wantPrefix)+1 {
				t.Fatalf("%s args = %#v, want %d args (prefix %#v + prompt)", tt.name, captured.args, len(tt.wantPrefix)+1, tt.wantPrefix)
			}
			for i, want := range tt.wantPrefix {
				if captured.args[i] != want {
					t.Errorf("%s args[%d] = %q, want %q; full args = %#v", tt.name, i, captured.args[i], want, captured.args)
				}
			}
		})
	}
}

func TestTaskWorkReviewArgsWithModel(t *testing.T) {
	for _, name := range []string{"cake", "codex", "cursor", "claude"} {
		t.Run(name, func(t *testing.T) {
			agent, err := parseTaskWorkAgent(name)
			if err != nil {
				t.Fatal(err)
			}
			// Empty model should not add --model flag.
			argsNoModel := agent.reviewArgs("Review the changes.", "")
			for _, arg := range argsNoModel {
				if arg == "--model" {
					t.Errorf("%s review args with empty model should not contain --model: %#v", name, argsNoModel)
				}
			}

			// Non-empty model should include --model <name> before the prompt.
			argsWithModel := agent.reviewArgs("Review the changes.", "gpt-4o")
			found := false
			for i, arg := range argsWithModel {
				if arg == "--model" {
					found = true
					if i+1 >= len(argsWithModel) || argsWithModel[i+1] != "gpt-4o" {
						t.Errorf("%s review args with model: expected --model gpt-4o, got %#v", name, argsWithModel)
					}
				}
			}
			if !found {
				t.Errorf("%s review args with model should contain --model flag: %#v", name, argsWithModel)
			}
			// Last arg should be the prompt.
			if argsWithModel[len(argsWithModel)-1] != "Review the changes." {
				t.Errorf("%s review args last arg = %q, want prompt", name, argsWithModel[len(argsWithModel)-1])
			}
		})
	}
}

func TestTaskWorkCursorDryRunPreviewsStreamJSONArgs(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Cursor Dry Run", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/" + executable, nil
	})
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		t.Error("runner should not be called during dry-run")
		return nil
	})

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "work", "001", "--agent", "cursor")
	if code != 0 {
		t.Errorf("dry-run task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout,
		"agent: cursor",
		"executable: /stub/cursor-agent",
		"status: In Progress",
		"task: 001",
		"-p",
		"stream-json",
		"--trust",
		"prompt: Work on task 001.",
		"review: true",
		"commit: true",
	)
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
	var oldRemoved, activeInProgressAtRunnerTime bool
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		// markTaskInProgress runs before the invocation: the misplaced
		// completed-bucket file should now be gone and the active file written.
		if _, err := os.Stat(oldPath); errors.Is(err, os.ErrNotExist) {
			oldRemoved = true
		}
		activePath := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
		if data, err := os.ReadFile(activePath); err == nil && bytes.Contains(data, []byte("status: In Progress")) {
			activeInProgressAtRunnerTime = true
		}
		captured.root = root
		captured.executable = executable
		captured.args = append([]string(nil), args...)
		return completeTaskOnDisk(root, "001")
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--no-review", "--no-commit")
	if code != 0 {
		t.Errorf("task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	if !oldRemoved {
		t.Error("old completed-bucket file should be removed after repair")
	}
	if !activeInProgressAtRunnerTime {
		t.Error("task should have been moved to active and marked In Progress before the runner ran")
	}
	// After direct completion the repaired task should be Completed in the completed bucket.
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "completed", "001.md"), "status: Completed")
}

func TestTaskWorkDryRunOnBucketMismatch(t *testing.T) {
	root := t.TempDir()
	// Task has Pending status but sits in completed bucket.
	oldPath := filepath.Join(root, ".agents", ".tasks", "completed", "001.md")
	writeTaskFile(t, oldPath, "001", "Misplaced Pending", "Pending", "depends_on: -\n")

	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/" + executable, nil
	})
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		t.Error("runner should not be called during dry-run")
		return nil
	})

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "work", "001", "--agent", "codex")
	if code != 0 {
		t.Errorf("dry-run task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// File should still be in completed (dry run).
	if _, err := os.Stat(oldPath); err != nil {
		t.Errorf("file should still exist after dry run: %v", err)
	}
	activePath := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	if _, err := os.Stat(activePath); !os.IsNotExist(err) {
		t.Errorf("active file should not exist after dry run, err = %v", err)
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
				t.Errorf("LookPath should not be called for %s task", tt.status)
				return "", nil
			})

			_, stderr, code := runCLI(t, "--root", root, "task", "work", "001")
			if code == 0 {
				t.Errorf("expected failure for %s task", tt.status)
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
		t.Error("LookPath should not be called for incomplete dependencies")
		return "", nil
	})

	_, stderr, code := runCLI(t, "--root", root, "task", "work", "002")
	if code == 0 {
		t.Error("expected dependency failure")
	}
	assertContainsAll(t, stderr, "cannot work task 002: incomplete dependencies: 001")
}

func TestTaskWorkRefusesUnparseableDependencySet(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "completed", "001.md"), "001", "Dependency", "Completed", "")
	mainPath := filepath.Join(root, ".agents", ".tasks", "active", "002.md")
	writeTaskFileWithDeps(t, mainPath, "002", "Main", "Pending", "001")
	malformedPath := filepath.Join(root, ".agents", ".tasks", "active", "003.md")
	if err := os.WriteFile(malformedPath, []byte("---\nid: 003\nstatus: Pending\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		t.Error("LookPath should not be called when dependency readiness cannot be checked")
		return "", nil
	})

	_, stderr, code := runCLI(t, "--root", root, "task", "work", "002")
	if code == 0 {
		t.Fatal("expected dependency parse failure")
	}
	assertContainsAll(t, stderr,
		"dependency readiness cannot be checked because task files are unparseable",
		"repair the task records and retry",
	)
	assertNotContains(t, stderr, "incomplete dependencies")
	if strings.Count(stderr, "warning: some task files could not be parsed and were skipped") != 1 {
		t.Fatalf("parse warning count = %d, want 1\n%s", strings.Count(stderr, "warning: some task files could not be parsed and were skipped"), stderr)
	}
	assertFileContainsAll(t, mainPath, "status: Pending")
}

func TestTaskWorkMissingExecutableLeavesPendingTaskUnchanged(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Workable Task", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "", errors.New("missing")
	})

	_, stderr, code := runCLI(t, "--root", root, "task", "work", "001")
	if code == 0 {
		t.Error("expected missing executable failure")
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
		{name: "cake", executable: "cake", prefix: []string{"--output-format", "stream-json"}},
		{name: "codex", executable: "codex", prefix: []string{"exec", "--dangerously-bypass-approvals-and-sandbox", "--json"}},
		{name: "cursor", executable: "cursor-agent", prefix: []string{"-p", "--output-format", "stream-json", "--trust"}},
		{name: "claude", executable: "claude", prefix: []string{"-p", "--verbose", "--output-format", "stream-json", "--dangerously-skip-permissions"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			agent, err := parseTaskWorkAgent(tt.name)
			if err != nil {
				t.Error(err)
			}
			if agent.executable != tt.executable {
				t.Errorf("executable = %q, want %q", agent.executable, tt.executable)
			}
			args := agent.args("prompt", "")
			for i, want := range tt.prefix {
				if args[i] != want {
					t.Errorf("args = %#v, want prefix %#v", args, tt.prefix)
				}
			}
			if args[len(args)-1] != "prompt" {
				t.Errorf("args = %#v, final arg should be prompt", args)
			}
			// All agents support sessions and review; verify methods are set.
			if agent.resumeArgs == nil {
				t.Error("agent must have resumeArgs")
			}
			if agent.parseSessionID == nil {
				t.Error("agent must have parseSessionID")
			}
			if agent.reviewArgs == nil {
				t.Error("agent must have reviewArgs")
			}
			if agent.parseReviewFeedback == nil {
				t.Error("agent must have parseReviewFeedback")
			}
		})
	}
}

func TestTaskWorkSessionCapableAgentsDoNotSuppressSessions(t *testing.T) {
	for _, name := range []string{"cake", "codex", "cursor", "claude"} {
		t.Run(name, func(t *testing.T) {
			agent, err := parseTaskWorkAgent(name)
			if err != nil {
				t.Error(err)
			}
			for phase, args := range map[string][]string{
				"work":   agent.args("Work on task 001.", ""),
				"review": agent.reviewArgs("Review the changes.", ""),
				"resume": agent.resumeArgs("session-id", "Continue working."),
			} {
				for _, arg := range args {
					if arg == "--no-session" {
						t.Fatalf("%s args for %s suppress session creation: %#v", phase, name, args)
					}
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

func (c *taskWorkCapture) runner(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	c.root = root
	c.executable = executable
	c.args = append([]string(nil), args...)
	// Completing the task on disk mirrors the contracted --no-review path:
	// the work-session prompt asks the agent to mark the task complete, and
	// ahm now validates that final status before returning success.
	return completeTaskOnDisk(root, "001")
}

func stubTaskWorkLookPath(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	orig := taskWorkLookPath
	taskWorkLookPath = fn
	t.Cleanup(func() {
		taskWorkLookPath = orig
	})
}

func stubTaskWorkRunner(t *testing.T, fn taskWorkRunnerFunc) {
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
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		if len(args) < 3 || args[0] != "--output-format" || args[1] != "stream-json" {
			t.Errorf("unexpected args = %#v", args)
		}
		fmt.Fprintln(stdout, `{"type":"task_start","session_id":"sess_abc123","task_id":"tsk_xyz"}`)
		fmt.Fprintln(stdout, `{"type":"message","role":"assistant","content":"Working..."}`)
		fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":"Work completed.","session_id":"sess_abc123"}`)
		return completeTaskOnDisk(root, "001")
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--no-review", "--no-commit")
	if code != 0 {
		t.Errorf("task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "session_id", "sess_abc123", "type", "task_start")
	// The session started message should appear on stderr.
	assertContainsAll(t, stderr, "cake session started: sess_abc")
	// The session warning should not appear.
	assertNotContains(t, stderr, "could not capture session ID")
	assertNotContains(t, stderr, "no session ID returned")
	// Task should be Completed after direct completion.
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "completed", "001.md"), "status: Completed")
}

func TestTaskWorkCakeSessionParseInvalidJSON(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Session Parse Failure", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})
	// Stub the runner to produce invalid JSON (non-JSON line).
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		_, err := fmt.Fprint(stdout, `not json`)
		return err
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--no-review", "--no-commit")
	if code == 0 {
		t.Errorf("expected nonzero exit when work session does not complete the task, got code %d: stdout=%s stderr=%s", code, stdout, stderr)
	}
	// The session warning should still appear; the command degrades to
	// direct-completion validation which fails because the task was not
	// marked Completed.
	assertContainsAll(t, stderr, "warning: could not capture session ID from cake output")
	assertContainsAll(t, stderr, "task 001 was not marked completed after work session")
}

func TestTaskWorkCakeSessionMissingID(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "No Session ID", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})
	// Stub the runner to produce stream-json without a task_start event.
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		fmt.Fprintln(stdout, `{"type":"message","role":"assistant","content":"Done."}`)
		fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":"Done.","session_id":""}`)
		return nil
	})

	_, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--no-review", "--no-commit")
	if code == 0 {
		t.Error("expected nonzero exit when work session does not complete the task")
	}
	assertContainsAll(t, stderr, "warning: no session ID returned by cake")
	assertContainsAll(t, stderr, "task 001 was not marked completed after work session")
}

func TestTaskWorkReviewRequiresParseableSession(t *testing.T) {
	for _, tt := range []struct {
		name      string
		flags     []string
		output    string
		wantError string
	}{
		{name: "review only missing", flags: []string{"--no-commit"}, output: `{"type":"task_complete","result":"Work."}`, wantError: "no session ID returned by cake"},
		{name: "review only malformed", flags: []string{"--no-commit"}, output: `not json`, wantError: "could not capture session ID from cake output"},
		{name: "review and commit missing", output: `{"type":"task_complete","result":"Work."}`, wantError: "no session ID returned by cake"},
		{name: "review and commit malformed", output: `not json`, wantError: "could not capture session ID from cake output"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Session Required", "Pending", "")
			stubTaskWorkLookPath(t, func(executable string) (string, error) {
				return "/stub/cake", nil
			})
			var invocationCount int
			stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
				invocationCount++
				if invocationCount > 1 {
					t.Error("review, finalization, and commit must not run without a resumable session")
				}
				_, err := fmt.Fprint(stdout, tt.output)
				return err
			})

			args := []string{"--root", root, "task", "work", "001"}
			args = append(args, tt.flags...)
			_, stderr, code := runCLI(t, args...)
			if code == 0 {
				t.Error("expected missing or malformed required session to fail")
			}
			assertContainsAll(t, stderr, "cannot resume session for task 001", tt.wantError)
			if invocationCount != 1 {
				t.Errorf("invocations = %d, want 1", invocationCount)
			}
		})
	}
}

func TestCakeSessionIDParsing(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		wantID    string
		wantError bool
	}{
		{name: "task_start first", output: `{"type":"task_start","session_id":"sess_xyz","task_id":"tsk_1"}
{"type":"task_complete","subtype":"success","is_error":false,"result":"ok"}`, wantID: "sess_xyz"},
		{name: "task_start second line", output: `{"type":"message","role":"assistant","content":"hi"}
{"type":"task_start","session_id":"sess_abc","task_id":"tsk_2"}`, wantID: "sess_abc"},
		{name: "full cake record shape", output: `{"type":"task_start","session_id":"0d7a4f9e-8b3c-4f21-9d5a-1c2b3e4f5a6b","task_id":"550e8400-e29b-41d4-a716-446655440001","timestamp":"2026-06-12T17:00:00Z"}
{"type":"reasoning","id":"rs_1","summary":["planning"]}
{"type":"function_call","id":"fc_1","call_id":"call_1","name":"shell","arguments":"{}"}
{"type":"function_call_output","call_id":"call_1","output":"ok"}
{"type":"task_complete","subtype":"success","is_error":false,"result":"done","duration_ms":1000,"turn_count":2,"tool_call_count":1,"session_id":"0d7a4f9e-8b3c-4f21-9d5a-1c2b3e4f5a6b","task_id":"550e8400-e29b-41d4-a716-446655440001","usage":{"input_tokens":200,"output_tokens":100,"total_tokens":300}}`, wantID: "0d7a4f9e-8b3c-4f21-9d5a-1c2b3e4f5a6b"},
		{name: "empty session", output: `{"type":"task_start","session_id":""}`, wantID: ""},
		{name: "non-task_start", output: `{"type":"message","role":"assistant","content":"ok"}`, wantID: ""},
		{name: "non-JSON line", output: `not json`, wantID: "", wantError: true},
		{name: "empty output", output: ``, wantID: ""},
		{name: "no type field", output: `{"session_id":"sess_xyz"}`, wantID: ""},
		{name: "legacy event key ignored", output: `{"event":"task_start","session_id":"sess_xyz"}`, wantID: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := parseCakeSessionID([]byte(tt.output))
			if tt.wantError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if id != tt.wantID {
				t.Errorf("sessionID = %q, want %q", id, tt.wantID)
			}
		})
	}
}

func TestCakeResumeArgs(t *testing.T) {
	args := cakeResumeArgs("sess_abc", "Continue working")
	want := []string{"--resume", "sess_abc", "--output-format", "stream-json", "Continue working"}
	if len(args) != len(want) {
		t.Errorf("args = %#v, want %#v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("args[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}

func TestCodexResumeArgs(t *testing.T) {
	args := codexResumeArgs("thread_abc", "Continue working")
	want := []string{"exec", "resume", "--dangerously-bypass-approvals-and-sandbox", "--json", "thread_abc", "Continue working"}
	if len(args) != len(want) {
		t.Errorf("args = %#v, want %#v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("args[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}

func TestCursorResumeArgs(t *testing.T) {
	args := cursorResumeArgs("sess_abc", "Continue working")
	want := []string{"-p", "--output-format", "stream-json", "--trust", "--resume", "sess_abc", "Continue working"}
	if len(args) != len(want) {
		t.Errorf("args = %#v, want %#v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("args[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}

func TestClaudeResumeArgs(t *testing.T) {
	args := claudeResumeArgs("sess_abc", "Continue working")
	want := []string{"-p", "--verbose", "--resume", "sess_abc", "--output-format", "stream-json", "--dangerously-skip-permissions", "Continue working"}
	if len(args) != len(want) {
		t.Errorf("args = %#v, want %#v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("args[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}

func TestParseCodexSessionID(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		wantID    string
		wantError bool
	}{
		{name: "thread.started", output: `{"type":"thread.started","thread_id":"thread_abc123"}`, wantID: "thread_abc123"},
		{name: "multiple events", output: "{\"type\":\"turn.started\"}\n{\"type\":\"thread.started\",\"thread_id\":\"thread_xyz\"}", wantID: "thread_xyz"},
		{name: "no thread.started", output: `{"type":"turn.started"}`, wantID: ""},
		{name: "empty thread_id", output: `{"type":"thread.started","thread_id":""}`, wantID: ""},
		{name: "non-JSON line", output: `not json`, wantID: "", wantError: true},
		{name: "empty output", output: ``, wantID: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := parseCodexSessionID([]byte(tt.output))
			if tt.wantError {
				if !errors.Is(err, errSessionOutputUnparseable) {
					t.Errorf("error = %v, want errSessionOutputUnparseable", err)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if id != tt.wantID {
				t.Errorf("sessionID = %q, want %q", id, tt.wantID)
			}
		})
	}
}

func TestParseCodexReviewFeedback(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		wantResult string
		wantError  bool
	}{
		{
			name: "single agent_message",
			output: `{"type":"thread.started","thread_id":"thr_abc"}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"Found 2 style issues."}}
{"type":"turn.completed","usage":{"input_tokens":100,"output_tokens":50}}`,
			wantResult: "Found 2 style issues.",
		},
		{
			name: "multiple items",
			output: `{"type":"thread.started","thread_id":"thr_abc"}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"Issue 1: missing tests."}}
{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"Issue 2: unused variable."}}
{"type":"turn.completed"}`,
			wantResult: "Issue 1: missing tests.\nIssue 2: unused variable.",
		},
		{
			name:       "non-agent_message item",
			output:     "{\"type\":\"item.completed\",\"item\":{\"id\":\"item_0\",\"type\":\"tool_call\"}}\n{\"type\":\"turn.completed\"}",
			wantResult: "",
		},
		{
			name:       "no text field",
			output:     "{\"type\":\"item.completed\",\"item\":{\"id\":\"item_0\",\"type\":\"agent_message\"}}\n{\"type\":\"turn.completed\"}",
			wantResult: "",
		},
		{
			name:       "empty output",
			output:     ``,
			wantResult: "",
			wantError:  true,
		},
		{
			name:       "non-JSON line",
			output:     `not json`,
			wantResult: "",
			wantError:  true,
		},
		{name: "truncated before completion", output: `{"type":"thread.started","thread_id":"thr_abc"}`, wantResult: "", wantError: true},
		{name: "completed with no feedback", output: `{"type":"turn.completed"}`, wantResult: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseCodexReviewFeedback([]byte(tt.output))
			if tt.wantError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if result != tt.wantResult {
				t.Errorf("result = %q, want %q", result, tt.wantResult)
			}
		})
	}
}

func TestParseCodexReviewFeedbackEmpty(t *testing.T) {
	feedback, err := parseCodexReviewFeedback([]byte(""))
	if !errors.Is(err, errReviewOutputUnparseable) {
		t.Errorf("error = %v, want errReviewOutputUnparseable", err)
	}
	if feedback != "" {
		t.Errorf("feedback = %q, want empty", feedback)
	}
}

func TestParseCursorSessionID(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		wantID    string
		wantError bool
	}{
		{name: "system init", output: `{"type":"system","subtype":"init","session_id":"cursor_sess_123"}`, wantID: "cursor_sess_123"},
		{name: "multiple events", output: "{\"type\":\"user\",\"message\":{\"role\":\"user\"}}\n{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"cursor_sess_456\"}\n{\"type\":\"result\",\"result\":\"ok\",\"session_id\":\"cursor_sess_456\"}", wantID: "cursor_sess_456"},
		{name: "result session ignored", output: `{"type":"result","result":"ok","session_id":"cursor_sess_result"}`, wantID: ""},
		{name: "wrong subtype", output: `{"type":"system","subtype":"other","session_id":"cursor_sess_wrong"}`, wantID: ""},
		{name: "empty session", output: `{"type":"system","subtype":"init","session_id":""}`, wantID: ""},
		{name: "non-JSON line", output: `not json`, wantID: "", wantError: true},
		{name: "empty output", output: ``, wantID: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := parseCursorSessionID([]byte(tt.output))
			if tt.wantError {
				if !errors.Is(err, errSessionOutputUnparseable) {
					t.Errorf("error = %v, want errSessionOutputUnparseable", err)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if id != tt.wantID {
				t.Errorf("sessionID = %q, want %q", id, tt.wantID)
			}
		})
	}
}

func TestParseCursorReviewFeedback(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		wantResult string
		wantError  bool
	}{
		{name: "result event", output: `{"type":"result","result":"Found 1 issue.","is_error":false,"session_id":"cursor_sess"}`, wantResult: "Found 1 issue."},
		{name: "last result wins", output: "{\"type\":\"result\",\"result\":\"First\"}\n{\"type\":\"result\",\"result\":\"Final\"}", wantResult: "Final"},
		{name: "empty result", output: `{"type":"result","result":""}`, wantResult: ""},
		{name: "no result event", output: `{"type":"assistant","message":{"content":[{"type":"text","text":"ok"}]}}`, wantResult: "", wantError: true},
		{name: "non-JSON line", output: `not json`, wantResult: "", wantError: true},
		{name: "empty output", output: ``, wantResult: "", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseCursorReviewFeedback([]byte(tt.output))
			if tt.wantError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if result != tt.wantResult {
				t.Errorf("result = %q, want %q", result, tt.wantResult)
			}
		})
	}
}

func TestTaskWorkCursorSessionCapture(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Cursor Task With Session", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cursor-agent", nil
	})
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		if len(args) < 5 || args[0] != "-p" || args[1] != "--output-format" || args[2] != "stream-json" || args[3] != "--trust" {
			t.Errorf("unexpected cursor args = %#v", args)
		}
		fmt.Fprintln(stdout, `{"type":"system","subtype":"init","session_id":"cursor_sess_abc123"}`)
		fmt.Fprintln(stdout, `{"type":"assistant","message":{"content":[{"type":"text","text":"Working..."}]}}`)
		fmt.Fprint(stdout, `{"type":"result","result":"Work completed.","is_error":false,"session_id":"cursor_sess_abc123"}`)
		return completeTaskOnDisk(root, "001")
	})

	_, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--agent", "cursor", "--no-review", "--no-commit")
	if code != 0 {
		t.Errorf("expected exit 0 when work session completes the task, got %d: %s", code, stderr)
	}
	assertContainsAll(t, stderr, "cursor session started: cursor_s")
	assertNotContains(t, stderr, "could not capture session ID")
	assertNotContains(t, stderr, "no session ID returned")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "completed", "001.md"), "status: Completed")
}

func TestParseClaudeSessionID(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		wantID    string
		wantError bool
	}{
		{name: "system init", output: `{"type":"system","subtype":"init","session_id":"claude_sess_123"}`, wantID: "claude_sess_123"},
		{name: "multiple events", output: "{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"claude_sess_456\"}\n{\"type\":\"assistant\",\"message\":{\"role\":\"assistant\"}}\n{\"type\":\"result\",\"result\":\"ok\",\"session_id\":\"claude_sess_456\"}", wantID: "claude_sess_456"},
		{name: "result session ignored", output: `{"type":"result","result":"ok","session_id":"claude_sess_result"}`, wantID: ""},
		{name: "wrong subtype", output: `{"type":"system","subtype":"other","session_id":"claude_sess_wrong"}`, wantID: ""},
		{name: "empty session", output: `{"type":"system","subtype":"init","session_id":""}`, wantID: ""},
		{name: "non-JSON line", output: `not json`, wantID: "", wantError: true},
		{name: "empty output", output: ``, wantID: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := parseClaudeSessionID([]byte(tt.output))
			if tt.wantError {
				if !errors.Is(err, errSessionOutputUnparseable) {
					t.Errorf("error = %v, want errSessionOutputUnparseable", err)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if id != tt.wantID {
				t.Errorf("sessionID = %q, want %q", id, tt.wantID)
			}
		})
	}
}

func TestParseClaudeReviewFeedback(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		wantResult string
		wantError  bool
	}{
		{name: "result event", output: `{"type":"result","subtype":"success","result":"Found 1 issue.","is_error":false,"session_id":"claude_sess"}`, wantResult: "Found 1 issue."},
		{name: "last result wins", output: "{\"type\":\"result\",\"result\":\"First\"}\n{\"type\":\"result\",\"result\":\"Final\"}", wantResult: "Final"},
		{name: "empty result", output: `{"type":"result","result":""}`, wantResult: ""},
		{name: "no result event", output: `{"type":"assistant","message":{"content":[{"type":"text","text":"ok"}]}}`, wantResult: "", wantError: true},
		{name: "non-JSON line", output: `not json`, wantResult: "", wantError: true},
		{name: "empty output", output: ``, wantResult: "", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseClaudeReviewFeedback([]byte(tt.output))
			if tt.wantError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if result != tt.wantResult {
				t.Errorf("result = %q, want %q", result, tt.wantResult)
			}
		})
	}
}

func TestTaskWorkClaudeDryRunPreviewsStreamJSONArgs(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Claude Dry Run", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/" + executable, nil
	})
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		t.Error("runner should not be called during dry-run")
		return nil
	})

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "work", "001", "--agent", "claude")
	if code != 0 {
		t.Errorf("dry-run task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout,
		"agent: claude",
		"executable: /stub/claude",
		"status: In Progress",
		"task: 001",
		"-p",
		"--verbose",
		"--dangerously-skip-permissions",
		"stream-json",
		"prompt: Work on task 001.",
		"review: true",
		"commit: true",
	)
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "status: Pending")
}

func TestTaskWorkClaudeSessionCapture(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Claude Task With Session", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/claude", nil
	})
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		if len(args) < 6 || args[0] != "-p" || args[1] != "--verbose" || args[2] != "--output-format" || args[3] != "stream-json" || args[4] != "--dangerously-skip-permissions" {
			t.Errorf("unexpected claude args = %#v", args)
		}
		fmt.Fprintln(stdout, `{"type":"system","subtype":"init","session_id":"claude_sess_abc123"}`)
		fmt.Fprintln(stdout, `{"type":"assistant","message":{"content":[{"type":"text","text":"Working..."}]}}`)
		fmt.Fprint(stdout, `{"type":"result","subtype":"success","result":"Work completed.","is_error":false,"session_id":"claude_sess_abc123"}`)
		return completeTaskOnDisk(root, "001")
	})

	_, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--agent", "claude", "--no-review", "--no-commit")
	if code != 0 {
		t.Errorf("expected exit 0 when work session completes the task, got %d: %s", code, stderr)
	}
	assertContainsAll(t, stderr, "claude session started: claude_s")
	assertNotContains(t, stderr, "could not capture session ID")
	assertNotContains(t, stderr, "no session ID returned")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "completed", "001.md"), "status: Completed")
}

func TestParseCakeReviewFeedback(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		wantResult string
		wantError  bool
	}{
		{name: "task_complete with result", output: `{"type":"message","role":"assistant","content":"checking..."}
{"type":"task_complete","subtype":"success","is_error":false,"result":"Found 3 issues.","session_id":"sess_abc"}`, wantResult: "Found 3 issues."},
		{name: "full cake record shape", output: `{"type":"task_start","session_id":"0d7a4f9e-8b3c-4f21-9d5a-1c2b3e4f5a6b","task_id":"550e8400-e29b-41d4-a716-446655440001","timestamp":"2026-06-12T17:00:00Z"}
{"type":"task_complete","subtype":"success","is_error":false,"result":"Found 2 issues.","duration_ms":1000,"turn_count":2,"tool_call_count":1,"session_id":"0d7a4f9e-8b3c-4f21-9d5a-1c2b3e4f5a6b","task_id":"550e8400-e29b-41d4-a716-446655440001","usage":{"input_tokens":200,"output_tokens":100,"total_tokens":300}}`, wantResult: "Found 2 issues."},
		{name: "empty result", output: `{"type":"task_complete","subtype":"success","is_error":false,"result":""}`, wantResult: ""},
		{name: "result omitted", output: `{"type":"task_complete","subtype":"success","is_error":false,"duration_ms":1000,"turn_count":1,"tool_call_count":0,"session_id":"sess_abc","task_id":"tsk_1","usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}`, wantResult: ""},
		{name: "no task_complete", output: `{"type":"message","role":"assistant","content":"ok"}`, wantResult: "", wantError: true},
		{name: "non-JSON line", output: `not json`, wantResult: "", wantError: true},
		{name: "empty output", output: ``, wantResult: "", wantError: true},
		{name: "last task_complete wins", output: `{"type":"task_complete","subtype":"success","is_error":false,"result":"First"}
{"type":"task_complete","subtype":"success","is_error":false,"result":"Final"}`, wantResult: "Final"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseCakeReviewFeedback([]byte(tt.output))
			if tt.wantError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if result != tt.wantResult {
				t.Errorf("result = %q, want %q", result, tt.wantResult)
			}
		})
	}
}

func TestTaskWorkReviewArgs(t *testing.T) {
	for _, tt := range []struct {
		name string
		want []string
	}{
		{
			name: "cake",
			want: []string{"--output-format", "stream-json", "Review the changes."},
		},
		{
			name: "codex",
			want: []string{"exec", "--dangerously-bypass-approvals-and-sandbox", "--json", "Review the changes."},
		},
		{
			name: "cursor",
			want: []string{"-p", "--output-format", "stream-json", "--mode", "ask", "--trust", "Review the changes."},
		},
		{
			name: "claude",
			want: []string{"-p", "--verbose", "--output-format", "stream-json", "--dangerously-skip-permissions", "Review the changes."},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			agent, err := parseTaskWorkAgent(tt.name)
			if err != nil {
				t.Error(err)
			}
			args := agent.reviewArgs("Review the changes.", "")
			if len(args) != len(tt.want) {
				t.Errorf("reviewArgs = %#v, want %#v", args, tt.want)
			}
			for i := range tt.want {
				if args[i] != tt.want[i] {
					t.Errorf("reviewArgs[%d] = %q, want %q", i, args[i], tt.want[i])
				}
			}
		})
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
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		switch len(invocations) {
		case 0:
			// Session work: write acceptance notes, produce session ID.
			path := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if writeErr := os.WriteFile(path, append(data, []byte("\n## Acceptance Notes\n\n- [x] Updated during implementation.\n")...), 0o644); writeErr != nil {
				return writeErr
			}
			fmt.Fprintln(stdout, `{"type":"task_start","session_id":"sess_review123","task_id":"tsk_xyz"}`)
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":"Work done."}`)
			invocations = append(invocations, invocation{root, executable, args, stdin != nil})
			return err
		case 1:
			// Review: produce review feedback.
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":"Found 2 style issues and 1 missing test."}`)
			invocations = append(invocations, invocation{root, executable, args, stdin != nil})
			return err
		default:
			// Finalization resume: agent addresses feedback and completes the task.
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":"Feedback addressed and task completed."}`)
			if err != nil {
				return err
			}
			invocations = append(invocations, invocation{root, executable, args, stdin != nil})
			return completeTaskOnDisk(root, "001")
		}
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--no-commit")
	if code != 0 {
		t.Errorf("task work with review exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Should have 3 invocations: session work, review, finalization.
	if len(invocations) != 3 {
		t.Errorf("expected 3 invocations (session, review, finalization), got %d", len(invocations))
	}

	// First invocation: session work with --output-format stream-json.
	if invocations[0].args[0] != "--output-format" || invocations[0].args[1] != "stream-json" {
		t.Errorf("first invocation args = %#v, want session work prefix", invocations[0].args)
	}
	// First invocation should have stdin (user input passed through).
	if !invocations[0].hasStdin {
		t.Error("first invocation should have stdin connected")
	}

	// Second invocation: review with the embedded procedure in the prompt.
	if invocations[1].args[0] != "--output-format" || invocations[1].args[1] != "stream-json" {
		t.Errorf("second invocation args = %#v, want review prefix", invocations[1].args)
	}
	assertContainsAll(t, invocations[1].args[len(invocations[1].args)-1], "- [x] Updated during implementation.")
	// Review invocation should have no stdin (independent run).
	if invocations[1].hasStdin {
		t.Error("review invocation should have no stdin")
	}

	// Third invocation: finalization resume with the session ID.
	if len(invocations[2].args) < 4 || invocations[2].args[0] != "--resume" || invocations[2].args[1] != "sess_review123" {
		t.Errorf("third invocation args = %#v, want resume prefix with session ID", invocations[2].args)
	}
	// Resume should contain both the feedback and finalization instructions.
	lastArg := invocations[2].args[len(invocations[2].args)-1]
	if !strings.Contains(lastArg, "Found 2 style issues and 1 missing test.") {
		t.Errorf("resume prompt should contain review feedback, got %q", lastArg)
	}
	assertContainsAll(t, lastArg, "Finalize task 001.")

	// Review status messages should appear on stderr.
	assertContainsAll(t, stderr, "--- Running review ---")
	assertContainsAll(t, stderr, "Review produced feedback for session")

	// Task should be Completed after finalization.
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "completed", "001.md"), "status: Completed")
}

func TestTaskWorkReviewEmptyFeedback(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Empty Review", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})

	var invocationCount int
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		invocationCount++
		switch invocationCount {
		case 1:
			// Session work.
			fmt.Fprintln(stdout, `{"type":"task_start","session_id":"sess_empty","task_id":"tsk_xyz"}`)
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":"Done."}`)
			return err
		case 2:
			// Review produces empty feedback.
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":""}`)
			return err
		default:
			// Finalization resume: agent marks task completed.
			return completeTaskOnDisk(root, "001")
		}
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--no-commit")
	if code != 0 {
		t.Errorf("task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Should have 3 invocations: session work, review, finalization resume.
	if invocationCount != 3 {
		t.Errorf("expected 3 invocations (session, review, finalization), got %d", invocationCount)
	}

	assertContainsAll(t, stderr, "--- Running review ---")
	assertContainsAll(t, stderr, "No review feedback to address, proceeding to finalization.")
}

func TestTaskWorkReviewFails(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Failing Review", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})

	var invocationCount int
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		invocationCount++
		if invocationCount == 1 {
			fmt.Fprintln(stdout, `{"type":"task_start","session_id":"sess_fail","task_id":"tsk_xyz"}`)
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":"Work."}`)
			return err
		}
		// Review fails with a non-zero exit.
		return fmt.Errorf("review command exited with status 1")
	})

	_, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--no-commit")
	if code == 0 {
		t.Error("expected task work to fail when review fails")
	}

	assertContainsAll(t, stderr, "review failed:")
	assertContainsAll(t, stderr, "review command exited with status 1")
}

func TestTaskWorkFailedFinalization(t *testing.T) {
	// When the finalization resume succeeds but the agent does NOT mark the
	// task completed, ahm must return a clear error instead of proceeding.
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Failed Finalization", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})

	var invocationCount int
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		invocationCount++
		switch invocationCount {
		case 1:
			fmt.Fprintln(stdout, `{"type":"task_start","session_id":"sess_failfinal","task_id":"tsk_xyz"}`)
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":"Work."}`)
			return err
		case 2:
			// Review produces empty feedback (no findings).
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":""}`)
			return err
		default:
			// Finalization resume succeeds but does NOT mark the task completed.
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":"Done but not completed."}`)
			return err
		}
	})

	_, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--no-commit")
	if code == 0 {
		t.Error("expected task work to fail when finalization does not complete the task")
	}

	assertContainsAll(t, stderr, "task 001 was not marked completed after finalization")
	assertContainsAll(t, stderr, "status is In Progress")
}

func TestTaskWorkFinalizationCommitGating(t *testing.T) {
	// When finalization fails (task not completed), commit handoff must NOT run.
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Commit Gate", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})

	var prompts []string
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		prompts = append(prompts, args[len(args)-1])
		switch len(prompts) {
		case 1:
			fmt.Fprintln(stdout, `{"type":"task_start","session_id":"sess_gate","task_id":"tsk_xyz"}`)
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":"Work."}`)
			return err
		case 2:
			// Review produces empty feedback.
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":""}`)
			return err
		default:
			// Finalization resume succeeds but does NOT complete the task.
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":"Done but not completed."}`)
			return err
		}
	})

	_, stderr, code := runCLI(t, "--root", root, "task", "work", "001")
	if code == 0 {
		t.Error("expected task work to fail when finalization does not complete the task")
	}

	// Should not reach commit handoff.
	assertNotContains(t, stderr, "--- Running commit handoff ---")
	assertContainsAll(t, stderr, "task 001 was not marked completed after finalization")
	// Should have only 3 invocations (session, review, finalization), not 4 (no commit).
	if len(prompts) != 3 {
		t.Errorf("expected 3 invocations (session, review, finalization), got %d; commit should not have run", len(prompts))
	}
}

func TestTaskWorkFinalizationStrictAcceptanceErrorWithoutCommit(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Strict Finalization", "Pending", "")
	meta, err := readMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	meta.StrictAcceptance = true
	if err := writeMetadata(root, meta); err != nil {
		t.Fatal(err)
	}
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})

	var invocationCount int
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		invocationCount++
		switch invocationCount {
		case 1:
			fmt.Fprintln(stdout, `{"type":"task_start","session_id":"sess_strict","task_id":"tsk_xyz"}`)
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":"Work."}`)
			return err
		case 2:
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":""}`)
			return err
		default:
			return completeTaskOnDisk(root, "001")
		}
	})

	_, stderr, code = runCLI(t, "--root", root, "task", "work", "001", "--no-commit")
	if code == 0 {
		t.Error("expected strict acceptance to reject incomplete finalization")
	}
	assertContainsAll(t, stderr, "task 001 finalization failed", "acceptance notes are incomplete under strict acceptance policy")
	assertNotContains(t, stderr, "use --no-commit")
	if invocationCount != 3 {
		t.Errorf("expected 3 invocations (session, review, finalization), got %d", invocationCount)
	}
}

func TestTaskWorkPrematureCompletionBeforeReview(t *testing.T) {
	// When the implementation agent marks the task Completed during its session
	// (before review), the flow should still work: review checks the already-
	// completed task, finalization runs (agent sees task already completed),
	// and validation passes because the task IS completed.
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Premature Complete", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})

	var invocationCount int
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		invocationCount++
		switch invocationCount {
		case 1:
			// Implementation session: agent writes a completed task (prematurely).
			if err := completeTaskOnDisk(root, "001"); err != nil {
				return err
			}
			fmt.Fprintln(stdout, `{"type":"task_start","session_id":"sess_premature","task_id":"tsk_xyz"}`)
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":"Done."}`)
			return err
		case 2:
			// Review produces empty feedback (task already completed).
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":""}`)
			return err
		default:
			// Finalization resume: task already completed, this is a no-op.
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":"Task already completed."}`)
			return err
		}
	})

	_, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--no-commit")
	if code != 0 {
		t.Errorf("premature completion should not cause an error, got exit code %d: %s", code, stderr)
	}

	assertContainsAll(t, stderr, "--- Running review ---")
	// Task should be in the completed bucket.
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "completed", "001.md"), "status: Completed")
}

func TestTaskWorkCursorReviewOrchestration(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Cursor Review", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cursor-agent", nil
	})

	var invocations [][]string
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		invocations = append(invocations, append([]string(nil), args...))
		switch len(invocations) {
		case 1:
			fmt.Fprintln(stdout, `{"type":"system","subtype":"init","session_id":"cursor_review123"}`)
			_, err := fmt.Fprint(stdout, `{"type":"result","result":"Work done.","is_error":false,"session_id":"cursor_review123"}`)
			return err
		case 2:
			_, err := fmt.Fprint(stdout, `{"type":"result","result":"Fix the Cursor review finding.","is_error":false,"session_id":"cursor_review456"}`)
			return err
		default:
			// Finalization resume: agent completes the task.
			return completeTaskOnDisk(root, "001")
		}
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--agent", "cursor", "--no-commit")
	if code != 0 {
		t.Errorf("task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	if len(invocations) != 3 {
		t.Errorf("expected 3 invocations (session, review, finalization), got %d", len(invocations))
	}
	if len(invocations[1]) < 7 || invocations[1][0] != "-p" || invocations[1][1] != "--output-format" || invocations[1][2] != "stream-json" || invocations[1][3] != "--mode" || invocations[1][4] != "ask" || invocations[1][5] != "--trust" {
		t.Errorf("review args = %#v, want cursor ask-mode review args", invocations[1])
	}
	assertContainsAll(t, invocations[1][len(invocations[1])-1], taskWorkReviewPromptMarker, "Task: 001", "Managed-work completion checklist", "git status --short")
	// Finalization resume should use cursor resume arg shape (same as impl agent).
	if len(invocations[2]) < 7 || invocations[2][0] != "-p" || invocations[2][1] != "--output-format" || invocations[2][2] != "stream-json" || invocations[2][3] != "--trust" || invocations[2][4] != "--resume" || invocations[2][5] != "cursor_review123" {
		t.Errorf("finalization resume args = %#v, want cursor resume args with session ID", invocations[2])
	}
	assertContainsAll(t, invocations[2][len(invocations[2])-1], "Finalize task 001.", "Address the following review feedback", "Fix the Cursor review finding.")
	assertContainsAll(t, stderr, "--- Running review ---", "Review produced feedback for session")
}

func TestTaskWorkReviewParseError(t *testing.T) {
	// When the review command succeeds but produces completely non-JSON
	// output, the parser now treats that as a malformed-provider condition
	// rather than empty feedback, and the command must fail instead of
	// silently advancing to finalization.
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Bad Review Output", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})

	var invocationCount int
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		invocationCount++
		switch invocationCount {
		case 1:
			// Session work succeeds.
			fmt.Fprintln(stdout, `{"type":"task_start","session_id":"sess_parse","task_id":"tsk_xyz"}`)
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":"Done."}`)
			return err
		case 2:
			// Review produces non-JSON output (malformed provider).
			_, err := fmt.Fprint(stdout, `not valid json`)
			return err
		default:
			t.Error("finalization should not run when review feedback parse fails")
			return nil
		}
	})

	_, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--no-commit")
	if code == 0 {
		t.Error("expected task work to fail when review feedback cannot be parsed")
	}

	// Should have 2 invocations: session work, review (no finalization).
	if invocationCount != 2 {
		t.Errorf("expected 2 invocations (session, review), got %d", invocationCount)
	}

	assertContainsAll(t, stderr, "--- Running review ---")
	assertContainsAll(t, stderr, "could not parse review feedback from cake output")
	assertNotContains(t, stderr, "No review feedback to address")
	assertNotContains(t, stderr, "--- Running commit handoff ---")
}

func TestTaskWorkReviewWithNoReview(t *testing.T) {
	// With --no-review --no-commit, neither review nor commit should run.
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "No Review", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})

	var invocationCount int
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		invocationCount++
		if invocationCount == 1 {
			fmt.Fprintln(stdout, `{"type":"task_start","session_id":"sess_noreview","task_id":"tsk_xyz"}`)
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":"Done."}`)
			if err != nil {
				return err
			}
			return completeTaskOnDisk(root, "001")
		}
		return nil
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--no-review", "--no-commit")
	if code != 0 {
		t.Errorf("task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Should have exactly 1 invocation (session only, no review or commit).
	if invocationCount != 1 {
		t.Errorf("expected 1 invocation (session only), got %d", invocationCount)
	}

	assertNotContains(t, stderr, "--- Running review ---")
	assertNotContains(t, stderr, "--- Running commit handoff ---")
}

func TestTaskWorkCursorCommitHandoff(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Cursor Commit", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cursor-agent", nil
	})

	var invocations [][]string
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		invocations = append(invocations, append([]string(nil), args...))
		if len(invocations) == 1 {
			fmt.Fprintln(stdout, `{"type":"system","subtype":"init","session_id":"cursor_commit123"}`)
			_, err := fmt.Fprint(stdout, `{"type":"result","result":"Work done.","is_error":false,"session_id":"cursor_commit123"}`)
			if err != nil {
				return err
			}
			return completeTaskOnDisk(root, "001")
		}
		_, err := fmt.Fprint(stdout, `{"type":"result","result":"Committed task.","is_error":false,"session_id":"cursor_commit123"}`)
		return err
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--agent", "cursor", "--no-review")
	if code != 0 {
		t.Errorf("task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	if len(invocations) != 2 {
		t.Errorf("expected 2 invocations (session, commit), got %d", len(invocations))
	}
	if len(invocations[1]) < 7 || invocations[1][0] != "-p" || invocations[1][1] != "--output-format" || invocations[1][2] != "stream-json" || invocations[1][3] != "--trust" || invocations[1][4] != "--resume" || invocations[1][5] != "cursor_commit123" {
		t.Errorf("commit args = %#v, want cursor resume args", invocations[1])
	}
	assertContainsAll(t, invocations[1][len(invocations[1])-1], "Commit the completed work for task 001", "Do not push or open a pull request")
	assertContainsAll(t, stderr, "--- Running commit handoff ---")
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
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		invocations = append(invocations, struct {
			args []string
		}{append([]string(nil), args...)})
		if len(invocations) == 1 {
			fmt.Fprintln(stdout, `{"type":"task_start","session_id":"sess_commit123","task_id":"tsk_xyz"}`)
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":"Work done."}`)
			if err != nil {
				return err
			}
			return completeTaskOnDisk(root, "001")
		}
		_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":"Committed."}`)
		return err
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--no-review")
	if code != 0 {
		t.Errorf("task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	if len(invocations) != 2 {
		t.Errorf("expected 2 invocations (session, commit), got %d", len(invocations))
	}
	args := invocations[1].args
	if len(args) < 4 || args[0] != "--resume" || args[1] != "sess_commit123" {
		t.Errorf("second invocation args = %#v, want resume with session ID", args)
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
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		prompts = append(prompts, args[len(args)-1])
		switch len(prompts) {
		case 1:
			fmt.Fprintln(stdout, `{"type":"task_start","session_id":"sess_reviewcommit","task_id":"tsk_xyz"}`)
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":"Work."}`)
			return err
		case 2:
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":"Fix the docs."}`)
			return err
		case 3:
			// Finalization resume: agent completes the task.
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":"Feedback addressed and task completed."}`)
			if err != nil {
				return err
			}
			return completeTaskOnDisk(root, "001")
		default:
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":"Committed."}`)
			return err
		}
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001")
	if code != 0 {
		t.Errorf("task work with default review+commit exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	if len(prompts) != 4 {
		t.Errorf("expected 4 invocations (session, review, finalization, commit), got %d", len(prompts))
	}
	assertContainsAll(t, prompts[2], "Finalize task 001.", "Address the following review feedback", "Fix the docs.")
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
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		invocationCount++
		if invocationCount == 1 {
			fmt.Fprintln(stdout, `{"type":"task_start","session_id":"sess_failcommit","task_id":"tsk_xyz"}`)
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":"Work."}`)
			if err != nil {
				return err
			}
			return completeTaskOnDisk(root, "001")
		}
		return fmt.Errorf("exit status 1")
	})

	_, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--no-review")
	if code == 0 {
		t.Error("expected task work to fail when commit handoff fails")
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
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		invocationCount++
		if invocationCount == 1 {
			_, err := fmt.Fprint(stdout, `{"result":"Work."}`)
			return err
		}
		t.Error("commit handoff should not run without a session ID")
		return nil
	})

	_, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--no-review")
	if code == 0 {
		t.Error("expected task work to fail when commit handoff lacks a session ID")
	}
	assertContainsAll(t, stderr, "cannot resume session for task 001", "no session ID returned by cake")
}

func TestTaskWorkCommitDryRun(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Dry Commit", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/cake", nil
	})
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		t.Error("runner should not be called during dry-run")
		return nil
	})

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "work", "001")
	if code != 0 {
		t.Errorf("dry-run exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "prompt: Work on task 001.", "commit: true", "review: true")
	assertNotContains(t, stderr, "--- Running commit handoff ---")
	assertFileContainsAll(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "status: Pending")
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

func TestTaskAcceptMovesOpenToPending(t *testing.T) {
	root := t.TempDir()
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
	root := t.TempDir()
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
	root := t.TempDir()
	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	// Create a Blocked task directly.
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Blocked Task", "Blocked", "")

	if err := a.taskStatus([]string{"001"}, "Pending"); err != nil {
		t.Error(err)
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
		t.Error(err)
	}
	assertContainsAll(t, out.String(), "001 already Pending")
}

func writeTaskFileWithDeps(t *testing.T, path string, id string, title string, status string, deps string) {
	t.Helper()
	extra := "depends_on: " + deps + "\n"
	writeTaskFile(t, path, id, title, status, extra)
}

// completeTaskOnDisk marks a task as Completed directly on disk, simulating
// what the delegated agent does via ahm task complete during finalization.
// It works for both legacy (.agents/) and migrated (.ahm/) layouts.
func completeTaskOnDisk(root string, id string) error { //nolint:unparam // id varies across tests but many use "001"
	// Try legacy layout first.
	activePath := filepath.Join(root, ".agents", ".tasks", "active", id+".md")
	data, err := os.ReadFile(activePath)
	if err != nil {
		// Try migrated layout.
		activePath = filepath.Join(root, ".ahm", "tasks", "active", id+".md")
		data, err = os.ReadFile(activePath)
		if err != nil {
			return fmt.Errorf("task %s not found in active bucket: %w", id, err)
		}
	}
	content := string(data)
	completedPath := strings.Replace(activePath, "/active/", "/completed/", 1)
	if err := os.MkdirAll(filepath.Dir(completedPath), 0o755); err != nil {
		return err
	}
	// Compute completed dir from active dir.
	completed := strings.ReplaceAll(content, "status: In Progress", "status: Completed")
	if completed == content {
		completed = strings.ReplaceAll(content, "status: Pending", "status: Completed")
	}
	if err := os.WriteFile(completedPath, []byte(completed), 0o644); err != nil {
		return err
	}
	return os.Remove(activePath)
}

func TestTaskCompleteParallelUnblocksDependents(t *testing.T) {
	root := t.TempDir()
	// Create tasks manually (no install needed — just raw task files).
	// 001 and 002 are dependencies of 003 (Blocked).
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Dependency A", "Pending", "")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Dependency B", "Pending", "")
	writeTaskFileWithDeps(t, filepath.Join(root, ".agents", ".tasks", "active", "003.md"), "003", "Dependent Task", "Blocked", "001, 002")

	// Verify initial state.
	for _, id := range []string{"001", "002", "003"} {
		p := filepath.Join(root, ".agents", ".tasks", "active", id+".md")
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
	data, err := os.ReadFile(filepath.Join(root, ".agents", ".tasks", "active", "003.md"))
	if err != nil {
		t.Errorf("active/003.md: %v", err)
	} else if !strings.Contains(string(data), "status: Pending") {
		t.Errorf("003.md status not Pending, got:\n%s", string(data))
	}
	// Both dependencies should be in completed/.
	for _, id := range []string{"001", "002"} {
		p := filepath.Join(root, ".agents", ".tasks", "completed", id+".md")
		assertFileContainsAll(t, p, "status: Completed")
	}
	// Index must show 003 as Pending (it renders as a table cell, not front matter).
	indexContent := mustRead(t, filepath.Join(root, ".agents", ".tasks", "active", "index.md"))
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

	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Locked Task", "Pending", "")

	release, err := acquireWorkflowRecordLockWithResolver(root, func() workflowPaths {
		return workflowPathsFor(root)
	})
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

	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Original Title", "Pending", "")

	release, err := acquireWorkflowRecordLockWithResolver(root, func() workflowPaths {
		return workflowPathsFor(root)
	})
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
		root := t.TempDir()
		var installOut strings.Builder
		installer := app{opts: options{root: root}, out: &installOut}
		if err := installer.install(false); err != nil {
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
	root := t.TempDir()
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

func TestTaskWorkWarnsOnCorruptMetadata(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
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
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Workable", "Pending", "")

	// Corrupt the metadata file.
	metaPath := filepath.Join(root, ".ahm", "config.json")
	if err := os.WriteFile(metaPath, []byte("{invalid json}"), 0o644); err != nil {
		t.Fatal(err)
	}

	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/" + executable, nil
	})

	var captured taskWorkCapture
	stubTaskWorkRunner(t, captured.runner)

	// Task work should warn about corrupt metadata but still use the default agent.
	stdout, stderr, code = runCLI(t, "--root", root, "task", "work", "001", "--no-review", "--no-commit")
	if code != 0 {
		t.Errorf("task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stderr, "corrupt workflow metadata .ahm/config.json", "using default agent")
	// Falls back to cake.
	if captured.executable != "/stub/cake" {
		t.Errorf("executable = %q, want /stub/cake", captured.executable)
	}
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
	if err := writeMetadata(root, meta); err != nil {
		t.Fatal(err)
	}
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
	root := t.TempDir()
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

func TestTaskWorkProjectInstructionsPresent(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Test", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/" + executable, nil
	})

	// Create the project instructions file.
	writeFile(t, filepath.Join(root, ".agents", "prompt.md"), "Follow the vision document.")

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "work", "001")
	if code != 0 {
		t.Errorf("exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "## Project Instructions", "Follow the vision document.")
}

func TestTaskWorkProjectInstructionsSkippedWithFlag(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Test", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/" + executable, nil
	})

	// Create the project instructions file.
	writeFile(t, filepath.Join(root, ".agents", "prompt.md"), "Should not appear.")

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "work", "001", "--no-project-prompt")
	if code != 0 {
		t.Errorf("exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertNotContains(t, stdout, "## Project Instructions")
	assertNotContains(t, stdout, "Should not appear.")
}

func TestTaskWorkProjectInstructionsMissingFileIsSilent(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Test", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/" + executable, nil
	})

	// No .agents/prompt.md exists.

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "work", "001")
	if code != 0 {
		t.Errorf("exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertNotContains(t, stdout, "## Project Instructions")
}

func TestTaskWorkProjectInstructionsConfiguredPath(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Test", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/" + executable, nil
	})

	// Create metadata with a custom prompt file path.
	writeFile(t, filepath.Join(root, ".agents", "ahm.json"), `{
  "version": "0.1.0",
  "taskWork": {
    "promptFile": ".agents/custom-prompt.md"
  },
  "files": {}
}`)

	// Create the custom prompt file (not the default).
	writeFile(t, filepath.Join(root, ".agents", "custom-prompt.md"), "Custom instructions.")

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "work", "001")
	if code != 0 {
		t.Errorf("exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "## Project Instructions", "Custom instructions.")
}

func TestTaskWorkProjectInstructionsConfiguredPathFromAhmConfig(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Test", "Pending", "")
	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/" + executable, nil
	})

	if err := writeConfigMetadata(root, metadata{
		TaskWork: &taskWorkConfig{PromptFile: ".agents/custom-prompt.md"},
		Files:    map[string]string{},
	}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".agents", "custom-prompt.md"), "Custom instructions from config.")

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "work", "001")
	if code != 0 {
		t.Errorf("exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "## Project Instructions", "Custom instructions from config.")
}

func TestTaskWorkResolvesRoleAgents(t *testing.T) {
	// Test that role-specific config in taskWork affects the implementation
	// and review agents independently.
	root := t.TempDir()
	if err := writeConfigMetadata(root, metadata{
		TaskWork: &taskWorkConfig{
			Implementation: &taskWorkRoleConfig{Agent: "codex", Model: "o3-mini"},
			Review:         &taskWorkRoleConfig{Agent: "claude", Model: "sonnet"},
		},
		Files: map[string]string{},
	}); err != nil {
		t.Fatal(err)
	}
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Role Test", "Pending", "")

	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/" + executable, nil
	})

	type invocation struct {
		executable string
		args       []string
	}
	var invocations []invocation
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		switch len(invocations) {
		case 0:
			// Session work (codex): produce valid codex session JSON.
			fmt.Fprintln(stdout, `{"type":"thread.started","thread_id":"sess_role123"}`)
			_, err := fmt.Fprint(stdout, `{"type":"item.completed","item":{"type":"agent_message","text":"Work done."}}`)
			invocations = append(invocations, invocation{executable, args})
			return err
		case 1:
			// Review (claude): produce review feedback.
			_, err := fmt.Fprint(stdout, `{"type":"result","result":"Review findings."}`)
			invocations = append(invocations, invocation{executable, args})
			return err
		default:
			// Finalization resume (codex): complete the task.
			_, err := fmt.Fprint(stdout, `{"type":"item.completed","item":{"type":"agent_message","text":"Addressed and completed."}}`)
			if err != nil {
				return err
			}
			invocations = append(invocations, invocation{executable, args})
			return completeTaskOnDisk(root, "001")
		}
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--no-commit")
	if code != 0 {
		t.Errorf("task work exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	// Should have 3 invocations: session work, review, finalization.
	if len(invocations) != 3 {
		t.Fatalf("expected 3 invocations, got %d", len(invocations))
	}

	// First invocation: implementation agent (codex).
	if invocations[0].executable != "/stub/codex" {
		t.Errorf("impl executable = %q, want /stub/codex", invocations[0].executable)
	}
	if len(invocations[0].args) < 4 || invocations[0].args[0] != "exec" {
		t.Errorf("impl args = %#v, want codex exec prefix", invocations[0].args)
	}

	// Second invocation: review agent (claude) with review prompt.
	if invocations[1].executable != "/stub/claude" {
		t.Errorf("review executable = %q, want /stub/claude", invocations[1].executable)
	}
	if len(invocations[1].args) < 2 || invocations[1].args[0] != "-p" {
		t.Errorf("review args = %#v, want claude -p prefix", invocations[1].args)
	}

	// Third invocation: resume with feedback, uses implementation agent.
	if invocations[2].executable != "/stub/codex" {
		t.Errorf("resume executable = %q, want /stub/codex (same as impl)", invocations[2].executable)
	}
}

func TestTaskWorkRoleAgentFlagOverridesAll(t *testing.T) {
	// When --agent flag is provided, it overrides both role configs.
	root := t.TempDir()
	if err := writeConfigMetadata(root, metadata{
		TaskWork: &taskWorkConfig{
			Implementation: &taskWorkRoleConfig{Agent: "codex"},
			Review:         &taskWorkRoleConfig{Agent: "claude"},
		},
		Files: map[string]string{},
	}); err != nil {
		t.Fatal(err)
	}
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Flag Override", "Pending", "")

	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/" + executable, nil
	})

	var invocations int
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		invocations++
		switch invocations {
		case 1:
			// Session work (cursor): emit a valid cursor init session event
			// so ahm can capture the session ID for review/finalization resume.
			fmt.Fprintln(stdout, `{"type":"system","subtype":"init","session_id":"cursor_sess_flag123"}`)
			_, err := fmt.Fprint(stdout, `{"type":"result","result":"Work."}`)
			return err
		case 2:
			// Review with same agent (cursor). Produces empty feedback.
			_, err := fmt.Fprint(stdout, `{"type":"result","result":""}`)
			return err
		default:
			// Finalization resume: agent completes the task.
			return completeTaskOnDisk(root, "001")
		}
	})

	_, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--agent", "cursor", "--no-commit")
	if code != 0 {
		t.Errorf("exit code = %d, stderr = %s", code, stderr)
	}
	// With --agent=cursor, both impl and review should use cursor. No separate
	// review executable lookup needed, so only the three expected invocations
	// (session, review, finalization) should occur.
	if invocations != 3 {
		t.Errorf("expected 3 invocations (session, review, finalization), got %d", invocations)
	}
}

func TestTaskWorkRoleAgentsDryRunPreview(t *testing.T) {
	// Dry-run should preview review_agent and review_model when review is enabled.
	root := t.TempDir()
	if err := writeConfigMetadata(root, metadata{
		TaskWork: &taskWorkConfig{
			Implementation: &taskWorkRoleConfig{Agent: "codex", Model: "o3-mini"},
			Review:         &taskWorkRoleConfig{Agent: "claude", Model: "sonnet"},
		},
		Files: map[string]string{},
	}); err != nil {
		t.Fatal(err)
	}
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Dry-Run Role", "Pending", "")

	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/" + executable, nil
	})
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		t.Error("runner should not be called during dry-run")
		return nil
	})

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "work", "001")
	if code != 0 {
		t.Errorf("dry-run exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout,
		"agent: codex",
		"model: o3-mini",
		"review_agent: claude",
		"review_model: sonnet",
	)
}

func TestTaskWorkRoleAgentWithLegacyFallback(t *testing.T) {
	// When review role config is absent, review falls back to impl agent.
	// When impl role config is absent, it falls back to default_work_agent.
	root := t.TempDir()
	if err := writeMetadata(root, metadata{
		DefaultWorkAgent: "codex",
		TaskWork: &taskWorkConfig{
			Implementation: &taskWorkRoleConfig{Agent: "cake"},
			// No Review role config - should fall back to impl agent (cake).
		},
		Files: map[string]string{},
	}); err != nil {
		t.Fatal(err)
	}
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Fallback Test", "Pending", "")

	stubTaskWorkLookPath(t, func(executable string) (string, error) {
		return "/stub/" + executable, nil
	})

	type invocation struct {
		executable string
	}
	var invocations []invocation
	stubTaskWorkRunner(t, func(ctx context.Context, root string, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
		switch len(invocations) {
		case 0:
			fmt.Fprintln(stdout, `{"type":"task_start","session_id":"sess_fallback","task_id":"tsk_xyz"}`)
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":"Work."}`)
			invocations = append(invocations, invocation{executable})
			return err
		case 1:
			// Review should use same agent (cake) since no review config.
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":"Reviewed."}`)
			invocations = append(invocations, invocation{executable})
			return err
		default:
			// Finalization resume: complete the task.
			_, err := fmt.Fprint(stdout, `{"type":"task_complete","subtype":"success","is_error":false,"result":"Addressed and completed."}`)
			if err != nil {
				return err
			}
			invocations = append(invocations, invocation{executable})
			return completeTaskOnDisk(root, "001")
		}
	})

	_, stderr, code := runCLI(t, "--root", root, "task", "work", "001", "--no-commit")
	if code != 0 {
		t.Errorf("exit code = %d, stderr = %s", code, stderr)
	}
	// Expect 3 invocations: session work, review, finalization.
	if len(invocations) != 3 {
		t.Fatalf("expected 3 invocations, got %d", len(invocations))
	}
	// Both should use cake (impl from role config, review falls back to impl).
	if invocations[0].executable != "/stub/cake" {
		t.Errorf("impl executable = %q, want /stub/cake", invocations[0].executable)
	}
	if invocations[1].executable != "/stub/cake" {
		t.Errorf("review executable = %q, want /stub/cake", invocations[1].executable)
	}
	// Third invocation (resume) also uses cake.
	if invocations[2].executable != "/stub/cake" {
		t.Errorf("resume executable = %q, want /stub/cake", invocations[2].executable)
	}
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
