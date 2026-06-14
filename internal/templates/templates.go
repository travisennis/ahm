package templates

import (
	"embed"
	"strings"
)

// Version is the embedded workflow template version.
const Version = "0.3.0"

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
	{Source: "workflow/AGENTS.md", Target: "AGENTS.md", CreateOnly: true},
	{Source: "workflow/TASKS.md", Target: ".agents/TASKS.md"},
	{Source: "workflow/PLANS.md", Target: ".agents/PLANS.md"},
	{Source: "workflow/RESEARCH.md", Target: ".agents/RESEARCH.md"},
	{Source: "workflow/DOCS.md", Target: ".agents/DOCS.md"},
	{Source: "workflow/tasks-README.md", Target: ".agents/.tasks/README.md"},
	{Source: "workflow/research-README.md", Target: ".agents/.research/README.md"},
	{Source: "workflow/deslop-SKILL.md", Target: ".agents/skills/deslop/SKILL.md"},
	{Source: "workflow/grooming-backlog-SKILL.md", Target: ".agents/skills/grooming-backlog/SKILL.md"},
	{Source: "workflow/adr-README.md", Target: "docs/adr/README.md"},
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
		Title: "AHM Workflow Routing",
		Body: "### Tasks\n" +
			"\n" +
			"When asked to create, choose, update, or work on a task, read\n" +
			"`.agents/TASKS.md`, then `.agents/.tasks/index.md`, then the specific task\n" +
			"file. Do not edit generated task indexes by hand; use `ahm` commands or\n" +
			"regenerate with `ahm index` when source metadata changes.\n" +
			"\n" +
			"### Research\n" +
			"\n" +
			"When asked to create, update, organize, or use research, read\n" +
			"`.agents/RESEARCH.md`, then use `.agents/.research/index.md` as the map.\n" +
			"\n" +
			"### ExecPlans\n" +
			"\n" +
			"Use `.agents/PLANS.md` for L/XL work and significant refactors or workflow\n" +
			"semantics changes.\n" +
			"\n" +
			"### Architecture Decision Records\n" +
			"\n" +
			"When creating, updating, or managing ADRs, read `docs/adr/README.md` for\n" +
			"the format and workflow rules. Use `ahm adr create`, `ahm adr accept`,\n" +
			"`ahm adr reject`, `ahm adr deprecate`, and `ahm adr supersede` for\n" +
			"ADR lifecycle management.\n" +
			"\n" +
			"### Documentation\n" +
			"\n" +
			"Before auditing or updating documentation, read `.agents/DOCS.md`.",
	},
	{
		ID:    "ahm-owned-files",
		Title: "AHM-Owned Files",
		Body: "Do not edit generated task, research, ExecPlan, or ADR indexes by hand. Update\n" +
			"the source records and run the appropriate `ahm` command.\n" +
			"\n" +
			"Use `ahm task complete <id>` and `ahm task cancel <id> --reason <text>` for\n" +
			"task state moves. Use `ahm adr` commands for ADR lifecycle changes.\n" +
			"\n" +
			"Treat `.agents/*` workflow guides and `docs/adr/README.md` as ahm-managed\n" +
			"templates. Change canonical guidance in the AHM repository, not through local\n" +
			"consumer edits.",
	},
}

// AgentSuggestions returns advisory AGENTS.md additions in starter-file order.
func AgentSuggestions() []AgentSuggestion {
	return agentSuggestions
}

// RenderAgentsMarkdown renders the starter AGENTS.md from advisory suggestions.
func RenderAgentsMarkdown() string {
	blocks := []string{"# Agent Instructions"}
	for _, suggestion := range AgentSuggestions() {
		blocks = append(blocks, "## "+suggestion.Title+"\n\n"+suggestion.Body)
	}
	return strings.Join(blocks, "\n\n") + "\n"
}
