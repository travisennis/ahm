---
status: proposed
date: 2026-07-11
decision-makers: Travis Ennis
informed: task 160a
---
# ahm docs check Command and project-docs Deprecation

## Context and Problem Statement

Project documentation drifts in mechanically detectable ways. Observed in this
repository and in a sibling project that follows the same progressive-disclosure
docs structure: broken relative links in the docs index and a guardrail (task
157), enforcement recorded as pending inside a guardrail and then forgotten
(task 158), entry-point creep in `AGENTS.md` (task 159), and machine-specific
`file:///Users/...` links that went unnoticed until an external audit. Prose
guidance did not prevent any of these; a check that runs unconditionally would
have caught each one.

Today the only mechanical project-doc validation is the opt-in `project-docs`
scope on `status` and `doctor` (`--check project-docs`). It checks broken
relative links (`project_doc_link_missing`) and design-doc index coverage
(`design_doc_unindexed`) over root-level docs and `docs/`. The agent routing
layer — root `AGENTS.md`, `CLAUDE.md`, and nested `AGENTS.md` files — is the
most load-bearing documentation surface and is not scanned at all. There is no
front-door command for documentation health, no link-portability check, no
entry-point size budget, and no way to connect code changes to the documents
that describe them.

How should `ahm` expose an expanded set of documentation health checks:
by growing the buried scope flag, or through a dedicated command?

## Decision Drivers

- Division of labor agreed in the 2026-07-02 design discussion: deterministic
  documentation *mechanics* live in `ahm`; documentation *judgment* (auditing
  quality, restructuring, deciding what to write) stays in agent skills;
  *triggering* lives in hooks so enforcement does not depend on an agent
  remembering.
- One validation implementation — no parallel checking subsystem beside the
  existing validation engine.
- Preserve the `project-docs` contract: read-only, never calls models, never
  edits files.
- Existing hook and CI configurations that invoke `--check project-docs` must
  not break in the same release that introduces the replacement.
- Zero configuration must be useful; configuration is optional refinement.
- Finding-code compatibility for existing consumers of validation output.

## Considered Options

- **Extend the `--check project-docs` scope in place.** Add the new checks
  behind the existing flag with no new command.
- **New `ahm docs check` command over the same validators.** Make `docs check`
  the front door, deprecate the scope value as a warning alias.
- **New standalone doc-linting subsystem.** A separate command with its own
  checking engine, independent of workflow validation.
- **Hard-remove `--check project-docs` when introducing the new command.**

## Decision Outcome

Chosen option: **new `ahm docs check` command that runs the expanded
`project-docs` validators through the existing validation engine, with
`--check project-docs` deprecated as a warning alias**, because a buried scope
flag on `status`/`doctor` is not discoverable as the front door for doc health,
while a separate subsystem would duplicate the validation engine, finding
model, and output formats. One implementation serves both entry points.
Hard-removing the scope value in the release that introduces its replacement
would punish existing hook configurations, so the alias keeps working, prints
a deprecation warning naming `ahm docs check`, and its removal is explicitly
deferred to a separate, later decision.

`ahm docs check` complements `ahm context docs`: the `context` command carries
instructions for how to do documentation work; `docs check` verifies whether
the documentation surface is healthy — the same procedure/state split used by
the context-command architecture (ADR 011).

### Command Surface

```
ahm docs check [--diff <range> | --staged] [--strict] [--json|--plain|--text]
```

- Read-only: never calls models, never edits files — the existing
  `project-docs` contract carries over unchanged.
- Exit 0 when clean or warnings-only; exit 1 on error-severity findings.
- `--strict` promotes warnings to errors, for CI use.
- `doc_review_suggested` findings are informational and never affect the exit
  code, including under `--strict`.

### Scanned Surface

The scope's existing surface (root-level `README*`, `CONTRIBUTING*`,
`CHANGELOG*`, `ARCHITECTURE*`, `DESIGN*`, and every Markdown file under
`docs/`) expands to include the root `AGENTS.md`, `CLAUDE.md`, and nested
`AGENTS.md` files.

### Checks and Finding Codes

| Check | Finding code | Severity |
| --- | --- | --- |
| Broken relative links | `project_doc_link_missing` | error (existing) |
| Design-doc index coverage | `design_doc_unindexed` | warning (existing) |
| Non-portable link targets (`file://`, absolute, home-directory) | `project_doc_link_not_portable` | error (new) |
| Entry-point line budget | `entry_point_over_budget` | warning (new) |
| Generalized index coverage | `doc_unindexed` | warning (new) |
| Diff-driven review suggestion | `doc_review_suggested` | info (new) |
| docMap entry points at a nonexistent doc | `doc_map_target_missing` | error (new) |

- `entry_point_over_budget` is a crude total line count on the root
  `AGENTS.md` only; `CLAUDE.md` is skipped when it is a symlink or a bare
  `@AGENTS.md` import. The default budget is 150 lines.
- `doc_unindexed` generalizes the design-docs index check to any `docs/`
  subdirectory containing an `index.md`. `design_doc_unindexed` keeps its
  code for compatibility; whether `doc_unindexed` eventually subsumes it is
  deferred.
- Deliberately excluded: duplicate-content detection (judgment, not
  mechanics — skill territory) and codemap-accuracy checks (drown in false
  positives from illustrative paths; revisit as opt-in info-level at most).

### Configuration

A `projectDocs` block in the repository config file for the current storage
mode — `.agents/ahm.json` in the legacy layout, committed `.ahm/config.json`
after migration to the committed `.ahm` layout (ADR 015). All keys are
optional; with zero configuration the static checks run with defaults.

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

The block lives in `ahm`'s existing config file rather than a separate
`docs/doc-map.json`: one config file, and `ahm` owns its schema.

### Diff Mode

`--staged` (index diff) or `--diff <range>` intersects changed paths with
`docMap` globs and emits `doc_review_suggested` findings naming the docs to
review. The suggestion is suppressed when the mapped doc itself changed in the
same diff. Diff mode is advisory-only by design — a machine knows a document's
subject changed, not that the document is stale. Static checks still run in
diff mode. Git usage is read-only within the existing safety boundary (the
`readGitContext` precedent): no staging, no ref mutation, no commits.

### Consequences

- Good, because the four observed drift classes (broken links, non-portable
  links, entry-point creep, forgotten enforcement via CI wiring) become
  mechanically detectable through one discoverable command.
- Good, because one validation implementation serves `docs check` and the
  deprecated scope alias — no behavior divergence between entry points.
- Good, because the routing layer (`AGENTS.md`, `CLAUDE.md`) is finally under
  validation.
- Good, because existing `--check project-docs` consumers keep working through
  the deprecation window and existing finding codes are unchanged.
- Good, because the read-only, model-free contract is preserved, so the
  command is safe in pre-commit hooks and CI.
- Bad, because two entry points to the same validators must be maintained
  until the alias is removed.
- Bad, because `design_doc_unindexed` and `doc_unindexed` overlap
  conceptually; the compatibility split leaves two codes for one idea until a
  later consolidation decision.
- Bad, because `docMap` is a manually curated mapping that can itself go
  stale; `doc_map_target_missing` catches dead doc targets but not missing
  path coverage.
- Bad, because the entry-point budget is a crude line count and can flag
  legitimately long routing files, mitigated by `entryPointBudget` config.

## More Information

- Full design: `.agents/.research/topics/ahm-docs-check.md` (synthesized
  2026-07-02, agreed with the maintainer).
- Current `project-docs` contract:
  [`docs/references/workflow-spec.md`](../references/workflow-spec.md)
  §Validation Scopes.
- Config file location decision:
  [ADR-015](015-use-committed-ahm-workflow-record-storage.md).
- Adjacent CLI-surface decision (procedure/state split):
  [ADR-011](011-expose-agent-instructions-through-context-command.md).
- Implementation: task tree 160 — 160b (static checks, command, deprecation),
  160c (docMap and diff mode), 160d (hook wiring, guidance, dogfooding). The
  three-tier hook enforcement pattern is recorded in the research note and
  decided with task 160d.
- Alias removal timing is an explicit follow-up, deferred until after 160b
  ships and hook configurations migrate.

