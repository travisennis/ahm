---
status: accepted
date: 2026-05-30
---
# Advisory AGENTS.md Suggestions

## Context

`ahm` installs a starter `AGENTS.md` only when a target repository does not
already have one. Once `AGENTS.md` exists, it is project-owned: `ahm init`,
`ahm upgrade`, and `--force` must not overwrite it or treat it as a managed
workflow file.

The starter file still contains useful guidance for repositories that already
have their own `AGENTS.md`. Existing projects need a way to see that guidance
without weakening the ownership boundary.

## Decision

Expose advisory `AGENTS.md` additions through:

```bash
ahm agents suggestions
```

The command prints Markdown snippets by default and structured suggestion
objects when `--json` is set. It may read the target root's `AGENTS.md` to mark
which exact blocks appear present, and `--all` prints every suggestion.

The command must never write or merge `AGENTS.md`. `ahm` proposes reusable
guidance; the repository's agent or maintainer decides whether and how to adapt
it into the existing project-owned instructions.

Suggestions are scoped to AHM-owned workflow routing and ownership boundaries.
They should not include generic agent operating loops, commit policy, code
style, verification policy, or other project-owned guidance.

The starter `AGENTS.md` content and advisory suggestions share one source of
truth in the template package so the created starter file and suggested blocks
do not drift.

## Rationale

- Preserves the existing create-only contract for `AGENTS.md`.
- Gives existing projects a discoverable path to adopt useful workflow guidance.
- Keeps Markdown merging out of `ahm`, where automatic edits could damage
  project-specific instructions.
- Provides JSON output so agents can inspect suggestions without scraping
  human-oriented Markdown.

## Consequences

### Positive

- Existing repositories can query `ahm` for suggested additions without risking
  changes to project-owned files.
- The ownership boundary remains simple: managed workflow files are written by
  `ahm`; `AGENTS.md` additions are advisory.
- Tests can enforce that the starter `AGENTS.md` and suggestion blocks remain
  synchronized.

### Negative

- Presence detection is intentionally lightweight and only detects exact block
  matches. Adapted or paraphrased guidance may still be suggested.
- Projects still need an agent or maintainer to merge chosen guidance manually.

## Alternatives Considered

- **Patch existing `AGENTS.md` automatically**: Rejected because project
  instructions are high-trust, formatting varies, and automatic Markdown merges
  could conflict with local conventions.
- **Only document the starter template path**: Rejected because it gives agents
  no stable structured surface and encourages scraping embedded files.
- **Manage `AGENTS.md` after initialization**: Rejected because it violates the
  established create-only ownership rule.

## References

- Task 046: Expose suggested AGENTS.md additions
- `internal/ahm/cli.go` and `internal/ahm/agents.go` — `agents suggestions`
  command wiring and implementation
- `internal/templates/templates.go` — suggestion source of truth
- `docs/references/workflow-spec.md` — `AGENTS.md` ownership semantics
