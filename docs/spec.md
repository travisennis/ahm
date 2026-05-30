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

The complete command and flag reference is maintained in
[`docs/cli.md`](cli.md). That reference documents output modes, aliases,
supported task enum values, dry-run behavior, validation finding codes, and
which commands write files.

Exit codes:

- `0`: success.
- `1`: runtime failure.
- `2`: invalid usage.

## Workflow State

Workflow state is repo-local under `.agents/`.

`ahm` writes `.agents/ahm.json` with the installed template version and managed
file hashes. This metadata lets future versions update files that have not been
locally changed while preserving user edits.

`AGENTS.md` is an entrypoint file, not a managed workflow document. `ahm init`
may create a starter `AGENTS.md` when it is missing, but `ahm` never overwrites
an existing `AGENTS.md` or treats it as a locally modified managed file.
`ahm agents suggestions` may print advisory snippets for a project-owned
`AGENTS.md`, but it does not modify the file.

Generated task, research, and ExecPlan indexes are owned by `ahm` and should
not be edited by hand.

Workflow validation is read-only. `status` and `doctor` report missing or stale
generated indexes, task status and bucket mismatches, broken task dependencies,
task-to-ExecPlan consistency issues, and broken relative Markdown links within
the managed workflow surface. Project-wide documentation is not scanned by
default; `ahm` validates the workflow files and artifacts it manages or indexes.

## File Format

All workflow markdown files are read with CRLF (`\r\n`) line endings normalized
to LF (`\n`) before parsing. Managed files written by `ahm` always use LF line
endings regardless of the original input. This ensures consistent front matter,
title, heading, and body processing across platforms.

## Atomic Write Guarantee

All managed writes (metadata, generated indexes, task files, installed/upgraded
templates) use a temporary-file-then-atomic-rename strategy that guarantees
crash safety:

1. Content is written to a sibling `<path>.tmp` file in the same directory.
2. The temp file is synced to disk (`fsync`).
3. The temp file is atomically renamed to the target path (`os.Rename`, which
   is atomic on Unix when source and destination are on the same filesystem).
4. The parent directory is synced so the rename survives a power loss.

A crash before the rename leaves the original file intact. A crash after the
rename is indistinguishable from a successful write. Stale `.tmp` files left
by a crash are cleaned up opportunistically at the start of `init`, `upgrade`,
and `index` commands, and are overwritten on the next write to the same path.

Advisory locking has been evaluated but is not implemented (see
`docs/adr/001-atomic-writes-and-concurrency.md` for the rationale).

The embedded templates are full workflow documents derived from
`agent-workflow-scaffold`, not short summaries. Important managed docs include
`.agents/TASKS.md`, `.agents/PLANS.md`, `.agents/RESEARCH.md`,
`.agents/DOCS.md`,
`.agents/skills/deslop/SKILL.md`, and `docs/adr/README.md`.
