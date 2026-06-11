# Require Task Cancellation Reasons

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This plan follows `.agents/PLANS.md`. It is self-contained so a contributor can resume from this file without the original task discussion.

## Purpose / Big Picture

After this change, cancelling a task with `ahm task cancel <id>` always records why the task was abandoned. A user can see the feature working by running `ahm task cancel 001 --reason "Superseded by 002"` and then opening `.agents/.tasks/cancelled/001.md`; the task body will contain a `## Cancellation Reason` section with that text. Running the same command without `--reason` fails before writing anything.

## Progress

- [x] (2026-06-11 12:51Z) Read task 073, task workflow guidance, current cancellation implementation, acceptance parser, CLI docs, spec, and ADR precedent.
- [x] (2026-06-11 12:51Z) Created ADR 007 documenting the durable cancellation-reason decision before code changes.
- [x] (2026-06-11 13:01Z) Added `--reason` wiring to `task cancel` and validation that the trimmed reason is required even with `--force`.
- [x] (2026-06-11 13:01Z) Persisted the reason in a `## Cancellation Reason` body section, updating an existing section when present and appending one when absent.
- [x] (2026-06-11 13:01Z) Reused the acceptance-note parser to warn when cancellation sees the seeded TODO placeholder.
- [x] (2026-06-11 13:01Z) Added tests for missing reason, provided reason, existing-section replacement, dry-run output, force behavior, and acceptance TODO warning.
- [x] (2026-06-11 13:01Z) Updated `docs/cli.md`, `docs/spec.md`, `.agents/TASKS.md`, `internal/templates/workflow/TASKS.md`, and grooming-backlog skill guidance.
- [x] (2026-06-11 13:01Z) Ran focused tests, template/package tests, formatting, and `just ci`.

## Surprises & Discoveries

- Observation: `taskStatus` is shared by accept, start, complete, cancel, and reopen, so cancellation-specific inputs should be passed through a small argument struct instead of making a separate command path.
  Evidence: `internal/ahm/task_commands.go` registers the status commands from one spec loop and calls `a.taskStatus(args, status)`.
- Observation: The existing acceptance parser already distinguishes a seeded TODO placeholder from other unchecked acceptance items.
  Evidence: `parseAcceptanceNotes` returns `taskAcceptancePlaceholder` when an unchecked item's text is exactly `TODO`.

## Decision Log

- Decision: Implement cancellation reason storage as body-section manipulation around `Task.Body`, leaving front matter unchanged.
  Rationale: ADR 007 requires body-only persistence, and `renderTask` already preserves and rewrites task bodies deterministically.
  Date/Author: 2026-06-11 / Codex
- Decision: Keep `taskStatus` as the single state-transition implementation, but pass a parsed options struct so only cancellation uses `reason`.
  Rationale: The existing shared function already handles bucket repair, dry-run, timestamps, writes, and index regeneration; reusing it avoids a parallel state-move implementation.
  Date/Author: 2026-06-11 / Codex

## Outcomes & Retrospective

Completed. `ahm task cancel <id>` now requires `--reason <text>`, rejects an empty trimmed reason even with `--force`, persists the reason in `## Cancellation Reason`, updates an existing cancellation section instead of duplicating it, includes the reason in dry-run output, and warns when acceptance notes still contain the seeded TODO placeholder. The CLI reference, workflow spec, installed task workflow, embedded task workflow template, and grooming-backlog skill guidance now describe the required reason flag.

Deslop follow-up fixed two review findings: AGENTS guidance now includes the required `--reason` flag, and replacement of an existing `## Cancellation Reason` section preserves a blank line before the next Markdown heading. Post-deslop validation passed with `go test ./internal/ahm -run 'TestTaskCancel|TestAgentsSuggestions'`, `go test ./internal/templates ./internal/ahm`, `just fmt`, and `just ci`.

## Context and Orientation

`ahm` is a Go CLI. Task command wiring lives in `internal/ahm/task_commands.go`. The function `taskCommand` creates Cobra subcommands, and `taskStatus` performs state transitions for `accept`, `start`, `complete`, `cancel`, and `reopen`. A task's expected bucket is selected by `bucketForStatus`: `Completed` tasks live in `.agents/.tasks/completed`, `Cancelled` tasks live in `.agents/.tasks/cancelled`, and every other status lives in `.agents/.tasks/active`.

Task parsing and rendering live in `internal/ahm/tasks.go`. `parseTask` reads Markdown front matter and strips the top-level title from the task body. `renderTask` writes canonical front matter, a top-level `# Title`, and the trimmed task body. This means cancellation reason persistence can safely edit `Task.Body` before calling `renderTask`.

Acceptance note parsing lives in `internal/ahm/task_acceptance.go`. `parseAcceptanceNotes` scans `## Acceptance Notes`, `## Acceptance Criteria`, or `## Acceptance` sections and returns findings. `taskAcceptancePlaceholder` means the section still contains the seeded `- [ ] TODO` placeholder.

The user-facing CLI reference is `docs/cli.md`. Workflow semantics are summarized in `docs/spec.md`. The canonical task workflow template is `internal/templates/workflow/TASKS.md`; `.agents/TASKS.md` is the installed copy in this repository. Generated indexes under `.agents/.tasks/` and `.agents/exec-plans/` must not be edited by hand.

ADR 007, `docs/adr/007-task-cancellation-reasons.md`, is the durable decision for this feature. It says `--reason` is required, `--force` does not bypass it, the reason is stored under `## Cancellation Reason`, dry-run includes the reason, and cancellation warns on seeded acceptance TODOs.

## Plan of Work

First, adjust command wiring in `internal/ahm/task_commands.go`. Replace the status command loop's direct `taskStatus(args, status)` call with a parsed struct, for example `taskStatusArgs{ids: args, status: status, reason: cancelReason}`. Add a `--reason` string flag only to the `cancel` command. Validate in `taskStatus` that when `status == "Cancelled"`, `strings.TrimSpace(reason)` is not empty. Return a usage error such as `task cancel requires --reason` before any file write or dry-run output. Do not check `a.opts.force` for this validation.

Second, add helper functions near `taskStatus` to maintain the cancellation section in `Task.Body`. The helper should normalize CRLF to LF, find a Markdown heading exactly named `Cancellation Reason` at level two or three using the existing `headingLevel` helper, replace that section's contents until the next heading at the same or higher level if present, or append a new `## Cancellation Reason` section otherwise. The persisted reason should be the trimmed flag value. It may contain multiple lines; preserve those lines under the heading.

Third, in the cancellation path, run `parseAcceptanceNotes` and print warnings only for `taskAcceptancePlaceholder`. Do this before dry-run output and before writing so users see the warning during preview as well. Do not warn on missing acceptance notes or other unchecked acceptance items for cancellation.

Fourth, update dry-run output. The existing dry-run status path emits `move` and `status`; for cancellation include `reason` too. The plain output should therefore include `reason: <text>` and JSON output should contain the same key.

Fifth, add tests in `internal/ahm/task_commands_test.go`. Integration-style `runCLI` tests should prove missing reasons fail, `--force` does not bypass the requirement, provided reasons persist in `.agents/.tasks/cancelled/<id>.md`, dry-run prints the reason and does not move the file, and seeded acceptance TODOs produce a warning. A direct unit test may cover replacing an existing `## Cancellation Reason` section if that is simpler than a full CLI scenario.

Sixth, update docs. In `docs/cli.md`, change `task cancel <id>` to `task cancel <id> --reason <text>`, document the required flag, dry-run preview, `--force` non-bypass behavior, and the acceptance TODO warning. In `docs/spec.md`, add the cancellation contract near workflow state or CLI contract. In `.agents/TASKS.md` and `internal/templates/workflow/TASKS.md`, change cancellation guidance from manually noting a reason to using the enforced `--reason` flag and body section.

Finally, run `go test ./internal/ahm -run 'TestTaskCancel|TestTaskStatus'`, `go test ./internal/ahm`, `just fmt`, and `just ci`. After validation passes, fill task 073 acceptance notes, update this plan's outcomes, move this file to `.agents/exec-plans/completed/073-task-cancellation-reasons.md`, update task 073 `exec_plan` to the completed path, and run `ahm task complete 073`.

## Concrete Steps

From repository root `/Users/travisennis/Projects/ahm`, run:

    go test ./internal/ahm -run 'TestTaskCancel|TestTaskStatus'
    go test ./internal/ahm
    just fmt
    just ci

Expected focused-test result after implementation is a passing `ok` line for `./internal/ahm`. Expected full CI result is success from the repository's `just ci` recipe.

Actual validation run:

    go test ./internal/ahm -run 'TestTaskCancel|TestTaskStatus'
    ok  	github.com/travisennis/ahm/internal/ahm	1.618s

    go test ./internal/templates ./internal/ahm
    ok  	github.com/travisennis/ahm/internal/templates	0.193s
    ok  	github.com/travisennis/ahm/internal/ahm	13.507s

    just fmt
    go fmt ./...

    just ci
    ok  	github.com/travisennis/ahm/internal/ahm	14.892s	coverage: 86.9% of statements
    ok  	github.com/travisennis/ahm/internal/templates	1.203s	coverage: 100.0% of statements
    0 issues.
    No vulnerabilities found.

Manual behavior to observe after implementation:

    ahm task cancel 001

This fails with an error naming the missing `--reason`. Then:

    ahm task cancel 001 --reason "Superseded by 002"

This prints `001 -> Cancelled`, moves the file to `.agents/.tasks/cancelled/001.md`, and the file contains:

    ## Cancellation Reason

    Superseded by 002

Dry-run:

    ahm --dry-run task cancel 001 --reason "Superseded by 002"

This prints a preview containing `move`, `status: Cancelled`, and `reason: Superseded by 002`, while leaving the original active task file in place.

## Validation and Acceptance

The implementation is accepted when cancellation without `--reason` fails before writing, cancellation with a reason writes or updates the `## Cancellation Reason` body section, `--force` still fails without a reason, dry-run previews the reason without moving files, seeded acceptance TODOs warn during cancellation, docs describe the behavior, and `just ci` passes.

Task 073 acceptance notes should be updated with the exact verification commands and outcomes before completing the task.

## Idempotence and Recovery

The helper that writes `## Cancellation Reason` must be idempotent: running cancellation again with a different reason should replace the existing section contents instead of appending duplicate sections. If tests or docs edits fail, rerun the focused tests after each fix. Do not edit generated indexes by hand; use `ahm index`, `ahm task complete`, or other `ahm` commands to regenerate them.

## Artifacts and Notes

Initial inspection showed the shared state-transition path:

    task.AddCommand(&cobra.Command{ ... RunE: func(...) error { return a.taskStatus(args, status) } })

The cancellation change should preserve that centralized move/index behavior.

## Interfaces and Dependencies

No new external dependencies are needed. The new internal interface should be a small struct local to `internal/ahm/task_commands.go`, such as:

    type taskStatusArgs struct {
        ids    []string
        status string
        reason string
    }

The command method should become:

    func (a *app) taskStatus(parsed taskStatusArgs) error

Tests that currently call `a.taskStatus([]string{"001"}, "Completed")` must be updated to pass the new struct or a small helper constructor. The command-line contract is:

    ahm task cancel <id> --reason <text>

Revision note 2026-06-11: Created initial plan and ADR before implementation because task 073 changes a state transition and user-visible workflow behavior.

Revision note 2026-06-11: Updated progress, validation evidence, and outcomes after implementation and CI completed successfully.

Revision note 2026-06-11: Recorded deslop follow-up fixes and validation after the review-readiness pass.
