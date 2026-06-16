# Implementation Quality

## Scope

Read this guardrail for internal refactors, parser changes, validation logic,
shared helpers, generated rendering, and performance-sensitive code paths.

## Compatibility Surfaces

- Existing package boundaries and helper APIs.
- Deterministic rendering and sorted output.
- Parser round trips and unknown-field preservation.
- Validation severity, finding codes, and read-only behavior.
- Memory and filesystem behavior on large task queues.

## Required Checks

- Start with the narrowest useful package or test.
- Add focused tests around parser, renderer, validation, or command behavior
  touched by the change.
- Run `just fmt` after Go edits.
- Run `just ci` before handoff when code changed and the environment supports
  it.

## Common Failure Modes

- Adding abstractions for one-off code.
- Mixing argument parsing, mutation, and output formatting in large handlers.
- Breaking deterministic output by iterating over maps unsorted.
- Collapsing parse errors into vague messages.
- Refactoring without updating `ARCHITECTURE.md` when module boundaries move.

## Related Docs

- `ARCHITECTURE.md`
- `CONTRIBUTING.md`
- `docs/references/workflow-spec.md`
- `docs/cli.md`
