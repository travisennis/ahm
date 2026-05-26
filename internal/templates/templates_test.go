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
			path: "workflow/deslop-SKILL.md",
			want: "Scale the review to the change size",
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

func TestManagedFilesIncludeADRTemplate(t *testing.T) {
	for _, file := range Files() {
		if file.Target == "docs/adr/README.md" {
			return
		}
	}
	t.Fatal("docs/adr/README.md is not managed")
}

func TestDeslopTemplateIsProjectGeneric(t *testing.T) {
	data, err := fs.ReadFile(FS, "workflow/deslop-SKILL.md")
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
			t.Fatalf("deslop template should be project-generic, but contains %q", term)
		}
	}
}
