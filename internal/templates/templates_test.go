package templates

import (
	"io/fs"
	"strings"
	"testing"
)

func TestWorkflowTemplatesKeepScaffoldDetail(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{
			path: "workflow/PLANS.md",
			want: "NON-NEGOTIABLE REQUIREMENTS",
		},
		{
			path: "workflow/TASKS.md",
			want: "## Choosing Work",
		},
		{
			path: "workflow/RESEARCH.md",
			want: "Research should usually flow from rough capture to durable project work",
		},
		{
			path: "workflow/DOCS.md",
			want: "Treat the repository's existing docs as the source",
		},
		{
			path: "workflow/preflight-SKILL.md",
			want: "Scale the review to the change size",
		},
		{
			path: "workflow/grooming-backlog-SKILL.md",
			want: "Every Open task is either Pending (ready to work), Blocked (blocker",
		},
		{
			path: "workflow/finding-improvements-SKILL.md",
			want: "Survey a codebase as a senior advisor",
		},
	}
	for _, tc := range cases {
		data, err := fs.ReadFile(FS, tc.path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), tc.want) {
			t.Fatalf("%s does not contain %q", tc.path, tc.want)
		}
	}
}

func TestManagedFilesOnlyInstallSkills(t *testing.T) {
	want := map[string]bool{
		".agents/skills/preflight/SKILL.md":            true,
		".agents/skills/grooming-backlog/SKILL.md":     true,
		".agents/skills/finding-improvements/SKILL.md": true,
	}
	files := Files()
	if len(files) != len(want) {
		t.Fatalf("managed files = %#v, want only skills", files)
	}
	for _, file := range files {
		if !want[file.Target] {
			t.Fatalf("unexpected managed file %q", file.Target)
		}
		delete(want, file.Target)
	}
	if len(want) > 0 {
		t.Fatalf("missing managed skill files: %#v", want)
	}
}

func TestPreflightTemplateIsProjectGeneric(t *testing.T) {
	data, err := fs.ReadFile(FS, "workflow/preflight-SKILL.md")
	if err != nil {
		t.Fatal(err)
	}

	body := string(data)
	for _, term := range []string{
		"Rust-level",
		"anyhow",
		"thiserror",
		"serde_json",
		"Tokio",
		"OpenAI-compatible",
		"cargo fmt",
		"cargo clippy",
		"clippy-strict",
	} {
		if strings.Contains(body, term) {
			t.Fatalf("preflight template should be project-generic, but contains %q", term)
		}
	}
}

func TestGroomingBacklogTemplateIsProjectGeneric(t *testing.T) {
	data, err := fs.ReadFile(FS, "workflow/grooming-backlog-SKILL.md")
	if err != nil {
		t.Fatal(err)
	}

	body := string(data)
	for _, term := range []string{
		"Cobra",
		"cargo",
		"npm",
		"preflight",
		"052",
		"053",
		"051",
	} {
		if strings.Contains(body, term) {
			t.Fatalf("grooming-backlog template should be project-generic, but contains %q", term)
		}
	}
}

func TestFindingImprovementsTemplateIsProjectGeneric(t *testing.T) {
	data, err := fs.ReadFile(FS, "workflow/finding-improvements-SKILL.md")
	if err != nil {
		t.Fatal(err)
	}

	body := string(data)
	// These are ahm-project-specific terms. Ecosystem tool references like
	// "cargo audit", "npm audit", "pip-audit", and "go vulncheck" are
	// legitimate cross-project examples and intentionally allowed.
	for _, term := range []string{
		"Cobra",
		"preflight",
		"052",
		"053",
		"051",
	} {
		if strings.Contains(body, term) {
			t.Fatalf("finding-improvements template should be project-generic, but contains %q", term)
		}
	}
}

func TestDocsTemplateIsProjectGeneric(t *testing.T) {
	data, err := fs.ReadFile(FS, "workflow/DOCS.md")
	if err != nil {
		t.Fatal(err)
	}

	body := string(data)
	for _, term := range []string{
		"Go",
		"Cobra",
		"CLI reference",
		"docs/cli.md",
		"just ci",
		"go test",
		"cargo",
		"npm",
	} {
		if strings.Contains(body, term) {
			t.Fatalf("docs template should be project-generic, but contains %q", term)
		}
	}
}
