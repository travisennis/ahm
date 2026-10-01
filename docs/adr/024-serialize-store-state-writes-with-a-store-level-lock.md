---
status: accepted
date: 2026-10-01
decision-makers: Travis Ennis
---
# Serialize store state writes with a store-level lock

## Context and Problem Statement

ADR 023 moved task records into a user-level home store. The store keeps
machine-wide derived state in `<store>/registry.json` — the mapping from a
project key to its directory, plus the remote spellings and paths each project
has been observed at — and per-project state in
`<store>/projects/<dir>/project.json`, which carries the store format version
and the non-decrementing `next_id` task ID counter.

Both files are read-modify-written with no lock. `recordStoreProject` reads the
registry, merges one project's observation, and writes the whole file back; the
same call reads the project state file and writes it back. Two `ahm store path`
runs in different projects can interleave and drop one project's registry
entry, and a dropped `remotes` or `paths` observation is not recoverable: the
key-to-directory mapping is recomputable, but what was observed is not. The
same interleaving reaches the state file, where `task create` raises `next_id`
under the repository-scoped record lock that `store path` never takes, so a
state write carrying a stale counter can lower it.

ADR 023's invariant names the residual hole: both counter writers re-read the
file and keep the higher value, which narrows the window to one read-to-rename
gap but cannot close it, because neither writer holds a lock. Task 267d
recorded that guard as interim and left the decision to the task that serializes
these writes.

## Decision Drivers

- A registry observation that is lost is lost for good; only the directory
  mapping is derivable again.
- `next_id` is the store's only evidence that a deleted top-level task ID was
  spent, so no write may move it down.
- The registry is machine-wide, so a per-project lock cannot serialize it: the
  lock has to sit at the store root, beside the file it protects.
- The lock must not deadlock with the record lock that `task create` and
  `store migrate` already hold while they write store state.
- `--dry-run` writes nothing, not even the store root, and read-only commands
  take no lock.
- Project mode must stay byte-identical.

## Considered Options

- **One store-level lock held across the whole read-modify-write of the
  registry and of a project's state file**, at `<store>/.lock/store-state`.
- **A per-project lock for the state file plus a store-level lock for the
  registry.** Smaller critical sections, two locks, and a second lock ordering
  to reason about, for two files a CLI rewrites in milliseconds.
- **Keep the unlocked writes and rely on the counter's higher-value merge.**
  The merge cannot cover the registry at all, and on the state file it only
  narrows the window.

## Decision Outcome

Chosen option: one store-level lock, because the registry is machine-wide and
the state file is written by the same call, so a single lock at the store root
is both the smaller answer and the only one that covers both files.

`<store>/.lock/store-state` serializes every read-modify-write of
`registry.json` and of a store project's `project.json`:

- `ahm store path`, a home-mode `ahm init`, and `ahm store migrate` hold it
  while they record a project's observation and state.
- `ahm task create`, `ahm init`, and `ahm store migrate` hold it while they
  raise the `next_id` counter.

The lock uses the record lock's protocol: atomic directory creation, a unique
owner token, a heartbeat, and stale reclamation after a conservative timeout,
so a crashed holder is reclaimed the same way.

Lock ordering is one-directional: the record lock is always acquired first, and
no command takes the record lock while holding the store-state lock. `task
create` and `store migrate` hold both, in that order, and so cannot deadlock
with another project's `store path`, which holds only the store-state lock.

The counter's higher-value merge stays as defense in depth. It is no longer the
guard for cooperating `ahm` processes — the lock is — but it still holds the
line against a writer that does not take the lock, such as a pre-lock binary
running against the same store, or a hand edit. Removing it would make the
counter's guarantee depend on every writer cooperating.

The store root's `.lock/` directory is a reserved lock path, like the record
lock's directory beside the records root. It is created only when a store write
happens, so `--dry-run` still creates no store at all, and read-only commands
(`status`, `prime`, `doctor`, and the list and show commands) never take it.

### Consequences

- Good, because two projects recording observations concurrently both keep
  them, and a registry observation is no longer lost to a race.
- Good, because no interleaving of `store path` and `task create` can lower the
  persisted `next_id`.
- Good, because project mode is unchanged: the lock is taken only where a store
  write happens, and no store write happens in project mode.
- Bad, because the store root gains a `.lock/` directory that outlives the run
  that created it, exactly as the record lock's directory does. A user who keeps
  the store in their own Git repository can see it as untracked while a store
  write runs or after a crash; the store root has no managed `.gitignore`,
  because the registry there is deliberately commit-visible.
- Bad, because a store write now needs the store root to be writable even when
  it changes no bytes: `store path` against an up-to-date store used to succeed
  on a read-only store root, and now fails while creating the lock directory.
- Bad, because projects that share a store serialize their store bookkeeping on
  one lock. The critical section is a few small file writes, so the contention
  is not a practical cost.
- Bad, because a crashed store-state holder can block store writes until the
  stale-lock timeout expires.

## More Information

- Partially supersedes [ADR 001](001-atomic-writes-and-concurrency.md), which
  deferred advisory locking, for store state writes. ADR 001 remains accepted
  for the atomic-write strategy and for rejecting broad advisory locking
  without specific evidence.
- Related decisions: [ADR 010](010-task-create-id-allocation-lock.md) owns the
  lock protocol this lock reuses; [ADR 023](023-store-task-records-in-a-user-level-home-store.md)
  owns the store layout.
- Task 269 implements this decision; the residual hole is recorded in the
  Surprises section of `docs/exec-plans/completed/267-home-store-for-task-records.md`.

## References

- `internal/ahm/store.go` — the registry and state writes, and the store-state lock
- `internal/ahm/lock.go` — the lock protocol
- `internal/ahm/task_id_counter.go` — the counter write
