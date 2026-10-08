package ahm

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateTaskFrontMatter_CRLF(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)

	// Write a valid task with CRLF.
	path := filepath.Join(root, ".ahm", "tasks", "active", "097.md")
	content := "---\r\n" +
		"id: 097\r\n" +
		"title: Validate CRLF\r\n" +
		"status: Pending\r\n" +
		"priority: P2\r\n" +
		"effort: S\r\n" +
		"labels: type:test, area:workflow\r\n" +
		"exec_plan: -\r\n" +
		"depends_on: -\r\n" +
		"---\r\n" +
		"# Validate CRLF\r\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	var report validationReport
	validateTaskFrontMatter([]byte(content), relPath(root, path), &report)
	for _, e := range report.Errors {
		t.Errorf("validation error for CRLF task: %s: %s", e.Code, e.Message)
	}
	for _, w := range report.Warnings {
		t.Errorf("validation warning for CRLF task: %s: %s", w.Code, w.Message)
	}
}

func TestDoctorReportsMalformedTaskEnums(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	writeTaskFile(t, filepath.Join(root, ".ahm", "tasks", "active", "001.md"), "001", "Bad Task", "Doing", "depends_on: []\n")

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.doctor(); !errors.Is(err, errValidationFailed) {
		t.Errorf("expected errValidationFailed, got: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		`"workflow_installed": true`,
		`"ok": false`,
		`"code": "task_malformed"`,
		`unsupported task status \"Doing\"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("doctor output missing %q:\n%s", want, got)
		}
	}
}

func TestDoctorReportsCompletedTaskAcceptanceFindings(t *testing.T) {
	root := t.TempDir()
	setupAhmRepo(t, root)
	writeCompletedTaskBody(t, root, "001", "Missing Acceptance", "## Summary\n\nDone.\n")
	writeCompletedTaskBody(t, root, "002", "Placeholder Acceptance", "## Acceptance Notes\n\n- [ ] TODO\n")
	writeCompletedTaskBody(t, root, "003", "Unchecked Acceptance", "## Acceptance Criteria\n\n* [ ] Verify it\n")

	// Generate indexes so doctor doesn't report missing-index errors.
	var indexOut strings.Builder
	indexer := app{opts: options{root: root}, out: &indexOut}
	if err := indexer.writeIndexes(); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	a := app{opts: options{root: root, json: true}, out: &out}
	if err := a.doctor(); err != nil {
		t.Error(err)
	}
	got := out.String()
	assertContainsAll(t, got,
		`"ok": true`,
		`"code": "task_acceptance_missing"`,
		`"code": "task_acceptance_placeholder"`,
		`"code": "task_acceptance_unchecked"`,
	)
}

func TestValidateTaskFrontMatterReportsParseErrors(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".ahm", "tasks", "active", "001.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	// Block scalar in front matter should produce a parse error, not missing-field errors.
	content := "---\n" +
		"id: 001\n" +
		"title: Bad\n" +
		"status: Pending\n" +
		"priority: P1\n" +
		"effort: M\n" +
		"labels: type:bug\n" +
		"exec_plan: -\n" +
		"depends_on: -\n" +
		"description: |\n" +
		"  multi\n" +
		"  line\n" +
		"---\n" +
		"# Bad\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	report := &validationReport{}
	validateTaskFrontMatter([]byte(content), relPath(root, path), report)
	if len(report.Errors) == 0 {
		t.Error("expected at least one error, got none")
	}
	found := false
	for _, e := range report.Errors {
		if e.Code == "task_malformed" && strings.Contains(e.Message, "unsupported block scalar") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected task_malformed error with block scalar message, got: %v", report.Errors)
	}
	// Verify no missing-field errors (which would be misleading)
	for _, e := range report.Errors {
		if e.Code == "task_missing_field" {
			t.Errorf("unexpected missing_field error when front matter is malformed: %v", e)
		}
	}
}

func writeCompletedTaskBody(t *testing.T, root string, id string, title string, body string) {
	t.Helper()
	writeFile(t, filepath.Join(root, ".ahm", "tasks", "completed", id+".md"), "---\n"+
		"id: "+id+"\n"+
		"title: "+title+"\n"+
		"status: Completed\n"+
		"priority: P2\n"+
		"effort: S\n"+
		"labels: type:task, area:tasks\n"+
		"exec_plan: -\n"+
		"depends_on: -\n"+
		"---\n"+
		"# "+title+"\n\n"+
		body)
}

func TestIsUncheckedChecklistItem(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  bool
	}{
		{
			name:  "unindented dash unchecked",
			lines: []string{"- [ ] Do it"},
			want:  true,
		},
		{
			name:  "unindented asterisk unchecked",
			lines: []string{"* [ ] Do it"},
			want:  true,
		},
		{
			name:  "indented dash unchecked",
			lines: []string{"  - [ ] Do it"},
			want:  true,
		},
		{
			name:  "indented asterisk unchecked",
			lines: []string{"  * [ ] Do it"},
			want:  true,
		},
		{
			name:  "tab indented asterisk unchecked",
			lines: []string{"\t* [ ] Do it"},
			want:  true,
		},
		{
			name:  "dash checked",
			lines: []string{"- [x] Done"},
			want:  false,
		},
		{
			name:  "asterisk checked",
			lines: []string{"* [x] Done"},
			want:  false,
		},
		{
			name:  "plain text",
			lines: []string{"just a line", "- not a checkbox"},
			want:  false,
		},
		{
			name:  "empty section",
			lines: []string{},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := false
			for _, line := range tt.lines {
				if isUncheckedChecklistItem(line) {
					got = true
				}
			}
			if got != tt.want {
				t.Errorf("isUncheckedChecklistItem() = %v, want %v", got, tt.want)
			}
		})
	}
}
