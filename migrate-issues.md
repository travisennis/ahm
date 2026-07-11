# Ref-Backed Records Migration: Release-Readiness Issues

Date reviewed: 2026-07-11

## Scope and Verdict

This review covers the current `ahm records migrate` command and the
ref-backed records behavior that repositories use after migration. There is no
top-level `ahm migrate` command.

The migration mechanics themselves are careful: the command previews changes,
preserves project-owned `.agents/` content, avoids staging files or moving
`HEAD`, rejects conflicting targets, and can resume an ordinary interrupted
migration. The resulting storage and synchronization system is not ready for
general use, however. Several reproduced paths can hide, overwrite, or remove
workflow records, and normal synchronization modifies committed project state.

Until the blockers below are fixed, ref-backed records should be treated as an
experimental feature for a solo user with one clone and one Git worktree, not
as a safe default or generally supported migration.

## Decision After Review

Ref-backed records are unreleased and no repositories have migrated to that
mode. The project will not add fallback handling or repair the synchronization
design for release. Tracker 172 replaces it directly with a simpler migration:

- `ahm records migrate` moves legacy committed `.agents/` source records into
  committed `.ahm/` paths.
- Records remain branch-scoped and use normal Git checkout, merge, conflict,
  clone, worktree, and recovery behavior.
- Workflow indexes under `.ahm/` become ignored, ephemeral derivatives that
  `ahm prime` regenerates from source records.
- Project-facing generated documentation such as `docs/adr/index.md` remains
  committed.
- Private-ref sync commands, metadata, remote restrictions, and automatic
  network/ref mutation are removed rather than maintained for compatibility.

The findings below are retained as evidence for rejecting the ref-backed
design. They are not a plan to preserve or repair that unreleased mode.

## Verified Release Blockers

### 1. A linked Git worktree can publish an empty backlog

`ahm prime` snapshots and pushes records from the current worktree. Git
worktrees share `refs/ahm/records` but do not share their gitignored `.ahm/`
working files. A newly created linked worktree therefore has the shared records
ref but no materialized records.

Failure sequence:

1. The main worktree has records and pushes them.
2. A linked worktree starts without local `.ahm/` record files.
3. `ahm prime` snapshots that absence as an empty records tree.
4. The empty snapshot fast-forwards the remote ref without warning.

Reproduced result: the remote records ref changed from one record file to zero
after running `prime` in a linked worktree. This is a high-risk path for an
agent workflow tool because coding agents commonly use Git worktrees.

Relevant code:

- `internal/ahm/prime.go:119-130`
- `internal/ahm/records.go:40-156`

### 2. The canonical fresh-clone `prime` flow does not materialize records

When the remote records ref exists and the local ref is missing, the ref
comparison returns `left_missing`. `prime` only materializes records when the
comparison is `behind` or `equal`, so it skips materialization. It then creates
an empty local snapshot and attempts a non-fast-forward push.

Reproduced result:

- the remote task was not materialized or shown in the briefing;
- `prime` reported a push failure;
- local and remote records refs ended in `diverged` state;
- the expected `.ahm/tasks/active/001.md` did not exist.

This breaks the primary multi-machine recovery path promised by ref-backed
storage.

Relevant code:

- `internal/ahm/prime.go:98-130`
- `internal/ahm/records.go:254-297`

### 3. `records pull` can discard an unpushed local snapshot

`records pull` verifies only that local working files match the local records
ref. It does not check whether that local ref is ahead of or diverged from the
remote ref before replacing it. Materialization then replaces or removes local
record files to match the remote tree.

Failure sequence:

1. Machine B creates a task and automatically snapshots it to its local ref.
2. Machine A independently updates and pushes the remote ref.
3. Machine B's push fails because it is not a fast-forward.
4. Machine B runs `ahm records pull`.
5. Pull succeeds and replaces B's local task with A's task.

Reproduced result: `title: Task from machine B` became
`title: Task from machine A`, and B's records ref no longer referenced its
snapshot.

Relevant code:

- `internal/ahm/records_commands.go:211-266`
- `internal/ahm/records.go:159-217`

### 4. Migration moves files that the records ref never stores

Migration moves every file under the task, research, and ExecPlan roots.
Record snapshots include only Markdown files, excluding generated indexes, in
a fixed set of known bucket directories. Research attachments, non-Markdown
files, and files in custom directories are therefore moved into a gitignored
location but omitted from the durability ref.

After the user runs the printed `git rm --cached` command, such files are
neither present in normal branch history nor backed by `refs/ahm/records`.
`records status` also ignores them.

Reproduced result: a migrated research `diagram.png` existed locally, was
absent from the records ref, and `records status` still reported
`working_clean: true`.

Relevant code:

- `internal/ahm/records_migrate.go:244-314`
- `internal/ahm/records.go:40-103`

### 5. Successful synchronization dirties committed configuration

Successful pull, push, sync, and `prime` operations write the current time to
`records_last_sync`. After migration, metadata is stored in the committed
`.ahm/config.json` file. Routine synchronization therefore changes a tracked
project file.

Reproduced result: starting from a clean repository, `ahm prime` left
`.ahm/config.json` modified and printed its own `Dirty Worktree` warning.

This recreates branch-history churn, causes cross-machine timestamp conflicts,
and conflicts directly with agent instructions that require a clean worktree
before starting work.

Relevant code:

- `internal/ahm/records_commands.go:549-552`
- `internal/ahm/install.go:483-495`
- `internal/ahm/prime.go:124-130`

### 6. Migration does not establish remote durability

Migration seeds only the local records ref. The success message tells the user
to commit the new configuration and gitignore but does not require an initial
records push. `prime` pushes only when the remote ref already exists, so it
does not seed an absent remote ref.

Reproduced result: after migration and `ahm prime`, the remote still had no
`refs/ahm/records`. An explicit `ahm records push` was required to create it.

A user can therefore migrate records out of branch history, run the documented
session command regularly, and still have no remote backup.

Relevant code:

- `internal/ahm/records_migrate.go:208-241`
- `internal/ahm/records_migrate.go:491-503`
- `internal/ahm/prime.go:85-130`

## Major Post-Migration Product and UX Issues

### Divergence has no supported resolution workflow

Two machines can easily create divergent snapshots, especially while task IDs
remain sequential. `records sync` detects divergence, but neither the CLI nor
the user documentation provides a safe merge, keep-local, keep-remote, or
recovery procedure. The current push diagnostic can suggest pulling even
though pull is unsafe in a divergent state.

Custom refs also do not normally receive reflogs by default, which makes an
accidentally replaced records ref harder to recover through ordinary Git
commands.

### Existing paths and integrations become stale

Migration moves records without rewriting their contents or project-owned
guidance. Existing Markdown links, scripts, editor settings, automation, and
agent instructions that refer to `.agents/.tasks`, `.agents/.research`, or
`.agents/exec-plans` can break. There is a compatibility mapping for legacy
`exec_plan` front matter, but not for arbitrary links or external tooling.

### Remote support is narrow and migration does not preflight it

The synchronization surface supports GitHub URLs and local remotes used for
testing. GitLab, Bitbucket, GitHub Enterprise, and common SSH aliases are
rejected. Migration defaults to the `origin` remote and can succeed without
verifying that the configured remote exists or is supported, leaving the user
to discover the limitation after records have moved out of branch history.

### Session startup now depends on network and Git-ref behavior

`ahm prime` changes from a local briefing into a command that checks the
network, fetches, snapshots, pushes, writes files, updates refs, regenerates
indexes, and rewrites metadata. This increases latency, credential and SSH
failure modes, concurrency risk, and the amount of recovery behavior that must
remain correct for every agent session.

## Fixes the Rejected Design Would Have Required

The project is not implementing this repair path; it is recorded to show the
maintenance cost avoided by returning source records to normal Git tracking.

1. Use one ancestry-aware synchronization state machine for `prime`, pull,
   push, and sync rather than maintaining partially different behavior.
2. Materialize remote records when the local ref or local record tree is
   absent.
3. Never interpret an absent local record tree as an intentional delete-all
   operation without an explicit destructive action.
4. Detect linked worktrees and refuse unsafe synchronization until worktree
   semantics are deliberately supported.
5. Reject pull when the local records ref is ahead or diverged.
6. Move `records_last_sync` and other machine-local state out of committed
   `.ahm/config.json`.
7. Seed the remote ref during migration, or make an explicit successful first
   push a required and clearly verified migration step.
8. Snapshot every non-derived file that migration moves, or refuse migration
   with a complete list of unsupported files.
9. Add a documented and tested divergence recovery workflow, plus a reflog or
   equivalent safety mechanism for the records ref.
10. Correct path and rollback documentation and tell users how to audit stale
    `.agents/` references.

## Missing Coverage in the Rejected Design

Add end-to-end tests for:

- migration followed by initial remote seeding;
- `prime` in a fresh clone;
- `prime` in a linked Git worktree;
- pulling with an ahead local ref;
- pulling with divergent local and remote refs;
- migration containing attachments and custom files;
- successful synchronization preserving a clean project worktree;
- remote authentication, unsupported remotes, and missing `origin`;
- interrupted materialization and recovery;
- two-machine sequential task creation and divergence handling.

## Verification Performed During Review

- Focused records, migration, workflow integration, and `prime` tests passed.
- `go test ./...` passed.
- Scratch-repository reproductions confirmed all six blockers above.

The passing suite demonstrates that the intended happy paths work, but it does
not currently exercise the multi-clone, multi-worktree, divergence, unsupported
file, initial remote, or clean-worktree scenarios that determine whether the
feature is safe for users.
