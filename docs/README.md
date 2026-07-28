# Documentation

This directory holds durable project documentation for `ahm`: operational
guides, stable references, guardrails for risky change surfaces, and ADRs.
`AGENTS.md` is the routing layer for coding agents; this file is the docs
index.

## Start Here

- [Vision](VISION.md): where ahm is going — the channel model, ownership
  split, git-safety boundary, and design tests for new work.
- [CLI reference](cli.md): entrypoint for command, flag, output, and validation
  contracts.
- [Workflow specification](references/workflow-spec.md): workflow state, file
  ownership, file formats, and atomic write behavior.
- [Glossary](references/glossary.md): concept definitions mapped to implementing
  types and authoritative docs.
- [Testing guide](guides/testing.md): agent integration smoke checks and golden
  transcript workflow.
- [Workflow upgrade guide](guides/workflow-upgrades.md): upgrade behavior notes.
- [Release process](release.md): publishing binaries, installer scripts, and
  changelog preparation.
- [Guardrails](guardrails/): short agent-facing rules by risk surface.
- [ADRs](adr/index.md): decision record lifecycle and decision history.

## Common Tasks

For topic-based doc routing (which docs to load for CLI changes, workflow
changes, agent orchestration, etc.), see the **Workflow Routing** section in
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
- `adr/`: architecture decision records and the generated ADR index.

Do not hand-edit generated indexes such as `docs/adr/index.md`; update source
records and run the appropriate `ahm` command.
