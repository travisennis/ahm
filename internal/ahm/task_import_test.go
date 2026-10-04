package ahm

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func importFile(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tasks.json")
	writeFile(t, path, data)
	return path
}

func runImport(t *testing.T, root, file string, flags ...string) (string, string, int) {
	t.Helper()
	args := append([]string{"--root", root}, flags...)
	return runCLI(t, append(args, "task", "import", "--from-file", file)...)
}

func importReport(t *testing.T, output string) taskImportReport {
	t.Helper()
	var report taskImportReport
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		t.Fatal(err, output)
	}
	return report
}

const realisticImport = `[
 {"ref":"child","title":"Ship parser","parent":"@tracker","status":"Pending","priority":"P1","effort":"M","labels":"type:feature, area:cli","depends_on":["@foundation"],"created":"2024-01-02T03:04:05Z","external_ref":"https://github.com/example/project/issues/42","body":"## Summary\n\nPreserve the export.\n\n## Comments\n\n**2024-01-03T03:04:05Z** — _Trav_: Reviewed.\n\n## Acceptance Notes\n\n- [ ] Exercise parser."},
 {"ref":"tracker","title":"Parser migration","status":"Tracking"},
 {"ref":"foundation","title":"Build foundation","status":"Completed","body":"## Acceptance Notes\n\n- [x] Foundation verified."},
 {"ref":"discarded","title":"Retired alternative","status":"Cancelled","body":"## Cancellation Reason\n\nSuperseded."}
]`

func TestTaskImportRealisticExport(t *testing.T) {
	for _, mode := range []string{"project", "home"} {
		t.Run(mode, func(t *testing.T) {
			root := projectRoot(t)
			if mode == "home" {
				setStoreHome(t)
				root = initHomeModeRepository(t)
			}
			file := importFile(t, realisticImport)
			renders := 0
			old := indexWritesForPathsHook
			indexWritesForPathsHook = func() { renders++ }
			t.Cleanup(func() { indexWritesForPathsHook = old })
			stdout, stderr, code := runImport(t, root, file, "--json")
			if code != 0 || stderr != "" {
				t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
			}
			if renders != 1 {
				t.Fatalf("index renders=%d", renders)
			}
			report := importReport(t, stdout)
			wantIDs := []string{"001a", "001", "002", "003"}
			for i, record := range report.Records {
				if record.ID != wantIDs[i] || record.Outcome != "imported" {
					t.Fatal(report)
				}
			}
			output, stderr, code := runCLI(t, "--root", root, "--json", "task", "show", "001a")
			if code != 0 {
				t.Fatal(stderr)
			}
			var child Task
			if err := json.Unmarshal([]byte(output), &child); err != nil {
				t.Fatal(err)
			}
			if child.Parent != "001" || !reflect.DeepEqual(child.DependsOn, []string{"002"}) || child.Created != "2024-01-02T03:04:05Z" || child.Updated == "" || !strings.Contains(child.Body, "_Trav_: Reviewed.") || child.ExternalRef == "" {
				t.Fatalf("child=%+v", child)
			}
			if mode == "home" && storeTaskIDCounter(t, root) != 4 {
				t.Fatal("counter not raised")
			}
			output, stderr, code = runCLI(t, "--root", root, "--json", "status")
			if code != 0 {
				t.Fatalf("status: %s %s", output, stderr)
			}
			var status struct {
				Validation validationReport `json:"validation"`
			}
			if err := json.Unmarshal([]byte(output), &status); err != nil {
				t.Fatal(err)
			}
			if len(status.Validation.Errors)+len(status.Validation.Warnings)+len(status.Validation.Info) != 0 {
				t.Fatalf("findings: %s", output)
			}
			if got := createTask(t, root, "Next"); got != "004" {
				t.Fatalf("next ID=%s", got)
			}
		})
	}
}

func TestTaskImportDryRunGoldenAndNoLocks(t *testing.T) {
	root := projectRoot(t)
	file := importFile(t, `[{"ref":"first","title":"First","depends_on":["@second"]},{"ref":"second","title":"Second"}]`)
	before := snapshotTree(t, root)
	golden := `{"dry_run":true,"records":[{"ref":"first","id":"001","path":".ahm/tasks/active/001.md","parent":"","depends_on":["002"],"outcome":"planned","errors":[]},{"ref":"second","id":"002","path":".ahm/tasks/active/002.md","parent":"","depends_on":[],"outcome":"planned","errors":[]}]}` + "\n"
	golden = strings.ReplaceAll(golden, `"path":".ahm/`, `"path":"`+filepath.ToSlash(reportedRoot(t, root))+`/.ahm/`)
	for _, flag := range []string{"--plain", "--json"} {
		out, errout, code := runImport(t, root, file, "--dry-run", flag)
		if code != 0 || errout != "" {
			t.Fatalf("%d %s", code, errout)
		}
		var got any
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatal(err)
		}
		if flag == "--plain" && out != golden {
			t.Fatalf("plain golden:\n%s", out)
		}
		if flag == "--json" {
			var want taskImportReport
			if err := json.Unmarshal([]byte(golden), &want); err != nil {
				t.Fatal(err)
			}
			formatted, err := json.MarshalIndent(want, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if out != string(formatted)+"\n" {
				t.Fatalf("JSON golden:\n%s", out)
			}
		}
		assertTreeUnchanged(t, root, before)
		if _, err := os.Stat(filepath.Join(root, ".ahm", ".lock")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("dry run created lock: %v", err)
		}
	}
}

func TestTaskImportRefusalsReportAllAndWriteNothing(t *testing.T) {
	root := projectRoot(t)
	file := importFile(t, `[{"ref":"bad","title":"","priority":"P9","effort":"XX","status":"Unknown","created":"yesterday","depends_on":["999","@absent"]},{"ref":"bad","title":"Another","parent":"@child"},{"ref":"child","title":"Child","parent":"@bad"}]`)
	before := snapshotTree(t, root)
	out, stderr, code := runImport(t, root, file, "--json")
	if code != 1 || stderr != "" {
		t.Fatalf("%d %s %s", code, stderr, out)
	}
	report := importReport(t, out)
	if len(report.Records[0].Errors) < 7 || len(report.Records[1].Errors) < 2 {
		t.Fatalf("incomplete refusals: %s", out)
	}
	assertTreeUnchanged(t, root, before)
}

func TestTaskImportMalformedDocuments(t *testing.T) {
	for _, data := range []string{`null`, `[null]`, `[{"title":"T","status":null}]`, `[{"title":"T","depends_on":null}]`, `[{"title":"T","depends_on":[null]}]`, `[{"title":"T","DEPENDS_ON":[null]}]`, `{}`, `[`, `[] []`, `[{"title":"T","id":"123"}]`, `[{"title":42}]`, `[{"title":"T","extra":{}}]`} {
		t.Run(data, func(t *testing.T) {
			root := projectRoot(t)
			before := snapshotTree(t, root)
			_, _, code := runImport(t, root, importFile(t, data), "--json")
			if code != 2 {
				t.Fatalf("code=%d", code)
			}
			assertTreeUnchanged(t, root, before)
		})
	}
}

func TestTaskImportRelationshipRefusals(t *testing.T) {
	for _, data := range []string{
		`[{"ref":"a","title":"A","depends_on":["@b"]},{"ref":"b","title":"B","depends_on":["@a"]}]`,
		`[{"ref":"a","title":"A","depends_on":["@a"]}]`,
		`[{"ref":"a","title":"A","depends_on":["@b"]},{"ref":"b","title":"B","status":"Cancelled"}]`,
		`[{"ref":"a","title":"A"},{"ref":"b","title":"B","parent":"@a"},{"title":"C","parent":"@b"}]`,
	} {
		t.Run(data, func(t *testing.T) {
			root := projectRoot(t)
			before := snapshotTree(t, root)
			out, _, code := runImport(t, root, importFile(t, data), "--json")
			if code != 1 {
				t.Fatalf("%d %s", code, out)
			}
			assertTreeUnchanged(t, root, before)
		})
	}
}

func TestTaskImportExistingReferencesAndMarkdownLinks(t *testing.T) {
	root := projectRoot(t)
	createTask(t, root, "Existing")
	file := importFile(t, `[{"title":"Child","parent":"1","depends_on":["001","1"],"body":"[Broken](missing.md)"}]`)
	out, stderr, code := runImport(t, root, file, "--json")
	if code != 0 {
		t.Fatalf("%d %s %s", code, stderr, out)
	}
	report := importReport(t, out)
	if report.Records[0].ID != "001a" || !reflect.DeepEqual(report.Records[0].DependsOn, []string{"001"}) {
		t.Fatal(out)
	}
	out, _, code = runCLI(t, "--root", root, "--json", "--check", "links", "status")
	if code != 0 || !strings.Contains(out, "markdown_link_missing") {
		t.Fatalf("%d %s", code, out)
	}
}

// TestTaskImportRaisesChildSuffixMarks covers the bulk allocator: an imported
// child's letter is a spent letter, so a later create under the same parent
// does not reuse it after the record is deleted.
func TestTaskImportRaisesChildSuffixMarks(t *testing.T) {
	setStoreHome(t)
	root := initHomeModeRepository(t)
	file := importFile(t, `[{"ref":"parent","title":"Parent"},{"ref":"child","title":"Child","parent":"@parent"}]`)
	if _, stderr, code := runImport(t, root, file, "--json"); code != 0 {
		t.Fatalf("import: %s", stderr)
	}
	if got := storeChildSuffixMark(t, root, "001"); got != "a" {
		t.Fatalf("imported child suffix mark = %q, want \"a\"", got)
	}
	if err := os.Remove(storeTaskFile(t, root, "active", "001a")); err != nil {
		t.Fatal(err)
	}
	if got := createTask(t, root, "Second Child", "--parent", "001"); got != "001b" {
		t.Errorf("child create after the imported child was deleted = %q, want 001b", got)
	}
}

func TestTaskImportRollbackRestoresFiles(t *testing.T) {
	for _, failure := range []string{"record", "index", "state"} {
		t.Run(failure, func(t *testing.T) {
			home := setStoreHome(t)
			root := initHomeModeRepository(t)
			createTask(t, root, "Existing")
			beforeRoot, beforeStore := snapshotTree(t, root), snapshotTree(t, home)
			store := storePathsOf(t, root)
			old := taskImportWriteHook
			t.Cleanup(func() { taskImportWriteHook = old })
			calls := 0
			taskImportWriteHook = func(path string) error {
				calls++
				if (failure == "record" && calls == 2) || (failure == "index" && strings.HasSuffix(path, "index.md")) || (failure == "state" && path == store.statePath()) {
					return fmt.Errorf("injected %s failure", failure)
				}
				// Every write runs under both locks. No nested record lock can be taken.
				for _, dir := range []string{filepath.Join(store.ProjectDir, ".lock", workflowRecordLockName), filepath.Join(store.Root, ".lock", storeStateLockName)} {
					if _, err := os.Stat(filepath.Join(dir, workflowLockOwnerFile)); err != nil {
						t.Errorf("missing held lock %s: %v", dir, err)
					}
				}
				return nil
			}
			out, stderr, code := runImport(t, root, importFile(t, `[{"title":"A"},{"title":"B"}]`), "--json")
			if code != 1 || !strings.Contains(stderr, "injected") || out != "" {
				t.Fatalf("%d %s %s", code, stderr, out)
			}
			// Restoring bytes rewrites modification times, so compare content only.
			for _, pair := range []struct {
				root   string
				before map[string]treeEntry
			}{{root, beforeRoot}, {home, beforeStore}} {
				after := snapshotTree(t, pair.root)
				if len(after) != len(pair.before) {
					t.Fatalf("rollback added files: %v", after)
				}
				for path, want := range pair.before {
					if after[path].content != want.content {
						t.Fatalf("rollback changed %s", path)
					}
				}
			}
			taskImportWriteHook = old
			if got := createTask(t, root, "After rollback"); got != "002" {
				t.Fatal(got)
			}
		})
	}
}

func TestTaskImportHomeDryRunPreservesStore(t *testing.T) {
	home := setStoreHome(t)
	root := initHomeModeRepository(t)
	beforeRoot, beforeStore := snapshotTree(t, root), snapshotTree(t, home)
	file := importFile(t, realisticImport)
	_, stderr, code := runImport(t, root, file, "--dry-run", "--json")
	if code != 0 {
		t.Fatal(stderr)
	}
	assertTreeUnchanged(t, root, beforeRoot)
	assertTreeUnchanged(t, home, beforeStore)
	if storeTaskIDCounter(t, root) != 1 {
		t.Fatal("dry-run moved counter")
	}
}

func TestTaskImportLargeExportAndRecordedCounter(t *testing.T) {
	setStoreHome(t)
	root := initHomeModeRepository(t)
	paths := workflowPathsForStore(root, storePathsOf(t, root))
	if err := writeTaskIDCounter(paths, 200); err != nil {
		t.Fatal(err)
	}
	records := make([]taskImportRecord, 155)
	for i := range records {
		records[i] = taskImportRecord{DependsOn: []string{}, Ref: fmt.Sprintf("issue-%d", i), Title: fmt.Sprintf("Issue %d", i), Body: "## Comments\n\n**2024-01-01T00:00:00Z** — _Trav_: Exported."}
		if i < 42 {
			records[i].DependsOn = []string{fmt.Sprintf("@issue-%d", i+1)}
		}
	}
	data, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	out, stderr, code := runImport(t, root, importFile(t, string(data)), "--plain")
	if code != 0 || stderr != "" {
		t.Fatalf("%d %s", code, stderr)
	}
	report := importReport(t, out)
	if len(report.Records) != 155 || report.Records[0].ID != "200" || report.Records[154].ID != "354" {
		t.Fatal(out)
	}
	if storeTaskIDCounter(t, root) != 355 || createTask(t, root, "Next") != "355" {
		t.Fatal("counter diverged")
	}
}

func TestTaskImportEmptyBatchAndStdin(t *testing.T) {
	root := projectRoot(t)
	before := snapshotTree(t, root)
	out, stderr, code := runImport(t, root, importFile(t, `[]`), "--plain")
	if code != 0 || stderr != "" || out != "{\"dry_run\":false,\"records\":[]}\n" {
		t.Fatalf("%d %s %s", code, stderr, out)
	}
	assertTreeUnchanged(t, root, before)
	var stdout, errout strings.Builder
	a := app{out: &stdout, err: &errout, in: strings.NewReader(`[{"title":"Stdin"}]`)}
	if err := a.run([]string{"--root", root, "--plain", "task", "import", "--from-file", "-"}); err != nil {
		t.Fatal(err)
	}
	if report := importReport(t, stdout.String()); report.Records[0].Outcome != "imported" {
		t.Fatal(stdout.String())
	}
}

func TestTaskImportRefWhitespaceRefused(t *testing.T) {
	for _, ref := range []string{"bad\fref", "bad\vref", "bad\u00a0ref", "bad\u2003ref"} {
		t.Run(ref, func(t *testing.T) {
			root := projectRoot(t)
			data, err := json.Marshal([]taskImportRecord{{Ref: ref, Title: "Task"}})
			if err != nil {
				t.Fatal(err)
			}
			// Marshal a non-nil dependency list to satisfy the input schema.
			data = []byte(strings.ReplaceAll(string(data), `"depends_on":null`, `"depends_on":[]`))
			out, _, code := runImport(t, root, importFile(t, string(data)), "--json")
			if code != 1 || !strings.Contains(out, "ref must not contain @ or whitespace") {
				t.Fatalf("%d %s", code, out)
			}
		})
	}
}

func TestTaskImportDuplicateFieldsRefused(t *testing.T) {
	for _, data := range []string{
		`[{"title":"First","title":"Second"}]`,
		`[{"title":"First","TITLE":"Second"}]`,
		`[{"title":"T","depends_on":[null],"depends_on":[]}]`,
		`[{"title":"T","depends_on":[],"DEPENDS_ON":[null]}]`,
		`[{"title":"T","status":null,"status":"Open"}]`,
		`[{"title":"T","body":"Safe","b\u006fdy":"Replacement"}]`,
	} {
		t.Run(data, func(t *testing.T) {
			root := projectRoot(t)
			before := snapshotTree(t, root)
			for _, flags := range [][]string{{"--json"}, {"--dry-run", "--plain"}} {
				out, stderr, code := runImport(t, root, importFile(t, data), flags...)
				if code != 2 || out != "" || !strings.Contains(stderr, "duplicate field") {
					t.Fatalf("code=%d stdout=%q stderr=%q", code, out, stderr)
				}
				assertTreeUnchanged(t, root, before)
			}
		})
	}
}
