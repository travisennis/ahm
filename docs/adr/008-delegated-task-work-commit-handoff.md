---
status: accepted
date: 2026-06-12
---
# Delegated Task Work Commit Handoff

## Context

ADR 006 added `ahm task work <id>` as an explicit delegation command and kept
commits out of scope. That was correct for the first task-work milestone because
the project needed a clear boundary: `ahm` validates task workflow state and
invokes an external coding-agent CLI, while the external agent owns source
edits, provider behavior, and git actions.

The local `scripts/task-workflow.sh` still captures a useful full workflow. It
starts a `cake` work session, runs an independent preflight review, resumes the
original work session to fix review feedback, then resumes that same session to
commit the completed work. The script's commit step explicitly tells the agent
to make sure the task is marked completed before committing and to include both
task files and project source files in one commit.

Task 057 brings that final commit handoff into `ahm` without changing the core
git boundary. The question is whether `ahm` may ask a delegated agent to commit
after explicit user consent, while still never running git commands itself.

## Decision

Add an explicit opt-in delegated commit handoff to `ahm task work` as
`--commit`. When passed with a session-capable supported agent, `ahm` resumes
the original work session after the initial work session and after review
feedback has been addressed if `--review` is also set. The resumed prompt asks
the delegated agent to commit the completed work for the task.

`ahm` must not run `git commit`, `git push`, branch operations, or pull-request
creation itself. It only sends a prompt to the delegated agent. The delegated
agent and the target repository's instructions, hooks, and user review own the
actual git behavior.

The commit handoff prompt must follow the behavior of `scripts/task-workflow.sh`:
it asks the agent to commit the completed work for the task, make sure the task
is marked completed before committing, and include both task files and project
source files in a single commit. The prompt must not prescribe Conventional
Commits or any other commit-message convention. Commit-message policy is owned
by the target project.

`--commit` does not require the user to also pass `--complete`. The base task
work prompt already instructs the agent to work the task through completion. The
commit handoff reinforces that the task must be marked completed before the
commit is made.

Pushes and pull-request creation remain out of scope for this feature.

## Rationale

- The existing shell workflow is the clearest source of user intent for the
  full task-work sequence, and `--commit` gives `ahm` the same explicit final
  handoff without making `ahm` a git automation tool.
- Keeping the commit action delegated preserves the no-implicit-git boundary
  from ADR 006. The user must request `--commit`, and the external agent still
  performs any git command under its own configuration.
- Not tying `--commit` to `--complete` avoids creating a confusing second
  completion model. Completion remains part of the task-work instructions and
  the commit prompt's precondition.
- Leaving commit-message convention to the target project avoids encoding this
  repository's Conventional Commit policy into a reusable workflow manager.

## Consequences

### Positive

- Users can run the full work, review, fix, and commit flow through `ahm`
  instead of keeping the broader behavior in a separate shell script.
- The git-operation boundary remains explicit and auditable.
- The commit handoff composes with existing in-memory session capture and
  review orchestration.

### Negative

- A delegated agent can still make a bad commit if its prompt interpretation,
  installed configuration, or project instructions are wrong. `ahm` cannot
  inspect or guarantee the resulting commit.
- The workflow still depends on provider-specific session capture and resume
  behavior.
- Users who want push or pull-request automation need a separate future
  decision.

## Alternatives Considered

- **Have `ahm` run `git commit` directly**: Rejected because it would violate
  the established no-implicit-git boundary and force `ahm` to own staging,
  commit-message policy, hooks, and failure recovery.
- **Require `--complete` with `--commit`**: Rejected because the normal work
  prompt already tells the agent to complete the task, and the script's commit
  step handles completion as a precondition.
- **Prescribe Conventional Commit messages**: Rejected because commit-message
  conventions belong to the target project, not to `ahm`.
- **Keep commit handoff only in `scripts/task-workflow.sh`**: Rejected because
  `ahm` now owns task-work orchestration and can provide the same behavior with
  task validation, agent selection, dry-run preview, and documentation.

## References

- ADR 006: Task Work Agent Delegation
- Task 057: Add opt-in task work commit handoff
- Task 055: Add optional task work review orchestration
- Task 056: Capture and reuse task work agent sessions
- Task 058: Add opt-in task work completion handoff
- `scripts/task-workflow.sh`
- `internal/ahm/task_session.go`
- `docs/cli.md`

