# Implement MADR ADR Management

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This plan follows `.agents/PLANS.md`. It is self-contained so a contributor can resume from this file without the original task discussion.

## Purpose / Big Picture

After this change, `ahm` can create, list, show, transition, supersede, migrate, index, and validate Architecture Decision Records in one supported format: a constrained MADR 4.x profile stored under `docs/adr/`. Users will be able to run commands such as `ahm adr create "Choose storage layout"`, `ahm adr list`, `ahm adr supersede 001 --by 009`, and `ahm index`, then inspect deterministic ADR files and a generated `docs/adr/index.md`.

The feature matters because ADRs already gate durable design work in this repository, but today they are hand-authored legacy Markdown with no command support. The implementation should make ADR workflow as predictable as task workflow while preserving existing decision history.

## Progress

- [x] (2026-06-14 00:00Z) Read task 074, task workflow guidance, documentation workflow guidance, research workflow guidance, ADR design research, existing ADR README, existing ADR files, and ExecPlan requirements.
- [x] (2026-06-14 00:00Z) Wrote ADR 009, `docs/adr/009-madr-adr-management.md`, to ratify the durable MADR-only design.
- [x] (2026-06-14 00:00Z) Created this feature ExecPlan and linked implementation tasks 075-081 to it.
- [ ] Implement task 075: ADR model parsing, rendering, collection, ID resolution, status validation, and legacy classification.
- [x] (2026-06-14 10:08-04:00) Implement task 076: `ahm adr create`.
- [x] (2026-06-14 10:42-04:00) Implement task 077: `ahm adr list` and `ahm adr show`.
- [ ] Implement task 078: ADR lifecycle commands and bidirectional supersession.
- [ ] Implement task 079: generated ADR index and ADR validation findings under the `workflow` check scope.
- [ ] Implement task 080: rewrite the managed ADR workflow template for MADR and update agent suggestions.
- [ ] Implement task 081: `ahm adr migrate` and migrate this repository's legacy ADR metadata.
- [ ] Complete the feature by running focused checks, template checks, and `just ci`; update this plan's Outcomes & Retrospective; then move this plan to `.agents/exec-plans/completed/`.

## Surprises & Discoveries

- Observation: This repository's `docs/adr/README.md` and `internal/templates/workflow/adr-README.md` are currently identical legacy-format guidance.
  Evidence: Both document `# ADR NNN: Short Decision Title`, bold `Status` and `Date` lines, and `Superseded in part` as a status.
- Observation: Existing ADR numbers 001-008 are all legacy Markdown records with no front matter, so ADR 009 can serve as the first constrained-MADR example without migrating history early.
  Evidence: `find docs/adr -maxdepth 1 -type f | sort` lists 001-008 plus README, and `docs/adr/007-task-cancellation-reasons.md` begins with `# ADR 007:` followed by bold metadata.

## Decision Log

- Decision: Use `docs/adr/` and three-digit `NNN-kebab-slug.md` filenames instead of upstream MADR's default `docs/decisions/` and four-digit examples.
  Rationale: This repository and existing `ahm` consumer guidance already use `docs/adr/`; preserving the location avoids needless migration and broken references.
  Date/Author: 2026-06-14 / Codex
- Decision: Support only the constrained MADR 4.x profile for new `ahm adr` records.
  Rationale: A single supported format keeps command behavior, validation, rendering, and migration tractable. Legacy records are tolerated only so migration can be staged safely.
  Date/Author: 2026-06-14 / Codex
- Decision: Keep ADR validation in the existing `workflow` check scope for v1.
  Rationale: ADRs are managed workflow documents like tasks, research, and ExecPlans. A new scope would add CLI surface before there is evidence users need it.
  Date/Author: 2026-06-14 / Codex
- Decision: Represent partial supersession in the ADR body, not in `status:`.
  Rationale: MADR's status model supports full supersession via `superseded by ADR-NNN` but has no first-class partial supersession status. Keeping the status `accepted` and recording the partial replacement in `## More Information` preserves clarity without inventing a non-MADR status.
  Date/Author: 2026-06-14 / Codex
- Decision: Implement `ahm adr migrate` as metadata-only conversion.
  Rationale: Legacy ADR bodies are historical records. Rewriting their sections into MADR would create noisy, risky churn without changing the decision data needed by commands.
  Date/Author: 2026-06-14 / Codex

## Outcomes & Retrospective

Not completed yet. Task 074 established ADR 009 and this implementation plan. The feature remains open until tasks 075-081 are implemented, this repository's legacy ADR metadata is migrated, and validation passes through `just ci`.

## Context and Orientation

`ahm` is a Go CLI for installing and maintaining repository-local agent workflow files. Cobra command wiring starts in `internal/ahm/cli.go`. Task command behavior lives mostly in `internal/ahm/task_commands.go`, and the task model, parser, renderer, ID helpers, and front matter parser live in `internal/ahm/tasks.go`. New ADR command work should mirror those task patterns where they fit, but without sharing task-specific concepts such as task buckets.

The existing ADR guidance is stored twice. `docs/adr/README.md` is this repository's installed copy, and `internal/templates/workflow/adr-README.md` is the embedded template installed into consumer repositories. Both currently describe the legacy format. The final feature must update the embedded template and, where appropriate, this repository's installed copy.

Existing ADRs live in `docs/adr/`. Files `001-atomic-writes-and-concurrency.md` through `008-delegated-task-work-commit-handoff.md` use the legacy shape: a heading like `# ADR 007: Task Cancellation Reasons`, then bold `Status` and `Date` lines. ADR 009, `docs/adr/009-madr-adr-management.md`, is the first constrained-MADR example and is the durable design decision for this plan.

The constrained MADR profile means the file has YAML-style front matter, but only scalar `key: value` lines are valid. The core fields are `status`, `date`, `decision-makers`, `consulted`, and `informed`. `decision-makers`, `consulted`, and `informed` are comma-separated strings, not YAML block lists. Unknown fields must be preserved on rewrite. Valid statuses are `proposed`, `accepted`, `rejected`, `deprecated`, and values matching `superseded by ADR-NNN`.

Generated indexes are source-controlled workflow artifacts owned by `ahm`. Existing generated index logic lives in `internal/ahm/indexes.go`; validation for managed workflow state lives in `internal/ahm/validation.go` and is surfaced by `internal/ahm/status.go`. Do not edit generated indexes by hand. Use `ahm index` or commands that regenerate indexes.

Atomic write helpers live in `internal/ahm/write.go`. Any ADR command that writes files should use those helpers so partial writes and stale temp files follow the same safety rules as tasks.

User-facing command documentation lives in `docs/cli.md`. Durable workflow semantics live in `docs/spec.md`. Upgrade behavior for managed templates lives in `docs/upgrades.md`. When ADR commands or templates change user-visible behavior, update these docs in the same task.

## Plan of Work

Milestone 1 is task 075, the ADR model. Add a new `internal/ahm/adrs.go` and `internal/ahm/adrs_test.go`. Define an ADR struct with fields for ID, slug, title, status, date, decision makers, consulted, informed, preserved unknown front matter, path, body, and a classification for malformed or legacy records. Implement collection over `docs/adr/*.md`, excluding `README.md` and `index.md`, sorted by numeric ID. Implement parsing with the existing scalar front matter parser. Parse the first H1 as the title. Implement stable rendering with canonical core field order and preservation of unknown fields. Implement `resolveADR` accepting `9`, `009`, and `009-slug` without substring matches. Implement `nextADRID`. Implement status validation for the fixed statuses and `superseded by ADR-NNN`. Legacy files must not abort collection; they should produce a distinct classification that later validation and migration can use.

Milestone 2 is task 076, creation. Wire a new `adr` command family in `internal/ahm/cli.go` and implement `ahm adr create <title>` near the task command patterns. Creation allocates the next ID, slugifies the title, writes `docs/adr/NNN-kebab-slug.md`, stamps `date:` with the current date, defaults `status:` to `proposed`, and seeds a constrained-MADR body. Add `--status`, `--description` or `-d`, and `--body-file <path|->` with behavior analogous to `task create`. Honor `--dry-run` and output modes. Regenerate indexes after a real create. Update `docs/cli.md`.

Milestone 3 is task 077, read commands. Implement `ahm adr list` and `ahm adr show <id>`. `list` prints ID, title, status, and date sorted by ID, with `--status <value>` filtering. For superseded records, prefix filtering on `superseded` is acceptable because the replacement ID is embedded in the status. `show` resolves by the same ID forms as the model. Support text, plain, and JSON output consistently with task commands. Legacy or malformed files should be reported without making all readable records unusable. Update `docs/cli.md`.

Milestone 4 is task 078, lifecycle and supersession. Implement `ahm adr accept <id>`, `ahm adr reject <id>`, and `ahm adr deprecate <id>` as front matter status transitions that also update `date:`. Implement `ahm adr supersede <old-id> --by <new-id>` as a bidirectional mutation. It sets the old record's status to `superseded by ADR-NNN`, writes or replaces a short supersession note in the old body, and adds a reciprocal reference to the new record's `## More Information` section. Reject unknown IDs, self-supersession, and attempts to supersede an already fully superseded record unless a later task explicitly adds a force option. Preserve all unrelated body content. Update `docs/cli.md`.

Milestone 5 is task 079, generated index and validation. Add `docs/adr/index.md` generation to `internal/ahm/indexes.go` with the standard generated-file banner and a deterministic table of ADR, Title, Status, and Date. Dry-run should report the ADR index only when stale. Add validation findings for parse failures, invalid statuses, filename/metadata ID disagreement, duplicate IDs, unresolved supersession references, stale generated ADR index, and legacy-format files that should be migrated. Put these findings under the existing `workflow` check scope. Legacy-format findings should be non-fatal enough that this repository remains usable before task 081 migrates records. Update `docs/spec.md` and `docs/cli.md`.

Milestone 6 is task 080, managed template and agent guidance. Rewrite `internal/templates/workflow/adr-README.md` to document only the constrained MADR profile and implemented `ahm adr` commands. Update agent suggestions in `internal/templates/templates.go` so generated AGENTS guidance treats `docs/adr/index.md` as ahm-owned and routes ADR creation and status changes through `ahm adr`. Bump `templates.Version` and update `docs/upgrades.md`. Keep this repository's workflow docs coherent with the new status model, especially replacing "Superseded in part" status guidance with the body-note rule from ADR 009. Run `go test ./internal/templates ./internal/ahm` before `just ci` for this milestone.

Milestone 7 is task 081, migration. Implement `ahm adr migrate` as an idempotent metadata-only converter for legacy records. It should derive ID and slug from the filename, convert `**Status:**` and `**Date:**` into constrained-MADR front matter, strip the `ADR NNN:` prefix from the H1, remove consumed bold metadata lines, and preserve the rest of the body. Map legacy `Proposed`, `Accepted`, and `Deprecated` directly to lowercase. Map full supersession only when the replacement ADR can be resolved unambiguously. For partial supersession, keep `status: accepted` and preserve or add a body note that names the partial replacement. Support `--dry-run`. After implementation, migrate this repository's ADRs 001-008 so validation is clean. Update `docs/cli.md`.

## Concrete Steps

From repository root `/Users/travisennis/Projects/ahm`, start each implementation task by reading `.agents/.tasks/index.md` and the specific task file. Do not start tasks 076-081 until their declared dependencies are complete.

For task 075, run:

    go test ./internal/ahm -run 'TestADR|TestResolveADR|TestNextADRID'
    go test ./internal/ahm

For command tasks 076-078 and 081, run a focused command test first, then the package:

    go test ./internal/ahm -run 'TestADR'
    go test ./internal/ahm

For task 079, include workflow/index validation tests:

    go test ./internal/ahm -run 'TestADR|TestIndex|TestStatus|TestDoctor|TestValidation'
    go test ./internal/ahm

For task 080, include template tests:

    go test ./internal/templates ./internal/ahm

After any Go edit, run:

    just fmt

Before final handoff for each implementation task, run:

    just ci

Expected successful package output is an `ok` line for `github.com/travisennis/ahm/internal/ahm`. Expected full CI result is a successful `just ci` run with tests, lint, and vulnerability checks passing.

## Validation and Acceptance

The full feature is accepted when a user can create a new ADR with `ahm adr create`, list it with `ahm adr list`, view it with `ahm adr show`, move it through accepted, rejected, and deprecated lifecycle states, supersede an older ADR with bidirectional references, generate a clean `docs/adr/index.md` with `ahm index`, and migrate legacy ADRs with `ahm adr migrate` without changing historical body prose.

`ahm status` and `ahm doctor` must report ADR workflow problems clearly. Legacy ADRs should produce actionable migration findings before task 081 and no findings after task 081 migrates this repository's own records.

Every task that changes CLI behavior must update `docs/cli.md`. Task 079 must update `docs/spec.md`. Task 080 must update `docs/upgrades.md` and run template tests. Each task must run `just ci` before completion unless an exact blocker is recorded.

## Idempotence and Recovery

ADR collection and rendering must be deterministic. Running `ahm index` twice without source changes should leave the worktree unchanged. Running `ahm adr migrate` after migration should report no changes. Running `ahm adr supersede <old> --by <new>` twice should replace or preserve the defined supersession notes without duplicating them.

If an ADR command fails halfway through validation before writing, it should leave files untouched. If a write fails, the atomic write helpers in `internal/ahm/write.go` should prevent partial target files. If generated indexes become stale, rerun `ahm index`; do not patch generated index files by hand.

## Artifacts and Notes

ADR 009 ratifies the user-visible design and should be treated as the source of truth for format and workflow decisions:

    docs/adr/009-madr-adr-management.md

This plan sequences the task family:

    075 -> 076, 077, 078, 079
    076 + 078 + 079 -> 080
    075 + 079 -> 081

Existing legacy ADRs to migrate in task 081:

    docs/adr/001-atomic-writes-and-concurrency.md
    docs/adr/002-advisory-agents-suggestions.md
    docs/adr/003-task-create-body-file.md
    docs/adr/004-exec-plan-lifecycle-validation.md
    docs/adr/005-task-acceptance-completion-checks.md
    docs/adr/006-task-work-agent-delegation.md
    docs/adr/007-task-cancellation-reasons.md
    docs/adr/008-delegated-task-work-commit-handoff.md

## Interfaces and Dependencies

No new external dependency is required for v1. Reuse existing Go standard library packages, Cobra patterns already present in the CLI, `internal/ahm/output.go` output helpers, `internal/ahm/write.go` atomic write helpers, and the existing scalar front matter parser unless task 075 proves a small shared helper extraction is necessary.

The new internal ADR model should live in `internal/ahm/adrs.go`. The implementation should expose small functions analogous to task helpers, with names like `collectADRs`, `parseADR`, `renderADR`, `resolveADR`, `nextADRID`, and `validADRStatus`. Keep function signatures concrete and local to the CLI package unless a test proves an exported boundary is needed.

The new command contract is:

    ahm adr create <title> [--status proposed|accepted|rejected|deprecated] [--description <text>|--body-file <path|->]
    ahm adr list [--status <value>]
    ahm adr show <id>
    ahm adr accept <id>
    ahm adr reject <id>
    ahm adr deprecate <id>
    ahm adr supersede <old-id> --by <new-id>
    ahm adr migrate [--dry-run]

Revision note 2026-06-14: Created initial plan and ADR 009 before implementation because tasks 075-081 add durable ADR workflow behavior, generated index semantics, migration behavior, and managed template changes.
