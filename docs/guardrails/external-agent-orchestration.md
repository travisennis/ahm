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
- Environment variables passed to each agent child process. Per-agent
  `blockedEnvVars` strip selected variables (e.g., `ANTHROPIC_API_KEY` for
  `claude`) while preserving the rest of the parent environment.

## Required Checks

- Run focused parser and task-work tests for local iteration.
- If argument builders, parsers, or orchestration change, follow
  `docs/guides/testing.md` and run the real agents with `just smoke-agents` when
  available.
- Refresh golden transcripts with `just capture-agent-fixtures` when an agent
  CLI upgrade changes output schema.
- Tasks labeled `area:agent` and `risk:external-service` need live-run evidence
  in Acceptance Notes before completion.

## Agent-Specific Permission Notes

- `codex` is invoked with `--dangerously-bypass-approvals-and-sandbox` so it can
  edit files, run verification, and complete tasks without interactive approval
  prompts in non-interactive mode.
- `claude` is invoked with `--dangerously-skip-permissions` in print mode (`-p`)
  for the work, review, and resume phases. This is required because Claude Code
  denies all mutating `Bash` and `Edit` tool operations by default when no
  interactive approval prompt is available. The flag must be repeated on every
  invocation; permission mode is per-process and is not inherited by `--resume`.
- `cursor` is invoked with `--trust` so the cursor-agent CLI can operate without
  per-action approval prompts.

`cake` does not require an explicit permission flag; it manages its own
non-interactive policy via the `cake` CLI configuration.

## Common Failure Modes

- Fixtures matching the parser's invented schema instead of the real CLI.
- Capturing no session ID and discovering it only during resume.
- Updating one agent path while leaving review or commit handoff inconsistent.
- Running live agent checks without recording version and transcript evidence.

## Related Docs

- `docs/guides/testing.md`
- `docs/cli.md`
- `internal/ahm/testdata/agents/README.md`
- `docs/adr/006-task-work-agent-delegation.md`
- `docs/adr/008-delegated-task-work-commit-handoff.md`
