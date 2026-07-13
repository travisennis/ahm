# ahm docs check: mechanical project-doc validation and hooks

Status: synthesized
Created: 2026-07-02
Updated: 2026-07-02
Related tasks: 160, 160a, 160b, 160c, 160d
Related plans: -
Confidence: high

## Summary

Design for an `ahm docs check` command: the ergonomic front door for
project documentation health checks, built on the existing `project-docs`
validation scope rather than a new subsystem. Adds link-portability,
entry-point-budget, and generalized index-coverage checks; expands the
scanned surface to agent entry points; deprecates `--check project-docs`;
adds a `projectDocs` config block (including a `docMap`) to
`.agents/ahm.json`; and defines a diff-driven advisory mode plus a
three-tier hook enforcement pattern. Agreed with the maintainer in a design
discussion on 2026-07-02.

## Notes / Evidence

Motivation: documentation drift observed in this repo and in cake despite
both following the progressive-disclosure docs structure, and despite doc
guidance existing in guardrails:

- Broken links in the docs index and documentation guardrail (this repo,
  task 157) — `docs/adr/README.md` does not exist.
- Enforcement recorded as pending and forgotten (this repo, task 158) —
  "markdownlint not yet in `just ci`" written into a guardrail with no
  tracked follow-up.
- Entry-point creep (this repo, task 159) — AGENTS.md at 142 lines with
  product-spec content absorbed into a routing section.
- Machine-specific `file:///Users/...` links in cake's ARCHITECTURE.md,
  unnoticed until an external audit.

All four are mechanically detectable. Prose guidance did not prevent any of
them; a check that runs unconditionally would have caught each one.

Division of labor established in the same discussion: documentation
*judgment* (auditing quality, restructuring, deciding what to write) stays
in agent skills; deterministic *mechanics* live in `ahm`; *triggering*
lives in hooks so it does not depend on an agent remembering.

### Command surface

```
ahm docs check [--diff <range> | --staged] [--strict] [--json|--plain|--text]
```

Read-only, never calls models, never edits files — the existing
`project-docs` contract. Exit 0 when clean or warnings-only; exit 1 on
error-severity findings; `--strict` promotes warnings to errors (for CI).
`doc_review_suggested` (info) never affects the exit code, including under
`--strict`.

`ahm context docs` (instructions: how to do doc work) and `ahm docs check`
(verification: is the doc surface healthy) are complementary channels,
matching the task 156 content-architecture split between procedure and
state.

### Checks and finding codes

Scanned surface: today's roots (`README*`, `CONTRIBUTING*`, `CHANGELOG*`,
`ARCHITECTURE*`, `DESIGN*`) plus `docs/`, extended with root `AGENTS.md`,
`CLAUDE.md`, and nested `AGENTS.md` files — the routing layer is the most
load-bearing doc surface and is currently not scanned at all.

| Check | Finding code | Severity |
| --- | --- | --- |
| Broken relative links | `project_doc_link_missing` | error (exists) |
| Design-doc index coverage | `design_doc_unindexed` | warning (exists; code kept for compatibility) |
| Non-portable links (`file://`, absolute, home-dir) | `project_doc_link_not_portable` | error (new) |
| Entry-point line budget | `entry_point_over_budget` | warning (new) |
| Generalized index coverage (any `docs/` subdir with `index.md`) | `doc_unindexed` | warning (new) |
| Diff-driven review suggestion | `doc_review_suggested` | info (new) |
| docMap entry targets missing doc | `doc_map_target_missing` | error (new) |

Entry-point budget details: crude total line count on the root `AGENTS.md`
only; skip `CLAUDE.md` when it is a symlink or a bare `@AGENTS.md` import.
Default budget 150 (the taxonomy ceiling used by the docs skill), override
via config.

Deliberately excluded: duplicate-content detection (judgment, not
mechanics — skill territory) and codemap-accuracy checks (drowns in false
positives from illustrative paths; revisit as opt-in info-level at most).

### Config

`projectDocs` block in `.agents/ahm.json`; all keys optional, zero config
runs static checks with defaults:

```json
{
  "projectDocs": {
    "entryPointBudget": 150,
    "exclude": ["docs/archive/**"],
    "docMap": [
      { "paths": ["internal/agent/**"], "docs": ["docs/guardrails/external-agent-orchestration.md"] }
    ]
  }
}
```

Decision: reuse `ahm.json` rather than a separate `docs/doc-map.json` —
one config file, and `ahm` owns its schema.

### Diff mode

`--staged` (index diff) or `--diff <range>`: intersect changed paths with
`docMap` globs and emit `doc_review_suggested` naming the docs to review.
Suppress the suggestion when the mapped doc itself changed in the same
diff. Advisory-only by design: a machine knows a doc's subject changed,
not that the doc is stale. Static checks still run in diff mode. Git usage
is read-only within the existing safety boundary (`readGitContext`
precedent).

### Deprecation

`--check project-docs` on `status`/`doctor` keeps working but prints a
deprecation warning naming `ahm docs check`. Removal is a separate, later
decision — hard-removing a scope value in the same release that introduces
its replacement punishes existing hook configs.

### Hook enforcement pattern (three tiers)

1. Session start → `ahm prime` (task 156b; unchanged by this design —
   docs findings stay out of the default briefing).
2. pre-commit → `ahm docs check` (blocking): broken and non-portable
   links never reach a commit.
3. Coding-agent commit path (e.g. Claude Code `PreToolUse` on
   `git commit`) → `ahm docs check --staged` (advisory): review
   suggestions injected at the moment the agent decides what the commit
   contains. Never blocking — blocking on "may need review" trains agents
   to game the map.
4. CI → `ahm docs check --strict` (backstop for changes that bypass
   hooks).

## Implications for this project

- Task tree 160 (160a ADR → 160b static checks + command + deprecation →
  160c docMap + diff mode → 160d hooks + guidance + dogfooding).
- Phase 1 (160a–160b) has no dependency on the 156/143/138 work.
- Phase 2 (160c–160d) touches the same guidance docs 156g rewrites and is
  adjacent to 156f's doctor check (AGENTS.md references `ahm prime`);
  coordinate ordering to avoid churning AGENTS.md and the guardrails twice.
- Task 158 (markdownlint in `just ci`) shares the enforcement surface with
  160d; wire them into CI together.

## Follow-ups

- Decide removal timing for the deprecated `--check project-docs` alias
  (after 160b ships and hook configs migrate).
- Revisit whether `doc_unindexed` should eventually subsume
  `design_doc_unindexed` (finding-code compatibility question for the ADR).
- Consider a docs-skill govern-mode recipe that generates a starter
  `docMap` from an existing docs tree.
