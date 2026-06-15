# Safety And Permissions

## Scope

Read this guardrail for filesystem writes, path handling, root detection,
permission assumptions, command execution, source-code safety boundaries,
atomic writes, and dry-run behavior.

## Compatibility Surfaces

- No implicit source-code patching by `ahm`.
- No implicit git commits, pushes, PRs, or branch operations.
- Root detection from `.git` and `.agents/ahm.json`.
- Atomic write and stale temp-file cleanup behavior.
- Dry-run no-write guarantees.
- Managed versus project-owned file boundaries.

## Required Checks

- Add or update tests for write paths, dry-run paths, and root/path edge cases.
- Re-read ADR 001 before changing atomic write behavior.
- Re-read `docs/spec.md` before changing ownership boundaries or validation
  side effects.
- Run focused tests first, then the verification expected by `CONTRIBUTING.md`.

## Common Failure Modes

- Writing during dry-run through shared helper state.
- Following a path outside the target repository without explicit intent.
- Making validation mutate files.
- Letting `--force` overwrite project-owned `AGENTS.md`.
- Adding command execution that bypasses the explicit `task work` delegation
  boundary.

## Related Docs

- `docs/spec.md`
- `docs/adr/001-atomic-writes-and-concurrency.md`
- `docs/cli.md`
- `ARCHITECTURE.md`
