# Agent Instructions

## Project

`ahm` is a Go CLI that manages repo-local agent workflow state. Tasks, research
notes, ExecPlans, config, and indexes live under `.ahm/`; project guidance
lives under `.agents/`. Records are branch-scoped and use normal Git behavior; `ahm` performs no ref or network operations.

Compatibility surfaces include CLI behavior, workflow metadata and formats,
indexes, templates, atomic writes, root detection, validation, orchestration,
and releases. `ahm` does not patch source, stage files, move `HEAD`, mutate
branches, or create project commits.

## Operating Loop

0. Run `ahm prime` before any work to prepare the worktree and get the briefing.
1. Use `ahm` intake first for tasks, ExecPlans, ADRs, or research; classify
   direct code, CLI, docs, or repository work immediately.
2. For a Pending task, run `ahm task start <id>` to begin its lifecycle.
3. Select the route below, load only its docs, and state both before editing.
4. Preserve compatibility unless explicitly changed; edit surgically and
   verify according to risk.
5. After implementation edits, run codex review (`tb__codex_review`), fix all
   findings, and rerun until clean; reconsider approaches that do not converge.
6. Use the oracle (`tb__oracle`) for unclear design, debugging, or path choices.
7. For task-backed work, run `ahm task complete <id>` to close the task lifecycle.
8. Run preflight checks and handoff.

Specialized workflow docs override this file when they conflict.

## Workflow Routing

### CLI, User Output, And Exit Behavior

For command wiring, flags, help, exit codes, output, or dry-run behavior, load
[`docs/guardrails/cli-and-user-output.md`](docs/guardrails/cli-and-user-output.md), [`docs/cli.md`](docs/cli.md),
the relevant [`docs/references/cli/`](docs/references/cli/) page, and [`ARCHITECTURE.md`](ARCHITECTURE.md).

### Workflow State, File Formats, And Upgrades

For `.ahm/config.json`, workflow formats, indexes, install, upgrade, context,
status, doctor, or templates, load [`docs/guardrails/workflow-state-and-file-formats.md`](docs/guardrails/workflow-state-and-file-formats.md),
[`docs/references/workflow-spec.md`](docs/references/workflow-spec.md), [`docs/guides/workflow-upgrades.md`](docs/guides/workflow-upgrades.md),
and [`ARCHITECTURE.md`](ARCHITECTURE.md).

### External Agent Orchestration

For `ahm task work`, agent definitions, parsers, sessions, handoff, or golden transcripts, load
[`docs/guardrails/external-agent-orchestration.md`](docs/guardrails/external-agent-orchestration.md) and [`docs/guides/testing.md`](docs/guides/testing.md).

### Safety, Permissions, And Atomic Writes

For filesystem writes, paths, root detection, command execution, or safety, load
[`docs/guardrails/safety-and-permissions.md`](docs/guardrails/safety-and-permissions.md), [`docs/references/workflow-spec.md`](docs/references/workflow-spec.md),
and [ADR 001](docs/adr/001-atomic-writes-and-concurrency.md).

### Dependencies, Build, CI, And Release

For dependencies, builds, CI, GoReleaser, version injection, or releases, load
[`docs/guardrails/dependencies-build-ci-release.md`](docs/guardrails/dependencies-build-ci-release.md), [`CONTRIBUTING.md`](CONTRIBUTING.md),
[`docs/guides/workflow-upgrades.md`](docs/guides/workflow-upgrades.md), and [`.github/workflows/`](.github/workflows/).

### Architecture And Implementation Quality

For refactors, module boundaries, helpers, validation, parsers, or performance, load
[`docs/guardrails/implementation-quality.md`](docs/guardrails/implementation-quality.md), [`ARCHITECTURE.md`](ARCHITECTURE.md), and relevant [ADRs](docs/adr/).

### Build, Test, And Verification Commands

Use [`CONTRIBUTING.md`](CONTRIBUTING.md) as the canonical command catalog and verification policy.

### Managed Work Intake With `ahm`

Run `ahm prime` before intake and after compaction, then use its scoped command,
such as `ahm context task` followed by `ahm task show <id>`. Reclassify implementation under the routes above.
Never hand-edit indexes; use source records plus the appropriate `ahm task`, `ahm adr`, or `ahm index` command.

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
