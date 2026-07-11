# ahm Specification

## Goals

`ahm` manages repo-local agent workflow state. A user can initialize a
repository, create and advance tasks, regenerate indexes, inspect session
context, and upgrade workflow state when `ahm` ships newer templates.

## Non-goals For v1

- No model or coding-agent calls except explicit `ahm task work <id>`
  delegation to a user-selected external coding-agent CLI.
- No source-code patching.
- No implicit git commits, pushes, PRs, or branch operations. Explicit
  ref-backed records commands may read and write `refs/ahm/*` through Git, but
  they must not move `HEAD`, create branch commits, stage files, write the
  project index, or modify project-owned `.agents/` content. `ahm task work
  <id>` may ask the delegated external agent to commit completed work (commit
  runs by default), but `ahm` does not itself create project commits.
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

- `context`: print live repository briefing or managed-work reference.
- `init`: install the managed `.agents` workflow state.
- `upgrade`: update managed workflow state for the embedded template version.
- `status`: report workflow health.
- `doctor`: report environment and workflow checks.
- `index`: regenerate generated indexes.
- `onboard`: print the paste-ready `AGENTS.md` bootstrap snippet.
- `records`: inspect, pull, push, sync, and diagnose ref-backed workflow records.
- `adr`: manage ADR records.
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

Workflow state is repo-local. Legacy committed-record repositories keep
ahm-managed records under `.agents/`. The opt-in ref-backed records migration
introduced by ADR 013 (`ahm records migrate`) moves ahm-managed state to
tool-owned `.ahm/` while leaving project-owned agent content under `.agents/`.

Workflow commands are storage-mode aware. In legacy repositories (metadata
`store_mode` absent or `"committed"`), task, research, ExecPlan, index,
validation, and install behavior is unchanged and uses `.agents/` paths. After
migration, the same commands read and write task records under `.ahm/tasks/`,
research under `.ahm/research/`, and ExecPlans under `.ahm/exec-plans/`, and
generated indexes are regenerated at the same relative paths under `.ahm/`.
Task front matter that still references an ExecPlan by its legacy
`.agents/exec-plans/...` path resolves to the migrated `.ahm/exec-plans/...`
location.

In ref-backed repositories, supported record mutations (`ahm task` lifecycle
and metadata commands, and `ahm index` after hand edits to records) also
refresh the local records ref with a snapshot of the `.ahm/` source records.
Generated indexes are excluded from those snapshots and remain local-only.
Snapshots reuse the previous commit when the record tree is unchanged, never
touch branches, `HEAD`, or the project index, and never contact the network;
pushing and pulling the records ref stays explicit through `ahm records`
commands. A failed snapshot degrades to a warning after the record write
succeeds.

`ahm` writes `.agents/ahm.json` with the installed template version, managed
file hashes for any legacy managed templates, and repository-scoped workflow
settings. This metadata lets future versions remove or migrate files that have
not been locally changed while preserving user edits.

`ahm` also recognizes committed `.ahm/config.json` as the next configuration
home. When `.ahm/config.json` exists, metadata reads prefer it over the legacy
`.agents/ahm.json`; when it does not exist, legacy behavior is unchanged.
Fresh `init` and ordinary `upgrade` still write `.agents/ahm.json` until an
explicit migration creates `.ahm/config.json`.

Example:

```json
{
  "version": "0.1.0",
  "strict_acceptance": true,
  "default_work_agent": "codex",
  "taskWork": {
    "promptFile": ".agents/prompt.md",
    "implementation": {
      "agent": "codex",
      "model": "gpt-5-codex"
    },
    "review": {
      "agent": "claude",
      "model": "sonnet"
    }
  },
  "store_mode": "ref",
  "records_ref": "refs/ahm/records",
  "records_remote": "origin",
  "records_last_sync": "2026-07-06T12:00:00Z",
  "files": {}
}
```

The optional record-storage fields configure the ADR 013 storage mode. Missing
`store_mode` means the current committed-record behavior. The supported
internal storage mode values are `committed`, `local`, and `ref`; `records_ref`
defaults to `refs/ahm/records`, and `records_remote` defaults to `origin` when
omitted.

The optional `strict_acceptance` boolean defaults to `false`. When it is `true`,
`ahm task complete <id>` fails if the task acceptance section is missing, still
contains the seeded `- [ ] TODO` placeholder, or contains unchecked checklist
items. The global `--force` flag overrides this strict completion gate for a
single command while still printing warnings.

The optional `default_work_agent` string selects the agent used by
`ahm task work <id>` when no `--agent` flag is provided. Supported values are
`cake`, `claude`, `codex`, and `cursor`; the command defaults to `cake` when neither the
flag nor metadata setting is present.

The optional `taskWork` block configures how `ahm task work` delegates work to
an external agent. It may contain the following fields:

- **`promptFile`** (string): Path (relative to the repository root) of a
  Markdown file whose content is appended to the built work prompt under a
  `## Project Instructions` heading. Defaults to `.agents/prompt.md`. A missing
  or unreadable file is silently ignored; `ahm` never creates, templates, or
  upgrades this file.

- **`implementation`** (object, optional): Role-specific defaults for the
  implementation work phase. Fields:
  - **`agent`** (string): Agent for this phase (`cake`, `claude`, `codex`,
    or `cursor`).
  - **`model`** (string): Model override for this phase (passed via the
    agent's `--model` flag).

- **`review`** (object, optional): Role-specific defaults for the independent
  review phase. Same fields as `implementation`. When omitted, review uses the
  same agent as `implementation` (after applying the full fallback chain).

Agent/model selection precedence for each phase:

1. `--agent` / `--model` CLI flags (apply to all phases).
2. Role-specific config under `taskWork`.
3. Legacy `default_work_agent`.
4. Built-in default: `"cake"` for agent, no model override.

Feedback-resume and commit handoff always use the implementation agent
because they resume the implementation session.

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
The lock is held while the command computes the next numeric ID (or child ID),
writes the new task file, and regenerates indexes, so concurrent creates in the
same repository receive distinct IDs and the final generated indexes include all
created tasks. `--dry-run` does not take the lock because it does not write
workflow state.

`ahm adr create` similarly serializes ID allocation under the repository-local
workflow lock (`.agents/.lock/adr-create`, or `.ahm/.lock/adr-create` in
ref-backed repositories). The lock is held while ADRs are collected, the
next numeric ID is computed, the new ADR file is written, and indexes are
regenerated. `--dry-run` does not take the lock.

When the `--parent <id>` flag is provided, `ahm task create` allocates the next
available lettered child ID under that parent (`137a`, `137b`, ..., `137z`) and
writes `parent: <id>` in the child task front matter. The parent must be a
top-level task (no letter suffix); child tasks cannot be parents. The allocation
scans parsed tasks and filesystem entries across all three task buckets to avoid
collisions. At most 26 children are allowed per parent. The workflow lock
serializes both top-level and child ID allocation.

## File Ownership Boundary

`ahm` owns the workflow files it installs, maintains, generates, and upgrades.
Consumer projects must not hand-edit ahm-owned generated files as a substitute
for using `ahm` commands.

The ownership categories are:

1. **Generated indexes** (`.agents/.tasks/index.md`,
   `.agents/.research/index.md`, `.agents/exec-plans/active/index.md`,
   `.agents/exec-plans/completed/index.md`, or the same relative paths under
   `.ahm/` after migration, plus `docs/adr/index.md`) — owned by `ahm`. Do
   not edit by hand. Update source records and run `ahm index`.

2. **Managed-work references** — owned by the `ahm` binary and exposed
   through scoped `ahm context task|plan|adr|research|docs`. Fresh `ahm init`
   does not copy reference documents such as `.agents/TASKS.md`,
   `.agents/DOCS.md`, or `docs/adr/README.md` into consumer repositories.
   Scoped commands such as `ahm context task` expose the
   full embedded reference document for that workflow, with record and index
   paths rendered for the repository's active storage mode. `ahm upgrade`
   removes previously managed copies when metadata proves ownership; locally
   modified copies are preserved as conflicts unless `--force` is used.

3. **Obsolete managed instruction/procedure files** — older releases copied
   workflow guides and procedure skills into repositories. `upgrade` removes
   pristine hash-owned copies and reports locally edited copies as conflicts;
   `--force` removes those obsolete copies. Fresh installs create none.

4. **Workflow source records** — task files, research notes, and ExecPlans live
   under `.agents/` in legacy committed-record repositories and under
   tool-owned `.ahm/` after ref-backed migration. Update them through their
   documented workflows (e.g., `ahm task create`, `ahm task complete <id>`, or
   `ahm index` after manual edits). In ref-backed repositories these records
   are working artifacts: they are gitignored locally, snapshotted to
   `refs/ahm/records`, and stay out of normal branch history. ADRs under
   `docs/adr/` remain project-owned durable documentation and use `ahm adr`
   lifecycle commands.

5. **`AGENTS.md`** — project-owned. `ahm init`, `ahm upgrade`, and `--force`
   never create, overwrite, or remove `AGENTS.md`. `ahm onboard` prints a
   paste-ready bootstrap snippet but does not modify the file.

`doctor` reports the informational finding `agents_prime_missing` when a root
`AGENTS.md` exists but does not reference `ahm prime`, and suggests running
`ahm onboard`. Absence of `AGENTS.md` is not a finding.

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
In-progress plans live under the active ExecPlan bucket in the current storage
mode; completed plans live under the completed ExecPlan bucket. Every ExecPlan
must maintain `Progress`, `Surprises & Discoveries`, `Decision Log`, and
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
- Block lists (`-` prefix): `depends_on:\n  - 001\n  - 002`
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

`ahm task create` and `ahm adr create` each use a repository-local lock under
`.agents/.lock/` (or `.ahm/.lock/` in ref-backed repositories) to serialize ID
allocation and index regeneration across concurrent invocations. Other managed
write paths rely on atomic rename semantics unless their read-compute-write
behavior needs a narrower lock.

### Generated Index Write Semantics

`ahm index` writes its 8 generated index files sequentially in sorted path
order. There is no cross-file atomicity: if a mid-batch write fails, earlier
files in the batch have already been updated, the failed file remains stale,
and later files are not written. This leaves a temporarily inconsistent index
state that self-heals on the next successful `ahm index` run. The individual
write of each file is still atomic (see Atomic Write Guarantee above); only
the batch as a whole has no rollback or transaction semantics.

Managed-work references are exposed by scoped
`ahm context task|plan|adr|research|docs` instead of being copied into target
repositories. Scoped reference output renders record and index paths for the
repository's active storage mode. `ahm prime` is the live session briefing with
repository state and active-mode workflow record paths; `--json` and `--plain`
expose the same structured briefing for integrations. Unscoped `ahm context`
is no longer a briefing command.
