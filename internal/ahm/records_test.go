package ahm

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestCollectRecordFilesSelectsAhmSourceRecords(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".ahm", ".tasks", "active", "001.md"), "# Task\n")
	writeFile(t, filepath.Join(root, ".ahm", ".tasks", "active", "index.md"), "# Generated\n")
	writeFile(t, filepath.Join(root, ".ahm", ".research", "topics", "storage.md"), "# Research\n")
	writeFile(t, filepath.Join(root, ".ahm", ".research", "index.md"), "# Generated\n")
	writeFile(t, filepath.Join(root, ".ahm", "exec-plans", "active", "plan.md"), "# Plan\n")
	writeFile(t, filepath.Join(root, ".ahm", "exec-plans", "active", "index.md"), "# Generated\n")
	writeFile(t, filepath.Join(root, ".ahm", "config.json"), "{}\n")
	writeFile(t, filepath.Join(root, ".agents", ".tasks", "active", "legacy.md"), "# Legacy\n")

	files, err := collectRecordFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, file := range files {
		got = append(got, file.RelPath)
	}
	want := []string{
		".ahm/.research/topics/storage.md",
		".ahm/.tasks/active/001.md",
		".ahm/exec-plans/active/plan.md",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("record files = %#v, want %#v", got, want)
	}
}

func TestCollectRecordFilesRejectsSymlinkedMarkdownRecords(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "outside.md"), "# Outside\n")
	if err := os.MkdirAll(filepath.Join(root, ".ahm", ".tasks", "active"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "outside.md"), filepath.Join(root, ".ahm", ".tasks", "active", "001.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "outside.md"), filepath.Join(root, ".ahm", ".tasks", "active", "ignored.txt")); err != nil {
		t.Fatal(err)
	}

	_, err := collectRecordFiles(root)
	if err == nil {
		t.Fatal("expected symlinked markdown record to be rejected")
	}
	assertContainsAll(t, err.Error(), "record file symlinks are not supported", ".ahm/.tasks/active/001.md")
}

func TestSnapshotRecordsRefCreatesPrivateRefWithoutMutatingProjectGitState(t *testing.T) {
	root := newGitRepo(t)
	writeFile(t, filepath.Join(root, ".gitignore"), ".ahm/*\n")
	writeFile(t, filepath.Join(root, "README.md"), "# Repo\n")
	git(t, root, "add", ".gitignore", "README.md")
	git(t, root, "commit", "-m", "initial")

	writeFile(t, filepath.Join(root, ".ahm", ".tasks", "active", "001.md"), "# Task\n")
	writeFile(t, filepath.Join(root, ".ahm", ".tasks", "active", "index.md"), "# Generated Task Index\n")
	writeFile(t, filepath.Join(root, ".ahm", ".research", "topics", "storage.md"), "# Research\n")
	writeFile(t, filepath.Join(root, ".ahm", ".research", "index.md"), "# Generated Research Index\n")
	writeFile(t, filepath.Join(root, ".ahm", "exec-plans", "active", "plan.md"), "# Plan\n")
	writeFile(t, filepath.Join(root, ".agents", ".tasks", "active", "legacy.md"), "# Legacy\n")

	writeFile(t, filepath.Join(root, "staged.txt"), "staged\n")
	git(t, root, "add", "staged.txt")

	before := gitState(t, root)
	snapshot, err := snapshotRecordsRef(context.Background(), root, recordsStorageConfig{Ref: defaultRecordsRef}, "test snapshot")
	if err != nil {
		t.Fatal(err)
	}
	after := gitState(t, root)
	assertSameProjectGitState(t, before, after)

	if snapshot.Commit == "" || snapshot.Tree == "" {
		t.Fatalf("empty snapshot identifiers: %#v", snapshot)
	}
	got := gitLines(t, root, "ls-tree", "-r", "--name-only", defaultRecordsRef)
	want := []string{
		".ahm/.research/topics/storage.md",
		".ahm/.tasks/active/001.md",
		".ahm/exec-plans/active/plan.md",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ref files = %#v, want %#v", got, want)
	}
}

func TestMaterializeRecordsRefWritesAhmFilesWithoutMutatingProjectGitState(t *testing.T) {
	root := newGitRepo(t)
	writeFile(t, filepath.Join(root, ".gitignore"), ".ahm/*\n")
	writeFile(t, filepath.Join(root, "README.md"), "# Repo\n")
	git(t, root, "add", ".gitignore", "README.md")
	git(t, root, "commit", "-m", "initial")

	writeFile(t, filepath.Join(root, ".ahm", ".tasks", "active", "001.md"), "# Task\n")
	writeFile(t, filepath.Join(root, ".ahm", ".research", "topics", "storage.md"), "# Research\n")
	writeFile(t, filepath.Join(root, ".ahm", "exec-plans", "active", "plan.md"), "# Plan\n")
	if _, err := snapshotRecordsRef(context.Background(), root, recordsStorageConfig{Ref: defaultRecordsRef}, "test snapshot"); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, ".ahm", ".tasks")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, ".ahm", ".research")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, ".ahm", "exec-plans")); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(root, "staged.txt"), "staged\n")
	git(t, root, "add", "staged.txt")
	before := gitState(t, root)
	written, err := materializeRecordsRef(context.Background(), root, defaultRecordsRef)
	if err != nil {
		t.Fatal(err)
	}
	after := gitState(t, root)
	assertSameProjectGitState(t, before, after)

	want := []string{
		".ahm/.research/topics/storage.md",
		".ahm/.tasks/active/001.md",
		".ahm/exec-plans/active/plan.md",
	}
	if !reflect.DeepEqual(written, want) {
		t.Fatalf("written = %#v, want %#v", written, want)
	}
	assertFileContainsAll(t, filepath.Join(root, ".ahm", ".tasks", "active", "001.md"), "# Task")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", ".research", "topics", "storage.md"), "# Research")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "exec-plans", "active", "plan.md"), "# Plan")
}

func TestCompareRecordsRefs(t *testing.T) {
	root := newGitRepo(t)
	writeFile(t, filepath.Join(root, "README.md"), "# Repo\n")
	git(t, root, "add", "README.md")
	git(t, root, "commit", "-m", "initial")

	writeFile(t, filepath.Join(root, ".ahm", ".tasks", "active", "001.md"), "# One\n")
	first, err := snapshotRecordsRef(context.Background(), root, recordsStorageConfig{Ref: "refs/ahm/left"}, "left")
	if err != nil {
		t.Fatal(err)
	}
	git(t, root, "update-ref", "refs/ahm/right", first.Commit)
	cmp, err := compareRecordsRefs(context.Background(), root, "refs/ahm/left", "refs/ahm/right")
	if err != nil {
		t.Fatal(err)
	}
	if cmp != recordsRefEqual {
		t.Fatalf("comparison = %q, want %q", cmp, recordsRefEqual)
	}

	writeFile(t, filepath.Join(root, ".ahm", ".tasks", "active", "002.md"), "# Two\n")
	if _, err := snapshotRecordsRef(context.Background(), root, recordsStorageConfig{Ref: "refs/ahm/left"}, "left again"); err != nil {
		t.Fatal(err)
	}
	cmp, err = compareRecordsRefs(context.Background(), root, "refs/ahm/left", "refs/ahm/right")
	if err != nil {
		t.Fatal(err)
	}
	if cmp != recordsRefAhead {
		t.Fatalf("comparison = %q, want %q", cmp, recordsRefAhead)
	}
	cmp, err = compareRecordsRefs(context.Background(), root, "refs/ahm/right", "refs/ahm/left")
	if err != nil {
		t.Fatal(err)
	}
	if cmp != recordsRefBehind {
		t.Fatalf("comparison = %q, want %q", cmp, recordsRefBehind)
	}
	cmp, err = compareRecordsRefs(context.Background(), root, "refs/ahm/missing", "refs/ahm/right")
	if err != nil {
		t.Fatal(err)
	}
	if cmp != recordsRefLeftMissing {
		t.Fatalf("comparison = %q, want %q", cmp, recordsRefLeftMissing)
	}
	cmp, err = compareRecordsRefs(context.Background(), root, "refs/ahm/left", "refs/ahm/missing")
	if err != nil {
		t.Fatal(err)
	}
	if cmp != recordsRefRightMissing {
		t.Fatalf("comparison = %q, want %q", cmp, recordsRefRightMissing)
	}
	writeFile(t, filepath.Join(root, ".ahm", ".tasks", "active", "003.md"), "# Three\n")
	if _, err := snapshotRecordsRef(context.Background(), root, recordsStorageConfig{Ref: "refs/ahm/right"}, "right again"); err != nil {
		t.Fatal(err)
	}
	cmp, err = compareRecordsRefs(context.Background(), root, "refs/ahm/left", "refs/ahm/right")
	if err != nil {
		t.Fatal(err)
	}
	if cmp != recordsRefDiverged {
		t.Fatalf("comparison = %q, want %q", cmp, recordsRefDiverged)
	}
}

func TestPushAndFetchRecordsRefUsePrivateRefWithoutMutatingProjectGitState(t *testing.T) {
	root := newGitRepo(t)
	writeFile(t, filepath.Join(root, "README.md"), "# Repo\n")
	git(t, root, "add", "README.md")
	git(t, root, "commit", "-m", "initial")
	writeFile(t, filepath.Join(root, ".ahm", ".tasks", "active", "001.md"), "# One\n")
	snapshot, err := snapshotRecordsRef(context.Background(), root, recordsStorageConfig{Ref: defaultRecordsRef}, "snapshot")
	if err != nil {
		t.Fatal(err)
	}

	remote := t.TempDir()
	git(t, remote, "init", "--bare", "-q")
	git(t, root, "remote", "add", "origin", remote)
	beforePush := gitState(t, root)
	if err := pushRecordsRef(context.Background(), root, recordsStorageConfig{Ref: defaultRecordsRef, Remote: "origin"}); err != nil {
		t.Fatal(err)
	}
	afterPush := gitState(t, root)
	assertSameProjectGitState(t, beforePush, afterPush)
	if got := strings.TrimSpace(git(t, remote, "rev-parse", defaultRecordsRef+"^{commit}")); got != snapshot.Commit {
		t.Fatalf("remote ref = %s, want %s", got, snapshot.Commit)
	}

	clone := newGitRepo(t)
	writeFile(t, filepath.Join(clone, "README.md"), "# Clone\n")
	git(t, clone, "add", "README.md")
	git(t, clone, "commit", "-m", "initial")
	git(t, clone, "remote", "add", "origin", remote)
	beforeFetch := gitState(t, clone)
	trackingRef, err := fetchRecordsRef(context.Background(), clone, recordsStorageConfig{Ref: defaultRecordsRef, Remote: "origin"})
	if err != nil {
		t.Fatal(err)
	}
	afterFetch := gitState(t, clone)
	assertSameProjectGitState(t, beforeFetch, afterFetch)
	if trackingRef != "refs/ahm/remotes/origin/records" {
		t.Fatalf("tracking ref = %q, want refs/ahm/remotes/origin/records", trackingRef)
	}
	if got := strings.TrimSpace(git(t, clone, "rev-parse", trackingRef+"^{commit}")); got != snapshot.Commit {
		t.Fatalf("fetched ref = %s, want %s", got, snapshot.Commit)
	}
	if _, err := resolveGitRef(context.Background(), clone, defaultRecordsRef); !errors.Is(err, errGitRefMissing) {
		t.Fatalf("local records ref should remain missing after fetch, got err %v", err)
	}
}

func TestRecordsRefValidationRejectsBranches(t *testing.T) {
	root := newGitRepo(t)
	err := validateRecordsRef(context.Background(), root, "refs/heads/main")
	if err == nil {
		t.Fatal("expected branch ref to be rejected")
	}
	assertContainsAll(t, err.Error(), "refs/ahm")
}

type projectGitState struct {
	Head      string
	Branch    string
	IndexData []byte
	Status    string
}

func gitState(t *testing.T, root string) projectGitState {
	t.Helper()
	indexData, err := os.ReadFile(filepath.Join(root, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	return projectGitState{
		Head:      strings.TrimSpace(git(t, root, "rev-parse", "HEAD")),
		Branch:    strings.TrimSpace(git(t, root, "branch", "--show-current")),
		IndexData: indexData,
		Status:    git(t, root, "status", "--short"),
	}
}

func assertSameProjectGitState(t *testing.T, before projectGitState, after projectGitState) {
	t.Helper()
	if before.Head != after.Head {
		t.Fatalf("HEAD changed from %s to %s", before.Head, after.Head)
	}
	if before.Branch != after.Branch {
		t.Fatalf("branch changed from %q to %q", before.Branch, after.Branch)
	}
	if !reflect.DeepEqual(before.IndexData, after.IndexData) {
		t.Fatal("git index bytes changed")
	}
	if before.Status != after.Status {
		t.Fatalf("git status changed:\nbefore:\n%s\nafter:\n%s", before.Status, after.Status)
	}
}

func newGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			t.Skip("git not available")
		}
		t.Fatal(err)
	}
	root := t.TempDir()
	git(t, root, "init", "-q", "-b", "main")
	git(t, root, "config", "user.name", "Test User")
	git(t, root, "config", "user.email", "test@example.com")
	return root
}

func git(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmdArgs := append([]string{"-C", root}, args...)
	cmd := exec.Command("git", cmdArgs...) // #nosec G204 // tests run git with explicit test arguments in a temp repo.
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func gitLines(t *testing.T, root string, args ...string) []string {
	t.Helper()
	out := strings.TrimSpace(git(t, root, args...))
	if out == "" {
		return nil
	}
	lines := strings.Split(out, "\n")
	sort.Strings(lines)
	return lines
}
