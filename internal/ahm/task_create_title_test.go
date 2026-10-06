package ahm

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTaskCreateWarnsOnDuplicateTitle covers the duplicate-title warning added
// to task create: an active record already carrying the title is reported, the
// task is still written, and the comparison is exact and case-insensitive over
// active records only.
func TestTaskCreateWarnsOnDuplicateTitle(t *testing.T) {
	initAndCreate := func(t *testing.T, root string, title string) (string, string, int) {
		t.Helper()
		if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
			t.Fatalf("init exit code = %d, stderr = %s", code, stderr)
		}
		return runCLI(t, "--root", root, "task", "create", title, "--status", "Pending")
	}

	t.Run("exact collision warns and still creates", func(t *testing.T) {
		root := projectRoot(t)
		stdout, stderr, code := initAndCreate(t, root, "Test body edit drift")
		if code != 0 || strings.TrimSpace(stdout) != "001" {
			t.Fatalf("first create stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
		}
		if strings.Contains(stderr, "duplicates the title") {
			t.Fatalf("first create warned about a duplicate:\n%s", stderr)
		}

		stdout, stderr, code = runCLI(t, "--root", root, "task", "create", "Test body edit drift", "--status", "Pending")
		if code != 0 || strings.TrimSpace(stdout) != "002" {
			t.Fatalf("second create stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
		}
		assertContainsAll(t, stderr, "duplicates the title of active task 001 [Pending]", `"Test body edit drift"`)
		if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "active", "002.md")); err != nil {
			t.Errorf("colliding create did not write the record: %v", err)
		}
	})

	t.Run("case-insensitive collision warns", func(t *testing.T) {
		root := projectRoot(t)
		if _, stderr, code := initAndCreate(t, root, "Test body edit drift"); code != 0 {
			t.Fatalf("first create stderr = %q, code = %d", stderr, code)
		}

		_, stderr, code := runCLI(t, "--root", root, "task", "create", "test BODY edit drift", "--status", "Pending")
		if code != 0 {
			t.Fatalf("case-insensitive create stderr = %q, code = %d", stderr, code)
		}
		assertContainsAll(t, stderr, "duplicates the title of active task 001 [Pending]")
	})

	t.Run("near miss does not warn", func(t *testing.T) {
		root := projectRoot(t)
		if _, stderr, code := initAndCreate(t, root, "Test body edit drift"); code != 0 {
			t.Fatalf("first create stderr = %q, code = %d", stderr, code)
		}

		_, stderr, code := runCLI(t, "--root", root, "task", "create", "Test body drift", "--status", "Pending")
		if code != 0 {
			t.Fatalf("near-miss create stderr = %q, code = %d", stderr, code)
		}
		assertNotContains(t, stderr, "duplicates the title")
	})

	t.Run("completed and cancelled records do not warn", func(t *testing.T) {
		root := projectRoot(t)
		if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
			t.Fatalf("init stderr = %q, code = %d", stderr, code)
		}
		writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "completed", "001.md"), "001", "Recurring chore", "Completed", "depends_on: -\n")
		writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "cancelled", "002.md"), "002", "Abandoned chore", "Cancelled", "depends_on: -\n")

		for _, title := range []string{"Recurring chore", "Abandoned chore"} {
			stdout, stderr, code := runCLI(t, "--root", root, "task", "create", title, "--status", "Pending")
			if code != 0 {
				t.Fatalf("create %q stdout = %q, stderr = %q, code = %d", title, stdout, stderr, code)
			}
			assertNotContains(t, stderr, "duplicates the title")
		}
	})

	t.Run("failing create does not warn", func(t *testing.T) {
		root := projectRoot(t)
		if _, stderr, code := initAndCreate(t, root, "Test body edit drift"); code != 0 {
			t.Fatalf("first create stderr = %q, code = %d", stderr, code)
		}

		// The dependency is missing, so the create is refused: the duplicate
		// title must not be reported for a task that is never written.
		_, stderr, code := runCLI(t, "--root", root, "task", "create", "Test body edit drift", "--depends-on", "999")
		if code != 2 {
			t.Fatalf("failing create stderr = %q, code = %d, want usage failure", stderr, code)
		}
		assertNotContains(t, stderr, "duplicates the title")
	})

	t.Run("dry-run warns without writing", func(t *testing.T) {
		root := projectRoot(t)
		if _, stderr, code := initAndCreate(t, root, "Dry run duplicate"); code != 0 {
			t.Fatalf("first create stderr = %q, code = %d", stderr, code)
		}

		stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "create", "Dry run duplicate", "--status", "Pending")
		if code != 0 {
			t.Fatalf("dry-run stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
		}
		assertContainsAll(t, stdout, "002")
		assertContainsAll(t, stderr, "duplicates the title of active task 001 [Pending]")
		if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "active", "002.md")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("dry-run wrote a record it should not have: %v", err)
		}
	})
}
