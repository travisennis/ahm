package templates

import "embed"

// FS contains the embedded workflow template files.
//
//go:embed workflow/*
var FS embed.FS
