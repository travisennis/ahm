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
		"## Task Workflow",
		"For the first task in a session",
		"## Ahm-Owned Files",
		"Do not hand-edit ahm-owned generated indexes",
		"## Generated Indexes",
		"Do not edit generated indexes by hand",
		"## Implementation Documentation",
	)
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
	assertNotContains(t, stdout, "## Task Workflow")

	stdout, stderr, code = runCLI(t, "--root", root, "agents", "suggestions", "--all")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"## Task Workflow",
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
		`"id": "task-workflow"`,
		`"present": true`,
		`"id": "ahm-owned-files"`,
		`"title": "Ahm-Owned Files"`,
	)
}
