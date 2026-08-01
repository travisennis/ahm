package ahm

import (
	"errors"
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

	// Tracking task with all children resolved: the tracking-children warning
	// must agree between the standalone and reused validation paths.
	writeFile(t, paths.taskFile("active", "302"), `---
id: 302
title: Tracker done
status: Tracking
priority: P1
effort: M
labels: type:task
exec_plan: -
depends_on: -
---
# Tracker done
`)
	writeFile(t, paths.taskFile("completed", "302a"), `---
id: 302a
title: Child done
status: Completed
priority: P1
effort: S
labels: type:task
exec_plan: -
depends_on: -
parent: 302
---
# Child done
`)
	// Tracking task with a still-open child: no warning in either path.
	writeFile(t, paths.taskFile("active", "303"), `---
id: 303
title: Tracker open
status: Tracking
priority: P1
effort: M
labels: type:task
exec_plan: -
depends_on: -
---
# Tracker open
`)
	writeFile(t, paths.taskFile("active", "303a"), `---
id: 303a
title: Child open
status: Pending
priority: P1
effort: S
labels: type:task
exec_plan: -
depends_on: -
parent: 303
---
# Child open
`)

	tasks, err := collectTasksForPaths(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	writes, err := indexWritesForPaths(root, tasks, paths, nil)
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
		reused := validateWorkflowStateForPaths(root, paths, tasks, writes, nil)
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

func TestValidationReportsTrackingChildrenComplete(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	// 001 is Tracking with all children Completed or Cancelled — should warn.
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Tracker Done", "Tracking", "depends_on: -\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "completed", "001a.md"), "001a", "Child A", "Completed", "depends_on: -\nparent: 001\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "cancelled", "001b.md"), "001b", "Child B", "Cancelled", "depends_on: -\nparent: 001\n")
	// 002 is Tracking with a child still open — no warning.
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "002", "Tracker Open", "Tracking", "depends_on: -\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "002a.md"), "002a", "Child Open", "Pending", "depends_on: -\nparent: 002\n")
	// 003 is Tracking with no children — no warning.
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "003.md"), "003", "Empty Tracker", "Tracking", "depends_on: -\n")
	// 004 is Tracking with all children resolved but its own dependency is
	// still open — no warning until the tracker's dependencies are satisfied.
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "004.md"), "004", "Tracker Waiting", "Tracking", "depends_on: 005\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "completed", "004a.md"), "004a", "Child Done", "Completed", "depends_on: -\nparent: 004\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "005.md"), "005", "Open Dep", "Pending", "depends_on: -\n")
	// 006 is Tracking with all children resolved but depends on a Cancelled
	// task — its own dependency is unsatisfiable, so no tracking warning
	// (task_dependency_cancelled already covers the dep itself).
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "006.md"), "006", "Tracker Cancelled Dep", "Tracking", "depends_on: 007\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "completed", "006a.md"), "006a", "Child Done", "Completed", "depends_on: -\nparent: 006\n")
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "cancelled", "007.md"), "007", "Cancelled Dep", "Cancelled", "depends_on: -\n")

	// Generate indexes so doctor doesn't report missing-index errors.
	var indexOut strings.Builder
	indexer := app{opts: options{root: root}, out: &indexOut}
	if err := indexer.writeIndexes(); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.doctor(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	assertContainsAll(t, got,
		`"code": "task_tracking_children_complete"`,
		`task 001 is Tracking but all its child tasks are Completed or Cancelled`,
	)
	assertNotContains(t, got, "task 002 is Tracking", "task 003 is Tracking", "task 004 is Tracking", "task 006 is Tracking")
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

func TestValidateTaskDuplicateIDsReportsError(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	paths := workflowPathsFor(root)

	// Write two task files with the same ID in different buckets.
	writeTaskFile(t, paths.taskFile("active", "042"), "042", "Duplicate Task A", "Pending", "")
	writeTaskFile(t, paths.taskFile("completed", "042"), "042", "Duplicate Task B", "Completed", "depends_on: -\n")

	report, tasks := validateWorkflowScopedForPaths(root, []string{CheckScopeWorkflow}, paths)

	if !hasFinding(report.Errors, "task_duplicate_id") {
		t.Fatalf("expected task_duplicate_id error, got errors: %#v", report.Errors)
	}
	// Verify the error message contains both file paths.
	for _, f := range report.Errors {
		if f.Code == "task_duplicate_id" {
			if f.Path != "" {
				t.Errorf("task_duplicate_id should have empty path, got %q", f.Path)
			}
			if !strings.Contains(f.Message, "042") || !strings.Contains(f.Message, "active/042.md") || !strings.Contains(f.Message, "completed/042.md") {
				t.Errorf("task_duplicate_id message missing expected paths: %q", f.Message)
			}
		}
	}

	// Verify the tasks list still includes both (validation is read-only, no filtering).
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks in list, got %d", len(tasks))
	}
}

func TestValidateTaskDuplicateIDsReportsErrorInReusedState(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	paths := workflowPathsFor(root)

	writeTaskFile(t, paths.taskFile("active", "042"), "042", "Duplicate Task A", "Pending", "")
	writeTaskFile(t, paths.taskFile("completed", "042"), "042", "Duplicate Task B", "Completed", "depends_on: -\n")

	tasks, err := collectTasksForPaths(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	writes, err := indexWritesForPaths(root, tasks, paths, nil)
	if err != nil {
		t.Fatal(err)
	}

	report := validateWorkflowStateForPaths(root, paths, tasks, writes, nil)
	if !hasFinding(report.Errors, "task_duplicate_id") {
		t.Fatalf("expected task_duplicate_id error in reused state, got errors: %#v", report.Errors)
	}
}

func TestValidateGeneratedIndexesContinuesAfterPartialADRParseError(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	indexer := app{opts: options{root: root}, out: &strings.Builder{}}
	if err := indexer.writeIndexes(); err != nil {
		t.Fatal(err)
	}
	writeADRFile(t, root, "001-valid.md", "---\nstatus: accepted\ndate: 2026-07-01\n---\n# Valid ADR\n\nBody.\n")
	writeADRFile(t, root, "002-bad.md", "---\nkey: >\n---\n# Bad\n")

	report, _ := validateWorkflowScopedForPaths(root, []string{CheckScopeWorkflow}, workflowPathsFor(root))
	if !hasFinding(report.Errors, "adr_malformed") {
		t.Errorf("missing adr_malformed error: %#v", report.Errors)
	}
	if !hasFinding(report.Warnings, "generated_index_stale") {
		t.Errorf("missing generated_index_stale warning: %#v", report.Warnings)
	}
	if hasFinding(report.Warnings, "generated_index_check_failed") {
		t.Errorf("unexpected generated_index_check_failed warning: %#v", report.Warnings)
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
	paths := workflowPathsFor(root)
	linkPath := filepath.Join(root, filepath.FromSlash(paths.researchRel()), "topics", "links.md")
	writeFile(t, linkPath, "# Links\n\n[missing](missing.md)\n\n```md\n[ignored](also-missing.md)\n```\n")

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.doctor(); err != nil {
		t.Error(err)
	}
	got := out.String()
	assertContainsAll(t, got,
		`"code": "markdown_link_missing"`,
		`"path": "`+relPath(root, linkPath)+`:3"`,
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
	paths := workflowPathsFor(root)
	// Quoted example links inside inline code spans and fenced code blocks must
	// not be treated as navigation, but a real broken link on the same line
	// (outside any backticks) must still be reported.
	writeFile(t, filepath.Join(root, filepath.FromSlash(paths.researchRel()), "topics", "links.md"),
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

func TestValidateManagedRecordLinksByFamilyAndLayout(t *testing.T) {
	layouts := []struct {
		name  string
		setup func(*testing.T, string)
	}{
		{name: "current", setup: setupAhmRepo},
		{name: "legacy", setup: initAndCreateLegacyMetadata},
	}
	families := []struct {
		name       string
		dir        func(string, workflowPaths) string
		sourceName string
		targetName string
	}{
		{
			name: "tasks",
			dir: func(root string, paths workflowPaths) string {
				return filepath.Join(root, filepath.FromSlash(paths.tasksRel()), "active")
			},
			sourceName: "001.md",
			targetName: "002.md",
		},
		{
			name: "research",
			dir: func(root string, paths workflowPaths) string {
				return filepath.Join(root, filepath.FromSlash(paths.researchRel()), "topics")
			},
			sourceName: "links.md",
			targetName: "target.md",
		},
		{
			name: "exec-plans",
			dir: func(root string, paths workflowPaths) string {
				return paths.execPlansDir("active")
			},
			sourceName: "links.md",
			targetName: "target.md",
		},
		{
			name: "adrs",
			dir: func(root string, _ workflowPaths) string {
				return filepath.Join(root, "docs", "adr")
			},
			sourceName: "001-links.md",
			targetName: "002-target.md",
		},
	}

	for _, layout := range layouts {
		for _, family := range families {
			t.Run(layout.name+"/"+family.name, func(t *testing.T) {
				root := t.TempDir()
				layout.setup(t, root)
				paths := workflowPathsFor(root)
				dir := family.dir(root, paths)
				writeFile(t, filepath.Join(dir, family.targetName), "# Target\n")
				writeFile(t, filepath.Join(dir, family.sourceName),
					"# Links\n\n[valid]("+family.targetName+")\n[missing](missing.md)\n")

				report, _ := validateWorkflowScopedForPaths(root, []string{CheckScopeLinks}, paths)
				var findings []validationFinding
				for _, finding := range report.Warnings {
					if finding.Code == "markdown_link_missing" {
						findings = append(findings, finding)
					}
				}
				if len(findings) != 1 {
					t.Fatalf("markdown_link_missing findings = %#v, want one", findings)
				}
				wantPath := relPath(root, filepath.Join(dir, family.sourceName)) + ":4"
				if findings[0].Path != wantPath {
					t.Errorf("finding path = %q, want %q", findings[0].Path, wantPath)
				}
				if strings.Contains(findings[0].Message, family.targetName) {
					t.Errorf("valid link target was reported missing: %#v", findings[0])
				}
			})
		}
	}
}

func TestValidateManagedRecordLinksIncludesGeneratedIndexes(t *testing.T) {
	layouts := []struct {
		name  string
		setup func(*testing.T, string)
	}{
		{name: "current", setup: setupAhmRepo},
		{name: "legacy", setup: initAndCreateLegacyMetadata},
	}

	for _, layout := range layouts {
		t.Run(layout.name, func(t *testing.T) {
			root := t.TempDir()
			layout.setup(t, root)
			paths := workflowPathsFor(root)
			indexes := []string{
				filepath.Join(root, filepath.FromSlash(paths.tasksRel()), "index.md"),
				filepath.Join(root, filepath.FromSlash(paths.researchRel()), "index.md"),
				filepath.Join(paths.execPlansDir("active"), "index.md"),
				filepath.Join(root, "docs", "adr", "index.md"),
			}
			for _, path := range indexes {
				writeFile(t, path, "# Index\n\n[missing](missing.md)\n")
			}

			report, _ := validateWorkflowScopedForPaths(root, []string{CheckScopeLinks}, paths)
			got := map[string]bool{}
			for _, finding := range report.Warnings {
				if finding.Code == "markdown_link_missing" {
					got[strings.TrimSuffix(finding.Path, ":3")] = true
				}
			}
			for _, path := range indexes {
				rel := relPath(root, path)
				if !got[rel] {
					t.Errorf("missing generated-index link finding for %s: %#v", rel, report.Warnings)
				}
			}
			if len(got) != len(indexes) {
				t.Errorf("generated-index finding paths = %#v, want exactly %d", got, len(indexes))
			}
		})
	}
}

func TestValidateManagedRecordLinksExcludesProjectOwnedMarkdown(t *testing.T) {
	layouts := []struct {
		name  string
		setup func(*testing.T, string)
	}{
		{name: "current", setup: setupAhmRepo},
		{name: "legacy", setup: initAndCreateLegacyMetadata},
	}

	for _, layout := range layouts {
		t.Run(layout.name, func(t *testing.T) {
			root := t.TempDir()
			layout.setup(t, root)
			paths := workflowPathsFor(root)
			taskDir := filepath.Join(root, filepath.FromSlash(paths.tasksRel()), "active")
			writeFile(t, filepath.Join(taskDir, "001.md"), "# Managed\n\n[missing](managed-missing.md)\n")

			for _, path := range []string{
				"README.md",
				"AGENTS.md",
				"CLAUDE.md",
				"ARCHITECTURE.md",
				"docs/guide.md",
				".agents/NOTES.md",
				".agents/skills/example/SKILL.md",
			} {
				writeFile(t, filepath.Join(root, filepath.FromSlash(path)),
					"# Project owned\n\n[missing]("+strings.ReplaceAll(path, "/", "-")+"-missing.md)\n")
			}
			for _, path := range []string{
				filepath.Join(root, filepath.FromSlash(paths.tasksRel()), "README.md"),
				filepath.Join(root, filepath.FromSlash(paths.researchRel()), "README.md"),
				filepath.Join(root, filepath.FromSlash(paths.execPlansRel("")), "README.md"),
				filepath.Join(root, "docs", "adr", "README.md"),
			} {
				writeFile(t, path, "# Preserved scaffold\n\n[missing](scaffold-missing.md)\n")
			}
			for _, path := range []string{
				filepath.Join(paths.tasksBucketDir("active"), "project-notes", "guide.md"),
				filepath.Join(root, filepath.FromSlash(paths.researchRel()), "topics", "project-notes", "guide.md"),
				filepath.Join(paths.execPlansDir("active"), "archive", "README.md"),
			} {
				writeFile(t, path, "# Nested project notes\n\n[missing](nested-missing.md)\n")
			}
			if paths.recordsDir == toolRecordsDirName {
				writeFile(t, filepath.Join(root, ".agents", ".research", "topics", "stale.md"),
					"# Legacy record\n\n[missing](legacy-layout-missing.md)\n")
			} else {
				writeFile(t, filepath.Join(root, ".ahm", "research", "topics", "stale.md"),
					"# Current record\n\n[missing](current-layout-missing.md)\n")
			}

			report, _ := validateWorkflowScopedForPaths(root, []string{CheckScopeLinks}, paths)
			var findings []validationFinding
			for _, finding := range report.Warnings {
				if finding.Code == "markdown_link_missing" {
					findings = append(findings, finding)
				}
			}
			if len(findings) != 1 {
				t.Fatalf("markdown_link_missing findings = %#v, want only the managed task finding", findings)
			}
			if !strings.Contains(findings[0].Message, "managed-missing.md") {
				t.Errorf("unexpected managed link finding: %#v", findings[0])
			}
		})
	}
}

func TestStatusAndDoctorManagedLinksScopesAndOutputModes(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	writeADRFile(t, root, "001-links.md",
		"---\nstatus: accepted\ndate: 2026-07-27\n---\n"+
			"# Links\n\n[missing](missing.md)\n")
	var indexOut strings.Builder
	indexer := app{opts: options{root: root}, out: &indexOut}
	if err := indexer.writeIndexes(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		args []string
	}{
		{name: "status default text", args: []string{"status"}},
		{name: "doctor default text", args: []string{"doctor"}},
		{name: "status links JSON", args: []string{"--json", "status", "--check", "links"}},
		{name: "doctor links plain", args: []string{"--plain", "doctor", "--check", "links"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"--root", root}, tt.args...)
			stdout, stderr, code := runCLI(t, args...)
			if code != 0 {
				t.Fatalf("exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
			}
			assertContainsAll(t, stdout,
				"markdown_link_missing",
				"docs/adr/001-links.md:7",
				"relative Markdown link target does not exist: missing.md",
			)
		})
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
	report, _ := validateWorkflowScopedForPaths(root, []string{CheckScopeWorkflow}, workflowPathsFor(root))
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
	paths := workflowPathsFor(root)
	// Add a broken link.
	writeFile(t, filepath.Join(root, filepath.FromSlash(paths.researchRel()), "topics", "links.md"), "# Links\n\n[missing](missing.md)\n")
	// Create a workflow issue.
	writeTaskFile(t, paths.taskFile("active", "001"), "001", "Bad Task", "Doing", "depends_on: -\n")

	// Only link checks.
	report, _ := validateWorkflowScopedForPaths(root, []string{CheckScopeLinks}, paths)
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

func TestValidateWorkflowScopedAll(t *testing.T) {
	// nil scopes = default checks (same as validateWorkflow): workflow + links.
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	paths := workflowPathsFor(root)
	writeFile(t, filepath.Join(root, filepath.FromSlash(paths.researchRel()), "topics", "links.md"), "# Links\n\n[missing](missing.md)\n")

	// No scopes = default checks run.
	report, _ := validateWorkflowScopedForPaths(root, nil, paths)
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
	// validateWorkflowScopedForPaths with nil scopes should produce the same result.
	report2, _ := validateWorkflowScopedForPaths(root, nil, paths)
	if report.OK != report2.OK {
		t.Error("validateWorkflowScopedForPaths(nil) should match validateWorkflowScopedForPaths with workflow+links")
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

	report, _ := validateWorkflowScopedForPaths(root, nil, workflowPathsFor(root))
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

	report, _ := validateWorkflowScopedForPaths(root, nil, workflowPathsFor(root))
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
	report, _ := validateWorkflowScopedForPaths(root, nil, workflowPathsFor(root))
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

func TestDocsCommandRemoved(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{{"docs"}, {"docs", "check"}} {
		stdout, stderr, code := runCLI(t, append([]string{"--root", root}, args...)...)
		if code != 2 {
			t.Errorf("%v: exit code = %d, want 2; stdout = %s, stderr = %s", args, code, stdout, stderr)
		}
		if !strings.Contains(stderr, `unknown command "docs" for "ahm"`) {
			t.Errorf("%v: expected unknown-command usage error, got %s", args, stderr)
		}
	}
}

func TestProjectDocsCheckScopeRemoved(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	for _, command := range []string{"status", "doctor"} {
		stdout, stderr, code := runCLI(t, "--root", root, command, "--check", "project-docs")
		if code != 2 {
			t.Errorf("%s: exit code = %d, want 2; stdout = %s, stderr = %s", command, code, stdout, stderr)
		}
		assertContainsAll(t, stderr, `unknown check scope "project-docs"`, "valid: workflow, links")
	}

	for _, scopes := range []string{"workflow", "links", "workflow,links"} {
		stdout, stderr, code := runCLI(t, "--root", root, "status", "--check", scopes)
		if code != 0 {
			t.Errorf("%s: exit code = %d, stdout = %s, stderr = %s", scopes, code, stdout, stderr)
		}
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
