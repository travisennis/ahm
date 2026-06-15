# Workflow State And File Formats

## Scope

Read this guardrail for `.agents/ahm.json`, task files, research notes,
ExecPlans, ADRs, generated indexes, install/upgrade/status/doctor behavior,
embedded templates, and file-format parsing or rendering.

## Compatibility Surfaces

- Metadata fields in `.agents/ahm.json`.
- Task front matter order, grammar, dash sentinels, and unknown-field
  preservation.
- ADR constrained-MADR front matter and lifecycle metadata.
- Generated task, research, ExecPlan, and ADR index contents.
- Managed template ownership and upgrade conflict behavior.
- CRLF normalization and LF output.

## Required Checks

- Update `docs/spec.md` when durable workflow semantics or file formats change.
- Update `docs/upgrades.md` when install, upgrade, template version, or managed
  template behavior changes.
- For template changes, run `go test ./internal/templates ./internal/ahm`
  before the final verification pass.
- Regenerate indexes only through source changes plus `ahm` commands; never
  hand-edit generated indexes.

## Common Failure Modes

- Treating project-owned `AGENTS.md` as an upgradable managed file.
- Forgetting that `ahm task ...` and `ahm adr ...` commands regenerate indexes.
- Breaking round-trip behavior for unknown front matter fields.
- Letting dry-run mutate metadata or task state.
- Changing generated output without deterministic sorting.

## Related Docs

- `docs/spec.md`
- `docs/upgrades.md`
- `.agents/TASKS.md`
- `.agents/PLANS.md`
- `.agents/RESEARCH.md`
- `docs/adr/README.md`
