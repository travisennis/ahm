# Workflow State And File Formats

## Scope

Read this guardrail for `.agents/ahm.json`, task files, research notes,
ExecPlans, ADRs, generated indexes, install/upgrade/context/status/doctor
behavior, embedded instructions, and file-format parsing or rendering.

## Compatibility Surfaces

- Metadata fields in `.agents/ahm.json`.
- Task front matter order, grammar, dash sentinels, and unknown-field
  preservation.
- ADR constrained-MADR front matter and lifecycle metadata.
- Generated task, research, ExecPlan, and ADR index contents.
- Legacy instruction-template removal and upgrade conflict behavior.
- CRLF normalization and LF output.

## Required Checks

- Update `docs/references/workflow-spec.md` when durable workflow semantics or
  file formats change.
- Update `docs/guides/workflow-upgrades.md` when install, upgrade, context,
  template version, or legacy instruction behavior changes.
- For template changes, run `go test ./internal/templates ./internal/ahm`
  before the final verification pass.
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
- `ahm context`
- `docs/adr/README.md`
