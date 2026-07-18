package ahm

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const legacyGitCleanupCommand = "git add .ahm/ && git rm -r --cached .agents/.tasks .agents/.research .agents/exec-plans .agents/ahm.json"

func newLegacyCommittedRepo(t *testing.T) string {
	t.Helper()
	root := newGitRepo(t)
	writeFile(t, filepath.Join(root, ".agents", "ahm.json"), `{
  "version": "test",
  "strict_acceptance": false,
  "files": {}
}
`)
	writeFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "# Task One\n")
	writeFile(t, filepath.Join(root, ".agents", ".tasks", "index.md"), "# Generated Index\n")
	writeFile(t, filepath.Join(root, ".agents", ".research", "topics", "note.md"), "# Note\n")
	writeFile(t, filepath.Join(root, ".agents", "exec-plans", "active", "plan.md"), "# Plan\n")
	writeFile(t, filepath.Join(root, ".agents", "prompt.md"), "project-owned prompt\n")
	git(t, root, "add", ".agents")
	git(t, root, "commit", "-q", "-m", "add legacy workflow records")
	return root
}

func TestRecordsMigrateDryRunPreviewsWithoutWriting(t *testing.T) {
	root := newLegacyCommittedRepo(t)

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "records", "migrate")
	if code != 0 {
		t.Fatalf("records migrate --dry-run exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"action: migrate",
		"dry_run: true",
		"moves: 4",
		".agents/.tasks/active/001.md -> .ahm/tasks/active/001.md",
		".agents/.tasks/index.md -> .ahm/tasks/index.md",
		".agents/.research/topics/note.md -> .ahm/research/topics/note.md",
		".agents/exec-plans/active/plan.md -> .ahm/exec-plans/active/plan.md",
		"gitignore: create",
		"config: create",
		"legacy_config: remove",
		"git_cleanup: "+legacyGitCleanupCommand,
		"no files, metadata, or gitignore were changed",
	)
	assertNotContains(t, stdout, ".agents/prompt.md")
	assertNotContains(t, stdout, "ref_action")
	assertNotContains(t, stdout, "ref:")

	if _, err := os.Stat(filepath.Join(root, ".ahm")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run created .ahm: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", "ahm.json")); err != nil {
		t.Fatalf("dry-run removed legacy config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", ".tasks", "active", "001.md")); err != nil {
		t.Fatalf("dry-run moved task file: %v", err)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "--json", "--dry-run", "records", "migrate")
	if code != 0 {
		t.Fatalf("records migrate --json --dry-run exit code = %d, stderr = %s", code, stderr)
	}
	var report recordsMigrateReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("records migrate --json output is not JSON: %v\n%s", err, stdout)
	}
	if !report.DryRun || report.Action != "migrate" || len(report.Moves) != 4 || report.GitCleanup != legacyGitCleanupCommand {
		t.Fatalf("records migrate --json --dry-run = %#v", report)
	}
}

func TestRecordsMigrateMovesRecordsAndPrintsGitCleanup(t *testing.T) {
	root := newLegacyCommittedRepo(t)
	headBefore := strings.TrimSpace(git(t, root, "rev-parse", "HEAD"))

	stdout, stderr, code := runCLI(t, "--root", root, "records", "migrate")
	if code != 0 {
		t.Fatalf("records migrate exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"action: migrate",
		"git_cleanup: "+legacyGitCleanupCommand,
		"migrated workflow records to .ahm/",
	)
	assertNotContains(t, stdout, "ref_action")
	assertNotContains(t, stdout, "seed_commit")
	assertNotContains(t, stdout, "ref:")

	// Records and generated indexes moved to .ahm/; sources are gone.
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "# Task One")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "index.md"), "# Generated Index")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "research", "topics", "note.md"), "# Note")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "exec-plans", "active", "plan.md"), "# Plan")
	for _, gone := range []string{
		filepath.Join(root, ".agents", ".tasks"),
		filepath.Join(root, ".agents", ".research"),
		filepath.Join(root, ".agents", "exec-plans"),
		filepath.Join(root, ".agents", "ahm.json"),
	} {
		if _, err := os.Stat(gone); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("legacy path %s still exists: %v", gone, err)
		}
	}

	// Project-owned .agents/ content is preserved.
	assertFileContainsAll(t, filepath.Join(root, ".agents", "prompt.md"), "project-owned prompt")

	// Committed config and internal gitignore are installed.
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "config.json"),
		`"version": "test"`,
		`"strict_acceptance": false`,
	)
	// Config must NOT contain ref-back fields.
	assertNotContains(t, mustRead(t, filepath.Join(root, ".ahm", "config.json")),
		"store_mode", "records_ref", "records_remote")

	// Gitignore ignores generated indexes and machine-local state, NOT source records.
	assertFileContainsAll(t, filepath.Join(root, ".ahm", ".gitignore"),
		"index.md",
		".lock/",
		"*.tmp",
	)
	assertNotContains(t, mustRead(t, filepath.Join(root, ".ahm", ".gitignore")),
		"/tasks/", "/research/", "/exec-plans/")

	// Migration does not run git rm, stage changes, or move HEAD.
	if got := strings.TrimSpace(git(t, root, "rev-parse", "HEAD")); got != headBefore {
		t.Fatalf("migration moved HEAD from %s to %s", headBefore, got)
	}
	if staged := strings.TrimSpace(git(t, root, "diff", "--cached", "--name-only")); staged != "" {
		t.Fatalf("migration staged changes:\n%s", staged)
	}
	if tracked := strings.TrimSpace(git(t, root, "ls-files", "--", ".agents/.tasks")); tracked == "" {
		t.Fatal("migration untracked legacy records instead of printing the git cleanup command")
	}
}

func TestRecordsMigrateRelinquishesFormerManagedSkills(t *testing.T) {
	root := newLegacyCommittedRepo(t)
	writeTaskFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "001", "Task One", "Pending", "depends_on: -\n")
	meta, err := readMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range projectOwnedProcedureSkills {
		content := "managed " + target + "\n"
		writeFile(t, filepath.Join(root, filepath.FromSlash(target)), content)
		meta.Files[target] = hashBytes([]byte(content))
	}
	if err := writeMetadata(root, meta); err != nil {
		t.Fatal(err)
	}

	if _, stderr, code := runCLI(t, "--root", root, "records", "migrate"); code != 0 {
		t.Fatalf("records migrate exit code = %d, stderr = %s", code, stderr)
	}

	migrated, err := readMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range projectOwnedProcedureSkills {
		assertFileContainsAll(t, filepath.Join(root, filepath.FromSlash(target)), "managed "+target)
		if _, ok := migrated.Files[target]; ok {
			t.Errorf("project-owned skill %s should not remain in migrated metadata", target)
		}
	}

	// Re-running migration repairs configs written by older ahm versions that
	// carried the stale ownership hashes into .ahm/config.json.
	for _, target := range projectOwnedProcedureSkills {
		migrated.Files[target] = "stale-managed-hash"
	}
	if err := writeMetadata(root, migrated); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runCLI(t, "--root", root, "records", "migrate")
	if code != 0 {
		t.Fatalf("repeated records migrate exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "config: update")
	migrated, err = readMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range projectOwnedProcedureSkills {
		if _, ok := migrated.Files[target]; ok {
			t.Errorf("stale project-owned skill %s should be removed from migrated metadata", target)
		}
		assertFileContainsAll(t, filepath.Join(root, filepath.FromSlash(target)), "managed "+target)
	}

	// A later forced upgrade must not reclaim or remove the project-owned files.
	stdout, stderr, code = runCLI(t, "--root", root, "--force", "upgrade")
	if code != 0 {
		t.Fatalf("forced upgrade exit code = %d, stderr = %s", code, stderr)
	}
	for _, target := range projectOwnedProcedureSkills {
		assertNotContains(t, stdout, target)
		assertFileContainsAll(t, filepath.Join(root, filepath.FromSlash(target)), "managed "+target)
	}
}

func TestRecordsMigrateIsIdempotentAndReportsGitCleanup(t *testing.T) {
	root := newLegacyCommittedRepo(t)
	if _, stderr, code := runCLI(t, "--root", root, "records", "migrate"); code != 0 {
		t.Fatalf("first records migrate failed: %s", stderr)
	}

	stdout, stderr, code := runCLI(t, "--root", root, "records", "migrate")
	if code != 0 {
		t.Fatalf("second records migrate exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"records storage is already migrated",
		"git_cleanup: "+legacyGitCleanupCommand,
		"moves: 0",
	)

	// Simulate the user running the git cleanup command.
	git(t, root, "rm", "-r", "-q", "--cached", ".agents/.tasks", ".agents/.research", ".agents/exec-plans", ".agents/ahm.json")
	git(t, root, "commit", "-q", "-m", "untrack migrated records")

	stdout, stderr, code = runCLI(t, "--root", root, "records", "migrate")
	if code != 0 {
		t.Fatalf("third records migrate exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "git_cleanup: none", "records storage is already migrated")
	assertNotContains(t, stdout, "git add .ahm/")
}

func TestRecordsMigrateConsolidatesLegacyDotRecordDirectories(t *testing.T) {
	root := newGitRepo(t)
	writeFile(t, filepath.Join(root, ".ahm", "config.json"), `{
  "version": "test",
  "strict_acceptance": false,
  "files": {}
}
`)
	writeFile(t, filepath.Join(root, ".ahm", ".tasks", "active", "001.md"), "# Task One\n")
	writeFile(t, filepath.Join(root, ".ahm", ".research", "topics", "note.md"), "# Note\n")
	git(t, root, "add", ".ahm")
	git(t, root, "commit", "-q", "-m", "add legacy dot record directories")

	stdout, stderr, code := runCLI(t, "--root", root, "records", "doctor")
	if code != 0 {
		t.Fatalf("records doctor exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"legacy dot-prefixed record paths under .ahm/ remain (.ahm/.tasks, .ahm/.research)",
		"ahm records migrate",
	)

	stdout, stderr, code = runCLI(t, "--root", root, "records", "migrate")
	if code != 0 {
		t.Fatalf("records migrate exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		".ahm/.tasks/active/001.md -> .ahm/tasks/active/001.md",
		".ahm/.research/topics/note.md -> .ahm/research/topics/note.md",
	)
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "# Task One")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "research", "topics", "note.md"), "# Note")
	for _, gone := range legacyDotRecordMigrationRoots {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(gone))); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("legacy path %s still exists: %v", gone, err)
		}
	}

	stdout, stderr, code = runCLI(t, "--root", root, "records", "doctor")
	if code != 0 {
		t.Fatalf("records doctor exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "migration: complete")
}

func TestRecordsMigrateResumesPartialStateAndRejectsConflicts(t *testing.T) {
	root := newLegacyCommittedRepo(t)

	// Simulate an interrupted migration: one record was already copied.
	writeFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "# Task One\n")
	stdout, stderr, code := runCLI(t, "--root", root, "records", "migrate")
	if code != 0 {
		t.Fatalf("resumed records migrate exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "migrated workflow records to .ahm/")
	if _, err := os.Stat(filepath.Join(root, ".agents", ".tasks")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("resumed migration left legacy task dir: %v", err)
	}

	// A differing target is a conflict, not a silent overwrite.
	conflicted := newLegacyCommittedRepo(t)
	writeFile(t, filepath.Join(conflicted, ".ahm", "tasks", "active", "001.md"), "# Different\n")
	_, stderr, code = runCLI(t, "--root", conflicted, "records", "migrate")
	if code != 1 {
		t.Fatalf("conflicting records migrate exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stderr,
		".ahm/tasks/active/001.md already exists with different content",
		"resolve the conflict before migrating",
	)
	assertFileContainsAll(t, filepath.Join(conflicted, ".agents", ".tasks", "active", "001.md"), "# Task One")
	assertFileContainsAll(t, filepath.Join(conflicted, ".ahm", "tasks", "active", "001.md"), "# Different")
}

func TestRecordsMigrateRequiresWorkflowMetadata(t *testing.T) {
	root := newGitRepo(t)
	_, stderr, code := runCLI(t, "--root", root, "records", "migrate")
	if code != 1 {
		t.Fatalf("records migrate exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stderr, "workflow metadata", "ahm init")
}

func TestRecordsDoctorDiagnosesPartialMigration(t *testing.T) {
	root := newLegacyCommittedRepo(t)

	if _, stderr, code := runCLI(t, "--root", root, "records", "migrate"); code != 0 {
		t.Fatalf("records migrate failed: %s", stderr)
	}

	// Migrated, but the git index still tracks legacy record paths.
	stdout, stderr, code := runCLI(t, "--root", root, "records", "doctor")
	if code != 0 {
		t.Fatalf("records doctor exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "ok: false", "project git index still tracks legacy record paths", "git add .ahm/")
	assertNotContains(t, stdout, "mode:")
	for _, outputMode := range []string{"--json", "--plain"} {
		structured, structuredErr, structuredCode := runCLI(t, "--root", root, outputMode, "records", "doctor")
		if structuredCode != 0 {
			t.Fatalf("records doctor %s exit code = %d, stderr = %s", outputMode, structuredCode, structuredErr)
		}
		assertContainsAll(t, structured, `"ok"`, `"migration"`)
		assertNotContains(t, structured, `"mode"`)
	}

	// Leftover legacy record files point back at migration.
	writeFile(t, filepath.Join(root, ".agents", ".tasks", "active", "002.md"), "# Straggler\n")
	stdout, stderr, code = runCLI(t, "--root", root, "records", "doctor")
	if code != 0 {
		t.Fatalf("records doctor exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "ok: false", "legacy record paths remain (.agents/.tasks)", "ahm records migrate")

	// Full cleanup ends with a complete migration check.
	if err := os.RemoveAll(filepath.Join(root, ".agents", ".tasks")); err != nil {
		t.Fatal(err)
	}
	git(t, root, "rm", "-r", "-q", "--cached", ".agents/.tasks", ".agents/.research", ".agents/exec-plans", ".agents/ahm.json")
	git(t, root, "commit", "-q", "-m", "untrack migrated records")
	stdout, stderr, code = runCLI(t, "--root", root, "records", "doctor")
	if code != 0 {
		t.Fatalf("records doctor exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "migration: complete")

	// Unmigrated repositories are pointed at the opt-in command.
	legacy := newLegacyCommittedRepo(t)
	stdout, stderr, code = runCLI(t, "--root", legacy, "records", "doctor")
	if code != 0 {
		t.Fatalf("records doctor exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "ok: false", "legacy record paths remain", "ahm records migrate")
}

func TestRecordsMigratePreservesAttachments(t *testing.T) {
	root := newLegacyCommittedRepo(t)
	// Add non-Markdown attachments and a custom subdirectory.
	writeFile(t, filepath.Join(root, ".agents", ".research", "topics", "diagram.png"), "PNG\x00content\n")
	writeFile(t, filepath.Join(root, ".agents", ".research", "inbox", "notes.txt"), "text attachment\n")
	writeFile(t, filepath.Join(root, ".agents", ".research", "sources", "reference.pdf"), "PDF\n")
	writeFile(t, filepath.Join(root, ".agents", ".research", "investigations", "subdir", "extra.md"), "# Subdir\n")
	git(t, root, "add", ".agents/.research")
	git(t, root, "commit", "-q", "-m", "add research attachments")

	if _, stderr, code := runCLI(t, "--root", root, "records", "migrate"); code != 0 {
		t.Fatalf("records migrate failed: %s", stderr)
	}

	// Non-Markdown attachments moved to .ahm/.
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "research", "topics", "diagram.png"), "PNG")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "research", "inbox", "notes.txt"), "text attachment")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "research", "sources", "reference.pdf"), "PDF")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "research", "investigations", "subdir", "extra.md"), "# Subdir")

	// Legacy paths are gone.
	for _, gone := range []string{
		filepath.Join(root, ".agents", ".research", "topics", "diagram.png"),
		filepath.Join(root, ".agents", ".research", "sources", "reference.pdf"),
	} {
		if _, err := os.Stat(gone); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("attachment still exists at %s: %v", gone, err)
		}
	}

	// Project-owned content preserved.
	assertFileContainsAll(t, filepath.Join(root, ".agents", "prompt.md"), "project-owned prompt")
}

func TestRecordsMigrateHandlesDirtySourceRecordContent(t *testing.T) {
	root := newLegacyCommittedRepo(t)
	// Dirty a source record — modified but not staged.
	writeFile(t, filepath.Join(root, ".agents", ".tasks", "active", "001.md"), "# Task One Modified\n")

	// Dry-run should still preview the full 4 moves (all records, modified content
	// is the dirtied one).
	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "records", "migrate")
	if code != 0 {
		t.Fatalf("dry-run with dirty source failed: %s", stderr)
	}
	assertContainsAll(t, stdout, "action: migrate", "moves: 4")

	// Actual migration should move the dirty content.
	if _, stderr, code := runCLI(t, "--root", root, "records", "migrate"); code != 0 {
		t.Fatalf("migrate with dirty source failed: %s", stderr)
	}
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "# Task One Modified")
	// Legacy path gone.
	if _, err := os.Stat(filepath.Join(root, ".agents", ".tasks", "active", "001.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("dirty source task still in legacy path after migration")
	}
}

func TestRecordsMigrateWithAttachmentsInDryRun(t *testing.T) {
	root := newLegacyCommittedRepo(t)
	writeFile(t, filepath.Join(root, ".agents", ".research", "topics", "image.png"), "image\n")
	git(t, root, "add", ".agents/.research")
	git(t, root, "commit", "-q", "-m", "add attachment")

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "records", "migrate")
	if code != 0 {
		t.Fatalf("dry-run with attachment failed: %s", stderr)
	}
	// Dry-run should include the attachment move in the count.
	assertContainsAll(t, stdout, "moves: 5")
	assertContainsAll(t, stdout, "image.png")
}
