# Replace ref-backed records with committed .ahm workflow state

This ExecPlan is a living document. The sections `Progress`, `Surprises &
Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to
date as work proceeds. This document is maintained in accordance with the
`ahm context plan` guidance; rerun that command before revising the plan.

Governing decision: [ADR 015](../../../docs/adr/015-use-committed-ahm-workflow-record-storage.md)
(accepted 2026-07-11), which supersedes ADR 013. Review evidence:
[`migrate-issues.md`](../../../migrate-issues.md) at the repository root.
Tracker: task 172; children 172c through 172i sequence the implementation.

## Purpose / Big Picture

`ahm` is a Go CLI that manages repo-local agent workflow records: task files,
research notes, and ExecPlans like this one. Today those records live as
committed Markdown under `.agents/` ("legacy layout"). An unreleased feature,
`ahm records migrate`, moves them to a gitignored `.ahm/` directory backed by a
private Git ref (`refs/ahm/records`) that `ahm` snapshots and synchronizes
itself. A release-readiness review (`migrate-issues.md`, 2026-07-11) reproduced
six data-loss and workflow-breaking paths in that synchronization design, and
ADR 015 replaced the design: source records move to `.ahm/` but remain ordinary
committed project files, and `ahm` performs no ref or network operations.

After this plan is complete, a user with a legacy `.agents/` repository can run
`ahm records migrate`, review an ordinary `git status` showing record files
moved to `.ahm/` paths, and commit that move like any other change. From then
on, clones, branches, linked worktrees, merges, and recovery all behave the way
committed files always behave; `ahm prime` works offline; generated workflow
indexes under `.ahm/` are gitignored and regenerated on demand; and the
ref-sync commands, metadata, and plumbing are gone from the binary.

## Progress

- [x] (2026-07-11) ADR 015 accepted; ADR 013 marked superseded (task 172a).
- [x] (2026-07-11) This ExecPlan written and linked from tracker 172 (task 172b).
- [x] (2026-07-11) Milestone 1: decouple record layout from ref storage mode (task 172c).
  `workflowPathsFor` selects `.ahm/` based on metadata source (`.ahm/config.json`)
  rather than `recordsStorage().Mode`. The commit prompt is unified; both layouts
  produce the same prompt. Six task-work tests fixed to use correct `.ahm/tasks/`
  paths when config is in `.ahm/config.json`.
- [x] (2026-07-11) Milestone 2: rework `ahm records migrate` to preserve Git tracking (task 172d).
  Removed ref seeding, changed gitignore to ignore only generated indexes and
  machine-local state, removed ref-action/ref-seed output fields, changed config
  writing to omit ref fields, updated all migration tests and messages, updated
  `newRefBackedWorkflowRepo` to construct state directly, updated `recordsDoctor`
  to handle committed mode, and updated `docs/references/cli/commands.md`.
- [x] Milestone 3: remove automatic ref sync from `prime` and mutations (task 172e).
- [x] Milestone 4: retire ref-backed records commands and sync metadata (task 172f).
- [x] (2026-07-11) Milestone 5: committed sources versus ignored generated artifacts (task 172g).
  - Added `ensureWorkflowGitignore()` helper that creates `.ahm/.gitignore` with standard entries when in migrated layout.
  - Added `regenerateIndexes()` helper that writes stale indexes without warning emission, safe for `prime` to use before `buildPrimeReport`.
  - Modified `prime()` to ensure dirs, gitignore, and regenerate indexes before validation and briefing (only when workflow metadata is present).
  - Modified `install()` to ensure `.ahm/.gitignore` exists during init/upgrade in migrated layout.
- [x] (2026-07-11) Milestone 6: migration and repository-topology regression coverage (task 172h).
  - Added `newCommittedModeRepoWithMultipleTasks` helper for setting up committed-mode repositories.
  - Added tests: `TestPrimeInCommittedModeShowsTaskBriefing`, `TestPrimeInCommittedModeRegeneratesStaleIndexes`,
    `TestPrimeInCommittedModePreservesGitState`, `TestPrimeInCommittedModeDryRunDoesNotWrite`,
    `TestIndexInCommittedModePreservesGitState`, `TestStatusInCommittedModePreservesGitState`,
    `TestBranchCheckoutRegeneratesIndexes`, `TestRecordsMigratePreservesAttachments`,
    `TestRecordsMigrateHandlesDirtySourceRecordContent`, `TestRecordsMigrateWithAttachmentsInDryRun`.
- [ ] Milestone 7: documentation, rollback guidance, and release notes (task 172i).

## Surprises & Discoveries

- None yet. Record observations here with short evidence snippets as
  implementation proceeds.

## Decision Log

- Decision: Sequence legacy-layout safety (Milestone 1) before any removal of
  ref behavior, and keep every milestone independently green under `just ci`.
  Rationale: legacy `.agents/` repositories are the only released, adopted
  state; they must keep working at every intermediate commit while the
  unreleased ref mode is carved out.
  Date/Author: 2026-07-11, task 172b.
- Decision: Remove the unreleased ref mode outright (`store_mode: ref`/`local`,
  `records_ref`, `records_remote`, `records_last_sync`, `refs/ahm/*` plumbing)
  rather than deprecating it or keeping a transitional mode.
  Rationale: ADR 015 records that ref-backed mode shipped in no release and has
  no migrated repositories, so there is no compatibility contract to preserve;
  keeping it would retain the highest-risk maintenance surface.
  Date/Author: 2026-07-11, per ADR 015.
- Decision: Milestones follow tracker 172's order (c, d, e, f, g, h, i), with
  the note that Milestone 5 (172g) depends only on Milestone 2 (172d) and may
  be executed before Milestone 4 (172f) if that ordering proves easier.
  Rationale: matches the recorded child-task dependency edges (172f needs 172d
  and 172e; 172g needs 172d; 172h needs 172f and 172g; 172i needs 172h).
  Date/Author: 2026-07-11, task 172b.

## Outcomes & Retrospective

## Context and Orientation

Everything below is knowable from the working tree; no prior plan is needed.

Vocabulary used throughout:

- "Source records" are the durable Markdown files (plus research attachments
  such as images) for tasks, research notes, and ExecPlans.
- "Workflow indexes" are generated listing files (`index.md`) that `ahm`
  derives from source records. They are deterministic and regenerable.
- "Legacy layout" keeps source records committed under `.agents/.tasks/`,
  `.agents/.research/`, and `.agents/exec-plans/`, with configuration in
  `.agents/ahm.json`.
- "Migrated layout" (the target of this plan) keeps source records committed
  under `.ahm/tasks/`, `.ahm/research/`, and `.ahm/exec-plans/`, with
  configuration in committed `.ahm/config.json`. Note the migrated bucket
  names drop the leading dot (`.tasks` becomes `tasks`).
- "Ref mode" is the unreleased ADR 013 design being removed: gitignored
  `.ahm/` records snapshotted to the private ref `refs/ahm/records` and
  synchronized by `ahm` itself.
- A "linked worktree" is an additional working directory attached to one Git
  repository via `git worktree add`. Worktrees share refs but have separate
  checked-out files.

Key files, all under `internal/ahm/` unless noted:

- `workflow_paths.go` defines `workflowPaths` and `workflowPathsFor(root)`,
  which today selects `.ahm/` paths whenever metadata's
  `recordsStorage().Mode != recordStoreModeCommitted`. This ties the `.ahm/`
  layout to the ref storage mode; Milestone 1 breaks that tie.
- `install.go` defines the metadata model: `recordStoreMode` with values
  `committed`, `local`, and `ref`; the JSON fields `store_mode`,
  `records_ref`, `records_remote`, `records_last_sync`; and the reader/writer
  that preserves unknown metadata fields. Metadata lives in `.agents/ahm.json`
  (legacy) or `.ahm/config.json` (migrated), the latter preferred when
  present.
- `records.go` is ref-mode plumbing: snapshotting `.ahm/` records into a tree,
  writing `refs/ahm/records`, materializing records from the ref.
- `records_commands.go` implements `ahm records status|pull|push|sync` and the
  shared sync metadata writes (`records_last_sync`).
- `records_migrate.go` implements `ahm records migrate`: preview, conflict
  detection, resumable moves, `.ahm/.gitignore` installation, local ref
  seeding, and the printed `git rm --cached` follow-up.
- `prime.go` implements `ahm prime`; in ref mode it fetches, snapshots,
  pushes, and materializes records in addition to regenerating indexes,
  validating, and printing the session briefing.
- `indexes.go` regenerates the task, research, and ExecPlan indexes, and the
  committed `docs/adr/index.md`.
- `root.go` finds the repository/workflow root; `validation.go` reports
  workflow findings used by `ahm status` and `ahm doctor`.
- Documentation surfaces: `ARCHITECTURE.md`, `docs/references/workflow-spec.md`,
  `docs/references/cli/commands.md`, `docs/guides/workflow-upgrades.md`, and
  the embedded context output printed by `ahm context ...`.

The six reproduced release blockers being designed out (details and code
references in `migrate-issues.md`): a linked worktree can publish an empty
backlog over the remote ref; a fresh clone's `prime` fails to materialize
records and diverges; `records pull` can discard an unpushed local snapshot;
migration moves attachments the ref never stores; successful sync dirties
committed config with `records_last_sync`; and migration never seeds the
remote, so the documented flow can run with no remote backup at all. The
replacement makes each impossible by construction: committed files travel with
clones, branches, and worktrees; there is nothing to pull, push, seed, or
snapshot; and no sync timestamp exists to dirty config.

Compatibility constraints that hold through every milestone: `ahm` never
stages files, writes the Git index, moves `HEAD`, mutates branches, creates
commits, or patches project source. Legacy `.agents/` repositories keep
working unchanged until their user runs migration. Unknown metadata fields are
preserved on rewrite. `AGENTS.md` is never overwritten. ADRs and
`docs/adr/index.md` stay committed.

## Plan of Work

The work proceeds as seven milestones, one per child task, ordered so that the
legacy layout is made safe before any ref behavior is removed. Each milestone
starts with `ahm task start <id>`, ends with `ahm task complete <id>` after its
acceptance notes are checked, keeps `just ci` green, and updates this plan's
`Progress`, `Decision Log`, and `Surprises & Discoveries` sections. Commit at
least once per milestone when asked to commit.

Milestone 1 — Decouple record layout from ref storage (task 172c). Today the
migrated `.ahm/` layout exists only as a side effect of ref mode. Introduce the
smallest metadata/path model that ADR 015 accepts: a repository is either
legacy (records under `.agents/`, metadata `.agents/ahm.json`) or migrated
(committed records under `.ahm/`, metadata `.ahm/config.json`), with no ref
coupling. Concretely, change `workflowPathsFor` in
`internal/ahm/workflow_paths.go` to select `.ahm/` based on the migrated
layout itself (the natural signal is the presence of `.ahm/config.json`, which
`readMetadata` already prefers) rather than on
`recordsStorage().Mode != recordStoreModeCommitted`. Update the metadata model
in `internal/ahm/install.go` so the migrated layout is representable without
`store_mode: ref`/`local`; removal of those fields lands here or in Milestone 4,
whichever keeps intermediate states compiling, but no new writes of ref-mode
fields may be introduced. Root detection in `root.go`, validation in
`validation.go`, and install/upgrade behavior must recognize both layouts. At
the end of this milestone a repository can be in the migrated layout with
records tracked by normal Git, and every workflow command reads and writes
committed `.ahm/` paths there, while legacy repositories are untouched.

Milestone 2 — Rework `ahm records migrate` (task 172d). Keep the command as
the explicit, opt-in migration trigger, and keep its careful mechanics:
dry-run preview, conflict rejection (never overwrite an existing target),
atomic writes, resumability after interruption, preservation of project-owned
`.agents/` content (`AGENTS.md`, `.agents/prompt.md`, skills, and anything
else not a workflow record), and the no-stage/no-commit/no-`HEAD` boundary.
Change what it produces: move task, research (including every attachment and
non-Markdown file), and ExecPlan records to `.ahm/` paths that remain tracked;
write committed `.ahm/config.json` carrying forward repository-scoped settings
without ref fields; install a `.ahm/.gitignore` that ignores only derived and
machine-local state (never source records); seed no ref; and print normal-Git
follow-up instructions (review `git status`, then `git add`-free guidance —
the user commits the already-tracked renames; no `git rm --cached` step
exists because nothing becomes untracked). In `internal/ahm/records_migrate.go`
this means deleting the ref-seeding step and the untracking instructions and
rewriting the success/dry-run output; update
`docs/references/cli/commands.md` alongside so help and reference stay
truthful.

Milestone 3 — Remove automatic ref sync from `prime` and workflow mutations
(task 172e). Return `ahm prime` in `internal/ahm/prime.go` to local
preparation and briefing: ensure workflow directories exist where appropriate,
regenerate ignored indexes, validate, and print the briefing, with no fetch,
push, snapshot, materialization, or ref reads, and no network dependency.
Remove the post-mutation snapshot hooks from task/research/ExecPlan mutation
paths and stop writing `records_last_sync` anywhere, so routine commands leave
committed config byte-identical. This closes blockers 1, 2, 5, and the
network-dependent-session concern for good: after this milestone `prime` in a
fresh clone or linked worktree can only read committed files and rewrite
ignored derived files.

Milestone 4 — Retire the ref command surface and metadata (task 172f). Remove
`ahm records status|pull|push|sync` (the ref-sync-only commands), the
`refs/ahm/*` plumbing in `internal/ahm/records.go`, remote-support
restrictions, and the `store_mode`/`records_ref`/`records_remote`/
`records_last_sync` metadata fields and `recordStoreMode` enum, together with
their tests. `ahm records migrate` remains, now matching ADR 015. General CLI
help must no longer advertise ref synchronization. Unknown, non-ref metadata
fields must still round-trip untouched — extend the metadata tests to prove
it. If any field removal was deferred from Milestone 1, it completes here.

Milestone 5 — Committed sources, ignored derivatives (task 172g). Finalize the
ownership boundary inside `.ahm/`: source records, attachments, and
`config.json` tracked; generated task/research/ExecPlan indexes, locks
(`.ahm/.lock/`), temporary files, and any machine-local artifacts ignored via
the managed `.ahm/.gitignore`. `docs/adr/index.md` stays committed because it
is project-facing documentation browsed without `ahm`. Because ignored indexes
can be stale after a branch checkout (the checkout changes source records but
not ignored files), `prime` must regenerate indexes from source records before
validating and printing the briefing, and commands and validation must treat
source records — never an existing index — as authoritative. `init`,
`upgrade`, `index`, and `prime` must recreate required directories and indexes
deterministically without dirtying a clean worktree. Touch
`internal/ahm/indexes.go`, `install.go`, `validation.go`, and the gitignore
generation in `records_migrate.go`.

Milestone 6 — Regression coverage for what actually failed (task 172h). This
is the verification milestone. Replace the removed ref-sync tests with
fixture-backed end-to-end coverage for the states users really have: legacy
committed repositories (with attachments, custom content, and dirty source
records), partially migrated/interrupted-and-resumed migration, dry-run,
conflicting targets, fresh clones, ordinary branches, and linked worktrees.
Assert the standing invariants: migration and routine commands preserve
`HEAD`, the Git index, branches, and unrelated worktree changes; successful
routine operations leave committed config and source records clean; branch
checkout followed by `prime` regenerates indexes from the checked-out records;
records changed on separate branches integrate through normal merge; and two
branches independently creating the same sequential task ID produce a visible
add/add conflict (test or document this — no cross-branch allocation is being
invented). Walk `migrate-issues.md` blocker by blocker and record, in this
plan and in the test names, whether each has a regression test or is
impossible because the behavior no longer exists. Focused tests and `just ci`
must pass.

Milestone 7 — Documentation, rollback, and release (task 172i). Update
`ARCHITECTURE.md`, `docs/references/workflow-spec.md`,
`docs/references/cli/commands.md`, `docs/guides/workflow-upgrades.md`,
templates, and the embedded `ahm context ...` output to describe one coherent
contract: project-owned committed `.agents/`, committed ahm-owned `.ahm/`
source records, ignored generated artifacts, migration, and rollback. Document
rollback explicitly: before the user commits the migration, `git restore .`
(or `git checkout -- .` plus deleting the new `.ahm/` directory if untracked
files were created) returns to the legacy layout; after committing,
`git revert <migration commit>` restores it. Remove ref-backed setup and sync
instructions everywhere except historical ADR context, and annotate
`migrate-issues.md` findings as resolved or made impossible. Finish with
release guidance: update `CHANGELOG.md`, describe the migration in release
notes without implying an adopter compatibility path, and run
`just release-check` (and `just prepare-release` when actually cutting the
release). Markdown lint (`just docs-md-lint`) and link checks must pass.

## Concrete Steps

All commands run from the repository root, `/…/ahm` (the directory containing
`go.mod` and `justfile`). The Go package root is not the repo root: build with
`just build` or `go build ./cmd/ahm`, never `go build .`.

Per milestone, the loop is:

    ahm prime                       # session briefing, index regen, validation
    ahm task start 172<letter>
    # edit code and tests per the milestone description above
    just fmt
    go test ./internal/ahm -run <FocusedPattern> -count=1
    just ci                         # full read-only suite before handoff
    # check the task's acceptance boxes in .agents/.tasks/active/172<letter>.md
    # update this plan: Progress, Decision Log, Surprises & Discoveries
    ahm task complete 172<letter>

Expected shape of a passing verification:

    $ just ci
    ...
    ok  	github.com/travisennis/ahm/internal/ahm	...
    (vet, fmt-check, tidy-check, lint all silent/passing)

For manual end-to-end checks of migration behavior, build a scratch repository
outside the checkout (never migrate this repository mid-development):

    ahm_bin=$(pwd)/bin/ahm && just build
    tmp=$(mktemp -d) && cd "$tmp" && git init -q demo && cd demo
    "$ahm_bin" init && git add -A && git commit -qm "init workflow"
    "$ahm_bin" task create "Example task"
    git add -A && git commit -qm "task"
    "$ahm_bin" records migrate --dry-run   # preview: moves, config, gitignore
    "$ahm_bin" records migrate
    git status --short                     # expect renames to .ahm/..., no staging

After Milestone 2, that `git status` must show the record moves as ordinary
working-tree changes (nothing staged), `.ahm/config.json` present, and no
`refs/ahm/records` (`git for-each-ref refs/ahm` prints nothing). After
Milestone 3, `ahm prime` in a fresh clone of the committed result must print
the briefing with the task visible and must succeed with networking disabled.

## Validation and Acceptance

The plan is done when the tracker-172 acceptance holds end to end, observable
as behavior:

1. In a scratch legacy repository, `ahm records migrate --dry-run` accurately
   previews moves, config changes, and Git follow-up; running it for real
   moves every source file (Markdown and attachments) to `.ahm/` paths,
   leaves them tracked, stages nothing, moves no `HEAD`, and creates no
   custom ref.
2. After committing the migration, a fresh `git clone` contains all records;
   `ahm prime` there works offline, regenerates ignored indexes, and shows
   the backlog. `git worktree add` produces a worktree where `ahm prime`
   likewise only reads committed files — there is no path by which it can
   delete or publish records.
3. Routine commands (`prime`, task mutations, `index`) leave `git status`
   clean apart from record edits the user intentionally made.
4. `ahm records status|pull|push|sync` no longer exist; `ahm --help` and the
   reference docs describe only supported behavior.
5. Creating the same next task ID on two branches and merging yields a normal
   add/add conflict visible in `git status` — covered by test or explicit
   documentation.
6. `just ci` passes at every milestone boundary; Milestone 6 adds the
   topology/migration suites; Milestone 7 passes `just docs-md-lint` and
   link checks.

Each child task's own Acceptance Notes are the milestone-level gate; check
them in the task file before `ahm task complete`.

## Idempotence and Recovery

Milestones are additive-then-subtractive and each leaves `main` releasable:
land the committed-layout capability first (Milestones 1–2), remove ref
behavior only afterwards (Milestones 3–4), so at no intermediate point does a
legacy repository lose function. If a milestone must be abandoned mid-flight,
`git revert` its commits; no persistent state outside the worktree exists
because `ahm` writes no refs and no external stores after this plan.

`ahm records migrate` itself must remain resumable: if interrupted, rerunning
completes the remaining moves without overwriting conflicting targets, and
`--dry-run` is always safe to rerun. User-facing rollback is pure Git —
`git restore` before commit, `git revert` after — and Milestone 7 documents it.

Scratch-repository experiments live in temporary directories and are deleted
afterwards; never run `records migrate` inside this repository until the
project itself decides to migrate (tracked separately, not by this plan).

## Artifacts and Notes

Evidence to capture here as milestones land: the dry-run and real transcript
of a scratch migration, the `git for-each-ref refs/ahm` empty output, a fresh
clone `prime` briefing, and the add/add conflict transcript from the
sequential-ID scenario. Keep snippets short and indented.

Reference chain: ADR 015 (decision), ADR 013 (superseded design),
`migrate-issues.md` (reproduced failures and rejected repair plan), task 172
(tracker), tasks 172c–172i (milestones), and the completed
`.agents/exec-plans/completed/138-ref-backed-workflow-records.md` (historical
implementation record of the design being removed — useful as a map of every
place ref mode touched).

## Interfaces and Dependencies

No new external dependencies; the work is Go standard library plus the
existing module set. Shapes that must exist at the end:

In `internal/ahm/workflow_paths.go`, `workflowPathsFor(root string)
workflowPaths` selects `toolRecordsDirName` (".ahm") for migrated-layout
repositories and `legacyRecordsDirName` (".agents") otherwise, with no
reference to ref storage modes.

In `internal/ahm/install.go`, the metadata struct has no `StoreMode`,
`RecordsRef`, `RecordsRemote`, or `RecordsLastSync` fields (JSON
`store_mode`, `records_ref`, `records_remote`, `records_last_sync`), and the
`recordStoreMode` enum is gone; unknown-field preservation on rewrite is
retained and tested. Layout selection derives from which config file anchors
the repository (`.ahm/config.json` versus `.agents/ahm.json`).

`internal/ahm/records.go` and the ref-sync portions of
`records_commands.go` are deleted; `records_migrate.go` exposes only the
committed namespace migration. `prime.go` has no network or ref code paths.
CLI command registration for `records` offers `migrate` alone.
