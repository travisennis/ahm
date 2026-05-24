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
