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
1. Classify the request by the risk surface below before editing.
2. Load only the routed docs needed for that request.
3. Preserve compatibility surfaces unless the task explicitly changes them.
4. Keep edits surgical and verify according to risk.
5. Handoff with changes, exact checks, and remaining risk.

When this file conflicts with a specialized workflow doc for that workflow,
the specialized doc wins.

## Workflow Routing

### CLI, User Output, And Exit Behavior
Use this workflow for command wiring, flags, help text, exit codes, output, and
dry-run behavior. Consult `docs/guardrails/cli-and-user-output.md`,
`docs/cli.md`, the relevant `docs/references/cli/` page, and
`ARCHITECTURE.md`. Keep documented behavior stable unless the task is
explicitly a breaking CLI change.

### Workflow State, File Formats, And Upgrades
Use this workflow for `.agents/ahm.json`, workflow formats, generated indexes,
install/upgrade/context/status/doctor behavior, and embedded templates. Consult
`docs/guardrails/workflow-state-and-file-formats.md`,
`docs/references/workflow-spec.md`, `docs/guides/workflow-upgrades.md`, and
`ARCHITECTURE.md`. Do not edit generated indexes by hand.
For `ahm context`, the default command is a session briefing; scoped commands
such as `ahm context task`, `ahm context adr`, `ahm context research`,
`ahm context plan`, and `ahm context docs` should expose the full scoped
instruction content, not the same briefing with a different label. Do not
remove or stop installing agent skills unless that is explicitly in scope.

### External Agent Orchestration
Use this workflow for `ahm task work`, agent definitions, arg builders,
parsers, session capture, handoff, and golden transcripts. Consult
`docs/guardrails/external-agent-orchestration.md` and `docs/guides/testing.md`. Parser
fixtures are not enough when a real agent CLI contract changes.

### Safety, Permissions, And Atomic Writes
Use this workflow for filesystem writes, path handling, root detection, command
execution, and safety boundaries. Consult
`docs/guardrails/safety-and-permissions.md`,
`docs/references/workflow-spec.md`, and ADR 001.
Keep writes explicit, dry-run aware, and crash-safe.

### Dependencies, Build, CI, And Release
Use this workflow for dependencies, build scripts, CI, GoReleaser, version
injection, and release behavior. Consult
`docs/guardrails/dependencies-build-ci-release.md`, `CONTRIBUTING.md`,
`docs/guides/workflow-upgrades.md`, and `.github/workflows/`. Preserve
binary/template version separation.

### Architecture And Implementation Quality
Use this workflow for refactors, module boundaries, shared helpers, validation,
parsers, and performance-sensitive code. Consult
`docs/guardrails/implementation-quality.md`, `ARCHITECTURE.md`, and relevant
ADRs. Prefer small concrete functions and deterministic output.

### Build, Test, And Verification Commands
When deciding what build, test, lint, verification, or commit-prep commands to
run, consult `CONTRIBUTING.md`. It is the canonical source for the command
catalog, verification expectations, and project-specific command pitfalls.

### Workflow Overlays
These overlays do not replace the specific workflow routes above. Use them first
to identify or manage the work item, then re-classify the concrete task and load
the relevant routed workflow docs before editing.

When asked to create, choose, update, or work on a task, run `ahm context task`,
inspect the task with `ahm task ...`, open the task file, then return to
Workflow Routing and choose the specific route or routes required by the task
content. When a task, workflow doc, or user request calls for an ExecPlan, read
`ahm context plan`. When one calls for an ADR, run `ahm context adr`.
When asked to create, update, organize, or use research, run `ahm context research`,
then use `.agents/.research/index.md` as the map.

## Repository Rules
- Do not commit or push unless explicitly asked.
- Assume uncommitted changes may belong to the user.
- Do not revert, overwrite, or clean files you did not intentionally change.
- Inspect `git status --short` before broad edits.
- Never hand-edit ahm-generated indexes; update source records and run the
  appropriate `ahm` command.
- `AGENTS.md` is project-owned after creation; `ahm init`, `ahm upgrade`, and
  `--force` must not overwrite an existing project `AGENTS.md`.

## Handoff
End with what changed, exact checks run, remaining risks or skipped checks, and
actionable next steps. For commits, include the commit hash, whether the
worktree is clean, and any leftover modified, deleted, or untracked files.
