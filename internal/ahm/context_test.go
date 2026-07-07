package ahm

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnscopedContextErrorsWithGuidance(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "--root", root, "context")
	if code != 2 {
		t.Fatalf("unscoped context exit code = %d (expected 2), stderr = %s", code, stderr)
	}
	assertContainsAll(t, stderr,
		"session briefing moved to `ahm prime`",
		"ahm prime",
		"Valid scoped contexts",
		"ahm context task",
		"ahm context adr",
	)
	if stdout != "" {
		t.Fatalf("unscoped context should not print any output to stdout, got:\n%s", stdout)
	}
}

func TestScopedContextPrintsEmbeddedInstructionDocument(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "--root", root, "context", "task")
	if code != 0 {
		t.Fatalf("context task exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"# Task Workflow",
		"## Choosing Work",
		"## Creating Tasks",
	)
	assertNotContains(t, stdout, "# ahm context", "git:", "## Useful Commands")
}

func TestUnscopedContextJSONErrors(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "--root", root, "--json", "context")
	if code != 2 {
		t.Fatalf("unscoped context --json exit code = %d (expected 2), stderr = %s", code, stderr)
	}
	assertContainsAll(t, stderr, "session briefing moved to `ahm prime`")
	if stdout != "" {
		t.Fatalf("unscoped context --json should not print anything to stdout, got:\n%s", stdout)
	}
}

func TestScopedContextJSONOutput(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "--root", root, "--json", "context", "task")
	if code != 0 {
		t.Fatalf("context task --json exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		`"scope": "task"`,
		`"instructions":`,
		`"id": "task-workflow"`,
		`# Task Workflow`,
		`## Choosing Work`,
		`"commands":`,
		`"ahm task show \u003cid\u003e"`,
	)
	assertNotContains(t, stdout, `"root"`, `"workflow"`, `"git"`, `"tasks"`)
}

func TestPrimeReportsValidationFindings(t *testing.T) {
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

func TestPrimeWarnsWhenMissingMetadataFallbackSkipsMalformedTasks(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "---\nbad key: value\n---\n# Broken Task\n")

	stdout, stderr, code := runCLI(t, "--root", root, "prime")
	if code != 0 {
		t.Fatalf("prime exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"metadata_missing",
		"task_malformed",
	)
	assertContainsAll(t, stderr, "warning: some task files could not be parsed and were skipped")
	if count := strings.Count(stderr, "warning: some task files could not be parsed and were skipped"); count != 1 {
		t.Fatalf("warning count = %d, stderr = %s", count, stderr)
	}
}

func TestPrimeWarningsOnlyValidationDisplay(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	// A Completed task in the active bucket yields a warning with no errors.
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Bucket Mismatch", "Completed", "")

	stdout, stderr, code := runCLI(t, "--root", root, "prime")
	if code != 0 {
		t.Fatalf("prime exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"warnings; run `ahm doctor`",
		"task_bucket_mismatch",
	)
	assertNotContains(t, stdout, "validation: ok")

	jsonOut, stderr, code := runCLI(t, "--root", root, "--json", "prime")
	if code != 0 {
		t.Fatalf("prime --json exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, jsonOut, `"validation_ok": false`)
}

func TestReadGitContextReportsDirtyWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	writeFile(t, filepath.Join(root, "tracked.txt"), "dirty\n")

	info := readGitContext(root)
	if !info.Available {
		t.Fatal("expected git to be available")
	}
	if info.Error != "" {
		t.Fatalf("unexpected git error: %s", info.Error)
	}
	if !info.Dirty || info.Changes == 0 {
		t.Fatalf("expected dirty worktree, got %#v", info)
	}
}
