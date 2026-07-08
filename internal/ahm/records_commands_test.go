package ahm

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecordsStatusReportsRemoteStateWithoutMutatingFiles(t *testing.T) {
	root := newGitRepo(t)
	remote := newBareRemote(t)
	git(t, root, "remote", "add", "origin", remote)
	writeRefRecordsConfig(t, root)
	writeFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "# Task\n")
	before := mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"))
	if _, err := snapshotRecordsRef(testContext(t), root, recordsStorageConfig{Ref: defaultRecordsRef}, "snapshot"); err != nil {
		t.Fatal(err)
	}
	if err := pushRecordsRef(testContext(t), root, recordsStorageConfig{Ref: defaultRecordsRef, Remote: "origin"}); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "--root", root, "records", "status")
	if code != 0 {
		t.Fatalf("records status exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"mode: ref",
		"ref: refs/ahm/records",
		"remote: origin",
		"remote_supported: true",
		"relation: equal",
		"working_clean: true",
	)
	after := mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"))
	if after != before {
		t.Fatalf("records status mutated record file: before %q after %q", before, after)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "--json", "records", "status")
	if code != 0 {
		t.Fatalf("records status --json exit code = %d, stderr = %s", code, stderr)
	}
	var jsonStatus recordsStatusReport
	if err := json.Unmarshal([]byte(stdout), &jsonStatus); err != nil {
		t.Fatalf("records status --json output is not JSON: %v\n%s", err, stdout)
	}
	if jsonStatus.Mode != "ref" || jsonStatus.Relation != "equal" || !jsonStatus.Working.Clean {
		t.Fatalf("records status --json = %#v", jsonStatus)
	}
}

func TestRecordsPushPullAndSyncUsePrivateRef(t *testing.T) {
	source := newGitRepo(t)
	remote := newBareRemote(t)
	git(t, source, "remote", "add", "origin", remote)
	writeRefRecordsConfig(t, source)
	writeFile(t, filepath.Join(source, ".ahm", "tasks", "active", "001.md"), "# One\n")

	stdout, stderr, code := runCLI(t, "--root", source, "records", "push")
	if code != 0 {
		t.Fatalf("records push exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "action: push", "message: pushed local records", ".ahm/tasks/active/001.md")
	if got := strings.TrimSpace(git(t, remote, "rev-parse", defaultRecordsRef+"^{commit}")); got == "" {
		t.Fatal("remote records ref was not pushed")
	}

	clone := newGitRepo(t)
	git(t, clone, "remote", "add", "origin", remote)
	writeRefRecordsConfig(t, clone)
	stdout, stderr, code = runCLI(t, "--root", clone, "records", "pull")
	if code != 0 {
		t.Fatalf("records pull exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "action: pull", "message: pulled remote records", ".ahm/tasks/active/001.md")
	assertFileContainsAll(t, filepath.Join(clone, ".ahm", "tasks", "active", "001.md"), "# One")

	writeFile(t, filepath.Join(clone, ".ahm", "tasks", "active", "001.md"), "# Two\n")
	stdout, stderr, code = runCLI(t, "--root", clone, "records", "sync")
	if code != 0 {
		t.Fatalf("records sync exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "action: sync", "snapshotted and pushed local record changes")

	_, stderr, code = runCLI(t, "--root", source, "records", "pull")
	if code != 0 {
		t.Fatalf("source records pull exit code = %d, stderr = %s", code, stderr)
	}
	assertFileContainsAll(t, filepath.Join(source, ".ahm", "tasks", "active", "001.md"), "# Two")
}

func TestRecordsDryRunPushDoesNotCreateRef(t *testing.T) {
	root := newGitRepo(t)
	remote := newBareRemote(t)
	git(t, root, "remote", "add", "origin", remote)
	writeRefRecordsConfig(t, root)
	writeFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "# Task\n")

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "records", "push")
	if code != 0 {
		t.Fatalf("records push --dry-run exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "action: push", "dry_run: true", "would snapshot .ahm records")
	if _, err := resolveGitRef(testContext(t), root, defaultRecordsRef); err == nil {
		t.Fatal("dry-run push created local records ref")
	}

	stdout, stderr, code = runCLI(t, "--root", root, "--plain", "--dry-run", "records", "sync")
	if code != 0 {
		t.Fatalf("records sync --plain --dry-run exit code = %d, stderr = %s", code, stderr)
	}
	var plainSync recordsOperationReport
	if err := json.Unmarshal([]byte(stdout), &plainSync); err != nil {
		t.Fatalf("records sync --plain --dry-run output is not compact JSON: %v\n%s", err, stdout)
	}
	if plainSync.Action != "sync" || !plainSync.DryRun {
		t.Fatalf("records sync --plain --dry-run = %#v", plainSync)
	}
	if strings.Contains(stdout, "\n  ") {
		t.Fatalf("records sync --plain should be compact JSON, got:\n%s", stdout)
	}
}

func TestRecordsPushReportsNonFastForward(t *testing.T) {
	first := newGitRepo(t)
	remote := newBareRemote(t)
	git(t, first, "remote", "add", "origin", remote)
	writeRefRecordsConfig(t, first)
	writeFile(t, filepath.Join(first, ".ahm", "tasks", "active", "001.md"), "# One\n")
	if _, _, code := runCLI(t, "--root", first, "records", "push"); code != 0 {
		t.Fatalf("initial records push failed")
	}

	second := newGitRepo(t)
	git(t, second, "remote", "add", "origin", remote)
	writeRefRecordsConfig(t, second)
	if _, _, code := runCLI(t, "--root", second, "records", "pull"); code != 0 {
		t.Fatalf("second records pull failed")
	}

	writeFile(t, filepath.Join(first, ".ahm", "tasks", "active", "001.md"), "# From first\n")
	if _, _, code := runCLI(t, "--root", first, "records", "sync"); code != 0 {
		t.Fatalf("first records sync failed")
	}

	writeFile(t, filepath.Join(second, ".ahm", "tasks", "active", "001.md"), "# From second\n")
	_, stderr, code := runCLI(t, "--root", second, "records", "push")
	if code != 1 {
		t.Fatalf("records push exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stderr, "not a fast-forward", "ahm records pull")
}

func TestRecordsDiagnosticsForUnsupportedRemote(t *testing.T) {
	root := newGitRepo(t)
	git(t, root, "remote", "add", "origin", "git@gitlab.com:example/repo.git")
	writeRefRecordsConfig(t, root)

	stdout, stderr, code := runCLI(t, "--root", root, "records", "doctor")
	if code != 0 {
		t.Fatalf("records doctor exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "ok: false", "unsupported URL", "GitHub remotes")

	_, stderr, code = runCLI(t, "--root", root, "records", "push")
	if code != 1 {
		t.Fatalf("records push exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stderr, "unsupported URL", "GitHub remotes")
}

func writeRefRecordsConfig(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, ".ahm", "config.json"), `{
  "version": "test",
  "strict_acceptance": false,
  "store_mode": "ref",
  "records_ref": "refs/ahm/records",
  "records_remote": "origin",
  "files": {}
}
`)
}

func newBareRemote(t *testing.T) string {
	t.Helper()
	remote := t.TempDir()
	git(t, remote, "init", "--bare", "-q")
	return remote
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}
