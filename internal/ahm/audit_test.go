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

func writeAuditVocabularyTask(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	writeTaskFile(t, path, "001", "Existing task", "Pending", "")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), "labels: type:task", "labels: type:task, area:unknown", 1))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAuditDryRunIncludesLiveStateAndRules(t *testing.T) {
	root := t.TempDir()
	writeAuditVocabularyTask(t, root)
	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "audit", "--agent", "codex")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	assertContainsAll(t, stdout, "Existing active tasks", "Existing task", "Existing label vocabulary", "Known validation findings", "strictly read-only", "Never reproduce secret values", "Result schema")
}

func TestAuditCreatesOpenTaskFromValidatedFinding(t *testing.T) {
	root := t.TempDir()
	writeAuditVocabularyTask(t, root)
	stubTaskWorkLookPath(t, func(string) (string, error) { return "/stub/codex", nil })
	stubTaskWorkRunner(t, func(_ context.Context, _ string, _ string, _ []string, _ io.Reader, stdout, _ io.Writer) error {
		_, err := fmt.Fprintln(stdout, `{"type":"item.completed","item":{"type":"agent_message","text":"{\"findings\":[{\"title\":\"Improve timeout errors\",\"problem\":\"Timeout failures lack context.\",\"relevant_files\":[\"internal/ahm/task_work.go\"],\"fix_direction\":\"Wrap timeout errors with phase details.\",\"acceptance_notes\":[\"Timeout errors name the failed phase.\"],\"labels\":[\"type:task\",\"area:unknown\"],\"priority\":\"P2\",\"effort\":\"S\"}]}"}}`)
		return err
	})
	stdout, stderr, code := runCLI(t, "--root", root, "audit", "--agent", "codex")
	if code != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "002: Improve timeout errors")
	data, err := os.ReadFile(filepath.Join(root, ".agents", ".tasks", "active", "002.md"))
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, string(data), "status: Open", "source:audit", "## Problem", "## Relevant Files", "## Fix Direction", "## Acceptance Notes", "Timeout errors name the failed phase")
}

func TestAuditInvalidResultCreatesNothing(t *testing.T) {
	root := t.TempDir()
	writeAuditVocabularyTask(t, root)
	stubTaskWorkLookPath(t, func(string) (string, error) { return "/stub/cake", nil })
	stubTaskWorkRunner(t, func(_ context.Context, _ string, _ string, _ []string, _ io.Reader, stdout, _ io.Writer) error {
		_, err := fmt.Fprintln(stdout, `{"findings":[{"title":"Incomplete"}]}`)
		return err
	})
	_, stderr, code := runCLI(t, "--root", root, "audit", "--agent", "cake")
	if code == 0 {
		t.Fatal("expected failure")
	}
	assertContainsAll(t, stderr, "invalid audit result", "no changes applied")
	if _, err := os.Stat(filepath.Join(root, ".agents", ".tasks", "active", "002.md")); !os.IsNotExist(err) {
		t.Fatalf("unexpected created task: %v", err)
	}
}

func TestValidateAuditResultRejectsDuplicateAndUnknownLabel(t *testing.T) {
	tasks := []Task{{ID: "001", Title: "Existing task", Labels: "type:task, area:unknown"}}
	finding := auditFinding{Title: "Existing task", Problem: "p", RelevantFiles: []string{"x"}, FixDirection: "f", AcceptanceNotes: []string{"a"}, Labels: []string{"type:task", "area:unknown"}, Priority: "P2", Effort: "S"}
	if err := validateAuditResult(auditResult{Findings: []auditFinding{finding}}, tasks); err == nil || !strings.Contains(err.Error(), "duplicates") {
		t.Fatalf("duplicate error=%v", err)
	}
	finding.Title = "New"
	finding.Labels = []string{"type:task", "area:missing"}
	if err := validateAuditResult(auditResult{Findings: []auditFinding{finding}}, tasks); err == nil || !strings.Contains(err.Error(), "unknown label") {
		t.Fatalf("label error=%v", err)
	}
}

func TestValidateAuditResultBootstrapVocabularyWithZeroTasks(t *testing.T) {
	finding := auditFinding{
		Title:           "New finding",
		Problem:         "p",
		RelevantFiles:   []string{"x"},
		FixDirection:    "f",
		AcceptanceNotes: []string{"a"},
		Labels:          []string{"type:task", "area:unknown"},
		Priority:        "P2",
		Effort:          "S",
	}
	if err := validateAuditResult(auditResult{Findings: []auditFinding{finding}}, nil); err != nil {
		t.Fatalf("expected no error with bootstrap labels, got: %v", err)
	}
}

func TestValidateAuditResultRejectsUnknownLabelWithZeroTasks(t *testing.T) {
	finding := auditFinding{
		Title:           "New finding",
		Problem:         "p",
		RelevantFiles:   []string{"x"},
		FixDirection:    "f",
		AcceptanceNotes: []string{"a"},
		Labels:          []string{"type:task", "area:nonexistent"},
		Priority:        "P2",
		Effort:          "S",
	}
	if err := validateAuditResult(auditResult{Findings: []auditFinding{finding}}, nil); err == nil || !strings.Contains(err.Error(), "unknown label") {
		t.Fatalf("expected unknown label error with zero tasks, got: %v", err)
	}
}

func TestAuditDryRunWithZeroTasksShowsBootstrapVocabulary(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "audit", "--agent", "codex")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	assertContainsAll(t, stdout, "Existing label vocabulary", "type:task (bootstrap", "area:unknown (bootstrap", "Result schema", "strictly read-only")
}

func TestAuditCreatesOpenTaskWithZeroTasks(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	stubTaskWorkLookPath(t, func(string) (string, error) { return "/stub/codex", nil })
	stubTaskWorkRunner(t, func(_ context.Context, _ string, _ string, _ []string, _ io.Reader, stdout, _ io.Writer) error {
		_, err := fmt.Fprintln(stdout, `{"type":"item.completed","item":{"type":"agent_message","text":"{\"findings\":[{\"title\":\"Improve timeout errors\",\"problem\":\"Timeout failures lack context.\",\"relevant_files\":[\"internal/ahm/task_work.go\"],\"fix_direction\":\"Wrap timeout errors with phase details.\",\"acceptance_notes\":[\"Timeout errors name the failed phase.\"],\"labels\":[\"type:task\",\"area:unknown\"],\"priority\":\"P2\",\"effort\":\"S\"}]}"}}`)
		return err
	})
	stdout, stderr, code := runCLI(t, "--root", root, "audit", "--agent", "codex")
	if code != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "001: Improve timeout errors")
	data, err := os.ReadFile(filepath.Join(root, ".ahm", "tasks", "active", "001.md"))
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, string(data), "status: Open", "labels: type:task, area:unknown, source:audit", "## Problem", "## Relevant Files", "## Fix Direction", "## Acceptance Notes", "Timeout errors name the failed phase")
	report, _ := validateWorkflow(root)
	if len(report.Errors) != 0 {
		t.Fatalf("created task failed workflow validation: %v", report.Errors)
	}
}

func TestAuditSummaryOutputModes(t *testing.T) {
	for _, mode := range []string{"--plain", "--json"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			writeAuditVocabularyTask(t, root)
			stubTaskWorkLookPath(t, func(string) (string, error) { return "/stub/cake", nil })
			stubTaskWorkRunner(t, func(_ context.Context, _ string, _ string, _ []string, _ io.Reader, stdout, _ io.Writer) error {
				_, err := fmt.Fprintln(stdout, `{"findings":[]}`)
				return err
			})
			stdout, stderr, code := runCLI(t, "--root", root, mode, "audit", "--agent", "cake")
			if code != 0 {
				t.Fatalf("exit=%d stderr=%s", code, stderr)
			}
			assertContainsAll(t, stdout, `"agent"`, `"cake"`, `"created"`)
		})
	}
}
