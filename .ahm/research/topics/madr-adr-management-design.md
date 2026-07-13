# MADR ADR Management Design (ahm adr)

Status: synthesized
Created: 2026-06-11
Updated: 2026-06-11
Related tasks: 074, 075, 076, 077, 078, 079, 080, 081
Related plans: -
Confidence: high

## Summary

Design synthesis for adding an `ahm adr` command family that manages
Architecture Decision Records the same way `ahm task` manages tasks. MADR 4.x
(constrained to ahm's front matter grammar) is the only supported format.
External source notes:
[.agents/.research/sources/adrs-cli-and-madr-format.md](../sources/adrs-cli-and-madr-format.md).

## Current State

- `ahm` installs `docs/adr/README.md` as a managed template
  (`internal/templates/workflow/adr-README.md`). It documents a custom
  Nygard-style format: `# ADR NNN: Title` heading, `**Status:**` and
  `**Date:**` bold lines, sections Context / Decision / Rationale /
  Consequences / Alternatives Considered / References, statuses Proposed,
  Accepted, Superseded, Superseded in part, Deprecated, three-digit numbering.
- This repository has ADRs 001–008 in that legacy format. No front matter, so
  they are invisible to any front-matter-based tooling.
- There are no `ahm adr` commands, no generated ADR index, and no ADR
  validation. Creation, numbering, status changes, and supersession links are
  all manual.
- Task workflow (`.agents/TASKS.md`, `workflow/adr-README.md`) already requires
  ADRs before implementation for feature/security/breaking work, so ADR
  ergonomics directly affect every consumer repo.

## Target Behavior

An `ahm adr` command family mirroring `ahm task`, inspired by
`joshrotenberg/adrs`:

| Command | Mirrors | Behavior |
| ------- | ------- | -------- |
| `adr create <title>` | `task create` | Allocate next ID, write `NNN-kebab-slug.md` seeded from an embedded MADR template, regenerate index. Flags: `--status`, `--description`/`-d`, `--body-file <path\|->`. |
| `adr list [--status <v>]` | `task list` | Table of ID, title, status, date with output modes. |
| `adr show <id>` | `task show` | Print one record; `<id>` accepts `9`, `009`, or `009-slug`. |
| `adr accept / reject / deprecate <id>` | `task start/complete/...` | Table-driven front matter status transitions. |
| `adr supersede <old-id> --by <new-id>` | (new) | Set old status to `superseded by ADR-NNN`, add a supersession note in the old body, add a reference in the new record. Bidirectional in one command. |
| `adr migrate` | `task migrate` | Convert legacy bold-line metadata into MADR front matter; body sections untouched. |
| `ahm index` | (existing) | Also generates `docs/adr/index.md`. |
| `ahm status` / `doctor` | (existing) | ADR validation findings. |

## Design Decisions (to ratify in ADR 009, task 074)

1. **Directory**: keep `docs/adr/` (not MADR's `docs/decisions/`) for
   continuity with the installed README and existing consumer repos.
2. **Numbering**: keep three-digit `NNN` IDs with kebab-case slug filenames
   (`009-short-title.md`). Numbers are stable and never reused. ID is derived
   from the filename prefix and must agree with any `id`-like metadata.
3. **Front matter profile**: MADR 4.x fields (`status`, `date`,
   `decision-makers`, `consulted`, `informed`) constrained to ahm's scalar-only
   front matter grammar — comma-separated scalars, no YAML block lists or
   block scalars. Unknown fields are preserved on rewrite, like tasks.
4. **Status set**: `proposed`, `accepted`, `rejected`, `deprecated`, plus the
   pattern `superseded by ADR-NNN`. The legacy `Superseded in part` status has
   no MADR equivalent; recommendation: keep status `accepted` and record the
   partial supersession in the body / More Information, with the new ADR
   stating what it replaces.
5. **Ownership boundary**: ADR bodies are user-owned content. `ahm` owns the
   generated `docs/adr/index.md` and the front matter mutations performed by
   `adr` commands (plus the defined supersession note). `docs/adr/README.md`
   stays a managed template.
6. **Migration**: metadata-only. `adr migrate` derives front matter from the
   legacy heading and bold lines and strips the `ADR NNN:` heading prefix; it
   does not rewrite legacy body sections into MADR sections. Decision history
   stays intact.
7. **Resilience**: legacy or malformed ADR files must not break `adr` commands
   or `ahm index` (mirrors task 033); they surface as validation findings
   pointing at `adr migrate`.

## Risks / Notes

- `ahm status` runs on this repo will flag ADRs 001–008 until the migration
  task lands; validation severity for legacy files needs care so self-hosting
  does not break CI mid-rollout.
- Rewriting `workflow/adr-README.md` is a managed-template content change:
  templates version bump, `docs/upgrades.md` entry, and agent-suggestion
  updates (`internal/templates/templates.go` mentions `docs/adr/README.md`).
- The ADR workflow rules embedded in `workflow/TASKS.md` and this repo's
  `AGENTS.md` reference the old template's statuses ("superseded in part") and
  must stay consistent.

## Open Questions (deferred, not blockers)

- Configurable ADR directory in `.agents/ahm.json` for repos that already use
  `docs/decisions/`.
- A task front matter field linking tasks to ADRs (today the link lives in
  task bodies).
- `adr search`, dependency graphs, tags: out of scope for v1.

## Follow-ups

- Tasks 074–081 implement this design; task 074 writes ADR 009 and the feature
  ExecPlan before implementation starts.
