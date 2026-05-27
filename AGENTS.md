# AGENTS.md

## Project Overview

`ahm` is an agent harness manager CLI that:

- Is written in Go 1.22.
- Installs and manages repo-local `.agents` workflow files for tasks, research
  notes, ExecPlans, ADRs, and generated indexes.
- Uses Cobra for CLI parsing.
- Embeds canonical workflow templates from `internal/templates/workflow/`.
- Tracks managed workflow files with `.agents/ahm.json` metadata.
- Preserves existing project `AGENTS.md` files; `AGENTS.md` is create-only and
  is never overwritten by `ahm init`, `ahm upgrade`, or `--force`.

Core mechanism: `ahm` writes and upgrades workflow documents, regenerates task,
research, and ExecPlan indexes, and provides task-management commands without
patching source code or performing git operations.

--------------------------------------------------------------------------------

## Start Here

1. Read this file fully before making changes.
2. For task work, read `.agents/TASKS.md`, then `.agents/.tasks/index.md`, then
   the specific task file.
3. Prefer narrow checks first, then `go fmt` or `just fmt` after Go edits, then
   `just ci` before final handoff for code changes.
4. Do not commit or push unless explicitly asked.
5. Never edit generated task, research, or ExecPlan indexes by hand.

--------------------------------------------------------------------------------

## Required Workflow

- Before final handoff for any code, test, config, fixture, template, or
  dependency change, run `just ci`.
- If `just ci` cannot be run, state the exact reason and list the narrower
  checks that were run instead.
- For Go code changes, use this verification sequence:
  1. Run the narrowest useful check first, such as `go test ./internal/ahm`,
     `go test ./internal/ahm -run <TestName>`, or
     `go test ./... -run <TestName>`.
  2. For local iteration, prefer focused `go test` commands, `just quick`, and
     `just fmt`.
  3. Run `gofmt` through `just fmt` after Go edits.
  4. Run `just ci` before final handoff or commit.
- When changing embedded workflow templates, also verify the behavior that
  consumes them. At minimum, run `go test ./internal/templates ./internal/ahm`
  before `just ci`.
- When changing CLI behavior, update `docs/cli.md` in the same change unless
  the behavior is intentionally undocumented.
- Before final handoff for CLI behavior changes:
  1. Search `docs/cli.md` for the old behavior or affected command.
  2. Update `docs/cli.md` in the same diff.
  3. Mention the docs update in the final summary.
- When changing durable workflow semantics, update `docs/spec.md` or
  `docs/upgrades.md` as appropriate.
- Do not commit or push code unless explicitly asked to.

--------------------------------------------------------------------------------

## Build/Test/Run

```bash
# Build local binary
just build

# Install ahm from this checkout
just install

# Run tests
just test
go test ./...

# Run tests with race detector and coverage
just test-race

# Run a focused package or test
go test ./internal/ahm
go test ./internal/ahm -run <TestName>

# Formatting
just fmt
just fmt-check

# Module cleanup
just tidy
just tidy-check

# Vet, lint, and vulnerability checks
just vet
just lint
just vuln

# Full CI check
just ci

# Mutating cleanup pass
just fix
```

Install local verification tools with:

```bash
just install-tools
```

--------------------------------------------------------------------------------

## Targeted Test Examples

```bash
# Good targeted test commands
go test ./internal/ahm
go test ./internal/templates
go test ./internal/ahm -run TestTask
go test ./... -run TestInstall
```

--------------------------------------------------------------------------------

## Code Map

Task command implementation:

- Task commands and resolution: `internal/ahm/cli.go`
- Task ID parsing/order helpers: `splitTaskID`, `nextTaskID`, `resolveTask`
- CLI tests: `internal/ahm/cli_test.go`
- User-facing CLI docs: `docs/cli.md`

Workflow templates:

- Embedded workflow templates: `internal/templates/workflow/`
- Template embedding and metadata: `internal/templates/templates.go`
- Template tests: `internal/templates`

--------------------------------------------------------------------------------

## Task Queue Rules

- When asked to create, choose, update, or work on a task, first read
  `.agents/TASKS.md`, then use `.agents/.tasks/index.md` as the task queue and
  open the specific task file before acting.
- Use task labels to filter work by type, area, and risk when the user asks for
  focused work.
- Do not edit generated task indexes by hand. Update task files and run
  `ahm index`.
- When marking a task as completed, use `ahm task complete <id>`. It updates
  task front matter, moves the file from `.agents/.tasks/active/` to
  `.agents/.tasks/completed/`, and regenerates indexes.
- Before running `ahm task complete <id>`, fill in Acceptance Notes when
  practical. If you edit only the completed task body afterward, no index
  regeneration is needed. If you edit task front matter afterward, rerun
  `ahm index`.
- When marking a task as cancelled, use `ahm task cancel <id>`. It updates task
  front matter, moves the file from `.agents/.tasks/active/` to
  `.agents/.tasks/cancelled/`, and regenerates indexes.
- Do not leave completed or cancelled tasks in `.agents/.tasks/active/`.

--------------------------------------------------------------------------------

## Research Rules

- When asked to create, update, organize, or use research, first read
  `.agents/RESEARCH.md`, then use `.agents/.research/index.md` as the research
  map and open the relevant research file before acting.
- Do not edit generated research indexes by hand. Update research source files
  and run `ahm index`.

--------------------------------------------------------------------------------

## ExecPlans

When writing complex features or significant refactors, use an ExecPlan as
described in `.agents/PLANS.md`.

Use an ExecPlan for:

- Tasks marked `Effort: L` or `Effort: XL`.
- Multi-package refactors.
- Changes to workflow install or upgrade semantics.
- Changes to generated index semantics.
- Changes to task state transitions, dependency resolution, or validation.
- Changes to embedded template ownership rules.

Keep `.agents/exec-plans/active/index.md` current when creating, completing, or
moving plans. Do not edit generated ExecPlan indexes by hand.

--------------------------------------------------------------------------------

## Git Worktree Safety

- Assume uncommitted changes may belong to the user.
- Do not revert, overwrite, or clean files you did not intentionally change.
- Before broad edits, inspect `git status --short`.
- Before final handoff, report remaining uncommitted or untracked files when
  relevant.

--------------------------------------------------------------------------------

## Commit Handoff Requirements

After any commit:

- Run `git status --short` before the final response.
- Include the commit hash in the final response.
- State whether the worktree is clean.
- If the worktree is not clean, list the remaining modified, deleted, or
  untracked files.
- Distinguish files changed by the agent from unrelated or pre-existing
  worktree changes when that context is known.

--------------------------------------------------------------------------------

## Commit Conventions

This project uses Conventional Commits. Commit messages and pull request titles
must use this format:

```text
<type>[(scope)]: <description>
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`,
`ci`, `chore`, `revert`.

Recommended scopes:

| Scope       | Description                                      |
| ----------- | ------------------------------------------------ |
| `cli`       | Command-line interface and argument parsing      |
| `workflow`  | Managed workflow files and `.agents` behavior    |
| `tasks`     | Task commands, parsing, indexes, and state moves |
| `research`  | Research indexes and workflow docs               |
| `plans`     | ExecPlan indexes and workflow docs               |
| `templates` | Embedded templates and template metadata         |
| `docs`      | Human-facing docs under `docs/`                  |
| `release`   | Build, release, and versioning changes           |

--------------------------------------------------------------------------------

## Code Style Guidelines

- Keep CLI behavior explicit and documented in `docs/cli.md`.
- Prefer small, focused functions over broad command handlers that mix parsing,
  filesystem mutation, and output formatting.
- Use concrete structs at command and file-format boundaries.
- Validate file formats at the boundary and return explicit errors.
- Preserve dry-run behavior for write commands.
- Keep generated indexes deterministic by sorting output consistently.
- Avoid global state except for embedded templates and constants.
- Do not add implicit git operations. `ahm` must not commit, push, open PRs, or
  modify source code in target repositories.

--------------------------------------------------------------------------------

## Architecture Decision Records

When a task introduces or changes a durable architectural decision, write or
update an ADR under `docs/adr/` before implementation. Follow
`docs/adr/README.md` when it exists in the target workflow.

--------------------------------------------------------------------------------

## Common Pitfalls

- `AGENTS.md` is create-only. Do not treat it as a managed file that can be
  overwritten by `upgrade` or `--force`.
- Generated indexes are owned by `ahm`; update source files and run `ahm index`.
- Completed and cancelled tasks must be moved with `ahm task complete <id>` or
  `ahm task cancel <id>`.
- `just fix` is mutating; use it intentionally and report resulting changes.
- Template changes may require updating template tests and CLI install/upgrade
  expectations.

--------------------------------------------------------------------------------

## Additional Notes

- CLI entrypoint: `cmd/ahm/main.go`.
- Command implementation: `internal/ahm/cli.go`.
- Embedded workflow templates: `internal/templates/workflow/`.
- Template embedding and metadata: `internal/templates/templates.go`.
- CLI reference: `docs/cli.md`.
- Workflow semantics: `docs/spec.md`.
- Upgrade behavior: `docs/upgrades.md`.
