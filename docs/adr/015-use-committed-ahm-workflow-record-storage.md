---
status: accepted
date: 2026-07-11
decision-makers: Travis Ennis
consulted: -
informed: task 172a
---
# Use Committed .ahm Workflow Record Storage

## Context and Problem Statement

ADR 013 chose gitignored `.ahm/` working files plus a private `refs/ahm/records`
ref as the durable storage model for tasks, research notes, and ExecPlans. The
goal was to keep workflow churn out of branch history while preserving
durability through explicit ref synchronization.

A release-readiness review of the implemented design (`migrate-issues.md`,
2026-07-11) reproduced six blockers in that synchronization surface: a linked
Git worktree can publish an empty backlog over the remote records ref; the
canonical fresh-clone `prime` flow fails to materialize records and ends in
ref divergence; `records pull` can silently discard an unpushed local
snapshot; migration moves research attachments and other non-Markdown files
into a gitignored location that snapshots never store; successful
synchronization dirties committed `.ahm/config.json` with a sync timestamp;
and migration never seeds the remote ref, so the documented workflow can run
indefinitely with no remote backup. Beyond the blockers, divergence had no
supported recovery workflow, custom refs lack reflogs by default, remote
support was narrow, and session startup became dependent on network and
Git-ref behavior.

The root cause is structural: gitignored working files backed by a private ref
make `ahm` a synchronization engine. It must own materialization, divergence
detection, conflict recovery, worktree semantics, clone bootstrap, and remote
compatibility — responsibilities Git already implements for ordinary tracked
files. Repairing the design would require an ancestry-aware sync state
machine, worktree detection, remote seeding, divergence recovery workflows,
and a large new test matrix (enumerated in `migrate-issues.md`) for an
unreleased feature with no adopters.

The `.agents/` to `.ahm/` namespace separation from ADR 013 remains valuable:
`.agents/` is becoming the ecosystem-standard home for project-owned agent
content, and `ahm` should not mix tool-owned workflow state into it. The
question is how `.ahm/` records are tracked, not where they live.

## Decision Drivers

- Source records must survive machine loss, fresh clones, linked worktrees,
  and branch operations without `ahm`-owned synchronization machinery.
- Routine `ahm` operation must not fetch, push, or mutate refs, branches,
  `HEAD`, or the project index, and must not require the network.
- Keep a clear ownership boundary: tool-owned workflow state under `.ahm/`,
  project-owned agent content under `.agents/`.
- Session start (`ahm prime`) must be fast, offline, and deterministic.
- Derived artifacts must not churn branch history or become merge surfaces.
- Committed configuration must not be dirtied by routine commands.
- Ref-backed mode is unreleased with no migrated repositories, so it can be
  replaced directly without a compatibility bridge.

## Considered Options

- **Repair the ref-backed design.** Implement the fix list from
  `migrate-issues.md`: an ancestry-aware sync state machine, worktree refusal,
  remote seeding, divergence recovery, machine-local metadata, and full
  multi-clone test coverage.
- **Gitignored `.ahm/` records with no sync.** Keep records local-only and
  out of history entirely.
- **Keep records committed under `.agents/`.** Revert the namespace move and
  retain the pre-ADR-013 layout.
- **Committed `.ahm/` source records under normal Git tracking.** Keep the
  namespace boundary from ADR 013; return source records to ordinary tracked
  project files.

## Decision Outcome

Chosen option: **committed `.ahm/` source records under normal Git tracking**,
because normal Git already provides the durability, clone, worktree, merge,
conflict, and recovery semantics that the ref-backed design had to reimplement
and got wrong. Repairing the ref design would permanently tax the project with
synchronization-engine maintenance for a feature no repository has adopted;
gitignored-only records lose the backlog with the machine; reverting to
`.agents/` gives up the ownership boundary that motivated the namespace move.

This ADR supersedes ADR 013. Ref-backed mode shipped in no release and has no
migrated repositories, so it is replaced directly rather than retained as a
compatibility mode: `refs/ahm/*` is no longer created, read, snapshotted, or
synchronized, and the ref-sync command surface and its configuration fields
(`store_mode: ref`/`local`, `records_ref`, `records_remote`,
`records_last_sync`) are removed rather than deprecated.

### Ownership and Tracking Rules

- **Source records** — task files, research notes and their attachments, and
  ExecPlans under `.ahm/tasks/`, `.ahm/research/`, and `.ahm/exec-plans/` —
  are ordinary committed project files. Every file migration moves stays
  tracked, so attachments and non-Markdown files keep normal Git durability.
- **Generated workflow indexes** under `.ahm/` (task, research, and ExecPlan
  indexes) are ephemeral machine-local derivatives: ignored via the managed
  `.ahm/.gitignore`, deterministic, and regenerated from source records by
  `ahm prime` and `ahm index`. They are never committed and never a merge
  surface.
- **Machine-local state** (locks, and any future per-machine metadata) is
  ignored and lives outside committed configuration. Successful routine
  commands leave a clean worktree apart from intentional record edits.
- **Configuration** in `.ahm/config.json` is committed and holds only
  repository-scoped settings.
- **`docs/adr/index.md`** is explicitly different from the workflow indexes:
  it is project-facing documentation, generated by `ahm adr` commands and
  `ahm index`, and remains committed.
- **`.agents/`** remains project-owned committed agent content that `ahm`
  reads but does not manage during routine operation.

### Branch-Scoped Records

Records are branch-scoped state governed by normal Git checkout and merge
semantics. A task created on one branch is not visible on other branches until
integrated, and there is no repository-global live backlog across simultaneous
branches. Sequential IDs are retained: simultaneous creation of the same next
ID on two branches produces an ordinary add/add conflict at merge time,
resolved by renumbering one record. This is an accepted tradeoff — conflicts
are visible in standard Git tooling instead of hidden in synchronization
behavior.

Linked worktrees and fresh clones need no special handling: each checkout
carries the committed records for its commit, and the first `ahm prime`
regenerates the ignored indexes locally.

### Command Behavior

- `ahm prime` returns to local repository preparation and briefing: validate
  workflow state, regenerate ignored indexes, and print the session briefing.
  It performs no fetch, push, or ref mutation and never requires the network.
- `ahm records migrate` is retained as the explicit migration trigger for
  legacy committed `.agents/` repositories. It moves task, research, and
  ExecPlan records to `.ahm/` paths, creates committed `.ahm/config.json`,
  and installs a `.ahm/.gitignore` covering only derived and machine-local
  state. It performs no ref operations and, as always, does not stage, commit,
  or move `HEAD`; the user reviews and commits the move as an ordinary change.
- **Rollback** uses normal Git: before the user commits, `git restore` (or
  `git checkout`) returns the tree to the pre-migration state; after commit,
  `git revert` of the migration commit restores the legacy layout. No
  `git rm --cached` step and no ref cleanup exist to undo.
- The ref-sync command surface (`records push`, `records pull`,
  `records sync`, ref-oriented `records status`) is retired.

### Consequences

- Good, because durability, cloning, worktrees, merging, conflict
  resolution, reflogs, and recovery are provided by Git rather than
  reimplemented, eliminating all six reproduced release blockers by
  construction.
- Good, because `ahm` restores its simple safety guarantee: no ref, branch,
  `HEAD`, index, or network mutation in routine operation.
- Good, because session startup is fast, offline, and deterministic again.
- Good, because the `.ahm/` versus `.agents/` ownership boundary is preserved
  without inventing a storage engine.
- Good, because attachments and non-Markdown record files are durable without
  a special snapshot format.
- Bad, because workflow-record churn (task edits, grooming, completed work)
  returns to branch history and pull requests — the cost ADR 013 tried to
  avoid. Teams that care can integrate record changes in separate commits or
  squash them.
- Bad, because there is no repository-global backlog view across unmerged
  branches; visibility follows integration.
- Bad, because sequential-ID collisions across branches surface as manual
  merge conflicts.
- Bad, because unreleased ref-backed code, tests, and documentation must be
  removed across migration, prime, config, and validation paths.

## More Information

- Release-readiness review with reproduced blockers: `migrate-issues.md`.
- Replacement tracker: task 172; this decision: task 172a.
- Research note: `.agents/.research/topics/records-storage-via-git-refs.md`.

- Supersedes [ADR-013](013-use-ref-backed-workflow-record-storage.md).
