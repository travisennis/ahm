# Safety And Permissions

## Scope

Read this guardrail for filesystem writes, path handling, root detection,
permission assumptions, command execution, source-code safety boundaries,
atomic writes, and dry-run behavior.

## Compatibility Surfaces

- No implicit source-code patching by `ahm`.
- No implicit git commits, pushes, PRs, or branch operations: this is a
  boundary of the `ahm` binary, not a prohibition on the human or agent
  working in the repository, whose commit and branch behavior is governed by
  `AGENTS.md` and `CONTRIBUTING.md`.
- Root detection from `.git` and `.ahm/config.json`, including refusal of the
  retired `.agents/ahm.json` layout.
- Git subprocess isolation from inherited repository-location environment.
- Atomic write and stale temp-file cleanup behavior.
- Dry-run no-write guarantees.
- Managed versus project-owned file boundaries.

## Required Checks

- Add or update tests for write paths, dry-run paths, and root/path edge cases.
- Treat `writeFileAtomic` as an atomicity primitive, not a containment check:
  it requires canonical path spelling, while callers must scope targets to an
  owned repository or workflow directory. Route every workflow record, index,
  and configuration write through `writeOwned`, which refuses a target outside
  an owned root and then writes atomically. A direct `writeFileAtomic` call is
  reserved for the store state that `ahm store path`, `ahm store migrate`, and a
  home-mode `ahm init` write, which they build from a resolved `storePaths`:
  the store's own `registry.json`, which always sits at the store root, and
  `project.json`, which is inside an owned root in `home` mode and outside one
  in `project` mode. The task ID counter in that same `project.json` is written
  by `task create`, `ahm init`, and `ahm store migrate`, which do hold the
  resolved paths, so it goes through `writeOwned`. A direct `os.Remove` is
  reserved for ahm-owned scratch and derived paths, never a path that came from
  user input: paths under a resolved records root built from that layout's own
  accessors (the task scan's record paths, its generated index paths, and the
  records directories a move emptied), the stale temp files `cleanupStaleTemps`
  reaps, the temp file an atomic write removes beside its target when a step
  fails, and the lock protocol's own directories — the quarantine a reclaimed
  lock moves through and the lock a failed acquire rolls back. `store migrate`
  is the only command that removes records on purpose, and a removal is not a
  write, so it has no containment counterpart (see the migration invariant in
  `ARCHITECTURE.md`). A direct `os.WriteFile` is reserved for the lock protocol's
  owner token inside the lock it just created.
- Route ahm-owned Git subprocesses through the shared environment filter; do
  not rely on `git -C` alone when hook-provided `GIT_*` variables may exist.
  The commands ahm runs stay read-only: the remote reads identity resolution
  needs, `prime`'s `git status --short --branch` worktree summary, and the
  `git status --porcelain` that `store migrate --to home` uses to find record
  content that exists only in the working tree. Read-only has to be enforced
  rather than assumed: the shared environment sets `GIT_OPTIONAL_LOCKS=0`,
  because both `git status` reads otherwise claim Git's optional lock to refresh
  its own index stat cache and so rewrite `.git/index`.
- Re-read ADR 001 before changing atomic write behavior.
- Re-read `docs/references/workflow-spec.md` before changing ownership
  boundaries or validation side effects.
- Run focused tests first, then the verification expected by `CONTRIBUTING.md`.

## Common Failure Modes

- Writing during dry-run through shared helper state.
- Following a path outside the target repository without explicit intent.
- Inheriting `GIT_DIR`, `GIT_WORK_TREE`, `GIT_INDEX_FILE`,
  `GIT_OBJECT_DIRECTORY`, or `GIT_COMMON_DIR` in a Git subprocess.
- Making validation mutate files.
- Letting `--force` overwrite project-owned `AGENTS.md`.
- Adding command execution beyond the Git subprocesses `runGit` runs.

## Related Docs

- `docs/references/workflow-spec.md`
- `docs/adr/001-atomic-writes-and-concurrency.md`
- `docs/adr/018-scrub-inherited-git-repository-location-environment.md`
- `docs/cli.md`
- `ARCHITECTURE.md`
