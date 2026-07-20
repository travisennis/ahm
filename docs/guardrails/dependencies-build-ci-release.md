# Dependencies, Build, CI, And Release

## Scope

Read this guardrail for Go module changes, tool versions, build scripts, CI,
GoReleaser config, binary version injection, and release behavior.

## Compatibility Surfaces

- Go version in `go.mod`.
- Module dependencies and transitive dependency risk.
- Local tool versions in `justfile`.
- CI command contract exposed by `just ci`.
- GoReleaser config and release artifacts.
- Binary version injection.

## Version constants

The repository maintains one version number:

| Constant | File | Appears in | Semantics |
| ---------- | ------ | ------------ | ---------- |
| `version.Binary` | `internal/version/version.go` | `ahm version` | Binary release version. Set by goreleaser ldflags at build time. Dev builds default to `"dev"`. |

## Required Checks

- Use `just tidy` or `just update-deps` intentionally; both may mutate module
  files.
- Run `just tidy-check` after dependency edits.
- Run `just release-check` for release pipeline changes.
- Run `just ci` before handoff for code, config, fixture, template, or
  dependency changes when available.

## Common Failure Modes

- Conflating `internal/version.Binary` with the removed `internal/templates.Version`.
- Updating dependencies without checking generated `go.sum` or vulnerability
  results.
- Changing CI commands without updating `CONTRIBUTING.md`.
- Treating `just fix` as read-only.

## Related Docs

- `CONTRIBUTING.md`
- `docs/guides/workflow-upgrades.md`
- `.github/workflows/`
- `.goreleaser.yaml`
- `justfile`
