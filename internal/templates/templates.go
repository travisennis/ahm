package templates

import "embed"

// Version is the embedded workflow template version.
const Version = "0.6.1"

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
	{Source: "workflow/tasks-README.md", Target: ".ahm/tasks/README.md", CreateOnly: true},
	{Source: "workflow/research-README.md", Target: ".ahm/research/README.md", CreateOnly: true},
	{Source: "workflow/adr-README.md", Target: "docs/adr/README.md", CreateOnly: true},
}

// Files returns the managed workflow files embedded in the CLI.
func Files() []File {
	return managedFiles
}
