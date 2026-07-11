package templates

import "embed"

// Version is the embedded workflow template version.
const Version = "0.5.0"

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

var managedFiles = []File{}

// Files returns the managed workflow files embedded in the CLI.
func Files() []File {
	return managedFiles
}
