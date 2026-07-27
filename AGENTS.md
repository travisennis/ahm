# Agent Instructions

## Project

`ahm` is a Go CLI that manages repo-local agent workflow state. Tasks, research
notes, ExecPlans, config, and indexes live under `.ahm/`; project guidance
lives under `.agents/`. Records are branch-scoped and use normal Git behavior;
`ahm` performs no ref or network operations.

Compatibility surfaces include CLI behavior, workflow metadata and formats,
indexes, templates, atomic writes, root detection, validation, orchestration,
and releases; [`ARCHITECTURE.md`](ARCHITECTURE.md) enumerates them. `ahm` does
not patch source, stage files, move `HEAD`, mutate branches, or create project
commits.

## Operating loop

1. Run `ahm prime` before any work; re-run it after context compaction.
2. If the request names a task, ExecPlan, ADR, or research record, inspect it
   through `ahm` before choosing implementation work.
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

Large or cross-cutting work requires an ExecPlan as directed by `ahm context plan`.

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

For `.ahm/config.json`, workflow formats, indexes, install, upgrade, context,
status, doctor, or templates, load:

- [Workflow state and file formats](docs/guardrails/workflow-state-and-file-formats.md),
  for the rules governing on-disk workflow records.
- [`docs/references/workflow-spec.md`](docs/references/workflow-spec.md), for the
  canonical format definitions.
- [`docs/guides/workflow-upgrades.md`](docs/guides/workflow-upgrades.md), for the
  migration path a format change owes existing repositories.
- [`ARCHITECTURE.md`](ARCHITECTURE.md), for where state is owned.

### External Agent Orchestration

For `ahm task work`, agent definitions, parsers, sessions, handoff, or golden
transcripts, load:

- [External agent orchestration](docs/guardrails/external-agent-orchestration.md),
  for the argument-building, parsing, and handoff contract.
- [`docs/guides/testing.md`](docs/guides/testing.md), for golden-transcript
  conventions.

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
context guidance, load:

- [Documentation](docs/guardrails/documentation.md), for which surfaces require
  which doc updates and where each one lives.

### Agent Instructions And Skills

For changes to this file, `.agents/`, or any other prose whose purpose is to
change how an agent behaves, load:

- [Agent-facing instructions](docs/guardrails/agent-instructions.md), for the
  evidence a behavior-shaping edit requires.

### Build, Test, And Verification Commands

Use [`CONTRIBUTING.md`](CONTRIBUTING.md) as the canonical command catalog and
verification policy.

### Managed Work Intake With `ahm`

Run `ahm prime` before intake and after compaction, then use its scoped command,
such as `ahm context task` followed by `ahm task show <id>`. Reclassify
implementation under the routes above. Never hand-edit indexes; use source
records plus the appropriate `ahm task`, `ahm adr`, or `ahm index` command.

## Repository Rules

- Do not commit or push unless explicitly asked.
- Assume uncommitted changes belong to the user; do not revert or clean files
  you did not intentionally change.
- Inspect `git status --short` before broad edits.
- Use Conventional Commits when writing commit messages.
- `AGENTS.md` is project-owned; `ahm init`, `ahm upgrade`, and `--force` must
  not overwrite it.

## Handoff

End with the selected route, routed docs loaded, changes, exact checks, risks
or skipped checks, and next steps. For commits, include the hash, worktree
status, and leftover modified, deleted, or untracked files.
