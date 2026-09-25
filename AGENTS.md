# Agent Instructions

## Project

`ahm` is a Go CLI that manages task records in either the project or a
user-level home store, and ADRs under `docs/adr/`. Task records are
branch-scoped committed files only in `project` mode; in `home` mode they live
in the machine-level store, are shared across branches, and are shared across
clones that resolve to the same project key. Config and generated indexes live
under `.ahm/` in project mode and beside the records
in home mode; project guidance lives under `.agents/` and `docs/`. `ahm`
performs no ref or network operations.

Compatibility surfaces include CLI behavior, workflow metadata and formats,
storage mode and home-store resolution, indexes, atomic writes, root detection,
validation, and releases;
[`ARCHITECTURE.md`](ARCHITECTURE.md) enumerates them. `ahm` does
not patch source, stage files, move `HEAD`, mutate branches, or create project
commits.

## Operating loop

1. Run `ahm prime` before any work; re-run it after context compaction.
2. If the request names a task or ADR, inspect it through `ahm` before choosing
   implementation work. A design plan under `docs/exec-plans/` and any other
   project document are read directly.
3. Select the route below, load only its documents, and state both before
   editing.
4. Read the smallest relevant code and tests.
5. Preserve compatibility unless the task explicitly changes it.
6. If work is managed, start and complete it through `ahm`.
7. Make surgical edits and run risk-proportionate checks.
8. After implementation edits, run a review in a subagent and address findings
   until none remain, then perform preflight. If a third round reports findings
   of the same class, stop patching: report the finding class and the suspected
   design flaw, and escalate to a design decision.
9. Hand off per [Handoff](#handoff).

Large or cross-cutting work requires a design plan under `docs/exec-plans/`,
written per [the ExecPlan workflow](docs/workflow/exec-plans.md).

## Workflow Routing

### CLI, User Output, And Exit Behavior

For command wiring, flags, help, exit codes, output, or dry-run behavior, load:

- [CLI and user output](docs/guardrails/cli-and-user-output.md), for wiring,
  help-text, exit-code, and output-mode expectations.
- [`docs/cli.md`](docs/cli.md), for the user-facing command overview.
- The relevant page under [`docs/references/cli/`](docs/references/cli/), for the
  exact per-command contract; [`global-contract.md`](docs/references/cli/global-contract.md)
  owns behavior shared by every command.
- [`ARCHITECTURE.md`](ARCHITECTURE.md), for the CLI boundary.

### Workflow State, File Formats, And Upgrades

For `.ahm/config.json`, workflow formats, indexes, install, cross-version
migration, status, doctor, or record files, load:

- [Workflow state and file formats](docs/guardrails/workflow-state-and-file-formats.md),
  for the rules governing on-disk workflow records.
- [`docs/references/workflow-spec.md`](docs/references/workflow-spec.md), for the
  canonical format definitions.
- [`docs/guides/workflow-upgrades.md`](docs/guides/workflow-upgrades.md), for the
  migration path a format change owes existing repositories.
- [`ARCHITECTURE.md`](ARCHITECTURE.md), for where state is owned.

### Safety, Permissions, And Atomic Writes

For filesystem writes, paths, root detection, command execution, or safety,
load:

- [Safety and permissions](docs/guardrails/safety-and-permissions.md), for the
  write, path, and execution boundaries.
- [`docs/references/workflow-spec.md`](docs/references/workflow-spec.md), for the
  durability requirements a record format assumes.
- [ADR 001](docs/adr/001-atomic-writes-and-concurrency.md), for the atomic-write
  and concurrency decision.

### Dependencies, Build, CI, And Release

For dependencies, builds, CI, GoReleaser, version injection, or releases, load:

- [Dependencies, build, CI, and release](docs/guardrails/dependencies-build-ci-release.md),
  for dependency and release policy.
- [`CONTRIBUTING.md`](CONTRIBUTING.md), for the commands.
- [`docs/guides/workflow-upgrades.md`](docs/guides/workflow-upgrades.md), when a
  release changes a workflow format.
- [`.github/workflows/`](.github/workflows/), which is the authority for CI
  behavior.

### Architecture And Implementation Quality

For refactors, module boundaries, helpers, validation, parsers, or performance,
load:

- [Implementation quality](docs/guardrails/implementation-quality.md), for style
  and structural expectations.
- [`ARCHITECTURE.md`](ARCHITECTURE.md), for the module map and the invariants a
  refactor must preserve.
- The relevant [ADRs](docs/adr/), for decisions already made in the changed area.

### Documentation

For README, architecture, CLI docs, workflow specs, upgrade docs, ADR prose, or
project workflow guidance, load:

- [Documentation](docs/guardrails/documentation.md), for which surfaces require
  which doc updates and where each one lives.
- [Task workflow](docs/workflow/tasks.md), [ADR workflow](docs/workflow/adrs.md),
  and [ExecPlan workflow](docs/workflow/exec-plans.md), for the project-owned
  procedures `ahm` no longer prints.

### Agent Instructions And Skills

For changes to this file, `.agents/`, or any other prose whose purpose is to
change how an agent behaves, load:

- [Agent-facing instructions](docs/guardrails/agent-instructions.md), for the
  evidence a behavior-shaping edit requires.

### Build, Test, And Verification Commands

Use [`CONTRIBUTING.md`](CONTRIBUTING.md) as the canonical command catalog and
verification policy.

### Task And ADR Procedure

Task, ADR, and planning practice is project-owned prose: see the [task
workflow](docs/workflow/tasks.md), the [ADR workflow](docs/workflow/adrs.md),
and the [ExecPlan workflow](docs/workflow/exec-plans.md). Run `ahm prime`
before intake and after compaction; it reports record counts and validation
findings and routes nothing. Reclassify implementation under the routes above.
Never hand-edit indexes; use source records plus the appropriate `ahm task`,
`ahm adr`, or `ahm index` command.

## Repository Rules

- Work happens on `master`. Commit directly to it, including development
  work, planning records, and release prep. CI runs on every push, and the
  repository has one maintainer, so a branch and a pull request add ceremony
  without adding a gate. Reach for a `feat/<slug>` branch, or a worktree, only
  when you want isolation for an experiment or when several streams of work
  run in parallel.
- Do not commit or push unless explicitly asked. An instruction to fix, build,
  commit, or ship authorizes the commits and the push it needs; say so in the
  handoff. After finishing, hand off with the commit hashes, the branch, and
  the worktree status.
- Assume uncommitted changes belong to the user; do not revert or clean files
  you did not intentionally change.
- Inspect `git status --short` before broad edits.
- Use Conventional Commits when writing commit messages.
- `AGENTS.md` is project-owned; `ahm init` and `--force` must not overwrite
  it.

## Handoff

End with the selected route, routed docs loaded, changes, exact checks, risks
or skipped checks, and next steps. For commits, include the hash, worktree
status, and leftover modified, deleted, or untracked files.
