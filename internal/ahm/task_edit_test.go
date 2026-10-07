package ahm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeEditableTask writes a task record with a body rich enough to prove a
// section-scoped edit leaves the rest of the body alone.
func writeEditableTask(t *testing.T, root string, id string, title string, extraFrontMatter string, body string) string {
	t.Helper()
	path := filepath.Join(root, ".ahm", "tasks", "active", id+".md")
	content := "---\n" +
		"id: " + id + "\n" +
		"title: " + title + "\n" +
		"status: Pending\n" +
		"priority: P2\n" +
		"effort: S\n" +
		"labels: type:task, area:cli\n" +
		"depends_on: -\n" +
		extraFrontMatter +
		"---\n" +
		"# " + title + "\n\n" +
		strings.TrimSpace(body) + "\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeVocabularyTask writes a spare task record whose only purpose is to put
// the given labels into the corpus vocabulary, since `task edit --add-label`
// accepts only labels some record already carries.
func writeVocabularyTask(t *testing.T, root string, bucket string, id string, labels string) {
	t.Helper()
	path := filepath.Join(root, ".ahm", "tasks", bucket, id+".md")
	content := "---\n" +
		"id: " + id + "\n" +
		"title: Vocabulary seed\n" +
		"status: Pending\n" +
		"priority: P2\n" +
		"effort: S\n" +
		"labels: " + labels + "\n" +
		"depends_on: -\n" +
		"---\n" +
		"# Vocabulary seed\n\n" +
		"## Summary\n\nSeed.\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// runTaskEdit runs `task edit` through Main so flag parsing, exit codes, and
// output modes are all exercised the way a caller sees them.
func runTaskEdit(t *testing.T, root string, args ...string) (string, string, int) {
	t.Helper()
	return runCLI(t, append([]string{"--root", root, "task", "edit"}, args...)...)
}

func TestTaskEditUpdatesScalarFieldsInOneWrite(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "270", "Scoped edit", "", "## Summary\n\nTODO.\n")

	stdout, stderr, code := runTaskEdit(t, root, "270", "--priority", "P1", "--effort", "M")
	if code != 0 {
		t.Fatalf("edit exit code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
	if got := strings.TrimSpace(stdout); got != "270 updated (priority, effort)" {
		t.Errorf("stdout = %q, want %q", got, "270 updated (priority, effort)")
	}
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "270.md"),
		"priority: P1",
		"effort: M",
		"updated: ",
	)
}

func TestTaskEditSetsEveryScalarField(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "270", "Old title", "", "## Summary\n\nTODO.\n")

	stdout, stderr, code := runTaskEdit(t, root, "270",
		"--title", "New title",
		"--priority", "P3",
		"--effort", "L",
		"--external-ref", "gh#42")
	if code != 0 {
		t.Fatalf("edit exit code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
	if got := strings.TrimSpace(stdout); got != "270 updated (title, priority, effort, external_ref)" {
		t.Errorf("stdout = %q", got)
	}
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "270.md"),
		"title: New title",
		"priority: P3",
		"effort: L",
		"external_ref: gh#42",
		"# New title",
	)
}

func TestTaskEditClearsExternalRef(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "270", "Ref holder", "external_ref: gh#7\n", "## Summary\n\nTODO.\n")

	_, stderr, code := runTaskEdit(t, root, "270", "--external-ref", "")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	assertNotContains(t, mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "270.md")), "external_ref")
}

func TestTaskEditSetsAndClearsParent(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "001", "Parent", "", "## Summary\n\nTODO.\n")
	writeEditableTask(t, root, "002", "Child", "", "## Summary\n\nTODO.\n")

	_, stderr, code := runTaskEdit(t, root, "002", "--parent", "001")
	if code != 0 {
		t.Fatalf("set parent exit code = %d, stderr = %q", code, stderr)
	}
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "parent: 001")

	_, stderr, code = runTaskEdit(t, root, "002", "--clear-parent")
	if code != 0 {
		t.Fatalf("clear parent exit code = %d, stderr = %q", code, stderr)
	}
	assertNotContains(t, mustRead(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md")), "parent:")
}

func TestTaskEditParentMustBeTopLevel(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "001", "Parent", "", "## Summary\n\nTODO.\n")
	writeEditableTask(t, root, "001a", "Subtask", "", "## Summary\n\nTODO.\n")
	writeEditableTask(t, root, "002", "Child", "", "## Summary\n\nTODO.\n")

	_, stderr, code := runTaskEdit(t, root, "002", "--parent", "001a")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
	}
	assertContainsAll(t, stderr, "child task")
}

func TestTaskEditRejectsSelfParent(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "001", "Self parent", "", "## Summary\n\nTODO.\n")

	_, stderr, code := runTaskEdit(t, root, "001", "--parent", "001")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
	}
	assertContainsAll(t, stderr, "cannot be its own parent")
}

func TestTaskEditParentAndClearParentConflict(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "001", "Parent", "", "## Summary\n\nTODO.\n")

	_, stderr, code := runTaskEdit(t, root, "001", "--parent", "001", "--clear-parent")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
	}
	assertContainsAll(t, stderr, "or --clear-parent")
}

func TestTaskEditLabelsAddAndRemoveWithoutClobbering(t *testing.T) {
	root := projectRoot(t)
	writeVocabularyTask(t, root, "active", "271", "area:docs")
	path := writeEditableTask(t, root, "270", "Labelled", "", "## Summary\n\nTODO.\n")

	stdout, stderr, code := runTaskEdit(t, root, "270", "--add-label", "area:docs", "--remove-label", "area:cli")
	if code != 0 {
		t.Fatalf("exit code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
	if got := strings.TrimSpace(stdout); got != "270 updated (labels)" {
		t.Errorf("stdout = %q", got)
	}
	assertFileContainsAll(t, path, "type:task", "area:docs")
	assertNotContains(t, mustRead(t, path), "area:cli")
}

func TestTaskEditLabelsAcceptCommaListsAndRepeats(t *testing.T) {
	root := projectRoot(t)
	writeVocabularyTask(t, root, "active", "271", "area:docs, risk:external, type:feature")
	path := writeEditableTask(t, root, "270", "Labelled", "", "## Summary\n\nTODO.\n")

	_, stderr, code := runTaskEdit(t, root, "270",
		"--add-label", "area:docs,risk:external",
		"--add-label", "type:feature",
		"--remove-label", "area:cli,type:task")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	content := mustRead(t, path)
	assertContainsAll(t, content, "area:docs", "risk:external", "type:feature")
	assertNotContains(t, content, "area:cli", "type:task")
}

func TestTaskEditLabelRemovalOfAbsentLabelIsUnchanged(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Labelled", "", "## Summary\n\nTODO.\n")
	before := mustRead(t, path)

	stdout, stderr, code := runTaskEdit(t, root, "270", "--remove-label", "area:nope")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got := strings.TrimSpace(stdout); got != "270 unchanged" {
		t.Errorf("stdout = %q, want %q", got, "270 unchanged")
	}
	if after := mustRead(t, path); after != before {
		t.Errorf("record changed on the unchanged path:\nbefore: %s\nafter: %s", before, after)
	}
}

func TestTaskEditRejectsInvalidLabels(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "270", "Labelled", "", "## Summary\n\nTODO.\n")

	cases := map[string][]string{
		"empty":       {"--add-label", ""},
		"whitespace":  {"--add-label", " area:docs"},
		"newline":     {"--add-label", "area:docs\ntype:bug"},
		"emptyRemove": {"--remove-label", ""},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			_, stderr, code := runTaskEdit(t, root, append([]string{"270"}, args...)...)
			if code != 2 {
				t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
			}
		})
	}
}

func TestTaskEditAddLabelRejectsLabelOutsideVocabulary(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Labelled", "", "## Summary\n\nTODO.\n")
	before := mustRead(t, path)

	stdout, stderr, code := runTaskEdit(t, root, "270", "--add-label", "area:nope")
	if code != 2 {
		t.Fatalf("exit code = %d, stdout = %q, stderr = %q, want 2", code, stdout, stderr)
	}
	assertContainsAll(t, stderr, "area:nope", "ahm task labels", "task create --labels")
	if after := mustRead(t, path); after != before {
		t.Errorf("record changed on the rejected path:\nbefore: %s\nafter: %s", before, after)
	}
}

func TestTaskEditAddLabelAllowedWhenVocabularyIsEmpty(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Unlabelled", "", "## Summary\n\nTODO.\n")
	// Strip the fixture's labels so the whole corpus carries none, which is the
	// one case where there is no vocabulary to check an addition against.
	content := mustRead(t, path)
	content = strings.Replace(content, "labels: type:task, area:cli", "labels: -", 1)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runTaskEdit(t, root, "270", "--add-label", "area:docs")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q, want 0", code, stderr)
	}
	assertFileContainsAll(t, path, "area:docs")
}

func TestTaskEditAddLabelAcceptsLabelIntroducedByCreate(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "270", "Target", "", "## Summary\n\nTODO.\n")

	if _, stderr, code := runCLI(t, "--root", root, "task", "create", "Seeded", "--labels", "area:special"); code != 0 {
		t.Fatalf("create exit code = %d, stderr = %q", code, stderr)
	}
	if _, stderr, code := runTaskEdit(t, root, "270", "--add-label", "area:special"); code != 0 {
		t.Fatalf("edit exit code = %d, stderr = %q", code, stderr)
	}
}

func TestTaskEditDryRunRejectsLabelOutsideVocabulary(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Labelled", "", "## Summary\n\nTODO.\n")
	before := mustRead(t, path)

	_, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "edit", "270", "--add-label", "area:nope")
	if code != 2 {
		t.Fatalf("exit code = %d, stderr = %q, want 2", code, stderr)
	}
	if after := mustRead(t, path); after != before {
		t.Errorf("record changed in dry run:\nbefore: %s\nafter: %s", before, after)
	}
}

func TestTaskEditForceDoesNotBypassLabelVocabulary(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Labelled", "", "## Summary\n\nTODO.\n")
	before := mustRead(t, path)

	_, stderr, code := runCLI(t, "--root", root, "--force", "task", "edit", "270", "--add-label", "area:nope")
	if code != 2 {
		t.Fatalf("exit code = %d, stderr = %q, want 2", code, stderr)
	}
	if after := mustRead(t, path); after != before {
		t.Errorf("record changed on the rejected path:\nbefore: %s\nafter: %s", before, after)
	}
}

func TestTaskEditAddLabelVocabularySpansAllBuckets(t *testing.T) {
	root := projectRoot(t)
	writeVocabularyTask(t, root, "completed", "900", "area:docs")
	path := writeEditableTask(t, root, "270", "Target", "", "## Summary\n\nTODO.\n")

	_, stderr, code := runTaskEdit(t, root, "270", "--add-label", "area:docs")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q, want 0", code, stderr)
	}
	assertFileContainsAll(t, path, "area:docs")
}

func TestTaskEditRejectedLabelLeavesOtherFieldsUnwritten(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Labelled", "", "## Summary\n\nTODO.\n")
	before := mustRead(t, path)

	_, stderr, code := runTaskEdit(t, root, "270", "--title", "Renamed", "--add-label", "area:nope")
	if code != 2 {
		t.Fatalf("exit code = %d, stderr = %q, want 2", code, stderr)
	}
	if after := mustRead(t, path); after != before {
		t.Errorf("record changed despite a rejected label:\nbefore: %s\nafter: %s", before, after)
	}
}

func TestTaskEditSectionReplacesOneSectionAndLeavesOthersIntact(t *testing.T) {
	root := projectRoot(t)
	body := "## Problem\n\nSomething is wrong.\n\n## Fix Direction\n\nChange the thing.\n"
	path := writeEditableTask(t, root, "270", "Sectioned", "", body)

	stdout, stderr, code := runTaskEdit(t, root, "270", "--section", "Fix Direction", "--body", "Change the other thing.")
	if code != 0 {
		t.Fatalf("exit code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
	if got := strings.TrimSpace(stdout); got != "270 updated (body:Fix Direction)" {
		t.Errorf("stdout = %q", got)
	}
	content := mustRead(t, path)
	assertContainsAll(t, content,
		"## Problem\n\nSomething is wrong.",
		"## Fix Direction\n\nChange the other thing.",
	)
	assertNotContains(t, content, "Change the thing.")
}

func TestTaskEditSectionCreatesMissingSection(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Sectioned", "", "## Problem\n\nSomething is wrong.\n")

	_, stderr, code := runTaskEdit(t, root, "270", "--section", "Design", "--body", "Do it this way.")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	assertFileContainsAll(t, path, "## Problem\n\nSomething is wrong.", "## Design\n\nDo it this way.")
}

func TestTaskEditSectionRequiresBody(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "270", "Sectioned", "", "## Problem\n\nSomething is wrong.\n")

	_, stderr, code := runTaskEdit(t, root, "270", "--section", "Problem")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
	}
	assertContainsAll(t, stderr, "--section requires --body or --body-file")
}

func TestTaskEditRefusesProtectedSections(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "270", "Protected", "", "## Summary\n\nTODO.\n")

	cases := map[string]struct {
		section string
		want    string
	}{
		"comments":            {section: "Comments", want: "task comment"},
		"cancellation reason": {section: "Cancellation Reason", want: "task cancel"},
		"case insensitive":    {section: "comments", want: "task comment"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, stderr, code := runTaskEdit(t, root, "270", "--section", tc.section, "--body", "anything")
			if code != 2 {
				t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
			}
			assertContainsAll(t, stderr, tc.want)
		})
	}
}

func TestTaskEditBodyReplacementDroppingCommentsIsRefused(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Commented",
		"", "## Summary\n\nTODO.\n\n## Comments\n\n**2026-06-24T18:30:00Z** — Observed.\n")

	_, stderr, code := runTaskEdit(t, root, "270", "--body", "## Summary\n\nRewritten.")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
	}
	assertContainsAll(t, stderr, "## Comments", "--force")
	assertFileContainsAll(t, path, "## Comments", "Observed.")
}

func TestTaskEditBodyReplacementDroppingCommentsSucceedsWithForce(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Commented",
		"", "## Summary\n\nTODO.\n\n## Comments\n\n**2026-06-24T18:30:00Z** — Observed.\n")

	_, stderr, code := runCLI(t, "--root", root, "--force", "task", "edit", "270", "--body", "## Summary\n\nRewritten.")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	assertFileContainsAll(t, path, "## Summary\n\nRewritten.")
	assertNotContains(t, mustRead(t, path), "## Comments")
}

func TestTaskEditBodyReplacementKeepingCommentsIsAllowed(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Commented",
		"", "## Summary\n\nTODO.\n\n## Comments\n\n**2026-06-24T18:30:00Z** — Observed.\n")

	replacement := "## Summary\n\nRewritten.\n\n## Comments\n\n**2026-06-24T18:30:00Z** — Observed.\n"
	_, stderr, code := runTaskEdit(t, root, "270", "--body", replacement)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	assertFileContainsAll(t, path, "## Summary\n\nRewritten.", "## Comments", "Observed.")
}

func TestTaskEditBodyAndBodyFileConflict(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "270", "Conflicted", "", "## Summary\n\nTODO.\n")

	bodyPath := filepath.Join(root, "body.md")
	if err := os.WriteFile(bodyPath, []byte("body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runTaskEdit(t, root, "270", "--body", "inline", "--body-file", bodyPath)
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
	}
	assertContainsAll(t, stderr, "not both")
}

func TestTaskEditBodyFileReadsFile(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "From file", "", "## Summary\n\nTODO.\n")
	bodyPath := filepath.Join(root, "body.md")
	if err := os.WriteFile(bodyPath, []byte("## Summary\n\nFrom a file.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runTaskEdit(t, root, "270", "--body-file", bodyPath)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	assertFileContainsAll(t, path, "## Summary\n\nFrom a file.")
}

func TestTaskEditBodyFileReadsStdin(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "From stdin", "", "## Summary\n\nTODO.\n")

	var out, errOut strings.Builder
	a := app{opts: options{root: root}, out: &out, err: &errOut, in: strings.NewReader("## Summary\n\nFrom stdin.\n")}
	if err := a.detectRoot(); err != nil {
		t.Fatal(err)
	}
	if err := a.taskEdit(taskEditArgs{
		id:       "270",
		bodyFile: "-",
		set:      map[string]bool{"body-file": true},
	}); err != nil {
		t.Fatal(err)
	}
	assertFileContainsAll(t, path, "## Summary\n\nFrom stdin.")
}

func TestTaskEditWithNoFlagsIsAUsageError(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "270", "Unedited", "", "## Summary\n\nTODO.\n")

	_, stderr, code := runTaskEdit(t, root, "270")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
	}
	assertContainsAll(t, stderr, "--title", "--priority", "--effort", "--add-label", "--remove-label", "--body", "--section")
}

func TestTaskEditDoesNotReadEditorEnvironment(t *testing.T) {
	// No code path spawns an editor, so the check is that the package source
	// never names the editor environment variables at all. A test asserting on
	// output could not fail, because nothing would print them either way.
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		text := string(source)
		for _, variable := range []string{"EDITOR", "VISUAL"} {
			// The declaration comment naming the variables is the one allowed
			// mention; any use of one is a launch of an external editor.
			for _, line := range strings.Split(text, "\n") {
				if !strings.Contains(line, variable) {
					continue
				}
				if strings.HasPrefix(strings.TrimSpace(line), "//") {
					continue
				}
				t.Errorf("%s references %s outside a comment: %s", name, variable, strings.TrimSpace(line))
			}
		}
	}
}

func TestTaskEditMatchingValuePrintsUnchangedAndWritesNothing(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Stable", "", "## Summary\n\nTODO.\n")
	before := mustRead(t, path)

	stdout, stderr, code := runTaskEdit(t, root, "270", "--priority", "P2", "--effort", "S")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got := strings.TrimSpace(stdout); got != "270 unchanged" {
		t.Errorf("stdout = %q, want %q", got, "270 unchanged")
	}
	if after := mustRead(t, path); after != before {
		t.Errorf("record changed on the unchanged path:\nbefore: %s\nafter: %s", before, after)
	}
}

func TestTaskEditDryRunPrintsPathAndDiffsWithoutWriting(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Preview", "", "## Summary\n\nTODO.\n")
	before := mustRead(t, path)

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "edit", "270", "--priority", "P1", "--effort", "L")
	if code != 0 {
		t.Fatalf("exit code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
	assertContainsAll(t, stdout,
		"270 edit: ",
		"270.md",
		"270 priority: P2 -> P1",
		"270 effort: S -> L",
	)
	if after := mustRead(t, path); after != before {
		t.Errorf("dry run wrote the record:\nbefore: %s\nafter: %s", before, after)
	}
}

func TestTaskEditJSONCarriesTaskAndChangedFields(t *testing.T) {
	root := projectRoot(t)
	writeVocabularyTask(t, root, "active", "271", "area:docs")
	writeEditableTask(t, root, "270", "Structured", "", "## Summary\n\nTODO.\n")

	stdout, stderr, code := runCLI(t, "--root", root, "--json", "task", "edit", "270", "--priority", "P1", "--add-label", "area:docs")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	var payload struct {
		ID      string   `json:"id"`
		Path    string   `json:"path"`
		Updated bool     `json:"updated"`
		Changed []string `json:"changed"`
		Task    struct {
			Priority string `json:"priority"`
			Labels   string `json:"labels"`
			Status   string `json:"status"`
		} `json:"task"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("decoding %q: %v", stdout, err)
	}
	if payload.ID != "270" {
		t.Errorf("id = %q, want 270", payload.ID)
	}
	if !payload.Updated {
		t.Errorf("updated = false, want true")
	}
	if strings.Join(payload.Changed, ",") != "priority,labels" {
		t.Errorf("changed = %v, want [priority labels]", payload.Changed)
	}
	if payload.Task.Priority != "P1" {
		t.Errorf("task.priority = %q, want P1", payload.Task.Priority)
	}
	if !strings.Contains(payload.Task.Labels, "area:docs") {
		t.Errorf("task.labels = %q, want it to contain area:docs", payload.Task.Labels)
	}
	if payload.Task.Status != "Pending" {
		t.Errorf("task.status = %q, want Pending unchanged", payload.Task.Status)
	}
}

func TestTaskEditJSONUnchangedReportsEmptyArray(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "270", "Stable", "", "## Summary\n\nTODO.\n")

	stdout, stderr, code := runCLI(t, "--root", root, "--json", "task", "edit", "270", "--priority", "P2")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, `"changed": []`) {
		t.Errorf("stdout = %q, want an empty changed array", stdout)
	}
	if !strings.Contains(stdout, `"updated": false`) {
		t.Errorf("stdout = %q, want updated false", stdout)
	}
}

func TestTaskEditHasNoStatusOrDependencyFlags(t *testing.T) {
	var a app
	cmd := a.taskEditCommand()
	for _, flag := range []string{"status", "depends-on", "labels"} {
		if cmd.Flags().Lookup(flag) != nil {
			t.Errorf("task edit registers --%s; status and dependencies belong to their own commands", flag)
		}
	}
	// Status and dependency commands keep their own behavior and guards.
	root := projectRoot(t)
	writeEditableTask(t, root, "001", "Dependency", "", "## Summary\n\nTODO.\n")
	writeEditableTask(t, root, "002", "Dependent", "", "## Summary\n\nTODO.\n")
	if _, stderr, code := runCLI(t, "--root", root, "task", "dep", "add", "002", "001"); code != 0 {
		t.Fatalf("dep add exit code = %d, stderr = %q", code, stderr)
	}
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "002.md"), "depends_on: 001")
}

func TestTaskEditRejectsBadEnumValues(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "270", "Enums", "", "## Summary\n\nTODO.\n")

	for _, flag := range []string{"--priority", "--effort"} {
		t.Run(flag, func(t *testing.T) {
			_, stderr, code := runTaskEdit(t, root, "270", flag, "nope")
			if code != 2 {
				t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
			}
			assertContainsAll(t, stderr, "unsupported task")
		})
	}
}

func TestTaskEditRejectsNewlinesInFrontMatterFields(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "270", "Newlines", "", "## Summary\n\nTODO.\n")

	cases := map[string][]string{
		"title":       {"--title", "One\nTwo"},
		"externalRef": {"--external-ref", "gh#1\ngh#2"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			_, stderr, code := runTaskEdit(t, root, append([]string{"270"}, args...)...)
			if code != 2 {
				t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
			}
		})
	}
}

func TestTaskEditNeverMovesRecordsBetweenBuckets(t *testing.T) {
	root := projectRoot(t)
	completed := filepath.Join(root, ".ahm", "tasks", "completed", "300.md")
	writeTaskFile(t, completed, "300", "Finished", "Completed", "depends_on: -\n")

	_, stderr, code := runTaskEdit(t, root, "300", "--priority", "P0")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	assertFileContainsAll(t, completed, "priority: P0", "status: Completed")
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "active", "300.md")); !os.IsNotExist(err) {
		t.Errorf("record moved to active, err = %v", err)
	}
}

func TestTaskEditPreservesUnknownFrontMatter(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Preserved",
		"custom_field: keep me\n", "## Summary\n\nTODO.\n")

	_, stderr, code := runTaskEdit(t, root, "270", "--priority", "P1")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	assertFileContainsAll(t, path, "custom_field: keep me", "priority: P1")
}

func TestTaskEditRegeneratesIndexes(t *testing.T) {
	root := projectRoot(t)
	setupAhmRepo(t, root)
	path := writeEditableTask(t, root, "270", "Indexed", "", "## Summary\n\nTODO.\n")

	_, stderr, code := runTaskEdit(t, root, "270", "--priority", "P1")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	assertFileContainsAll(t, path, "priority: P1")
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "index.md"), "Indexed")
}

func TestTaskEditStampsUpdatedTimestamp(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Stamped", "", "## Summary\n\nTODO.\n")

	before := time.Now().Add(-time.Second)
	_, stderr, code := runTaskEdit(t, root, "270", "--priority", "P1")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	task, err := parseTask(path, "active")
	if err != nil {
		t.Fatal(err)
	}
	stamped, err := time.Parse(time.RFC3339, task.Updated)
	if err != nil {
		t.Fatalf("parsing updated %q: %v", task.Updated, err)
	}
	if stamped.Before(before) {
		t.Errorf("updated = %s, want at or after %s", task.Updated, before)
	}
}

func TestTaskEditPreservesConcurrentUpdateLandingBeforeLock(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Concurrent", "", "## Summary\n\nTODO.\n")

	original := taskEditPreLockHook
	defer func() { taskEditPreLockHook = original }()

	// Land a competing write on the same record after this edit has validated
	// its flags but before it takes the lock. The edit re-resolves under the
	// lock, so the competing field must survive alongside the edit's own.
	var out strings.Builder
	a := app{opts: options{root: root}, out: &out}
	if err := a.detectRoot(); err != nil {
		t.Fatal(err)
	}
	taskEditPreLockHook = func() {
		a.invalidateTasks()
		tasks, err := a.getTasks()
		if err != nil {
			t.Error(err)
			return
		}
		task, err := resolveTaskFromTasks("270", tasks)
		if err != nil {
			t.Error(err)
			return
		}
		task.Title = "Renamed concurrently"
		if err := writeOwned(a.workflowPaths(), task.Path, []byte(renderTask(task))); err != nil {
			t.Error(err)
		}
		a.invalidateTasks()
	}
	if err := a.taskEdit(taskEditArgs{
		id:     "270",
		effort: "M",
		set:    map[string]bool{"effort": true},
	}); err != nil {
		t.Fatal(err)
	}
	assertFileContainsAll(t, path, "effort: M", "title: Renamed concurrently")
}

func TestTaskEditResolvesTargetByIDRules(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "270", "Exact", "", "## Summary\n\nTODO.\n")
	writeEditableTask(t, root, "270a", "Prefix sibling", "", "## Summary\n\nTODO.\n")

	t.Run("exact match wins over a prefix sibling", func(t *testing.T) {
		if _, stderr, code := runTaskEdit(t, root, "270", "--priority", "P1"); code != 0 {
			t.Fatalf("exit code = %d, stderr = %q", code, stderr)
		}
		assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "270.md"), "priority: P1")
		assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "270a.md"), "priority: P2")
	})

	t.Run("unpadded id resolves to the zero-padded record", func(t *testing.T) {
		writeEditableTask(t, root, "071", "Padded child", "", "## Summary\n\nTODO.\n")
		writeEditableTask(t, root, "072", "Padded numeric", "", "## Summary\n\nTODO.\n")
		// "72" matches no record exactly and no child uniquely, so it is
		// reported rather than guessed.
		if _, stderr, code := runTaskEdit(t, root, "072a", "--priority", "P0"); code != 1 {
			t.Errorf("exit code = %d, stderr = %q", code, stderr)
		}
		// The unpadded pattern resolves to the zero-padded record.
		if _, stderr, code := runTaskEdit(t, root, "72", "--priority", "P0"); code != 0 {
			t.Fatalf("exit code = %d, stderr = %q", code, stderr)
		}
		assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "072.md"), "priority: P0")
		assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", "071.md"), "priority: P2")
	})

	t.Run("ambiguous prefix is refused", func(t *testing.T) {
		writeEditableTask(t, root, "280a", "First child", "", "## Summary\n\nTODO.\n")
		writeEditableTask(t, root, "280b", "Second child", "", "## Summary\n\nTODO.\n")
		_, stderr, code := runTaskEdit(t, root, "280", "--priority", "P0")
		if code != 1 {
			t.Errorf("exit code = %d, stderr = %q, want 1", code, stderr)
		}
		assertContainsAll(t, stderr, "ambiguous", "280a", "280b")
	})
}

func TestTaskEditReportsNotFound(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "270", "Present", "", "## Summary\n\nTODO.\n")

	_, stderr, code := runTaskEdit(t, root, "999", "--priority", "P1")
	if code != 1 {
		t.Errorf("exit code = %d, stderr = %q, want 1", code, stderr)
	}
	assertContainsAll(t, stderr, "not found")
}

func TestTaskEditDryRunDoesNotWrite(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Untouched", "", "## Summary\n\nTODO.\n")
	before := mustRead(t, path)

	_, stderr, code := runCLI(t, "--root", root, "--dry-run", "task", "edit", "270", "--section", "Summary", "--body", "Replaced.")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if after := mustRead(t, path); after != before {
		t.Errorf("dry run wrote the record:\nbefore: %s\nafter: %s", before, after)
	}
}

func TestTaskEditStripsDuplicateH1FromWholeBody(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Headed", "", "## Summary\n\nTODO.\n")

	_, stderr, code := runTaskEdit(t, root, "270", "--title", "Renamed", "--body", "# Renamed\n\n## Summary\n\nRewritten.")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	content := mustRead(t, path)
	if strings.Count(content, "# Renamed") != 1 {
		t.Errorf("H1 duplicated or missing:\n%s", content)
	}
	assertFileContainsAll(t, path, "## Summary\n\nRewritten.")
}

func TestTaskEditRejectsDuplicateIDs(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "270", "First", "", "## Summary\n\nTODO.\n")
	// The same ID in another bucket is the duplicate case resolveTaskForMutation
	// guards: an edit must not guess which record the caller meant.
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "completed", "270.md"), "270", "Second", "Completed", "depends_on: -\n")

	_, stderr, code := runTaskEdit(t, root, "270", "--priority", "P1")
	if code != 1 {
		t.Errorf("exit code = %d, stderr = %q, want 1", code, stderr)
	}
	assertContainsAll(t, stderr, "duplicated")
}

func TestTaskEditRejectsEmptyTitle(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "270", "Titled", "", "## Summary\n\nTODO.\n")

	_, stderr, code := runTaskEdit(t, root, "270", "--title", "")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
	}
	assertContainsAll(t, stderr, "--title cannot be empty")
}
func TestTaskCreateBodyFlagSetsFullBody(t *testing.T) {
	root := projectRoot(t)
	stdout, stderr, code := runCLI(t, "--root", root, "task", "create", "Inline body",
		"--body", "## Summary\n\nWritten inline.\n\n## Acceptance Notes\n\n- [ ] done")
	if code != 0 {
		t.Fatalf("exit code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
	path := filepath.Join(root, ".ahm", "tasks", "active", strings.TrimSpace(stdout)+".md")
	assertFileContainsAll(t, path, "## Summary\n\nWritten inline.", "## Acceptance Notes", "- [ ] done")
	assertNotContains(t, mustRead(t, path), "TODO.")
}

func TestTaskCreateExternalRefFlag(t *testing.T) {
	root := projectRoot(t)
	stdout, stderr, code := runCLI(t, "--root", root, "task", "create", "Referenced", "--external-ref", "gh#99")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	assertFileContainsAll(t, filepath.Join(root, ".ahm", "tasks", "active", strings.TrimSpace(stdout)+".md"),
		"external_ref: gh#99")
}

func TestTaskCreateBodyConflicts(t *testing.T) {
	root := projectRoot(t)
	bodyPath := filepath.Join(root, "body.md")
	if err := os.WriteFile(bodyPath, []byte("## Summary\n\nFrom file.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := map[string][]string{
		"body and body-file":   {"--body", "inline", "--body-file", bodyPath},
		"body and description": {"--body", "inline", "--description", "summary"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			_, stderr, code := runCLI(t, append([]string{"--root", root, "task", "create", "Conflict"}, args...)...)
			if code != 2 {
				t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
			}
			assertContainsAll(t, stderr, "not both")
		})
	}
}

func TestTaskCreateRejectsNewlineExternalRef(t *testing.T) {
	root := projectRoot(t)
	_, stderr, code := runCLI(t, "--root", root, "task", "create", "Bad ref", "--external-ref", "a\nb")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
	}
	assertContainsAll(t, stderr, "external reference must not contain newlines")
}

func TestTaskEditHelpDocumentsFlags(t *testing.T) {
	root := projectRoot(t)
	stdout, _, code := runCLI(t, "--root", root, "task", "edit", "--help")
	if code != 0 {
		t.Fatalf("help exit code = %d", code)
	}
	assertContainsAll(t, stdout,
		"--title", "--priority", "--effort", "--add-label", "--remove-label",
		"--external-ref", "--parent", "--clear-parent", "--body", "--body-file", "--section",
	)
}

func TestTaskEditBodyReplacementDroppingCommentsContentIsRefused(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Commented",
		"", "## Summary\n\nTODO.\n\n## Comments\n\n**2026-06-24T18:30:00Z** — Observed.\n")

	// Keeping the heading while emptying it still deletes the comment log, so
	// the guard judges the section's content, not just its presence.
	_, stderr, code := runTaskEdit(t, root, "270", "--body", "## Summary\n\nRewritten.\n\n## Comments\n")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
	}
	assertContainsAll(t, stderr, "## Comments")
	assertFileContainsAll(t, path, "Observed.")
}

func TestTaskEditIdenticalBodyIsUnchanged(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Stable", "", "## Summary\n\nTODO.\n")
	body := "## Summary\n\nTODO.\n"
	before := mustRead(t, path)

	stdout, stderr, code := runTaskEdit(t, root, "270", "--body", body)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got := strings.TrimSpace(stdout); got != "270 unchanged" {
		t.Errorf("stdout = %q, want %q", got, "270 unchanged")
	}
	if after := mustRead(t, path); after != before {
		t.Errorf("record changed on the unchanged path:\nbefore: %s\nafter: %s", before, after)
	}
}

func TestTaskEditRejectsUnusableSectionNames(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "270", "Sectioned", "", "## Summary\n\nTODO.\n")

	cases := map[string][]string{
		"empty":         {"--section", ""},
		"whitespace":    {"--section", "   "},
		"hash prefix":   {"--section", "## Summary"},
		"newline":       {"--section", "Summary\nOther"},
		"hash anywhere": {"--section", "Fix #12"},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			_, stderr, code := runTaskEdit(t, root, append([]string{"270"}, append(extra, "--body", "text")...)...)
			if code != 2 {
				t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
			}
		})
	}
}

func TestTaskEditRejectsEmptyListSentinelAsLabel(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "270", "Labelled", "", "## Summary\n\nTODO.\n")

	for _, value := range []string{"-", "[]"} {
		t.Run(value, func(t *testing.T) {
			_, stderr, code := runTaskEdit(t, root, "270", "--add-label", value)
			if code != 2 {
				t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
			}
			assertContainsAll(t, stderr, "empty-list sentinel")
		})
	}
}

func TestTaskEditJSONPathMatchesTaskPath(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "270", "Structured", "", "## Summary\n\nTODO.\n")

	stdout, stderr, code := runCLI(t, "--root", root, "--json", "task", "edit", "270", "--priority", "P1")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	var editPayload struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(stdout), &editPayload); err != nil {
		t.Fatalf("decoding %q: %v", stdout, err)
	}
	// Every command renders a record path through the same helper, so the edit
	// payload has to agree with `task show`. Comparing `path` against the
	// payload's own `task.path` would be a tautology, since both are the same
	// expression; this is the assertion that actually pins the renderer.
	showStdout, stderr, code := runCLI(t, "--root", root, "--json", "task", "show", "270")
	if code != 0 {
		t.Fatalf("show exit code = %d, stderr = %q", code, stderr)
	}
	var showPayload struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(showStdout), &showPayload); err != nil {
		t.Fatalf("decoding %q: %v", showStdout, err)
	}
	if editPayload.Path != showPayload.Path {
		t.Errorf("task edit path = %q, task show path = %q, want the same rendering", editPayload.Path, showPayload.Path)
	}
	if !strings.Contains(filepath.ToSlash(editPayload.Path), "270.md") {
		t.Errorf("path = %q, want it to name the record file", editPayload.Path)
	}
}

func TestTaskEditBodyFileShorthandIsRegistered(t *testing.T) {
	var a app
	cmd := a.taskEditCommand()
	if cmd.Flags().ShorthandLookup("F") == nil {
		t.Error("task edit does not register the -F shorthand documented for --body-file")
	}
}

func TestTaskCreateEmptyBodyIsRefused(t *testing.T) {
	root := projectRoot(t)
	bodyPath := filepath.Join(root, "body.md")
	if err := os.WriteFile(bodyPath, []byte("## Summary\n\nFrom file.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// An explicit empty --body must be refused, not treated as an omission: it
	// would otherwise skip the exclusivity checks and write the default scaffold.
	cases := map[string][]string{
		"alone":            {"--body", ""},
		"with body-file":   {"--body", "", "--body-file", bodyPath},
		"with description": {"--body", "", "--description", "summary"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			_, stderr, code := runCLI(t, append([]string{"--root", root, "task", "create", "Empty body"}, args...)...)
			if code != 2 {
				t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
			}
		})
	}
	// No record was created by any of the refused invocations.
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "active", "001.md")); !os.IsNotExist(err) {
		t.Errorf("a refused create wrote a record, err = %v", err)
	}
}

func TestTaskEditBodyReplacementDemotingProtectedHeadingIsRefused(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Demoted",
		"", "## Summary\n\nTODO.\n\n## Comments\n\n**2026-06-24T18:30:00Z** — Observed.\n")

	// The content survives, but under a `###` heading that `task comment` does
	// not recognize, so the next comment opens a second section beside it.
	_, stderr, code := runTaskEdit(t, root, "270", "--body", "## Summary\n\nNew.\n\n### Comments\n\n**2026-06-24T18:30:00Z** — Observed.\n")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
	}
	assertContainsAll(t, stderr, "## Comments")
	assertFileContainsAll(t, path, "## Comments", "Observed.")
}

func TestTaskEditBodyReplacementPromotingProtectedHeadingIsRefused(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "270", "Promoted",
		"", "## Summary\n\nTODO.\n\n### Comments\n\n**2026-06-24T18:30:00Z** — Observed.\n")

	_, stderr, code := runTaskEdit(t, root, "270", "--body", "## Summary\n\nNew.\n\n## Comments\n\n**2026-06-24T18:30:00Z** — Observed.\n")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
	}
	assertContainsAll(t, stderr, "## Comments")
}

func TestTaskEditForceDoesNotOverrideProtectedSectionRefusal(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "270", "Protected", "", "## Summary\n\nTODO.\n")

	_, stderr, code := runCLI(t, "--root", root, "--force", "task", "edit", "270", "--section", "Comments", "--body", "forged")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
	}
	assertContainsAll(t, stderr, "task comment")
}

func TestTaskEditBodyReplacementDroppingCancellationReasonIsRefused(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Cancelled", "", "## Summary\n\nTODO.\n")

	// A cancellation reason is protected exactly like a comment log.
	_, stderr, code := runCLI(t, "--root", root, "task", "cancel", "270", "--reason", "Superseded by 169")
	if code != 0 {
		t.Fatalf("cancel exit code = %d, stderr = %q", code, stderr)
	}
	cancelled := filepath.Join(root, ".ahm", "tasks", "cancelled", "270.md")
	assertFileContainsAll(t, cancelled, "## Cancellation Reason", "Superseded by 169")

	_, stderr, code = runTaskEdit(t, root, "270", "--body", "## Summary\n\nRewritten.")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
	}
	assertContainsAll(t, stderr, "## Cancellation Reason")
	assertFileContainsAll(t, cancelled, "Superseded by 169")
	_ = path
}

func TestTaskEditBodyReplacementRewritingProtectedContentIsRefused(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Commented",
		"", "## Summary\n\nTODO.\n\n## Comments\n\n**2026-06-24T18:30:00Z** — Observed.\n")

	cases := map[string]string{
		"replaced text":   "## Summary\n\nNew.\n\n## Comments\n\n(see git history)\n",
		"nested heading":  "## Summary\n\nNew.\n\n## Comments\n\n### Comments\n\nObserved.\n",
		"different depth": "## Summary\n\nNew.\n\n## Comments\n\n### Notes\n\nObserved.\n",
		"comment removed": "## Summary\n\nNew.\n\n## Comments\n\n**2026-06-24T18:30:00Z** — Second.\n",
	}
	for name, replacement := range cases {
		t.Run(name, func(t *testing.T) {
			_, stderr, code := runTaskEdit(t, root, "270", "--body", replacement)
			if code != 2 {
				t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
			}
			assertContainsAll(t, stderr, "## Comments")
			assertFileContainsAll(t, path, "Observed.")
		})
	}
}

func TestTaskEditBodyReplacementRewrappedProtectedContentIsAllowed(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Commented",
		"", "## Summary\n\nTODO.\n\n## Comments\n\n**2026-06-24T18:30:00Z** — Observed.\n")

	// Re-wrapping a preserved section changes no content, so the guard allows it.
	replacement := "## Summary\n\nRewritten.\n\n## Comments\n\n**2026-06-24T18:30:00Z** —\nObserved.\n"
	_, stderr, code := runTaskEdit(t, root, "270", "--body", replacement)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	assertFileContainsAll(t, path, "## Summary\n\nRewritten.", "## Comments", "Observed.")
}

func TestTaskEditBodyReplacementInvertingProtectedTokenIsRefused(t *testing.T) {
	// `Obsolete` is a substring of `Not Obsolete`, so a substring check accepts
	// the exact inversion of the recorded reason. The same shape negates a
	// comment token, or appends to it with no whitespace boundary.
	cases := map[string]struct {
		body        string
		replacement string
		kept        string
		unwanted    string
	}{
		"negated cancellation reason": {
			body:        "## Summary\n\nTODO.\n\n## Cancellation Reason\n\nObsolete\n",
			replacement: "## Summary\n\nNew.\n\n## Cancellation Reason\n\nNot Obsolete: superseded by 170\n",
			kept:        "Obsolete",
			unwanted:    "Not Obsolete",
		},
		"comment token with a suffix appended": {
			body:        "## Summary\n\nTODO.\n\n## Comments\n\nObserved.\n",
			replacement: "## Summary\n\nNew.\n\n## Comments\n\nObserved.REVERTED, do not ship\n",
			kept:        "Observed.",
			unwanted:    "REVERTED",
		},
		"negated comment token": {
			body:        "## Summary\n\nTODO.\n\n## Comments\n\nLGTM\n",
			replacement: "## Summary\n\nNew.\n\n## Comments\n\nNot LGTM\n",
			kept:        "LGTM",
			unwanted:    "Not LGTM",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			root := projectRoot(t)
			path := writeEditableTask(t, root, "270", "Protected", "", tc.body)
			_, stderr, code := runTaskEdit(t, root, "270", "--body", tc.replacement)
			if code != 2 {
				t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
			}
			assertContainsAll(t, stderr, "--force")
			assertFileContainsAll(t, path, tc.kept)
			assertNotContains(t, mustRead(t, path), tc.unwanted)
		})
	}
}

func TestTaskEditBodyReplacementAppendingToProtectedLogIsAllowed(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Commented",
		"", "## Summary\n\nTODO.\n\n## Comments\n\n**2026-06-24T18:30:00Z** — real log\n")

	// Appending a new entry after the preserved log leaves the existing entries
	// intact on their token boundaries, so the guard allows it.
	replacement := "## Summary\n\nRewritten.\n\n## Comments\n\n**2026-06-24T18:30:00Z** — real log\n\n**2026-06-25T09:00:00Z** — another\n"
	_, stderr, code := runTaskEdit(t, root, "270", "--body", replacement)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	assertFileContainsAll(t, path, "## Summary\n\nRewritten.", "real log", "another")
}

func TestTaskEditBodyReplacementForgingACommentSectionAheadIsRefused(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Commented",
		"", "## Summary\n\nObserved.\n\n## Comments\n\n**2026-06-24T18:30:00Z** — real log\n")

	// `task comment` appends to the first `## Comments` heading, so a forged copy
	// placed ahead of the real log would capture every later comment even though
	// the log itself survives the edit.
	_, stderr, code := runTaskEdit(t, root, "270", "--body",
		"## Summary\n\nObserved.\n\n## Comments\n\n**2026-06-24T18:30:00Z** — forged\n\n## Comments\n\n**2026-06-24T18:30:00Z** — real log\n")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
	}
	assertContainsAll(t, stderr, "## Comments", "--force")
	assertFileContainsAll(t, path, "real log")
	assertNotContains(t, mustRead(t, path), "forged")
}

func TestTaskEditBodyReplacementEmptyingTheFirstCommentSectionIsRefused(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Commented",
		"", "## Summary\n\nObserved.\n\n## Comments\n\n**2026-06-24T18:30:00Z** — real log\n")

	// Moving the log under a second heading still leaves the emptied first one as
	// the section `task comment` writes to next.
	_, stderr, code := runTaskEdit(t, root, "270", "--body",
		"## Summary\n\nObserved.\n\n## Comments\n\n## Comments\n\n**2026-06-24T18:30:00Z** — real log\n")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
	}
	assertContainsAll(t, stderr, "## Comments")
	assertFileContainsAll(t, path, "real log")
}

func TestTaskEditBodyReplacementFillingAnEmptyCommentHeadingIsRefused(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Commented",
		"", "## Summary\n\nObserved.\n\n## Comments\n\n## Comments\n\n**2026-06-24T18:30:00Z** — real log\n")

	// The empty heading keeps its position, so filling it would move the write
	// target `task comment` appends to away from the real log.
	_, stderr, code := runTaskEdit(t, root, "270", "--body",
		"## Summary\n\nObserved.\n\n## Comments\n\n**2026-06-24T18:30:00Z** — forged\n\n## Comments\n\n**2026-06-24T18:30:00Z** — real log\n")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
	}
	assertContainsAll(t, stderr, "## Comments", "--force")
	assertNotContains(t, mustRead(t, path), "forged")
}

func TestTaskEditBodyReplacementBesideAnEmptyCommentHeadingIsAllowed(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Commented",
		"", "## Summary\n\nObserved.\n\n## Comments\n\n## Comments\n\n**2026-06-24T18:30:00Z** — real log\n")

	// Leaving the protected sections exactly as they are keeps the pairing
	// intact, so an unrelated section replacement is allowed.
	_, stderr, code := runTaskEdit(t, root, "270", "--section", "Summary", "--body", "Now fixed.")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	assertFileContainsAll(t, path, "## Summary\n\nNow fixed.", "real log")
}

func TestTaskEditBodyReplacementMergingCommentSectionsIsRefused(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Commented",
		"", "## Summary\n\nObserved.\n\n## Comments\n\nfirst log\n\n## Comments\n\nsecond log\n")
	merged := "## Summary\n\nObserved.\n\n## Comments\n\nfirst log\n\nsecond log\n"

	// Merging two logs leaves the second with nothing to pair with, so the result
	// is refused. `--force` is the deliberate override.
	_, stderr, code := runTaskEdit(t, root, "270", "--body", merged)
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
	}
	assertContainsAll(t, stderr, "## Comments", "--force")

	_, stderr, code = runCLI(t, "--root", root, "--force", "task", "edit", "270", "--body", merged)
	if code != 0 {
		t.Fatalf("force exit code = %d, stderr = %q", code, stderr)
	}
	assertFileContainsAll(t, path, "first log", "second log")
}

func TestTaskEditBodyReplacementMovingTheCommentLogIsAllowed(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Commented",
		"", "## Summary\n\nObserved.\n\n## Comments\n\n**2026-06-24T18:30:00Z** — real log\n")

	// The same single section, re-wrapped, re-indented, and moved ahead of
	// Summary: positional pairing still carries it.
	replacement := "## Comments\n\n  **2026-06-24T18:30:00Z** —\n  real log\n\n## Summary\n\nObserved.\n"
	_, stderr, code := runTaskEdit(t, root, "270", "--body", replacement)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	assertFileContainsAll(t, path, "## Comments", "real log", "## Summary")
}

func TestTaskEditBodyReplacementAddingACommentSectionAfterIsAllowed(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Commented",
		"", "## Summary\n\nObserved.\n\n## Comments\n\n**2026-06-24T18:30:00Z** — real log\n")

	// An added protected section after the one being carried is not a drop: the
	// guard protects existing provenance, it does not police new headings.
	replacement := "## Summary\n\nObserved.\n\n## Comments\n\n**2026-06-24T18:30:00Z** — real log\n\n## Comments\n\nnew section\n"
	_, stderr, code := runTaskEdit(t, root, "270", "--body", replacement)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	assertFileContainsAll(t, path, "## Summary\n\nObserved.", "real log", "new section")
}

func TestTaskEditSectionIntroducingAProtectedHeadingAheadIsRefused(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Commented",
		"", "## Summary\n\nObserved.\n\n## Notes\n\nOld.\n\n## Comments\n\n**2026-06-24T18:30:00Z** — real log\n")

	// The replacement lands before the real log, so it would become the section
	// `task comment` writes to next.
	_, stderr, code := runTaskEdit(t, root, "270", "--section", "Notes", "--body",
		"New.\n\n## Comments\n\n**2026-06-24T18:30:00Z** — forged\n")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
	}
	assertContainsAll(t, stderr, "## Comments")
	assertFileContainsAll(t, path, "real log", "Old.")
}

func TestTaskEditSectionIntroducingAProtectedHeadingAfterIsAllowed(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Commented",
		"", "## Summary\n\nObserved.\n\n## Comments\n\n**2026-06-24T18:30:00Z** — real log\n\n## Notes\n\nOld.\n")

	// The introduced heading lands after the real log, so the log stays the
	// section `task comment` writes to and the new heading is inert.
	_, stderr, code := runTaskEdit(t, root, "270", "--section", "Notes", "--body",
		"New.\n\n## Comments\n\n**2026-06-24T18:30:00Z** — later\n")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	assertFileContainsAll(t, path, "## Summary\n\nObserved.", "real log", "later", "New.")
}

func TestTaskEditSectionNoOpReportsUnchanged(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Sectioned", "", "## Summary\n\nOriginal.\n")
	before := mustRead(t, path)

	stdout, stderr, code := runTaskEdit(t, root, "270", "--section", "Summary", "--body", "Original.")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got := strings.TrimSpace(stdout); got != "270 unchanged" {
		t.Errorf("stdout = %q, want %q", got, "270 unchanged")
	}
	if after := mustRead(t, path); after != before {
		t.Errorf("record changed on the unchanged path:\nbefore: %s\nafter: %s", before, after)
	}
}

func TestTaskEditLabelNoOpOnNonCanonicalStoredValueIsUnchanged(t *testing.T) {
	root := projectRoot(t)
	// `task create --labels` writes the raw flag value, so a record can hold
	// `type:task,area:cli` rather than the canonical spaced form.
	path := writeEditableTask(t, root, "270", "Labelled", "", "## Summary\n\nTODO.\n")
	content := strings.Replace(mustRead(t, path), "labels: type:task, area:cli", "labels: type:task,area:cli", 1)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	before := mustRead(t, path)

	stdout, stderr, code := runTaskEdit(t, root, "270", "--add-label", "area:cli")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	if got := strings.TrimSpace(stdout); got != "270 unchanged" {
		t.Errorf("stdout = %q, want %q", got, "270 unchanged")
	}
	if after := mustRead(t, path); after != before {
		t.Errorf("record changed on the unchanged path:\nbefore: %s\nafter: %s", before, after)
	}
}

func TestTaskEditRejectsEmptyBodyFile(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "270", "Conflicted", "", "## Summary\n\nTODO.\n")

	_, stderr, code := runTaskEdit(t, root, "270", "--body-file", "")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
	}
	assertContainsAll(t, stderr, "--body-file cannot be empty")
}

func TestTaskCreateEmptyBodyFileIsRefused(t *testing.T) {
	root := projectRoot(t)
	cases := map[string][]string{
		"alone":            {"--body-file", ""},
		"with description": {"--body-file", "", "--description", "summary"},
		"with body":        {"--body-file", "", "--body", "inline"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			_, stderr, code := runCLI(t, append([]string{"--root", root, "task", "create", "Empty file"}, args...)...)
			if code != 2 {
				t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(root, ".ahm", "tasks", "active", "001.md")); !os.IsNotExist(err) {
		t.Errorf("a refused create wrote a record, err = %v", err)
	}
}

func TestTaskCreateBodyFileShorthandIsRegistered(t *testing.T) {
	var a app
	task := a.taskCommand()
	create, _, err := task.Find([]string{"create"})
	if err != nil {
		t.Fatal(err)
	}
	if create.Flags().ShorthandLookup("F") == nil {
		t.Error("task create does not register the -F shorthand the design grants --body-file")
	}
}

func TestTaskEditJSONUpdatedTimestampMatchesTheWrite(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "270", "Stamped", "", "## Summary\n\nTODO.\n")

	stdout, stderr, code := runCLI(t, "--root", root, "--json", "task", "edit", "270", "--priority", "P1")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	var payload struct {
		Task struct {
			Updated string `json:"updated"`
		} `json:"task"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("decoding %q: %v", stdout, err)
	}
	if payload.Task.Updated == "" {
		t.Fatal("payload task.updated is empty, want the timestamp the write landed")
	}
	task, err := parseTask(filepath.Join(root, ".ahm", "tasks", "active", "270.md"), "active")
	if err != nil {
		t.Fatal(err)
	}
	if payload.Task.Updated != task.Updated {
		t.Errorf("payload task.updated = %q, record updated = %q, want the same value", payload.Task.Updated, task.Updated)
	}
}

func TestTaskEditJSONUnchangedPathCarriesTheRecord(t *testing.T) {
	root := projectRoot(t)
	writeEditableTask(t, root, "270", "Stable", "", "## Summary\n\nTODO.\n")

	stdout, stderr, code := runCLI(t, "--root", root, "--json", "task", "edit", "270", "--priority", "P2")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	var payload struct {
		Updated bool `json:"updated"`
		Changed []string
		Task    *struct {
			ID       string `json:"id"`
			Priority string `json:"priority"`
		} `json:"task"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("decoding %q: %v", stdout, err)
	}
	if payload.Updated {
		t.Error("updated = true on the unchanged path")
	}
	if len(payload.Changed) != 0 {
		t.Errorf("changed = %v, want empty", payload.Changed)
	}
	if payload.Task == nil {
		t.Fatal("task is absent from the unchanged payload")
	}
	if payload.Task.ID != "270" || payload.Task.Priority != "P2" {
		t.Errorf("task = %+v, want the unchanged record", payload.Task)
	}
}

func TestTaskEditReportRendersThePreviewPathInDryRun(t *testing.T) {
	// The structured `path` keeps the record-path rendering while the dry-run
	// text line uses the slash-normalized preview one. On POSIX the two are
	// equal, so this pins the distinction directly.
	var out strings.Builder
	report := taskEditReport{
		ID:          "270",
		Path:        `C:\repo\.ahm\tasks\active\270.md`,
		DryRun:      true,
		previewPath: "C:/repo/.ahm/tasks/active/270.md",
		Changed:     []string{"priority"},
		Changes:     []taskEditChange{{Field: "priority", From: "P2", To: "P1"}},
	}
	if err := report.RenderText(&out); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "270 edit: C:/repo/.ahm/tasks/active/270.md") {
		t.Errorf("dry-run text = %q, want the preview path", text)
	}
	if !strings.Contains(text, "270 priority: P2 -> P1") {
		t.Errorf("dry-run text = %q, want the field diff", text)
	}

	// A dry-run report with no preview path falls back to the structured one.
	out.Reset()
	report.previewPath = ""
	if err := report.RenderText(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `270 edit: C:\repo\.ahm\tasks\active\270.md`) {
		t.Errorf("fallback text = %q, want the structured path", out.String())
	}
}

func TestTaskEditSectionCannotDeleteANestedProtectedLog(t *testing.T) {
	root := projectRoot(t)
	// A `### Comments` heading nested inside Summary belongs to the Summary
	// section, because a section runs to the next heading of the same or a
	// higher level, so replacing Summary would delete the log.
	path := writeEditableTask(t, root, "270", "Nested",
		"", "## Summary\n\nObserved this.\n\n### Comments\n\n**2026-06-24T18:30:00Z** — real log\n")

	_, stderr, code := runTaskEdit(t, root, "270", "--section", "Summary", "--body", "Now fixed.")
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %q, want 2", code, stderr)
	}
	assertContainsAll(t, stderr, "## Comments", "--force")
	assertFileContainsAll(t, path, "real log")
}

func TestTaskEditSectionProtectedDropSucceedsWithForce(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Nested",
		"", "## Summary\n\nObserved this.\n\n### Comments\n\n**2026-06-24T18:30:00Z** — real log\n")

	_, stderr, code := runCLI(t, "--root", root, "--force", "task", "edit", "270", "--section", "Summary", "--body", "Now fixed.")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	assertFileContainsAll(t, path, "## Summary\n\nNow fixed.")
	assertNotContains(t, mustRead(t, path), "real log")
}

func TestTaskEditSectionBesideAProtectedSectionIsAllowed(t *testing.T) {
	root := projectRoot(t)
	path := writeEditableTask(t, root, "270", "Separate",
		"", "## Summary\n\nObserved this.\n\n## Acceptance Notes\n\n- [ ] one\n\n## Comments\n\n**2026-06-24T18:30:00Z** — real log\n")

	// A sibling protected section is outside the replaced range, so an ordinary
	// section edit next to it is unaffected by the guard.
	_, stderr, code := runTaskEdit(t, root, "270", "--section", "Acceptance Notes", "--body", "- [x] one")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr)
	}
	assertFileContainsAll(t, path, "## Summary\n\nObserved this.", "## Comments", "real log", "- [x] one")
}
