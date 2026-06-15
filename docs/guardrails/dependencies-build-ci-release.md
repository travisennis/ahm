# Dependencies, Build, CI, And Release

## Scope

Read this guardrail for Go module changes, tool versions, build scripts, CI,
GoReleaser config, binary version injection, workflow template versioning, and
release behavior.

## Compatibility Surfaces

- Go version in `go.mod`.
- Module dependencies and transitive dependency risk.
- Local tool versions in `justfile`.
- CI command contract exposed by `just ci`.
- GoReleaser config and release artifacts.
- Binary version versus workflow template version.

## Required Checks

- Use `just tidy` or `just update-deps` intentionally; both may mutate module
  files.
- Run `just tidy-check` after dependency edits.
- Run `just release-check` for release pipeline changes.
- Run `just ci` before handoff for code, config, fixture, template, or
  dependency changes when available.

## Common Failure Modes

- Conflating `internal/version.Binary` with `internal/templates.Version`.
- Updating dependencies without checking generated `go.sum` or vulnerability
  results.
- Changing CI commands without updating `CONTRIBUTING.md`.
- Treating `just fix` as read-only.

## Related Docs

- `CONTRIBUTING.md`
- `docs/upgrades.md`
- `.github/workflows/`
- `.goreleaser.yaml`
- `justfile`
