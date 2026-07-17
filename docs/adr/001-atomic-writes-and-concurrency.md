---
status: accepted
date: 2026-05-29
---
# Atomic Writes and Concurrency Protection

## Context

All managed write paths in `ahm` previously used `os.WriteFile` directly,
writing in place with no protection against partial writes from crashes or
interleaved writes from concurrent `ahm` invocations. The affected write sites
are:

- `.agents/ahm.json` metadata (written by `writeMetadata`)
- Installed/upgraded workflow template files (written by `install` paths)
- Seven generated index files (written by `writeIndexes`)
- Task files created, status-moved, and dependency-updated by task commands

A crash mid-write can truncate a file or leave it with partial content,
producing corrupt JSON metadata or stale indexes. Concurrent invocations can
interleave writes to the same file.

This ADR records the write-safety strategy and the decision on advisory
locking.

## Decision

### Atomic writes (accepted)

Introduce a single `writeFileAtomic` helper that writes to a unique sibling
`.tmp` file in the same directory, calls `fsync` on the temp file, atomically
renames it to the target path, and `fsync`s the parent directory on Unix. All
managed write paths are routed through this helper.

The helper guarantees:

- **Crash safety**: A crash before the rename leaves the original file intact.
  A crash after the rename is indistinguishable from a successful write.
- **No partial content**: Readers always see either the old content or the new
  content, never a truncated or mixed write.
- **Stale `.tmp` cleanup**: On write failure, the unique `.tmp` file from that
  attempt is cleaned up. A broader stale-`.tmp` scan runs opportunistically at
  the start of `init`, `upgrade`, and `index` commands to clean up orphaned
  temp files left by a previous crash. The scan only removes `.tmp` files that
  are older than a conservative threshold (currently five minutes) so that
  temp files from an active writer are never reaped.

### Advisory locking (deferred)

After implementing atomic writes, the remaining risk from concurrent
invocations is low:

1. Atomic writes already prevent torn/corrupt reads. Two writers to the same
   file will both succeed; the last rename wins, which is no worse than the
   previous behavior of the last `WriteFile` winning.
2. `ahm` is a CLI tool, not a long-running daemon. Concurrent invocations on
   the same repository are rare in practice.
3. Advisory `flock` is not available on Windows without a substantially more
   complex cross-platform abstraction.

Given these points, advisory locking is **not implemented** in this change.
If evidence of real-world concurrent-corruption problems emerges, locking can
be added later without changing the atomic write core. The `.agents/.lock`
path is reserved for that future use.

## Rationale

- `writeFileAtomic` (tmp + rename + fsync) is a well-understood pattern used
  by tools like `etcd`, `consul`, and the Go standard library's `os.Rename`
  guidance.
- Unique temp file names avoid races where one process's stale-temp cleanup
  could delete another process's in-progress temp file.
- Keeping the helper in a separate file (`internal/ahm/write.go`) avoids
  further bloating `cli.go` and makes the write guarantee easy to audit.
- Routing every managed write through one function makes it trivial to add
  locking later if needed.
- The stale-`.tmp` cleanup is conservative: it only removes files matching
  `*.tmp` that (a) are within the workflow state directories, (b) are older
  than `cleanupStaleTempMaxAge` (five minutes), (c) can be inspected without
  stat errors, and (d) are regular files. The age threshold prevents a cleanup
  scan from deleting a temp file that another `ahm` process is currently
  writing, which would cause that process's atomic rename to fail with
  `ENOENT`.
- Deferring locking avoids Windows-compatibility complexity and keeps the
  change focused on the highest-impact safety improvement.

## Consequences

### Positive

- All managed file writes are now crash-safe. A crash during `index`,
  `task create`, `task complete`, `task cancel`, or `init`/`upgrade` will
  never corrupt a managed file.
- The codebase has a single, testable write primitive that can be audited
  for correctness.
- The `.gitignore` entry for `*.tmp` inside `.agents/` prevents accidental
  tracking of temp files.

### Negative

- Each atomic write involves an extra `fsync` call on the parent directory,
  which is a minor I/O cost on some filesystems. This is negligible for
  a CLI tool writing small files.
- Stale `.tmp` files from pre-atomic-write versions of `ahm` (if any exist)
  will not be cleaned up by the older binary. This is a one-time transition
  concern: after upgrading to the new binary, any `init`, `upgrade`, or
  `index` run will clean them.

## Alternatives Considered

- **Full advisory locking with `flock`**: Rejected for this change because
  the complexity (especially cross-platform) is not justified by the risk.
  Atomic writes remove the corruption risk; locking addresses a much rarer
  concurrent-invocation scenario. Can be added later without breaking the
  atomic write abstraction.

- **Write to a separate temp directory then copy/move**: Rejected in favor
  of sibling `.tmp` files because same-directory rename is guaranteed atomic
  on Unix (same filesystem), and it avoids the need to pick a temp directory
  location that might be on a different filesystem.

- **Use a database (SQLite, etc.)**: Rejected as over-engineered for a small
  set of JSON metadata and Markdown index files.

## More Information

- Superseded in part by [ADR-010](010-task-create-id-allocation-lock.md),
  which adopts a narrow repository-local lock for `ahm task create` ID
  allocation while preserving this ADR's rejection of broad advisory locking.

## References

- Task 008: Add atomic writes and concurrency protection
- `internal/ahm/write.go` — implementation of `writeFileAtomic`
- `internal/ahm/write_test.go` — tests for `writeFileAtomic`
- `.gitignore` — `*.tmp` pattern added for `.agents/` temp files
