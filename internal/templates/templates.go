package templates

import "embed"

const Version = "0.1.0"

//go:embed workflow/**
var FS embed.FS

type File struct {
	Source string
	Target string
}

func Files() []File {
	return []File{
		{Source: "workflow/AGENTS.md", Target: "AGENTS.md"},
		{Source: "workflow/TASKS.md", Target: ".agents/TASKS.md"},
		{Source: "workflow/PLANS.md", Target: ".agents/PLANS.md"},
		{Source: "workflow/RESEARCH.md", Target: ".agents/RESEARCH.md"},
		{Source: "workflow/tasks-README.md", Target: ".agents/.tasks/README.md"},
		{Source: "workflow/research-README.md", Target: ".agents/.research/README.md"},
		{Source: "workflow/research-index.md", Target: ".agents/.research/index.md"},
		{Source: "workflow/exec-plans-active-index.md", Target: ".agents/exec-plans/active/index.md"},
		{Source: "workflow/exec-plans-completed-index.md", Target: ".agents/exec-plans/completed/index.md"},
		{Source: "workflow/deslop-SKILL.md", Target: ".agents/skills/deslop/SKILL.md"},
		{Source: "workflow/adr-README.md", Target: "docs/adr/README.md"},
	}
}
