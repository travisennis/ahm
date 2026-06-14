package ahm

import (
	"path/filepath"
	"testing"

	"github.com/travisennis/ahm/internal/templates"
)

func TestAgentsSuggestionsPrintsMissingMarkdownWithoutWriting(t *testing.T) {
	root := t.TempDir()
	agentsPath := filepath.Join(root, "AGENTS.md")
	original := "# Project Agent Instructions\n\nKeep this.\n"
	writeFile(t, agentsPath, original)

	stdout, stderr, code := runCLI(t, "--root", root, "agents", "suggestions")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"# Suggested AGENTS.md Additions",
		"## AHM Workflow Routing",
		"### Tasks",
		"When asked to create, choose, update, or work on a task",
		"## AHM-Owned Files",
		"Do not edit generated task, research, ExecPlan, or ADR indexes by hand",
		"Use `ahm task complete <id>` and `ahm task cancel <id> --reason <text>` for",
	)
	assertNotContains(t, stdout, "Do not commit or push unless explicitly asked.")
	assertNotContains(t, stdout, "## Operating Loop")
	assertFileContainsAll(t, agentsPath, "Keep this.")
	if got := mustRead(t, agentsPath); got != original {
		t.Fatalf("AGENTS.md was modified:\n%s", got)
	}
}

func TestAgentsSuggestionsOmitsPresentBlocksUnlessAll(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "AGENTS.md"), templates.RenderAgentsMarkdown())

	stdout, stderr, code := runCLI(t, "--root", root, "agents", "suggestions")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "No missing suggestions detected.")
	assertNotContains(t, stdout, "## AHM Workflow Routing")

	stdout, stderr, code = runCLI(t, "--root", root, "agents", "suggestions", "--all")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"## AHM Workflow Routing",
		"_Already appears present in AGENTS.md._",
	)
}

func TestAgentsSuggestionsJSONIncludesPresence(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "AGENTS.md"), templates.RenderAgentsMarkdown())

	stdout, stderr, code := runCLI(t, "--root", root, "--json", "agents", "suggestions")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		`"target": "AGENTS.md"`,
		`"exists": true`,
		`"id": "ahm-workflow-routing"`,
		`"present": true`,
		`"id": "ahm-owned-files"`,
		`"title": "AHM-Owned Files"`,
	)
}
