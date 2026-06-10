package ahm

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/travisennis/ahm/internal/templates"
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
	validateTaskFrontMatter([]byte(content), relPath(root, path), &report)
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

func TestDoctorReportsCompletedTaskAcceptanceFindings(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	writeCompletedTaskBody(t, root, "001", "Missing Acceptance", "## Summary\n\nDone.\n")
	writeCompletedTaskBody(t, root, "002", "Placeholder Acceptance", "## Acceptance Notes\n\n- [ ] TODO\n")
	writeCompletedTaskBody(t, root, "003", "Unchecked Acceptance", "## Acceptance Criteria\n\n* [ ] Verify it\n")

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.doctor(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	assertContainsAll(t, got,
		`"ok": true`,
		`"code": "task_acceptance_missing"`,
		`"code": "task_acceptance_placeholder"`,
		`"code": "task_acceptance_unchecked"`,
	)
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
	assertContainsAll(t, got,
		`"code": "metadata_missing"`,
		`"installed_version": null`,
	)
	assertNotContains(t, got,
		`"code": "generated_index_missing"`,
		`"code": "markdown_link_missing"`,
		`"installed_version": ""`,
	)
}

func TestStatusWithMetadataShowsInstalledVersion(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	// JSON mode: installed_version shows the version string.
	var jOut strings.Builder
	a := app{opts: options{root: root, json: true}, out: &jOut}
	if err := a.status(); err != nil {
		t.Fatalf("status error: %v", err)
	}
	jGot := jOut.String()
	assertContainsAll(t, jGot, `"installed_version": "`+templates.Version+`"`)

	// Text mode: installed_version shows the version string.
	var tOut strings.Builder
	a2 := app{opts: options{root: root}, out: &tOut}
	if err := a2.status(); err != nil {
		t.Fatalf("status error: %v", err)
	}
	tGot := tOut.String()
	assertContainsAll(t, tGot, "installed_version: "+templates.Version)
}

func TestDoctorWithoutMetadataShowsInstalledVersionNone(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	// JSON mode: installed_version shows null.
	var jOut strings.Builder
	a := app{opts: options{root: root, json: true}, out: &jOut}
	if err := a.doctor(); !errors.Is(err, errValidationFailed) {
		t.Fatalf("expected errValidationFailed, got: %v", err)
	}
	jGot := jOut.String()
	assertContainsAll(t, jGot, `"installed_version": null`)
	assertNotContains(t, jGot, `"installed_version": ""`)

	// Text mode: installed_version shows none.
	var tOut strings.Builder
	a2 := app{opts: options{root: root}, out: &tOut}
	if err := a2.doctor(); !errors.Is(err, errValidationFailed) {
		t.Fatalf("expected errValidationFailed, got: %v", err)
	}
	tGot := tOut.String()
	assertContainsAll(t, tGot, "installed_version: none")
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

func TestStatusReportsCompletedTaskReferencingIncompleteCompletedExecPlan(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".agents", ".tasks", "completed", "001.md"), "---\n"+
		"id: 001\n"+
		"title: Plan Incomplete\n"+
		"status: Completed\n"+
		"priority: P2\n"+
		"effort: S\n"+
		"labels: type:task\n"+
		"exec_plan: rollout\n"+
		"depends_on: []\n"+
		"---\n"+
		"# Plan Incomplete\n\n"+
		"## Summary\n\nDone.\n")
	writeFile(t, filepath.Join(root, ".agents", "exec-plans", "completed", "rollout.md"), "# Rollout\n\n"+
		"## Progress\n\n- [x] Do it.\n\n"+
		"## Surprises & Discoveries\n\nNone.\n\n"+
		"## Decision Log\n\n- Chose this.\n\n"+
		"## Outcomes & Retrospective\n\n")

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.status(); err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, out.String(),
		`"code": "task_completed_exec_plan_incomplete"`,
		`completed task 001 references ExecPlan without a completed Outcomes \u0026 Retrospective section`,
	)
}

func TestValidateExecPlansReportsLifecycleFindings(t *testing.T) {
	tests := []struct {
		name       string
		bucket     string
		content    string
		tasks      []Task
		wantWarn   string
		wantInfo   string
		wantNoWarn string
	}{
		{
			name:   "active with outcomes",
			bucket: "active",
			content: "# Plan\n\n" +
				"## Progress\n\n- [ ] Do it.\n\n" +
				"## Surprises & Discoveries\n\nNone yet.\n\n" +
				"## Decision Log\n\n- Chose this.\n\n" +
				"## Outcomes & Retrospective\n\nDone early.\n",
			tasks:    []Task{{ExecPlan: ".agents/exec-plans/active/plan.md"}},
			wantWarn: "exec_plan_active_with_outcomes",
		},
		{
			name:   "completed without outcomes",
			bucket: "completed",
			content: "# Plan\n\n" +
				"### Progress\n\n- [x] Do it.\n\n" +
				"### Surprises & Discoveries\n\nNone.\n\n" +
				"### Decision Log\n\n- Chose this.\n\n" +
				"### Outcomes & Retrospective\n\n" +
				"## Later Section\n\nThis does not count as outcomes.\n",
			tasks:    []Task{{ExecPlan: ".agents/exec-plans/completed/plan.md"}},
			wantWarn: "exec_plan_completed_without_outcomes",
		},
		{
			name:   "completed with open progress",
			bucket: "completed",
			content: "# Plan\n\n" +
				"## Progress\n\n- [ ] Do it.\n\n" +
				"## Surprises & Discoveries\n\nNone.\n\n" +
				"## Decision Log\n\n- Chose this.\n\n" +
				"## Outcomes & Retrospective\n\nDone.\n",
			tasks:    []Task{{ExecPlan: ".agents/exec-plans/completed/plan.md"}},
			wantWarn: "exec_plan_completed_with_open_progress",
		},
		{
			name:   "missing section",
			bucket: "active",
			content: "# Plan\n\n" +
				"## Progress\n\n- [ ] Do it.\n\n" +
				"## Decision Log\n\n- Chose this.\n\n" +
				"## Outcomes & Retrospective\n\n",
			tasks:    []Task{{ExecPlan: ".agents/exec-plans/active/plan.md"}},
			wantWarn: "exec_plan_missing_section",
		},
		{
			name:   "orphan info",
			bucket: "active",
			content: "# Plan\n\n" +
				"## Progress\n\n- [ ] Do it.\n\n" +
				"## Surprises & Discoveries\n\nNone.\n\n" +
				"## Decision Log\n\n- Chose this.\n\n" +
				"## Outcomes & Retrospective\n\n",
			wantInfo:   "exec_plan_orphan",
			wantNoWarn: "exec_plan_orphan",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, ".agents", "exec-plans", tt.bucket, "plan.md")
			writeFile(t, path, tt.content)

			report := validationReport{OK: true, Errors: []validationFinding{}, Warnings: []validationFinding{}, Info: []validationFinding{}}
			validateExecPlans(root, tt.tasks, &report)

			if tt.wantWarn != "" && !hasFinding(report.Warnings, tt.wantWarn) {
				t.Fatalf("missing warning %q: %#v", tt.wantWarn, report.Warnings)
			}
			if tt.wantInfo != "" && !hasFinding(report.Info, tt.wantInfo) {
				t.Fatalf("missing info %q: %#v", tt.wantInfo, report.Info)
			}
			if tt.wantNoWarn != "" && hasFinding(report.Warnings, tt.wantNoWarn) {
				t.Fatalf("unexpected warning %q: %#v", tt.wantNoWarn, report.Warnings)
			}
		})
	}
}

func TestDoctorJSONReportsExecPlanInfoWithoutFailing(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".agents", "exec-plans", "active", "orphan.md"), "# Orphan\n\n"+
		"## Progress\n\n- [ ] Do it.\n\n"+
		"## Surprises & Discoveries\n\nNone yet.\n\n"+
		"## Decision Log\n\n- Chose this.\n\n"+
		"## Outcomes & Retrospective\n\n")

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.doctor(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	assertContainsAll(t, got,
		`"ok": true`,
		`"info": [`,
		`"code": "exec_plan_orphan"`,
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

func hasFinding(findings []validationFinding, code string) bool {
	for _, finding := range findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

func writeCompletedTaskBody(t *testing.T, root string, id string, title string, body string) {
	t.Helper()
	writeFile(t, filepath.Join(root, ".agents", ".tasks", "completed", id+".md"), "---\n"+
		"id: "+id+"\n"+
		"title: "+title+"\n"+
		"status: Completed\n"+
		"priority: P2\n"+
		"effort: S\n"+
		"labels: type:task, area:tasks\n"+
		"exec_plan: -\n"+
		"depends_on: -\n"+
		"---\n"+
		"# "+title+"\n\n"+
		body)
}

func TestValidateWorkflowScopedWorkflowOnly(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	// Add a broken link that would trigger markdown_link_missing.
	writeFile(t, filepath.Join(root, ".agents", ".research", "topics", "links.md"), "# Links\n\n[missing](missing.md)\n")
	// Add a workflow issue: remove a non-create-only managed file.
	if err := os.Remove(filepath.Join(root, ".agents", "TASKS.md")); err != nil {
		t.Fatal(err)
	}

	// Only workflow checks.
	report, _ := validateWorkflowScoped(root, []string{CheckScopeWorkflow})
	// Should find managed_file_missing (workflow check) but NOT markdown_link_missing.
	foundManagedMissing := false
	for _, e := range report.Errors {
		if e.Code == "managed_file_missing" {
			foundManagedMissing = true
		}
	}
	if !foundManagedMissing {
		t.Fatal("expected managed_file_missing in workflow-only scope")
	}
	for _, e := range report.Errors {
		if e.Code == "markdown_link_missing" {
			t.Fatal("unexpected markdown_link_missing in workflow-only scope")
		}
	}
}

func TestValidateWorkflowScopedLinksOnly(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	// Add a broken link.
	writeFile(t, filepath.Join(root, ".agents", ".research", "topics", "links.md"), "# Links\n\n[missing](missing.md)\n")
	// Create a workflow issue: remove a non-create-only managed file.
	if err := os.Remove(filepath.Join(root, ".agents", "TASKS.md")); err != nil {
		t.Fatal(err)
	}

	// Only link checks.
	report, _ := validateWorkflowScoped(root, []string{CheckScopeLinks})
	// Should find markdown_link_missing.
	foundLinkMissing := false
	for _, w := range report.Warnings {
		if w.Code == "markdown_link_missing" {
			foundLinkMissing = true
			break
		}
	}
	if !foundLinkMissing {
		t.Fatal("expected markdown_link_missing in links-only scope")
	}
	// Should NOT find managed_file_missing (workflow check).
	for _, e := range report.Errors {
		if e.Code == "managed_file_missing" {
			t.Fatal("unexpected managed_file_missing in links-only scope")
		}
	}
	// No workflow errors since we only ran link checks.
	if !report.OK {
		t.Fatal("expected OK for links-only scope, got errors")
	}
}

func TestValidateWorkflowScopedProjectDocsNoDocs(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	// A fresh install has only docs/adr/README.md (no broken links) and no
	// other project docs; the project-docs scope should produce no findings.
	report, tasks := validateWorkflowScoped(root, []string{CheckScopeProjectDocs})
	if !report.OK {
		t.Fatal("expected OK for project-docs scope, got errors")
	}
	if len(report.Errors)+len(report.Warnings)+len(report.Info) > 0 {
		t.Fatalf("unexpected findings for project-docs scope: %#v", report)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected no tasks for project-docs scope, got %d", len(tasks))
	}
}

func TestValidateWorkflowScopedProjectDocsCommonDocsValid(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	// Common project docs with only valid relative links should pass.
	writeFile(t, filepath.Join(root, "docs", "guide.md"), "# Guide\n")
	writeFile(t, filepath.Join(root, "README.md"), "# Project\n\nSee [the guide](docs/guide.md).\n")
	writeFile(t, filepath.Join(root, "CONTRIBUTING.md"), "# Contributing\n\n[Back to README](README.md)\n")

	report, _ := validateWorkflowScoped(root, []string{CheckScopeProjectDocs})
	if !report.OK {
		t.Fatal("expected OK for valid project docs, got errors")
	}
	for _, w := range report.Warnings {
		if w.Code == "project_doc_link_missing" {
			t.Fatalf("unexpected project_doc_link_missing for valid docs: %#v", w)
		}
	}
}

func TestValidateWorkflowScopedProjectDocsBrokenLinks(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(root, "README.md"), "# Project\n\nSee [missing doc](docs/nope.md).\n")
	writeFile(t, filepath.Join(root, "docs", "design.md"), "# Design\n\n[gone](./absent.md)\n")

	report, _ := validateWorkflowScoped(root, []string{CheckScopeProjectDocs})
	var found []string
	for _, w := range report.Warnings {
		if w.Code == "project_doc_link_missing" {
			found = append(found, w.Path)
		}
	}
	if len(found) != 2 {
		t.Fatalf("expected 2 project_doc_link_missing findings, got %d: %#v", len(found), report.Warnings)
	}
}

func TestValidateWorkflowScopedProjectDocsNotDefault(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	// A broken project-doc link must not surface in the default (all) scope.
	writeFile(t, filepath.Join(root, "README.md"), "# Project\n\nSee [missing doc](docs/nope.md).\n")

	report, _ := validateWorkflowScoped(root, nil)
	for _, w := range report.Warnings {
		if w.Code == "project_doc_link_missing" {
			t.Fatalf("project-docs check ran by default; found %#v", w)
		}
	}
}

func TestValidateWorkflowScopedDesignDocsAbsent(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	// No docs/design-docs/ surface: no design-doc findings.
	report, _ := validateWorkflowScoped(root, []string{CheckScopeProjectDocs})
	for _, w := range report.Warnings {
		if w.Code == "design_doc_unindexed" {
			t.Fatalf("unexpected design_doc_unindexed without design docs: %#v", w)
		}
	}
}

func TestValidateWorkflowScopedDesignDocsDirWithoutIndex(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	// A design-docs directory without index.md does not adopt the convention.
	writeFile(t, filepath.Join(root, "docs", "design-docs", "auth.md"), "# Auth\n")

	report, _ := validateWorkflowScoped(root, []string{CheckScopeProjectDocs})
	for _, w := range report.Warnings {
		if w.Code == "design_doc_unindexed" {
			t.Fatalf("unexpected design_doc_unindexed without index.md: %#v", w)
		}
	}
}

func TestValidateWorkflowScopedDesignDocsValid(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(root, "docs", "design-docs", "auth.md"), "# Auth\n")
	writeFile(t, filepath.Join(root, "docs", "design-docs", "storage.md"), "# Storage\n")
	writeFile(t, filepath.Join(root, "docs", "design-docs", "index.md"),
		"# Design Docs\n\n- [Auth](auth.md)\n- [Storage](storage.md)\n")

	report, _ := validateWorkflowScoped(root, []string{CheckScopeProjectDocs})
	for _, w := range report.Warnings {
		if w.Code == "design_doc_unindexed" || w.Code == "project_doc_link_missing" {
			t.Fatalf("unexpected finding for valid design docs: %#v", w)
		}
	}
}

func TestValidateWorkflowScopedDesignDocsUnindexed(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(root, "docs", "design-docs", "auth.md"), "# Auth\n")
	writeFile(t, filepath.Join(root, "docs", "design-docs", "orphan.md"), "# Orphan\n")
	writeFile(t, filepath.Join(root, "docs", "design-docs", "index.md"),
		"# Design Docs\n\n- [Auth](auth.md)\n")

	report, _ := validateWorkflowScoped(root, []string{CheckScopeProjectDocs})
	var found []string
	for _, w := range report.Warnings {
		if w.Code == "design_doc_unindexed" {
			found = append(found, w.Path)
		}
	}
	if len(found) != 1 || found[0] != "docs/design-docs/orphan.md" {
		t.Fatalf("expected 1 design_doc_unindexed for orphan.md, got %#v", found)
	}
}

func TestValidateWorkflowScopedDesignDocsIndexEntryMissing(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	// The index points at a source file that does not exist. This reuses the
	// project-doc relative-link finding rather than adding a parallel check.
	writeFile(t, filepath.Join(root, "docs", "design-docs", "index.md"),
		"# Design Docs\n\n- [Gone](gone.md)\n")

	report, _ := validateWorkflowScoped(root, []string{CheckScopeProjectDocs})
	foundLink := false
	for _, w := range report.Warnings {
		if w.Code == "project_doc_link_missing" && strings.HasPrefix(w.Path, "docs/design-docs/index.md") {
			foundLink = true
		}
		if w.Code == "design_doc_unindexed" {
			t.Fatalf("design_doc_unindexed should not fire for missing index entry: %#v", w)
		}
	}
	if !foundLink {
		t.Fatal("expected project_doc_link_missing for missing design-doc index entry")
	}
}

func TestValidateWorkflowScopedDesignDocsNotDefault(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(root, "docs", "design-docs", "orphan.md"), "# Orphan\n")
	writeFile(t, filepath.Join(root, "docs", "design-docs", "index.md"), "# Design Docs\n")

	report, _ := validateWorkflowScoped(root, nil)
	for _, w := range report.Warnings {
		if w.Code == "design_doc_unindexed" {
			t.Fatalf("design-doc check ran by default; found %#v", w)
		}
	}
}

func TestValidateWorkflowScopedAll(t *testing.T) {
	// nil scopes = default checks (same as validateWorkflow): workflow + links,
	// but not the opt-in project-docs scope.
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".agents", ".research", "topics", "links.md"), "# Links\n\n[missing](missing.md)\n")

	// No scopes = default checks run.
	report, _ := validateWorkflowScoped(root, nil)
	foundLinkMissing := false
	for _, w := range report.Warnings {
		if w.Code == "markdown_link_missing" {
			foundLinkMissing = true
			break
		}
	}
	if !foundLinkMissing {
		t.Fatal("expected markdown_link_missing when running all checks")
	}
	// validateWorkflow should produce the same result.
	report2, _ := validateWorkflow(root)
	if report.OK != report2.OK {
		t.Fatal("validateWorkflowScoped(nil) should match validateWorkflow")
	}
	if len(report.Errors) != len(report2.Errors) {
		t.Fatalf("error count mismatch: %d vs %d", len(report.Errors), len(report2.Errors))
	}
}

func TestCLIStatusInvalidCheckScope(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "--root", root, "status", "--check", "bogus")
	if code != 2 {
		t.Fatalf("expected exit code 2 for invalid check scope, got %d; stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "unknown check scope") {
		t.Fatalf("expected unknown check scope error, got: %s", stderr)
	}
	_ = stdout
}

func TestCLIDoctorWithCheckScope(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	// Doctor with --check workflow should succeed (no issues in a fresh install).
	stdout, stderr, code := runCLI(t, "--root", root, "doctor", "--check", "workflow")
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s, stdout=%s", code, stderr, stdout)
	}
	assertContainsAll(t, stdout, `"ok": true`)
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
	validateTaskFrontMatter([]byte(content), relPath(root, path), report)
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
