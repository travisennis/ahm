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
