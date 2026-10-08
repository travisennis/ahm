package ahm

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

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
	root := projectRoot(t)
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
	root := projectRoot(t)
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
	// 004 has an incomplete dependency, so the deps-complete warning must not
	// be reported for it. Match that finding's text rather than the bare id: the
	// report echoes the temp-dir root, whose random suffix can contain "004" on
	// its own, and 004 does carry a separate missing-reason warning.
	assertNotContains(t, got, "task 004 is Blocked but all its dependencies are Completed")
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
