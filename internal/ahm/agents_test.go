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
		t.Errorf("exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"# Suggested AGENTS.md Integration",
		"## How To Apply",
		"If `AGENTS.md` already has an Operating Loop",
		"session-start `ahm prime` step",
		"If the file has neither an Operating Loop nor workflow routing",
		"## Operating Loop Integration",
		"Before any work, run `ahm prime`",
		"Do managed-work intake",
		"skip `ahm` intake and classify the request",
		"## Managed Work Intake With `ahm`",
		"`ahm` is for understanding and managing higher-order workflow records",
		"inspect the relevant task with `ahm task ...`",
		"Session start: run `ahm prime`",
		"re-classify the discovered work under the repository's",
		"## ahm-Owned Files",
		"Never hand-edit generated task, research, ExecPlan, or ADR indexes",
		"Use `ahm task` commands",
		"owns workflow routing",
	)
	assertNotContains(t, stdout, "Do not commit or push unless explicitly asked.")
	assertNotContains(t, stdout, "### Tasks")
	assertFileContainsAll(t, agentsPath, "Keep this.")
	if got := mustRead(t, agentsPath); got != original {
		t.Errorf("AGENTS.md was modified:\n%s", got)
	}
}

func TestAgentsSuggestionsOmitsPresentBlocksUnlessAll(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "AGENTS.md"), templates.RenderAgentsMarkdown())

	stdout, stderr, code := runCLI(t, "--root", root, "agents", "suggestions")
	if code != 0 {
		t.Errorf("exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout, "No missing suggestions detected.")
	assertNotContains(t, stdout, "## Managed Work Intake With `ahm`")

	stdout, stderr, code = runCLI(t, "--root", root, "agents", "suggestions", "--all")
	if code != 0 {
		t.Errorf("exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		"## How To Apply",
		"## Operating Loop Integration",
		"## Managed Work Intake With `ahm`",
		"_Already appears present in AGENTS.md._",
	)
}

func TestAgentsSuggestionsJSONIncludesPresence(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "AGENTS.md"), templates.RenderAgentsMarkdown())

	stdout, stderr, code := runCLI(t, "--root", root, "--json", "agents", "suggestions")
	if code != 0 {
		t.Errorf("exit code = %d, stderr = %s", code, stderr)
	}
	assertContainsAll(t, stdout,
		`"target": "AGENTS.md"`,
		`"exists": true`,
		`"id": "ahm-apply-guidance"`,
		`"id": "operating-loop-integration"`,
		`"id": "ahm-workflow-routing"`,
		`"present": true`,
		`"id": "ahm-owned-files"`,
		`"title": "ahm-Owned Files"`,
	)
}
