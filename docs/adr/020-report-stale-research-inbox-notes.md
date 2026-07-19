---
status: accepted
date: 2026-07-19
decision-makers: Travis Ennis, Codex
---
# Report stale research inbox notes

## Context and Problem Statement

Research notes captured in an inbox have no mechanical lifecycle signal. They
can remain untriaged for months while continuing to appear as recent research,
leaving each consumer repository to invent its own cleanup convention. Ahm
needs a read-only, configurable signal that makes old inbox notes visible
without deciding their disposition automatically.

## Decision Drivers

- Make stale untriaged research visible in the normal `status`, `doctor`, and
  `prime` workflow surfaces.
- Preserve the read-only validation boundary: ahm reports, but never moves,
  converts, or deletes a research note.
- Use the same behavior in current `.ahm/` and legacy `.agents/` record layouts
  so older consumers receive the fix when they upgrade the binary.
- Keep the configuration additive and preserve it through metadata round trips
  and `ahm upgrade`.
- Avoid a Git dependency for note age so the check remains fast, deterministic,
  and usable outside a Git worktree.

## Considered Options

- Emit a warning after a configurable number of days, and surface the same age
  in `prime`.
- Emit informational findings instead of warnings.
- Leave inbox aging entirely to project-local guidance.

## Decision Outcome

Chosen option: emit a warning after a configurable number of days and surface
the same age in `prime`, because a warning is visible enough to prompt triage
without turning advisory research maintenance into a failing validation error.

The existing workflow validation scope will inspect Markdown files directly
under the selected record layout's research inbox. The additive finding code is
`research_inbox_stale`. It applies to both `.ahm/research/inbox/` and legacy
`.agents/.research/inbox/` through the existing layout-aware path resolver.

Repository metadata gains an optional `research` object with an
`inboxStaleDays` integer. Absence means the default threshold of 21 days, `0`
disables stale-inbox findings, a positive value replaces the default, and
negative values are rejected as invalid metadata. The
metadata reader and writer preserve this object in both `.ahm/config.json` and
legacy `.agents/ahm.json`, including across `ahm upgrade`.

Age uses the most recent valid ISO date available from conventional research
metadata (`updated`, then `date`, then `created`, accepting either flat YAML
front matter or the documented `Updated:`, `Date:`, and `Created:` header
lines). If no valid date is present, age falls back to the file modification
time. Ahm calculates non-negative elapsed whole days in UTC and considers a
note stale when its age is greater than or equal to the configured threshold.
It does not invoke Git for an alternate timestamp.

`ahm prime` includes age for inbox entries in its Recent Research section and
marks entries stale when the threshold is enabled and reached. Structured
output gains additive age and stale fields for inbox notes. The embedded
`ahm context research` reference names the three normal dispositions: promote
the note to a durable research topic, convert actionable work to a task, or
delete material that has no continuing value.

### Consequences

- Good, because every managed repository receives the same low-cost inbox
  lifecycle signal.
- Good, because warning text and research guidance make the available human
  dispositions explicit while keeping mutation authority with the user or
  agent.
- Good, because existing metadata remains valid and the feature can be disabled
  per repository.
- Bad, because file modification time is an imperfect fallback and can change
  when files are copied or checked out.
- Bad, because warning-tier findings can add noise in repositories that
  intentionally keep long-lived raw notes; those repositories must opt out
  with `inboxStaleDays: 0`.

## More Information

- Task 217: Add stale research-inbox disposition: doctor check and prime
  surfacing.
- `internal/ahm/validation.go` — workflow validation implementation.
- `internal/ahm/prime.go` — session briefing and structured research summary.
- `internal/ahm/install.go` — repository metadata round-trip behavior.
