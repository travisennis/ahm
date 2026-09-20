# Workflow State And File Formats

## Scope

Read this guardrail for `.ahm/config.json`, task files, ADRs, generated
indexes, install/upgrade/status/doctor behavior, and file-format parsing or
rendering.

## Compatibility Surfaces

- Metadata fields in `.ahm/config.json`.
- Task front matter order, grammar, dash sentinels, and unknown-field
  preservation.
- ADR constrained-MADR front matter and lifecycle metadata.
- Generated task and ADR index contents.
- Legacy instruction/procedure-file removal and upgrade conflict behavior.
- CRLF normalization and LF output.

## Required Checks

- Update `docs/references/workflow-spec.md` when durable workflow semantics or
  file formats change.
- Update `docs/guides/workflow-upgrades.md` when install, upgrade, or legacy
  instruction behavior changes.
- Keep `ahm prime` a pure state report: regenerated indexes, validation
  findings, and record counts. Workflow instructions belong to the project's
  own prose under `docs/workflow/`.
- Regenerate indexes only through source changes plus `ahm` commands; never
  hand-edit generated indexes.

## Common Failure Modes

- Treating project-owned `AGENTS.md` as an ahm-created or upgradable file.
- Forgetting that `ahm task ...` and `ahm adr ...` commands regenerate indexes.
- Breaking round-trip behavior for unknown front matter fields.
- Letting dry-run mutate metadata or task state.
- Changing generated output without deterministic sorting.

## Related Docs

- `docs/references/workflow-spec.md`
- `docs/guides/workflow-upgrades.md`
- `docs/workflow/tasks.md`
