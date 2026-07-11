package ahm

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskGroomDryRunScopesOneTask(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "First", "Open", "")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Second", "Open", "")

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "groom", "001", "--agent", "codex")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	assertContainsAll(t, stdout, "tasks:", "- 001", "Result schema", "Existing label vocabulary")
	if strings.Contains(stdout, "Second") {
		t.Fatalf("dry-run included out-of-scope task: %s", stdout)
	}
}

func TestTaskGroomAppliesValidatedVerdict(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "First", "Open", "")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Dependency", "Pending", "")
	stubTaskWorkLookPath(t, func(string) (string, error) { return "/stub/codex", nil })
	stubTaskWorkRunner(t, func(_ context.Context, _ string, _ string, _ []string, _ io.Reader, stdout, _ io.Writer) error {
		_, err := fmt.Fprintln(stdout, `{"type":"item.completed","item":{"type":"agent_message","text":"{\"verdicts\":[{\"task\":\"001\",\"action\":\"accept\",\"comment\":\"Ready after dependency correction.\",\"add_deps\":[\"002\"],\"remove_deps\":[],\"labels\":[\"type:task\"]}]}"}}`)
		return err
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "groom", "001", "--agent", "codex")
	if code != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "001: accept", "commented", "added deps 002")
	data, err := os.ReadFile(filepath.Join(root, ".agents", ".tasks", "active", "001.md"))
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, string(data), "status: Pending", "depends_on: 002", "Ready after dependency correction.")
}

func TestTaskGroomInvalidOutputMakesNoChanges(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	writeTaskFile(t, path, "001", "First", "Open", "")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	stubTaskWorkLookPath(t, func(string) (string, error) { return "/stub/cake", nil })
	stubTaskWorkRunner(t, func(_ context.Context, _ string, _ string, _ []string, _ io.Reader, stdout, _ io.Writer) error {
		_, err := fmt.Fprintln(stdout, `not json`)
		return err
	})

	_, stderr, code := runCLI(t, "--root", root, "task", "groom", "001", "--agent", "cake")
	if code == 0 {
		t.Fatal("expected failure")
	}
	assertContainsAll(t, stderr, "invalid groom result", "no changes applied", "not json")
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("task changed after invalid output")
	}
}

func TestTaskGroomRejectsIncompleteBatchAndDependencyCycle(t *testing.T) {
	tasks := []Task{
		{ID: "001", Status: "Open", Labels: "type:task", DependsOn: []string{"002"}},
		{ID: "002", Status: "Open", Labels: "type:task"},
	}
	result := groomResult{Verdicts: []groomVerdict{{Task: "001", Action: "comment", Comment: "Still blocked.", AddDeps: []string{}, RemoveDeps: []string{}, Labels: []string{}}}}
	if _, err := validateGroomResult(result, tasks, tasks); err == nil || !strings.Contains(err.Error(), "missing verdict for task 002") {
		t.Fatalf("missing verdict error = %v", err)
	}
	result.Verdicts = append(result.Verdicts, groomVerdict{Task: "002", Action: "comment", Comment: "Still blocked.", AddDeps: []string{"001"}, RemoveDeps: []string{}, Labels: []string{}})
	if _, err := validateGroomResult(result, tasks, tasks); err == nil || !strings.Contains(err.Error(), "would create a cycle") {
		t.Fatalf("cycle error = %v", err)
	}
}

func TestParseGroomResultRejectsMissingAndUnknownFields(t *testing.T) {
	for _, raw := range []string{
		`{"verdicts":[{"task":"001","action":"comment","comment":"x","add_deps":[],"remove_deps":[]}]}`,
		`{"verdicts":[],"unexpected":true}`,
	} {
		if _, err := parseGroomResult([]byte(raw)); err == nil {
			t.Fatalf("parseGroomResult(%s) succeeded", raw)
		}
	}
}
