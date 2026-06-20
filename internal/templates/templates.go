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
		Body: "Run `ahm context` for the current repository briefing before starting work.\n" +
			"Use `ahm --json context` when a structured response is more useful than\n" +
			"agent-readable Markdown.\n" +
			"\n" +
			"When asked to create, choose, update, or work on a task, run `ahm context task`,\n" +
			"then inspect task state with `ahm task` commands before acting. When work\n" +
			"calls for an ExecPlan, run `ahm context plan`. When it calls for an ADR, run\n" +
			"`ahm context adr` and use `ahm adr` commands for lifecycle management. When\n" +
			"asked to create, update, organize, or use research, run `ahm context research`,\n" +
			"then use `.agents/.research/index.md` as the map. Before auditing or updating\n" +
			"documentation, run `ahm context docs`.",
	},
	{
		ID:    "ahm-owned-files",
		Title: "ahm-Owned Files",
		Body: "Never hand-edit generated task, research, ExecPlan, or ADR indexes. Update the\n" +
			"source records and run the appropriate `ahm` command. Use `ahm task` commands\n" +
			"for task state moves and `ahm adr` commands for ADR lifecycle changes. Treat\n" +
			"`ahm context` output as the canonical workflow guidance — do not recreate\n" +
			"removed workflow guide files; those instructions now come from the `ahm` binary.",
	},
}

// AgentSuggestions returns advisory AGENTS.md additions in starter-file order.
func AgentSuggestions() []AgentSuggestion {
	return agentSuggestions
}

// RenderAgentsMarkdown renders all suggestion blocks as a single AGENTS.md
// string. Used by tests to build fixtures where every block is present.
func RenderAgentsMarkdown() string {
	blocks := []string{"# Agent Instructions"}
	for _, suggestion := range AgentSuggestions() {
		blocks = append(blocks, "## "+suggestion.Title+"\n\n"+suggestion.Body)
	}
	return strings.Join(blocks, "\n\n") + "\n"
}
