# Testing

This document covers verification that goes beyond `just ci`, in particular
the live checks for agent integration. For the standard build and test
commands, see the Build/Test/Run section of `AGENTS.md`.

## Agent Integration Smoke Checklist

`ahm task work` orchestrates external coding-agent CLIs (cake, codex,
cursor). The argument builders and output parsers in
`internal/ahm/task_commands.go` encode assumptions about those CLIs —
flag shapes, JSONL event schemas, session resume semantics — that unit tests
alone cannot validate: hand-written fixtures can round-trip a parser's own
invented schema. That failure mode shipped a broken cake integration in
bf6c1ae (fixed in task 087) because no real agent run happened between
implementation and use.

All agents now use the repository-owned preflight review workflow for
`--review`, so each affected agent must be smoke-tested after changes.

Any change that touches the following must be smoke-tested against the real
binaries before handoff:

- `taskWorkAgent` argument builders (`args`, `resumeArgs`, `reviewArgs`)
- agent output parsers (`parseCakeSessionID`, `parseCakeReviewFeedback`,
  `parseCodexSessionID`, `parseCodexReviewFeedback`, and successors)
- the orchestration flow (`taskWorkWithSession`, `runReview`,
  `runCompletion`, `runCommit`)

For each affected agent that is installed:

1. Run the real agent end-to-end via `just smoke-agents` (preferred), or
   manually via `ahm task work <id> --agent <name> --complete` in a scratch
   ahm-managed repository.
2. Verify stderr contains `session started:` and does not contain
   `no session ID returned by` or `could not capture session ID`.
3. Record the agent version (`cake --version`, `codex --version`,
   `cursor-agent --version`) and a
   short transcript snippet in the task's Acceptance Notes as live-run
   evidence.

Tasks labeled `area:agent` and `risk:external-service` must include this
live-run evidence in their Acceptance Notes before `ahm task complete`, per
the Required Workflow in `AGENTS.md`.

## Live Smoke Test

```bash
just smoke-agents
```

Runs `TestAgentSmoke` (`internal/ahm/task_work_smoke_test.go`) with
`AHM_AGENT_SMOKE=1`. The test drives each installed session-capable agent
through the real `ahm task work --complete` path in a throwaway repository
with a do-nothing task: one work session plus one resume per agent, which
exercises session capture and `resumeArgs` against a real session ID. Agents
not on PATH are skipped per-subtest, and without the environment variable the
test skips entirely, so `just ci` is unaffected.

Cost expectation: a few real LLM calls per installed agent (typically under
a minute total). Run it after any change listed in the checklist above.

## Golden Agent Transcripts

```bash
just capture-agent-fixtures
```

The parser unit tests run hermetically in `just ci` against golden
transcripts captured from the real agent CLIs, committed under
`internal/ahm/testdata/agents/` with version-stamped provenance sidecars.
The `codex-review.jsonl` golden exercises the new preflight-backed review path.
Refresh them with the recipe above when an agent CLI upgrade may have changed
its output schema; this also makes real LLM calls. See
`internal/ahm/testdata/agents/README.md` for details on layout, scrubbing,
and provenance.
