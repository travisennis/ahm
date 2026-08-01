package ahm

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// execPlanWithAllSections is a plan body that satisfies validateExecPlanSections
// for an active plan: every mandatory section is present and Outcomes is empty.
const execPlanWithAllSections = `# Plan

## Progress

## Surprises & Discoveries

## Decision Log

## Outcomes & Retrospective
`

// setupRecordTree fills root with tasks, ExecPlans, ADRs, and research notes so
// a mutation exercises every record kind the index generation reads.
func setupRecordTree(t *testing.T, root string, count int) workflowPaths {
	t.Helper()
	setupAhmRepo(t, root)
	paths := workflowPathsFor(root)
	for i := 1; i <= count; i++ {
		id := fmt.Sprintf("%03d", i)
		writeTaskFile(t, paths.taskFile("active", id), id, "Task "+id, "Pending", "depends_on: -\n")
		writeFile(t, filepath.Join(paths.execPlansDir("active"), fmt.Sprintf("plan-%d.md", i)), execPlanWithAllSections)
		writeFile(t, filepath.Join(root, "docs", "adr", fmt.Sprintf("%03d-decision.md", i)),
			"---\nid: "+id+"\nstatus: accepted\ndate: 2026-07-01\n---\n# "+id+" Decision\n\nBody.\n")
		writeFile(t, filepath.Join(root, ".ahm", "research", "topics", fmt.Sprintf("note-%d.md", i)), "# Note\n")
	}
	return paths
}

// countWorkflowReads runs fn with readWorkflowFile instrumented and returns the
// number of reads per repo-relative path.
func countWorkflowReads(t *testing.T, root string, fn func()) map[string]int {
	t.Helper()
	counts := map[string]int{}
	original := readWorkflowFileHook
	t.Cleanup(func() { readWorkflowFileHook = original })
	readWorkflowFileHook = func(path string) { counts[relPath(root, path)]++ }
	fn()
	readWorkflowFileHook = original
	return counts
}

// TestMutationReadsEachRecordOnce pins the reuse contract: index generation and
// the post-mutation validation that follows it share one read of every ExecPlan,
// ADR, and generated index instead of making a full second pass.
func TestMutationReadsEachRecordOnce(t *testing.T) {
	root := t.TempDir()
	setupRecordTree(t, root, 3)

	// Regenerate first so the measured run is steady state and not dominated by
	// the initial index creation.
	if stdout, stderr, code := runCLI(t, "--root", root, "index"); code != 0 {
		t.Fatalf("index exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}

	counts := countWorkflowReads(t, root, func() {
		stdout, stderr, code := runCLI(t, "--root", root, "task", "start", "001")
		if code != 0 {
			t.Fatalf("task start exit=%d stdout=%s stderr=%s", code, stdout, stderr)
		}
	})

	var measured int
	for path, count := range counts {
		isExecPlan := strings.HasPrefix(path, ".ahm/exec-plans/") && !strings.HasSuffix(path, "/index.md")
		isADR := strings.HasPrefix(path, "docs/adr/") && !strings.HasSuffix(path, "/index.md")
		isGeneratedIndex := strings.HasSuffix(path, "/index.md")
		if !isExecPlan && !isADR && !isGeneratedIndex {
			continue
		}
		measured++
		if count != 1 {
			t.Errorf("read count for %s = %d, want 1", path, count)
		}
	}
	// Guard against the assertion passing because nothing matched: 3 ExecPlans,
	// 3 ADRs, and the 8 generated indexes.
	if measured != 14 {
		t.Fatalf("measured %d records, want 14 (counts: %v)", measured, counts)
	}
}

// TestStandaloneValidationReadsFreshAfterMutation is the other half of the
// contract: the reuse is a per-command handoff, so status and doctor running
// after a mutation in the same process still see out-of-band edits.
func TestStandaloneValidationReadsFreshAfterMutation(t *testing.T) {
	root := t.TempDir()
	paths := setupRecordTree(t, root, 2)

	mutator := app{opts: options{root: root}, out: &strings.Builder{}, err: &strings.Builder{}}
	if err := mutator.writeIndexes(); err != nil {
		t.Fatal(err)
	}

	// Edit records behind ahm's back, exactly as a hand edit or a merge would.
	planPath := filepath.Join(paths.execPlansDir("active"), "plan-1.md")
	writeFile(t, planPath, "# Plan\n\n## Progress\n")
	indexPath := filepath.Join(paths.tasksBucketDir("active"), "index.md")
	writeFile(t, indexPath, "# Stale hand edit\n")

	report, _ := validateWorkflowScopedForPaths(root, []string{CheckScopeWorkflow}, paths)
	if !hasFinding(report.Warnings, "exec_plan_missing_section") {
		t.Errorf("missing exec_plan_missing_section warning for out-of-band plan edit: %#v", report.Warnings)
	}
	if !hasFinding(report.Warnings, "generated_index_stale") {
		t.Errorf("missing generated_index_stale warning for out-of-band index edit: %#v", report.Warnings)
	}
}

// TestRecordCachePutOverridesStaleRead covers the invariant the index write loop
// depends on: content written during a command must be visible to later reads
// through the same cache, or post-mutation validation would compare against
// pre-write bytes and report a phantom stale index.
func TestRecordCachePutOverridesStaleRead(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.md")
	writeFile(t, path, "before\n")

	cache := newRecordCache()
	data, err := cache.readFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "before\n" {
		t.Fatalf("first read = %q, want %q", data, "before\n")
	}

	cache.put(path, []byte("after\n"))
	data, err = cache.readFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "after\n" {
		t.Fatalf("read after put = %q, want %q", data, "after\n")
	}
}

// TestNilRecordCacheReadsThrough keeps the nil cache honest: callers with
// nothing to hand off must still get current bytes on every call.
func TestNilRecordCacheReadsThrough(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.md")
	writeFile(t, path, "first\n")

	var cache *recordCache
	if data, err := cache.readFile(path); err != nil || string(data) != "first\n" {
		t.Fatalf("first read = %q, %v", data, err)
	}
	writeFile(t, path, "second\n")
	if data, err := cache.readFile(path); err != nil || string(data) != "second\n" {
		t.Fatalf("second read = %q, %v, want the updated content", data, err)
	}
}
