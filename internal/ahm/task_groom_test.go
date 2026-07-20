package ahm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
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
	assertContainsAll(t, stdout, "correction_retry:", "available: true", "max_attempts: 1", "trigger: semantic_validation_failure")
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

func TestTaskGroomAppliesValidatedCakeStreamVerdict(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "First", "Open", "")
	stubTaskWorkLookPath(t, func(string) (string, error) { return "/stub/cake", nil })
	stubTaskWorkRunner(t, func(_ context.Context, _ string, _ string, args []string, _ io.Reader, stdout, _ io.Writer) error {
		assertContainsAll(t, strings.Join(args, " "), "--output-format stream-json", "--output-schema")
		_, err := fmt.Fprintln(stdout, `{"type":"message","role":"assistant","content":"{\"verdicts\":[{\"task\":\"001\",\"action\":\"accept\",\"comment\":\"Ready.\",\"add_deps\":[],\"remove_deps\":[],\"labels\":[\"type:task\"]}]}"}`)
		return err
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "groom", "001", "--agent", "cake")
	if code != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "001: accept", "commented")
	data, err := os.ReadFile(filepath.Join(root, ".agents", ".tasks", "active", "001.md"))
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, string(data), "status: Pending", "Ready.")
}

func TestTaskGroomAppliesStructuredRevisionAndAccepts(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := `---
id: 001
title: Improve grooming
status: Open
priority: P3
effort: S
labels: type:task, area:tasks
exec_plan: -
depends_on: -
provenance: audit
---
# Improve grooming

## Problem

Too vague.

## Historical Notes

Preserve this text.

## Acceptance Notes

- [ ] TODO

## Comments

- Existing comment.
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	stubTaskWorkLookPath(t, func(string) (string, error) { return "/stub/codex", nil })
	stubTaskWorkRunner(t, func(_ context.Context, _ string, _ string, _ []string, _ io.Reader, stdout, _ io.Writer) error {
		_, err := fmt.Fprintln(stdout, `{"verdicts":[{"task":"001","action":"accept","comment":"Repaired from repository evidence.","add_deps":[],"remove_deps":[],"labels":["type:task","area:tasks"],"revision":{"priority":"P2","effort":"M","sections":[{"role":"problem","content":"The groom command cannot repair objective gaps."},{"role":"relevant_files","content":"- internal/ahm/task_groom.go"},{"role":"acceptance_notes","content":"- [ ] Structured revisions are applied safely."}]}}]}`)
		return err
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "groom", "001", "--agent", "codex")
	if code != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "001: accept", "priority P2", "effort M", "revised problem,relevant_files,acceptance_notes")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, string(data), "status: Pending", "priority: P2", "effort: M", "provenance: audit", "## Historical Notes", "Preserve this text.", "- Existing comment.", "## Relevant Files", "Structured revisions are applied safely.")
}

func TestTaskGroomRejectsInvalidRevisionBatchWithoutWrites(t *testing.T) {
	tasks := []Task{
		{ID: "001", Status: "Open", Priority: "P2", Effort: "S", Labels: "type:task, area:tasks", Body: "## Acceptance Notes\n\n- [ ] Ready"},
		{ID: "002", Status: "Open", Priority: "P2", Effort: "S", Labels: "type:task, area:tasks", Body: "## Acceptance Notes\n\n- [ ] Ready"},
	}
	result := groomResult{Verdicts: []groomVerdict{
		{Task: "001", Action: "revise", Comment: "Still needs review.", AddDeps: []string{}, RemoveDeps: []string{}, Labels: []string{}, Revision: &groomRevision{Sections: []groomSectionRevision{{Role: "problem", Content: "Concrete problem."}}}},
		{Task: "002", Action: "revise", Comment: "Still needs review.", AddDeps: []string{}, RemoveDeps: []string{}, Labels: []string{}, Revision: &groomRevision{Sections: []groomSectionRevision{{Role: "problem", Content: ""}}}},
	}}
	if _, err := validateGroomResult(result, tasks, tasks); err == nil || !strings.Contains(err.Error(), "section problem is empty") {
		t.Fatalf("invalid revision error = %v", err)
	}
	if tasks[0].Body != "## Acceptance Notes\n\n- [ ] Ready" {
		t.Fatal("validation mutated the first task before rejecting the batch")
	}
}

func TestDelegatedResultArgsCakeWritesAndCleansSchema(t *testing.T) {
	agent, err := parseTaskWorkAgent("cake")
	if err != nil {
		t.Fatal(err)
	}
	args, cleanup, err := delegatedResultArgs(agent, "prompt", "model-name", groomResultSchema)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	assertContainsAll(t, joined, "--output-format stream-json", "--output-schema", "--model model-name", "prompt")
	var schemaPath string
	for i, arg := range args {
		if arg == "--output-schema" && i+1 < len(args) {
			schemaPath = args[i+1]
			break
		}
	}
	if schemaPath == "" {
		t.Fatalf("schema path missing from args: %v", args)
	}
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != groomResultSchema {
		t.Fatalf("schema file = %q", data)
	}
	cleanup()
	if _, err := os.Stat(schemaPath); !os.IsNotExist(err) {
		t.Fatalf("schema file still exists after cleanup: %v", err)
	}
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
	calls := 0
	stubTaskWorkRunner(t, func(_ context.Context, _ string, _ string, _ []string, _ io.Reader, stdout, _ io.Writer) error {
		calls++
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
	if calls != 1 {
		t.Fatalf("delegation calls = %d, want 1", calls)
	}
}

func TestValidateGroomResultReportsAllIndependentErrors(t *testing.T) {
	tasks := []Task{
		{ID: "001", Status: "Blocked", Labels: "type:task, area:tasks"},
		{ID: "002", Status: "Open", Labels: "type:task, area:tasks"},
	}
	result := groomResult{Verdicts: []groomVerdict{
		{Task: "001", Action: "accept", AddDeps: []string{"999"}, RemoveDeps: []string{}, Labels: []string{"area:unknown"}},
		{Task: "002", Action: "comment", Comment: "", AddDeps: []string{}, RemoveDeps: []string{}, Labels: []string{}, Revision: &groomRevision{}},
	}}

	_, err := validateGroomResult(result, tasks, tasks)
	if err == nil {
		t.Fatal("validateGroomResult succeeded")
	}
	assertContainsAll(t, err.Error(),
		"task 001 cannot be accepted from Blocked",
		"task 001 has invalid dependency 999",
		"task 001 uses unknown label area:unknown",
		"task 002 comment action requires a comment",
		"task 002 comment action cannot include a revision",
		"task 002 revision is empty",
	)
}

func TestTaskGroomCorrectsSemanticFailureAndAppliesOnce(t *testing.T) {
	root := t.TempDir()
	blockedPath := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	openPath := filepath.Join(root, ".agents", ".tasks", "active", "002.md")
	dependencyPath := filepath.Join(root, ".agents", ".tasks", "active", "003.md")
	writeTaskFile(t, blockedPath, "001", "Blocked task", "Blocked", "")
	writeTaskFile(t, openPath, "002", "Open task", "Open", "")
	writeTaskFile(t, dependencyPath, "003", "Dependency", "Pending", "")
	stubTaskWorkLookPath(t, func(string) (string, error) { return "/stub/codex", nil })
	calls := 0
	stubTaskWorkRunner(t, func(_ context.Context, _ string, _ string, args []string, _ io.Reader, stdout, _ io.Writer) error {
		calls++
		if calls == 1 {
			_, err := fmt.Fprintln(stdout, `{"verdicts":[{"task":"001","action":"accept","comment":"Incorrect original.","add_deps":[],"remove_deps":[],"labels":[],"revision":null},{"task":"002","action":"comment","comment":"Valid original.","add_deps":[],"remove_deps":[],"labels":[],"revision":null}]}`)
			return err
		}
		prompt := strings.Join(args, "\n")
		assertContainsAll(t, prompt,
			"Original request and target scope:",
			"- 001 [Blocked]",
			"- 002 [Open]",
			"Original structured result:",
			`"task": "001"`,
			"task 001 cannot be accepted from Blocked",
		)
		before, err := os.ReadFile(openPath)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(before), "Valid original.") {
			t.Fatal("original valid verdict was applied before correction")
		}
		_, err = fmt.Fprintln(stdout, `{"verdicts":[{"task":"001","action":"comment","comment":"Still blocked.","add_deps":[],"remove_deps":[],"labels":[],"revision":null},{"task":"002","action":"revise","comment":"Valid corrected verdict.","add_deps":["003"],"remove_deps":[],"labels":[],"revision":{"priority":"","effort":"","sections":[{"role":"problem","content":"Corrected problem once."}]}}]}`)
		return err
	})

	stdout, stderr, code := runCLI(t, "--root", root, "task", "groom", "--agent", "codex")
	if code != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "Correction retry succeeded", "task 001 cannot be accepted from Blocked", "001: comment", "002: revise", "added deps 003", "revised problem")
	assertContainsAll(t, stderr, "groom correction retry: attempting after 1 semantic validation error(s)")
	if calls != 2 {
		t.Fatalf("delegation calls = %d, want 2", calls)
	}
	for path, comment := range map[string]string{blockedPath: "Still blocked.", openPath: "Valid corrected verdict."} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(string(data), comment) != 1 {
			t.Fatalf("%s comment count = %d, want 1\n%s", path, strings.Count(string(data), comment), data)
		}
		if strings.Contains(string(data), "Incorrect original.") || strings.Contains(string(data), "Valid original.") {
			t.Fatalf("original verdict was partially applied:\n%s", data)
		}
	}
	openData, err := os.ReadFile(openPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(openData), "depends_on: 003") != 1 || strings.Count(string(openData), "Corrected problem once.") != 1 {
		t.Fatalf("corrected dependency or revision was duplicated:\n%s", openData)
	}
}

func TestTaskGroomCorrectionVisibleInJSON(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	writeTaskFile(t, path, "001", "Blocked task", "Blocked", "")
	stubTaskWorkLookPath(t, func(string) (string, error) { return "/stub/codex", nil })
	calls := 0
	stubTaskWorkRunner(t, func(_ context.Context, _ string, _ string, _ []string, _ io.Reader, stdout, _ io.Writer) error {
		calls++
		action, comment := "accept", "Invalid."
		if calls == 2 {
			action, comment = "comment", "Corrected."
		}
		_, err := fmt.Fprintf(stdout, `{"verdicts":[{"task":"001","action":%q,"comment":%q,"add_deps":[],"remove_deps":[],"labels":[],"revision":null}]}`+"\n", action, comment)
		return err
	})

	stdout, stderr, code := runCLI(t, "--root", root, "--json", "task", "groom", "001", "--agent", "codex")
	if code != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	var summary groomSummary
	if err := json.Unmarshal([]byte(stdout), &summary); err != nil {
		t.Fatalf("decode JSON summary: %v\n%s", err, stdout)
	}
	if summary.Correction == nil || !summary.Correction.Attempted || !summary.Correction.Succeeded {
		t.Fatalf("correction summary = %+v", summary.Correction)
	}
	if len(summary.Correction.ValidationErrors) != 1 || !strings.Contains(summary.Correction.ValidationErrors[0], "cannot be accepted from Blocked") {
		t.Fatalf("validation errors = %v", summary.Correction.ValidationErrors)
	}
}

func TestTaskGroomInvalidCorrectionMakesNoChanges(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	writeTaskFile(t, path, "001", "Blocked task", "Blocked", "")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	stubTaskWorkLookPath(t, func(string) (string, error) { return "/stub/codex", nil })
	calls := 0
	stubTaskWorkRunner(t, func(_ context.Context, _ string, _ string, _ []string, _ io.Reader, stdout, _ io.Writer) error {
		calls++
		_, err := fmt.Fprintln(stdout, `{"verdicts":[{"task":"001","action":"accept","comment":"Still invalid.","add_deps":[],"remove_deps":[],"labels":[],"revision":null}]}`)
		return err
	})

	_, stderr, code := runCLI(t, "--root", root, "task", "groom", "001", "--agent", "codex")
	if code == 0 {
		t.Fatal("expected failure")
	}
	assertContainsAll(t, stderr, "groom correction retry failed", "original structured result", "corrected structured result", "corrected validation errors", "task 001 cannot be accepted from Blocked")
	if calls != 2 {
		t.Fatalf("delegation calls = %d, want 2", calls)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("task changed after invalid original and corrected batches")
	}
}

func TestTaskGroomUnparseableCorrectionIsBoundedAndConcise(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	writeTaskFile(t, path, "001", "Blocked task", "Blocked", "")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	stubTaskWorkLookPath(t, func(string) (string, error) { return "/stub/codex", nil })
	calls := 0
	stubTaskWorkRunner(t, func(_ context.Context, _ string, _ string, _ []string, _ io.Reader, stdout, _ io.Writer) error {
		calls++
		if calls == 1 {
			_, err := fmt.Fprintln(stdout, `{"verdicts":[{"task":"001","action":"accept","comment":"Invalid.","add_deps":[],"remove_deps":[],"labels":[],"revision":null}]}`)
			return err
		}
		_, err := fmt.Fprintln(stdout, "provider secret payload that is not JSON")
		return err
	})

	_, stderr, code := runCLI(t, "--root", root, "task", "groom", "001", "--agent", "codex")
	if code == 0 {
		t.Fatal("expected failure")
	}
	assertContainsAll(t, stderr, "groom correction retry failed", "correction attempt failed", "correction output was not parseable", "original structured result", "original validation errors")
	if strings.Contains(stderr, "provider secret payload") {
		t.Fatalf("raw correction transcript was dumped:\n%s", stderr)
	}
	if calls != 2 {
		t.Fatalf("delegation calls = %d, want 2", calls)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("task changed after unparseable correction")
	}
}

func TestTaskGroomProviderFailureDoesNotRetry(t *testing.T) {
	root := t.TempDir()
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Open task", "Open", "")
	stubTaskWorkLookPath(t, func(string) (string, error) { return "/stub/codex", nil })
	calls := 0
	stubTaskWorkRunner(t, func(_ context.Context, _ string, _ string, _ []string, _ io.Reader, _ io.Writer, _ io.Writer) error {
		calls++
		return fmt.Errorf("provider unavailable")
	})

	_, stderr, code := runCLI(t, "--root", root, "task", "groom", "001", "--agent", "codex")
	if code == 0 {
		t.Fatal("expected failure")
	}
	assertContainsAll(t, stderr, "groom delegation failed", "provider unavailable")
	if calls != 1 {
		t.Fatalf("delegation calls = %d, want 1", calls)
	}
}

func TestTaskGroomStateRaceDoesNotRetry(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	writeTaskFile(t, path, "001", "Open task", "Open", "")
	stubTaskWorkLookPath(t, func(string) (string, error) { return "/stub/codex", nil })
	calls := 0
	stubTaskWorkRunner(t, func(_ context.Context, _ string, _ string, _ []string, _ io.Reader, stdout, _ io.Writer) error {
		calls++
		writeTaskFile(t, path, "001", "Changed task", "Blocked", "")
		_, err := fmt.Fprintln(stdout, `{"verdicts":[{"task":"001","action":"accept","comment":"Initially valid.","add_deps":[],"remove_deps":[],"labels":[],"revision":null}]}`)
		return err
	})

	_, stderr, code := runCLI(t, "--root", root, "task", "groom", "001", "--agent", "codex")
	if code == 0 {
		t.Fatal("expected failure")
	}
	assertContainsAll(t, stderr, "groom targets changed before apply", "cannot be accepted from Blocked")
	if calls != 1 {
		t.Fatalf("delegation calls = %d, want 1", calls)
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

func TestReplaceGroomSectionInsertsBeforeComments(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		role     string
		content  string
		wantLast string
	}{
		{
			name:     "inserts_before_existing_comments",
			body:     "## Problem\n\nSomething.\n\n## Comments\n\n- Old comment.\n",
			role:     "relevant_files",
			content:  "- main.go",
			wantLast: "## Comments",
		},
		{
			name:     "appends_when_no_comments",
			body:     "## Problem\n\nSomething.\n",
			role:     "acceptance_notes",
			content:  "- [ ] Test\n",
			wantLast: "## Acceptance Notes",
		},
		{
			name:     "inserts_before_comments_only",
			body:     "## Comments\n\nComment.\n",
			role:     "problem",
			content:  "The problem.\n",
			wantLast: "## Comments",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := replaceGroomSection(tt.body, tt.role, tt.content)
			if err != nil {
				t.Fatalf("replaceGroomSection() error = %v", err)
			}
			// Verify no error and the last section heading is what we expect.
			lines := strings.Split(got, "\n")
			lastHeading := ""
			for _, line := range lines {
				level := headingLevel(line)
				if level == 2 {
					lastHeading = strings.TrimSpace(line)
				}
			}
			if lastHeading != tt.wantLast {
				t.Errorf("last h2 heading = %q, want %q\nbody:\n%s", lastHeading, tt.wantLast, got)
			}
		})
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

func TestValidateGroomResultNoAliasing(t *testing.T) {
	fullCap := func(deps ...string) []string {
		s := make([]string, len(deps))
		copy(s, deps)
		return s
	}
	spareCap := func(deps ...string) []string {
		s := make([]string, 0, len(deps)+3)
		return append(s, deps...)
	}

	tasks := []Task{
		{ID: "001", Status: "Open", Labels: "type:task", DependsOn: fullCap("002", "003")},
		{ID: "002", Status: "Open", Labels: "type:task", DependsOn: spareCap("004")},
		{ID: "003", Status: "Pending", Labels: "type:task"},
		{ID: "004", Status: "Pending", Labels: "type:task"},
	}

	before := make([][]string, len(tasks))
	for i := range tasks {
		before[i] = append([]string(nil), tasks[i].DependsOn...)
	}

	result := groomResult{Verdicts: []groomVerdict{
		{Task: "001", Action: "accept", Comment: "Ready.", AddDeps: []string{"004"}, RemoveDeps: []string{"002"}, Labels: []string{}},
		{Task: "002", Action: "comment", Comment: "Still blocked.", AddDeps: []string{}, RemoveDeps: []string{"004"}, Labels: []string{}},
		{Task: "003", Action: "comment", Comment: "OK.", AddDeps: []string{}, RemoveDeps: []string{}, Labels: []string{}},
		{Task: "004", Action: "comment", Comment: "OK.", AddDeps: []string{}, RemoveDeps: []string{}, Labels: []string{}},
	}}

	for run := 0; run < 50; run++ {
		if _, err := validateGroomResult(result, tasks, tasks); err != nil {
			t.Fatalf("run %d: validateGroomResult error = %v", run, err)
		}
		for i := range tasks {
			if !slices.Equal(tasks[i].DependsOn, before[i]) {
				t.Fatalf("run %d: task %s DependsOn mutated: got %v, want %v", run, tasks[i].ID, tasks[i].DependsOn, before[i])
			}
		}
	}
}

func TestGroomTargetsRejectsNonOpenBlockedExplicitIDs(t *testing.T) {
	tasks := []Task{
		{ID: "001", Status: "Open", Labels: "type:task"},
		{ID: "002", Status: "Blocked", Labels: "type:task"},
		{ID: "003", Status: "Pending", Labels: "type:task"},
		{ID: "004", Status: "In Progress", Labels: "type:task"},
		{ID: "005", Status: "Tracking", Labels: "type:task"},
		{ID: "006", Status: "Completed", Labels: "type:task"},
		{ID: "007", Status: "Cancelled", Labels: "type:task"},
	}
	t.Run("open", func(t *testing.T) {
		got, err := groomTargets(tasks, "001")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != "001" {
			t.Fatalf("got %v, want [001]", got)
		}
	})
	t.Run("blocked", func(t *testing.T) {
		got, err := groomTargets(tasks, "002")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != "002" {
			t.Fatalf("got %v, want [002]", got)
		}
	})
	for _, id := range []string{"003", "004", "005", "006", "007"} {
		t.Run(id, func(t *testing.T) {
			_, err := groomTargets(tasks, id)
			if err == nil {
				t.Fatalf("expected error for task %s", id)
			}
			if !strings.Contains(err.Error(), "cannot be groomed") {
				t.Fatalf("unexpected error for %s: %v", id, err)
			}
		})
	}
}

func TestValidateGroomResultRejectsTerminalTaskVerdicts(t *testing.T) {
	completed := Task{ID: "001", Status: "Completed", Labels: "type:task"}
	cancelled := Task{ID: "002", Status: "Cancelled", Labels: "type:task"}
	all := []Task{completed, cancelled}

	for _, action := range []string{"comment", "revise", "accept"} {
		t.Run("completed_"+action, func(t *testing.T) {
			result := groomResult{Verdicts: []groomVerdict{
				{Task: "001", Action: action, Comment: "x", AddDeps: []string{}, RemoveDeps: []string{}, Labels: []string{}},
			}}
			_, err := validateGroomResult(result, all, all)
			if err == nil || !strings.Contains(err.Error(), "is Completed and cannot be groomed") {
				t.Fatalf("expected Completed rejection for %s, got %v", action, err)
			}
		})
		t.Run("cancelled_"+action, func(t *testing.T) {
			result := groomResult{Verdicts: []groomVerdict{
				{Task: "002", Action: action, Comment: "x", AddDeps: []string{}, RemoveDeps: []string{}, Labels: []string{}},
			}}
			_, err := validateGroomResult(result, all, all)
			if err == nil || !strings.Contains(err.Error(), "is Cancelled and cannot be groomed") {
				t.Fatalf("expected Cancelled rejection for %s, got %v", action, err)
			}
		})
	}
}

func TestTaskGroomRejectsNonOpenBlockedBeforeDelegation(t *testing.T) {
	for _, tc := range []struct{ id, status, bucket string }{
		{"003", "Pending", "active"},
		{"004", "In Progress", "active"},
		{"005", "Tracking", "active"},
		{"006", "Completed", "completed"},
		{"007", "Cancelled", "cancelled"},
	} {
		t.Run(tc.status, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, ".agents", ".tasks", tc.bucket, tc.id+".md")
			writeTaskFile(t, path, tc.id, tc.status+" task", tc.status, "")
			writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Open task", "Open", "")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			stubTaskWorkLookPath(t, func(string) (string, error) {
				t.Fatal("agent lookup must not run for a non-groomable task")
				return "", nil
			})

			stdout, stderr, code := runCLI(t, "--root", root, "task", "groom", tc.id, "--agent", "codex")
			if code == 0 {
				t.Fatalf("expected failure for %s task, stdout=%s stderr=%s", tc.status, stdout, stderr)
			}
			if !strings.Contains(stderr, "cannot be groomed") && !strings.Contains(stderr, "is in status") {
				t.Fatalf("expected rejection for %s in stderr, got: %s", tc.status, stderr)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("original record missing: %v", err)
			}
			if string(after) != string(before) {
				t.Fatalf("record was modified:\n%s", after)
			}
			if tc.bucket != "active" {
				if _, err := os.Stat(filepath.Join(root, ".agents", ".tasks", "active", tc.id+".md")); !os.IsNotExist(err) {
					t.Fatalf("terminal task was written to active bucket: %v", err)
				}
			}
		})
	}
}

func TestApplyGroomVerdictsRejectsTerminalTaskBeforeWrites(t *testing.T) {
	root := t.TempDir()
	openPath := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	completedPath := filepath.Join(root, ".agents", ".tasks", "completed", "002.md")
	writeTaskFile(t, openPath, "001", "Open task", "Open", "")
	writeTaskFile(t, completedPath, "002", "Completed task", "Completed", "")
	openBefore, err := os.ReadFile(openPath)
	if err != nil {
		t.Fatal(err)
	}
	completedBefore, err := os.ReadFile(completedPath)
	if err != nil {
		t.Fatal(err)
	}

	a := &app{opts: options{root: root}, out: io.Discard, err: io.Discard}
	verdicts := []groomVerdict{
		{Task: "001", Action: "comment", Comment: "Should not be written."},
		{Task: "002", Action: "comment", Comment: "Must be rejected."},
	}
	_, err = a.applyGroomVerdicts(verdicts, []Task{
		{ID: "001", Status: "Open", Path: openPath},
		{ID: "002", Status: "Completed", Path: completedPath},
	}, "test")
	if err == nil || !strings.Contains(err.Error(), "cannot be relocated by grooming") {
		t.Fatalf("expected terminal-task rejection, got %v", err)
	}
	openAfter, err := os.ReadFile(openPath)
	if err != nil {
		t.Fatal(err)
	}
	completedAfter, err := os.ReadFile(completedPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(openAfter) != string(openBefore) || string(completedAfter) != string(completedBefore) {
		t.Fatal("groom application wrote task records before rejecting a terminal task")
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", ".tasks", "active", "002.md")); !os.IsNotExist(err) {
		t.Fatalf("terminal task was written to active bucket: %v", err)
	}
}

func TestApplyGroomVerdictsNoAliasing(t *testing.T) {
	root := t.TempDir()
	writeTaskFileWithDeps(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "First", "Open", "002,003")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Second", "Pending", "")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "003.md"), "003", "Third", "Pending", "")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "004.md"), "004", "Fourth", "Pending", "")

	a := &app{opts: options{root: root}, out: io.Discard, err: io.Discard}
	verdicts := []groomVerdict{
		{Task: "001", Action: "accept", Comment: "Ready.", AddDeps: []string{"004"}, RemoveDeps: []string{"002"}, Labels: []string{}},
	}

	for run := 0; run < 10; run++ {
		tasks, err := a.getTasks()
		if err != nil {
			t.Fatal(err)
		}
		before := make([][]string, len(tasks))
		for i := range tasks {
			before[i] = append([]string(nil), tasks[i].DependsOn...)
		}

		summary, err := a.applyGroomVerdicts(verdicts, tasks, "test")
		if err != nil {
			t.Fatalf("run %d: applyGroomVerdicts error = %v", run, err)
		}
		if len(summary.Changes) != 1 {
			t.Fatalf("run %d: changes = %d, want 1", run, len(summary.Changes))
		}
		change := summary.Changes[0]
		if !slices.Equal(change.AddedDeps, []string{"004"}) {
			t.Errorf("run %d: AddedDeps = %v, want [004]", run, change.AddedDeps)
		}
		if !slices.Equal(change.RemovedDeps, []string{"002"}) {
			t.Errorf("run %d: RemovedDeps = %v, want [002]", run, change.RemovedDeps)
		}

		for i := range tasks {
			if !slices.Equal(tasks[i].DependsOn, before[i]) {
				t.Fatalf("run %d: task %s DependsOn mutated: got %v, want %v", run, tasks[i].ID, tasks[i].DependsOn, before[i])
			}
		}

		data, err := os.ReadFile(filepath.Join(root, ".agents", ".tasks", "active", "001.md"))
		if err != nil {
			t.Fatal(err)
		}
		assertContainsAll(t, string(data), "depends_on: 003, 004", "status: Pending")

		// Reset the file for the next deterministic run.
		if run < 9 {
			writeTaskFileWithDeps(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "First", "Open", "002,003")
			a.invalidateTasks()
		}
	}
}
