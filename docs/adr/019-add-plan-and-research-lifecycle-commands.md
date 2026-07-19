---
status: proposed
date: 2026-07-19
decision-makers: Travis Ennis
---
# Add Plan and Research Lifecycle Commands

## Context and Problem Statement

Tasks and ADRs have command-owned creation, lookup, lifecycle transitions, and
index regeneration. ExecPlans and research notes do not. Agents must currently
choose a record-layout-specific directory, invent a filename, copy a template,
move files between lifecycle buckets, update task references, and regenerate
indexes by hand. Those manual steps are the same operations that `ahm` already
owns for other workflow records, and mistakes surface only later through
`status` or `doctor` findings.

The storage premise recorded in task 154 changed while the task was blocked.
ADR 013's private-ref model was superseded by ADR 015. Both supported layouts
now store source records as ordinary committed files: legacy repositories use
`.agents/`, and repositories migrated with `ahm records migrate` use `.ahm/`.
Every new command therefore needs to resolve paths through `workflowPaths`,
must not perform Git or network operations, and must preserve normal branch,
worktree, merge, and recovery behavior.

ExecPlans intentionally have no front matter. Their lifecycle is encoded by
placement in `exec-plans/active/` or `exec-plans/completed/` and by the
`Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes &
Retrospective` sections established by ADR 004. Research is deliberately less
structured: Markdown notes live in `inbox`, `investigations`, `sources`,
`topics`, or `archived`; the plain-text metadata header described by `ahm
context research` is recommended but optional. The command design must improve
ergonomics without silently imposing a new incompatible record format.

## Decision Drivers

- Eliminate the most common path, filename, template, task-link, and index
  mistakes without turning flexible Markdown records into a database schema.
- Preserve ADR 004's placement-and-sections ExecPlan lifecycle.
- Preserve raw research notes and existing research attachments.
- Behave identically in legacy `.agents/` and migrated `.ahm/` layouts.
- Keep every write explicit, dry-run aware, serialized by the workflow-record
  lock, atomic per file, and recoverable after partial failure.
- Match established `task` and `adr` conventions for text, JSON, plain output,
  usage errors, lookup ambiguity, idempotency, and deterministic ordering.
- Keep generated indexes derived and local-only; source records remain normal
  committed files and `ahm` performs no Git operations.

## Considered Options

- Keep creation and lifecycle entirely manual and improve only documentation.
  This preserves flexibility but leaves the known failure-prone steps
  unassisted.
- Add only `create`, `list`, and `show`. This is a useful first increment but
  does not solve lifecycle moves or stale task-to-plan references.
- Add full `plan` and `research` command families over the existing formats.
  Commands own placement, safe transitions, and generated-index updates while
  record bodies remain user-owned.
- Introduce front matter and stable IDs for both record types. This would make
  parsing and transitions uniform, but it creates an unnecessary migration and
  breaks the intentional lightweight nature of research and ExecPlans.

## Decision Outcome

Chosen option: **add singular `ahm plan` and `ahm research` command families
over the existing file formats**, because it removes mechanical workflow errors
without changing the meaning or storage format of existing records.

The first supported Plan surface is:

- `ahm plan create <title> [--task <id>] [--description <text> | --body-file
  <path|->]`
- `ahm plan list [--status active|completed]`
- `ahm plan show <identifier>`
- `ahm plan complete <identifier>`

`plan create` writes to the active bucket. With `--task`, its filename is
`<task-id>-<kebab-title>.md`; without a task it is `<kebab-title>.md`. A
collision is an explicit error rather than an implicit overwrite. The optional
task must resolve uniquely and must not already reference a different plan.
The command writes the new repository-relative active path into that task's
`exec_plan` field. The default body is a complete but unfilled ExecPlan
skeleton. `--description` seeds Purpose / Big Picture, while `--body-file`
supplies the body after the command-owned H1 and is mutually exclusive with
`--description`. Before writing either form, the command requires all four ADR
004 sections, requires Outcomes & Retrospective to be empty for the new active
plan, and rejects duplicate or malformed lifecycle headings. A body file cannot
be used to create a plan that immediately produces lifecycle findings.

`plan list` reads both buckets by default, sorts by repository-relative path,
and reports title, lifecycle status, path, and referencing task IDs. `plan
show` prints raw Markdown in text mode and a parsed record in structured modes.
Identifiers accept a repository-relative path, a filename or stem, or a unique
numeric task prefix such as `154`; ambiguity is an error and lookup never
guesses.

`plan complete` is a guarded lifecycle transition, not a validation bypass. It
requires all four ADR 004 sections, a non-empty Outcomes & Retrospective, and no
open checklist items in Progress. It then moves the plan to the completed
bucket, updates every task `exec_plan` value that resolves to the old active
path, and regenerates indexes. Running it against the same completed plan is
idempotent. No `--force` bypass is included in the first version. A prematurely
completed plan can be corrected manually until a safe `plan reopen` contract
is justified.

The first supported Research surface is:

- `ahm research create <title> [--kind
  inbox|investigation|source|topic] [--description <text> | --body-file
  <path|->]`
- `ahm research list [--kind
  inbox|investigation|source|topic|archived]`
- `ahm research show <identifier>`
- `ahm research move <identifier> --to
  inbox|investigation|source|topic|archived`
- `ahm research archive <identifier>`

The CLI uses singular kind names while mapping them to the existing plural
directories. `research create` defaults to `inbox`, uses a kebab-case filename,
and refuses collisions. Its default body uses the current optional header and
sections from `ahm context research`; a body file permits a raw note and is not
rewritten to inject metadata. Commands therefore accept every existing
Markdown note whether or not it has the recommended header.

`research list` is bucket-aware and deterministically reports kind, title, and
path. `research show` uses an explicit path, filename, or unique stem and
rejects ambiguity. `research move` changes only bucket placement and generated
indexes; it does not infer or rewrite optional `Status`, `Updated`, links, or
body prose. `research archive` is the discoverable equivalent of `research
move --to archived`. Repeating either transition at its destination is
idempotent.

Before any plan or research move, `ahm` computes the intended post-mutation
state and refuses the command if the move would introduce a broken relative
link in the managed workflow surface. The command does not rewrite user-owned
Markdown links. Users can update links to their destination paths first, then
rerun the move. This keeps lifecycle assistance inside ahm's safety boundary
without broad, surprising body edits.

Both command families use `workflowPaths` for layout selection and the shared
workflow-record lock for read-compute-write consistency. They validate all
inputs and the proposed destination before the first write, use the existing
atomic write or safe same-filesystem rename paths, regenerate all generated
indexes through the centralized index renderer, and emit post-mutation
workflow findings. Multi-file operations are atomic per file, not
transactional, consistent with ADR 001. Their ordering and idempotent retry
behavior must leave a partial failure diagnosable and resumable. `--dry-run`
reports source-record moves, task-reference updates, and stale index targets
without writing anything.

Creation templates are dedicated embedded artifacts compiled into the binary.
Commands must not parse the prose returned by `ahm context plan` or `ahm
context research`, and they must not depend on installed template copies.
Implementation keeps each command template and its corresponding context
guidance semantically aligned through tests. Changing scoped guidance requires
the normal embedded template-version bump; changing only command rendering is
tied to the binary version.

### Consequences

- Good, because common record operations become layout-aware and regenerate
  indexes automatically.
- Good, because `plan complete` makes the existing validation rules a
  transition precondition and repairs structured task references in the same
  command.
- Good, because old and raw Markdown records remain readable and manageable
  without migration.
- Good, because dedicated embedded templates avoid stale installed copies and
  avoid coupling record creation to long instructional prose.
- Bad, because plan completion can touch several task files and remains only
  per-file atomic; retry and diagnostics are required after an I/O failure.
- Bad, because safe research moves may require users to update relative links
  before the transition.
- Bad, because research's optional metadata can disagree with its directory;
  the directory is authoritative for command lifecycle and list output.
- Neutral, because source-record moves appear as ordinary Git changes for the
  user to review, stage, merge, or revert; ahm itself performs no Git action.

## Open Questions

- Should a later `plan reopen` command refuse plans referenced by completed
  tasks, or atomically reopen those tasks as well? The first version omits the
  command rather than choosing an implicit task transition.
- Should research metadata become a validated format in a future ADR? This
  decision deliberately keeps the current header optional and treats bucket
  placement as authoritative.
- Should research attachments eventually be addressable as a note-owned group?
  The first version moves only the selected Markdown file and refuses a move
  that would break discoverable managed links.
- Should `plan create --task` become required after usage data shows whether
  standalone plans are intentional? ADR 004 currently permits them and reports
  only an informational orphan finding, so this design keeps them valid.

## Follow-up Task Breakdown

1. Add shared workflow-document discovery, unambiguous identifier resolution,
   post-state link checking, structured output types, and golden CLI fixtures
   for plan and research records in both record layouts.
2. Add dedicated embedded Plan and Research creation templates and tests that
   keep their required sections aligned with validation and context guidance.
3. Implement `plan create`, `plan list`, and `plan show`, including optional
   task linkage, dry-run behavior, output modes, and documentation.
4. Implement guarded `plan complete`, task-reference rewriting, idempotent
   recovery, post-mutation validation, and legacy/migrated layout tests.
5. Implement `research create`, `research list`, and `research show`, including
   raw body input, kind mapping, deterministic output, and documentation.
6. Implement `research move` and `research archive` with post-state link
   safety, idempotent recovery, attachment-preservation tests, and both layouts.
7. Update the CLI reference, workflow specification, architecture module map,
   workflow-upgrade notes, embedded context guidance, and end-to-end command
   transcripts; run the full repository verification and release checks.

## More Information

- Task 154: design spike and acceptance scope.
- ADR 001: atomic writes and concurrency protection.
- ADR 004: ExecPlan lifecycle validation and mandatory sections.
- ADR 015: committed `.ahm` workflow record storage, superseding ADR 013.
- `internal/ahm/workflow_paths.go`: legacy and migrated record-path selection.
- `internal/ahm/validation.go`: ExecPlan lifecycle and task-reference checks.
- `internal/ahm/indexes.go`: research and ExecPlan discovery and generated
  indexes.
- `internal/templates/workflow/PLANS.md` and `RESEARCH.md`: current embedded
  workflow guidance.
