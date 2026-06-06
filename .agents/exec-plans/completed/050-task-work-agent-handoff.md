# Add Task Work Agent Handoff Command

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document follows `.agents/PLANS.md`. It is self-contained so a contributor can continue the implementation from this file alone.

## Purpose / Big Picture

After this change, a user can run `ahm task work <id>` and let `ahm` resolve and validate a task before handing a clear task-work prompt to an installed coding-agent CLI. The command makes task workflow state visible by marking pending tasks in progress before a successful handoff, while keeping implementation edits, credentials, model selection, review orchestration, and git commits outside `ahm`.

The visible behavior is: `ahm task work 050 --agent codex` resolves task `050`, refuses invalid task states or incomplete dependencies, checks that the selected executable exists, updates a pending task to `In Progress`, and runs the external CLI from the repository root with a prompt that tells the agent to read the workflow docs and task file.

## Progress

- [x] (2026-06-06 12:39Z) Read `.agents/TASKS.md`, the generated task index, task `050`, `.agents/PLANS.md`, existing ADR conventions, `scripts/task-workflow.sh`, and the relevant CLI, metadata, task, spec, and docs files.
- [x] (2026-06-06 12:39Z) Wrote ADR 006 to decide the delegation boundary, config storage, state transition rule, and git-operation limits.
- [x] (2026-06-06 12:39Z) Created this active ExecPlan and linked task `050` to it.
- [x] (2026-06-06 12:52Z) Implemented repository metadata support for `default_work_agent`.
- [x] (2026-06-06 12:52Z) Added `ahm task work <id>` with `--agent`, task validation, executable lookup, pending-to-in-progress transition, prompt construction, and external process execution.
- [x] (2026-06-06 12:52Z) Added focused tests for agent selection precedence, invalid states, missing executable errors, state transition behavior, and per-agent argv/prompt construction.
- [x] (2026-06-06 12:52Z) Updated `docs/spec.md` and `docs/cli.md` for the new command and config setting.
- [x] (2026-06-06 12:52Z) Ran focused tests, `just fmt`, and `just ci`; filled task acceptance notes and prepared task completion.

## Surprises & Discoveries

- Observation: `.agents/ahm.json` already stores repository-scoped workflow settings, specifically `strict_acceptance`, in addition to template metadata.
  Evidence: `internal/ahm/install.go` defines `metadata.StrictAcceptance`, and `docs/spec.md` documents repository-scoped workflow settings in `.agents/ahm.json`.
- Observation: Cursor's current CLI uses the executable `cursor-agent` and supports non-interactive print mode with `-p` or `--print`, plus `--output-format text`.
  Evidence: Official Cursor CLI docs describe `cursor-agent -p "..." --output-format text`.
- Observation: Full CI flagged `exec.Command` with a variable executable as gosec G204.
  Evidence: `just ci` failed on `internal/ahm/task_commands.go` until the single launch site received a targeted `//nolint:gosec` rationale tied to the supported-agent allowlist.
- Observation: The deslop pass found small documentation and coverage gaps after the initial CI pass.
  Evidence: ADR 006 still referenced the active ExecPlan path, the global `--dry-run` support table omitted `task work`, and focused tests did not cover dry-run preview or invalid configured agents.

## Decision Log

- Decision: Store the default work agent as `default_work_agent` in `.agents/ahm.json`.
  Rationale: This repository already uses `.agents/ahm.json` for workflow settings, and adding a separate config file for a single setting would add migration and validation surface without enough benefit.
  Date/Author: 2026-06-06 / Codex
- Decision: `task work` marks pending tasks `In Progress` after task validation and executable lookup, before invoking the selected agent.
  Rationale: A successful handoff should claim the task in the queue, but a missing executable should not mutate task state.
  Date/Author: 2026-06-06 / Codex
- Decision: Review orchestration, automatic completion, and commits are out of scope for the MVP.
  Rationale: `ahm` must not perform implicit git operations, and external agent sessions own provider-specific behavior.
  Date/Author: 2026-06-06 / Codex

## Outcomes & Retrospective

Task `050` is implemented. `ahm task work <id>` now resolves and validates tasks, selects `cake`, `codex`, or `cursor` using flag-over-config-over-default precedence, checks executable availability, marks pending tasks `In Progress`, and invokes the selected CLI from the repository root with a generated prompt. The command leaves completed and cancelled tasks alone, refuses incomplete dependencies, and supports dry-run preview output.

Documentation now records the new command and the `.agents/ahm.json` `default_work_agent` setting in `docs/cli.md`, updates the coding-agent non-goal exception and config shape in `docs/spec.md`, and captures the delegation boundary in `docs/adr/006-task-work-agent-delegation.md`.

Validation passed with:

    go test ./internal/ahm -run 'TestTaskWork'
    go test ./internal/ahm
    just fmt
    just ci

The main tradeoff is that the executable launch needs a targeted gosec suppression because the command intentionally delegates to an external executable selected from a small allowlist.

The post-implementation deslop pass corrected the stale ADR reference to the completed ExecPlan path, updated the global CLI dry-run support table to include `task work`, and added tests for dry-run preview and invalid configured agents.

## Context and Orientation

`ahm` is a Go CLI. Cobra command wiring lives in `internal/ahm/cli.go` and task subcommands live in `internal/ahm/task_commands.go`. Task files are parsed and rendered by `internal/ahm/tasks.go`; task dependency helpers are in `internal/ahm/task_deps.go`; generated task and ExecPlan indexes are written by `internal/ahm/indexes.go`.

Repository metadata is represented by the `metadata` struct in `internal/ahm/install.go` and read from `.agents/ahm.json` with `readMetadata`. This file already contains template version, managed file hashes, and the `strict_acceptance` workflow setting.

The legacy helper `scripts/task-workflow.sh` shows the user goal but is broader than the MVP: it runs `cake`, an independent deslop review, resumes the original session, and asks the agent to commit. This plan intentionally implements only the initial deterministic handoff command.

An external coding-agent CLI means a separately installed executable such as `cake`, `codex`, or `cursor-agent`. `ahm` should detect whether the executable is present with `exec.LookPath`, then run it with a generated prompt. `ahm` should not know or handle credentials for any of those tools.

## Plan of Work

First, extend the metadata shape in `internal/ahm/install.go` with `DefaultWorkAgent string` using JSON key `default_work_agent` and omit it when empty. Add a small helper that reads metadata and returns the configured default work agent if present.

Next, add a `task work <id>` subcommand in `internal/ahm/task_commands.go`. It should accept `--agent`, normalize supported names case-insensitively, and select the agent in this order: explicit flag, `.agents/ahm.json` `default_work_agent`, then `cake`. Unsupported names should be `usageError`s so the process exits with code 2.

Add task validation for `task work`. Resolve the task using existing resolution behavior. Refuse `Completed` and `Cancelled` tasks. Build a map of completed tasks and require every dependency in `task.DependsOn` to be completed before handoff. Missing executable errors should name both the selected agent and the executable that must be installed.

Add prompt and argv construction helpers. The generated prompt must include the resolved task ID and task file path, and must instruct the delegated agent to read `AGENTS.md`, `.agents/TASKS.md`, `.agents/.tasks/index.md`, and the task file before implementation. It should tell the agent not to commit unless the user explicitly asks. Use these invocation shapes for the MVP:

- `cake --output-format text <prompt>`
- `codex exec <prompt>`
- `cursor-agent -p --output-format text <prompt>`

Run the selected executable from `a.opts.root` with stdin, stdout, and stderr inherited from the `app` where possible. Add narrow package-level variables for `exec.LookPath` and command execution so tests can verify argv construction without requiring real external CLIs.

For state transition, if the task status is `Pending`, rewrite it as `In Progress`, update `updated`, keep it in `.agents/.tasks/active/<id>.md`, regenerate indexes, and then invoke the external command. If the task is already `In Progress`, `Open`, or `Blocked`, do not rewrite it. The command should not complete or cancel tasks.

Update `docs/spec.md` to revise the v1 non-goal from "No model or coding-agent calls" to an explicit delegated-agent exception for `task work`, and document that there are still no implicit git operations. Update `docs/cli.md` with `task work`, `--agent`, config precedence, supported agents, missing executable errors, and the `default_work_agent` setting.

## Concrete Steps

Work from `/Users/travisennis/Projects/ahm`.

Run focused tests during implementation:

    go test ./internal/ahm -run 'TestTaskWork|TestTaskLifecycle'

After Go edits, format:

    just fmt

Before handoff, run full CI:

    just ci

If `just ci` fails because external tools are unavailable or because the sandbox blocks a required dependency/network action, record the exact failure and the narrower checks that passed.

## Validation and Acceptance

The new focused tests should prove these behaviors:

- `ahm task work 001` defaults to `cake` when no flag or config is set.
- `.agents/ahm.json` `default_work_agent` changes the selected agent, and `--agent` overrides it.
- Unsupported agent names fail with usage exit code 2.
- Completed and cancelled tasks are refused before executable lookup or invocation.
- Pending tasks with incomplete dependencies are refused.
- A missing executable produces an actionable runtime error and leaves a pending task unchanged.
- Successful handoff of a pending task rewrites it to `In Progress`, regenerates indexes, and invokes the expected executable argv from the repository root.
- The prompt includes the resolved task ID and task path.

Run `just ci` and expect it to pass before marking task `050` complete.

## Idempotence and Recovery

Creating the ADR and ExecPlan is additive. Re-running `ahm index` after task metadata or plan changes is safe because indexes are generated. Re-running `task work` on an already `In Progress` task should invoke the selected agent without rewriting the task again.

If an external executable is missing, install or configure that CLI and rerun the command. The missing executable path does not mutate task files. If the delegated CLI exits non-zero, `ahm` should return a runtime error but keep the task marked `In Progress` because the handoff was attempted.

## Artifacts and Notes

The selected Cursor invocation is based on the official Cursor CLI docs, which document the `cursor-agent` executable and `-p` or `--print` non-interactive mode with `--output-format`.

## Interfaces and Dependencies

The implementation should add helpers with names close to these, adjusted to fit existing style:

- `selectTaskWorkAgent(flagValue string) (taskWorkAgent, error)`
- `buildTaskWorkPrompt(task Task) string`
- `taskWorkInvocation(agent taskWorkAgent, prompt string) (string, []string)`
- `runExternalCommand(root string, executable string, args []string) error`

No new Go module dependency is needed. Use the standard library `os/exec`.

## Revision Notes

2026-06-06: Created the initial plan after reading task `050`, repository workflow docs, existing ADRs, task command code, metadata code, and the legacy shell workflow. The plan records the decisions needed before implementation because the task is `Effort: L` and introduces external agent execution.

2026-06-06: Completed the implementation, documentation, tests, and validation. The plan was moved from active to completed after recording outcomes so workflow validation can confirm the task-to-ExecPlan lifecycle.

2026-06-06: Ran the deslop review pass requested after handoff. The review kept three concrete fixes: correct ADR path, document global dry-run support for `task work`, and add focused tests for dry-run plus invalid configured agents. No broader simplification was applied because the command helpers are locally justified by validation, prompt construction, process execution, and test isolation.
