---
status: accepted
date: 2026-06-26
---
# Default Task Work Review and Commit On

## Context and Problem Statement

`ahm task work <id>` was almost always invoked as `ahm task work <id> --review --commit`. Requiring the positive flags every time was friction against the actual usage pattern. The opt-in design from ADR 008 created a workflow where the common case required explicit flags, while the uncommon case (skipping review or commit) was the default.

Task 118 implemented this change. This ADR ratifies the decision retroactively to keep the ADR trail complete.

## Decision Drivers

- Real usage: nearly every `ahm task work` invocation passes `--review --commit`.
- Defaults should match common usage, with opt-out for exceptions.
- Agent support: all four supported agents (cake, codex, cursor, claude) support sessions and review, so capability-gating branches were dead code.

## Considered Options

- **Keep opt-in `--review`/`--commit`**: Default-off with positive flags. Rejected because it added friction to every invocation and inverted the actual usage pattern.
- **Default-on, no opt-out**: Run review and commit unconditionally. Rejected because sometimes users want to iterate rapidly or skip the review step.
- **Default-on with `--no-review`/`--no-commit` opt-out**: The chosen option.

## Decision Outcome

Chosen option: review and commit run by default, with `--no-review` and `--no-commit` opt-out flags replacing the removed `--review` and `--commit` flags.

### Consequences

- Good: the common workflow is now the default, reducing flag noise.
- Good: dead capability-gating code (`supportsSessions`, `supportsReview`) was removed, simplifying agent configuration.
- Good: the non-session fallback path was removed since all supported agents support sessions.
- Bad: existing scripts or aliases that explicitly pass `--review --commit` will break (those flags are removed).
- Bad: users who want to skip review or commit must now use a negated flag.

## More Information

- Implements task 118.
- Supersedes [ADR-008](008-delegated-task-work-commit-handoff.md), which introduced the now-replaced opt-in commit design.
- See also task 119, which removed the `--complete` flag (completion is agent-driven via the base work prompt).
