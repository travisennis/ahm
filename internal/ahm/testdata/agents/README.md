# Golden Agent Transcripts

These files are real output captured from the agent CLIs that `ahm task work`
orchestrates. They exist so the output parsers in
`internal/ahm/task_parsers.go` are tested against actual tool output instead
of hand-written fixtures that can encode a parser's own invented schema (the
failure mode behind the cake stream-json regression fixed in task 087).

## Layout

Each `<name>.jsonl` transcript has a `<name>.meta` provenance sidecar
recording the agent version, capture date, and the exact command used
(JSONL cannot carry comments). `task_commands_golden_test.go` consumes the
transcripts in `just ci` and fails — never skips — when one is missing.

- `cake-work.jsonl` — `cake --output-format stream-json` work run.
- `cake-review.jsonl` — `cake --skills preflight` review run.
- `codex-exec.jsonl` — Codex JSONL work run.
- `codex-review.jsonl` — Codex JSONL review run with the preflight prompt.
- `codex-resume.jsonl` — Codex JSONL resume run resuming the
  session captured in `codex-exec.jsonl`; the two are a linked pair.

## Refreshing

```bash
just capture-agent-fixtures
```

The recipe re-runs each agent found on PATH with a trivial prompt and low
token limits, scrubs machine-specific paths and the local username, and
rewrites the goldens and sidecars. Agents not on PATH are skipped.

This makes real LLM calls and costs money: run it manually when an agent CLI
upgrade may have changed its output schema, never in CI. Session IDs and
usage numbers in the transcripts are fine to commit.

The `command:` recorded in each sidecar is the capture command, not the exact
invocation ahm builds: captures add cost-limiting flags (`--max-tokens`,
`model_reasoning_effort`) that ahm never passes. The flags that shape the
output schema — output format, session mode, skills, resume form — match the
arg builders in `internal/ahm/task_agents.go`. The Codex capture recipe also
uses `--dangerously-bypass-approvals-and-sandbox` so future refreshes exercise
the same non-interactive permission posture as `ahm task work`.
