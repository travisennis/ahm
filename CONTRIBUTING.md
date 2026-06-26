# Contributing

## Local Setup

Install Go 1.26.3 and the repository verification tools:

```bash
just install-tools
```

Install a development build from this checkout:

```bash
just install
```

This repository uses `prek` with a pre-commit-compatible config:

```bash
prek install
prek install --hook-type commit-msg
```

## Command Catalog

```bash
just build          # build bin/ahm
just install        # install ahm from this checkout
just test           # go test ./...
                    # or: go test github.com/travisennis/ahm/internal/...
just test-race      # go test -race -cover ./...
                    # or: go test -race -cover github.com/travisennis/ahm/internal/...
just vet            # go vet ./...
                    # or: go vet github.com/travisennis/ahm/internal/...
just fmt            # go fmt ./...
just fmt-check      # fail if gofmt would change files
just tidy           # go mod tidy
just tidy-check     # fail if go mod tidy would change files
just lint           # golangci-lint
just vuln           # govulncheck ./...
just release-check  # goreleaser check and snapshot build
just prepare-release  # calculate version, update changelog, and run release checks
just quick          # go test ./... plus go vet ./...
just ci             # full read-only CI suite
just fix            # mutating tidy plus fmt
```

Agent integration commands make real LLM calls and are not part of CI:

```bash
just smoke-agents
just capture-agent-fixtures
```

See `docs/guides/testing.md` before running either command.

## Project-Specific Guidance

**Repo root is not the Go package.** Do not use `go build .` or `go run .`.
Always build with `go build ./cmd/ahm` or the `just build` recipe.

**Go package paths.** When `go test ./...` is unavailable (restricted shells,
sandboxed agents), use the full module path from `go.mod`:
`go test github.com/travisennis/ahm/internal/...`

**Final verification.** Prefer `just ci` (or its alias `just verify`) for the
full read-only CI suite before handoff.

**Task inspection.** Use `ahm task show <id>` to inspect a single task. For
queue views, use `ahm task list --status <status>` with one or more of:
`Open`, `Pending`, `In Progress`, `Blocked`, `Tracking`, `Completed`,
`Cancelled`. Do not pass `--status All` — it is not a valid status; the
`--status` flag accepts only the status names listed above.

**Multiline commit messages.** When writing a commit message that spans
multiple lines, use `git commit -F - <<'EOF'` with a heredoc. Do not use
command substitution inside `git commit -m` — it behaves inconsistently
across shells and is difficult to read.

## Verification Expectations

Run the narrowest useful check first. For Go edits, start with a focused
`go test` package or test name, then run `just fmt` after edits, and run
`just ci` before handoff for code, test, config, fixture, template, or
dependency changes.

If `just ci` cannot be run, state the exact reason and list the narrower
checks that were run instead.

Template changes require the behavior that consumes them to be tested. At
minimum, run:

```bash
go test ./internal/templates ./internal/ahm
```

Changes to external agent argument builders, parsers, or orchestration require
the live smoke checklist in `docs/guides/testing.md`.

## Code Style

- Keep changes narrow and match the existing style.
- Prefer small, focused functions over broad command handlers.
- Use concrete structs at command and file-format boundaries.
- Validate file formats at the boundary and return explicit errors.
- Preserve dry-run behavior for write commands.
- Keep generated indexes deterministic by sorting output consistently.
- Avoid global state except embedded templates and constants.
- Do not add implicit git operations.

## Documentation

Update documentation when a change affects user-visible behavior, commands,
configuration, file formats, workflow semantics, architecture, release
behavior, setup, security, or compatibility.

- CLI behavior changes usually require `docs/cli.md` and the affected
  `docs/references/cli/` page.
- Durable workflow semantics usually require
  `docs/references/workflow-spec.md` or `docs/guides/workflow-upgrades.md`.
- Implementation moves require `ARCHITECTURE.md` updates when the module map or
  boundary descriptions change.
- ADR lifecycle and format changes must stay aligned with `docs/adr/README.md`.

Before auditing or changing docs, run `ahm context docs`.

## Commit And PR Workflow

Do not commit or push unless explicitly asked.

Commit messages and pull request titles must use Conventional Commits:

```text
<type>[(scope)]: <description>
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`,
`ci`, `chore`, `revert`.

Recommended scopes:

| Scope | Description |
| --- | --- |
| `cli` | Command-line interface and argument parsing |
| `workflow` | Managed workflow files and `.agents` behavior |
| `tasks` | Task commands, parsing, indexes, and state moves |
| `research` | Research indexes and workflow docs |
| `plans` | ExecPlan indexes and workflow docs |
| `templates` | Embedded templates and template metadata |
| `docs` | Human-facing docs under `docs/` |
| `release` | Build, release, and versioning changes |

After any commit, run `git status --short` and hand off with the commit hash,
worktree cleanliness, and any remaining modified, deleted, or untracked files.

## Release Workflow

Releases are tag-driven GitHub Releases built by GoReleaser. To prepare a
release, install `svu` and `git-cliff`, then run:

```bash
just prepare-release
```

The script uses `svu` to calculate the next SemVer tag. Review and commit the
generated `CHANGELOG.md` update, then create and push the tag. See
[`docs/release.md`](docs/release.md) for installer commands and the full
release checklist.
