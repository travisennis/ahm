package templates

import (
	"io/fs"
	"strings"
	"testing"
)

func TestWorkflowTemplatesKeepCoreGuidance(t *testing.T) {
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
			want: "Follow this procedure for every implementation task",
		},
		{
			path: "workflow/RESEARCH.md",
			want: "Research should usually flow from rough capture to durable project work",
		},
		{
			path: "workflow/DOCS.md",
			want: "Treat the repository's existing docs as the source",
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

func TestManagedFilesContainScaffoldTargets(t *testing.T) {
	files := Files()
	if len(files) == 0 {
		t.Fatal("managed files should contain at least one scaffold target")
	}
	// Verify the .ahm/ scaffold targets are present.
	targets := make(map[string]bool)
	for _, f := range files {
		targets[f.Target] = true
	}
	for _, want := range []string{
		".ahm/tasks/README.md",
		".ahm/research/README.md",
		"docs/adr/README.md",
	} {
		if !targets[want] {
			t.Errorf("missing managed file target %q", want)
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
