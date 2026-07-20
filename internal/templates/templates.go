package templates

import "embed"

// Version is the embedded workflow template version.
const Version = "0.6.5"

// FS contains the embedded workflow template files.
//
//go:embed workflow/*
var FS embed.FS
