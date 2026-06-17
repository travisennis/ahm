package ahm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskDepUpdatePreservesOptionalFrontMatter(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Main Task", "Pending", "depends_on: []\n"+
		"created: 2026-05-01\n"+
		"updated: 2026-05-02\n"+
		"parent: 000\n"+
		"external_ref: gh-123\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Dependency", "Pending", "depends_on: []\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskDepUpdate([]string{"001", "002"}, true); err != nil {
		t.Error(err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".agents", ".tasks", "active", "001.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	// created is preserved; updated is overwritten with current timestamp.
	for _, want := range []string{
		"depends_on: 002",
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

func TestTaskDepUpdatePreservesUnknownFrontMatter(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Main Task", "Pending",
		"assignee: alice\n"+
			"due: 2026-06-01\n"+
			"tags: bug, urgent\n"+
			"ticket: JIRA-456\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Dependency", "Pending", "depends_on: []\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskDepUpdate([]string{"001", "002"}, true); err != nil {
		t.Error(err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".agents", ".tasks", "active", "001.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"assignee: alice",
		"due: 2026-06-01",
		"tags: bug, urgent",
		"ticket: JIRA-456",
		"depends_on: 002",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("rewritten task missing %q:\n%s", want, content)
		}
	}
}

func TestTaskDependencyTreeOutput(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Root", "Pending", "depends_on: 002, 999\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Middle", "Pending", "depends_on: 003\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "003.md"), "003", "Leaf", "Pending", "depends_on: 002\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskDepTree([]string{"001"}); err != nil {
		t.Error(err)
	}
	assertContainsAll(t, out.String(),
		"001 [Pending] Root",
		"  002 [Pending] Middle",
		"    003 [Pending] Leaf",
		"      cycle to 002",
		"  999 [missing]",
	)
}

func TestTaskDependencyCycleDetection(t *testing.T) {
	tasks := []Task{
		{ID: "001", Status: "Pending", DependsOn: []string{"002"}},
		{ID: "002", Status: "Pending", DependsOn: []string{"001"}},
		{ID: "003", Status: "Completed", DependsOn: []string{"004"}},
		{ID: "004", Status: "Pending", DependsOn: []string{"003"}},
		{ID: "005", Status: "Cancelled", DependsOn: []string{"006"}},
		{ID: "006", Status: "Pending", DependsOn: []string{"005"}},
	}
	cycles := taskDependencyCycles(tasks)
	if len(cycles) != 1 {
		t.Errorf("cycles = %#v", cycles)
	}
	if got := strings.Join(cycles[0], " -> "); got != "001 -> 002 -> 001" {
		t.Errorf("cycle = %q", got)
	}
}

func TestTaskDepCyclesCommand(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Cycle A", "Pending", "depends_on: 002\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Cycle B", "Pending", "depends_on: 001\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "completed", "003.md"), "003", "Completed Cycle", "Completed", "depends_on: 004\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "004.md"), "004", "Ignored Active", "Pending", "depends_on: 003\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskDepCycles(); err != nil {
		t.Error(err)
	}
	got := out.String()
	if !strings.Contains(got, "001 -> 002 -> 001") {
		t.Errorf("cycle output = %q", got)
	}
	if strings.Contains(got, "003") || strings.Contains(got, "004") {
		t.Errorf("completed-task cycle should be ignored:\n%s", got)
	}
}

func TestTaskDepAddNoOp(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Main Task", "Pending", "depends_on: -\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Existing Dep", "Pending", "depends_on: -\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}

	// First add the dependency.
	if err := a.taskDepUpdate([]string{"001", "002"}, true); err != nil {
		t.Error(err)
	}
	firstOut := out.String()
	if !strings.Contains(firstOut, "depends_on: 002") {
		t.Errorf("first add output = %q", firstOut)
	}

	// Read the file and save content.
	path := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Reset output and try adding the same dependency again (no-op).
	out.Reset()
	if err := a.taskDepUpdate([]string{"001", "002"}, true); err != nil {
		t.Error(err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("file content changed on no-op dep add:\nbefore: %s\nafter:  %s", before, after)
	}

	if !strings.Contains(out.String(), "already depends on 002") {
		t.Errorf("output missing no-op message: %q", out.String())
	}
}

func TestTaskDepRemoveNoOp(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Main Task", "Pending", "depends_on: -\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Not A Dep", "Pending", "depends_on: -\n")

	path := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.taskDepUpdate([]string{"001", "002"}, false); err != nil {
		t.Error(err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("file content changed on no-op dep remove:\nbefore: %s\nafter:  %s", before, after)
	}

	if !strings.Contains(out.String(), "does not depend on 002") {
		t.Errorf("output missing no-op message: %q", out.String())
	}
}

func TestTaskDepAddRejectsSelfDependency(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Main Task", "Pending", "depends_on: -\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	err := a.taskDepUpdate([]string{"001", "001"}, true)
	if err == nil {
		t.Error("expected error for self-dependency, got nil")
	}
	if !strings.Contains(err.Error(), "001 cannot depend on itself") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTaskDepAddRejectsCancelledDependency(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Main Task", "Pending", "depends_on: -\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "cancelled", "002.md"), "002", "Cancelled Task", "Cancelled", "depends_on: -\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	err := a.taskDepUpdate([]string{"001", "002"}, true)
	if err == nil {
		t.Error("expected error for cancelled dependency, got nil")
	}
	if !strings.Contains(err.Error(), "001 cannot depend on cancelled task 002") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTaskDepAddRejectsCycle(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Task A", "Pending", "depends_on: 002\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Task B", "Pending", "depends_on: -\n")

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	err := a.taskDepUpdate([]string{"002", "001"}, true)
	if err == nil {
		t.Error("expected error for cycle, got nil")
	}
	if !strings.Contains(err.Error(), "would create a cycle") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMainDependencyCyclesIntegration(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Cycle A", "Pending", "depends_on: 002\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Cycle B", "Pending", "depends_on: 001\n")

	stdout, stderr, code = runCLI(t, "--root", root, "task", "dep", "cycles")
	if code != 0 {
		t.Errorf("cycles exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "001 -> 002 -> 001") {
		t.Errorf("cycles stdout = %q", stdout)
	}
}
