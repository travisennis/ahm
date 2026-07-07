# Agent Instructions

## Project

`ahm` is a Go CLI that installs and manages repo-local `.agents` workflow
files for tasks, research notes, ExecPlans, ADRs, generated indexes, and
delegated coding-agent work.

Compatibility surfaces: CLI commands, flags, exit codes, output formats,
`.agents/ahm.json`, workflow file formats, generated indexes, embedded
templates, atomic writes, root detection, validation codes, external agent
orchestration, release/version semantics, and the guarantee that `ahm` does
not implicitly patch source code or run git operations.

## Operating Loop

0. **Before any work**: run `ahm prime` to sync workflow records, regenerate
   indexes, and get the session briefing. This is the canonical session-start
   command for coding agents.

1. Do work intake first:
   - If the request is about a task, ExecPlan, ADR, or research note, use `ahm`
     to understand that managed work item before choosing implementation docs.
   - If the request is directly about code, CLI behavior, tests, docs, or repo
     mechanics, skip `ahm` intake and classify the request directly.
2. Classify the concrete work by the Workflow Routing section below.
3. Load only the routed docs required for that concrete work.
4. State the selected route and loaded docs before editing.
5. Preserve compatibility surfaces unless the task explicitly changes them.
6. Keep edits surgical and verify according to risk.
7. Handoff with changes, exact checks, and remaining risk.

When this file conflicts with a specialized workflow doc for that workflow,
the specialized doc wins.

## Managed Work Intake With `ahm`

`ahm` is for understanding and managing higher-order workflow records. It is
not the implementation route. Use it first when the user asks about a managed
work item, then return to Workflow Routing and choose the route for the actual
change.

Use these entry points:

- Tasks: run `ahm context task`, inspect the relevant task with `ahm task show <id>`
  (which prints the task file contents), and open the task file directly only when
  `ahm` is unavailable or when manually editing the task record.
- ExecPlans: run `ahm context plan` when the request or task calls for an
  ExecPlan.
- ADRs: run `ahm context adr` when the request or task calls for an ADR.
- Research: run `ahm context research` and use `.agents/.research/index.md` as
  the map when asked to create, update, organize, or use research.
- General session briefing: run `ahm context` only when asked for broad project
  context or when no narrower managed-work context applies.

After `ahm` intake, re-classify the discovered work under Workflow Routing.
For example, a task about CLI flags still uses the CLI routing docs; a task
about atomic writes still uses the Safety routing docs; a task about templates
or workflow formats still uses the Workflow State routing docs.

## Workflow Routing

### CLI, User Output, And Exit Behavior

Use this workflow for command wiring, flags, help text, exit codes, output, and
dry-run behavior. Consult
[`docs/guardrails/cli-and-user-output.md`](docs/guardrails/cli-and-user-output.md),
[`docs/cli.md`](docs/cli.md), the relevant
[`docs/references/cli/`](docs/references/cli/) page, and
[`ARCHITECTURE.md`](ARCHITECTURE.md). Keep documented behavior stable unless the task is
explicitly a breaking CLI change.

### Workflow State, File Formats, And Upgrades

Use this workflow for `.agents/ahm.json`, workflow formats, generated indexes,
install/upgrade/context/status/doctor behavior, and embedded templates. Consult
[`docs/guardrails/workflow-state-and-file-formats.md`](docs/guardrails/workflow-state-and-file-formats.md),
[`docs/references/workflow-spec.md`](docs/references/workflow-spec.md),
[`docs/guides/workflow-upgrades.md`](docs/guides/workflow-upgrades.md), and
[`ARCHITECTURE.md`](ARCHITECTURE.md). Do not edit generated indexes by hand.
For `ahm context`, the default command is a session briefing; scoped commands
such as `ahm context task`, `ahm context adr`, `ahm context research`,
`ahm context plan`, and `ahm context docs` should expose the full scoped
instruction content, not the same briefing with a different label. Do not
remove or stop installing agent skills unless that is explicitly in scope.

### External Agent Orchestration

Use this workflow for `ahm task work`, agent definitions, arg builders,
parsers, session capture, handoff, and golden transcripts. Consult
[`docs/guardrails/external-agent-orchestration.md`](docs/guardrails/external-agent-orchestration.md)
and [`docs/guides/testing.md`](docs/guides/testing.md). Parser fixtures are not
enough when a real agent CLI contract changes.

### Safety, Permissions, And Atomic Writes

Use this workflow for filesystem writes, path handling, root detection, command
execution, and safety boundaries. Consult
[`docs/guardrails/safety-and-permissions.md`](docs/guardrails/safety-and-permissions.md),
[`docs/references/workflow-spec.md`](docs/references/workflow-spec.md), and
[ADR 001](docs/adr/001-atomic-writes-and-concurrency.md).
Keep writes explicit, dry-run aware, and crash-safe.

### Dependencies, Build, CI, And Release

Use this workflow for dependencies, build scripts, CI, GoReleaser, version
injection, and release behavior. Consult
[`docs/guardrails/dependencies-build-ci-release.md`](docs/guardrails/dependencies-build-ci-release.md),
[`CONTRIBUTING.md`](CONTRIBUTING.md),
[`docs/guides/workflow-upgrades.md`](docs/guides/workflow-upgrades.md), and
[`.github/workflows/`](.github/workflows/). Preserve binary/template version
separation.

### Architecture And Implementation Quality

Use this workflow for refactors, module boundaries, shared helpers, validation,
parsers, and performance-sensitive code. Consult
[`docs/guardrails/implementation-quality.md`](docs/guardrails/implementation-quality.md),
[`ARCHITECTURE.md`](ARCHITECTURE.md), and relevant
[ADRs](docs/adr/). Prefer small concrete functions and deterministic output.

### Build, Test, And Verification Commands

When deciding what build, test, lint, verification, or commit-prep commands to
run, consult [`CONTRIBUTING.md`](CONTRIBUTING.md). It is the canonical source
for the command catalog, verification expectations, and project-specific
command pitfalls.

## Repository Rules

- Do not commit or push unless explicitly asked.
- Assume uncommitted changes may belong to the user.
- Use Conventional Commit standard when writing commit messages.
- Do not revert, overwrite, or clean files you did not intentionally change.
- Inspect `git status --short` before broad edits.
- Never hand-edit ahm-generated indexes; update source records and run the
  appropriate `ahm` command.
- `AGENTS.md` is project-owned after creation; `ahm init`, `ahm upgrade`, and
  `--force` must not overwrite an existing project `AGENTS.md`.

## Handoff

End with the selected workflow route, routed docs loaded, what changed, exact
checks run, remaining risks or skipped checks, and actionable next steps. For
commits, include the commit hash, whether the worktree is clean, and any
leftover modified, deleted, or untracked files.
