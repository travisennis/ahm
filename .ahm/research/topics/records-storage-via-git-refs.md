# Records Storage Via Private Git Refs

Status: active
Created: 2026-06-29
Updated: 2026-07-06
Related tasks: -
Related plans: -
Confidence: medium

## Summary

`ahm` currently stores per-project workflow records (tasks, research notes, and
ExecPlans) as committed files inside the consumer repository. That keeps the
records durable, but it also makes short-lived agent workflow artifacts show up
in branch history, pull requests, and every contributor's working tree.

The revised goal is not to make records usable without `ahm`. `ahm` is designed
primarily for coding-agent workflows, and depending on `ahm` to acquire and
sync its own workflow state is acceptable. The actual product problem is that
tasks, completed tasks, backlog-grooming churn, scratch research, and draft
ExecPlans are mostly **working artifacts**: they may live for a while, but their
important outcomes should graduate into durable project docs, ADRs, or design
docs. The ceremony of creating and advancing them should not remain in the
project's normal commit history.

This note now proposes a two-layer model:

1. **Working layer:** task, research, and ExecPlan files stay at the familiar
   `.agents/` paths as plain Markdown, but are gitignored and untracked from
   the project branch.
2. **Durability/sync layer:** `ahm` snapshots those gitignored working files to
   a private git ref namespace such as `refs/ahm/records`, and explicitly syncs
   that ref to a remote.

The private ref is not the conceptual source of truth that users edit directly.
It is a backup and sync transport for gitignored workflow records. This keeps
ephemeral workflow ceremony out of branch history while still protecting a
solo developer's backlog from laptop loss, disk failure, or moving between
machines.

The agent-facing entry point should probably be a new command such as
`ahm prime`: sync records, validate workflow state, and print a compact
backlog briefing before each coding-agent work session.

Revision note, 2026-07-06: ADR 013 now records the accepted namespace decision.
The recommended working layer moves from `.agents/` to tool-owned `.ahm/`;
`.agents/` remains committed project-owned agent content that `ahm` may read
but does not manage. Older `.agents/` path references below document the
research path that led to the decision and should not be used as implementation
guidance.

## Research Questions

- How can `ahm` keep tasks, research notes, and ExecPlans out of project
  branches and pull requests while preserving enough durability for active
  backlogs?
- What storage model best matches `ahm`'s agent-first workflow, where agents
  can be instructed to run an initialization command before each session?
- Can private git refs provide a same-remote backup/sync layer without touching
  user branches, the index, staging area, `HEAD`, or normal project history?
- What should `ahm` report when workflow records are stale, unsynced, missing
  locally, or diverged from the remote ref?
- Which unresolved distributed-collaboration problems can be deferred because
  solo development is the primary use case?

## Local Context

`ahm`'s current behavior, which this proposal builds on or changes:

- **No implicit git operations** is a documented non-goal and part of the tool's
  trust model (`docs/references/workflow-spec.md` Non-goals;
  `ARCHITECTURE.md` System Boundaries). `ahm` does not commit, push, create PRs,
  or run git operations itself.
- **Records are committed in-repo today.** Tasks live in `.agents/.tasks/`,
  research in `.agents/.research/`, ExecPlans in `.agents/exec-plans/` — all
  project-owned and tracked (File Ownership Boundary category 4). ADRs already
  live separately under `docs/adr/`.
- **Generated indexes** (`.agents/.tasks/index.md`, `.agents/.research/index.md`,
  the two `exec-plans/*/index.md`, `docs/adr/index.md`) are deterministic and
  `ahm`-owned (category 1).
- **Writes are root-scoped and atomic** (temp-file-then-rename; ADR 001).
  `ahm` never writes outside the detected repository root today.
- **`ahm` parses task front matter** into a typed model with a canonical field
  order and enum validation (`docs/references/workflow-spec.md` File Format).
  This parser is what makes a limited task merge feasible.
- **`ahm.json`** already carries repo-scoped settings (`strict_acceptance`,
  `default_work_agent`), so storage and sync settings have a natural home.
- **A `docs/design-docs/` convention** is already recognized by
  `status --check project-docs`, giving durable plans/research a committed
  graduation target.

This proposal sits inside a three-bucket model for what `ahm` puts in a
project:

1. **Tool-level** (instructions, skills) — identical in every repo; belong with
   the binary or at user scope, not in the project tree.
2. **Project-specific but working/ephemeral** (tasks, scratch research, draft
   plans) — present on disk, synced by `ahm`, but not committed to the project
   branch. **This is the bucket this note addresses.**
3. **Project-specific and durable/shareable** (ADRs, graduated design plans,
   durable project documentation) — committed in `docs/`. Unchanged by this
   proposal.

See [[agent-instruction-retrieval-via-ahm]] for the bucket-1 (instructions and
skills) half of the same overall direction.

## Design Criteria

The old design criterion "readable/acquirable without `ahm`" is no longer
load-bearing. The updated criteria are:

- **Keep workflow ceremony out of project history.** Creating tasks, grooming a
  backlog, writing scratch research, and drafting ExecPlans should not produce
  normal branch commits unless the result graduates into durable docs.
- **Keep local ergonomics.** Records should remain plain Markdown under
  `.agents/` so agents and humans can inspect and edit them with normal tools
  after `ahm` has materialized them.
- **Protect active backlog state.** Gitignored-only records are not enough;
  losing a laptop must not mean losing weeks of task and research state.
- **Use the same remote when possible.** For solo projects, backing up records
  through the repository's existing Git remote is preferable to requiring a
  separate service or sync tool.
- **Make agent sessions deterministic.** A coding agent should have one command
  to run before work starts that syncs records and reports the current state.
- **Avoid surprising source-control state.** Routine record storage must not
  touch project branches, the index, staging area, or `HEAD`.
- **Defer large-team collaboration complexity.** The primary design target is
  solo development and low-contention small-team use. Large-team task
  reconciliation can remain out of scope until evidence says otherwise.

## Options Considered

### Option A — Gitignored files in-repo

`.agents/` stays where it is; `ahm` ensures `.gitignore` entries for task,
research, and ExecPlan records. Records are present on disk, readable after
materialization, and uncommitted. Almost no new code.

Weaknesses: no backup or sync. `git clean -x`, disk loss, or a stolen laptop
can wipe the active backlog unless the user has an independent backup system.
This solves branch-history pollution but not durability.

### Option B — External per-repo store

Records move to a central location such as `$XDG_DATA_HOME/ahm/<repo-key>/`.
This is truly out of the tree and can survive `git clean`.

Weaknesses: needs a stable repo key (remote URL? canonical path? both break on
no-remote / multi-remote / worktrees / moved directories), makes records harder
for agents to discover, breaks `ahm`'s root-scoped-writes posture (ADR 001), and
still needs an independent sync/backup answer.

### Option C (naive git) — Orphan branch + worktree

An orphan branch (`ahm-records`) checked out at `.agents/` via
`git worktree add`, with the main branch's `.gitignore` hiding `.agents/`. This
is the `gh-pages` pattern. Gives versioned history, push/pull sharing, and no
pollution of the main branch's history or PRs.

Weaknesses (why it is rejected in this form):

- `.agents/` becomes a real worktree with its own `.git` pointer; `git clean`,
  `rm -rf .agents`, branch switches, and worktree-scanning IDEs interact in
  confusing ways for users who do not think in worktrees.
- A real branch named `ahm-records` clutters `git branch -a`, appears in PR
  base/compare dropdowns, and is one fat-finger from being checked out — a new
  form of the intrusion the proposal is trying to remove.
- The records branch needs the same fetch/merge plumbing as code, inheriting
  merge conflicts on shared records with none of the mitigations below.

### Option D (recommended) — Gitignored working files plus private-ref sync

Keep `.agents/` as **plain gitignored files** (the robust working model from
Option A). Add durability by snapshotting those files onto a **private ref
namespace** with low-level git plumbing.

- Records live locally as normal files under `.agents/`.
- Snapshots live at `refs/ahm/records` — a custom ref, **not a branch**. It is
  invisible to `git branch`, should not appear in PR branch dropdowns, and is
  not accidentally checked out.
- On mutation, or at sync boundaries, `ahm` snapshots `.agents/` records onto
  the ref with plumbing such as `hash-object` / `mktree` / `commit-tree` /
  `update-ref`. This does not touch the working tree, index, staging area,
  `HEAD`, or any branch.
- Remote sync is explicit through `ahm records push`, `ahm records pull`,
  `ahm records sync`, or the agent-facing `ahm prime` command. Ordinary project
  git commands do not need to know the ref exists.

## Why The Recommended Option Wins

| | A: gitignored only | B: external store | D: gitignored + private ref |
|---|---|---|---|
| Out of project commits/PRs | yes | yes | yes |
| Familiar `.agents/` working files | yes | no | yes |
| Survives laptop loss if synced | no | only with separate backup | yes |
| Uses existing Git remote | no | no | yes |
| No branch/worktree clutter | yes | yes | yes |
| Agent can prime state in one command | partial | partial | yes |
| User branches/index untouched in routine use | yes | yes | yes |

Option D addresses the two real requirements together: workflow records stop
polluting project history, and active backlog state can still be recovered from
the same remote after a machine loss. The key reframing is that private refs are
not needed to make records "readable without `ahm`"; they are the backup/sync
layer for an `ahm`-managed working directory.

## Agent-First Flow: `ahm prime`

Because `ahm` is primarily designed for coding agents, the sync model can lean
on explicit agent instructions instead of hidden background behavior.

Proposed command:

```text
ahm prime
  -> fetch/pull refs/ahm/records from the configured remote
  -> merge/materialize records into .agents/
  -> regenerate local generated indexes
  -> run workflow validation
  -> print a session briefing:
     - ready/open/blocked task counts
     - high-priority ready tasks
     - stale or unsynced record warnings
     - active ExecPlans
     - recent research notes
     - suggested next ahm commands
```

Agent instructions can then say: run `ahm prime` before every work session.

For human use, normal mutating commands should avoid surprise network pushes,
but they can emit a note when records appear stale or unsynced:

```text
note: workflow records have local changes not synced to refs/ahm/records; run
`ahm records sync` or `ahm prime`
```

Open policy choice: whether `ahm prime` should always sync, sync by default
with `--no-sync`, or only sync when `records_auto_sync` / `prime_sync` is set.
Given the agent-first model, defaulting `prime` to sync is reasonable as long
as the command is explicitly documented as network-capable.

## Custom Ref Host Support

Git itself supports pushing "branches, tags, or other references" with explicit
refspecs, and the destination of a refspec can be a branch, tag, or other ref.
The default fetch refspec after clone generally maps `refs/heads/*`, so
`refs/ahm/*` will not be fetched by normal clone/fetch unless `ahm` supplies an
explicit refspec such as `+refs/ahm/*:refs/ahm/*`.

GitHub looks compatible with this design:

- GitHub's Git references API can list all references, including namespaces
  beyond heads and tags.
- Creating a reference through that API requires a fully qualified ref that
  starts with `refs` and has at least two slashes; `refs/ahm/records` fits that
  shape.

GitHub smoke test on 2026-06-30 confirmed the core behavior using temporary
private repo `travisennis/ahm-custom-ref-smoke-20260630110156`:

- `git push origin refs/ahm/records:refs/ahm/records` succeeded.
- A normal fresh clone fetched no `refs/ahm/*` refs.
- The normal fresh clone did not contain `.agents/` records from `main`.
- `git fetch origin refs/ahm/records:refs/ahm/records` succeeded.
- The fetched custom ref matched the pushed commit
  `26ff098cbc1386dcdd061c55a22d7694a71592fb`.
- The fetched ref tree contained the expected `.agents/` records, including
  `.agents/.tasks/active/001.md` and `.agents/ahm.json`.
- GitHub's refs API listed `refs/ahm/records`.
- Creating and deleting a separate `refs/ahm/delete-probe` ref both succeeded.

Initial support can target GitHub only. Other hosts, including hosted Bitbucket
Data Center, are not blocking requirements for the first ADR or implementation.
If/when those remotes matter, server-side hooks, branch/ref restrictions, or
enterprise configuration may reject custom namespaces. `ahm` should then include
a remote probe such as `ahm records doctor --remote` or make `ahm records sync`
fail with a precise diagnostic when the remote rejects `refs/ahm/*`.

Sources:

- Git push documentation: https://git-scm.com/docs/git-push
- Git fetch documentation: https://git-scm.com/docs/git-fetch
- GitHub Git refs API: https://docs.github.com/en/rest/git/refs

## The Safety Boundary

Today's contract is "`ahm` runs no git operations." Option D changes that
surface and needs an ADR. The replacement boundary should be honest rather than
overly sloganized:

> In ref-backed record mode, `ahm` may write local workflow files under
> `.agents/`, local refs under `refs/ahm/*`, and the minimal repo configuration
> required to fetch/push that namespace. It must not commit, create PRs, mutate
> user branches, stage files, write the index, move `HEAD`, or patch project
> source code.

That is less absolute than "`ahm` only writes `refs/ahm/*`", but it matches the
actual design:

- `.gitignore` and `.agents/ahm.json` must change during opt-in migration.
- Fetch/push operations may update Git's internal files such as `FETCH_HEAD` and
  packed refs.
- Remote sync mutates a remote ref and depends on credentials/network access.
- Untracking already-committed records still requires an index mutation, so
  migration should preview and print the required `git rm --cached` command for
  the user to run rather than performing it silently.

This preserves the load-bearing trust guarantee: `ahm` will not surprise the
user's branch, staging area, code history, or project source files.

## Merge And Conflict Scope

Any shared-records scheme inherits conflicts: two machines or people can edit
the same task/research note/ExecPlan before syncing. The primary target is solo
development and low-contention small-team use, so the design should keep merge
machinery modest until real usage demands more.

Reasonable initial policy:

1. **Keep generated indexes out of the ref entirely.** Regenerate them locally;
   never snapshot them. Index churn should not become a merge surface.
2. **Prefer whole-record conflict detection for prose.** Research notes,
   ExecPlans, and task bodies are free-form prose. If both sides changed the
   same body, report a conflict rather than pretending to semantically merge it.
3. **Use structured merge only where it is clearly safe.** Task front matter can
   reuse the existing parser for simple non-overlapping field edits, such as one
   side adding labels while the other changes priority.
4. **Do not over-solve task ID collisions yet.** Sequential IDs can remain for a
   solo-first ref-backed prototype. If true distributed task creation becomes a
   supported target, revisit random/hash-like stable task IDs then.

The design should avoid claiming that semantic merge solves the general problem.
It can reduce common task-front-matter conflicts, but it does not solve
distributed state-machine reconciliation for status transitions, dependent
unblocking, cancellation, acceptance notes, or concurrent prose edits.

## Design Decisions To Settle

- **Snapshot cadence** — snapshot on every record mutation, at `prime`/`sync`
  boundaries, or both. Leaning: local snapshot on mutation, remote push during
  explicit sync/prime.
- **Remote sync policy** — ordinary task/research/plan commands should not
  surprise-push. `ahm prime` and `ahm records sync` are allowed to fetch/push
  because their purpose is explicit.
- **Staleness reporting** — define what counts as stale: local ref ahead of
  remote, working files newer than local ref, last successful sync older than a
  threshold, remote ref newer than local, or missing local materialization.
- **Conflict policy** — whole-record conflict reports for prose; limited
  structured merge for safe task front-matter cases; clear manual resolution
  commands for the rest.
- **Non-git fallback** — when the repo is not a git repo (and `ahm` does not
  currently require one), fall back to gitignored local files with explicit
  warnings that durability requires an external backup.
- **Records boundary** — confirm exactly which buckets move onto the ref:
  tasks, scratch research, and draft ExecPlans. ADRs and graduated design docs
  remain committed.
- **Remote support probe** — verify GitHub in a real test repo and provide a
  diagnostic path for Bitbucket Data Center / enterprise hosts that reject
  `refs/ahm/*`.

## Compatibility & Migration

### What Stays The Same

Records remain plain Markdown at the same `.agents/` paths, so existing code
paths for `task create/show/list/complete`, research handling, ExecPlans,
`context`, `index`, `status`, `doctor`, and `task work` can continue reading
and writing those files locally. File formats, front-matter grammar and
canonical order, validation codes, atomic writes, exit codes, and JSON/plain
output shapes should remain stable unless a later task explicitly changes them.

Existing installs should not silently change storage mode. With no new storage
setting, records stay committed and behave exactly as today. The new mode should
be opt-in through a migration command or a future major workflow upgrade that
still previews the branch/index effects.

### Compatibility-Surface Deltas

1. **Git-operation guarantee reworded** — "`ahm` runs no git operations" becomes
   a narrower, documented boundary for ref-backed record mode. This is a
   compatibility-surface change and needs an ADR.
2. **New metadata** — `.agents/ahm.json` likely needs fields such as
   `store_mode`, `records_ref`, `records_remote`, and last-sync metadata.
3. **New CLI surface** — `ahm records push/pull/sync/status/doctor` and
   possibly `ahm prime`.
4. **Context/status output changes** — once records are gitignored, record churn
   stops appearing in `git status --short`. `ahm` should replace that lost
   signal with explicit records sync/staleness status.
5. **`ahm task work` commit handoff changes** — external agents should no longer
   sweep task-file edits into project commits. The task state goes to the
   records ref; source changes still go to normal commits.
6. **Generated indexes become local-only** — generated indexes should not be
   snapshotted to the ref, because they are regenerated from source records.

### Storage Model (precise)

The files stay on disk in the working directory, readable and editable after
`ahm` materializes them. They are removed from the project branch's tracked tree
and hidden with `.gitignore`, so they do not appear in normal project commits,
pull requests, or branch history. They remain recoverable through commits under
the private `refs/ahm/records` ref when the user runs `ahm records sync` or
`ahm prime`.

A fresh clone of the project branch does **not** bring the records. That is now
acceptable: the expected setup path for agents and humans is to run `ahm prime`
or `ahm records pull`, which fetches the private ref and materializes `.agents/`
records locally.

### Migrating An Existing Project

Migration splits into two categories with different mechanics:

- **ahm-owned files** (generated indexes): `ahm` can regenerate these locally
  and should not snapshot them to the records ref.
- **Project-owned records** (task, research, ExecPlan files): these contain user
  content, are not in `meta.Files`, and `ahm` must never delete or rewrite them
  without an explicit migration flow.

`ahm records migrate` (opt-in, dry-run-previewed) would:

1. Pre-flight: require a git repo for ref-backed mode; otherwise offer
   gitignored-only mode with durability warnings.
2. Seed `refs/ahm/records` from the current `.agents/` record tree.
3. Write/merge `.gitignore` entries for task/research/ExecPlan records
   (atomic, append, idempotent).
4. Set storage/sync metadata in `.agents/ahm.json`.
5. Configure or record the remote/refspec strategy for `refs/ahm/*`.
6. Print the one command the user runs to untrack existing committed records,
   such as:

```bash
git rm -r --cached .agents/.tasks .agents/.research .agents/exec-plans
```

`ahm` should not run that command silently because it writes the git index.

This should stay out of routine `ahm upgrade`. Upgrade is template versioning;
on a legacy repo it can print a non-failing advisory that records can be
migrated.

### Sharpest Migration Hazard

After the untrack commit propagates, a fresh clone or a teammate who pulls it
has `.agents/` records absent from the working tree until `ahm prime` or
`ahm records pull` re-materializes them. The files are not lost if the ref was
seeded and pushed, but the migration requires coordination:

- seed the ref,
- push/sync the ref,
- commit the untrack/gitignore change,
- run `ahm prime` after cloning or pulling the migration commit.

This implies `doctor` checks for:

- `store_mode: ref` with missing local records but an available records ref,
- local records not snapshotted to the local ref,
- local ref not pushed to the configured remote,
- remote ref newer than the local ref,
- stale last-sync metadata.

Rollback is safe if migration never deletes content: drop the storage metadata,
remove the `.gitignore` lines, `git add .agents/...`, and optionally delete
`refs/ahm/records`.

## Risks & Open Problems

### Design-Breaking Or Decision-Gating

1. **Remote support is initially GitHub-only.** GitHub custom-ref
   push/fetch/delete behavior is smoke-tested and compatible. Other hosts,
   including hosted Bitbucket Data Center, are future compatibility work rather
   than ADR blockers.

2. **Stale local state is the new failure mode.** Once records are gitignored,
   the project branch no longer advertises them. If `ahm prime` is skipped or
   sync fails, an agent may work from stale backlog state. This is manageable,
   but only if command output and validation make staleness obvious.

3. **Durability depends on push/sync actually happening.** A local private ref
   alone does not protect against laptop loss. The design must make unsynced
   state visible enough that a solo developer notices before the backup gap
   becomes real.

### Serious, Possibly Mitigable

4. **Merge remains mostly unsolved for shared editing.** Solo-first use makes
   this acceptable for an initial design, but the project should not oversell
   semantic merge. Research, ExecPlans, and task bodies are prose; task status
   transitions have cross-task side effects.

5. **Distributed task IDs are deferred, not solved.** Sequential IDs are
   acceptable for solo-first usage. If `ahm` later supports active multi-clone
   or team task creation, random/hash-like stable IDs may be needed.

6. **`.git` bloat has no natural pruning.** Per-mutation snapshot commits are
   reachable from the ref and will not be pruned by normal garbage collection.
   A future compaction command may be needed.

7. **The safety boundary is nuanced.** Ref-backed mode necessarily touches some
   git internals and repo config. The ADR must be precise about what is allowed
   and what remains forbidden.

### Real But Contained

8. **Multiple working copies can clobber each other.** Two worktrees or clones
   can each snapshot different `.agents/` states to the same ref. This is rare
   in the solo-first target but should be detected by non-fast-forward checks.

9. **Discoverability changes.** A non-`ahm` human cloning the repo will not see
   tasks/research/ExecPlans until `ahm prime` runs. This is now acceptable, but
   project docs or `ahm agents suggestions` should make the workflow explicit.

10. **Historical-format compatibility surface.** The ref is a long-lived
    serialization. If `ahm records checkout <old>` or history browsing becomes
    a supported feature, old task/research/ExecPlan formats become a backward
    compatibility concern.

## Implications For This Project

- Lifting "no implicit git operations" for ref-backed records is a
  compatibility-surface change (`ARCHITECTURE.md` System Boundaries;
  workflow-spec Non-goals) and warrants an **ADR** before implementation.
- New `.agents/ahm.json` metadata and a new `ahm records` / `ahm prime` command
  surface touch the CLI and workflow-state compatibility surfaces.
- Generated-index handling changes: indexes become local-only artifacts, never
  part of the shared ref.
- Agent instructions can become simpler: run `ahm prime` before each session,
  then choose work from the returned backlog state.
- Durable outcomes remain committed in ADRs, design docs, and project docs.

## Follow-ups

- Prototype `ahm records sync` in a throwaway branch and verify that the
  snapshot/fetch/push path leaves user branches, index, staging area, and `HEAD`
  untouched.
- Preserve the GitHub smoke-test evidence or repeat it in an automated probe
  before ADR acceptance. The 2026-06-30 manual test passed for push, explicit
  fetch, default clone exclusion, refs API listing, and delete on a probe ref.
- Defer hosted Bitbucket Data Center and other enterprise remote probes until
  those remotes become real requirements.
- Specify `ahm prime`: sync behavior, output shape, stale-state reporting,
  validation behavior, and whether it is allowed to perform network operations
  by default.
- Specify records staleness metadata and command output notes for human use.
- Specify `ahm records migrate` (opt-in, dry-run-previewed, prints the
  user-run `git rm --cached`), the `ahm upgrade` advisory, and the `doctor`
  checks for missing, stale, unsynced, or diverged records.
- Draft an ADR only after the GitHub custom-ref smoke test and the safety
  boundary wording are concrete.
- Defer task ID redesign until there is evidence that active multi-clone or
  small-team task creation needs it; keep a note that random/hash-like IDs are
  the likely direction if that changes.
