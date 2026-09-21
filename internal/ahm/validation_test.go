package ahm

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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

	tasks, err := collectTasksForPaths(paths)
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
	if err := installer.install(); err != nil {
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

func TestValidationReportsBlockedDepsComplete(t *testing.T) {
	root := t.TempDir()
	// 002 is Blocked but all its deps (001) are Completed.
	// Use writeTaskFileWithDeps for all tasks so depends_on is always present.
	writeTaskFileWithDeps(t, filepath.Join(root, ".ahm", "tasks", "completed", "001.md"), "001", "Done Dep", "Completed", "-")
	writeTaskFileWithDeps(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "002", "Still Blocked Task", "Blocked", "001")
	// 003 is Pending with no deps — should not trigger the warning.
	writeTaskFileWithDeps(t, filepath.Join(root, ".ahm", "tasks", "active", "003.md"), "003", "Pending Dep", "Pending", "-")
	writeTaskFileWithDeps(t, filepath.Join(root, ".ahm", "tasks", "active", "004.md"), "004", "Legitimately Blocked", "Blocked", "003")

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	// doctor returns an error when validation has errors; warnings don't cause errors.
	_ = a.doctor()
	got := out.String()
	assertContainsAll(t, got,
		`"code": "task_blocked_deps_complete"`,
		`task 002 is Blocked but all its dependencies are Completed`,
	)
	// 004 has an incomplete dependency, so it must not be reported. Match the
	// finding text rather than the bare id: the report echoes the temp-dir root,
	// whose random suffix can contain "004" on its own.
	assertNotContains(t, got, "task 004 is Blocked")
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
	if err := installer.install(); err != nil {
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
	if err := installer.install(); err != nil {
		t.Fatal(err)
	}
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Completed In Active", "Completed", "depends_on: []\n")
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

	tasks, err := collectTasksForPaths(paths)
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
	if err := installer.install(); err != nil {
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
		`convert it to MADR front matter manually`,
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
	if err := installer.install(); err != nil {
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
	if err := installer.install(); err != nil {
		t.Fatal(err)
	}
	linkPath := writeLinkCarrierTask(t, root, "001", "[missing](missing.md)\n\n```md\n[ignored](also-missing.md)\n```\n")

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.doctor(); err != nil {
		t.Error(err)
	}
	got := out.String()
	assertContainsAll(t, got,
		`"code": "markdown_link_missing"`,
		`"path": "`+relPath(root, linkPath)+`:12"`,
		`relative Markdown link target does not exist: missing.md`,
	)
	assertNotContains(t, got, "also-missing.md")
}

// writeLinkCarrierTask writes a valid task whose body carries Markdown so
// link-scope tests exercise a surviving record family. The body starts on
// line 12 of the rendered file.
func writeLinkCarrierTask(t *testing.T, root string, id string, body string) string {
	t.Helper()
	path := workflowPathsFor(root).taskFile("active", id)
	writeFile(t, path, "---\n"+
		"id: "+id+"\n"+
		"title: Links\n"+
		"status: Pending\n"+
		"priority: P2\n"+
		"effort: S\n"+
		"labels: type:task\n"+
		"depends_on: -\n"+
		"---\n"+
		"# Links\n\n"+
		body)
	return path
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
	if err := installer.install(); err != nil {
		t.Fatal(err)
	}
	// Quoted example links inside inline code spans and fenced code blocks must
	// not be treated as navigation, but a real broken link on the same line
	// (outside any backticks) must still be reported.
	writeLinkCarrierTask(t, root, "001",
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

func TestValidateManagedRecordLinksByFamily(t *testing.T) {
	families := []struct {
		name       string
		dir        func(string, workflowPaths) string
		sourceName string
		targetName string
	}{
		{
			name: "tasks",
			dir: func(root string, paths workflowPaths) string {
				return filepath.Join(root, filepath.FromSlash(paths.recordsRel()), "active")
			},
			sourceName: "001.md",
			targetName: "002.md",
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

	for _, family := range families {
		t.Run(family.name, func(t *testing.T) {
			root := t.TempDir()
			setupAhmRepo(t, root)
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

func TestValidateManagedRecordLinksIncludesGeneratedIndexes(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	paths := workflowPathsFor(root)
	indexes := []string{
		filepath.Join(root, filepath.FromSlash(paths.recordsRel()), "index.md"),
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
}

func TestValidateManagedRecordLinksExcludesProjectOwnedMarkdown(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	paths := workflowPathsFor(root)
	taskDir := filepath.Join(root, filepath.FromSlash(paths.recordsRel()), "active")
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
		filepath.Join(root, filepath.FromSlash(paths.recordsRel()), "README.md"),
		filepath.Join(root, "docs", "adr", "README.md"),
	} {
		writeFile(t, path, "# Preserved scaffold\n\n[missing](scaffold-missing.md)\n")
	}
	for _, path := range []string{
		filepath.Join(paths.tasksBucketDir("active"), "project-notes", "guide.md"),
	} {
		writeFile(t, path, "# Nested project notes\n\n[missing](nested-missing.md)\n")
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
}

// TestValidateLinksSkipsRetiredRecordFamilies pins acceptance criterion one of
// task 264b: nothing in the retired research and ExecPlan trees is read any
// more, so a broken link inside one of them is never reported and deleting the
// directory changes no ahm behavior.
func TestValidateLinksSkipsRetiredRecordFamilies(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	paths := workflowPathsFor(root)
	for _, file := range []string{
		".ahm/research/inbox/note.md",
		".ahm/research/topics/note.md",
		".ahm/research/index.md",
		".ahm/exec-plans/active/note.md",
		".ahm/exec-plans/active/index.md",
	} {
		writeFile(t, filepath.Join(root, filepath.FromSlash(file)),
			"# Retired record\n\n[missing](missing.md)\n")
	}

	report, _ := validateWorkflowScopedForPaths(root, []string{CheckScopeLinks}, paths)
	for _, finding := range report.Warnings {
		if finding.Code == "markdown_link_missing" {
			t.Errorf("retired record family was link-checked: %#v", finding)
		}
	}
}

// TestRetiredRecordFamiliesChangeNothing pins acceptance criteria one and three
// of task 264b end to end. With retired records on disk, including broken links,
// stale indexes, and a task that still carries a dangling exec_plan value, the
// read-write commands read no file under either retired tree, report no finding
// for one, and report exactly the same findings once the trees are gone.
func TestRetiredRecordFamiliesChangeNothing(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	paths := workflowPathsFor(root)
	trees := []string{".ahm/research", ".ahm/exec-plans"}
	// A surviving task that still carries the retired field.
	writeFile(t, paths.taskFile("active", "001"), "---\n"+
		"id: 001\n"+
		"title: Retired Field\n"+
		"status: Pending\n"+
		"priority: P2\n"+
		"effort: S\n"+
		"labels: type:task\n"+
		"exec_plan: 999-old-plan\n"+
		"depends_on: -\n"+
		"---\n"+
		"# Retired Field\n\n## Acceptance Notes\n\n- [x] Done.\n")
	writeADRFile(t, root, "001-good-decision.md", "---\nstatus: accepted\ndate: 2026-07-01\n---\n# Good Decision\n\nBody.\n")
	for _, tree := range trees {
		writeFile(t, filepath.Join(root, filepath.FromSlash(tree), "inbox", "note.md"),
			"# Retired record\n\n[missing](missing.md)\n")
		writeFile(t, filepath.Join(root, filepath.FromSlash(tree), "index.md"),
			"# Retired index\n\n[missing](missing.md)\n")
	}

	reads := map[string]int{}
	original := readWorkflowFileHook
	readWorkflowFileHook = func(path string) { reads[relPath(root, path)]++ }
	t.Cleanup(func() { readWorkflowFileHook = original })

	// Exercise the read-write commands twice: once with the retired trees in
	// place and once after deleting them. index runs first so the surviving
	// generated state is settled and the later commands exit 0.
	commands := []string{"index", "status", "doctor", "prime"}
	beforeReport := runRetiredTreeCommands(t, root, commands)
	for _, tree := range trees {
		if err := os.RemoveAll(filepath.Join(root, filepath.FromSlash(tree))); err != nil { // #nosec G703 -- path built from t.TempDir
			t.Fatal(err)
		}
	}
	afterReport := runRetiredTreeCommands(t, root, commands)

	for path := range reads {
		if strings.Contains(path, "research") || strings.Contains(path, "exec-plans") {
			t.Errorf("read a retired record: %s", path)
		}
	}
	if !reflect.DeepEqual(beforeReport.Errors, afterReport.Errors) ||
		!reflect.DeepEqual(beforeReport.Warnings, afterReport.Warnings) ||
		!reflect.DeepEqual(beforeReport.Info, afterReport.Info) {
		t.Errorf("removing the retired trees changed the findings\nbefore: %+v\nafter: %+v", beforeReport, afterReport)
	}
}

// runRetiredTreeCommands runs each command, requires success, and returns the
// validation report of the read-write commands whose findings could mention a
// retired record.
func runRetiredTreeCommands(t *testing.T, root string, commands []string) validationReport {
	t.Helper()
	var report validationReport
	for _, command := range commands {
		stdout, stderr, code := runCLI(t, "--root", root, command)
		if code != 0 {
			t.Fatalf("%s exit code = %d, stdout = %s, stderr = %s", command, code, stdout, stderr)
		}
		assertNotContains(t, stdout+stderr, "exec_plan", "research_inbox_stale", "markdown_link_missing")
	}
	report, _ = validateWorkflowScopedForPaths(root, nil, workflowPathsFor(root))
	for _, findings := range [][]validationFinding{report.Errors, report.Warnings, report.Info} {
		for _, finding := range findings {
			if strings.Contains(finding.Path, "research") || strings.Contains(finding.Path, "exec-plans") {
				t.Errorf("retired record produced a finding: %#v", finding)
			}
			if strings.Contains(finding.Code, "research") || strings.Contains(finding.Code, "exec_plan") {
				t.Errorf("retired finding code emitted: %#v", finding)
			}
		}
	}
	return report
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
	writeLinkCarrierTask(t, root, "002", "[missing](missing.md)\n")
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
	if err := installer.install(); err != nil {
		t.Fatal(err)
	}
	paths := workflowPathsFor(root)
	// Add a broken link.
	writeLinkCarrierTask(t, root, "002", "[missing](missing.md)\n")
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
	if err := installer.install(); err != nil {
		t.Fatal(err)
	}
	paths := workflowPathsFor(root)
	writeLinkCarrierTask(t, root, "002", "[missing](missing.md)\n")

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
	if err := installer.install(); err != nil {
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
	if err := installer.install(); err != nil {
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
	path := filepath.Join(root, ".ahm", "tasks", "active", "001.md")
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
	writeMetadataFile(t, root, metadata{Version: "0.1.0", Files: map[string]string{}})
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
		if err.Code == "metadata_corrupt" && err.Path != configMetadataRelPath {
			t.Errorf("metadata_corrupt finding path = %q, want %q: %v", err.Path, configMetadataRelPath, err)
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

func TestPostMutation_ScopeIsWorkflowOnly(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)

	// Create a workflow finding: a completed task sitting in the active bucket.
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Completed In Active", "Completed", "depends_on: -\n")

	// Create a broken markdown link that would trigger markdown_link_missing.
	writeLinkCarrierTask(t, root, "002", "[missing](missing.md)\n")

	stdout, stderr, code := runCLI(t, "--root", root, "index")
	if code != 0 {
		t.Errorf("index exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	// Verify the workflow finding appears.
	assertContainsAll(t, stderr,
		"completed task should be in .ahm/tasks/completed",
	)
	// Verify the markdown_link_missing finding does NOT appear.
	assertNotContains(t, stderr,
		"markdown_link_missing",
	)
}

func TestPostMutation_DryRunSkipsValidation(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)

	// Create a workflow finding: a completed task sitting in the active bucket.
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Completed In Active", "Completed", "depends_on: -\n")

	stdout, stderr, code := runCLI(t, "--dry-run", "--root", root, "index")
	if code != 0 {
		t.Errorf("index exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	// The validation should not run during dry-run, so no warnings.
	if strings.Contains(stderr, "completed task should be in") {
		t.Errorf("dry-run index emitted unexpected warning on stderr:\n%s", stderr)
	}
}

func TestDocsCommandRemoved(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(); err != nil {
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
	if err := installer.install(); err != nil {
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

func TestIsUncheckedChecklistItem(t *testing.T) {
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
			got := false
			for _, line := range tt.lines {
				if isUncheckedChecklistItem(line) {
					got = true
				}
			}
			if got != tt.want {
				t.Errorf("isUncheckedChecklistItem() = %v, want %v", got, tt.want)
			}
		})
	}
}
