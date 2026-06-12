# Add Opt-In Task Work Commit Handoff

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document follows `.agents/PLANS.md`. It is self-contained so a contributor can continue the implementation from this file alone.

## Purpose / Big Picture

After this change, a user can run `ahm task work <id> --commit` to ask the same delegated coding-agent session that worked a task to commit the completed work. The behavior mirrors the final step of `scripts/task-workflow.sh`: after the work session, and after deslop review feedback is addressed when `--review` is also set, `ahm` resumes the original session and asks the agent to commit the completed work for the task. `ahm` does not run git commands itself.

The observable behavior is that dry-run output previews `commit: true`, and a real `cake` orchestration with `--commit` runs an additional `cake --resume <session> --output-format json <prompt>` invocation whose prompt says to commit the completed work, ensure the task is marked completed before committing, and include task files and project source files in one commit.

## Progress

- [x] (2026-06-12 00:35Z) Read task 057, `.agents/PLANS.md`, ADR README guidance, ADR 006, `scripts/task-workflow.sh`, current task-work implementation, CLI docs, README, spec, and relevant tests.
- [x] (2026-06-12 00:35Z) Created ADR 008 to permit explicit delegated commit handoff while preserving the rule that `ahm` does not run git operations directly.
- [x] (2026-06-12 00:35Z) Marked ADR 006 as superseded in part by ADR 008.
- [x] (2026-06-12 00:50Z) Implemented `--commit` flag, dry-run preview, session follow-up, prompt builder, and tests.
- [x] (2026-06-12 00:50Z) Updated README, CLI docs, and spec for the commit handoff boundary.
- [x] (2026-06-12 00:53Z) Ran focused tests, package tests, formatting, and full CI.
- [x] (2026-06-12 00:54Z) Filled task acceptance notes, updated this plan's outcomes, moved this plan to completed, and completed task 057.
- [x] (2026-06-12 01:05Z) Ran deslop review and tightened `--commit` so missing session capture fails instead of silently skipping the requested commit handoff.

## Surprises & Discoveries

- Observation: `taskWorkWithSession` already mentions commit handoff in its comment, but the implementation only runs review and completion follow-ups.
  Evidence: `internal/ahm/task_commands.go` says the captured session is available for later review, completion, and commit handoff steps, while the function parameters only include `review` and `complete`.
- Observation: The current dry-run preview records `complete: true` but omits `review: true`, even though docs say dry-run previews selected arguments and status.
  Evidence: `taskWork` only adds `preview["complete"] = true`.
- Observation: Before the deslop pass, `--commit` could silently skip the
  requested commit handoff when session capture failed.
  Evidence: `taskWorkWithSession` returned success after warning on missing or
  unparsable session IDs before checking follow-up flags.

## Decision Log

- Decision: Implement `--commit` as a session follow-up after review and completion follow-ups, with the commit step last.
  Rationale: This matches `scripts/task-workflow.sh`, where the commit happens after deslop feedback is addressed and after the task is completed or confirmed complete.
  Date/Author: 2026-06-12 / Codex
- Decision: Do not require `--complete` when `--commit` is passed.
  Rationale: The base work prompt already instructs the delegated agent to complete the task; the commit prompt should only reinforce that the task must be marked completed before committing.
  Date/Author: 2026-06-12 / Codex
- Decision: Do not prescribe commit-message convention in the prompt or docs.
  Rationale: Commit policy belongs to the target project and its hooks, not to `ahm`.
  Date/Author: 2026-06-12 / Codex
- Decision: Do not add special design work for sessionless agents in this task.
  Rationale: Full task-work orchestration is session-based. Other agents will be supported explicitly when their session/resume contracts are implemented.
  Date/Author: 2026-06-12 / Codex

## Outcomes & Retrospective

Task 057 is implemented. `ahm task work <id>` now accepts `--commit`, includes
the requested commit handoff in dry-run preview, and after session work,
optional review feedback, and optional completion handoff, resumes the original
session with a prompt asking the delegated agent to commit the completed work.

The implementation preserves the git boundary: `ahm` does not run git commands,
does not choose commit-message convention, does not push, and does not open pull
requests. ADR 008 records the delegated commit handoff decision and ADR 006 now
points to it as a partial supersession.

Documentation in README, `docs/spec.md`, and `docs/cli.md` describes `--commit`
as explicit delegated orchestration. Tests cover commit handoff invocation,
review-plus-commit ordering, dry-run preview, prompt content, and failure
wrapping.

A deslop pass found one correctness issue after completion: an explicit
`--commit` request should fail if session capture fails, because otherwise the
CLI can report success without running the requested commit handoff. The
implementation now returns an error for missing or unparsable session IDs when
`--commit` is set, while preserving the older warning-and-skip behavior for
`--complete`.

Validation passed with:

    go test ./internal/ahm -run 'TestTaskWork'
    just fmt
    go test ./internal/ahm
    go test ./internal/templates
    just ci

After the deslop fix, validation additionally passed with:

    go test ./internal/ahm -run 'TestTaskWorkCommit|TestTaskWorkCakeSession|TestTaskWorkCompletionMissingSession'

## Context and Orientation

`ahm` is a Go CLI. Cobra wiring and task commands live in `internal/ahm/task_commands.go`. The `task work` command currently accepts `--agent`, `--review`, and `--complete`. It resolves a task, validates dependencies, chooses an external agent, optionally marks a pending task `In Progress`, and invokes the selected agent.

The relevant types and helpers are in `internal/ahm/task_commands.go`. `taskWorkArgs` stores parsed flags. `taskWorkAgent` stores provider-specific executable and argument builders. `taskWorkWithSession` captures `cake` JSON output, parses `session_id`, and then optionally calls `runReview` and `runCompletion`. `cakeResumeArgs` builds `cake --resume <sessionID> --output-format json <prompt>`.

`scripts/task-workflow.sh` is the behavior source for this feature. Its final step runs `cake --resume "$session_id"` with this intent: now that deslop feedback has been addressed, commit the completed work for the ticket, make sure the task is marked completed before committing, and include task files and project source files in a single commit.

The durable decision is ADR 008, `docs/adr/008-delegated-task-work-commit-handoff.md`. ADR 006 is still valid for the original delegation boundary, except for the part that excluded commit handoff.

## Plan of Work

First, add a `commit bool` field to `taskWorkArgs`, register `--commit` on `task work`, and include `commit: true` in dry-run output when the flag is set. If dry-run output is already being touched, also include `review: true` when requested so the preview describes all orchestration flags consistently.

Next, extend `taskWorkWithSession` to accept a commit flag and run a new `runCommit` helper after review and completion. The commit helper should mirror `runCompletion`: print a short stage marker to stderr, build a prompt, use `agent.resumeArgs(sessionID, prompt)`, and run the delegated command. The returned error should be wrapped as `commit handoff failed: ...`.

Add `buildTaskWorkCommitPrompt(taskID string) string`. The prompt should say to commit the completed work for the task, make sure the task is marked completed before committing, and include both task files and project source files in a single commit. It should also say not to push or open a PR. It should not mention Conventional Commits.

Update tests in `internal/ahm/task_commands_test.go`. Add tests for `--commit` orchestration, `--review --commit` ordering, dry-run preview, prompt content, and commit failure behavior. Existing completion tests should keep passing. The tests should stub `taskWorkRunCommand` and assert the invocation sequence rather than requiring real external CLIs.

Update `docs/cli.md` for `--commit`, supported-agent capabilities, dry-run preview, examples, and the safety boundary. Update `README.md` and `docs/spec.md` so they state that `ahm` does not run git operations directly but can perform explicit delegated commit handoff when requested.

## Concrete Steps

Work from `/Users/travisennis/Projects/ahm`.

Run focused tests during implementation:

    go test ./internal/ahm -run 'TestTaskWork'

After Go edits, format:

    just fmt

Before handoff, run full CI:

    just ci

If generated task or ExecPlan indexes become stale after metadata moves, run:

    ahm index

## Validation and Acceptance

The implementation is accepted when these behaviors are observable:

- `ahm --dry-run task work 001 --commit` previews `commit: true` and does not invoke an external agent.
- `ahm task work 001 --commit` with a stubbed `cake` session runs the initial work invocation and then one resume invocation for commit.
- `ahm task work 001 --review --commit` runs work, review, review-feedback resume when feedback exists, then commit resume, in that order.
- The commit prompt includes the task ID, says to make sure the task is marked completed before committing, says to include task files and project source files in one commit, and says not to push or open a PR.
- The commit prompt does not mention Conventional Commits.
- If the commit resume command fails, the CLI returns a non-zero error that names commit handoff.
- README and CLI docs explain that `--commit` is explicit and delegated; `ahm` itself does not run git commands.
- `just ci` passes.

## Idempotence and Recovery

Creating ADR 008 and this ExecPlan is additive. Re-running `ahm index` is safe because indexes are generated. Re-running `task work --commit` may ask the delegated agent to commit again, so users should only rerun it after checking repository state. If the delegated commit command fails, fix the underlying issue and rerun the command or manually resume the agent; `ahm` should not partially mutate task state beyond the existing `Pending` to `In Progress` claim.

## Artifacts and Notes

The key script prompt to preserve is from `scripts/task-workflow.sh`:

    Now that all of the deslop feedback has been addressed, commit the completed work for ticket ${task_id}. Make sure the task is marked completed before committing. Include both task files and project source files in a single commit.

The prompt in Go can adjust wording from ticket to task, but must preserve the behavior.

## Interfaces and Dependencies

No new Go module dependencies are needed. Use the existing `taskWorkRunCommand`, `taskWorkLookPath`, `taskWorkAgent.resumeArgs`, and `cakeResumeArgs` test seams.

The intended helper signatures are:

    func (a *app) runCommit(agent taskWorkAgent, executable, sessionID, taskID string) error
    func (a *app) buildTaskWorkCommitPrompt(taskID string) string

The `taskWorkWithSession` signature should grow a commit flag:

    func (a *app) taskWorkWithSession(agent taskWorkAgent, executable string, args []string, review bool, complete bool, commit bool, taskID string) error

## Revision Notes

2026-06-12: Created the initial plan after reading task 057, ADR guidance, ADR 006, the script workflow, task-work code, docs, and tests. The plan records the explicit decisions made with the user before implementation.

2026-06-12: Completed the implementation, documentation, tests, and validation.
The plan was moved from active to completed after recording outcomes so workflow
validation can confirm task-to-ExecPlan lifecycle coherence.

2026-06-12: Ran the deslop review pass requested after completion. The review
kept one concrete fix: explicit `--commit` now fails when session capture fails
instead of silently skipping commit handoff. This keeps `--complete` behavior
unchanged pending a later decision.
