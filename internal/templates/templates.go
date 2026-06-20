package templates

import (
	"embed"
	"strings"
)

// Version is the embedded workflow template version.
const Version = "0.4.0"

// FS contains the embedded workflow template files.
//
//go:embed workflow/*
var FS embed.FS

// File maps one embedded template source to its repository target path.
type File struct {
	Source     string
	Target     string
	CreateOnly bool
}

var managedFiles = []File{
	{Source: "workflow/preflight-SKILL.md", Target: ".agents/skills/preflight/SKILL.md"},
	{Source: "workflow/grooming-backlog-SKILL.md", Target: ".agents/skills/grooming-backlog/SKILL.md"},
	{Source: "workflow/finding-improvements-SKILL.md", Target: ".agents/skills/finding-improvements/SKILL.md"},
}

// Files returns the managed workflow files embedded in the CLI.
func Files() []File {
	return managedFiles
}

// AgentSuggestion is one advisory block that may be added to a project-owned
// AGENTS.md.
type AgentSuggestion struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

var agentSuggestions = []AgentSuggestion{
	{
		ID:    "ahm-workflow-routing",
		Title: "ahm Workflow Routing",
		Body: "Start by running `ahm context` for the current repository briefing.\n" +
			"Use `ahm --json context` when a structured response is more useful than\n" +
			"agent-readable Markdown.\n" +
			"\n" +
			"### Tasks\n" +
			"\n" +
			"When asked to create, choose, update, or work on a task, run\n" +
			"`ahm context task`, then use `ahm task next`, `ahm task ready`,\n" +
			"`ahm task list`, `ahm task blocked`, or `ahm task show <id>` to inspect\n" +
			"task state before acting. Do not edit generated task indexes by hand; use\n" +
			"`ahm` commands or regenerate with `ahm index` when source metadata changes.\n" +
			"\n" +
			"### Research\n" +
			"\n" +
			"When asked to create, update, organize, or use research, run\n" +
			"`ahm context research`, then use `.agents/.research/index.md` as the map.\n" +
			"\n" +
			"### ExecPlans\n" +
			"\n" +
			"Run `ahm context plan` before L/XL work and significant refactors or workflow\n" +
			"semantics changes that need an ExecPlan.\n" +
			"\n" +
			"### Architecture Decision Records\n" +
			"\n" +
			"When creating, updating, or managing ADRs, use `ahm context adr` for\n" +
			"the format and workflow rules, then use `ahm adr create`, `ahm adr accept`,\n" +
			"`ahm adr reject`, `ahm adr deprecate`, and `ahm adr supersede` for\n" +
			"ADR lifecycle management.\n" +
			"\n" +
			"### Documentation\n" +
			"\n" +
			"Before auditing or updating documentation, run `ahm context docs`.",
	},
	{
		ID:    "ahm-owned-files",
		Title: "ahm-Owned Files",
		Body: "Do not edit generated task, research, ExecPlan, or ADR indexes by hand. Update\n" +
			"the source records and run the appropriate `ahm` command.\n" +
			"\n" +
			"Use `ahm task complete <id>` and `ahm task cancel <id> --reason <text>` for\n" +
			"task state moves. Use `ahm adr` commands for ADR lifecycle changes.\n" +
			"\n" +
			"Treat `ahm context` output as the canonical workflow guidance. Do not\n" +
			"recreate removed workflow guide files; those instructions now come from the\n" +
			"`ahm` binary.",
	},
}

// AgentSuggestions returns advisory AGENTS.md additions in starter-file order.
func AgentSuggestions() []AgentSuggestion {
	return agentSuggestions
}

// RenderAgentsMarkdown renders advisory AGENTS.md content from suggestions.
func RenderAgentsMarkdown() string {
	blocks := []string{"# Agent Instructions"}
	for _, suggestion := range AgentSuggestions() {
		blocks = append(blocks, "## "+suggestion.Title+"\n\n"+suggestion.Body)
	}
	return strings.Join(blocks, "\n\n") + "\n"
}
