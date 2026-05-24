package ahm

import (
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
