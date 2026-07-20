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

| Task | Read |
| ---- | ---- |
| Change CLI commands, flags, output, or dry-run behavior | [CLI guardrail](guardrails/cli-and-user-output.md), [CLI reference](cli.md), [architecture](../ARCHITECTURE.md) |
| Change workflow files, generated indexes, `.agents/ahm.json`, or upgrades | [workflow guardrail](guardrails/workflow-state-and-file-formats.md), [workflow specification](references/workflow-spec.md), [workflow upgrade guide](guides/workflow-upgrades.md) |
| Change external agent delegation or transcript parsing | [external agent guardrail](guardrails/external-agent-orchestration.md), [testing guide](guides/testing.md) |
| Change filesystem writes, root detection, locking, or validation safety | [safety guardrail](guardrails/safety-and-permissions.md), [workflow specification](references/workflow-spec.md), [ADR 001](adr/001-atomic-writes-and-concurrency.md) |
| Change dependencies, build scripts, CI, or release packaging | [build and release guardrail](guardrails/dependencies-build-ci-release.md), [contributing guide](../CONTRIBUTING.md), [release process](release.md), [workflow upgrade guide](guides/workflow-upgrades.md) |
| Refactor implementation boundaries or shared helpers | [implementation guardrail](guardrails/implementation-quality.md), [architecture](../ARCHITECTURE.md) |
| Look up a concept, type, or term | [glossary](references/glossary.md) |
| Audit or update documentation | [documentation guardrail](guardrails/documentation.md), `ahm context docs` |

## Structure

- `guardrails/`: concise, operational rules for risky change surfaces.
- `guides/`: repeatable workflows and procedures.
- `references/`: stable contracts, schemas, formats, and lookup material.
- `adr/`: architecture decision records and the generated ADR index.

Do not hand-edit generated indexes such as `docs/adr/index.md`; update source
records and run the appropriate `ahm` command.
