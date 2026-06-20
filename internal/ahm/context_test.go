package ahm

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/travisennis/ahm/internal/templates"
)

func TestContextPrintsSessionBriefing(t *testing.T) {
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

	stdout, stderr, code := runCLI(t, "--root", root, "context")
	if code != 0 {
		t.Fatalf("context exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"# ahm context",
		"workflow: installed "+templates.Version,
		"validation: ok",
		"tasks: open=0 ready=1 blocked=0 in_progress=1",
		"next: 002 [Pending] P2 S Ready Work",
		"in_progress: 001 [In Progress] P2 S Current Work",
		"## Instructions",
		"This briefing already includes repository state.",
		"Use scoped context before work that needs workflow rules",
		"`ahm context task`",
		"`ahm context docs`",
		"## Useful Commands",
		"`ahm task show <id>`",
	)
	assertNotContains(t, stdout, "Start by running `ahm context`")
}

func TestContextScopePrintsEmbeddedInstructionDocument(t *testing.T) {
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

func TestContextJSONOutput(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "--root", root, "--json", "context", "task")
	if code != 0 {
		t.Fatalf("context --json exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		`"root": "`+root+`"`,
		`"template_version": "`+templates.Version+`"`,
		`"validation_ok": true`,
		`"instructions":`,
		`"id": "task-workflow"`,
		`# Task Workflow`,
		`## Choosing Work`,
		`"commands":`,
		`"ahm task show \u003cid\u003e"`,
	)
}

func TestContextReportsValidationFindingsWithoutFailing(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Missing Dependency", "Pending", "depends_on: 999\n")

	stdout, stderr, code := runCLI(t, "--root", root, "context")
	if code != 0 {
		t.Fatalf("context exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"validation:",
		"task_dependency_missing",
		"task 001 depends on missing task 999",
		"`ahm doctor`",
	)
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
