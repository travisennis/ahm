# Split task_commands.go god file into focused files

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This plan follows `.agents/PLANS.md`. It is self-contained so a contributor can resume from this file without the original task discussion.

## Purpose / Big Picture

After this change, `internal/ahm/task_commands.go` (currently 1549 lines — the largest file in the project) will be split into 8-9 focused files by concern. No behavior will change. Every function will keep its current name, signature, and behavior. Only file boundaries move.

This matters because the file violates the architectural invariant from `ARCHITECTURE.md`: "Command handlers should stay thin: parse args, validate boundaries, delegate to focused helpers, then emit output." The file currently mixes command wiring, agent definitions, session orchestration, agent output parsers, task CRUD, list/filter/sort, enum validation, and task ID resolution all in one place. A maintainer fixing a bug in one area must navigate 1549 lines of unrelated code.

The target split:

| New file | Content |
|----------|---------|
| `task_agents.go` | `taskWorkAgent` struct, `parseTaskWorkAgent`, `selectTaskWorkAgent`, `codexBypassApprovalsAndSandboxFlag` |
| `task_session.go` | `taskWorkWithSession`, `runReview`, `runCompletionHandoff`, `runCommitHandoff`, `taskWorkReviewPrompt`, `completedTaskMessage`, `truncatedID` |
| `task_parsers.go` | All `parse*SessionID` and `parse*ReviewFeedback` functions (cake, codex, cursor, claude) |
| `task_create.go` | `taskCreateParsed`, `taskCreateParsedLocked`, `resolveTaskCreateBody`, `nextTaskID`, `taskCreateArgs` |
| `task_work.go` | `taskWork`, `ensureTaskDependenciesComplete`, `buildTaskWorkPrompt`, `markTaskInProgress`, `runTaskWorkCommand`, `taskWorkDryRunStatus`, `taskWorkArgs`, `taskWorkLookPath`, `taskWorkRunCommand` |
| `task_status.go` | `taskStatusWithArgs`, `taskUnblockDependents`, `taskDependsOn`, `taskUnblockPreview`, `warnCancellationAcceptancePlaceholder`, `upsertCancellationReason`, `isCancellationReasonHeading`, `bucketForStatus`, `taskStatusArgs` |
| `task_list.go` | `taskList`, `taskListCommand`, `taskLabels`, `taskNext`, `printTaskLine`, `filterTasks`, `filterTasksByStatus`, `filterTasksByLabels`, `taskLabelSet`, `taskLabelSummary`, `summarizeTaskLabels`, `depsComplete`, `priorityRank`, `normalizeTaskLabels` |
| `task_enum.go` | `validateTaskCreateEnums`, `validateTaskEnums`, `enumError`, `normalizeTaskStatus`, `enumKey`, `validTaskStatus`, `validTaskPriority`, `validTaskEffort`, `containsString` |
| `task_find.go` | `resolveTaskFromTasks`, `resolveTask` |

After the split, `task_commands.go` will contain only the `taskCommand()` wiring function and the `taskListCommand` constructor (roughly 80 lines).

## Progress

- [ ] File-by-file split: create each target file by moving functions, in dependency order (task_enum → task_agents → task_parsers → task_session → task_create → task_work → task_status → task_list → task_find)
- [ ] Clean up `task_commands.go` to contain only the wiring function
- [ ] Build, vet, and test: `go build ./cmd/ahm && go vet ./internal/ahm/ && go test -count=1 ./internal/ahm/`
- [ ] Format: `just fmt-check`
- [ ] Update docs if ARCHITECTURE.md module map references `task_commands.go` as the sole task file

## Surprises & Discoveries

To be filled during implementation. Likely items: unused imports from moving subsets of functions; tests that import unexported functions from the original file (same package, so no import needed); cross-references between new files that require a shared import.

## Decision Log

TBD during implementation.

## Outcomes & Retrospective
