package ahm

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSortTaskListFields(t *testing.T) {
	tasks := []Task{
		{ID: "010", Title: "Beta", Status: "Blocked", Priority: "P2", Effort: "L", Created: "2026-01-03T00:00:00Z", Updated: "2026-01-01T00:00:00Z"},
		{ID: "002", Title: "alpha", Status: "Completed", Priority: "P0", Effort: "XL", Created: "2026-01-01T10:00:00-05:00", Updated: "2026-01-03T00:00:00Z"},
		{ID: "001", Title: "Alpha", Status: "Open", Priority: "P1", Effort: "XS", Created: "2026-01-01T14:00:00Z", Updated: "2026-01-02T00:00:00Z"},
	}
	tests := map[string]string{
		"priority": "002,001,010",
		"id":       "001,002,010",
		"created":  "001,002,010",
		"updated":  "010,001,002",
		"effort":   "001,010,002",
		"status":   "001,010,002",
		"title":    "001,002,010",
	}
	for field, want := range tests {
		t.Run(field, func(t *testing.T) {
			got := append([]Task(nil), tasks...)
			if err := sortTaskList(got, field, false); err != nil {
				t.Fatal(err)
			}
			if ids := joinedTaskIDs(got); ids != want {
				t.Errorf("%s sort = %s, want %s", field, ids, want)
			}
		})
	}
	defaultTasks := append([]Task(nil), tasks...)
	if err := sortTaskList(defaultTasks, "", false); err != nil {
		t.Fatal(err)
	}
	if got, want := joinedTaskIDs(defaultTasks), tests["priority"]; got != want {
		t.Errorf("default sort = %s, want %s", got, want)
	}
}

func TestSortTaskListReverseAndValidation(t *testing.T) {
	tasks := []Task{
		{ID: "001", Title: "Alpha"},
		{ID: "002", Title: "alpha"},
		{ID: "003", Title: "Beta"},
	}
	if err := sortTaskList(tasks, "title", true); err != nil {
		t.Fatal(err)
	}
	if got, want := joinedTaskIDs(tasks), "003,002,001"; got != want {
		t.Errorf("reverse title sort = %s, want %s", got, want)
	}

	err := sortTaskList(tasks, "deadline", false)
	if err == nil {
		t.Fatal("expected unsupported sort field error")
	}
	assertContainsAll(t, err.Error(), "unsupported task sort field", "priority, id, created, updated, effort, status, title")
}

func TestSortTaskListDomainRanks(t *testing.T) {
	tests := []struct {
		name  string
		field string
		tasks []Task
		want  string
	}{
		{
			name:  "priority",
			field: "priority",
			tasks: []Task{{ID: "005", Priority: "P4"}, {ID: "003", Priority: "P2"}, {ID: "001", Priority: "P0"}, {ID: "004", Priority: "P3"}, {ID: "002", Priority: "P1"}},
			want:  "001,002,003,004,005",
		},
		{
			name:  "effort",
			field: "effort",
			tasks: []Task{{ID: "005", Effort: "XL"}, {ID: "003", Effort: "M"}, {ID: "001", Effort: "XS"}, {ID: "004", Effort: "L"}, {ID: "002", Effort: "S"}},
			want:  "001,002,003,004,005",
		},
		{
			name:  "status",
			field: "status",
			tasks: []Task{{ID: "007", Status: "Cancelled"}, {ID: "004", Status: "Blocked"}, {ID: "001", Status: "Open"}, {ID: "006", Status: "Completed"}, {ID: "003", Status: "In Progress"}, {ID: "005", Status: "Tracking"}, {ID: "002", Status: "Pending"}},
			want:  "001,002,003,004,005,006,007",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := sortTaskList(test.tasks, test.field, false); err != nil {
				t.Fatal(err)
			}
			if got := joinedTaskIDs(test.tasks); got != test.want {
				t.Errorf("%s sort = %s, want %s", test.field, got, test.want)
			}
		})
	}
}

func TestTaskListCommandsSupportSorting(t *testing.T) {
	root := t.TempDir()
	active := filepath.Join(root, ".agents", ".tasks", "active")
	writeTaskFile(t, filepath.Join(active, "001.md"), "001", "Zulu", "Pending", "")
	writeTaskFile(t, filepath.Join(active, "002.md"), "002", "Alpha", "Pending", "")
	writeTaskFile(t, filepath.Join(active, "003.md"), "003", "Middle", "Blocked", "effort: M\n")
	writeTaskFile(t, filepath.Join(active, "004.md"), "004", "First effort", "Blocked", "effort: XS\n")

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "list", args: []string{"task", "list", "--sort", "title"}, want: []string{"Alpha", "First effort", "Middle", "Zulu"}},
		{name: "ready", args: []string{"task", "ready", "--sort", "id", "--reverse"}, want: []string{"Alpha", "Zulu"}},
		{name: "blocked", args: []string{"task", "blocked", "--sort", "effort"}, want: []string{"First effort", "Middle"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"--root", root}, test.args...)
			stdout, stderr, code := runCLI(t, args...)
			if code != 0 {
				t.Fatalf("exit code = %d, stderr = %s", code, stderr)
			}
			assertOrderedStrings(t, stdout, test.want...)
		})
	}
}

func TestTaskListSortOrderAcrossOutputModes(t *testing.T) {
	root := t.TempDir()
	active := filepath.Join(root, ".agents", ".tasks", "active")
	writeTaskFile(t, filepath.Join(active, "001.md"), "001", "First task", "Pending", "")
	writeTaskFile(t, filepath.Join(active, "002.md"), "002", "Second task", "Pending", "")
	writeTaskFile(t, filepath.Join(active, "003.md"), "003", "Third task", "Pending", "")

	for _, outputMode := range []string{"--text", "--json", "--plain"} {
		t.Run(strings.TrimPrefix(outputMode, "--"), func(t *testing.T) {
			stdout, stderr, code := runCLI(t, "--root", root, outputMode, "task", "list", "--sort", "id", "--reverse")
			if code != 0 {
				t.Fatalf("exit code = %d, stderr = %s", code, stderr)
			}
			assertOrderedStrings(t, stdout, "Third task", "Second task", "First task")
		})
	}
}

func TestTaskListSortCLIValidationAndHelp(t *testing.T) {
	root := t.TempDir()
	_, stderr, code := runCLI(t, "--root", root, "task", "list", "--sort", "deadline")
	if code != 2 {
		t.Fatalf("invalid sort exit code = %d, want 2; stderr = %s", code, stderr)
	}
	assertContainsAll(t, stderr, "unsupported task sort field", "priority, id, created, updated, effort, status, title")

	_, stderr, code = runCLI(t, "--root", root, "task", "next", "--sort", "id")
	if code != 2 {
		t.Fatalf("task next --sort exit code = %d, want 2; stderr = %s", code, stderr)
	}
	assertContainsAll(t, stderr, "unknown flag: --sort")

	for _, command := range []string{"list", "ready", "blocked"} {
		stdout, helpErr, helpCode := runCLI(t, "task", command, "--help")
		if helpCode != 0 {
			t.Fatalf("task %s --help exit code = %d, stderr = %s", command, helpCode, helpErr)
		}
		assertContainsAll(t, stdout, "--sort", "priority, id, created, updated, effort, status, or title", "--reverse")
	}
}

func joinedTaskIDs(tasks []Task) string {
	ids := make([]string, len(tasks))
	for i, task := range tasks {
		ids[i] = task.ID
	}
	return strings.Join(ids, ",")
}

func assertOrderedStrings(t *testing.T, output string, values ...string) {
	t.Helper()
	position := -1
	for _, value := range values {
		next := strings.Index(output, value)
		if next < 0 {
			t.Fatalf("output missing %q:\n%s", value, output)
		}
		if next <= position {
			t.Fatalf("%q is out of order in output:\n%s", value, output)
		}
		position = next
	}
}
