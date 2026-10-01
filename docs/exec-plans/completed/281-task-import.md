# Import task batches from JSON

This living ExecPlan follows `docs/workflow/exec-plans.md`. Task 281 and
ADR 025 authorize the import contract.

## Purpose / Big Picture

A user can convert an offline issue export into task records in one command,
preview allocated IDs and relationships, and fix every refused record before
anything is written. Run `ahm --dry-run task import --from-file tasks.json`,
then the same command without `--dry-run` to create the records.

## Progress

- [x] (2026-10-01) Inspect task 281, routes, allocator, records, and locks.
- [x] (2026-10-01) Record ADR 025 and implementation plan.
- [x] (2026-10-01) Implement JSON boundary, allocation, references, and rollback.
- [x] (2026-10-01) Focused tests pass, including a 155-record export.
- [x] (2026-10-01) Update contracts; subagent review, preflight, and CI pass.
- [x] (2026-10-01) Record acceptance evidence and prepare task completion.

## Surprises & Discoveries

The store counter shares its state file with store observations. Rollback must
hold the store-state lock while snapshotting, writing, and restoring that file.
Existing index generation is sequential and has no batch rollback. Imported
records must be sorted by canonical ID before index rendering; otherwise a
child-first document produces stale indexes. The task described missing links
as errors, but the current validator reports `markdown_link_missing` warnings;
import preserves that behavior. Three review rounds found JSON boundary gaps:
null values, case-insensitive field matching, and duplicate keys. The map used
for raw validation discards earlier duplicate values; validating that map and
then decoding the original object does not establish one consistent schema
interpretation.

## Decision Log

Use an array of objects with optional unique `ref` keys. References prefixed
with `@` identify an object in this document; ordinary numeric IDs identify
existing records. This prevents ambiguity between external issue numbers and
newly allocated task IDs. Allocate top-level tasks in input order before
children, then validate dependencies against the entire proposed graph.

Preserve comments and cancellation reasons in the body. Generate IDs, updated
timestamps, paths, and buckets; refuse unknown JSON fields. Defaults match task
create. Import accepts dependencies on completed tasks for historical records,
but refuses cancelled dependencies on active tasks and dependency cycles.
Relative links remain importer-owned and are checked by the normal status link
validation. This preserves task create's body behavior.

Restore touched record, counter, and generated index files on ordinary write
failure. Report restoration failures explicitly. Atomicity after process death
or power loss is limited to individual files, as ADR 025 explains; no new
journal or record format is introduced.

On 2026-10-01, Trav chose to reject duplicate fields as malformed input. Read
object keys as tokens before building a map, and compare names with
`strings.EqualFold` to match Go's typed decoder. This rejects exact, escaped,
and differently capitalized duplicate names before any value can be hidden.

## Outcomes & Retrospective

Task 281 delivers offline bulk JSON task creation with forward parents and
references, provenance, stable reports, dry-run safety, and ordinary write
failure rollback. A realistic 155-record export and project/home storage tests
pass. The owner-approved duplicate-field rule resolves the review escalation:
tokenized keys preserve information that map decoding would discard. Final
subagent review and all three preflight passes found no remaining issues.
`just ci` passes, including race tests, lint, vulnerability scanning, Markdown,
build, and six release snapshot targets. No commit or push was requested.
The accepted limitation is per-file atomicity after abrupt termination; no
crash-atomic batch journal was added. All planned deliverables are complete.

## Context and Orientation

`internal/ahm/task_commands.go` registers task commands. `task_create.go` owns
`nextTaskIDForPaths` and `nextChildTaskIDForPaths`; these consider parsed tasks
and filenames on disk. `task_id_counter.go` persists the store's next top-level
number. `tasks.go` parses and renders records. `indexes.go` generates task and
ADR indexes. `lock.go` supplies record and store-state locks, acquired in that
order. `write.go` supplies contained atomic writes through `writeOwned`.

## Plan of Work

Add `internal/ahm/task_import.go` with concrete input and output structs, strict
JSON decoding, a read-only planner, and a transaction limited to generated
paths. The planner validates all scalar fields and references, uses existing
allocators, resolves forward parents and dependencies, and checks cycles before
writing. Extract the existing counter's locked write body so import can hold
its state lock across rollback without nested acquisition. Register import in
`task_commands.go`. Update CLI inventory and detailed input/output contracts,
task format ownership prose, workflow write semantics, and the architecture map.

## Milestones

First implement the planner and command. Tests must demonstrate forward parents
and dependencies, every refusal in one response, and no writes on invalid input.
Run `go test ./internal/ahm -run TaskImport` from the repository root.

Next implement file restoration around record, index, and counter writes.
Inject a write failure in tests and compare all preexisting bytes; verify a
subsequent create uses the next unused ID in home mode. Verify the single record-lock scope, assert both lock owners during home-mode
writes, count index generations, and prove dry-run acquires neither lock.

Finally update documentation, run a subagent review and address findings, then
apply the preflight skill and run `just fmt`, `just cli-parity`, and `just ci`.
Record the exact results here and in the task's Acceptance Notes.

## Concrete Steps

Work in `/Users/travisennis/Projects/ahm`. Use temporary-directory test fixtures
and the existing `runCLI`, `projectRoot`, and home-store helpers. No test may
resolve the user's real store. Run focused tests first, format Go, then run the
full CI suite. CLI success emits a report with one outcome per input object;
semantic refusal returns exit 1, malformed documents return exit 2.

## Validation and Acceptance

A realistic export must carry Markdown comments, older created timestamps,
external references, forward dependencies, and a forward parent/child pair.
After import, status must report no findings for the clean example. A broken
relative link must produce `markdown_link_missing` under status's link scope.
Golden JSON and plain output tests must pin the report schema and outcomes.
Invalid batches and dry runs must leave record, counter, registry, and indexes
unchanged. Ordinary write failures must restore the pre-import files.

## Idempotence and Recovery

Import is additive and does not deduplicate external references. Repeating a
successful command creates another batch. Preview first. After a refusal, fix
the file and rerun. After a reported rollback failure or abrupt termination,
inspect the reported paths, remove only records from that failed batch, and run
`ahm index`; keep a raised counter to avoid reusing published IDs.

## Artifacts and Notes

ADR 025 records the durable contract. Verification passed on 2026-10-01:
`go test ./internal/ahm -run TaskImport -count=1`, `just fmt`,
`just cli-parity`, `just docs-md-lint`, `git diff --check`, and `just ci`.
The full suite includes race tests (89.4% package coverage), vet, lint (zero
issues), vulnerability scanning (none found), Markdown lint, build, and six
release snapshot targets. The final duplicate-field regression suite, subagent review, and all three
preflight passes now pass. No verification checks were skipped.

## Interfaces and Dependencies

Use the Go standard library and existing Cobra dependency. Introduce
`taskImportRecord` and `taskImportReport` at the JSON boundary, with a per-record
outcome containing ref, ID, path, parent, dependencies, outcome, and errors.
Use `writeOwned` for all writes. Never modify registry or existing task records.

Revision: initial plan records the accepted scope and the failure boundaries
before implementation.

Revision: focused tests established canonical index ordering and the existing
link-warning contract; implementation and test milestones are complete.

Revision: recorded the required design escalation after the third JSON-boundary
review finding; task remains In Progress pending the duplicate-key decision.

Revision: recorded passing full CI; the design question remains open and
preflight and task completion are deferred.

Revision: owner chose duplicate-field rejection; token-based key validation
resolves the lossy-map design flaw and implementation resumes.

## How did we do?

All three preflight passes found the implementation consistent with the
approved scope. The final subagent review reports no concrete findings after
the owner-approved duplicate-key design change. CLI inventory and contracts
cover the new command, fields, relationships, refusals, and failure recovery.

## Feedback to keep

Token-based key validation is necessary to reject hidden values before map
construction. Canonical ID sorting is necessary for clean generated indexes.
Keep both regression sets and the tests for home-store counter and rollback.

## Feedback to ignore

A durable transaction journal would expand storage semantics beyond the
accepted design. The documented per-file crash guarantee remains appropriate.
No unrelated refactor or new dependency is needed.

## Plan of attack

Run final CI, record passing results in the Acceptance Notes, move this plan to
the completed bucket, and complete task 281 through ahm. Leave the work on
master uncommitted because the user has not requested a commit or push.

## Preflight compliance

Scale: L, because this adds a public command and persistence/concurrency behavior.
Root AGENTS.md and task workflow were read; targeted discovery found no nested
AGENTS.md under changed paths. Task context is 281, inspected through ahm.
Design context is this ExecPlan and the project ExecPlan workflow. Relevant
architecture and ADRs 001, 024, and 025 were reviewed. Documentation impact
covers docs/cli.md, the global, task-command, and task-format references,
workflow-spec.md, ARCHITECTURE.md, and ADR 025 with its generated index.
Tracked diffs and untracked implementation/tests were reviewed. Pass 1 checked
rules and documentation; Pass 2 checked Go/Cobra boundaries, canonical task
models, allocation, references, locks, rollback, and error propagation; Pass 3
checked the focused helper extraction and rejected unrelated abstractions.
Focused TaskImport tests, formatting, CLI parity, and final full CI passed.
No remaining preflight findings.

Revision: final review, preflight, and CI passed; acceptance is recorded and
the plan moves to completed before task completion.
