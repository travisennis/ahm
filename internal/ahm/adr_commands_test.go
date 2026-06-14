package ahm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestADRCreateWritesParseableRecord(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "adr", "create", "Choose Storage Layout",
		"--description", "We need a durable storage decision.",
		"--status", "accepted",
		"--decision-makers", "Travis Ennis")
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Fatalf("create stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
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
		t.Fatalf("parse created ADR: %v\n%s", err, content)
	}
	if adr.ID != "001" || adr.Slug != "choose-storage-layout" || adr.Title != "Choose Storage Layout" {
		t.Fatalf("created ADR identity = %#v", adr)
	}
}

func TestADRCreateBodyFile(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	bodyPath := filepath.Join(root, "body.md")
	body := "# Body File ADR\n\n## Context and Problem Statement\n\nCustom body.\n"
	if err := os.WriteFile(bodyPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "adr", "create", "Body File ADR", "--body-file", bodyPath)
	if code != 0 || strings.TrimSpace(stdout) != "001" {
		t.Fatalf("create stdout = %q, stderr = %q, code = %d", stdout, stderr, code)
	}

	content := mustRead(t, filepath.Join(root, "docs", "adr", "001-body-file-adr.md"))
	if got := strings.Count(content, "# Body File ADR"); got != 1 {
		t.Fatalf("expected one generated H1, got %d:\n%s", got, content)
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
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "001" {
		t.Fatalf("create stdout = %q, want 001", out.String())
	}
	assertFileContainsAll(t, filepath.Join(root, "docs", "adr", "001-stdin-adr.md"), "From stdin.")
}

func TestADRCreateDryRun(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, "--root", root, "--dry-run", "adr", "create", "Preview ADR")
	if code != 0 {
		t.Fatalf("dry-run exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
	}
	assertContainsAll(t, stdout, "create: ", "docs/adr/001-preview-adr.md", "id: 001")
	if _, err := os.Stat(filepath.Join(root, "docs", "adr", "001-preview-adr.md")); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not create ADR, err = %v", err)
	}
}

func TestADRCreateErrors(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLI(t, "--root", root, "init")
	if code != 0 {
		t.Fatalf("init exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, stderr, code := runCLI(t, append([]string{"--root", root}, tt.args...)...)
			if code != tt.code {
				t.Fatalf("exit code = %d, stderr = %s", code, stderr)
			}
			if !strings.Contains(stderr, tt.want) {
				t.Fatalf("stderr = %q, want %q", stderr, tt.want)
			}
		})
	}

	bodyPath := filepath.Join(root, "empty.md")
	if err := os.WriteFile(bodyPath, []byte("   \n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code = runCLI(t, "--root", root, "adr", "create", "Empty Body", "--body-file", bodyPath)
	if code != 2 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "is empty") {
		t.Fatalf("stderr = %q, want empty body error", stderr)
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
			t.Fatalf("list exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
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
			t.Fatalf("list exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
		}
		got := strings.TrimSpace(stdout)
		assertContainsAll(t, got, `"id":"002"`, `"title":"Proposed Decision"`, `"status":"proposed"`, `"date":"2026-06-02"`)
		assertNotContains(t, got, "Legacy Decision", "Superseded Decision")
	})

	t.Run("json superseded prefix", func(t *testing.T) {
		stdout, stderr, code := runCLI(t, "--root", root, "--json", "adr", "list", "--status", "superseded")
		if code != 0 {
			t.Fatalf("list exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
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
			t.Fatalf("show exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
		}
		assertContainsAll(t, stdout, "status: accepted", "# MADR ADR Management", "## Context")
	})

	t.Run("plain prints compact json", func(t *testing.T) {
		stdout, stderr, code := runCLI(t, "--root", root, "--plain", "adr", "show", "009-madr-adr-management")
		if code != 0 {
			t.Fatalf("show exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
		}
		got := strings.TrimSpace(stdout)
		assertContainsAll(t, got, `"ID":"009"`, `"Title":"MADR ADR Management"`, `"Status":"accepted"`)
		assertNotContains(t, got, "\n  ")
	})

	t.Run("json prints parsed record", func(t *testing.T) {
		stdout, stderr, code := runCLI(t, "--root", root, "--json", "adr", "show", "009")
		if code != 0 {
			t.Fatalf("show exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
		}
		assertContainsAll(t, stdout, `"ID": "009"`, `"Title": "MADR ADR Management"`, `"Status": "accepted"`)
	})
}

func TestADRCommandsTolerateMalformedRecords(t *testing.T) {
	root := t.TempDir()
	writeADRFile(t, root, "001-good-decision.md", "---\nstatus: accepted\ndate: 2026-06-01\n---\n# Good Decision\n\nBody.\n")
	writeADRFile(t, root, "002-bad-decision.md", "---\nstatus: accepted\n# Missing close\n")

	t.Run("list skips malformed record with warning", func(t *testing.T) {
		stdout, stderr, code := runCLI(t, "--root", root, "adr", "list")
		if code != 0 {
			t.Fatalf("list exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
		}
		assertContainsAll(t, stdout, "001 [accepted] 2026-06-01 Good Decision")
		assertNotContains(t, stdout, "002")
		assertContainsAll(t, stderr, "warning:")
	})

	t.Run("show resolves valid record despite malformed record", func(t *testing.T) {
		stdout, stderr, code := runCLI(t, "--root", root, "adr", "show", "001")
		if code != 0 {
			t.Fatalf("show exit code = %d, stdout = %s, stderr = %s", code, stdout, stderr)
		}
		assertContainsAll(t, stdout, "# Good Decision")
		assertContainsAll(t, stderr, "warning:")
	})

	t.Run("show does not resolve malformed record", func(t *testing.T) {
		_, stderr, code := runCLI(t, "--root", root, "adr", "show", "002")
		if code != 1 {
			t.Fatalf("show exit code = %d, stderr = %s", code, stderr)
		}
		assertContainsAll(t, stderr, `ADR "002" not found`)
	})
}
