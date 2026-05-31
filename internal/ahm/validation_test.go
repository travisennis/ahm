package ahm

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateTaskFrontMatter_CRLF(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	// Write a valid task with CRLF.
	path := filepath.Join(root, ".agents", ".tasks", "active", "097.md")
	content := "---\r\n" +
		"id: 097\r\n" +
		"title: Validate CRLF\r\n" +
		"status: Pending\r\n" +
		"priority: P2\r\n" +
		"effort: S\r\n" +
		"labels: type:test, area:workflow\r\n" +
		"exec_plan: -\r\n" +
		"depends_on: -\r\n" +
		"---\r\n" +
		"# Validate CRLF\r\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	var report validationReport
	validateTaskFrontMatter(root, path, &report)
	for _, e := range report.Errors {
		t.Errorf("validation error for CRLF task: %s: %s", e.Code, e.Message)
	}
	for _, w := range report.Warnings {
		t.Errorf("validation warning for CRLF task: %s: %s", w.Code, w.Message)
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
	if err := a.status(); !errors.Is(err, errValidationFailed) {
		t.Fatalf("expected errValidationFailed, got: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		`"ok": false`,
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
	if err := a.doctor(); !errors.Is(err, errValidationFailed) {
		t.Fatalf("expected errValidationFailed, got: %v", err)
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

func TestStatusWithoutMetadataDoesNotCascadeWorkflowArtifactFindings(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.status(); !errors.Is(err, errValidationFailed) {
		t.Fatalf("expected errValidationFailed, got: %v", err)
	}
	got := out.String()
	assertContainsAll(t, got, `"code": "metadata_missing"`)
	assertNotContains(t, got,
		`"code": "generated_index_missing"`,
		`"code": "markdown_link_missing"`,
	)
}

func TestStatusReportsWorkflowArtifactConsistency(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Completed In Active", "Completed", "depends_on: []\n")
	writeFile(t, filepath.Join(root, ".agents", ".research", "topics", "new-note.md"), "# New Note\n\nThis should make the research index stale.\n")
	if err := os.Remove(filepath.Join(root, ".agents", ".tasks", "cancelled", "index.md")); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.status(); !errors.Is(err, errValidationFailed) {
		t.Fatalf("expected errValidationFailed, got: %v", err)
	}
	got := out.String()
	assertContainsAll(t, got,
		`"code": "task_bucket_mismatch"`,
		`completed task should be in .agents/.tasks/completed`,
		`"code": "generated_index_missing"`,
		`"path": ".agents/.tasks/cancelled/index.md"`,
		`"code": "generated_index_stale"`,
		`"path": ".agents/.research/index.md"`,
	)
}

func TestStatusReportsCompletedTaskReferencingActiveExecPlan(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".agents", ".tasks", "completed", "001.md"), "---\n"+
		"id: 001\n"+
		"title: Plan Still Active\n"+
		"status: Completed\n"+
		"priority: P2\n"+
		"effort: S\n"+
		"labels: type:task\n"+
		"exec_plan: rollout\n"+
		"depends_on: []\n"+
		"---\n"+
		"# Plan Still Active\n\n"+
		"## Summary\n\nDone.\n")
	writeFile(t, filepath.Join(root, ".agents", "exec-plans", "active", "rollout.md"), "# Rollout\n\n## Outcomes & Retrospective\n\n")

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.status(); err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, out.String(),
		`"code": "task_completed_exec_plan_active"`,
		`completed task 001 references active ExecPlan .agents/exec-plans/active/rollout.md`,
	)
}

func TestStatusReportsMarkdownLinksInWorkflowFiles(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".agents", ".research", "topics", "links.md"), "# Links\n\n[missing](missing.md)\n\n```md\n[ignored](also-missing.md)\n```\n")

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.doctor(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	assertContainsAll(t, got,
		`"code": "markdown_link_missing"`,
		`"path": ".agents/.research/topics/links.md:3"`,
		`relative Markdown link target does not exist: missing.md`,
	)
	assertNotContains(t, got, "also-missing.md")
}

func TestValidateTaskFrontMatterReportsParseErrors(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".agents", ".tasks", "active", "001.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	// Block scalar in front matter should produce a parse error, not missing-field errors.
	content := "---\n" +
		"id: 001\n" +
		"title: Bad\n" +
		"status: Pending\n" +
		"priority: P1\n" +
		"effort: M\n" +
		"labels: type:bug\n" +
		"exec_plan: -\n" +
		"depends_on: -\n" +
		"description: |\n" +
		"  multi\n" +
		"  line\n" +
		"---\n" +
		"# Bad\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	report := &validationReport{}
	validateTaskFrontMatter(root, path, report)
	if len(report.Errors) == 0 {
		t.Fatal("expected at least one error, got none")
	}
	found := false
	for _, e := range report.Errors {
		if e.Code == "task_malformed" && strings.Contains(e.Message, "unsupported block scalar") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected task_malformed error with block scalar message, got: %v", report.Errors)
	}
	// Verify no missing-field errors (which would be misleading)
	for _, e := range report.Errors {
		if e.Code == "task_missing_field" {
			t.Fatalf("unexpected missing_field error when front matter is malformed: %v", e)
		}
	}
}
