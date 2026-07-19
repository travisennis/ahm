---
status: accepted
date: 2026-06-16
decision-makers: Travis Ennis, Codex
consulted: docs/adr/001-atomic-writes-and-concurrency.md
informed: concurrent ahm task create bug report
---
# Task Create ID Allocation Lock

## Context and Problem Statement

ADR 001 accepted atomic writes and deferred broad advisory locking. That was
enough to prevent torn or partially written files, but it did not protect
read-compute-write sequences that must allocate a unique identifier before
writing.

Parallel `ahm task create` invocations exposed this gap. Multiple processes
could read the same existing task set, compute the same next numeric task ID,
and then race to write the same `.agents/.tasks/active/<id>.md` path. Atomic
rename preserved file integrity, but the last writer won and multiple commands
reported the same task ID.

## Decision Drivers

- Task IDs are a workflow compatibility surface and must remain unique.
- `task create` also regenerates task indexes, so the critical section must
  include both ID allocation and index regeneration.
- The fix should not introduce broad locking for unrelated workflow commands
  without evidence that they need it.
- The implementation should remain small and cross-platform.
- `--dry-run` must keep its no-write guarantee and should not take a lock.

## Considered Options

- Add a repository-local lock only around `ahm task create`.
- Add broad repository-local locking around every workflow mutation.
- Retry task creation after detecting that the selected path already exists.
- Switch task IDs to non-sequential unique values.
- Use platform advisory locking such as `flock`.

## Decision Outcome

Chosen option: "Add a repository-local lock only around `ahm task create`",
because the observed race is specific to sequential task ID allocation and the
related generated-index writes.

`ahm task create` holds a lock under `.agents/.lock/task-create` while it:

1. Reads existing task files.
2. Computes the next numeric task ID.
3. Writes the new active task file.
4. Regenerates indexes.

The lock is implemented with atomic directory creation instead of
platform-specific `flock`. If a process crashes while holding the lock, later
commands remove stale lock directories after a conservative timeout.
Reclamation atomically renames the observed stale directory into a unique
quarantine and verifies its filesystem identity before deletion. Release also
verifies the acquired directory's identity and reports lost ownership instead
of treating a missing or replacement directory as a successful release.

This ADR supersedes ADR 001 only for task-create ID allocation. ADR 001 remains
accepted for the general atomic-write strategy and for rejecting broad advisory
locking without specific evidence.

### Consequences

Good, because parallel task creation now allocates distinct IDs.

Good, because the final task indexes include every concurrently created task.

Good, because the lock is narrow and does not serialize unrelated workflow
commands.

Bad, because a crashed process can temporarily block task creation until the
stale-lock timeout expires.

Bad, because other read-compute-write workflow mutations are still not
serialized. They should get their own narrow locks only when a concrete race is
identified.

## More Information

- Partially supersedes [ADR-001](001-atomic-writes-and-concurrency.md).
- Implemented by `internal/ahm/lock.go` and `internal/ahm/task_create.go`.
- Covered by `TestTaskCreateParallelAllocatesUniqueIDs` and
  `TestTaskCreateWaitsForIDAllocationLock`.
