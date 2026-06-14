# Auto-Unblock Dependent Tasks After Completion

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This plan follows `.agents/PLANS.md`. It was created as a retrospective process correction after task 096 was implemented and completed, because task 096 changed durable task state-transition semantics and should have had an ExecPlan before implementation.

## Purpose / Big Picture

After this change, finishing a task with `ahm task complete <id>` also makes newly unblocked work visible in the ready queue. A user can see the behavior by creating a `Blocked` task that depends on a pending task, completing the pending dependency, and observing the dependent task change to `Pending` once every dependency is complete. The command still leaves tasks blocked when they have other incomplete dependencies or when they are blocked for a reason unrelated to the completed task.

## Progress

- [x] (2026-06-14 03:38Z) Confirmed task 096 had already been implemented, validated, and completed without an ExecPlan.
- [x] (2026-06-14 03:38Z) Added completion-path logic in `internal/ahm/task_commands.go` to find directly dependent active `Blocked` tasks and move only fully satisfied dependents to `Pending`.
- [x] (2026-06-14 03:38Z) Preserved dry-run behavior by reporting dependent unblock previews without writing task files or indexes.
- [x] (2026-06-14 03:38Z) Added focused tests for direct unblock, multi-dependency blocked behavior, unrelated blocked tasks, and dry-run no-write behavior.
- [x] (2026-06-14 03:38Z) Updated `docs/cli.md`, `docs/spec.md`, `.agents/TASKS.md`, and `internal/templates/workflow/TASKS.md`.
- [x] (2026-06-14 03:38Z) Filled task 096 Acceptance Notes, marked task 096 completed, and reran `just ci`.
- [x] (2026-06-14 03:38Z) Added this retrospective completed ExecPlan and linked it from task 096 to repair the workflow record.

## Surprises & Discoveries

- Observation: `taskStatusWithArgs` is the shared state-transition path for completion and already owns dependency validation, dry-run output, atomic writes, bucket moves, and index regeneration.
  Evidence: `internal/ahm/task_commands.go` performs dependency checks, writes the target task with `writeFileAtomic`, removes the old bucket file when needed, and calls `a.writeIndexes()`.
- Observation: The existing task collection behavior sorts tasks deterministically by ID, so dependent unblock output remains deterministic without adding another sort.
  Evidence: `collectTasks` in `internal/ahm/tasks.go` sorts parsed tasks with `taskLess`.

## Decision Log

- Decision: Reuse the existing completion path instead of adding a separate unblock command or post-index repair pass.
  Rationale: `task complete` is the moment when dependency satisfaction changes, and the shared path already handles timestamps, dry-run previews, atomic writes, and index regeneration.
  Date/Author: 2026-06-14 / Codex
- Decision: Only unblock active tasks with explicit `Blocked` status that directly depend on the completed task and whose full dependency list is complete.
  Rationale: This avoids changing tasks blocked for product, design, or other non-dependency reasons, and it avoids scanning unrelated completed dependencies into pending work.
  Date/Author: 2026-06-14 / Codex
- Decision: Store unblock preview entries under `unblocked` in dry-run output, with `id`, `path`, and `status`.
  Rationale: Existing dry-run output is map-based and human/JSON emitters already handle nested slices; the preview is explicit without adding command-specific formatting.
  Date/Author: 2026-06-14 / Codex
- Decision: Create this ExecPlan retrospectively as completed instead of reopening task 096.
  Rationale: The implementation and validation were already complete; the missing artifact was the durable plan record, not unfinished product work.
  Date/Author: 2026-06-14 / Codex

## Outcomes & Retrospective

Completed. `ahm task complete <id>` now unblocks directly dependent active `Blocked` tasks when every dependency is complete, keeps multi-dependency tasks blocked until all dependencies are completed, leaves unrelated blocked tasks unchanged, includes unblock changes in dry-run output, and regenerates indexes once for the completion plus any dependent status changes. The CLI reference, workflow spec, installed task workflow, and embedded task workflow template now describe the behavior.

This plan was created after implementation to address review feedback that task 096 changed durable workflow semantics without an ExecPlan. The process gap is recorded here; no code changes were needed during the review follow-up.

Validation passed with:

    go test ./internal/ahm -run 'TestTaskComplete(UnblocksDirectDependents|LeavesMultiDependencyBlockedUntilAllComplete|DoesNotUnblockUnrelatedBlockedTasks|DryRunReportsUnblockedDependentsWithoutWriting|SucceedsWithCompletedDependencies|RefusesIncompleteDependencies)'
    go test ./internal/templates ./internal/ahm
    just fmt
    just ci

## Context and Orientation

`ahm` is a Go CLI for managing repository-local workflow files. Task records live under `.agents/.tasks/active`, `.agents/.tasks/completed`, and `.agents/.tasks/cancelled`. A task has front matter fields such as `id`, `status`, and `depends_on`; `depends_on` is a comma-separated list of task IDs that must be completed before the task can be completed safely.

Task command behavior lives in `internal/ahm/task_commands.go`. The relevant command path is `taskStatusWithArgs`, which is used by status-transition commands including `task complete`. Task parsing and rendering live in `internal/ahm/tasks.go`; `Task` is the project-owned in-memory representation, `collectTasks` reads task files, `depsComplete` checks dependency satisfaction, and `renderTask` writes canonical front matter and body content.

Generated indexes are owned by `ahm` and must not be edited by hand. After task or ExecPlan metadata changes, run `ahm index` unless an `ahm task ...` command already regenerated indexes.

## Plan of Work

The implementation changed `taskStatusWithArgs` in `internal/ahm/task_commands.go`. It loads the task list once, validates dependencies before completing the requested task, computes the completion timestamp, and when the target status is `Completed`, calls `taskUnblockDependents`. That helper builds a set of completed task IDs including the task being completed, scans active `Blocked` tasks, filters to tasks that directly depend on the completed ID, and changes only tasks whose full dependency list is complete to `Pending`.

The write path first writes the completed task to its completed bucket and removes the old active file if needed. It then rewrites each newly pending dependent task in place under `.agents/.tasks/active`, calls `a.writeIndexes()`, prints the completed task transition, and prints one `ID -> Pending` line for each unblocked dependent.

Dry-run builds the same list of dependent changes but writes nothing. Its output keeps the existing `move` and `status` preview and adds an `unblocked` list when dependent tasks would change.

Tests in `internal/ahm/task_commands_test.go` cover the behavior. Documentation changes in `docs/cli.md`, `docs/spec.md`, `.agents/TASKS.md`, and `internal/templates/workflow/TASKS.md` describe the state-transition semantics and dry-run preview.

## Concrete Steps

From repository root `/Users/travisennis/Projects/ahm`, run:

    go test ./internal/ahm -run 'TestTaskComplete(UnblocksDirectDependents|LeavesMultiDependencyBlockedUntilAllComplete|DoesNotUnblockUnrelatedBlockedTasks|DryRunReportsUnblockedDependentsWithoutWriting|SucceedsWithCompletedDependencies|RefusesIncompleteDependencies)'
    go test ./internal/templates ./internal/ahm
    just fmt
    just ci

Expected result is passing `ok` lines for the focused and package tests, followed by a successful `just ci` run.

Manual behavior to observe after implementation:

    ahm task complete 001

When active task `002` is `Blocked` with `depends_on: 001`, the command prints:

    001 -> Completed
    002 -> Pending

Task `002` remains in `.agents/.tasks/active/002.md`, but its front matter changes to `status: Pending`. If task `002` has `depends_on: 001, 003` and task `003` is not completed, task `002` stays `Blocked`.

Dry-run:

    ahm --dry-run task complete 001

This prints a preview containing the completion move and an `unblocked` entry for each dependent task that would become pending, while leaving every task file and generated index unchanged.

## Validation and Acceptance

The implementation is accepted when completing a task unblocks directly dependent `Blocked` tasks whose full dependency set is complete, leaves multi-dependency tasks blocked until every dependency is completed, does not change unrelated blocked tasks, reports pending unblocks during dry-run without writing, updates CLI/workflow/spec/template documentation, and passes `just ci`.

Task 096 acceptance notes record the exact validation commands and were checked before completion.

## Idempotence and Recovery

The unblock scan is idempotent for the same task graph: once a dependent task is changed to `Pending`, later completions do not match it because the helper only scans explicit `Blocked` tasks. Dry-run computes the same changes without writing files. If task or ExecPlan metadata is edited after completion, run `ahm index` to regenerate indexes. Do not patch generated indexes by hand.

## Artifacts and Notes

The key completion output for a direct dependent is:

    001 -> Completed
    002 -> Pending

The key dry-run fields are:

    move: .agents/.tasks/completed/001.md
    status: Completed
    unblocked:
      - id: 002
        path: .agents/.tasks/active/002.md
        status: Pending

## Interfaces and Dependencies

No external dependencies were added. The new internal helpers are:

    func (a *app) taskUnblockDependents(tasks []Task, completedID string, updated string) []Task
    func taskDependsOn(task Task, depID string) bool
    func taskUnblockPreview(tasks []Task) []map[string]any

They use the existing `Task` model, `depsComplete`, `renderTask`, and `writeFileAtomic` helpers.

Revision note 2026-06-14: Created this retrospective completed ExecPlan to address review feedback that task 096 changed durable workflow semantics without an ExecPlan.
