package ahm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTaskEditWarnsOnDuplicateTitle covers the rename side of the duplicate-title
// warning: `task edit --title` reuses the create-side rule, reports a collision
// with an active record and still writes, never reports a task against itself,
// and never reports a collision the edit did not introduce.
func TestTaskEditWarnsOnDuplicateTitle(t *testing.T) {
	taskTitle := func(t *testing.T, root, id string) string {
		t.Helper()
		out, stderr, code := runCLI(t, "--root", root, "--json", "task", "show", id)
		if code != 0 {
			t.Fatalf("task show %s: %s", id, stderr)
		}
		var task Task
		if err := json.Unmarshal([]byte(out), &task); err != nil {
			t.Fatal(err)
		}
		return task.Title
	}

	t.Run("rename to an active title warns and still writes", func(t *testing.T) {
		root := projectRoot(t)
		createTask(t, root, "First")
		createTask(t, root, "Second")

		stdout, stderr, code := runCLI(t, "--root", root, "task", "edit", "002", "--title", "First")
		if code != 0 {
			t.Fatalf("edit stdout=%q stderr=%q code=%d", stdout, stderr, code)
		}
		assertContainsAll(t, stdout, "002 updated (title)")
		assertContainsAll(t, stderr, "task 002 duplicates the title of active task 001 [Open]", `"First"`)
		if got := taskTitle(t, root, "002"); got != "First" {
			t.Errorf("rename did not write, title = %q", got)
		}
	})

	t.Run("rename to the same title does not warn", func(t *testing.T) {
		root := projectRoot(t)
		createTask(t, root, "Solo")

		_, stderr, code := runCLI(t, "--root", root, "task", "edit", "001", "--title", "Solo")
		if code != 0 {
			t.Fatalf("edit stderr=%q code=%d", stderr, code)
		}
		assertNotContains(t, stderr, "duplicates the title")

		// A case-only rename is a real change but must not report the task against
		// itself, so it still produces no warning.
		_, stderr, code = runCLI(t, "--root", root, "task", "edit", "001", "--title", "solo")
		if code != 0 {
			t.Fatalf("case rename stderr=%q code=%d", stderr, code)
		}
		assertNotContains(t, stderr, "duplicates the title")
	})

	t.Run("a non-title edit does not report a pre-existing pair", func(t *testing.T) {
		root := projectRoot(t)
		if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
			t.Fatalf("init: %s", stderr)
		}
		// A duplicate pair that already exists before the command runs: the edit
		// does not introduce it, so it must stay silent.
		writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Shared title", "Pending", "depends_on: -\n")
		writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "002", "Shared title", "Pending", "depends_on: -\n")

		_, stderr, code := runCLI(t, "--root", root, "task", "edit", "001", "--priority", "P1")
		if code != 0 {
			t.Fatalf("edit stderr=%q code=%d", stderr, code)
		}
		assertNotContains(t, stderr, "duplicates the title")
	})

	t.Run("a case-only rename does not re-report a pre-existing pair", func(t *testing.T) {
		root := projectRoot(t)
		if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
			t.Fatalf("init: %s", stderr)
		}
		writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Shared title", "Pending", "depends_on: -\n")
		writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "002", "Shared title", "Pending", "depends_on: -\n")

		_, stderr, code := runCLI(t, "--root", root, "task", "edit", "001", "--title", "shared title")
		if code != 0 {
			t.Fatalf("edit stderr=%q code=%d", stderr, code)
		}
		assertNotContains(t, stderr, "duplicates the title")
		if got := taskTitle(t, root, "001"); got != "shared title" {
			t.Errorf("case-only rename did not write, title = %q", got)
		}
	})

	t.Run("a completed title is not compared", func(t *testing.T) {
		root := projectRoot(t)
		if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
			t.Fatalf("init: %s", stderr)
		}
		writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "completed", "001.md"), "001", "Recurring chore", "Completed", "depends_on: -\n")
		createTask(t, root, "Other")

		_, stderr, code := runCLI(t, "--root", root, "task", "edit", "002", "--title", "Recurring chore")
		if code != 0 {
			t.Fatalf("edit stderr=%q code=%d", stderr, code)
		}
		assertNotContains(t, stderr, "duplicates the title")
	})

	t.Run("dry-run warns without writing", func(t *testing.T) {
		root := projectRoot(t)
		createTask(t, root, "First")
		createTask(t, root, "Second")

		_, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "edit", "002", "--title", "First")
		if code != 0 {
			t.Fatalf("dry-run stderr=%q code=%d", stderr, code)
		}
		assertContainsAll(t, stderr, "task 002 duplicates the title of active task 001 [Open]")
		if got := taskTitle(t, root, "002"); got != "Second" {
			t.Errorf("dry-run wrote a rename it should not have, title = %q", got)
		}
	})
}

// TestTaskImportWarnsOnDuplicateTitle covers the batch side: import reuses the
// create-side comparison for collisions with pre-existing active records and for
// duplicates inside the batch, and never changes the structured report.
func TestTaskImportWarnsOnDuplicateTitle(t *testing.T) {
	t.Run("collision with an existing active record warns", func(t *testing.T) {
		root := projectRoot(t)
		createTask(t, root, "Existing")

		out, stderr, code := runImport(t, root, importFile(t, `[{"title":"Existing"},{"title":"Fresh"}]`), "--json")
		if code != 0 {
			t.Fatalf("import code=%d stdout=%q stderr=%q", code, out, stderr)
		}
		assertContainsAll(t, stderr, "task 002 duplicates the title of active task 001 [Open]", `"Existing"`)
		report := importReport(t, out)
		for i, record := range report.Records {
			if record.Outcome != "imported" {
				t.Fatalf("record %d outcome=%q, report=%s", i, record.Outcome, out)
			}
		}
	})

	t.Run("duplicate inside the batch warns on the later record", func(t *testing.T) {
		root := projectRoot(t)

		_, stderr, code := runImport(t, root, importFile(t, `[{"title":"Same"},{"title":"Same"}]`), "--json")
		if code != 0 {
			t.Fatalf("import code=%d stderr=%q", code, stderr)
		}
		assertContainsAll(t, stderr, "task 002 duplicates the title of active task 001 [Open]", `"Same"`)
	})

	t.Run("a completed batch record is not compared", func(t *testing.T) {
		root := projectRoot(t)

		_, stderr, code := runImport(t, root, importFile(t, `[{"title":"Recurring chore","status":"Completed"},{"title":"Recurring chore","status":"Completed"}]`), "--json")
		if code != 0 {
			t.Fatalf("import code=%d stderr=%q", code, stderr)
		}
		assertNotContains(t, stderr, "duplicates the title")
	})

	t.Run("a clean batch warns nothing", func(t *testing.T) {
		root := projectRoot(t)
		createTask(t, root, "Existing")

		_, stderr, code := runImport(t, root, importFile(t, `[{"title":"Alpha"},{"title":"Beta"}]`), "--json")
		if code != 0 {
			t.Fatalf("import code=%d stderr=%q", code, stderr)
		}
		if stderr != "" {
			t.Fatalf("clean batch wrote to stderr: %q", stderr)
		}
	})

	t.Run("a refused batch warns nothing", func(t *testing.T) {
		root := projectRoot(t)
		createTask(t, root, "Existing")

		out, stderr, code := runImport(t, root, importFile(t, `[{"title":"Existing","depends_on":["999"]}]`), "--json")
		if code != 1 || stderr != "" {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, out, stderr)
		}
		assertNotContains(t, out, "duplicates the title")
	})

	t.Run("dry-run warns and leaves the report and the tree unchanged", func(t *testing.T) {
		root := projectRoot(t)
		createTask(t, root, "Existing")
		before := snapshotTree(t, root)

		out, stderr, code := runImport(t, root, importFile(t, `[{"title":"Existing"}]`), "--dry-run", "--json")
		if code != 0 {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, out, stderr)
		}
		assertContainsAll(t, stderr, "task 002 duplicates the title of active task 001 [Open]")
		report := importReport(t, out)
		if !report.DryRun || report.Records[0].Outcome != "planned" || report.Records[0].ID != "002" {
			t.Fatalf("dry-run report changed: %s", out)
		}
		if strings.Contains(out, "duplicates the title") {
			t.Fatalf("warning leaked into stdout report: %s", out)
		}
		assertTreeUnchanged(t, root, before)

		// stdin is the same path with a reader, and must behave identically.
		var stdout, errout strings.Builder
		a := app{out: &stdout, err: &errout, in: strings.NewReader(`[{"title":"Existing"}]`)}
		if err := a.run([]string{"--root", root, "--dry-run", "--json", "task", "import", "--from-file", "-"}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(errout.String(), "duplicates the title") {
			t.Fatalf("stdin dry-run did not warn: %q", errout.String())
		}
		if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "active", "002.md")); !os.IsNotExist(err) {
			t.Fatalf("dry-run wrote a record it should not have: %v", err)
		}
	})
}
