package templates

import (
	"embed"
	"strings"
)

// Version is the embedded workflow template version.
const Version = "0.2.0"

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
		ID:    "task-workflow",
		Title: "Task Workflow",
		Body: "For the first task in a session, read `.agents/TASKS.md`, then use\n" +
			"`.agents/.tasks/index.md` as the generated task queue and open the specific\n" +
			"task file. For later tasks in the same session, reread only the task index\n" +
			"and specific task file unless `.agents/TASKS.md` changed or the task changes\n" +
			"task workflow semantics.",
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
			"ExecPlan files and run `ahm index`. Do not run `ahm index` after\n" +
			"`ahm task create`, `ahm task start <id>`, `ahm task complete <id>`,\n" +
			"`ahm task cancel <id>`, or `ahm task reopen <id>` unless you edit metadata\n" +
			"by hand afterward; those commands already regenerate indexes.",
	},
	{
		ID:    "ahm-owned-files",
		Title: "Ahm-Owned Files",
		Body: "`ahm` owns the workflow files it installs, maintains, generates, and upgrades.\n" +
			"Do not hand-edit ahm-owned generated indexes or managed templates in a consumer\n" +
			"project to change ahm behavior or guidance. Update the source task, research\n" +
			"note, ExecPlan, or other project-owned record, then run the appropriate `ahm`\n" +
			"command (`ahm index`, `ahm task complete <id>`, `ahm upgrade`, etc.). If the\n" +
			"installed guidance itself needs to change, update the canonical templates in\n" +
			"the `ahm` repository and let projects receive the change through `ahm upgrade`.\n" +
			"\n" +
			"Exceptions: `AGENTS.md` is project-owned after creation; task files, research\n" +
			"notes, ExecPlans, and ADRs are workflow source records that may be updated\n" +
			"through their documented workflows; generated indexes must be regenerated, not\n" +
			"hand-edited; managed template files under `.agents/` and `docs/adr/README.md`\n" +
			"should not be customized locally as a way to change ahm-provided process\n" +
			"guidance.",
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
	{
		ID:    "implementation-docs",
		Title: "Implementation Documentation",
		Body: "When moving implementation between files or packages, update repository code\n" +
			"maps and implementation-location references even if user-facing behavior is\n" +
			"unchanged.",
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
		blocks = append(blocks, suggestion.Body)
	}
	return strings.Join(blocks, "\n\n") + "\n"
}
