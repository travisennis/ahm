# ahm Specification

## Goals

`ahm` manages repo-local agent workflow files. A user can initialize a
repository, create and advance tasks, regenerate indexes, and upgrade workflow
docs when `ahm` ships newer templates.

## Non-goals For v1

- No model or coding-agent calls except explicit `ahm task work <id>`
  delegation to a user-selected external coding-agent CLI.
- No source-code patching.
- No implicit git commits, pushes, PRs, or branch operations. `ahm task work
  <id> --commit` may explicitly ask the delegated external agent to commit
  completed work, but `ahm` does not run git operations itself.
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
[`docs/cli.md`](../cli.md). That reference documents output modes, aliases,
supported task enum values, dry-run behavior, validation finding codes, and
which commands write files.

Exit codes:

- `0`: success.
- `1`: runtime failure.
- `2`: invalid usage.

## Workflow State

Workflow state is repo-local under `.agents/`.

`ahm` writes `.agents/ahm.json` with the installed template version, managed
file hashes, and repository-scoped workflow settings. This metadata lets future
versions update files that have not been locally changed while preserving user
edits.

Example:

```json
{
  "version": "0.1.0",
  "strict_acceptance": true,
  "default_work_agent": "codex",
  "files": {
    ".agents/TASKS.md": "..."
  }
}
```

The optional `strict_acceptance` boolean defaults to `false`. When it is `true`,
`ahm task complete <id>` fails if the task acceptance section is missing, still
contains the seeded `- [ ] TODO` placeholder, or contains unchecked checklist
items. The global `--force` flag overrides this strict completion gate for a
single command while still printing warnings.

The optional `default_work_agent` string selects the agent used by
`ahm task work <id>` when no `--agent` flag is provided. Supported values are
`cake`, `claude`, `codex`, and `cursor`; the command defaults to `cake` when neither the
flag nor metadata setting is present.

`ahm task cancel <id>` requires `--reason <text>`. The reason is trimmed and
must be non-empty; `--force` does not bypass this requirement. Cancellation
stores the reason in the task Markdown body under `## Cancellation Reason`,
updating that section when it already exists and appending it otherwise.
`--dry-run` validates and previews the reason without writing. Cancellation
warns, but does not fail, when acceptance notes still contain the seeded
`- [ ] TODO` placeholder.

When `ahm task complete <id>` completes a task, it also scans active `Blocked`
tasks that directly depend on that completed ID. Dependents whose full
`depends_on` list is now satisfied are moved to `Pending` with an `updated`
timestamp before indexes are regenerated. Dependents with remaining incomplete
dependencies, and blocked tasks that do not depend on the completed task, are
left unchanged. `--dry-run` reports the completion move and dependent unblock
changes without writing task files or indexes.

`ahm task create` allocates task IDs under a repository-local workflow lock.
The lock is held while the command computes the next numeric ID, writes the new
task file, and regenerates indexes, so concurrent creates in the same
repository receive distinct IDs and the final generated indexes include all
created tasks. `--dry-run` does not take the lock because it does not write
workflow state.

## File Ownership Boundary

`ahm` owns the workflow files it installs, maintains, generates, and upgrades.
Consumer projects must not hand-edit ahm-owned files as a substitute for using
`ahm` commands or updating upstream templates.

The ownership categories are:

1. **Generated indexes** (`.agents/.tasks/index.md`,
   `.agents/.research/index.md`, `.agents/exec-plans/active/index.md`,
   `.agents/exec-plans/completed/index.md`, `docs/adr/index.md`) — owned by
   `ahm`. Do not edit by hand. Update source records and run `ahm index`.

2. **Managed template files** (`.agents/TASKS.md`, `.agents/RESEARCH.md`,
   `.agents/PLANS.md`, `.agents/DOCS.md`, `.agents/skills/*/SKILL.md`,
   `docs/adr/README.md`) — owned by `ahm`. Install and upgrade via `ahm init`
   and `ahm upgrade`. Do not customize locally to change ahm-provided process
   guidance; update the canonical templates in the `ahm` repository instead.

3. **Workflow source records** (task files in `.agents/.tasks/`, research
   notes in `.agents/.research/`, ExecPlans in `.agents/exec-plans/`, ADRs
   under `docs/adr/`) — project-owned. Update through their documented
   workflows (e.g., `ahm task create`, `ahm task complete <id>`,
   `ahm adr create`, ADR lifecycle commands, or manual edits to source
   markdown files).

4. **`AGENTS.md`** — project-owned after creation. `ahm init` may create a
   starter `AGENTS.md` when it is missing, but `ahm` never overwrites an
   existing `AGENTS.md` or treats it as a locally modified managed file.
   `ahm agents suggestions` prints advisory snippets for project-owned
   `AGENTS.md` but does not modify the file.

Workflow validation is read-only. `status` and `doctor` report missing or stale
generated indexes, task status and bucket mismatches, broken task dependencies,
completed task acceptance-note drift, task-to-ExecPlan consistency issues,
ExecPlan lifecycle coherence issues, ADR record issues, and broken relative
Markdown links within the managed workflow surface. Project-wide documentation
is not scanned by default; `ahm` validates the workflow files and artifacts it
manages or indexes.

### Validation Scopes

`status` and `doctor` accept a `--check` flag that limits validation to a
specific scope. The default (no `--check`) runs the `workflow` and `links`
validation groups over the managed workflow surface. `project-docs` is opt-in
and never runs as part of the default scope.

Supported scopes:

- `workflow` — managed file consistency, task front matter, dependency cycles,
  task bucket placement, ExecPlan references and lifecycle, ADR records,
  generated index freshness. This is the core workflow validation set.
- `links` — relative Markdown link existence within the managed workflow
  surface. Link validation is independent of workflow state and can be run
  separately to focus on documentation drift.
- `project-docs` — opt-in, read-only health checks over a project's own
  documentation. It discovers common documentation surfaces rather than
  assuming a fixed layout: root-level `README*`, `CONTRIBUTING*`, `CHANGELOG*`,
  `ARCHITECTURE*`, and `DESIGN*` Markdown files, plus every Markdown file under
  `docs/` (covering `docs/adr/`). It reports broken relative Markdown links via
  `project_doc_link_missing`. When a repository already uses the
  `docs/design-docs/` convention (a `docs/design-docs/` directory containing an
  `index.md`), this scope also checks that every design-doc Markdown file is
  represented in the index, emitting `design_doc_unindexed` otherwise. Index
  entries that point at missing files and broken links inside design-doc files
  reuse `project_doc_link_missing` rather than a parallel check. `ahm` never
  creates, rewrites, or formats design-doc indexes. This scope runs only when
  requested explicitly with `--check project-docs`; it is never part of the
  default and never calls models or edits source files.

Scopes compose: `--check workflow --check links` or `--check workflow,links`
runs both the workflow and link validators. Passing an unknown scope value is a
usage error.

```bash
ahm --check workflow status
ahm --check links --json doctor
```

The output format and exit codes are the same regardless of which scopes are
active; only the reported findings change.

ExecPlan lifecycle state is implicit in file placement and Markdown sections.
In-progress plans live under `.agents/exec-plans/active/`; completed plans live
under `.agents/exec-plans/completed/`. Every ExecPlan must maintain
`Progress`, `Surprises & Discoveries`, `Decision Log`, and
`Outcomes & Retrospective` sections. Active plans should not have completed
outcomes, completed plans should have completed outcomes, and completed plans
should not retain open `- [ ]` progress items. Unreferenced ExecPlans are
reported as informational findings.

ADR validation is part of the `workflow` scope. `ahm` reports malformed ADR
records, invalid constrained-MADR statuses, filename/metadata ID mismatches,
duplicate ADR IDs, supersession statuses that point at missing ADRs, and stale
`docs/adr/index.md` content. Legacy bold-metadata ADR files are warning-tier
findings that point at `ahm adr migrate`; they do not make `status` or
`doctor` fail before migration is run.

## File Format

All workflow markdown files are read with CRLF (`\r\n`) line endings normalized
to LF (`\n`) before parsing. Managed files written by `ahm` always use LF line
endings regardless of the original input. This ensures consistent front matter,
title, heading, and body processing across platforms.

### Canonical Front Matter Order

Task front matter is written in a fixed canonical order. This ensures
deterministic output and clean diffs regardless of the order in which fields
appear in the source file. The canonical order, which `renderTask` always
produces, is:

1. `id`
2. `title`
3. `status`
4. `priority`
5. `effort`
6. `labels`
7. `exec_plan`
8. `depends_on`
9. `created` (optional, omitted when empty)
10. `updated` (optional, omitted when empty)
11. `parent` (optional, omitted when empty)
12. `external_ref` (optional, omitted when empty)
13. Extra/unknown fields (sorted by key)

Optional fields (`created`, `updated`, `parent`, `external_ref`) are emitted
only when non-empty. Extra fields not recognized as standard task fields are
emitted in alphabetical order after all standard fields.

### Front Matter Grammar

Task front matter uses a flat `key: value` format. Each line holds one field.
The value is everything after the first colon, trimmed of leading and trailing
whitespace. Double-quoted values have the wrapping quotes stripped.

Supported value forms:

- Simple: `key: value` → `"value"`
- Colon in value: `labels: type:bug, area:tasks` → `"type:bug, area:tasks"`
- Double-quoted: `title: "My Task: The Reckoning"` → `"My Task: The Reckoning"`
- Inline list: `depends_on: 001, 002` or `depends_on: [001, 002]`
- Dash sentinel: `depends_on: -` (empty list, see Dash Sentinel Semantics)

Unsupported forms that produce a parse error:

- Block scalars (`|` and `>`): `description: |\n  multi\n  line`
- Block lists (`- ` prefix): `depends_on:\n  - 001\n  - 002`
- Keys with spaces: `bad key: value`

Comments (`#` at line start) and blank lines within front matter are ignored.

## Dash Sentinel Semantics

Certain optional task front matter fields use the dash (`-`) as a sentinel
value to represent an absent or unset field.

When `ahm` parses a task file, a field that uses `defaultDash` and is missing
from the front matter is read as an empty string and normalized to `-` before
the task struct is used internally. When `ahm` writes the task back to disk, the field is always
written with its current value; if that value is `-` (either because it was
originally absent or because it was explicitly set to `-`), the output is the
same in both cases.

The `defaultDash` normalization is applied to `status`, `priority`, `effort`,
`labels`, and `exec_plan` during parsing. However, `status`, `priority`, and
`effort` also undergo enum validation that rejects `-`; in valid task files
these fields always hold a recognized enum value. The fields where `-` is an
accepted value are:

- `labels` — default `-` indicates no labels have been assigned.
- `exec_plan` — default `-` indicates the task is not linked to an ExecPlan.

Note that `depends_on` uses `-` and `[]` interchangeably for an empty dependency
list; both produce `-` on write (see `docs/cli.md`).

The practical consequence is that a round-trip (parse, modify, write) cannot
distinguish between an absent field and an explicit `-`. This is an accepted
convention: the dash sentinel means "not set" and preserves symmetry with the
grammar used in task creation (where `ahm task create` seeds `exec_plan: -`,
`depends_on: -`).

## Atomic Write Guarantee

All managed writes (metadata, generated indexes, task files, installed/upgraded
templates) use a temporary-file-then-atomic-rename strategy that guarantees
crash safety:

1. Content is written to a unique sibling temp file in the same directory.
2. The temp file is synced to disk (`fsync`).
3. The temp file is atomically renamed to the target path (`os.Rename`, which
   is atomic on Unix when source and destination are on the same filesystem).
4. The parent directory is synced so the rename survives a power loss.

A crash before the rename leaves the original file intact. A crash after the
rename is indistinguishable from a successful write. Stale `.tmp` files left
by a crash are cleaned up opportunistically at the start of `init`, `upgrade`,
and `index` commands.

`ahm task create` also uses a repository-local lock under `.agents/.lock/` to
serialize ID allocation and index regeneration across concurrent invocations.
Other managed write paths rely on atomic rename semantics unless their
read-compute-write behavior needs a narrower lock.

### Generated Index Write Semantics

`ahm index` writes its 8 generated index files sequentially in sorted path
order. There is no cross-file atomicity: if a mid-batch write fails, earlier
files in the batch have already been updated, the failed file remains stale,
and later files are not written. This leaves a temporarily inconsistent index
state that self-heals on the next successful `ahm index` run. The individual
write of each file is still atomic (see Atomic Write Guarantee above); only
the batch as a whole has no rollback or transaction semantics.

The embedded templates are full workflow documents derived from
`agent-workflow-scaffold`, not short summaries. Important managed docs include
`.agents/TASKS.md`, `.agents/PLANS.md`, `.agents/RESEARCH.md`,
`.agents/DOCS.md`,
`.agents/skills/preflight/SKILL.md`,
`.agents/skills/grooming-backlog/SKILL.md`, and `docs/adr/README.md`.
