# External Agent Orchestration

## Scope

Read this guardrail for `ahm task work`, supported agent definitions, argument
builders, output parsers, session capture, review/complete/commit handoff,
golden transcripts, and live agent smoke tests.

## Compatibility Surfaces

- Supported agent names and default-agent metadata.
- External CLI flag shapes and invocation order.
- JSONL or text output parsing for session IDs and review feedback.
- Resume semantics for review, completion, and commit handoff.
- Golden transcript fixture layout and provenance sidecars.

## Required Checks

- Run focused parser and task-work tests for local iteration.
- If argument builders, parsers, or orchestration change, follow
  `docs/testing.md` and run the real agents with `just smoke-agents` when
  available.
- Refresh golden transcripts with `just capture-agent-fixtures` when an agent
  CLI upgrade changes output schema.
- Tasks labeled `area:agent` and `risk:external-service` need live-run evidence
  in Acceptance Notes before completion.

## Common Failure Modes

- Fixtures matching the parser's invented schema instead of the real CLI.
- Capturing no session ID and discovering it only during resume.
- Updating one agent path while leaving review or commit handoff inconsistent.
- Running live agent checks without recording version and transcript evidence.

## Related Docs

- `docs/testing.md`
- `docs/cli.md`
- `internal/ahm/testdata/agents/README.md`
- `docs/adr/006-task-work-agent-delegation.md`
- `docs/adr/008-delegated-task-work-commit-handoff.md`
