package templates

import (
	"embed"
	"strings"
)

// Version is the embedded workflow template version.
var Version = "0.2.0"

// FS contains the embedded workflow template files.
//
//go:embed workflow/**
var FS embed.FS

// File maps one embedded template source to its repository target path.
type File struct {
	Source     string
	Target     string
	CreateOnly bool
}

// Files returns the managed workflow files embedded in the CLI.
func Files() []File {
	return []File{
		{Source: "workflow/AGENTS.md", Target: "AGENTS.md", CreateOnly: true},
		{Source: "workflow/TASKS.md", Target: ".agents/TASKS.md"},
		{Source: "workflow/PLANS.md", Target: ".agents/PLANS.md"},
		{Source: "workflow/RESEARCH.md", Target: ".agents/RESEARCH.md"},
		{Source: "workflow/DOCS.md", Target: ".agents/DOCS.md"},
		{Source: "workflow/tasks-README.md", Target: ".agents/.tasks/README.md"},
		{Source: "workflow/research-README.md", Target: ".agents/.research/README.md"},
		{Source: "workflow/deslop-SKILL.md", Target: ".agents/skills/deslop/SKILL.md"},
		{Source: "workflow/adr-README.md", Target: "docs/adr/README.md"},
	}
}

// AgentSuggestion is one advisory block that may be added to a project-owned
// AGENTS.md.
type AgentSuggestion struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

// AgentSuggestions returns advisory AGENTS.md additions in starter-file order.
func AgentSuggestions() []AgentSuggestion {
	return []AgentSuggestion{
		{
			ID:    "task-workflow",
			Title: "Task Workflow",
			Body: "Before creating, choosing, updating, or working on tasks, read\n" +
				"`.agents/TASKS.md`. Use `.agents/.tasks/index.md` as the generated task queue,\n" +
				"then open the specific task file before acting.",
		},
		{
			ID:    "research-workflow",
			Title: "Research Workflow",
			Body: "Before creating, updating, organizing, or using research, read\n" +
				"`.agents/RESEARCH.md`. Use `.agents/.research/index.md` as the research map.",
		},
		{
			ID:    "exec-plans",
			Title: "ExecPlans",
			Body: "For large work, read `.agents/PLANS.md`. Tasks marked `L` or `XL` require an\n" +
				"ExecPlan before implementation.",
		},
		{
			ID:    "docs-workflow",
			Title: "Documentation Workflow",
			Body: "Before auditing or updating documentation, read `.agents/DOCS.md`. Prefer the\n" +
				"target repository's existing documentation conventions over adding new\n" +
				"structures.",
		},
		{
			ID:    "generated-indexes",
			Title: "Generated Indexes",
			Body: "Do not edit generated indexes by hand. Update source task, research, or\n" +
				"ExecPlan files and run `ahm index`.",
		},
		{
			ID:    "task-state-transitions",
			Title: "Task State Transitions",
			Body: "Use `ahm task complete <id>` and `ahm task cancel <id>` for task state\n" +
				"transitions that move files between task buckets.",
		},
		{
			ID:    "no-implicit-commits",
			Title: "No Implicit Commits",
			Body:  "Do not commit or push unless explicitly asked.",
		},
	}
}

// RenderAgentsMarkdown renders the starter AGENTS.md from advisory suggestions.
func RenderAgentsMarkdown() string {
	blocks := []string{"# Agent Instructions"}
	for _, suggestion := range AgentSuggestions() {
		blocks = append(blocks, suggestion.Body)
	}
	return strings.Join(blocks, "\n\n") + "\n"
}
