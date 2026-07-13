# adrs CLI and MADR 4 Format

Status: active
Created: 2026-06-11
Updated: 2026-06-11
Related tasks: 074, 075, 076, 077, 078, 079, 080, 081
Related plans: -
Confidence: medium

## Summary

External research backing the planned `ahm adr` feature: the command surface of
`joshrotenberg/adrs` (a Rust ADR manager) and the structure of the MADR 4.x
template, which is the only ADR format `ahm adr` will support.

## Notes / Evidence

### joshrotenberg/adrs (https://github.com/joshrotenberg/adrs)

Command surface:

- `init` — establish a new ADR repository.
- `new` — create a record; `--format` selects Nygard (default) or MADR 4.0.0;
  `--variant` selects full, minimal, or bare template variants; `--supersedes`
  marks decisions that replace earlier ones.
- `edit` — open an existing ADR.
- `list` — display all records; supports `--status` and `--tag` filters.
- `search` — full-text query across titles and content.
- `link` — establish relationships between ADRs with automatic reverse-link
  derivation.
- `status` — update an ADR's state.
- `config` — display current settings.
- `doctor` — validate repository integrity.
- `generate` — produce a table of contents, dependency graphs, or mdbook
  output.
- `export` / `import` — convert to and from JSON-ADR.
- `template` — manage templates.
- `completions`, `cheatsheet` — shell completion and workflow reference.

Other behavior: sequential automatic numbering, ADR-directory auto-discovery
with config-file override, a `--ng` mode that enables YAML front matter and
tags, and an MCP server for agent integration.

### MADR 4.x template (https://github.com/adr/madr, template/adr-template.md)

Optional YAML front matter fields:

- `status:` — `proposed | rejected | accepted | deprecated | … | superseded by
  ADR-0123`
- `date:` — `YYYY-MM-DD` when the decision was last updated
- `decision-makers:` — everyone involved in the decision
- `consulted:` — subject-matter experts (two-way communication)
- `informed:` — people kept up-to-date (one-way communication)

Body sections (those marked optional may be removed):

1. H1 short title representative of the solved problem and found solution.
2. `## Context and Problem Statement`
3. `## Decision Drivers` (optional bullet list)
4. `## Considered Options` (bullet list of option titles)
5. `## Decision Outcome` — `Chosen option: "{title}", because {justification}`
   - `### Consequences` (optional `Good, because … / Bad, because …` bullets)
   - `### Confirmation` (optional; how compliance is confirmed)
6. `## Pros and Cons of the Options` (optional; per-option
   `Good/Neutral/Bad, because …` bullets)
7. `## More Information` (optional; agreements, links to related decisions)

MADR upstream conventions: records live in `docs/decisions/` with four-digit
numbering (`0001-…`), but both are conventions rather than format requirements.

## Implications for this project

- The MADR front matter is scalar-friendly. `ahm`'s front matter grammar
  (`internal/ahm/tasks.go`, `parseFrontMatterLine`) rejects YAML block lists
  and block scalars, so `decision-makers`, `consulted`, and `informed` must be
  comma-separated scalars. This is a deliberate constrained MADR profile.
- `status` is not a closed enum: `superseded by ADR-0123` must be validated as
  a pattern, unlike the fixed task status set.
- The `adrs` features worth mirroring map onto existing `ahm` patterns:
  `new`→`adr create`, `list`/`status`→`adr list`/lifecycle commands,
  `link --supersedes`→`adr supersede`, `generate toc`→the generated
  `docs/adr/index.md`, `doctor`→validation findings in `ahm status`/`doctor`.
- Out of scope for v1 (deliberate): full-text `search` (grep suffices),
  dependency graphs, mdbook output, JSON-ADR export/import, tags, editor
  integration, and an MCP server.

## Follow-ups

- See [.agents/.research/topics/madr-adr-management-design.md](../topics/madr-adr-management-design.md)
  for the synthesized design and task breakdown.
