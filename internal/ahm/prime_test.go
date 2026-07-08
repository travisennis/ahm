package ahm

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/travisennis/ahm/internal/templates"
)

func TestPrimePrintsSessionBriefing(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Current Work", "In Progress", "depends_on: -\n")
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "002", "Ready Work", "Pending", "depends_on: -\n")
	indexer := app{opts: options{root: root}, out: &strings.Builder{}}
	if err := indexer.writeIndexes(); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "--root", root, "prime")
	if code != 0 {
		t.Fatalf("prime exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"root: "+root,
		"workflow: installed "+templates.Version,
		"validation: ok",
		"## In Progress",
		"001 [In Progress] P2 S Current Work",
		"## Ready",
		"002 [Pending] P2 S Ready Work",
		"Blocked: 0",
		"Open: 0",
		"## Managed Work Intake",
		"- Work a task → `ahm context task`, then `ahm task show <id>`",
		"- ExecPlan work → `ahm context plan`",
		"- ADR work → `ahm context adr`",
		"- Research notes → `ahm context research`",
		"- Documentation work → `ahm context docs`",
		"ahm manages work records, not implementation; after intake, classify the implementation under the project's own workflow routing (AGENTS.md).",
		"Before executing a multi-step plan, materialize it as ahm tasks (or an ExecPlan) — plans in context die at compaction; records survive.",
		"## Useful Commands",
		"`ahm task show <id>`",
	)
	assertNotContains(t, stdout,
		"# Dirty Worktree",
		"run `ahm task ready` for",
	)
}

func TestPrimeJSONOutput(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "--root", root, "--json", "prime")
	if code != 0 {
		t.Fatalf("prime --json exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		`"root": "`+root+`"`,
		`"template_version": "`+templates.Version+`"`,
		`"commands":`,
		`"in_progress":`,
		`"ready":`,
		`"ready_total":`,
		`"blocked":`,
		`"open":`,
	)
	assertNotContains(t, stdout, `"instructions"`)
}

func TestPrimePlainOutput(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Test Task", "Pending", "depends_on: -\n")

	stdout, stderr, code := runCLI(t, "--root", root, "--plain", "prime")
	if code != 0 {
		t.Fatalf("prime --plain exit code = %d, stderr = %s", code, stderr)
	}
	// Compact JSON should be a single line
	if strings.Count(stdout, "\n") > 2 {
		t.Fatalf("plain output should be compact JSON (single line), got %d lines:\n%s", strings.Count(stdout, "\n"), stdout)
	}
	assertContainsAll(t, stdout, `"root":"`+root+`"`, `"ready_total":`)
}

func TestPrimeNoSyncFlagSkipsRefSyncAndReportsStaleRecords(t *testing.T) {
	root := newGitRepo(t)
	writeRefRecordsConfig(t, root)
	writeTaskFile(t, filepath.Join(root, ".ahm", ".tasks", "active", "001.md"), "001", "Ref Task", "Pending", "depends_on: -\n")

	stdout, stderr, code := runCLI(t, "--root", root, "prime", "--no-sync")
	if code != 0 {
		t.Fatalf("prime --no-sync exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"# Records: local records ref is missing; run 'ahm records status'",
		"001 [Pending] P2 S Ref Task",
		"## Managed Work Intake",
	)
	if _, err := resolveGitRef(testContext(t), root, defaultRecordsRef); !errors.Is(err, errGitRefMissing) {
		t.Fatalf("prime --no-sync touched records ref: %v", err)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "--json", "prime", "--no-sync")
	if code != 0 {
		t.Fatalf("prime --json --no-sync exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		`"mode": "ref"`,
		`"stale": true`,
		`"message": "local records ref is missing; run 'ahm records status'"`,
	)
}

func TestPrimeReadyCapWithOverflow(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	// Create 7 ready tasks — only 5 should display with overflow pointer
	for i := 1; i <= 7; i++ {
		id := fmt.Sprintf("%03d", i)
		writeTaskFileWithPriority(t, filepath.Join(root, ".agents", ".tasks", "active", id+".md"), id, "Ready Task "+id, "Pending", "P2", "depends_on: -\n")
	}
	indexer := app{opts: options{root: root}, out: &strings.Builder{}}
	if err := indexer.writeIndexes(); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "--root", root, "prime")
	if code != 0 {
		t.Fatalf("prime exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"run `ahm task ready` for 2 more",
	)
	// Count ready task lines (lines like "001 [Pending] P2 S Ready Task 001")
	readyLines := 0
	for _, line := range strings.Split(stdout, "\n") {
		if strings.Contains(line, "[Pending]") && strings.Contains(line, "Ready Task") {
			readyLines++
		}
	}
	if readyLines != 5 {
		t.Fatalf("expected 5 ready task lines, got %d", readyLines)
	}
}

func TestPrimeDirtyWorktreeWarning(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	// Create and track a file, then make it dirty
	writeFile(t, filepath.Join(root, "tracked.txt"), "clean\n")
	if out, err := exec.Command("git", "-C", root, "add", "tracked.txt").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, out)
	}
	if out, err := exec.Command("git", "-C", root, "commit", "-m", "initial", "--allow-empty").CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}
	// Now dirty the tracked file
	writeFile(t, filepath.Join(root, "tracked.txt"), "dirty\n")

	stdout, stderr, code := runCLI(t, "--root", root, "prime")
	if code != 0 {
		t.Fatalf("prime exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"# Dirty Worktree",
		"The working directory has uncommitted changes",
		"Resolve them before starting new work.",
	)
}

func TestPrimeNoDirtyWarningOnCleanTree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}

	stdout, stderr, code := runCLI(t, "--root", root, "prime")
	if code != 0 {
		t.Fatalf("prime exit code = %d, stderr = %s", code, stderr)
	}
	assertNotContains(t, stdout, "# Dirty Worktree")
}

func TestPrimeNoWrites(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	// Capture the file tree before prime
	var beforeFiles []string
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			beforeFiles = append(beforeFiles, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk before prime: %v", err)
	}

	_, stderr, code := runCLI(t, "--root", root, "prime")
	if code != 0 {
		t.Fatalf("prime exit code = %d, stderr = %s", code, stderr)
	}

	// Check file tree after prime
	var afterFiles []string
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			afterFiles = append(afterFiles, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk after prime: %v", err)
	}

	if len(beforeFiles) != len(afterFiles) {
		t.Fatalf("prime created or removed files: before=%d, after=%d", len(beforeFiles), len(afterFiles))
	}
}

func TestPrimeReportsValidationFindingsWithoutFailing(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Missing Dependency", "Pending", "depends_on: 999\n")

	stdout, stderr, code := runCLI(t, "--root", root, "prime")
	if code != 0 {
		t.Fatalf("prime exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"validation:",
		"task_dependency_missing",
		"task 001 depends on missing task 999",
		"`ahm doctor`",
	)
}

func TestPrimeShowsReadyOnlyWhenTasksExist(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	// Only a non-ready task (Completed)
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Done Work", "Completed", "depends_on: -\n")
	indexer := app{opts: options{root: root}, out: &strings.Builder{}}
	if err := indexer.writeIndexes(); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "--root", root, "prime")
	if code != 0 {
		t.Fatalf("prime exit code = %d, stderr = %s", code, stderr)
	}
	assertNotContains(t, stdout, "## Ready")
}

func TestPrimeShowsInProgressOnlyWhenTasksExist(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	// Only ready tasks, no in-progress
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Ready Work", "Pending", "depends_on: -\n")
	indexer := app{opts: options{root: root}, out: &strings.Builder{}}
	if err := indexer.writeIndexes(); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "--root", root, "prime")
	if code != 0 {
		t.Fatalf("prime exit code = %d, stderr = %s", code, stderr)
	}
	assertNotContains(t, stdout, "## In Progress")
	assertContainsAll(t, stdout, "## Ready")
}
