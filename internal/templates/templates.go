package templates

import (
	"embed"
	"strings"
)

// Version is the embedded workflow template version.
const Version = "0.4.1"

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
		Body: "`ahm` is for understanding and managing higher-order workflow records. It is\n" +
			"not the implementation route. Use it first when the request is about a managed\n" +
			"task, ExecPlan, ADR, or research note, then return to this AGENTS.md and choose\n" +
			"the route for the actual code, docs, CLI, safety, or release change.\n" +
			"\n" +
			"When asked to create, choose, update, or work on a task, run `ahm context task`,\n" +
			"inspect the relevant task with `ahm task ...`, and open the task file before\n" +
			"editing. When work calls for an ExecPlan, run `ahm context plan`. When it calls\n" +
			"for an ADR, run `ahm context adr` and use `ahm adr` commands for lifecycle\n" +
			"management. When asked to create, update, organize, or use research, run\n" +
			"`ahm context research` and use `.agents/.research/index.md` as the map.\n" +
			"\n" +
			"After `ahm` intake, re-classify the discovered work under the project's normal\n" +
			"workflow routing and load those routed docs before editing. State the selected\n" +
			"route and loaded docs in handoff so skipped routing is visible.",
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
