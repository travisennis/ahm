package ahm

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestValidateWorkflowStateMatchesStandaloneValidation(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	paths := workflowPathsFor(root)
	path := paths.taskFile("active", "301")
	writeFile(t, path, `---
id: 301
title: Missing labels
status: Pending
priority: P2
effort: S
exec_plan: -
depends_on: -
---
# Missing labels

## Acceptance Notes

- [ ] Preserve validation findings.
`)

	tasks, err := collectTasksForPaths(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	writes, err := indexWritesForPaths(root, tasks, paths)
	if err != nil {
		t.Fatal(err)
	}
	for target, content := range writes {
		if err := writeFileAtomic(target, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	assertEquivalent := func(context string) {
		t.Helper()
		standalone, _ := validateWorkflowScopedForPaths(root, []string{CheckScopeWorkflow}, paths)
		reused := validateWorkflowStateForPaths(root, paths, tasks, writes)
		if standalone.OK != reused.OK ||
			!reflect.DeepEqual(standalone.Errors, reused.Errors) ||
			!reflect.DeepEqual(standalone.Warnings, reused.Warnings) ||
			!reflect.DeepEqual(standalone.Info, reused.Info) {
			t.Fatalf("%s: reused validation differs from standalone\nstandalone: %+v\nreused: %+v", context, standalone, reused)
		}
	}
	assertEquivalent("valid metadata")

	writeFile(t, filepath.Join(root, ".ahm", "config.json"), "{")
	assertEquivalent("corrupt metadata")
}

func TestValidateTaskFrontMatter_CRLF(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)

	// Write a valid task with CRLF.
	path := filepath.Join(root, ".ahm", "tasks", "active", "097.md")
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
	setupAhmRepo(t, root)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Blocked Task", "Pending", "depends_on: 999\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "002", "Cycle A", "Pending", "depends_on: 003\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "003.md"), "003", "Cycle B", "Pending", "depends_on: 002\n")

	var out strings.Builder

	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.status(); !errors.Is(err, errValidationFailed) {
		t.Errorf("expected errValidationFailed, got: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		`"ok": false`,
		`"code": "task_dependency_missing"`,
		`task 001 depends on missing task 999`,
		`"code": "task_dependency_cycle"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("status output missing %q:\n%s", want, got)
		}
	}
}

func TestValidationReportsCancelledDependency(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Active Task", "Pending", "depends_on: 002\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "cancelled", "002.md"), "002", "Cancelled Task", "Cancelled", "depends_on: -\n")

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.doctor(); err != nil {
		t.Error(err)
	}
	got := out.String()
	assertContainsAll(t, got,
		`"ok": true`,
		`"code": "task_dependency_cancelled"`,
		`task 001 depends on cancelled task 002`,
	)
}

func TestDoctorReportsStaleResearchInboxDisposition(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	created := time.Now().UTC().AddDate(0, 0, -30).Format(time.DateOnly)
	writeFile(t, filepath.Join(root, ".ahm", "research", "inbox", "old-note.md"), "# Old Note\n\nCreated: "+created+"\n")
	indexer := app{opts: options{root: root}, out: &strings.Builder{}}
	if err := indexer.writeIndexes(); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.doctor(); err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, out.String(),
		`"code": "research_inbox_stale"`,
		`"path": ".ahm/research/inbox/old-note.md"`,
		"threshold 21",
		"promote it to research/topics",
		"convert it to a task",
		"delete it if it has no continuing value",
	)
}

func TestValidationReportsBlockedDepsComplete(t *testing.T) {
	root := t.TempDir()
	// 002 is Blocked but all its deps (001) are Completed.
	// Use writeTaskFileWithDeps for all tasks so depends_on is always present.
	writeTaskFileWithDeps(t, filepath.Join(root, ".agents", ".tasks", "completed", "001.md"), "001", "Done Dep", "Completed", "-")
	writeTaskFileWithDeps(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Still Blocked Task", "Blocked", "001")
	// 003 is Pending with no deps — should not trigger the warning.
	writeTaskFileWithDeps(t, filepath.Join(root, ".agents", ".tasks", "active", "003.md"), "003", "Pending Dep", "Pending", "-")
	writeTaskFileWithDeps(t, filepath.Join(root, ".agents", ".tasks", "active", "004.md"), "004", "Legitimately Blocked", "Blocked", "003")

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	// doctor returns an error when validation has errors; warnings don't cause errors.
	_ = a.doctor()
	got := out.String()
	assertContainsAll(t, got,
		`"code": "task_blocked_deps_complete"`,
		`task 002 is Blocked but all its dependencies are Completed`,
	)
	// 004 should not appear in the blocked-deps-complete findings.
	assertNotContains(t, got, "004")
}

func TestDoctorReportsMalformedTaskEnums(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Bad Task", "Doing", "depends_on: []\n")

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.doctor(); !errors.Is(err, errValidationFailed) {
		t.Errorf("expected errValidationFailed, got: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		`"workflow_installed": true`,
		`"ok": false`,
		`"code": "task_malformed"`,
		`unsupported task status \"Doing\"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("doctor output missing %q:\n%s", want, got)
		}
	}
}

func TestDoctorReportsCompletedTaskAcceptanceFindings(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	writeCompletedTaskBody(t, root, "001", "Missing Acceptance", "## Summary\n\nDone.\n")
	writeCompletedTaskBody(t, root, "002", "Placeholder Acceptance", "## Acceptance Notes\n\n- [ ] TODO\n")
	writeCompletedTaskBody(t, root, "003", "Unchecked Acceptance", "## Acceptance Criteria\n\n* [ ] Verify it\n")

	// Generate indexes so doctor doesn't report missing-index errors.
	var indexOut strings.Builder
	indexer := app{opts: options{root: root}, out: &indexOut}
	if err := indexer.writeIndexes(); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.doctor(); err != nil {
		t.Error(err)
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
		t.Errorf("expected errValidationFailed, got: %v", err)
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

	// JSON mode: installed_version shows the binary version.
	var jOut strings.Builder
	a := app{opts: options{root: root, json: true}, out: &jOut}
	if err := a.status(); err != nil {
		t.Errorf("status error: %v", err)
	}
	jGot := jOut.String()
	assertContainsAll(t, jGot, `"installed": true`, `"installed_version": "dev"`)

	// Text mode: installed_version shows the binary version.
	var tOut strings.Builder
	a2 := app{opts: options{root: root}, out: &tOut}
	if err := a2.status(); err != nil {
		t.Errorf("status error: %v", err)
	}
	tGot := tOut.String()
	assertContainsAll(t, tGot, "installed: true", "installed_version: dev")
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
		t.Errorf("expected errValidationFailed, got: %v", err)
	}
	jGot := jOut.String()
	assertContainsAll(t, jGot, `"installed_version": null`)
	assertNotContains(t, jGot, `"installed_version": ""`)

	// Text mode: installed_version shows none.
	var tOut strings.Builder
	a2 := app{opts: options{root: root}, out: &tOut}
	if err := a2.doctor(); !errors.Is(err, errValidationFailed) {
		t.Errorf("expected errValidationFailed, got: %v", err)
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
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Completed In Active", "Completed", "depends_on: []\n")
	writeFile(t, filepath.Join(root, ".ahm", "research", "topics", "new-note.md"), "# New Note\n\nThis should make the research index stale.\n")
	if err := os.Remove(filepath.Join(root, ".ahm", "tasks", "cancelled", "index.md")); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.status(); !errors.Is(err, errValidationFailed) {
		t.Errorf("expected errValidationFailed, got: %v", err)
	}
	got := out.String()
	assertContainsAll(t, got,
		`"code": "task_bucket_mismatch"`,
		`completed task should be in .ahm/tasks/completed`,
		`"code": "generated_index_missing"`,
		`"path": ".ahm/tasks/cancelled/index.md"`,
		`"code": "generated_index_stale"`,
		`"path": ".ahm/research/index.md"`,
	)
}

func TestStatusReportsCompletedTaskReferencingActiveExecPlan(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".ahm", "tasks", "completed", "001.md"), "---\n"+
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
	writeFile(t, filepath.Join(root, ".ahm", "exec-plans", "active", "rollout.md"), "# Rollout\n\n## Outcomes & Retrospective\n\n")

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.status(); err != nil {
		t.Error(err)
	}
	assertContainsAll(t, out.String(),
		`"code": "task_completed_exec_plan_active"`,
		`completed task 001 references active ExecPlan .ahm/exec-plans/active/rollout.md`,
	)
}

func TestStatusReportsCompletedTaskReferencingIncompleteCompletedExecPlan(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".ahm", "tasks", "completed", "001.md"), "---\n"+
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
	writeFile(t, filepath.Join(root, ".ahm", "exec-plans", "completed", "rollout.md"), "# Rollout\n\n"+
		"## Progress\n\n- [x] Do it.\n\n"+
		"## Surprises & Discoveries\n\nNone.\n\n"+
		"## Decision Log\n\n- Chose this.\n\n"+
		"## Outcomes & Retrospective\n\n")

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.status(); err != nil {
		t.Error(err)
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
			validateExecPlans(root, workflowPathsFor(root), tt.tasks, &report)

			if tt.wantWarn != "" && !hasFinding(report.Warnings, tt.wantWarn) {
				t.Errorf("missing warning %q: %#v", tt.wantWarn, report.Warnings)
			}
			if tt.wantInfo != "" && !hasFinding(report.Info, tt.wantInfo) {
				t.Errorf("missing info %q: %#v", tt.wantInfo, report.Info)
			}
			if tt.wantNoWarn != "" && hasFinding(report.Warnings, tt.wantNoWarn) {
				t.Errorf("unexpected warning %q: %#v", tt.wantNoWarn, report.Warnings)
			}
		})
	}
}

func TestDoctorJSONReportsExecPlanInfoWithoutFailing(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	writeFile(t, filepath.Join(root, ".ahm", "exec-plans", "active", "orphan.md"), "# Orphan\n\n"+
		"## Progress\n\n- [ ] Do it.\n\n"+
		"## Surprises & Discoveries\n\nNone yet.\n\n"+
		"## Decision Log\n\n- Chose this.\n\n"+
		"## Outcomes & Retrospective\n\n")

	// Generate indexes so doctor doesn't report missing-index errors.
	var indexOut strings.Builder
	indexer := app{opts: options{root: root}, out: &indexOut}
	if err := indexer.writeIndexes(); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.doctor(); err != nil {
		t.Error(err)
	}
	got := out.String()
	assertContainsAll(t, got,
		`"ok": true`,
		`"info": [`,
		`"code": "exec_plan_orphan"`,
	)
}

func TestValidateADRsReportsFindings(t *testing.T) {
	root := t.TempDir()
	writeADRFile(t, root, "001-good-decision.md", "---\nstatus: accepted\ndate: 2026-06-01\n---\n# Good Decision\n\nBody.\n")
	writeADRFile(t, root, "002-invalid-status.md", "---\nstatus: doing\ndate: 2026-06-02\n---\n# Invalid Status\n\nBody.\n")
	writeADRFile(t, root, "003-missing-replacement.md", "---\nstatus: superseded by ADR-999\ndate: 2026-06-03\n---\n# Missing Replacement\n\nBody.\n")
	writeADRFile(t, root, "004-legacy-decision.md", "# ADR 004: Legacy Decision\n\n**Status:** Accepted\n**Date:** 2026-06-04\n\n## Context\n\nBody.\n")
	writeADRFile(t, root, "005-broken-front-matter.md", "---\nstatus: accepted\n# Missing close\n")
	writeADRFile(t, root, "006-id-mismatch.md", "---\nid: 007\nstatus: accepted\ndate: 2026-06-06\n---\n# ID Mismatch\n\nBody.\n")
	writeADRFile(t, root, "008-duplicate-a.md", "---\nstatus: accepted\ndate: 2026-06-08\n---\n# Duplicate A\n\nBody.\n")
	writeADRFile(t, root, "008-duplicate-b.md", "---\nstatus: accepted\ndate: 2026-06-08\n---\n# Duplicate B\n\nBody.\n")

	report := validationReport{OK: true, Errors: []validationFinding{}, Warnings: []validationFinding{}, Info: []validationFinding{}}
	validateADRs(root, &report)

	for _, code := range []string{
		"adr_invalid_status",
		"adr_supersede_missing",
		"adr_malformed",
		"adr_id_mismatch",
		"adr_duplicate_id",
	} {
		if !hasFinding(report.Errors, code) {
			t.Errorf("missing ADR error %q: %#v", code, report.Errors)
		}
	}
	// Verify duplicate ID error has an empty path (no single file blamed).
	for _, f := range report.Errors {
		if f.Code == "adr_duplicate_id" && f.Path != "" {
			t.Errorf("adr_duplicate_id should have empty path, got %q", f.Path)
		}
	}
	if !hasFinding(report.Warnings, "adr_legacy_format") {
		t.Errorf("missing adr_legacy_format warning: %#v", report.Warnings)
	}
}

func TestStatusAndDoctorReportLegacyADRsWithoutFailing(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	writeADRFile(t, root, "001-legacy-decision.md", "# ADR 001: Legacy Decision\n\n**Status:** Accepted\n**Date:** 2026-06-01\n\n## Context\n\nBody.\n")

	var statusOut strings.Builder
	a := app{opts: options{root: root, json: true}, out: &statusOut}
	if err := a.status(); err != nil {
		t.Errorf("status should not fail for legacy ADR warning: %v", err)
	}
	assertContainsAll(t, statusOut.String(),
		`"ok": true`,
		`"code": "adr_legacy_format"`,
		`run ahm adr migrate`,
	)

	var doctorOut strings.Builder
	a2 := app{opts: options{root: root, json: true}, out: &doctorOut}
	if err := a2.doctor(); err != nil {
		t.Errorf("doctor should not fail for legacy ADR warning: %v", err)
	}
	assertContainsAll(t, doctorOut.String(),
		`"ok": true`,
		`"code": "adr_legacy_format"`,
	)
}

func TestStatusReportsADRErrors(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	writeADRFile(t, root, "001-invalid-status.md", "---\nstatus: doing\ndate: 2026-06-01\n---\n# Invalid Status\n\nBody.\n")

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.status(); !errors.Is(err, errValidationFailed) {
		t.Errorf("expected errValidationFailed, got: %v", err)
	}
	assertContainsAll(t, out.String(),
		`"ok": false`,
		`"code": "adr_invalid_status"`,
		`unsupported ADR status \"doing\"`,
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
		t.Error(err)
	}
	got := out.String()
	assertContainsAll(t, got,
		`"code": "markdown_link_missing"`,
		`"path": ".agents/.research/topics/links.md:3"`,
		`relative Markdown link target does not exist: missing.md`,
	)
	assertNotContains(t, got, "also-missing.md")
}

func TestWalkMarkdownLinks(t *testing.T) {
	data := []byte("[first](one.md)\n" +
		"`[inline](ignored-inline.md)` [second](two.md)\n" +
		"```md\n[fenced](ignored-backtick.md)\n```\n" +
		"~~~md\n[fenced](ignored-tilde.md)\n~~~\n" +
		"![image](image.png)\n")

	type link struct {
		lineNo int
		target string
	}
	var got []link
	walkMarkdownLinks(data, func(lineNo int, target string) {
		got = append(got, link{lineNo: lineNo, target: target})
	})

	want := []link{
		{lineNo: 1, target: "one.md"},
		{lineNo: 2, target: "two.md"},
		{lineNo: 9, target: "image.png"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("walkMarkdownLinks() = %#v, want %#v", got, want)
	}
}

func TestStatusReportsMarkdownLinksInWorkflowFilesWithCodeSpans(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	// Quoted example links inside inline code spans and fenced code blocks must
	// not be treated as navigation, but a real broken link on the same line
	// (outside any backticks) must still be reported.
	writeFile(t, filepath.Join(root, ".agents", ".research", "topics", "links.md"),
		"# Links\n\n"+
			"Span: `[ADRs](adr/index.md)` and span2: `[broken](also-missing.md)`.\n\n"+
			"```md\n[fenced](fenced-missing.md)\n```\n\n"+
			"[real](real-missing.md)\n")

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.doctor(); err != nil {
		t.Error(err)
	}
	got := out.String()
	assertContainsAll(t, got,
		`"code": "markdown_link_missing"`,
		`relative Markdown link target does not exist: real-missing.md`,
	)
	assertNotContains(t, got, "adr/index.md")
	assertNotContains(t, got, "also-missing.md")
	assertNotContains(t, got, "fenced-missing.md")
}

func TestValidateProjectDocsIgnoresCodeSpansAndFences(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(root, "docs", "guide.md"), "# Guide\n")
	writeFile(t, filepath.Join(root, "README.md"),
		"# Project\n\n"+
			"See `[ADRs](adr/README.md)` and `[x](missing.md)`.\n\n"+
			"```md\n[fenced](fenced-missing.md)\n```\n\n"+
			"But [real](docs/nope.md) is a real link.\n")

	report, _ := validateWorkflowScoped(root, []string{CheckScopeProjectDocs})
	var targets []string
	for _, w := range report.Warnings {
		if w.Code == "project_doc_link_missing" {
			targets = append(targets, w.Message)
		}
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 project_doc_link_missing for docs/nope.md, got %d: %#v", len(targets), report.Warnings)
	}
	if !strings.Contains(targets[0], "docs/nope.md") {
		t.Errorf("expected finding for docs/nope.md, got %q", targets[0])
	}
	for _, msg := range targets {
		for _, quoted := range []string{"adr/README.md", "missing.md", "fenced-missing.md"} {
			if strings.Contains(msg, quoted) {
				t.Errorf("quoted link inside code span was flagged: %q", msg)
			}
		}
	}
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
	writeFile(t, filepath.Join(root, ".ahm", "tasks", "completed", id+".md"), "---\n"+
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
	setupAhmRepo(t, root)
	// Add a broken link that would trigger markdown_link_missing.
	writeFile(t, filepath.Join(root, ".ahm", "research", "topics", "links.md"), "# Links\n\n[missing](missing.md)\n")
	// Add a workflow issue.
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Bad Task", "Doing", "depends_on: -\n")

	// Only workflow checks.
	report, _ := validateWorkflowScoped(root, []string{CheckScopeWorkflow})
	// Should find task_malformed (workflow check) but NOT markdown_link_missing.
	foundTaskMalformed := false
	for _, e := range report.Errors {
		if e.Code == "task_malformed" {
			foundTaskMalformed = true
		}
	}
	if !foundTaskMalformed {
		t.Error("expected task_malformed in workflow-only scope")
	}
	for _, e := range report.Errors {
		if e.Code == "markdown_link_missing" {
			t.Error("unexpected markdown_link_missing in workflow-only scope")
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
	// Create a workflow issue.
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Bad Task", "Doing", "depends_on: -\n")

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
		t.Error("expected markdown_link_missing in links-only scope")
	}
	// Should NOT find task_malformed (workflow check).
	for _, e := range report.Errors {
		if e.Code == "task_malformed" {
			t.Error("unexpected task_malformed in links-only scope")
		}
	}
	// No workflow errors since we only ran link checks.
	if !report.OK {
		t.Error("expected OK for links-only scope, got errors")
	}
}

func TestValidateWorkflowScopedProjectDocsNoDocs(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	// A fresh install has no project docs; the project-docs scope should
	// produce no findings.
	report, tasks := validateWorkflowScoped(root, []string{CheckScopeProjectDocs})
	if !report.OK {
		t.Error("expected OK for project-docs scope, got errors")
	}
	if len(report.Errors)+len(report.Warnings)+len(report.Info) > 0 {
		t.Errorf("unexpected findings for project-docs scope: %#v", report)
	}
	if len(tasks) != 0 {
		t.Errorf("expected no tasks for project-docs scope, got %d", len(tasks))
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
		t.Error("expected OK for valid project docs, got errors")
	}
	for _, w := range report.Warnings {
		if w.Code == "project_doc_link_missing" {
			t.Errorf("unexpected project_doc_link_missing for valid docs: %#v", w)
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
		t.Errorf("expected 2 project_doc_link_missing findings, got %d: %#v", len(found), report.Warnings)
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
			t.Errorf("project-docs check ran by default; found %#v", w)
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
			t.Errorf("unexpected design_doc_unindexed without design docs: %#v", w)
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
			t.Errorf("unexpected design_doc_unindexed without index.md: %#v", w)
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
			t.Errorf("unexpected finding for valid design docs: %#v", w)
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
		t.Errorf("expected 1 design_doc_unindexed for orphan.md, got %#v", found)
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
			t.Errorf("design_doc_unindexed should not fire for missing index entry: %#v", w)
		}
	}
	if !foundLink {
		t.Error("expected project_doc_link_missing for missing design-doc index entry")
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
			t.Errorf("design-doc check ran by default; found %#v", w)
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
		t.Error("expected markdown_link_missing when running all checks")
	}
	// validateWorkflow should produce the same result.
	report2, _ := validateWorkflow(root)
	if report.OK != report2.OK {
		t.Error("validateWorkflowScoped(nil) should match validateWorkflow")
	}
	if len(report.Errors) != len(report2.Errors) {
		t.Errorf("error count mismatch: %d vs %d", len(report.Errors), len(report2.Errors))
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
		t.Errorf("expected exit code 2 for invalid check scope, got %d; stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "unknown check scope") {
		t.Errorf("expected unknown check scope error, got: %s", stderr)
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
		t.Errorf("expected exit code 0, got %d; stderr=%s, stdout=%s", code, stderr, stdout)
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
		t.Error("expected at least one error, got none")
	}
	found := false
	for _, e := range report.Errors {
		if e.Code == "task_malformed" && strings.Contains(e.Message, "unsupported block scalar") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected task_malformed error with block scalar message, got: %v", report.Errors)
	}
	// Verify no missing-field errors (which would be misleading)
	for _, e := range report.Errors {
		if e.Code == "task_missing_field" {
			t.Errorf("unexpected missing_field error when front matter is malformed: %v", e)
		}
	}
}

func TestValidateReportsCorruptMetadata(t *testing.T) {
	root := t.TempDir()
	// Init first to create valid workflow.
	setupAhmRepo(t, root)

	// Corrupt the metadata file.
	metaPath := filepath.Join(root, ".ahm", "config.json")
	if err := os.WriteFile(metaPath, []byte("{invalid json}"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, _ := validateWorkflow(root)
	foundCorrupt := false
	for _, err := range report.Errors {
		if err.Code == "metadata_corrupt" {
			foundCorrupt = true
			break
		}
	}
	if !foundCorrupt {
		t.Errorf("expected metadata_corrupt error, got: %v", report.Errors)
	}
	// Should not produce metadata_missing (which is only for absent file).
	for _, err := range report.Errors {
		if err.Code == "metadata_missing" {
			t.Errorf("unexpected metadata_missing error for corrupt file: %v", err)
		}
	}
}

func TestValidateReportsCorruptAhmConfig(t *testing.T) {
	root := t.TempDir()
	// Keep a valid legacy metadata file present; .ahm/config.json should be
	// preferred and reported as the corrupt source.
	if err := writeMetadata(root, metadata{Version: "0.1.0", Files: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".ahm", "config.json"), "{invalid json}")

	report, _ := validateWorkflow(root)
	foundCorrupt := false
	for _, err := range report.Errors {
		if err.Code == "metadata_corrupt" && err.Path == ".ahm/config.json" {
			foundCorrupt = true
			break
		}
	}
	if !foundCorrupt {
		t.Errorf("expected metadata_corrupt error for .ahm/config.json, got: %v", report.Errors)
	}
	for _, err := range report.Errors {
		if err.Code == "metadata_corrupt" && err.Path == ".agents/ahm.json" {
			t.Errorf("unexpected legacy metadata path for corrupt .ahm/config.json: %v", err)
		}
	}
}

func TestValidateReportsMissingMetadata(t *testing.T) {
	root := t.TempDir()
	// No init, no metadata at all.
	report, _ := validateWorkflow(root)
	foundMissing := false
	for _, err := range report.Errors {
		if err.Code == "metadata_missing" {
			foundMissing = true
			break
		}
	}
	if !foundMissing {
		t.Errorf("expected metadata_missing error, got: %v", report.Errors)
	}
	// Should not produce metadata_corrupt.
	for _, err := range report.Errors {
		if err.Code == "metadata_corrupt" {
			t.Errorf("unexpected metadata_corrupt error for missing file: %v", err)
		}
	}
}

func TestPostMutation_TaskCompleteReferencesActiveExecPlan(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)

	// Create a task with exec_plan referencing an active plan.
	writeFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"),
		"---\n"+
			"id: 001\n"+
			"title: Needs ExecPlan Move\n"+
			"status: Pending\n"+
			"priority: P2\n"+
			"effort: S\n"+
			"labels: type:task\n"+
			"exec_plan: rollout\n"+
			"depends_on: -\n"+
			"---\n"+
			"# Needs ExecPlan Move\n\n"+
			"## Summary\n\nDone.\n"+
			"## Acceptance Notes\n\n- [x] All done.\n")

	// Create an active ExecPlan that the task references.
	writeFile(t, filepath.Join(root, ".ahm", "exec-plans", "active", "rollout.md"),
		"# Rollout\n\n## Outcomes & Retrospective\n\n")

	stdout, stderr, code := runCLI(t, "--root", root, "task", "complete", "001")
	if code != 0 {
		t.Errorf("task complete exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	// Verify the warning appears on stderr.
	assertContainsAll(t, stderr,
		"completed task 001 references active ExecPlan",
	)
}

func TestPostMutation_IndexDetectsExecPlanDrift(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)

	// Create a completed task that still references an active ExecPlan.
	writeFile(t, filepath.Join(root, ".ahm", "tasks", "completed", "001.md"),
		"---\n"+
			"id: 001\n"+
			"title: Done But Plan Active\n"+
			"status: Completed\n"+
			"priority: P2\n"+
			"effort: S\n"+
			"labels: type:task\n"+
			"exec_plan: rollout\n"+
			"depends_on: -\n"+
			"---\n"+
			"# Done But Plan Active\n\n"+
			"## Summary\n\nDone.\n")

	// Create an active ExecPlan.
	writeFile(t, filepath.Join(root, ".ahm", "exec-plans", "active", "rollout.md"),
		"# Rollout\n\n## Outcomes & Retrospective\n\n")

	stdout, stderr, code := runCLI(t, "--root", root, "index")
	if code != 0 {
		t.Errorf("index exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	// Verify the warning appears on stderr.
	assertContainsAll(t, stderr,
		"completed task 001 references active ExecPlan",
	)
}

func TestPostMutation_ScopeIsWorkflowOnly(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)

	// Create a completed task referencing an active ExecPlan (workflow finding).
	writeFile(t, filepath.Join(root, ".ahm", "tasks", "completed", "001.md"),
		"---\n"+
			"id: 001\n"+
			"title: Done But Plan Active\n"+
			"status: Completed\n"+
			"priority: P2\n"+
			"effort: S\n"+
			"labels: type:task\n"+
			"exec_plan: rollout\n"+
			"depends_on: -\n"+
			"---\n"+
			"# Done But Plan Active\n\n"+
			"## Summary\n\nDone.\n")

	// Create an active ExecPlan.
	writeFile(t, filepath.Join(root, ".ahm", "exec-plans", "active", "rollout.md"),
		"# Rollout\n\n## Outcomes & Retrospective\n\n")

	// Create a broken markdown link that would trigger markdown_link_missing.
	writeFile(t, filepath.Join(root, ".ahm", "research", "topics", "links.md"),
		"# Links\n\n[missing](missing.md)\n")

	stdout, stderr, code := runCLI(t, "--root", root, "index")
	if code != 0 {
		t.Errorf("index exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	// Verify the workflow finding appears.
	assertContainsAll(t, stderr,
		"completed task 001 references active ExecPlan",
	)
	// Verify the markdown_link_missing finding does NOT appear.
	assertNotContains(t, stderr,
		"markdown_link_missing",
	)
}

func TestPostMutation_DryRunSkipsValidation(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)

	// Create a completed task that still references an active ExecPlan.
	writeFile(t, filepath.Join(root, ".ahm", "tasks", "completed", "001.md"),
		"---\n"+
			"id: 001\n"+
			"title: Done But Plan Active\n"+
			"status: Completed\n"+
			"priority: P2\n"+
			"effort: S\n"+
			"labels: type:task\n"+
			"exec_plan: rollout\n"+
			"depends_on: -\n"+
			"---\n"+
			"# Done But Plan Active\n\n"+
			"## Summary\n\nDone.\n")

	// Create an active ExecPlan.
	writeFile(t, filepath.Join(root, ".ahm", "exec-plans", "active", "rollout.md"),
		"# Rollout\n\n## Outcomes & Retrospective\n\n")

	stdout, stderr, code := runCLI(t, "--dry-run", "--root", root, "index")
	if code != 0 {
		t.Errorf("index exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	// The validation should not run during dry-run, so no warnings.
	if strings.Contains(stderr, "completed task 001") {
		t.Errorf("dry-run index emitted unexpected warning on stderr:\n%s", stderr)
	}
}

// --- New checks added by task 160b ---

func TestValidateProjectDocLinkPortability(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	// Non-portable link targets should be caught.
	writeFile(t, filepath.Join(root, "README.md"),
		"# Project\n\n"+
			"- [file link](file:///Users/me/doc.md)\n"+
			"- [home link](~/docs/guide.md)\n"+
			"- [abs link](/etc/passwd)\n"+
			"- [good link](docs/guide.md)\n")
	writeFile(t, filepath.Join(root, "docs", "guide.md"), "# Guide\n")

	report, _ := validateWorkflowScoped(root, []string{CheckScopeProjectDocs})

	// Should have 3 non-portable errors.
	var notPortable int
	for _, e := range report.Errors {
		if e.Code == "project_doc_link_not_portable" {
			notPortable++
		}
	}
	if notPortable != 3 {
		t.Errorf("expected 3 project_doc_link_not_portable errors, got %d: %#v", notPortable, report.Errors)
	}

	// The good link (docs/guide.md) should not produce link-missing warnings.
	// Non-portable links like ~/docs/guide.md may fire both portability and
	// missing-link findings; that's correct.
	foundGoodLinkMissing := false
	for _, w := range report.Warnings {
		if w.Code == "project_doc_link_missing" && strings.Contains(w.Message, "docs/guide.md") && !strings.Contains(w.Message, "~/") {
			foundGoodLinkMissing = true
		}
	}
	if foundGoodLinkMissing {
		t.Errorf("the good link (docs/guide.md) should not be missing: %#v", report.Warnings)
	}
}

func TestValidateProjectDocLinkPortabilityIgnoresCodeSpansAndFences(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	// Non-portable links inside code spans and fenced blocks must be ignored.
	writeFile(t, filepath.Join(root, "README.md"),
		"# Project\n\n"+
			"See `[example](file:///nope.md)`.\n\n"+
			"```md\n[fenced](~/fenced.md)\n```\n\n"+
			"Real [bad](file:///real.md) though.\n")

	report, _ := validateWorkflowScoped(root, []string{CheckScopeProjectDocs})

	var notPortable int
	for _, e := range report.Errors {
		if e.Code == "project_doc_link_not_portable" {
			notPortable++
		}
	}
	if notPortable != 1 {
		t.Errorf("expected 1 project_doc_link_not_portable (only real.md), got %d: %#v", notPortable, report.Errors)
	}
}

func TestValidateEntryPointBudget(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	// AGENTS.md with many lines (over default budget of 150).
	var lines []string
	for i := 0; i < 200; i++ {
		lines = append(lines, fmt.Sprintf("Line %d", i))
	}
	writeFile(t, filepath.Join(root, "AGENTS.md"), strings.Join(lines, "\n")+"\n")

	report, _ := validateWorkflowScoped(root, []string{CheckScopeProjectDocs})

	found := false
	for _, w := range report.Warnings {
		if w.Code == "entry_point_over_budget" {
			found = true
			if !strings.Contains(w.Message, "budget 150") {
				t.Errorf("expected default budget 150, got: %s", w.Message)
			}
		}
	}
	if !found {
		t.Error("expected entry_point_over_budget warning")
	}
}

func TestValidateEntryPointBudgetUnderBudget(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	// AGENTS.md within budget.
	writeFile(t, filepath.Join(root, "AGENTS.md"), "# AGENTS.md\n\nShort file.\n")

	report, _ := validateWorkflowScoped(root, []string{CheckScopeProjectDocs})

	for _, w := range report.Warnings {
		if w.Code == "entry_point_over_budget" {
			t.Errorf("unexpected entry_point_over_budget: %#v", w)
		}
	}
}

func TestValidateEntryPointBudgetConfig(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	// Set a custom budget of 10 lines.
	meta, _ := readMetadata(root)
	meta.ProjectDocs = &projectDocsConfig{EntryPointBudget: 10}
	if err := writeMetadata(root, meta); err != nil {
		t.Fatal(err)
	}

	// 15-line AGENTS.md.
	var lines []string
	for i := 0; i < 15; i++ {
		lines = append(lines, fmt.Sprintf("Line %d", i))
	}
	writeFile(t, filepath.Join(root, "AGENTS.md"), strings.Join(lines, "\n")+"\n")

	report, _ := validateWorkflowScoped(root, []string{CheckScopeProjectDocs})

	found := false
	for _, w := range report.Warnings {
		if w.Code == "entry_point_over_budget" {
			found = true
			if !strings.Contains(w.Message, "budget 10") {
				t.Errorf("expected custom budget 10, got: %s", w.Message)
			}
		}
	}
	if !found {
		t.Error("expected entry_point_over_budget with custom budget")
	}
}

func TestValidateDocIndexCoverage(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	// docs/guardrails/ with an index.md but an orphan file.
	writeFile(t, filepath.Join(root, "docs", "guardrails", "a.md"), "# A\n")
	writeFile(t, filepath.Join(root, "docs", "guardrails", "orphan.md"), "# Orphan\n")
	writeFile(t, filepath.Join(root, "docs", "guardrails", "index.md"),
		"# Guardrails Index\n\n- [A](a.md)\n")
	writeFile(t, filepath.Join(root, "docs", "adr", "README.md"), "# Preserved ADR Guide\n")

	report, _ := validateWorkflowScoped(root, []string{CheckScopeProjectDocs})

	foundUnindexed := false
	for _, w := range report.Warnings {
		if w.Code == "doc_unindexed" {
			if strings.Contains(w.Path, "docs/adr/README.md") {
				t.Errorf("preserved ADR scaffold should not produce doc_unindexed: %#v", w)
			}
			foundUnindexed = true
			if !strings.Contains(w.Path, "orphan.md") {
				t.Errorf("expected orphan.md in doc_unindexed, got path: %s", w.Path)
			}
		}
	}
	if !foundUnindexed {
		t.Error("expected doc_unindexed warning for orphan.md")
	}

	// The design-docs check should NOT be duplicated by doc_unindexed.
	designDir := filepath.Join(root, "docs", "design-docs")
	writeFile(t, filepath.Join(designDir, "orphan.md"), "# Orphan\n")
	writeFile(t, filepath.Join(designDir, "index.md"), "# Design Docs\n")

	report2, _ := validateWorkflowScoped(root, []string{CheckScopeProjectDocs})
	for _, w := range report2.Warnings {
		if w.Code == "doc_unindexed" && strings.Contains(w.Path, "design-docs") {
			t.Errorf("design-docs should not produce doc_unindexed: %#v", w)
		}
	}
}

func TestValidateDocIndexCoverageNoIndex(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	// A docs/ subdirectory without index.md should not trigger checks.
	writeFile(t, filepath.Join(root, "docs", "random", "file.md"), "# File\n")

	report, _ := validateWorkflowScoped(root, []string{CheckScopeProjectDocs})

	for _, w := range report.Warnings {
		if w.Code == "doc_unindexed" {
			t.Errorf("unexpected doc_unindexed for directory with no index: %#v", w)
		}
	}
}

func TestProjectDocFilesIncludesAgentsAndClaude(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "AGENTS.md"), "# Agents\n")
	writeFile(t, filepath.Join(root, "CLAUDE.md"), "# Claude\n")
	writeFile(t, filepath.Join(root, "nested", "AGENTS.md"), "# Nested Agents\n")

	files := projectDocFiles(root)

	hasAgents := false
	hasClaude := false
	hasNested := false
	for _, f := range files {
		if filepath.Base(f) == "AGENTS.md" && filepath.Dir(f) == root {
			hasAgents = true
		}
		if filepath.Base(f) == "CLAUDE.md" {
			hasClaude = true
		}
		if filepath.Base(f) == "AGENTS.md" && filepath.Dir(f) != root {
			hasNested = true
		}
	}
	if !hasAgents {
		t.Error("root AGENTS.md not in projectDocFiles")
	}
	if !hasClaude {
		t.Error("CLAUDE.md not in projectDocFiles")
	}
	if !hasNested {
		t.Error("nested AGENTS.md not in projectDocFiles")
	}
}

// --- ahm docs check command tests ---

func TestDocsCheckCommandText(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "--root", root, "docs", "check")
	if code != 0 {
		t.Errorf("exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "ok") {
		t.Errorf("expected 'ok' in output, got: %s", stdout)
	}
	if stderr != "" {
		t.Errorf("unexpected stderr: %s", stderr)
	}
}

func TestDocsCheckCommandJSON(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	stdout, _, code := runCLI(t, "--root", root, "--json", "docs", "check")
	if code != 0 {
		t.Errorf("exit code = %d, stdout = %s", code, stdout)
	}
	if !strings.Contains(stdout, `"ok"`) {
		t.Errorf("expected JSON with ok field, got: %s", stdout)
	}
	if !strings.Contains(stdout, `"errors"`) {
		t.Errorf("expected JSON with errors field, got: %s", stdout)
	}
}

func TestDocsCheckCommandPlain(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	stdout, _, code := runCLI(t, "--root", root, "--plain", "docs", "check")
	if code != 0 {
		t.Errorf("exit code = %d, stdout = %s", code, stdout)
	}
	if !strings.Contains(stdout, `"ok"`) {
		t.Errorf("expected compact JSON with ok field, got: %s", stdout)
	}
}

func TestDocsCheckCommandErrors(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	// Create a non-portable link that should be an error.
	writeFile(t, filepath.Join(root, "README.md"), "# Readme\n\n[bad](file:///etc/hosts)\n")

	_, _, code := runCLI(t, "--root", root, "docs", "check")
	if code != 1 {
		t.Errorf("expected exit code 1 for errors, got %d", code)
	}
}

func TestDocsCheckCommandStrict(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	// Over-budget AGENTS.md produces a warning normally, but --strict
	// promotes it to an error.
	var lines []string
	for i := 0; i < 200; i++ {
		lines = append(lines, fmt.Sprintf("Line %d", i))
	}
	writeFile(t, filepath.Join(root, "AGENTS.md"), strings.Join(lines, "\n")+"\n")

	// Without --strict, warnings pass.
	_, _, code := runCLI(t, "--root", root, "docs", "check")
	if code != 0 {
		t.Errorf("expected exit 0 for warnings-only, got %d", code)
	}

	// With --strict, warnings become errors.
	_, _, code = runCLI(t, "--root", root, "docs", "check", "--strict")
	if code != 1 {
		t.Errorf("expected exit 1 with --strict, got %d", code)
	}
}

func TestDeprecationWarningForProjectDocsScope(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	// --check project-docs on status should emit deprecation warning.
	stdout, stderr, code := runCLI(t, "--root", root, "status", "--check", "project-docs")
	if code != 0 {
		t.Errorf("exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "--check project-docs is deprecated") {
		t.Errorf("expected deprecation warning on stderr, got: %s", stderr)
	}

	// --check project-docs on doctor should emit deprecation warning.
	stdout, stderr, code = runCLI(t, "--root", root, "doctor", "--check", "project-docs")
	if code != 0 {
		t.Errorf("exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "--check project-docs is deprecated") {
		t.Errorf("expected deprecation warning on stderr, got: %s", stderr)
	}

	// Regular status/doctor without --check project-docs should NOT emit warning.
	stdout, stderr, code = runCLI(t, "--root", root, "status")
	if code != 0 {
		t.Errorf("exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	if strings.Contains(stderr, "--check project-docs is deprecated") {
		t.Errorf("unexpected deprecation warning in normal status: %s", stderr)
	}
}

func TestDeprecatedProjectDocsScopeStillFunctions(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	// Create a broken link that --check project-docs should catch.
	writeFile(t, filepath.Join(root, "README.md"), "# Readme\n\n[missing](docs/nope.md)\n")

	stdout, stderr, code := runCLI(t, "--root", root, "status", "--check", "project-docs")
	// Deprecation warning on stderr.
	if !strings.Contains(stderr, "deprecated") {
		t.Errorf("expected deprecation warning, got stderr: %s", stderr)
	}
	// The finding should still be reported.
	if !strings.Contains(stdout, "project_doc_link_missing") {
		t.Errorf("expected project_doc_link_missing finding, got stdout: %s", stdout)
	}
	if code != 0 {
		t.Errorf("exit code = %d (warnings-only should exit 0), stdout = %s", code, stdout)
	}
}

func TestExecPlanSectionHasOpenProgress(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  bool
	}{
		{
			name:  "unindented dash unchecked",
			lines: []string{"- [ ] Do it"},
			want:  true,
		},
		{
			name:  "unindented asterisk unchecked",
			lines: []string{"* [ ] Do it"},
			want:  true,
		},
		{
			name:  "indented dash unchecked",
			lines: []string{"  - [ ] Do it"},
			want:  true,
		},
		{
			name:  "indented asterisk unchecked",
			lines: []string{"  * [ ] Do it"},
			want:  true,
		},
		{
			name:  "tab indented asterisk unchecked",
			lines: []string{"\t* [ ] Do it"},
			want:  true,
		},
		{
			name:  "dash checked",
			lines: []string{"- [x] Done"},
			want:  false,
		},
		{
			name:  "asterisk checked",
			lines: []string{"* [x] Done"},
			want:  false,
		},
		{
			name:  "plain text",
			lines: []string{"just a line", "- not a checkbox"},
			want:  false,
		},
		{
			name:  "empty section",
			lines: []string{},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			section := execPlanSection{Lines: tt.lines}
			if got := execPlanSectionHasOpenProgress(section); got != tt.want {
				t.Errorf("execPlanSectionHasOpenProgress() = %v, want %v", got, tt.want)
			}
		})
	}
}
