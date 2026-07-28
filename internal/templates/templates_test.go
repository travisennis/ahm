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
			path: "workflow/ADR.md",
			want: "# Architecture Decision Records",
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

func TestTasksTemplateRoutesToResearch(t *testing.T) {
	data, err := fs.ReadFile(FS, "workflow/TASKS.md")
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, "run `ahm context research`") {
		t.Fatal("tasks template should route to research context")
	}
	if strings.Contains(body, "ahm context docs") {
		t.Fatal("tasks template should not route to a removed docs context")
	}
	if !strings.Contains(body, "project's own documentation guidance") {
		t.Fatal("tasks template should defer documentation policy to the project")
	}
	if !strings.Contains(body, "conceptual order: research evidence,") {
		t.Fatal("tasks template should state conceptual order of research -> ADR -> ExecPlan")
	}
}

func TestTasksTemplatePreservesADRandExecPlanTriggers(t *testing.T) {
	data, err := fs.ReadFile(FS, "workflow/TASKS.md")
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, s := range []string{
		"ahm context adr",
		"durable",
		"architectural decision",
		"ahm context plan",
		"cross-cutting or substantially uncertain",
		"Before implementation",
		"Implement only the task's problem",
		"Run the repository's routed verification",
		"ahm task complete",
		"must run before any git commit",
	} {
		if !strings.Contains(body, s) {
			t.Fatalf("tasks template should preserve %q", s)
		}
	}
}

func TestTasksTemplateRecordsDocsInAcceptanceNotes(t *testing.T) {
	data, err := fs.ReadFile(FS, "workflow/TASKS.md")
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, "Record the") {
		t.Fatal("tasks template should record docs disposition in Acceptance Notes")
	}
	if !strings.Contains(body, "documents checked and updated") {
		t.Fatal("tasks template should mention docs checked and updated")
	}
	if !strings.Contains(body, "Do not require documentation changes for every task") {
		t.Fatal("tasks template should not require docs for every task")
	}
}

func TestResearchTemplateHasDurableCreationThreshold(t *testing.T) {
	data, err := fs.ReadFile(FS, "workflow/RESEARCH.md")
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, "Create durable research") {
		t.Fatal("research template should define a durable-research creation trigger")
	}
	if !strings.Contains(body, "not every question during") {
		t.Fatal("research template should state a concrete non-trigger for transient investigation")
	}
}

func TestResearchTemplateConnectsToADRs(t *testing.T) {
	data, err := fs.ReadFile(FS, "workflow/RESEARCH.md")
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, "Feed evidence to an") {
		t.Fatal("research template should connect evidence to ADRs")
	}
	if !strings.Contains(body, "**ADRs** are authoritative for architectural decisions") {
		t.Fatal("research template should state ADR authority")
	}
	if !strings.Contains(body, "**Tasks** are authoritative for actionable scope") {
		t.Fatal("research template should state task authority")
	}
	if !strings.Contains(body, "**ExecPlans** are authoritative for implementation plans") {
		t.Fatal("research template should state ExecPlan authority")
	}
}

func TestResearchTemplatePreservesStorageAndIndexGuidance(t *testing.T) {
	data, err := fs.ReadFile(FS, "workflow/RESEARCH.md")
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, s := range []string{
		"`inbox/` for raw ideas",
		"`investigations/` for project-specific findings",
		"`sources/` for notes from external",
		"`topics/` for synthesized, durable notes",
		"`archived/` for stale or superseded",
		"Inbox notes must eventually receive a disposition",
		"regenerate the indexes with `ahm index`",
	} {
		if !strings.Contains(body, s) {
			t.Fatalf("research template should preserve %q", s)
		}
	}
}
