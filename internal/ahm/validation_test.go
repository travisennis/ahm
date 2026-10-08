package ahm

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestValidateWorkflowStateMatchesStandaloneValidation(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	paths := workflowPathsFor(root)
	path := paths.taskFile("active", "301")
	writeFile(t, path, `---
id: 301
title: Missing labels
status: Pending
priority: P2
effort: S
exec_plan: -
depends_on: -
---
# Missing labels

## Acceptance Notes

- [ ] Preserve validation findings.
`)

	// Tracking task with all children resolved: the tracking-children warning
	// must agree between the standalone and reused validation paths.
	writeFile(t, paths.taskFile("active", "302"), `---
id: 302
title: Tracker done
status: Tracking
priority: P1
effort: M
labels: type:task
exec_plan: -
depends_on: -
---
# Tracker done
`)
	writeFile(t, paths.taskFile("completed", "302a"), `---
id: 302a
title: Child done
status: Completed
priority: P1
effort: S
labels: type:task
exec_plan: -
depends_on: -
parent: 302
---
# Child done
`)
	// Tracking task with a still-open child: no warning in either path.
	writeFile(t, paths.taskFile("active", "303"), `---
id: 303
title: Tracker open
status: Tracking
priority: P1
effort: M
labels: type:task
exec_plan: -
depends_on: -
---
# Tracker open
`)
	writeFile(t, paths.taskFile("active", "303a"), `---
id: 303a
title: Child open
status: Pending
priority: P1
effort: S
labels: type:task
exec_plan: -
depends_on: -
parent: 303
---
# Child open
`)

	tasks, err := collectTasksForPaths(paths)
	if err != nil {
		t.Fatal(err)
	}
	writes, err := indexWritesForPaths(root, tasks, paths, nil)
	if err != nil {
		t.Fatal(err)
	}
	for target, content := range writes {
		if err := writeFileAtomic(target, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	assertEquivalent := func(context string) {
		t.Helper()
		standalone, _ := validateWorkflowScopedForPaths(root, []string{CheckScopeWorkflow}, paths)
		reused := validateWorkflowStateForPaths(root, paths, tasks, writes, nil, nil)
		if standalone.OK != reused.OK ||
			!reflect.DeepEqual(standalone.Errors, reused.Errors) ||
			!reflect.DeepEqual(standalone.Warnings, reused.Warnings) ||
			!reflect.DeepEqual(standalone.Info, reused.Info) {
			t.Fatalf("%s: reused validation differs from standalone\nstandalone: %+v\nreused: %+v", context, standalone, reused)
		}
	}
	assertEquivalent("valid metadata")

	writeFile(t, filepath.Join(root, ".ahm", "config.json"), "{")
	assertEquivalent("corrupt metadata")
}

func TestStatusWithoutMetadataDoesNotCascadeWorkflowArtifactFindings(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.status(); !errors.Is(err, errValidationFailed) {
		t.Errorf("expected errValidationFailed, got: %v", err)
	}
	got := out.String()
	assertContainsAll(t, got,
		`"code": "metadata_missing"`,
		`"installed_version": null`,
	)
	assertNotContains(t, got,
		`"code": "generated_index_missing"`,
		`"code": "markdown_link_missing"`,
		`"installed_version": ""`,
	)
}

func TestStatusWithMetadataShowsInstalledVersion(t *testing.T) {
	root := projectRoot(t)
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(); err != nil {
		t.Fatal(err)
	}

	// JSON mode: installed_version shows the binary version.
	var jOut strings.Builder
	a := app{opts: options{root: root, json: true}, out: &jOut}
	if err := a.status(); err != nil {
		t.Errorf("status error: %v", err)
	}
	jGot := jOut.String()
	assertContainsAll(t, jGot, `"installed": true`, `"installed_version": "dev"`)

	// Text mode: installed_version shows the binary version.
	var tOut strings.Builder
	a2 := app{opts: options{root: root}, out: &tOut}
	if err := a2.status(); err != nil {
		t.Errorf("status error: %v", err)
	}
	tGot := tOut.String()
	assertContainsAll(t, tGot, "installed: true", "installed_version: dev")
}

func TestDoctorWithoutMetadataShowsInstalledVersionNone(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	// JSON mode: installed_version shows null.
	var jOut strings.Builder
	a := app{opts: options{root: root, json: true}, out: &jOut}
	if err := a.doctor(); !errors.Is(err, errValidationFailed) {
		t.Errorf("expected errValidationFailed, got: %v", err)
	}
	jGot := jOut.String()
	assertContainsAll(t, jGot, `"installed_version": null`)
	assertNotContains(t, jGot, `"installed_version": ""`)

	// Text mode: installed_version shows none.
	var tOut strings.Builder
	a2 := app{opts: options{root: root}, out: &tOut}
	if err := a2.doctor(); !errors.Is(err, errValidationFailed) {
		t.Errorf("expected errValidationFailed, got: %v", err)
	}
	tGot := tOut.String()
	assertContainsAll(t, tGot, "installed_version: none")
}

// TestRetiredRecordFamiliesChangeNothing pins acceptance criteria one and three
// of task 264b end to end. With retired records on disk, including broken links,
// stale indexes, and a task that still carries a dangling exec_plan value, the
// read-write commands read no file under either retired tree, report no finding
// for one, and report exactly the same findings once the trees are gone.
func TestRetiredRecordFamiliesChangeNothing(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	paths := workflowPathsFor(root)
	trees := []string{".ahm/research", ".ahm/exec-plans"}
	// A surviving task that still carries the retired field.
	writeFile(t, paths.taskFile("active", "001"), "---\n"+
		"id: 001\n"+
		"title: Retired Field\n"+
		"status: Pending\n"+
		"priority: P2\n"+
		"effort: S\n"+
		"labels: type:task\n"+
		"exec_plan: 999-old-plan\n"+
		"depends_on: -\n"+
		"---\n"+
		"# Retired Field\n\n## Acceptance Notes\n\n- [x] Done.\n")
	writeADRFile(t, root, "001-good-decision.md", "---\nstatus: accepted\ndate: 2026-07-01\n---\n# Good Decision\n\nBody.\n")
	for _, tree := range trees {
		writeFile(t, filepath.Join(root, filepath.FromSlash(tree), "inbox", "note.md"),
			"# Retired record\n\n[missing](missing.md)\n")
		writeFile(t, filepath.Join(root, filepath.FromSlash(tree), "index.md"),
			"# Retired index\n\n[missing](missing.md)\n")
	}

	reads := map[string]int{}
	original := readWorkflowFileHook
	// Canonicalize both sides: the runCLI commands resolve their root while the
	// direct validator calls use the raw spelling, and on Windows those can
	// differ (an 8.3 short name against the long form).
	readWorkflowFileHook = func(path string) { reads[relPath(canonicalPath(root), canonicalPath(path))]++ }
	t.Cleanup(func() { readWorkflowFileHook = original })

	// Exercise the read-write commands twice: once with the retired trees in
	// place and once after deleting them. index runs first so the surviving
	// generated state is settled and the later commands exit 0.
	commands := []string{"index", "status", "doctor", "prime"}
	beforeReport := runRetiredTreeCommands(t, root, commands)
	for _, tree := range trees {
		if err := os.RemoveAll(filepath.Join(root, filepath.FromSlash(tree))); err != nil { // #nosec G703 -- path built from t.TempDir
			t.Fatal(err)
		}
	}
	afterReport := runRetiredTreeCommands(t, root, commands)

	for path := range reads {
		if strings.Contains(path, "research") || strings.Contains(path, "exec-plans") {
			t.Errorf("read a retired record: %s", path)
		}
	}
	if !reflect.DeepEqual(beforeReport.Errors, afterReport.Errors) ||
		!reflect.DeepEqual(beforeReport.Warnings, afterReport.Warnings) ||
		!reflect.DeepEqual(beforeReport.Info, afterReport.Info) {
		t.Errorf("removing the retired trees changed the findings\nbefore: %+v\nafter: %+v", beforeReport, afterReport)
	}
}

// runRetiredTreeCommands runs each command, requires success, and returns the
// validation report of the read-write commands whose findings could mention a
// retired record.
func runRetiredTreeCommands(t *testing.T, root string, commands []string) validationReport {
	t.Helper()
	var report validationReport
	for _, command := range commands {
		stdout, stderr, code := runCLI(t, "--root", root, command)
		if code != 0 {
			t.Fatalf("%s exit code = %d, stdout = %s, stderr = %s", command, code, stdout, stderr)
		}
		assertNotContains(t, stdout+stderr, "exec_plan", "research_inbox_stale", "markdown_link_missing")
	}
	report, _ = validateWorkflowScopedForPaths(root, nil, workflowPathsFor(root))
	for _, findings := range [][]validationFinding{report.Errors, report.Warnings, report.Info} {
		for _, finding := range findings {
			if strings.Contains(finding.Path, "research") || strings.Contains(finding.Path, "exec-plans") {
				t.Errorf("retired record produced a finding: %#v", finding)
			}
			if strings.Contains(finding.Code, "research") || strings.Contains(finding.Code, "exec_plan") {
				t.Errorf("retired finding code emitted: %#v", finding)
			}
		}
	}
	return report
}

func TestValidateWorkflowScopedWorkflowOnly(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	// Add a broken link that would trigger markdown_link_missing.
	writeLinkCarrierTask(t, root, "002", "[missing](missing.md)\n")
	// Add a workflow issue.
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Bad Task", "Doing", "depends_on: -\n")

	// Only workflow checks.
	report, _ := validateWorkflowScopedForPaths(root, []string{CheckScopeWorkflow}, workflowPathsFor(root))
	// Should find task_malformed (workflow check) but NOT markdown_link_missing.
	foundTaskMalformed := false
	for _, e := range report.Errors {
		if e.Code == "task_malformed" {
			foundTaskMalformed = true
		}
	}
	if !foundTaskMalformed {
		t.Error("expected task_malformed in workflow-only scope")
	}
	for _, e := range report.Errors {
		if e.Code == "markdown_link_missing" {
			t.Error("unexpected markdown_link_missing in workflow-only scope")
		}
	}
}

func TestValidateWorkflowScopedLinksOnly(t *testing.T) {
	root := projectRoot(t)
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(); err != nil {
		t.Fatal(err)
	}
	paths := workflowPathsFor(root)
	// Add a broken link.
	writeLinkCarrierTask(t, root, "002", "[missing](missing.md)\n")
	// Create a workflow issue.
	writeTaskFile(t, paths.taskFile("active", "001"), "001", "Bad Task", "Doing", "depends_on: -\n")

	// Only link checks.
	report, _ := validateWorkflowScopedForPaths(root, []string{CheckScopeLinks}, paths)
	// Should find markdown_link_missing.
	foundLinkMissing := false
	for _, w := range report.Warnings {
		if w.Code == "markdown_link_missing" {
			foundLinkMissing = true
			break
		}
	}
	if !foundLinkMissing {
		t.Error("expected markdown_link_missing in links-only scope")
	}
	// Should NOT find task_malformed (workflow check).
	for _, e := range report.Errors {
		if e.Code == "task_malformed" {
			t.Error("unexpected task_malformed in links-only scope")
		}
	}
	// No workflow errors since we only ran link checks.
	if !report.OK {
		t.Error("expected OK for links-only scope, got errors")
	}
}

func TestValidateWorkflowScopedAll(t *testing.T) {
	// nil scopes = default checks (same as validateWorkflow): workflow + links.
	root := projectRoot(t)
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(); err != nil {
		t.Fatal(err)
	}
	paths := workflowPathsFor(root)
	writeLinkCarrierTask(t, root, "002", "[missing](missing.md)\n")

	// No scopes = default checks run.
	report, _ := validateWorkflowScopedForPaths(root, nil, paths)
	foundLinkMissing := false
	for _, w := range report.Warnings {
		if w.Code == "markdown_link_missing" {
			foundLinkMissing = true
			break
		}
	}
	if !foundLinkMissing {
		t.Error("expected markdown_link_missing when running all checks")
	}
	// validateWorkflowScopedForPaths with nil scopes should produce the same result.
	report2, _ := validateWorkflowScopedForPaths(root, nil, paths)
	if report.OK != report2.OK {
		t.Error("validateWorkflowScopedForPaths(nil) should match validateWorkflowScopedForPaths with workflow+links")
	}
	if len(report.Errors) != len(report2.Errors) {
		t.Errorf("error count mismatch: %d vs %d", len(report.Errors), len(report2.Errors))
	}
}

func TestCLIStatusInvalidCheckScope(t *testing.T) {
	root := projectRoot(t)
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, "--root", root, "status", "--check", "bogus")
	if code != 2 {
		t.Errorf("expected exit code 2 for invalid check scope, got %d; stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "unknown check scope") {
		t.Errorf("expected unknown check scope error, got: %s", stderr)
	}
	_ = stdout
}

func TestCLIDoctorWithCheckScope(t *testing.T) {
	root := projectRoot(t)
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(); err != nil {
		t.Fatal(err)
	}

	// Doctor with --check workflow should succeed (no issues in a fresh install).
	stdout, stderr, code := runCLI(t, "--root", root, "doctor", "--check", "workflow")
	if code != 0 {
		t.Errorf("expected exit code 0, got %d; stderr=%s, stdout=%s", code, stderr, stdout)
	}
	assertContainsAll(t, stdout, `"ok": true`)
}

func TestPostMutation_ScopeIsWorkflowOnly(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)

	// Create a workflow finding: a completed task sitting in the active bucket.
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Completed In Active", "Completed", "depends_on: -\n")

	// Create a broken markdown link that would trigger markdown_link_missing.
	writeLinkCarrierTask(t, root, "002", "[missing](missing.md)\n")

	stdout, stderr, code := runCLI(t, "--root", root, "index")
	if code != 0 {
		t.Errorf("index exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	// Verify the workflow finding appears.
	assertContainsAll(t, stderr,
		"completed task should be in .ahm/tasks/completed",
	)
	// Verify the markdown_link_missing finding does NOT appear.
	assertNotContains(t, stderr,
		"markdown_link_missing",
	)
}

func TestPostMutation_DryRunSkipsValidation(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)

	// Create a workflow finding: a completed task sitting in the active bucket.
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Completed In Active", "Completed", "depends_on: -\n")

	stdout, stderr, code := runCLI(t, "--dry-run", "--root", root, "index")
	if code != 0 {
		t.Errorf("index exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	// The validation should not run during dry-run, so no warnings.
	if strings.Contains(stderr, "completed task should be in") {
		t.Errorf("dry-run index emitted unexpected warning on stderr:\n%s", stderr)
	}
}

func TestDocsCommandRemoved(t *testing.T) {
	root := projectRoot(t)
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{{"docs"}, {"docs", "check"}} {
		stdout, stderr, code := runCLI(t, append([]string{"--root", root}, args...)...)
		if code != 2 {
			t.Errorf("%v: exit code = %d, want 2; stdout = %s, stderr = %s", args, code, stdout, stderr)
		}
		if !strings.Contains(stderr, `unknown command "docs" for "ahm"`) {
			t.Errorf("%v: expected unknown-command usage error, got %s", args, stderr)
		}
	}
}

func TestProjectDocsCheckScopeRemoved(t *testing.T) {
	root := projectRoot(t)
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(); err != nil {
		t.Fatal(err)
	}

	for _, command := range []string{"status", "doctor"} {
		stdout, stderr, code := runCLI(t, "--root", root, command, "--check", "project-docs")
		if code != 2 {
			t.Errorf("%s: exit code = %d, want 2; stdout = %s, stderr = %s", command, code, stdout, stderr)
		}
		assertContainsAll(t, stderr, `unknown check scope "project-docs"`, "valid: workflow, links")
	}

	for _, scopes := range []string{"workflow", "links", "workflow,links"} {
		stdout, stderr, code := runCLI(t, "--root", root, "status", "--check", scopes)
		if code != 0 {
			t.Errorf("%s: exit code = %d, stdout = %s, stderr = %s", scopes, code, stdout, stderr)
		}
	}
}

// runStatusReport runs status (doctor when doctor is true) against root in the
// requested output mode ("" for text, "json", or "plain") and returns the
// emitted report. status and doctor emit their report before returning
// errValidationFailed, so that error is tolerated; any other error fails.
func runStatusReport(t *testing.T, doctor bool, root string, mode string) string {
	t.Helper()
	var out strings.Builder
	opts := options{root: root}
	switch mode {
	case "json":
		opts.json = true
	case "plain":
		opts.plain = true
	}
	a := app{opts: opts, out: &out}
	run := a.status
	if doctor {
		run = a.doctor
	}
	if err := run(); err != nil && !errors.Is(err, errValidationFailed) {
		t.Fatalf("report error: %v", err)
	}
	return out.String()
}

// TestStatusAndDoctorReportEffectiveStrictAcceptance covers an installed
// project whose committed configuration sets strict_acceptance false and true:
// both reports surface the effective value in text, JSON, and plain output.
func TestStatusAndDoctorReportEffectiveStrictAcceptance(t *testing.T) {
	for _, tc := range []struct {
		name   string
		strict bool
	}{
		{name: "false", strict: false},
		{name: "true", strict: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := projectRoot(t)
			var installOut strings.Builder
			installer := app{opts: options{root: root}, out: &installOut}
			if err := installer.install(); err != nil {
				t.Fatal(err)
			}
			if tc.strict {
				meta, err := readMetadata(root)
				if err != nil {
					t.Fatal(err)
				}
				meta.StrictAcceptance = true
				writeMetadataFile(t, root, meta)
			}

			value := "false"
			if tc.strict {
				value = "true"
			}
			for _, doctor := range []bool{false, true} {
				assertContainsAll(t, runStatusReport(t, doctor, root, ""), "strict_acceptance: "+value, "records_mode: project")
				assertContainsAll(t, runStatusReport(t, doctor, root, "json"), `"strict_acceptance": `+value, `"records_mode": "project"`)
				assertContainsAll(t, runStatusReport(t, doctor, root, "plain"), `"strict_acceptance":`+value, `"records_mode":"project"`)
			}
		})
	}
}

// TestStatusAndDoctorReportStrictAcceptanceUnknownWhenUninstalled covers a
// project with no committed configuration: the value is unknown, so it renders
// as none/null rather than defaulting to false.
func TestStatusAndDoctorReportStrictAcceptanceUnknownWhenUninstalled(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, doctor := range []bool{false, true} {
		assertContainsAll(t, runStatusReport(t, doctor, root, ""), "strict_acceptance: none", "records_mode: none")
		jsonOut := runStatusReport(t, doctor, root, "json")
		assertContainsAll(t, jsonOut, `"strict_acceptance": null`, `"records_mode": null`)
		assertNotContains(t, jsonOut, `"strict_acceptance": false`, `"records_mode": "project"`)
		assertContainsAll(t, runStatusReport(t, doctor, root, "plain"), `"strict_acceptance":null`, `"records_mode":null`)
	}
}

// TestStatusAndDoctorReportStrictAcceptanceUnknownWhenMetadataCorrupt covers a
// project whose committed configuration cannot be parsed: both values are
// unknown, and the report still carries the corrupt-metadata finding.
func TestStatusAndDoctorReportStrictAcceptanceUnknownWhenMetadataCorrupt(t *testing.T) {
	root := projectRoot(t)
	writeFile(t, filepath.Join(root, ".ahm", "config.json"), "{not json\n")

	for _, doctor := range []bool{false, true} {
		assertContainsAll(t, runStatusReport(t, doctor, root, ""), "strict_acceptance: none", "records_mode: none")
		jsonOut := runStatusReport(t, doctor, root, "json")
		assertContainsAll(t, jsonOut, `"strict_acceptance": null`, `"records_mode": null`, `"code": "metadata_corrupt"`)
	}
}

// TestStatusAndDoctorRecordsOnlyReportStrictAcceptanceUnknown covers a
// --project selection: the committed configuration is deliberately not read,
// so the value is unknown even though the project's configuration sets it.
func TestStatusAndDoctorRecordsOnlyReportStrictAcceptanceUnknown(t *testing.T) {
	setStoreHome(t)
	root := t.TempDir()
	writeHomeModeConfig(t, root)
	if _, stderr, code := runCLI(t, "--root", root, "init"); code != 0 {
		t.Fatalf("init: %s", stderr)
	}
	meta, err := readMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	meta.StrictAcceptance = true
	writeMetadataFile(t, root, meta)
	if _, stderr, code := runCLI(t, "--root", root, "index"); code != 0 {
		t.Fatalf("index: %s", stderr)
	}
	store, err := resolveStore(root)
	if err != nil {
		t.Fatal(err)
	}

	for _, doctor := range []bool{false, true} {
		for _, mode := range []struct {
			name string
			opts options
			want string
		}{
			{name: "text", want: "strict_acceptance: none"},
			{name: "json", opts: options{json: true}, want: `"strict_acceptance": null`},
			{name: "plain", opts: options{plain: true}, want: `"strict_acceptance":null`},
		} {
			t.Run(fmt.Sprintf("doctor=%v/%s", doctor, mode.name), func(t *testing.T) {
				opts := mode.opts
				opts.project = store.Key
				var out strings.Builder
				a := app{opts: opts, out: &out}
				if err := a.detectRoot(); err != nil {
					t.Fatal(err)
				}
				run := a.status
				if doctor {
					run = a.doctor
				}
				if err := run(); err != nil {
					t.Fatalf("report error: %v\n%s", err, out.String())
				}
				// The selection resolves records into the store, and selecting the
				// project from the registry is itself the evidence it is installed.
				installedWant := map[string]string{
					"text":  "installed: true",
					"json":  `"installed": true`,
					"plain": `"installed":true`,
				}[mode.name]
				modeWant := map[string]string{
					"text":  "records_mode: home",
					"json":  `"records_mode": "home"`,
					"plain": `"records_mode":"home"`,
				}[mode.name]
				if doctor {
					installedWant = strings.Replace(installedWant, "installed", "workflow_installed", 1)
				}
				assertContainsAll(t, out.String(), mode.want, modeWant, installedWant)
				assertNotContains(t, out.String(), "strict_acceptance: true", `"strict_acceptance": true`)
			})
		}
	}
}
