---
status: accepted
date: 2026-06-06
---
# Task Work Agent Delegation

## Supersession Note

ADR 008 supersedes the commit-handoff portion of this decision. `ahm` still
does not run git operations directly, but it may perform an explicit delegated
commit handoff to a supported session-capable agent when the user passes
`--commit`.

## Context

`ahm` manages repository-local task workflow files, but implementation work is
still done by a human or by a separate coding-agent CLI. A local helper script
currently starts a `cake` session for a task, runs an independent review, resumes
the original session, and asks that session to commit. That script is useful,
but it is shell-specific, `cake`-specific, and mixes task selection with agent
session orchestration and git operations.

The project specification currently says v1 has no model or coding-agent calls
and no implicit git operations. Adding `ahm task work <id>` changes the
coding-agent boundary, so the command needs a durable contract that keeps `ahm`
responsible for deterministic task workflow behavior while leaving provider
credentials, model behavior, edits, and commits to the selected external CLI.

## Decision

Add `ahm task work <id>` as a delegation command. The command resolves the task
with existing task ID resolution, refuses completed and cancelled tasks, verifies
task dependencies are complete, selects a supported external agent, marks a
pending task `In Progress`, and then executes the selected external CLI from the
repository root with a generated task-work prompt.

The supported agents are `cake`, `claude`, `codex`, and `cursor`. `cake` is the
default. Repositories may set `"default_work_agent": "<agent>"` in
`.agents/ahm.json`. The `--agent <cake|claude|codex|cursor>` flag overrides
repository configuration for a single invocation. Unsupported agent names are
usage errors.

The command only performs one deterministic state transition before delegation:
`Pending` becomes `In Progress`. Tasks already `In Progress`, `Open`, or
`Blocked` are left in their current state if they pass the completed/cancelled
and dependency checks. Missing external executables are detected before any task
file is rewritten.

For session-capable agents (`cake`, `claude`, `codex`, and `cursor`), `ahm`
requests structured JSON output (`--output-format stream-json` for `cake`,
`--json` for `codex`, `--output-format stream-json` for `cursor-agent`,
`--verbose --output-format stream-json` for `claude`) and
parses the session identifier from the first output event:
`task_start.session_id` for `cake`, `thread.started.thread_id` for `codex`,
`system/init.session_id` for `cursor`, and `system/init.session_id` for
`claude`. The session ID is retained in
memory for the current review, revision, completion, and commit within the same
workflow invocation. Provider output is parsed only for the session identifier
and review-feedback fields needed by the orchestration hooks; results are still
produced by the delegated agent.

For Codex specifically, `ahm` invokes `codex exec` and `codex exec resume` with
`--dangerously-bypass-approvals-and-sandbox`. This is a deliberately broad
non-interactive automation posture: it avoids sandbox and approval deadlocks
while allowing Codex to edit files, run verification that writes outside the
repository cache, complete tasks, and perform the optional commit handoff.

With `--review`, `ahm` runs an independent review pass using the
repository-owned preflight review workflow through the selected delegated agent
and feeds actionable feedback back into the original work session. Review
orchestration is opt-in and requires a session-capable agent. `ahm` does not
pass credentials, choose models, complete tasks, commit changes, push branches,
or open pull requests. Those actions remain owned by the delegated agent and
the user's installed CLI configuration.

## Rationale

- Reusing `.agents/ahm.json` for `default_work_agent` keeps repository workflow
  settings in one existing file instead of adding a second config file for one
  setting.
- Marking `Pending` tasks `In Progress` before a successful handoff records that
  the task has been claimed while avoiding status drift when the selected agent
  executable is missing.
- Review orchestration is an optional step requested via `--review` and is
  always performed by the delegated agent, preserving the `ahm` rule that it
  must not perform implicit git operations.
- Making commits out of scope preserves the existing `ahm` rule that it must
  not perform implicit git operations.
- Command-line precedence over repository config follows common CLI practice and
  lets one-off invocations use a different agent without mutating repo policy.

## Consequences

### Positive

- Users can ask `ahm` to hand a resolved task to a coding agent without copying
  task IDs or recreating task prompts manually.
- The repo-local default agent is explicit and testable.
- `ahm` keeps ownership of task validation and state transition behavior while
  confining provider-specific output parsing to the small capability boundary
  needed for session resume and review feedback.

### Negative

- `task work` intentionally depends on external executables and can fail because
  the selected CLI is not installed or authenticated.
- The generated prompt and argument shapes must be maintained as external CLIs
  evolve.
- Codex task work runs without Codex sandboxing or approval prompts, so it can
  perform broad local actions under the user's account. Users should only select
  Codex for task work in repositories and working trees where that trust tradeoff
  is acceptable.
- A delegated agent can still edit files or run git commands according to its
  own configuration; `ahm` only controls the handoff boundary.

## Alternatives Considered

- **Keep the shell script only**: Rejected because it cannot use `ahm` task
  resolution, dependency checks, or repository config.
- **Add `.agents/config.json`**: Rejected for the MVP because one new setting
  does not justify a second workflow configuration file.
- **Do not update task state**: Rejected because a successful handoff should be
  visible in the task queue.
- **Run the full cake review and commit workflow**: Initially rejected because
  review orchestration conflicted with the no-implicit-git boundary. Task 055
  added opt-in review orchestration as a separate `--review` flag, keeping
  review as a delegated agent action rather than an `ahm` action. Commits
  remain excluded.

## References

- Task 050: Add task work agent handoff command
- Task 055: Add optional task work review orchestration
- Task 056: Capture and reuse task work agent sessions
- Task 084: Upgrade cursor agent to full task work orchestration
- Task 082: Add Claude Code agent support to ahm task work
- `.agents/exec-plans/completed/050-task-work-agent-handoff.md`
- `scripts/task-workflow.sh`
- `docs/references/workflow-spec.md`
- `internal/ahm/task_work.go`, `internal/ahm/task_agents.go`, `internal/ahm/task_session.go`, and `internal/ahm/task_parsers.go`
- `internal/ahm/install.go`
