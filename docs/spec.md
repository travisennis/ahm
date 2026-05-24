# ahm Specification

## Goals

`ahm` manages repo-local agent workflow files. A user can initialize a
repository, create and advance tasks, regenerate indexes, and upgrade workflow
docs when `ahm` ships newer templates.

## Non-goals For v1

- No model or coding-agent calls.
- No source-code patching.
- No implicit git commits, pushes, PRs, or branch operations.
- No database.

## CLI Contract

Usage:

```bash
ahm [global flags] <command> [command flags]
```

Global flags:

- `--root <path>`
- `--json`
- `--plain`
- `--quiet`
- `--verbose`
- `--dry-run`
- `--force`
- `--help`
- `--version`

Commands:

- `init`: install the managed `.agents` workflow.
- `upgrade`: update managed workflow files from embedded templates.
- `status`: report workflow health.
- `doctor`: report environment and workflow checks.
- `index`: regenerate generated indexes.
- `task`: manage tasks and dependencies.

Exit codes:

- `0`: success.
- `1`: runtime failure.
- `2`: invalid usage.

## Workflow State

Workflow state is repo-local under `.agents/`.

`ahm` writes `.agents/ahm.json` with the installed template version and managed
file hashes. This metadata lets future versions update files that have not been
locally changed while preserving user edits.

Generated indexes are owned by `ahm` and should not be edited by hand.

The embedded templates are full workflow documents derived from
`agent-workflow-scaffold`, not short summaries. Important managed docs include
`.agents/TASKS.md`, `.agents/PLANS.md`, `.agents/RESEARCH.md`,
`.agents/skills/deslop/SKILL.md`, and `docs/adr/README.md`.
