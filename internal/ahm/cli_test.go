package ahm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFrontMatter(t *testing.T) {
	meta, body := parseFrontMatter("---\nid: 001\ntitle: Test Task\n---\n# Test Task\n")
	if meta["id"] != "001" {
		t.Fatalf("id = %q", meta["id"])
	}
	if meta["title"] != "Test Task" {
		t.Fatalf("title = %q", meta["title"])
	}
	if !strings.Contains(body, "# Test Task") {
		t.Fatalf("body = %q", body)
	}
}

func TestStripHeading(t *testing.T) {
	got := stripHeading("\n# Test Task\n\n## Summary\n\nBody\n", "Test Task")
	if !strings.HasPrefix(got, "## Summary") {
		t.Fatalf("body = %q", got)
	}
}

func TestNextTaskID(t *testing.T) {
	got := nextTaskID([]Task{{ID: "001"}, {ID: "002a"}, {ID: "010"}})
	if got != "011" {
		t.Fatalf("nextTaskID = %q", got)
	}
}

func TestParseTaskCreateArgsAllowsFlagsAfterTitle(t *testing.T) {
	got, err := parseTaskCreateArgs([]string{"Smoke task", "--description", "Verify task creation", "--priority", "P1"})
	if err != nil {
		t.Fatal(err)
	}
	if got.title != "Smoke task" {
		t.Fatalf("title = %q", got.title)
	}
	if got.description != "Verify task creation" {
		t.Fatalf("description = %q", got.description)
	}
	if got.priority != "P1" {
		t.Fatalf("priority = %q", got.priority)
	}
}

func TestParseTaskCreateArgsRejectsUnsupportedEnums(t *testing.T) {
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
			_, err := parseTaskCreateArgs(tt.args)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want %q", err, tt.want)
			}
		})
	}
}

func TestParseTaskRejectsUnsupportedEnums(t *testing.T) {
	path := filepath.Join(t.TempDir(), "001.md")
	content := "---\n" +
		"id: 001\n" +
		"title: Bad Task\n" +
		"status: Pending\n" +
		"priority: Urgent\n" +
		"effort: S\n" +
		"labels: type:task\n" +
		"exec_plan: -\n" +
		"depends_on: []\n" +
		"---\n" +
		"# Bad Task\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := parseTask(path, "active")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `unsupported task priority "Urgent"`) {
		t.Fatalf("error = %q", err)
	}
}

func TestStatusReportsValidationFindings(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".agents", "TASKS.md"), []byte("locally changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Blocked Task", "Pending", "depends_on: 999\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Cycle A", "Pending", "depends_on: 003\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "003.md"), "003", "Cycle B", "Pending", "depends_on: 002\n")

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.status(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		`"ok": false`,
		`"code": "managed_file_missing"`,
		`"path": "AGENTS.md"`,
		`"code": "managed_file_modified"`,
		`"path": ".agents/TASKS.md"`,
		`"code": "task_dependency_missing"`,
		`task 001 depends on missing task 999`,
		`"code": "task_dependency_cycle"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("status output missing %q:\n%s", want, got)
		}
	}
}

func TestDoctorReportsMalformedTaskEnums(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Bad Task", "Doing", "depends_on: []\n")

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.doctor(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		`"workflow_installed": true`,
		`"ok": false`,
		`"code": "task_malformed"`,
		`unsupported task status \"Doing\"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, got)
		}
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
	for _, want := range []string{
		"created: 2026-05-01",
		"updated: 2026-05-02",
		"parent: 000",
		"external_ref: gh-123",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("rewritten task missing %q:\n%s", want, content)
		}
	}
}

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
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".agents", ".tasks", "active", "001.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"depends_on: 002",
		"created: 2026-05-01",
		"updated: 2026-05-02",
		"parent: 000",
		"external_ref: gh-123",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("rewritten task missing %q:\n%s", want, content)
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

func TestRenderRootIndex(t *testing.T) {
	index := renderRootIndex([]Task{
		{ID: "001", Title: "Done", Status: "Completed", Priority: "P1", Effort: "S", Labels: "type:task", Bucket: "completed"},
		{ID: "002", Title: "Ready", Status: "Pending", Priority: "P0", Effort: "S", Labels: "type:task", Bucket: "active", DependsOn: []string{"001"}},
	})
	if !strings.Contains(index, "## Next Ready Queue") {
		t.Fatalf("missing ready queue:\n%s", index)
	}
	if !strings.Contains(index, "[002](active/002.md) - Ready") {
		t.Fatalf("missing ready task:\n%s", index)
	}
}

func writeTaskFile(t *testing.T, path string, id string, title string, status string, extraFrontMatter string) {
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
		"labels: type:task\n" +
		"exec_plan: -\n" +
		extraFrontMatter +
		"---\n" +
		"# " + title + "\n\n" +
		"## Summary\n\nTODO.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
