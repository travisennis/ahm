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

## Version constants

The repository maintains two independent version numbers:

| Constant | File | Appears in | Semantics |
| ---------- | ------ | ------------ | ---------- |
| `version.Binary` | `internal/version/version.go` | `ahm version` | Binary release version. Set by goreleaser ldflags at build time. Dev builds default to `"dev"`. |
| `templates.Version` | `internal/templates/templates.go` | `ahm status` → `template_version`; stamped into `.agents/ahm.json` on install/upgrade | Embedded workflow template schema version. Bumps only when the `//go:embed workflow/*` template pack changes (new files, content changes, new agent suggestions). |

These are semantically independent — a release can ship with newer binary code
and unchanged templates, or templated changes that don't warrant a binary
release tag. There is no automated alignment check between them, and none is
needed.

## Required Checks

- Use `just tidy` or `just update-deps` intentionally; both may mutate module
  files.
- Run `just tidy-check` after dependency edits.
- Run `just release-check` for release pipeline changes.
- Run `just ci` before handoff for code, config, fixture, template, or
  dependency changes when available.

## Common Failure Modes

- Conflating `internal/version.Binary` with `internal/templates.Version`. See [Version constants](#version-constants) below.
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
