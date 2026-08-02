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

### Worktree Setup

Worktrees are the parallel-work mechanism, used only when multiple tasks are
worked at the same time (for example, two agents, or a human and an agent,
implementing different branches at once). For a single task, skip the
worktree and work directly on a `feat/<slug>` branch in the main checkout;
the worktree overhead buys nothing unless the work is genuinely parallel.

Create a worktree for a task with `git worktree add -b feat/<slug>
../ahm-<slug> master` from the repository root, then `cd ../ahm-<slug>` and
run `ahm prime` before any work (prime regenerates the branch's indexes and
prints the briefing).

Linked worktrees share the main checkout's hooks directory (verify with
`git rev-parse --git-path hooks` from the worktree), so one `prek install` at
the main checkout covers all worktrees and no per-worktree hook install is
needed.

Cleanup after the PR merges: `git worktree remove ../ahm-<slug>` and
`git branch -d feat/<slug>`.

Agents: the cake agent runtime cannot yet create sibling-directory worktrees
through its shell sandbox (writes are restricted to the project directory),
so agents do not create worktrees and instead work in the main checkout on a
`feat/<slug>` branch. Until cake is patched, the worktree flow above is for
humans working in parallel; ahm does not rely on agent-created worktrees.

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
just docs-md-lint   # lint markdown structure (npx markdownlint-cli2); not yet in ci
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
- ADR lifecycle and format changes must stay aligned with the embedded
  `ahm context adr` reference in `internal/templates/workflow/ADR.md`.

Before auditing or changing docs, read
[the documentation guardrail](docs/guardrails/documentation.md). `ahm` does not
own general project documentation and has no documentation context scope.

## Commit And PR Workflow

All work happens on a feature branch named `feat/<slug>` (for example,
`feat/263c-branch-workflow`); nothing is committed directly to `master`, and
commits reach `master` only through a pull request with CI green. For a
single task, create the branch in the main checkout; use a worktree only when
multiple tasks are worked in parallel (see Worktree Setup above).

The standard sequence:

1. Create the branch from an updated master: `git checkout -b feat/<slug>`.
2. Run `ahm prime` before any work and after any checkout; it regenerates the
   branch-scoped indexes and prints the briefing.
3. Implement, then commit on the branch. Do not commit or push unless
   explicitly asked: a task or instruction that names branch work authorizes
   commits on the branch, while pushing and opening a PR require an explicit
   instruction or a proof step that asks for them.
4. Rebase the branch onto master before opening or updating the PR
   (`git fetch origin && git rebase origin/master`).
5. Push, open a pull request, wait for CI to pass, then merge.
6. Return to `master`, pull, and run `ahm prime` to regenerate master's
   indexes.

A pre-commit guard hook (`scripts/hooks/require-feature-branch.sh`, installed
by `prek install`) refuses commits on `master` with no bypass, and GitHub
branch protection enforces the same rule for pushes. The release flow is the
one deliberate exception: `just prepare-release` runs on `master`, and the
changelog commit it produces moves to a `release/vX.Y.Z` branch that merges
via PR (see Release Workflow below and `docs/release.md`).

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

Releases are tag-driven GitHub Releases built by GoReleaser, prepared on
`master`. Because the guard hook blocks direct master commits and master is
branch-protected, the changelog commit moves to a short-lived
`release/vX.Y.Z` branch and merges via pull request; the tag is then created
on master and pushed directly (tag pushes are not branch-protected). To
prepare a release, install `svu` and `git-cliff`, then run:

```bash
just prepare-release
```

The script uses `svu` to calculate the next SemVer tag, updates
`CHANGELOG.md`, runs the release checks, and prints the exact commit, PR, and
tag commands. Follow [`docs/release.md`](docs/release.md) for the full
checklist: review the changelog diff, move it to a `release/vX.Y.Z` branch,
commit and open a PR, merge once CI is green, then create and push the tag
from master.
