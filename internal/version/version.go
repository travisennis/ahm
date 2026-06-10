// Package version holds the ahm binary version string.
//
// Binary is set at build time via ldflags during goreleaser releases.
// Dev builds use the default value, which should match the template version
// at the time of development but is semantically independent: Binary advances
// with each release, while templates.Version advances only when embedded
// workflow templates change.
package version

// Binary is the ahm binary version, overridden by goreleaser ldflags.
var Binary = "0.2.0"
