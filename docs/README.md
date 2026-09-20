# Documentation

This directory holds durable project documentation for `ahm`: operational
guides, stable references, guardrails for risky change surfaces, and ADRs.
`AGENTS.md` is the routing layer for coding agents; this file is the docs
index.

## Start Here

- [Vision](VISION.md): where ahm is going — the ownership split, git-safety
  boundary, and design tests for new work.
- [CLI reference](cli.md): entrypoint for command, flag, output, and validation
  contracts.
- [Workflow specification](references/workflow-spec.md): workflow state, file
  ownership, file formats, and atomic write behavior.
- [Glossary](references/glossary.md): concept definitions mapped to implementing
  types and authoritative docs.
- [Workflow procedures](workflow/tasks.md): the project-owned [task](workflow/tasks.md),
  [ADR](workflow/adrs.md), and [ExecPlan](workflow/exec-plans.md) procedures.
- [Design plans](exec-plans/README.md): in-progress and completed plans for
  large or cross-cutting work.
- [Workflow upgrade guide](guides/workflow-upgrades.md): upgrade behavior notes.
- [Release process](release.md): publishing binaries, installer scripts, and
  changelog preparation.
- [Guardrails](guardrails/): short agent-facing rules by risk surface.
- [ADRs](adr/index.md): decision record lifecycle and decision history.

## Common Tasks

For topic-based doc routing (which docs to load for CLI changes, workflow
changes, or agent instructions), see the **Workflow Routing** section in
[`AGENTS.md`](../AGENTS.md).

| Task | Read |
| ---- | ---- |
| Look up a concept, type, or term | [glossary](references/glossary.md) |
| Audit or update documentation | [documentation guardrail](guardrails/documentation.md) |
| Change agent instructions or skills | [agent instructions guardrail](guardrails/agent-instructions.md) |

## Structure

- `guardrails/`: concise, operational rules for risky change surfaces.
- `guides/`: repeatable workflows and procedures.
- `references/`: stable contracts, schemas, formats, and lookup material.
- `workflow/`: project-owned task, ADR, and ExecPlan procedures.
- `exec-plans/`: design plans for large or cross-cutting work.
- `adr/`: architecture decision records and the generated ADR index.

Do not hand-edit generated indexes such as `docs/adr/index.md`; update source
records and run the appropriate `ahm` command.
