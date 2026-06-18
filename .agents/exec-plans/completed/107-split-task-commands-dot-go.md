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

- [x] File-by-file split: created each target file by moving functions verbatim (task_enum, task_find, task_agents, task_parsers, task_session, task_create, task_work, task_status, task_list)
- [x] Cleaned up `task_commands.go` to contain only the wiring functions (`taskCommand` and `taskListCommand`)
- [x] Build, vet, and test: `go build ./cmd/ahm && go vet ./internal/ahm/ && go test -count=1 ./internal/ahm/` all pass
- [x] Format: `just fmt-check` passes
- [x] Updated `ARCHITECTURE.md` module map: replaced the single `task_commands.go` bullet with one bullet per new file

## Surprises & Discoveries

- The plan estimated `task_commands.go` would shrink to ~80 lines, but the two
  wiring functions it specifies (`taskCommand` + `taskListCommand`) are 181
  lines as written. Only those two functions remain, so the acceptance intent
  (file holds wiring only) is met; the ~80 line figure was an undercount.
- The plan's file map listed `runCompletionHandoff`/`runCommitHandoff` for
  `task_session.go`, but the actual functions are named `runCompletion` and
  `runCommit` (plus their `buildTaskWork*Prompt` helpers). Names were preserved
  per the "no renames" constraint; all completion/commit code went to
  `task_session.go` as intended.
- The plan's file map did not place `taskShow`, the `taskStatus` thin wrapper,
  the per-agent `*ResumeArgs` functions, or the agent stream-event types. These
  were assigned by concern (see Decision Log).
- No unused-import problems arose because imports were selected per file up
  front. Tests are same-package (`package ahm`) and reference moved unexported
  functions directly, so no test files needed changes; the full
  `internal/ahm` suite passes unchanged.
- `ARCHITECTURE.md` (repo root), not `docs/ARCHITECTURE.md`, holds the module
  map; that is the file updated. Historical references to `task_commands.go` in
  completed ExecPlans, completed task files, and ADRs were left untouched as
  point-in-time records.

## Decision Log

- Per-agent `*ResumeArgs` functions (`cakeResumeArgs`, `codexResumeArgs`,
  `cursorResumeArgs`, `claudeResumeArgs`) and the stream-event types
  (`cakeStreamEvent`, `codexStreamEvent`/`codexItemEvent`, `cursorStreamEvent`,
  `claudeStreamEvent`) went to `task_parsers.go` alongside the parsers. They all
  encode the same per-agent CLI protocol and the event types are used only by
  the parsers, so co-locating keeps each agent's protocol code together. The
  agent registry (`task_agents.go`) references them by name within the package.
- `taskShow` went to `task_list.go`, grouped with the other read/display
  commands (`taskNext`, `taskLabels`, `taskList`); `taskNext` already shows a
  single task, so this is the same concern.
- The `taskStatus` thin wrapper went to `task_status.go` next to
  `taskStatusWithArgs` it delegates to.
- `const codexBypassApprovalsAndSandboxFlag` went to `task_agents.go` (used by
  the agent definitions); the `taskWorkLookPath`/`taskWorkRunCommand` test-seam
  var block went to `task_work.go` next to `runTaskWorkCommand`.

## Outcomes & Retrospective

`internal/ahm/task_commands.go` was reduced from 1548 lines to 181 lines holding
only `taskCommand` and `taskListCommand`. Behavior is unchanged: every function
kept its name, signature, and body; only file boundaries moved. The work split
into nine new focused files:

| File | Lines |
|------|-------|
| `task_parsers.go` | 228 |
| `task_list.go` | 257 |
| `task_status.go` | 251 |
| `task_create.go` | 156 |
| `task_work.go` | 151 |
| `task_session.go` | 152 |
| `task_agents.go` | 112 |
| `task_enum.go` | 73 |
| `task_find.go` | 55 |

Verification: `go build ./cmd/ahm`, `go vet ./internal/ahm/`,
`go test -count=1 ./internal/ahm/` (ok, ~20s), and `just fmt-check` all pass.
No CLI behavior, output format, or exported/unexported contract changed.

## Preflight Pass

A preflight review (M/L scale; external-agent-orchestration surface) was run on
the change. The decisive check for a move-only refactor was a line-level
equivalence proof: stripping package/import/blank lines from the original
`task_commands.go` (git HEAD) and from the union of the ten resulting files
yields 1426 identical sorted lines with a zero `diff` — proving no function was
dropped, duplicated, or altered. Final validation `just ci` (fmt-check,
tidy-check, vet, test-race, lint, vuln, build, release-check) passed end to end.

The preflight also caught stale `internal/ahm/task_commands.go` pointers that
this refactor invalidated, and repointed them at the new files: six ADRs
(003 → task_create, 005/007 → task_status, 006 → task_work/agents/session/parsers,
008 → task_session, 010 → task_create), `docs/guides/testing.md`,
`internal/ahm/testdata/agents/README.md`, and the `task_work_smoke_test.go`
header comment. The ADR README sanctions "fixes stale references" as a valid
ADR edit. One reference was deliberately left untouched: the orientation note in
the unrelated, still-active `.agents/exec-plans/active/074-madr-adr-management.md`,
which is another contributor's in-flight plan and out of this ticket's scope.

Revision note: this section was appended after the task was marked complete to
record the preflight pass, the move-equivalence evidence, and the follow-on
doc-pointer fixes. No code behavior changed during preflight.
