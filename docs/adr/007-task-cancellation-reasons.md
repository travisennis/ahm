---
status: accepted
date: 2026-06-11
---
# Task Cancellation Reasons

## Context

`ahm task cancel <id>` moves a task to `Cancelled` but previously did not
require or persist the reason the task was abandoned. The workflow guidance
asked operators to note the rationale manually, but the CLI did not enforce
that convention. That made cancelled tasks poor historical records and forced
old decisions to be reconstructed later.

## Decision

`ahm task cancel <id>` requires `--reason <text>`. The reason must be non-empty
after trimming whitespace. The global `--force` flag does not bypass this
requirement.

The cancellation reason is stored in the task body under a Markdown section
named `## Cancellation Reason`. It is not stored in front matter. When the
section already exists, `ahm task cancel` replaces that section's contents with
the supplied reason. Otherwise, it appends the section to the task body.

Cancellation still moves the task to `.agents/.tasks/cancelled/<id>.md` and
regenerates indexes. Dry-run cancellation validates the reason, reports the
target move, and includes the supplied reason in preview output without writing
the task file.

`ahm task cancel` also warns when the task's `## Acceptance Notes` section still
contains the seeded `- [ ] TODO` placeholder. This warning is informational and
does not block cancellation.

## Rationale

- Cancellation rationale is human workflow context, so it belongs in the task
  Markdown body where future readers already look for task history.
- A required flag gives scripts and humans an explicit, reviewable command
  contract without adding prompts or interactive behavior.
- Keeping the reason out of front matter avoids expanding the task metadata
  schema for prose content.
- `--force` should not bypass missing rationale because cancellation is already
  destructive to queue intent; the reason is the minimum audit trail.
- Warning about seeded acceptance TODOs catches the most common incomplete task
  scaffold without making cancellation depend on completion-level acceptance
  checks.

## Consequences

### Positive

- Cancelled tasks record why they were abandoned.
- Automation can rely on a stable non-interactive `--reason` flag.
- Dry-run output shows the exact cancellation reason that would be persisted.

### Negative

- Existing scripts that run `ahm task cancel <id>` must add `--reason`.
- The cancellation reason parser depends on conventional Markdown headings.

## Alternatives Considered

- **Optional reason with warning**: Rejected because the current non-enforced
  guidance already failed to preserve rationale.
- **Store `cancellation_reason` in front matter**: Rejected because reasons are
  prose workflow notes, not queue metadata.
- **Allow `--force` to bypass missing reasons**: Rejected because it would
  preserve the data-loss path this change is meant to close.
- **Prompt interactively for a reason**: Rejected because `ahm` commands should
  remain scriptable and predictable.

## References

- Task 073: Require a reason when cancelling a task
- `.agents/exec-plans/completed/073-task-cancellation-reasons.md`
- `.agents/TASKS.md` - task cancellation workflow
- `internal/ahm/task_status.go` - task cancellation behavior
- `internal/ahm/task_acceptance.go` - acceptance TODO parser
