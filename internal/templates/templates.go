package templates

import (
	"embed"
	"strings"
)

// Version is the embedded workflow template version.
const Version = "0.4.4"

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
		ID:    "ahm-apply-guidance",
		Title: "How To Apply",
		Body: "Preserve the target repository's project description, compatibility surfaces,\n" +
			"workflow routes, repository rules, code style, and handoff expectations. Add or\n" +
			"adapt only the parts needed to connect that repository's existing workflow to\n" +
			"`ahm`.\n" +
			"\n" +
			"If `AGENTS.md` already has an Operating Loop, update it so managed-work intake\n" +
			"happens before normal workflow routing. If `AGENTS.md` has workflow routing but\n" +
			"no Operating Loop, add a short `## Operating Loop` before `## Workflow Routing`.\n" +
			"If the file has neither an Operating Loop nor workflow routing, do not invent a\n" +
			"full project workflow; add only sections for Managed Work Intake With `ahm`\n" +
			"and `ahm-Owned Files`.\n" +
			"\n" +
			"Do not copy these suggestions blindly if they conflict with stronger\n" +
			"project-specific instructions. Adapt section names to the target file.",
	},
	{
		ID:    "operating-loop-integration",
		Title: "Operating Loop Integration",
		Body: "When an Operating Loop exists or should be added, it should include this\n" +
			"ordering:\n" +
			"\n" +
			"1. Do managed-work intake first:\n" +
			"   - If the request is about a task, ExecPlan, ADR, or research note, use `ahm`\n" +
			"     to understand that managed work item before choosing implementation docs.\n" +
			"   - If the request is directly about code, CLI behavior, tests, docs, build,\n" +
			"     release, or repo mechanics, skip `ahm` intake and classify the request\n" +
			"     directly.\n" +
			"2. Classify the concrete work by the repository's normal workflow routing.\n" +
			"3. Load only the routed docs required for that concrete work.\n" +
			"4. State the selected route and loaded docs before editing or in handoff,\n" +
			"   following the target repository's existing convention.\n" +
			"5. Preserve project compatibility surfaces unless the task explicitly changes\n" +
			"   them.\n" +
			"6. Keep edits surgical and verify according to risk.\n" +
			"7. Hand off with changes, exact checks, and remaining risk.",
	},
	{
		ID:    "ahm-workflow-routing",
		Title: "Managed Work Intake With `ahm`",
		Body: "`ahm` is for understanding and managing higher-order workflow records. It is\n" +
			"not the implementation route. Use it first when the user asks about a managed\n" +
			"work item, then return to the repository's normal workflow routing and choose\n" +
			"the route for the actual change.\n" +
			"\n" +
			"Use these entry points:\n" +
			"\n" +
			"- Tasks: run `ahm context task`, inspect the relevant task with `ahm task ...`,\n" +
			"  and open the task file before editing.\n" +
			"- ExecPlans: run `ahm context plan` when the request or task calls for an\n" +
			"  ExecPlan.\n" +
			"- ADRs: run `ahm context adr` when the request or task calls for an ADR, and\n" +
			"  use `ahm adr` commands for lifecycle changes.\n" +
			"- Research: run `ahm context research` and use `.agents/.research/index.md` as\n" +
			"  the map when asked to create, update, organize, or use research.\n" +
			"- General session briefing: run `ahm context` only when asked for broad project\n" +
			"  context or when no narrower managed-work context applies.\n" +
			"\n" +
			"After `ahm` intake, re-classify the discovered work under the repository's\n" +
			"normal Workflow Routing. For example, a task about CLI flags still uses the\n" +
			"CLI route; a task about atomic writes still uses the safety route; a task about\n" +
			"templates or workflow formats still uses the workflow-state route.",
	},
	{
		ID:    "ahm-owned-files",
		Title: "ahm-Owned Files",
		Body: "Never hand-edit generated task, research, ExecPlan, or ADR indexes. Update the\n" +
			"source records and run the appropriate `ahm` command.\n" +
			"\n" +
			"Use `ahm task` commands for task state moves and `ahm adr` commands for ADR\n" +
			"lifecycle changes.\n" +
			"\n" +
			"Treat scoped `ahm context task|plan|adr|research|docs` as the managed-work\n" +
			"reference for ahm-managed artifacts. Project `AGENTS.md` owns workflow routing\n" +
			"and implementation decisions. Unscoped `ahm context` provides a live repository\n" +
			"briefing. Do not recreate removed workflow guide files such as\n" +
			"`.agents/TASKS.md`, `.agents/PLANS.md`, `.agents/RESEARCH.md`,\n" +
			"`.agents/DOCS.md`, or `docs/adr/README.md`.",
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
