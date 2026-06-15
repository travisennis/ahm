# CLI And User Output

## Scope

Read this guardrail for command wiring, flags, aliases, help text, exit codes,
output modes, dry-run text, validation output, and user-visible task or ADR
command behavior.

## Compatibility Surfaces

- Command names, aliases, required args, and flag semantics.
- Exit codes: success, runtime failure, and usage failure.
- Text, JSON, and plain output shapes.
- Validation finding codes and severity behavior.
- Dry-run output and no-write guarantees.

## Required Checks

- Update `docs/cli.md` in the same change unless the behavior is intentionally
  undocumented.
- Search `docs/cli.md` for the affected command or old behavior before handoff.
- Run focused CLI tests first, then the repository verification expected by
  `CONTRIBUTING.md`.

## Common Failure Modes

- Changing text output that scripts or tests depend on.
- Adding a flag in Cobra without documenting precedence or dry-run behavior.
- Returning the wrong exit code for usage errors versus validation failures.
- Updating human text while leaving JSON/plain output stale.

## Related Docs

- `docs/cli.md`
- `docs/spec.md`
- `ARCHITECTURE.md`
- `CONTRIBUTING.md`
