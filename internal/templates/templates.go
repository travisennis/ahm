package templates

import "embed"

// Version is the embedded workflow template version.
const Version = "0.4.6"

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
