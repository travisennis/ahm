# Documentation

## Scope

Read this guardrail for README, architecture, contributing, CLI docs,
workflow specs, upgrade docs, ADR docs, task/research/ExecPlan workflow docs,
and generated documentation indexes.

## Compatibility Surfaces

- `AGENTS.md` routing authority.
- `ARCHITECTURE.md` repository map and invariants.
- `CONTRIBUTING.md` command catalog, verification, and commit workflow.
- `docs/cli.md` command contract.
- `docs/references/workflow-spec.md` workflow semantics and file formats.
- `.agents/*` workflow guides and generated indexes.
- `docs/adr/README.md` and generated `docs/adr/index.md`.

## Required Checks

- Read `.agents/DOCS.md` before auditing or updating documentation.
- Prefer existing documentation locations and style.
- Do not edit generated indexes by hand.
- Run available link or documentation checks. For this repo, use
  `ahm --check project-docs status` for project-doc link health when useful.

## Common Failure Modes

- Duplicating authority instead of linking to the source of truth.
- Updating CLI behavior without updating `docs/cli.md`.
- Updating workflow semantics without updating
  `docs/references/workflow-spec.md` or `docs/guides/workflow-upgrades.md`.
- Leaving module maps stale after moving implementation.
- Putting long one-off procedure details back into `AGENTS.md`.

## Related Docs

- `.agents/DOCS.md`
- `README.md`
- `ARCHITECTURE.md`
- `CONTRIBUTING.md`
- `docs/cli.md`
- `docs/README.md`
- `docs/references/workflow-spec.md`
- `docs/references/glossary.md`
- `docs/guides/workflow-upgrades.md`
- `docs/adr/README.md`
