# Documentation

## Scope

Read this guardrail for README, architecture, contributing, CLI docs,
workflow specs, upgrade docs, ADR docs, context guidance, local
task/research/ExecPlan workflow docs, and generated documentation indexes.

## Compatibility Surfaces

- `AGENTS.md` routing authority.
- `ARCHITECTURE.md` repository map and invariants.
- `CONTRIBUTING.md` command catalog, verification, and commit workflow.
- `docs/cli.md` command contract.
- `docs/references/workflow-spec.md` workflow semantics and file formats.
- `ahm context` guidance.
- Local `.agents/*` workflow guides and generated indexes.
- ADR workflow docs and generated `docs/adr/index.md`.

## Ownership

- Agent routing, operating loop: `AGENTS.md`
- Codemap, boundaries, invariants: `ARCHITECTURE.md`
- Contributor setup, commands, verification: `CONTRIBUTING.md`
- Docs navigation: `docs/README.md`
- CLI contracts: `docs/cli.md`
- Workflow semantics: `docs/references/workflow-spec.md`
- Risk-surface rules: `docs/guardrails/`

When two docs cover the same topic, the authority listed above wins. Replace
duplicates with a link.

## Update Triggers

Require a documentation check when a change touches:

- CLI commands, flags, aliases, exit codes, or output modes.
- Workflow formats, `.agents/ahm.json` metadata, or generated index shapes.
- Architecture boundaries, module map, or cross-cutting invariants.
- Setup, build, test, lint, release, or CI commands.
- Public API or user-visible behavior.

## Required Checks

- Run `ahm context docs` before auditing or updating documentation.
- Prefer existing documentation locations and style.
- Do not edit generated indexes by hand.
- Run `just docs-md-lint` before committing markdown changes.
- Run available link or documentation checks. For this repo, use
  `ahm --check project-docs status` for project-doc link health when useful.

## Enforcement

Markdown structure is checked by `markdownlint-cli2` via `just docs-md-lint`.
Configuration lives in `.markdownlint-cli2.jsonc`.

The check runs as part of `just ci` and blocks the pipeline on any
markdownlint error.

## Retiring Docs

When a doc is no longer current:

- If it has durable historical value (e.g. an ADR), mark it superseded or
  archived and link to the replacement.
- If it has no durable value and is not referenced, delete it in the same
  change that removes its links.
- Never leave multiple current docs with conflicting instructions.

## Common Failure Modes

- Duplicating authority instead of linking to the source of truth.
- Updating CLI behavior without updating `docs/cli.md`.
- Updating workflow semantics without updating
  `docs/references/workflow-spec.md` or `docs/guides/workflow-upgrades.md`.
- Leaving module maps stale after moving implementation.
- Putting long one-off procedure details back into `AGENTS.md`.

## Related Docs

- `ahm context docs`
- `README.md`
- `ARCHITECTURE.md`
- `CONTRIBUTING.md`
- `docs/cli.md`
- `docs/README.md`
- `docs/references/workflow-spec.md`
- `docs/references/glossary.md`
- `docs/guides/workflow-upgrades.md`
- `docs/adr/index.md`
