---
status: accepted
date: 2026-09-21
decision-makers: Travis Ennis
---
# Store task records in a user-level home store

## Context and Problem Statement

ADR 015 made task records ordinary committed project files under
`.ahm/tasks/`. That gives durability, cloning, worktree, merge, and recovery
semantics for free, and it makes task churn part of branch history. It also
makes task visibility branch-scoped: a task created on one branch does not
exist on another until it is integrated.

The maintainer of `ahm` uses the tool across many repositories on one machine,
with coding agents as the primary consumers of the backlog. For that use, the
properties ADR 015 optimizes for are the properties that hurt. Task churn in
branch history is noise, reviewing a backlog through pull requests is
ceremony, and branch scoping means the working backlog disappears when the
branch changes. What is wanted is a stable per-project task list that lives
outside the project's Git history — an issue tracker, minus the network, the
account, and the sharing.

ADR 013 considered "move records to an external per-repo store" and rejected
it: discovery becomes harder for agents, it creates a repo-key problem, and it
still needs a separate backup and sync story. Those objections were raised
when `ahm` was expected to preserve backlog state across machine loss and
machine-to-machine work, and in the context of a design that had already made
`ahm` responsible for synchronization. Under the narrower requirement — one
maintainer, one machine, durability explicitly not required — the objections
change shape rather than disappear: discovery must be solved by the tool
rather than by the file path, the repo-key problem is real and needs a
specified key derivation, and the backup story becomes "the user may put the
store in a Git repository if they want one."

ADRs are different. They are durable project documentation, the decision of
record for the project, and they belong in the project's history. Only the
task family moves.

## Decision Drivers

- A task list that is stable across branches, worktrees, and clones of the
  same project, and that does not appear in project commit history.
- `ahm` must remain a records CLI: no synchronization engine, no refs, no
  network, no commits, no staging, no `HEAD` movement.
- Agents must be able to find the records from the project directory with no
  extra configuration and without a discovery convention they cannot guess.
- Existing repositories must keep working unchanged until a person opts in.
- The ownership boundary must stay checkable: every write lands under a known
  owned root.
- The store must survive being placed inside a user-managed Git repository,
  without `ahm` ever running Git against it.

## Considered Options

- **Keep committed `.ahm/tasks/` records (status quo, ADR 015).** Durable and
  simple, but branch-scoped and noisy in history, which is the problem.
- **Ignore `.ahm/tasks/` in the managed `.ahm/.gitignore`.** Removes the noise
  with one line, but the list is lost with the checkout, is invisible to a
  fresh clone, and still has no per-machine project namespace.
- **A user-level home store keyed by project identity.** Records move to
  `~/.ahm/projects/<key>/tasks/`. Discovery requires a derived key, locks and
  indexes move with the records, and there is no branch scoping by default.
- **A global flat store with no per-project namespace.** Simplest to write,
  but every project shares one ID space and one backlog; unusable.
- **An in-project nested Git repository or submodule for tasks.** Keeps tasks
  next to the project and out of its history, but adds a second repository for
  a person to manage and makes agents straddle two checkouts.
- **Place the store under an XDG data directory
  (`$XDG_DATA_HOME/ahm`).** More conventional on Linux, but `XDG_DATA_HOME` is
  unset on macOS, so both spellings resolve to a dot directory in practice,
  and a visible `~/.ahm` is easier to inspect and to place under the user's own
  version control.

## Decision Outcome

Chosen option: **a user-level home store keyed by project identity**, because
it delivers the stable, history-free, branch-independent task list the
maintainer wants, while leaving every responsibility `ahm` deliberately
refuses — synchronization, durability guarantees, sharing — with the user.

The store root is `~/.ahm`, overridable with the `AHM_HOME` environment
variable, which is an absolute path. The store root also holds the
machine-level `config.json` and a `projects/` directory. Each project's
records live at `<store>/projects/<slug>-<hash>/tasks/{active,completed,cancelled}/`,
which mirrors the in-project `.ahm/tasks/` layout exactly so that record and
generated-index paths keep their relative shape.

Project identity is derived per command, not stored in the project:

- With a Git remote, the key is the canonical form of the `origin` remote URL
  (`host/owner/repo`), lowercased, with scheme, userinfo, default port,
  trailing `.git`, and trailing slash removed. Credentials are never hashed or
  persisted.
- With exactly one remote and no `origin`, that remote is used. With several
  remotes and no `origin`, identity falls back to the path rule rather than
  guessing.
- With no remote, or a `file://` or local-path remote, the key is the
  SHA-256 of the symlink-resolved absolute project root. Symlink resolution
  also normalizes platform aliases and on-disk casing.

A registry at `<store>/registry.json` maps each key to its directory name and
records what has been observed: the key kind, remote spellings seen, absolute
paths seen, and creation time. The registry is derived data and is never the
authority for record contents; it is the authority for key-to-directory
mapping so that directory naming rules can change without orphaning data. It
is where a future adoption or migration command learns that a key or a path
changed.

A project's storage mode is recorded in the committed `.ahm/config.json` as
`tasks_location`, with the values `project` and `home`. A missing key means
`project`, so every existing repository is unchanged until a person opts in. A
repository with no `.ahm/config.json` is a new project and defaults to `home`
when it is initialized. Root detection keeps its current rule: a directory is a
managed root when it holds `.git` or `.ahm/config.json`.

ADRs and `docs/adr/index.md` stay committed project files. Generated task
indexes move with the task records into the store. The workflow record lock,
stale-temp cleanup, write containment, and the persisted task ID counter all
live in the store, next to the records they protect. The task ID counter is
persisted and never decremented, because Git history no longer proves that a
deleted ID was once used.

Relationships between records stay inside one project: `depends_on` keeps bare
IDs and there is no cross-project reference syntax and no cross-project task
listing.

Link resolution in task records is order-dependent: a relative Markdown link
is first resolved against the record's own directory, and, if that target does
not exist, against the record's logical in-project directory
(`.ahm/tasks/<bucket>/<id>.md`). This keeps every link written under the
committed-in-project model working after a move, with no new authoring
convention to teach. An ADR that links to a task record cannot be preserved
this way, because the target leaves the repository; that case remains a
warning.

Store paths are displayed to users as `store:` followed by a store-relative
path, so output never embeds absolute machine paths except in the store root
field that `prime` and `status` report. In `project` mode, command output is
byte-identical to the behavior before this decision.

### Consequences

- Good, because the backlog is stable across branches, worktrees, and clones,
  and stops appearing in project history and pull requests.
- Good, because ADRs, project documentation, and configuration remain in the
  project where durable decisions belong.
- Good, because the change is opt-in for existing repositories and invisible
  until a person migrates.
- Good, because `ahm` gains no synchronization responsibility: the store is
  files on the local machine, and placing it in a user-managed Git repository
  is the user's choice, not a supported feature.
- Good, because the store mirrors the in-project layout, so generated indexes
  and intra-record links keep working unchanged.
- Bad, because task records gain no durability guarantee: losing the machine
  loses the backlog unless the user manages a copy.
- Bad, because there is no review, no diff, and no history for task changes,
  and no visibility for anyone else who clones the repository.
- Bad, because identity derived from a remote or a path changes when a
  repository is renamed, moved, or re-pointed, and the task list then appears
  empty until an adoption command exists and is run.
- Bad, because every command now resolves a Git remote, where before only
  `ahm prime` read Git state.
- Bad, because `ahm` must maintain a store format version and its refusal
  path, and a new class of drift finding for records left in the project.
- Bad, because the two-key model is invisible in normal use, so a person
  cannot tell from the repository alone which directory holds its tasks.

### Relationship to ADR 015

This decision partially supersedes ADR 015. ADR 015 remains accepted and
authoritative for the ADR family, for committed `.ahm/config.json`, and for
the rule that generated indexes are derived, ignored, and never a merge
surface. This decision replaces only its statements that task records are
ordinary committed project files and that records are branch-scoped. ADR 015
is not marked superseded, because most of its decision still holds.

## More Information

- Tracker task 267; delivery plan
  `docs/exec-plans/active/267-home-store-for-task-records.md`.
- Related decisions: ADR 001 (atomic writes and concurrency), ADR 013
  (ref-backed storage, superseded), ADR 015 (committed `.ahm` storage),
  ADR 018 (scrubbing inherited Git repository-location environment).
