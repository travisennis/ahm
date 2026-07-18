package ahm

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

func TestADRCreateWritesParseableRecord(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "adr", "create", "Choose Storage Layout",
		"--description", "We need a durable storage decision.",
		"--status", "accepted",
		"--decision-makers", "Travis Ennis")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Errorf("create stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}

	path := filepath.Join(root, "docs", "adr", "001-choose-storage-layout.md")
	content := mustRead(t, path)
	assertContainsAll(t, content,
		"status: accepted",
		"date: ",
		"decision-makers: Travis Ennis",
		"# Choose Storage Layout",
		"## Context and Problem Statement",
		"We need a durable storage decision.",
		"## Decision Drivers",
		"## Decision Outcome",
	)
	adr, err := parseADR(path)
	if err != nil {
		t.Errorf("parse created ADR: %v\n%s", err, content)
	}
	if adr.ID != "001" || adr.Slug != "choose-storage-layout" || adr.Title != "Choose Storage Layout" {
		t.Errorf("created ADR identity = %#v", adr)
	}
}

func TestADRCreateBodyFile(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	bodyPath := filepath.Join(root, "body.md")
	body := "# Body File ADR\n\n## Context and Problem Statement\n\nCustom body.\n"
	if err := os.WriteFile(bodyPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "adr", "create", "Body File ADR", "--body-file", bodyPath)
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Errorf("create stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}

	content := mustRead(t, filepath.Join(root, "docs", "adr", "001-body-file-adr.md"))
	if got := strings.Count(content, "# Body File ADR"); got != 1 {
		t.Errorf("expected one generated H1, got %d:\n%s", got, content)
	}
	assertContainsAll(t, content, "status: proposed", "## Context and Problem Statement", "Custom body.")
	assertNotContains(t, content, "Chosen option: TODO")
}

func TestADRCreateBodyFileFromStdin(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	a := app{opts: options{root: root}, out: &out, in: strings.NewReader("## Context and Problem Statement\n\nFrom stdin.\n")}
	if err := a.adrCreateParsed(adrCreateArgs{title: "Stdin ADR", status: "proposed", bodyFile: "-"}); err != nil {
		t.Error(err)
	}
	if strings.TrimSpace(out.String()) != "001" {
		t.Errorf("create stdout = %q, want 001", out.String())
	}
	assertFileContainsAll(t, filepath.Join(root, "docs", "adr", "001-stdin-adr.md"), "From stdin.")
}

func TestADRCreateDryRun(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "--dry-run", "adr", "create", "Preview ADR")
	if code != 0 {
		t.Errorf("dry-run exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "create: ", "docs/adr/001-preview-adr.md", "id: 001")
	if _, err := os.Stat(filepath.Join(root, "docs", "adr", "001-preview-adr.md")); !os.IsNotExist(err) {
		t.Errorf("dry-run should not create ADR, err = %v", err)
	}
}

func TestADRCreateErrors(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	tests := []struct {
		name string
		args []string
		code int
		want string
	}{
		{name: "empty title", args: []string{"adr", "create", "   "}, code: 2, want: "adr create requires a title"},
		{name: "unsupported status", args: []string{"adr", "create", "Bad Status", "--status", "doing"}, code: 2, want: `unsupported ADR status "doing"`},
		{name: "unreadable body file", args: []string{"adr", "create", "Missing Body", "--body-file", filepath.Join(root, "missing.md")}, code: 1, want: "reading ADR body from"},
		{name: "conflicting body inputs", args: []string{"adr", "create", "Conflict", "--description", "summary", "--body-file", filepath.Join(root, "missing.md")}, code: 2, want: "--body-file or --description"},
		{name: "title with newline", args: []string{"adr", "create", "Bad\ntitle"}, code: 2, want: "adr create title must not contain newlines"},
		{name: "title with CRLF", args: []string{"adr", "create", "Bad\r\ntitle"}, code: 2, want: "adr create title must not contain newlines"},
		{name: "decision-makers with newline", args: []string{"adr", "create", "Bad DM", "--decision-makers", "Travis\nAlice"}, code: 2, want: "adr create decision-makers must not contain newlines"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, stderr, code := runCLI(t, append([]string{"--root", root}, tt.args...)...)
			if code != tt.code {
				t.Errorf("exit code = %d, stderr = %s", code, stderr)
			}
			if !strings.Contains(stderr, tt.want) {
				t.Errorf("stderr = %q, want %q", stderr, tt.want)
			}
		})
	}

	bodyPath := filepath.Join(root, "empty.md")
	if err := os.WriteFile(bodyPath, []byte("   \n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code = runCLI(t, "--root", root, "adr", "create", "Empty Body", "--body-file", bodyPath)
	if code != 2 {
		t.Errorf("exit code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "is empty") {
		t.Errorf("stderr = %q, want empty body error", stderr)
	}
}

func TestADRListOutputModesAndStatusFiltering(t *testing.T) {
	root := t.TempDir()
	writeADRFile(t, root, "001-legacy-decision.md", "# ADR 001: Legacy Decision\n\n**Status:** Accepted\n**Date:** 2026-06-01\n\n## Context\n\nLegacy body.\n")
	writeADRFile(t, root, "002-proposed-decision.md", "---\nstatus: proposed\ndate: 2026-06-02\n---\n# Proposed Decision\n\nBody.\n")
	writeADRFile(t, root, "003-superseded-decision.md", "---\nstatus: superseded by ADR-004\ndate: 2026-06-03\n---\n# Superseded Decision\n\nBody.\n")

	t.Run("text", func(t *testing.T) {
		stdout, stderr, code := runCLI(t, "--root", root, "adr", "list")
		if code != 0 {
			t.Errorf("list exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
		}
		assertContainsAll(t, stdout,
			"001 [Accepted] 2026-06-01 Legacy Decision",
			"002 [proposed] 2026-06-02 Proposed Decision",
			"003 [superseded by ADR-004] 2026-06-03 Superseded Decision",
		)
	})

	t.Run("plain", func(t *testing.T) {
		stdout, stderr, code := runCLI(t, "--root", root, "--plain", "adr", "list", "--status", "proposed")
		if code != 0 {
			t.Errorf("list exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
		}
		got := strings.TrimSpace(stdout)
		assertContainsAll(t, got, `"id":"002"`, `"title":"Proposed Decision"`, `"status":"proposed"`, `"date":"2026-06-02"`)
		assertNotContains(t, got, "Legacy Decision", "Superseded Decision")
	})

	t.Run("json superseded prefix", func(t *testing.T) {
		stdout, stderr, code := runCLI(t, "--root", root, "--json", "adr", "list", "--status", "superseded")
		if code != 0 {
			t.Errorf("list exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
		}
		assertContainsAll(t, stdout,
			`"id": "003"`,
			`"title": "Superseded Decision"`,
			`"status": "superseded by ADR-004"`,
		)
		assertNotContains(t, stdout, "Legacy Decision", "Proposed Decision")
	})
}

func TestADRShowOutputModes(t *testing.T) {
	root := t.TempDir()
	writeADRFile(t, root, "009-madr-adr-management.md", "---\nstatus: accepted\ndate: 2026-06-14\n---\n# MADR ADR Management\n\n## Context\n\nBody.\n")

	t.Run("text prints markdown", func(t *testing.T) {
		stdout, stderr, code := runCLI(t, "--root", root, "adr", "show", "9")
		if code != 0 {
			t.Errorf("show exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
		}
		assertContainsAll(t, stdout, "status: accepted", "# MADR ADR Management", "## Context")
	})

	t.Run("plain prints compact json", func(t *testing.T) {
		stdout, stderr, code := runCLI(t, "--root", root, "--plain", "adr", "show", "009-madr-adr-management")
		if code != 0 {
			t.Errorf("show exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
		}
		got := strings.TrimSpace(stdout)
		assertContainsAll(t, got, `"ID":"009"`, `"Title":"MADR ADR Management"`, `"Status":"accepted"`)
		assertNotContains(t, got, "\n  ")
	})

	t.Run("json prints parsed record", func(t *testing.T) {
		stdout, stderr, code := runCLI(t, "--root", root, "--json", "adr", "show", "009")
		if code != 0 {
			t.Errorf("show exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
		}
		assertContainsAll(t, stdout, `"ID": "009"`, `"Title": "MADR ADR Management"`, `"Status": "accepted"`)
	})
}

func TestADRStatusCommandsRewriteOnlyFrontMatter(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	path := filepath.Join(root, "docs", "adr", "001-status-decision.md")
	body := "# Status Decision\n\n## Context\n\nBody with trailing spaces.  \n\n## More Information\n\n- Keep this.\n"
	writeADRFile(t, root, "001-status-decision.md", "---\nstatus: proposed\ndate: 2000-01-01\nsource: hand-authored\n---\n"+body)

	stdout, stderr, code = runCLI(t, "--root", root, "adr", "accept", "001")
	if code != 0 {
		t.Errorf("accept exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "001 -> accepted")
	content := mustRead(t, path)
	assertContainsAll(t, content, "status: accepted", "date: ")
	assertNotContains(t, content, "date: 2000-01-01")
	if got := bodyAfterRawFrontMatter(t, content); got != body {
		t.Errorf("body changed\ngot:\n%q\nwant:\n%q", got, body)
	}
}

func TestADRStatusCommandNoOpPreservesFileAndReportsUnchanged(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "text", args: nil, want: []string{"001 already accepted"}},
		{name: "json", args: []string{"--json"}, want: []string{`"adr": "001"`, `"status": "accepted"`, `"date": "2000-01-01"`, `"changed": false`}},
		{name: "plain", args: []string{"--plain"}, want: []string{`"adr":"001"`, `"status":"accepted"`, `"date":"2000-01-01"`, `"changed":false`}},
		{name: "dry-run", args: []string{"--dry-run"}, want: []string{"adr: 001", "status: accepted", "date: 2000-01-01", "changed: false"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "docs", "adr", "001-accepted.md")
			writeADRFile(t, root, "001-accepted.md", "---\nstatus: accepted\ndate: 2000-01-01\n---\n# Accepted\n\nBody.\n")
			before := mustRead(t, path)

			args := append([]string{"--root", root}, tt.args...)
			args = append(args, "adr", "accept", "001")
			stdout, stderr, code := runCLI(t, args...)
			if code != 0 {
				t.Fatalf("exit code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
			}
			assertContainsAll(t, stdout, tt.want...)
			if after := mustRead(t, path); after != before {
				t.Errorf("ADR changed on no-op status update:\nbefore: %q\nafter:  %q", before, after)
			}
		})
	}
}

func TestADRSupersedeUpdatesBothRecordsAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	oldPath := filepath.Join(root, "docs", "adr", "001-old-decision.md")
	newPath := filepath.Join(root, "docs", "adr", "002-new-decision.md")
	writeADRFile(t, root, "001-old-decision.md", "---\nstatus: accepted\ndate: 2026-06-01\n---\n# Old Decision\n\n## Context\n\nOld body.\n\n## Supersession\n\nStale note.\n")
	writeADRFile(t, root, "002-new-decision.md", "---\nstatus: accepted\ndate: 2026-06-02\n---\n# New Decision\n\n## Context\n\nNew body.\n\n## More Information\n\n- Existing reference.\n")

	stdout, stderr, code = runCLI(t, "--root", root, "adr", "supersede", "001", "--by", "002")
	if code != 0 {
		t.Errorf("supersede exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "001 -> superseded by ADR-002")

	oldContent := mustRead(t, oldPath)
	assertContainsAll(t, oldContent, "status: superseded by ADR-002", "## Supersession", "Superseded by [ADR-002](002-new-decision.md).")
	assertNotContains(t, oldContent, "Stale note.")
	newContent := mustRead(t, newPath)
	assertContainsAll(t, newContent, "## More Information", "- Existing reference.", "- Supersedes [ADR-001](001-old-decision.md).")

	stdout, stderr, code = runCLI(t, "--root", root, "adr", "supersede", "001", "--by", "002")
	if code != 0 {
		t.Errorf("rerun supersede exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	oldContent = mustRead(t, oldPath)
	newContent = mustRead(t, newPath)
	if got := strings.Count(oldContent, "Superseded by [ADR-002](002-new-decision.md)."); got != 1 {
		t.Errorf("old supersession note count = %d\n%s", got, oldContent)
	}
	if got := strings.Count(newContent, "- Supersedes [ADR-001](001-old-decision.md)."); got != 1 {
		t.Errorf("new supersession reference count = %d\n%s", got, newContent)
	}
}

func TestADRSupersedePreservesCRLFBodyNewlines(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	oldPath := filepath.Join(root, "docs", "adr", "001-old-decision.md")
	newPath := filepath.Join(root, "docs", "adr", "002-new-decision.md")
	writeADRFile(t, root, "001-old-decision.md", strings.ReplaceAll("---\nstatus: accepted\ndate: 2026-06-01\n---\n# Old Decision\n\n## Context\n\nOld body.\n", "\n", "\r\n"))
	writeADRFile(t, root, "002-new-decision.md", strings.ReplaceAll("---\nstatus: accepted\ndate: 2026-06-02\n---\n# New Decision\n\n## More Information\n\n- Existing reference.\n", "\n", "\r\n"))

	stdout, stderr, code = runCLI(t, "--root", root, "adr", "supersede", "001", "--by", "002")
	if code != 0 {
		t.Errorf("supersede exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "001 -> superseded by ADR-002")

	oldContent := mustRead(t, oldPath)
	newContent := mustRead(t, newPath)
	assertContainsAll(t, oldContent, "status: superseded by ADR-002\r\n", "Superseded by [ADR-002](002-new-decision.md).")
	assertContainsAll(t, newContent, "- Existing reference.\r\n", "\r\n- Supersedes [ADR-001](001-old-decision.md).")
	if strings.Contains(strings.ReplaceAll(bodyAfterRawFrontMatter(t, oldContent), "\r\n", ""), "\n") {
		t.Errorf("old ADR body contains bare LF:\n%q", oldContent)
	}
	if strings.Contains(strings.ReplaceAll(bodyAfterRawFrontMatter(t, newContent), "\r\n", ""), "\n") {
		t.Errorf("new ADR body contains bare LF:\n%q", newContent)
	}
}

func TestADRSupersedeErrors(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Errorf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	writeADRFile(t, root, "001-old-decision.md", "---\nstatus: accepted\ndate: 2026-06-01\n---\n# Old Decision\n\nBody.\n")
	writeADRFile(t, root, "002-new-decision.md", "---\nstatus: accepted\ndate: 2026-06-02\n---\n# New Decision\n\nBody.\n")
	writeADRFile(t, root, "003-other-decision.md", "---\nstatus: accepted\ndate: 2026-06-03\n---\n# Other Decision\n\nBody.\n")
	writeADRFile(t, root, "004-superseded-decision.md", "---\nstatus: superseded by ADR-002\ndate: 2026-06-04\n---\n# Superseded Decision\n\nBody.\n")
	writeADRFile(t, root, "005-noncanonical-superseded.md", "---\nstatus: Superseded by ADR-002\ndate: 2026-06-05\n---\n# Non-Canonical Superseded\n\nBody.\n")
	writeADRFile(t, root, "006-proposed-old.md", "---\nstatus: proposed\ndate: 2026-06-06\n---\n# Proposed Old\n\nBody.\n")
	writeADRFile(t, root, "007-rejected-old.md", "---\nstatus: rejected\ndate: 2026-06-07\n---\n# Rejected Old\n\nBody.\n")
	writeADRFile(t, root, "008-deprecated-new.md", "---\nstatus: deprecated\ndate: 2026-06-08\n---\n# Deprecated New\n\nBody.\n")
	writeADRFile(t, root, "009-proposed-new.md", "---\nstatus: proposed\ndate: 2026-06-09\n---\n# Proposed New\n\nBody.\n")

	tests := []struct {
		name string
		args []string
		code int
		want string
	}{
		{name: "missing replacement", args: []string{"adr", "supersede", "001"}, code: 2, want: "adr supersede requires --by"},
		{name: "unknown old", args: []string{"adr", "supersede", "999", "--by", "002"}, code: 1, want: `ADR "999" not found`},
		{name: "unknown new", args: []string{"adr", "supersede", "001", "--by", "999"}, code: 1, want: `ADR "999" not found`},
		{name: "self", args: []string{"adr", "supersede", "001", "--by", "001"}, code: 2, want: "cannot supersede an ADR with itself"},
		{name: "already superseded elsewhere", args: []string{"adr", "supersede", "004", "--by", "003"}, code: 1, want: "ADR 004 is already superseded by ADR-002"},
		{name: "already superseded non-canonical casing", args: []string{"adr", "supersede", "005", "--by", "003"}, code: 1, want: "ADR 005 is already Superseded by ADR-002"},
		{name: "supersede from proposed", args: []string{"adr", "supersede", "006", "--by", "002"}, code: 1, want: "ADR 006 is proposed; only accepted ADRs can be superseded"},
		{name: "supersede from rejected", args: []string{"adr", "supersede", "007", "--by", "002"}, code: 1, want: "ADR 007 is rejected; only accepted ADRs can be superseded"},
		{name: "replacement is deprecated", args: []string{"adr", "supersede", "001", "--by", "008"}, code: 1, want: "replacement ADR 008 is deprecated; only accepted ADRs can be the replacement"},
		{name: "replacement is proposed", args: []string{"adr", "supersede", "001", "--by", "009"}, code: 1, want: "replacement ADR 009 is proposed; only accepted ADRs can be the replacement"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, stderr, code := runCLI(t, append([]string{"--root", root}, tt.args...)...)
			if code != tt.code {
				t.Errorf("exit code = %d, stderr = %s", code, stderr)
			}
			assertContainsAll(t, stderr, tt.want)
		})
	}
}

func TestADRCommandsTolerateMalformedRecords(t *testing.T) {
	root := t.TempDir()
	writeADRFile(t, root, "001-good-decision.md", "---\nstatus: accepted\ndate: 2026-06-01\n---\n# Good Decision\n\nBody.\n")
	writeADRFile(t, root, "002-bad-decision.md", "---\nstatus: accepted\n# Missing close\n")

	t.Run("list skips malformed record with warning", func(t *testing.T) {
		stdout, stderr, code := runCLI(t, "--root", root, "adr", "list")
		if code != 0 {
			t.Errorf("list exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
		}
		assertContainsAll(t, stdout, "001 [accepted] 2026-06-01 Good Decision")
		assertNotContains(t, stdout, "002")
		assertContainsAll(t, stderr, "warning:")
	})

	t.Run("show resolves valid record despite malformed record", func(t *testing.T) {
		stdout, stderr, code := runCLI(t, "--root", root, "adr", "show", "001")
		if code != 0 {
			t.Errorf("show exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
		}
		assertContainsAll(t, stdout, "# Good Decision")
		assertContainsAll(t, stderr, "warning:")
	})

	t.Run("show does not resolve malformed record", func(t *testing.T) {
		_, stderr, code := runCLI(t, "--root", root, "adr", "show", "002")
		if code != 1 {
			t.Errorf("show exit code = %d, stderr = %s", code, stderr)
		}
		assertContainsAll(t, stderr, `ADR "002" not found`)
	})
}

func TestADRMigrateDryRunReportsChanges(t *testing.T) {
	root := t.TempDir()
	writeADRFile(t, root, "001-legacy.md", "# ADR 001: Legacy\n\n**Status:** Accepted\n**Date:** 2026-06-01\n\nBody.\n")
	writeADRFile(t, root, "002-old.md", "# ADR 002: Old\n\n**Status:** Accepted\n**Date:** 2026-06-02\n\nBody.\n")

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "adr", "migrate")
	if code != 0 {
		t.Errorf("exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "migrations:", "docs/adr/001-legacy.md:", "docs/adr/002-old.md:")
}

func TestADRMigrateIdempotent(t *testing.T) {
	root := t.TempDir()
	writeADRFile(t, root, "001-legacy.md", "# ADR 001: Legacy\n\n**Status:** Accepted\n**Date:** 2026-06-01\n\nBody.\n")

	stdout, stderr, code := runCLI(t, "--root", root, "adr", "migrate")
	if code != 0 {
		t.Errorf("first migrate exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "migrated 1 ADR files")

	stdout, stderr, code = runCLI(t, "--root", root, "--dry-run", "adr", "migrate")
	if code != 0 {
		t.Errorf("second dry-run exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "No ADR migrations found")
}

func bodyAfterRawFrontMatter(t *testing.T, text string) string {
	t.Helper()
	_, body, _, ok, err := splitRawFrontMatter(text)
	if err != nil {
		t.Errorf("splitRawFrontMatter returned error: %v", err)
	}
	if !ok {
		t.Errorf("missing front matter:\n%s", text)
	}
	return body
}

func TestADRMigrateContentFormat(t *testing.T) {
	root := t.TempDir()
	writeADRFile(t, root, "001-legacy.md", "# ADR 001: Legacy Decision\n\n**Status:** Accepted\n**Date:** 2026-06-01\n\n## Context\n\nBody.\n")

	stdout, stderr, code := runCLI(t, "--root", root, "adr", "migrate")
	if code != 0 {
		t.Errorf("migrate exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "migrated 1 ADR files")

	path := filepath.Join(root, "docs", "adr", "001-legacy.md")
	content := mustRead(t, path)
	assertContainsAll(t, content,
		"---",
		"status: accepted",
		"date: 2026-06-01",
		"---",
		"# Legacy Decision",
		"## Context",
		"Body.",
	)
	assertNotContains(t, content, "ADR 001")
	assertNotContains(t, content, "**Status:**")
	assertNotContains(t, content, "**Date:**")
}

func TestADRMigratePartialSupersession(t *testing.T) {
	root := t.TempDir()
	writeADRFile(t, root, "006-legacy.md", "# ADR 006: Partially Superseded\n\n**Status:** Accepted, superseded in part by ADR 008\n**Date:** 2026-06-06\n\n## Context\n\nBody.\n")

	stdout, stderr, code := runCLI(t, "--root", root, "adr", "migrate")
	if code != 0 {
		t.Errorf("migrate exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "migrated 1 ADR files")

	content := mustRead(t, filepath.Join(root, "docs", "adr", "006-legacy.md"))
	assertContainsAll(t, content,
		"status: accepted",
		"# Partially Superseded",
		"## Supersession",
		"Superseded in part by ADR-008.",
		"## Context",
	)
	assertNotContains(t, content, "ADR 006")
	assertNotContains(t, content, "superseded in part by")
}

func TestADRMigrateSkipsAlreadyMigrated(t *testing.T) {
	root := t.TempDir()
	writeADRFile(t, root, "001-migrated.md", "---\nstatus: accepted\ndate: 2026-06-01\n---\n# Already Migrated\n\n## Context\n\nBody.\n")

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "adr", "migrate")
	if code != 0 {
		t.Errorf("dry-run exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "No ADR migrations found")
}

func TestADRMigratePreservesBodyContent(t *testing.T) {
	root := t.TempDir()
	body := "## Context\n\nThe old way was broken.\n\n## Decision\n\nWe will adopt MADR.\n\n## Rationale\n\nConsistency.\n\n## Consequences\n\n- Good.\n- Bad.\n"
	writeADRFile(t, root, "005-legacy.md", "# ADR 005: Body Preservation Test\n\n**Status:** Accepted\n**Date:** 2026-06-05\n\n"+body)

	stdout, stderr, code := runCLI(t, "--root", root, "adr", "migrate")
	if code != 0 {
		t.Errorf("migrate exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "migrated 1 ADR files")

	content := mustRead(t, filepath.Join(root, "docs", "adr", "005-legacy.md"))
	assertContainsAll(t, content,
		"## Context",
		"The old way was broken.",
		"## Decision",
		"We will adopt MADR.",
		"## Rationale",
		"Consistency.",
		"## Consequences",
		"- Good.",
		"- Bad.",
	)
	assertNotContains(t, content, "**Status:**")
	assertNotContains(t, content, "**Date:**")
}

func TestADRMigrateFullSupersession(t *testing.T) {
	root := t.TempDir()
	writeADRFile(t, root, "002-superseded.md", "# ADR 002: Superseded Decision\n\n**Status:** Superseded\n**Date:** 2026-06-02\n\nSuperseded by ADR 005.\n\n## Context\n\nOld decision body.\n")

	stdout, stderr, code := runCLI(t, "--root", root, "adr", "migrate")
	if code != 0 {
		t.Errorf("migrate exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "migrated 1 ADR files")

	content := mustRead(t, filepath.Join(root, "docs", "adr", "002-superseded.md"))
	assertContainsAll(t, content,
		"status: superseded by ADR-005",
		"# Superseded Decision",
		"## Context",
		"Old decision body.",
	)
	assertNotContains(t, content, "**Status:**")
	assertNotContains(t, content, "ADR 002")
}

func TestADRMigrateUnrecognizedStatus(t *testing.T) {
	root := t.TempDir()
	writeADRFile(t, root, "003-bad.md", "# ADR 003: Unknown Status\n\n**Status:** In Review\n**Date:** 2026-06-03\n\nBody.\n")

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "adr", "migrate")
	if code != 0 {
		t.Errorf("dry-run exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, `unrecognized status "In Review"; fix manually`)
}

func TestADRCreateParallelAllocatesUniqueIDs(t *testing.T) {
	root := t.TempDir()
	var installOut strings.Builder
	installer := app{opts: options{root: root}, out: &installOut}
	if err := installer.install(false); err != nil {
		t.Fatal(err)
	}

	const creates = 5
	type result struct {
		id    string
		title string
	}
	var wg sync.WaitGroup
	results := make(chan result, creates)
	errs := make(chan error, creates)
	for i := 0; i < creates; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out strings.Builder
			a := app{opts: options{root: root}, out: &out}
			parsed := adrCreateArgs{
				title:  fmt.Sprintf("Parallel ADR %d", i+1),
				status: "proposed",
			}
			err := a.adrCreateParsed(parsed)
			if err != nil {
				errs <- err
				return
			}
			results <- result{id: strings.TrimSpace(out.String()), title: parsed.title}
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	var got []string
	seen := map[string]bool{}
	for r := range results {
		got = append(got, r.id)
		if seen[r.id] {
			t.Errorf("duplicate id %s", r.id)
		}
		seen[r.id] = true
		slug := adrSlug(r.title)
		path := filepath.Join(root, "docs", "adr", r.id+"-"+slug+".md")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("adr %s (%s) not found at %s: %v", r.id, r.title, path, err)
		}
	}
	sort.Strings(got)
	want := []string{"001", "002", "003", "004", "005"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("parallel adr IDs = %v, want %v", got, want)
	}

	indexContent := mustRead(t, filepath.Join(root, "docs", "adr", "index.md"))
	for i := 0; i < creates; i++ {
		assertContainsAll(t, indexContent, fmt.Sprintf("Parallel ADR %d", i+1))
	}
}
func TestADRLifecycleTransitions(t *testing.T) {

	// Each transition group gets a unique ID range to avoid mutation
	// from earlier subtests leaking into later ones.

	t.Run("accept", func(t *testing.T) {
		root := t.TempDir()
		runCLI(t, "--root", root, "init")
		writeADRFile(t, root, "010-proposed.md", "---\nstatus: proposed\ndate: 2026-06-01\n---\n# Proposed\n\nBody.\n")
		writeADRFile(t, root, "011-accepted.md", "---\nstatus: accepted\ndate: 2026-06-02\n---\n# Accepted\n\nBody.\n")
		writeADRFile(t, root, "012-rejected.md", "---\nstatus: rejected\ndate: 2026-06-03\n---\n# Rejected\n\nBody.\n")
		writeADRFile(t, root, "013-deprecated.md", "---\nstatus: deprecated\ndate: 2026-06-04\n---\n# Deprecated\n\nBody.\n")
		writeADRFile(t, root, "014-superseded.md", "---\nstatus: superseded by ADR-011\ndate: 2026-06-05\n---\n# Superseded\n\nBody.\n")
		writeADRFile(t, root, "015-noncanonical.md", "---\nstatus: Superseded by ADR-011\ndate: 2026-06-06\n---\n# Non-Canonical\n\nBody.\n")

		tests := []struct {
			name string
			args []string
			code int
			want string
		}{
			{name: "from proposed", args: []string{"adr", "accept", "010"}, code: 0, want: "010 -> accepted"},
			{name: "idempotent", args: []string{"adr", "accept", "011"}, code: 0, want: "011 already accepted"},
			{name: "from rejected", args: []string{"adr", "accept", "012"}, code: 1, want: "ADR 012 is rejected; cannot set to accepted"},
			{name: "from deprecated", args: []string{"adr", "accept", "013"}, code: 1, want: "ADR 013 is deprecated; cannot set to accepted"},
			{name: "from superseded", args: []string{"adr", "accept", "014"}, code: 1, want: "ADR 014 is superseded by ADR-011; cannot set to accepted"},
			{name: "from noncanonical superseded", args: []string{"adr", "accept", "015"}, code: 1, want: "ADR 015 is Superseded by ADR-011; cannot set to accepted"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				gotStdout, gotStderr, gotCode := runCLI(t, append([]string{"--root", root}, tt.args...)...)
				if gotCode != tt.code {
					t.Errorf("exit code = %d, stdout = %q, stderr = %q", gotCode, gotStdout, gotStderr)
				}
				if tt.code == 0 {
					if !strings.Contains(gotStdout, tt.want) {
						t.Errorf("stdout = %q, want %q", gotStdout, tt.want)
					}
				} else {
					if !strings.Contains(gotStderr, tt.want) {
						t.Errorf("stderr = %q, want %q", gotStderr, tt.want)
					}
				}
			})
		}
	})

	t.Run("reject", func(t *testing.T) {
		root := t.TempDir()
		runCLI(t, "--root", root, "init")
		writeADRFile(t, root, "020-proposed.md", "---\nstatus: proposed\ndate: 2026-06-01\n---\n# Proposed\n\nBody.\n")
		writeADRFile(t, root, "021-accepted.md", "---\nstatus: accepted\ndate: 2026-06-02\n---\n# Accepted\n\nBody.\n")
		writeADRFile(t, root, "022-rejected.md", "---\nstatus: rejected\ndate: 2026-06-03\n---\n# Rejected\n\nBody.\n")
		writeADRFile(t, root, "023-deprecated.md", "---\nstatus: deprecated\ndate: 2026-06-04\n---\n# Deprecated\n\nBody.\n")
		writeADRFile(t, root, "024-superseded.md", "---\nstatus: superseded by ADR-021\ndate: 2026-06-05\n---\n# Superseded\n\nBody.\n")

		tests := []struct {
			name string
			args []string
			code int
			want string
		}{
			{name: "from proposed", args: []string{"adr", "reject", "020"}, code: 0, want: "020 -> rejected"},
			{name: "idempotent", args: []string{"adr", "reject", "022"}, code: 0, want: "022 already rejected"},
			{name: "from accepted", args: []string{"adr", "reject", "021"}, code: 1, want: "ADR 021 is accepted; cannot set to rejected"},
			{name: "from deprecated", args: []string{"adr", "reject", "023"}, code: 1, want: "ADR 023 is deprecated; cannot set to rejected"},
			{name: "from superseded", args: []string{"adr", "reject", "024"}, code: 1, want: "ADR 024 is superseded by ADR-021; cannot set to rejected"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				gotStdout, gotStderr, gotCode := runCLI(t, append([]string{"--root", root}, tt.args...)...)
				if gotCode != tt.code {
					t.Errorf("exit code = %d, stdout = %q, stderr = %q", gotCode, gotStdout, gotStderr)
				}
				if tt.code == 0 {
					if !strings.Contains(gotStdout, tt.want) {
						t.Errorf("stdout = %q, want %q", gotStdout, tt.want)
					}
				} else {
					if !strings.Contains(gotStderr, tt.want) {
						t.Errorf("stderr = %q, want %q", gotStderr, tt.want)
					}
				}
			})
		}
	})

	t.Run("deprecate", func(t *testing.T) {
		root := t.TempDir()
		runCLI(t, "--root", root, "init")
		writeADRFile(t, root, "030-proposed.md", "---\nstatus: proposed\ndate: 2026-06-01\n---\n# Proposed\n\nBody.\n")
		writeADRFile(t, root, "031-accepted.md", "---\nstatus: accepted\ndate: 2026-06-02\n---\n# Accepted\n\nBody.\n")
		writeADRFile(t, root, "032-rejected.md", "---\nstatus: rejected\ndate: 2026-06-03\n---\n# Rejected\n\nBody.\n")
		writeADRFile(t, root, "033-deprecated.md", "---\nstatus: deprecated\ndate: 2026-06-04\n---\n# Deprecated\n\nBody.\n")
		writeADRFile(t, root, "034-superseded.md", "---\nstatus: superseded by ADR-031\ndate: 2026-06-05\n---\n# Superseded\n\nBody.\n")

		tests := []struct {
			name string
			args []string
			code int
			want string
		}{
			{name: "from accepted", args: []string{"adr", "deprecate", "031"}, code: 0, want: "031 -> deprecated"},
			{name: "idempotent", args: []string{"adr", "deprecate", "033"}, code: 0, want: "033 already deprecated"},
			{name: "from proposed", args: []string{"adr", "deprecate", "030"}, code: 1, want: "ADR 030 is proposed; cannot set to deprecated"},
			{name: "from rejected", args: []string{"adr", "deprecate", "032"}, code: 1, want: "ADR 032 is rejected; cannot set to deprecated"},
			{name: "from superseded", args: []string{"adr", "deprecate", "034"}, code: 1, want: "ADR 034 is superseded by ADR-031; cannot set to deprecated"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				gotStdout, gotStderr, gotCode := runCLI(t, append([]string{"--root", root}, tt.args...)...)
				if gotCode != tt.code {
					t.Errorf("exit code = %d, stdout = %q, stderr = %q", gotCode, gotStdout, gotStderr)
				}
				if tt.code == 0 {
					if !strings.Contains(gotStdout, tt.want) {
						t.Errorf("stdout = %q, want %q", gotStdout, tt.want)
					}
				} else {
					if !strings.Contains(gotStderr, tt.want) {
						t.Errorf("stderr = %q, want %q", gotStderr, tt.want)
					}
				}
			})
		}
	})

	t.Run("propose", func(t *testing.T) {
		root := t.TempDir()
		runCLI(t, "--root", root, "init")
		writeADRFile(t, root, "040-proposed.md", "---\nstatus: proposed\ndate: 2026-06-01\n---\n# Proposed\n\nBody.\n")
		writeADRFile(t, root, "041-accepted.md", "---\nstatus: accepted\ndate: 2026-06-02\n---\n# Accepted\n\nBody.\n")
		writeADRFile(t, root, "042-rejected.md", "---\nstatus: rejected\ndate: 2026-06-03\n---\n# Rejected\n\nBody.\n")
		writeADRFile(t, root, "043-deprecated.md", "---\nstatus: deprecated\ndate: 2026-06-04\n---\n# Deprecated\n\nBody.\n")
		writeADRFile(t, root, "044-superseded.md", "---\nstatus: superseded by ADR-041\ndate: 2026-06-05\n---\n# Superseded\n\nBody.\n")
		writeADRFile(t, root, "045-noncanonical.md", "---\nstatus: Superseded by ADR-041\ndate: 2026-06-06\n---\n# Non-Canonical\n\nBody.\n")

		tests := []struct {
			name string
			args []string
			code int
			want string
		}{
			{name: "from accepted", args: []string{"adr", "propose", "041"}, code: 0, want: "041 -> proposed"},
			{name: "idempotent", args: []string{"adr", "propose", "040"}, code: 0, want: "040 already proposed"},
			{name: "from rejected", args: []string{"adr", "propose", "042"}, code: 1, want: "ADR 042 is rejected; cannot set to proposed"},
			{name: "from deprecated", args: []string{"adr", "propose", "043"}, code: 1, want: "ADR 043 is deprecated; cannot set to proposed"},
			{name: "from superseded", args: []string{"adr", "propose", "044"}, code: 1, want: "ADR 044 is superseded by ADR-041; cannot set to proposed"},
			{name: "from noncanonical superseded", args: []string{"adr", "propose", "045"}, code: 1, want: "ADR 045 is Superseded by ADR-041; cannot set to proposed"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				gotStdout, gotStderr, gotCode := runCLI(t, append([]string{"--root", root}, tt.args...)...)
				if gotCode != tt.code {
					t.Errorf("exit code = %d, stdout = %q, stderr = %q", gotCode, gotStdout, gotStderr)
				}
				if tt.code == 0 {
					if !strings.Contains(gotStdout, tt.want) {
						t.Errorf("stdout = %q, want %q", gotStdout, tt.want)
					}
				} else {
					if !strings.Contains(gotStderr, tt.want) {
						t.Errorf("stderr = %q, want %q", gotStderr, tt.want)
					}
				}
			})
		}
	})
}

func TestADRProposePreservesBodyAndRegeneratesIndex(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	path := filepath.Join(root, "docs", "adr", "001-status-decision.md")
	body := "# Status Decision\n\n## Context\n\nBody with trailing spaces.  \n\n## More Information\n\n- Keep this.\n"
	writeADRFile(t, root, "001-status-decision.md", "---\nstatus: accepted\ndate: 2000-01-01\nsource: hand-authored\n---\n"+body)

	stdout, stderr, code = runCLI(t, "--root", root, "adr", "propose", "001")
	if code != 0 {
		t.Fatalf("propose exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "001 -> proposed")
	content := mustRead(t, path)
	assertContainsAll(t, content, "status: proposed", "date: ")
	assertNotContains(t, content, "date: 2000-01-01")
	if got := bodyAfterRawFrontMatter(t, content); got != body {
		t.Errorf("body changed\ngot:\n%q\nwant:\n%q", got, body)
	}

	// Verify index is regenerated
	indexContent := mustRead(t, filepath.Join(root, "docs", "adr", "index.md"))
	assertContainsAll(t, indexContent, "001", "proposed")
}

func TestADRProposeDryRun(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	writeADRFile(t, root, "001-accepted.md", "---\nstatus: accepted\ndate: 2026-06-01\n---\n# Accepted ADR\n\nBody.\n")

	stdout, stderr, code = runCLI(t, "--root", root, "--dry-run", "adr", "propose", "001")
	if code != 0 {
		t.Errorf("dry-run exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "adr: 001", "status: proposed")
	content := mustRead(t, filepath.Join(root, "docs", "adr", "001-accepted.md"))
	assertContainsAll(t, content, "status: accepted")
	assertNotContains(t, content, "status: proposed")
}

func TestADRMigrateMissingBoldLines(t *testing.T) {
	root := t.TempDir()
	writeADRFile(t, root, "004-nodate.md", "# ADR 004: No Date\n\n**Status:** Accepted\n\n## Context\n\nBody.\n")

	stdout, stderr, code := runCLI(t, "--root", root, "--dry-run", "adr", "migrate")
	if code != 0 {
		t.Errorf("dry-run exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "missing Status or Date bold lines; fix manually")
}
