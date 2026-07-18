# ahm Commands

This reference covers non-task `ahm` commands. For global flags and output
modes, see [the global CLI contract](global-contract.md). For task lifecycle
commands, see [task commands](task-commands.md).

## Commands

### `audit [flags]`

Delegates a strictly read-only codebase improvement survey to a supported
coding-agent CLI. The prompt includes active task titles and labels for
deduplication, the current label vocabulary, and known validation findings.
It requires self-contained findings and forbids source changes and disclosure
of secret values.

Ahm validates the entire schema-constrained result before writing. It then
creates one standard task per finding through the task creation machinery,
with status `Open` and the `source:audit` provenance label. `Open` is the
acceptance gate; the audit command has no interactive acceptance step.

Agent selection and `--agent`, `--model`, and `--timeout` behavior match
`task groom`. `--dry-run` prints the prompt and schema without delegation or
writes. Invalid output is printed for inspection, creates no tasks, and exits
nonzero. Text, `--plain`, and `--json` summaries share one structured result.

```bash
ahm audit
ahm audit --agent codex
ahm --json audit
ahm --dry-run audit
```

### `help`

Prints built-in help.

Aliases:

- `--help`
- `-h`

Example:

```bash
ahm help
```

### `version`

Prints the ahm binary version. This is the release tag version injected at
build time, distinct from the embedded workflow template version shown in
`status` and `doctor`.

Alias:

- `--version`

Example:

```bash
ahm version
```

### `adr create <title> [flags]`

Creates a new MADR-profile Architecture Decision Record under `docs/adr/` and
regenerates indexes.

The next ID is the next zero-padded numeric ID after the highest existing ADR
filename, such as `001`, `002`, and `003`. The title is built from all non-flag
arguments and becomes both the H1 and the kebab-case filename slug:

```bash
ahm adr create "Choose storage layout"
ahm adr create Choose storage layout --status accepted
```

Command flags:

| Flag | Description |
| ---- | ----------- |
| `--status <value>` | Sets initial ADR status. Default is `proposed`; supported values are `proposed`, `accepted`, `rejected`, and `deprecated`. |
| `--description <text>`, `-d <text>` | Seeds `## Context and Problem Statement`. Default is `TODO.` |
| `--body-file <path>` | Reads the ADR body from a file, or from stdin when the path is `-`. |
| `--decision-makers <text>` | Sets the scalar `decision-makers:` front matter value. |

By default the created ADR has scalar front matter, today's `date:`, and MADR
sections for context, decision drivers, considered options, decision outcome,
consequences, and more information.

`--body-file` provides the full Markdown body that appears after the generated
H1 title. `ahm` still owns ID allocation, front matter, the `# <title>` heading,
the ADR location, and index regeneration; only the body content below the H1 is
taken from the file. The file content is whitespace-trimmed and CRLF line
endings are normalized to LF.

If the body file starts with an `# <title>` line that matches the ADR title, it
is automatically stripped to avoid a duplicate top-level heading. A different
H1 is preserved as intentional body content.

`--body-file` and `--description` are mutually exclusive. The command reports an
explicit error when the body file cannot be read, when stdin is requested but
unavailable, or when the resolved body is empty.

Useful global flags:

- `--dry-run`: prints the target path and ID without creating the ADR.
- `--json` or `--plain`: affects only dry-run output. Successful non-dry-run
  creation prints the ADR ID.

### `adr list`

Lists ADRs parsed from `docs/adr/`, including legacy ADR records that have not
yet been migrated. Malformed ADR files are skipped with a warning so readable
records remain usable.

Text output is sorted by ADR ID:

```text
009 [accepted] 2026-06-14 MADR ADR Management
```

Useful flags:

- `--status <status>`: filters ADRs by one or more statuses. Accepts a
  comma-separated list (`--status proposed,accepted`) or repeated flags
  (`--status proposed --status deprecated`). Matching is case-insensitive.
  A prefix such as `--status superseded` matches statuses like
  `superseded by ADR-009`.
- `--json`: emits ADR list entries with `id`, `title`, `status`, and `date`.
- `--plain`: emits the same entries as compact JSON.

Example:

```bash
ahm adr list
ahm adr list --status accepted
ahm adr list --status superseded
ahm --json adr list
```

### `adr show <id>`

Shows one ADR. The ID accepts the same forms as ADR resolution elsewhere:
`9`, `009`, or `009-madr-adr-management`.

By default, this prints the raw ADR Markdown file. With `--json`, it prints the
parsed ADR record. With `--plain`, it prints the parsed ADR record as compact
JSON. Malformed ADR files are skipped with a warning during resolution.

Example:

```bash
ahm adr show 009
ahm adr show 9
ahm --json adr show 009-madr-adr-management
```

### `adr accept <id>`

Sets a MADR-profile ADR's `status:` to `accepted`, updates `date:` to today's
date, and regenerates indexes. The command rewrites only front matter.

Rerunning on an already-`accepted` ADR is idempotent: it preserves the file,
including its existing `date:`, and reports `NNN already accepted`. Structured
and dry-run output reports `changed: false`. The command refuses ADRs that are
`rejected`, `deprecated`, or `superseded by ADR-NNN`.

Example:

```bash
ahm adr accept 009
```

### `adr reject <id>`

Sets a MADR-profile ADR's `status:` to `rejected`, updates `date:` to today's
date, and regenerates indexes. The command rewrites only front matter.

Rerunning on an already-`rejected` ADR is idempotent: it preserves the file and
its existing `date:`, and reports an unchanged result. The command refuses ADRs
that are `accepted`, `deprecated`, or `superseded by ADR-NNN`.

Example:

```bash
ahm adr reject 009
```

### `adr deprecate <id>`

Sets a MADR-profile ADR's `status:` to `deprecated`, updates `date:` to today's
date, and regenerates indexes. The command rewrites only front matter.

Rerunning on an already-`deprecated` ADR is idempotent: it preserves the file
and its existing `date:`, and reports an unchanged result. The command refuses
ADRs that are `proposed`, `rejected`, or `superseded by ADR-NNN`.

Example:

```bash
ahm adr deprecate 009
```

### `adr propose <id>`

Returns an `accepted` MADR-profile ADR to `proposed` status, updates `date:` to
today's date, and regenerates indexes. The command rewrites only front matter.

This is a correction command for ADRs that were prematurely accepted before
review was complete. It is not a general undo: `rejected`, `deprecated`, and
`superseded by ADR-NNN` ADRs are terminal and refused.

Rerunning on an already-`proposed` ADR is idempotent: it preserves the file and
its existing `date:`, and reports an unchanged result.

Example:

```bash
ahm adr propose 009
```

### `adr supersede <old-id> --by <new-id>`

Marks one MADR-profile ADR as superseded by another and writes the reciprocal
body references in one command.

The old ADR gets:

- `status: superseded by ADR-NNN`
- `date:` updated to today's date
- a `## Supersession` note linking to the replacement ADR

The replacement ADR gets a `## More Information` reference back to the
superseded ADR. Rerunning the same command replaces the managed notes instead
of duplicating them. The command rejects unknown IDs, self-supersession, and
attempts to point an already-superseded ADR at a different replacement. Only
`accepted` ADRs can be superseded; `proposed`, `rejected`, and `deprecated`
ADRs are refused. The replacement ADR must also be `accepted`.

Example:

```bash
ahm adr supersede 009 --by 010
```

### `adr migrate`

Converts legacy ADR records (H1 + bold `**Status:**` / `**Date:**` metadata) to
the constrained MADR front matter profile that `ahm` commands require. The
conversion is metadata-only: body sections such as Context, Decision, and
Rationale are never rewritten.

The command finds all ADR files under `docs/adr/` and converts each one that
still uses the legacy format. Already-migrated files (those with YAML front
matter) are skipped, so rerunning is safe and produces no changes.

Legacy status mapping:

| Legacy status | MADR status |
| ------------- | ----------- |
| `Proposed` | `proposed` |
| `Accepted` | `accepted` |
| `Deprecated` | `deprecated` |
| `Accepted, superseded in part by ADR NNN` | `accepted` + `## Supersession` body note |
| `Superseded` | `superseded by ADR-NNN` (resolved from body) |

If a supersession replacement cannot be resolved unambiguously, the file is
reported and requires a manual fix. If a `## Supersession` or similar heading
already exists in the body, the migration preserves it rather than adding a
redundant note.

Useful flags:

- `--dry-run`: preview which files would change without modifying them.
- `--json` or `--plain`: structured migration report for scripting.

Examples:

```bash
ahm adr migrate --dry-run
ahm adr migrate
ahm --json adr migrate --dry-run
```

### `onboard`

Prints the minimal bootstrap and safety snippet maintainers can paste into the
project-owned `AGENTS.md`. The snippet requires `ahm prime` before work and
after context compaction, and directs record state changes and generated-index
updates through ahm. It deliberately omits project operating loops and workflow
routing. The command never reads or writes `AGENTS.md`.

Text mode adds brief paste/import framing. `--plain` prints the bare snippet;
`--json` returns an object with a `snippet` field.

```bash
ahm onboard
ahm --plain onboard
ahm --json onboard
```

### `records migrate`

Migrates ahm-managed workflow state from `.agents/` into tool-owned `.ahm/`,
keeping migrated source records under normal Git tracking. Migration is
explicit and opt-in; it is not part of `ahm upgrade`.

Migration:

- Moves every file under `.agents/.tasks/`, `.agents/.research/`, and
  `.agents/exec-plans/` (including generated indexes) to the same relative
  paths under `.ahm/`, then removes the emptied legacy directories.
- Installs or updates `.ahm/.gitignore` with entries that ignore only
  generated workflow indexes and machine-local state. Source records remain
  committed.
- Writes committed `.ahm/config.json` preserving all metadata fields, and
  removes legacy `.agents/ahm.json`.
- Prints a `git add` / `git rm --cached` instruction covering the legacy
  record paths that are still tracked in the project git index. The user runs
  that command and commits the result together with the new `.ahm/` state;
  `ahm` never stages, commits, or moves `HEAD`.

Migration never touches project-owned `.agents/` content such as
`.agents/prompt.md`, `AGENTS.md`, or `.agents/skills/`, and it does not move
`HEAD`, create branch commits, stage files, write the project index, or
create or update any `refs/ahm/*` ref.

The former preflight, grooming-backlog, and finding-improvements skill files
remain in place. Migration removes their old entries from managed-file
metadata, after which ahm never inspects, reports, overwrites, or removes them.

After migration, workflow commands operate on the `.ahm/` paths: task,
research, ExecPlan, index, validation, and install behavior reads and writes
source records and generated indexes under `.ahm/`. Source records are
ordinary committed project files backed by normal Git tracking, durability,
merging, clone, and worktree behavior. Generated indexes are local-only and
rebuilt on demand by `ahm prime` and `ahm index`.

The command is idempotent and resumable. Re-running after an interrupted
migration completes the remaining steps: targets that already hold identical
content are treated as moved, while a target with different content fails with
a conflict diagnostic instead of being overwritten. A fully migrated
repository reports `records storage is already migrated` plus any remaining
git-index cleanup.

To roll back a migration:

1. Before committing: run `git restore .` to restore legacy paths, then
   delete the new `.ahm/` directory. Note that migration carries any
   uncommitted record edits into `.ahm/`; this rollback restores the last
   committed record content, discarding those edits, so copy them out of
   `.ahm/` first if you need them. For the same reason, avoid `git clean -fd`
   between migrating and committing — the moved records are untracked until
   the printed git cleanup command runs.
2. After committing: run `git revert <migration commit>` to restore the
   legacy layout.

Useful flags:

- `--dry-run`: previews the full plan — file moves, gitignore and config
  changes, legacy config removal, and the user-run git commands — without
  writing files or metadata.
- `--json`: prints indented JSON.
- `--plain`: prints compact JSON.

Example:

```bash
ahm --dry-run records migrate
ahm records migrate
```

### `records doctor`

Reports diagnostics for records migration state. The migration check
reports leftover legacy record files or config under `.agents/`, legacy
dot-prefixed record paths under `.ahm/`, and legacy record paths still tracked
in the project git index, pointing at `ahm records migrate` or the required
Git cleanup commands.

Text output includes `ok: true|false` and a `checks:` section. With `--json` or
`--plain`, the same information is emitted as structured JSON. The checks
describe migration state only; they do not emit a `mode` field because both
supported layouts use ordinary committed source records. Removing the former
`checks.mode` key is a deliberate structured-output compatibility change.

Example:

```bash
ahm records doctor
ahm --json records doctor
```

### `prime`

Prints a session briefing with repository state, task backlog, and managed-work
routing. This is the canonical session-start command for coding agents.

`prime` regenerates indexes, validates workflow state, and prints the briefing.

The briefing includes (in order, omitting empty sections):

- Dirty-worktree warning when the working tree has uncommitted changes.
- Repository root, installed workflow version, and validation status.
- `## In Progress` — full task lines for all in-progress tasks.
- `## Ready` — ready task lines capped at 5, with an overflow pointer to
  `ahm task ready` for the remainder.
- Blocked and open task counts, with commands to expand each.
- Active ExecPlans and the five most recent research notes, sorted globally
  across buckets by descending filename with a deterministic path tie-breaker.
- `## Managed Work Intake` — the routing table for managed-work types,
  layout-specific workflow record paths, and notes on workflow and multi-step
  plans.
- `## Useful Commands` — common follow-up commands.

Supports `--json`, `--plain`, and `--text` output.

Useful flags:

- `--json`: prints structured output with `root`, `workflow`, `git`,
  `tasks` (with `in_progress`, `ready`, `ready_total`, `blocked`, `open`),
  `plans` (active ExecPlan summaries), `research` (recent research notes),
  and `commands`.
- `--plain`: prints compact JSON.

Examples:

```bash
ahm prime
ahm --json prime
ahm --plain prime
```

### `context`

Prints a managed-work reference for one scope.

Unscoped `ahm context` is no longer valid as a session briefing. The session
briefing has moved to `ahm prime`.

With a scope, `context` prints the full embedded reference document for that
artifact type. For example, `ahm context task` prints the task workflow
reference for creating, choosing, updating, or working on tasks. Scoped output
is pure reference text with no live briefing wrapper. References that name
workflow record, generated index, or metadata paths render those paths for the
repository's legacy or post-migration layout.

Required scope:

- `task`
- `adr`
- `research`
- `plan`
- `docs`

The command is read-only. It does not run `ahm index`, start tasks, mutate
workflow files, invoke external agents, or run mutating git commands.

Useful flags:

- `--json`: prints structured output with `scope`, `instructions`, and
  `commands`.
- `--plain`: prints compact JSON.

Examples:

```bash
ahm context task
ahm --json context adr
```

### `docs check [--strict]`

Runs read-only structural checks over the project documentation surface.

Reports:

- Broken relative Markdown links (`project_doc_link_missing`).
- Non-portable link targets including `file://` URIs, absolute filesystem
  paths, and home-directory paths (`project_doc_link_not_portable`).
- Entry-point line budget overages on root `AGENTS.md`
  (`entry_point_over_budget`).
- Generalized doc index coverage in any `docs/` subdirectory that contains
  an `index.md` (`doc_unindexed`).
- Design-doc index coverage (`design_doc_unindexed`, for compatibility).

Exit 0 when clean or warnings-only; exit 1 on error-severity findings.
`--strict` promotes warnings to errors for CI use. The command never calls
models and never edits files.

The scanned surface includes root-level common documentation markers
(`AGENTS.md`, `README*`, `CONTRIBUTING*`, `CHANGELOG*`, `ARCHITECTURE*`,
`DESIGN*`), `CLAUDE.md`, nested `AGENTS.md` files, and every Markdown file
under `docs/`.

`projectDocs` configuration is read from the repository config file
(`.agents/ahm.json` or committed `.ahm/config.json`):

- `entryPointBudget` (default 150): line-count budget for root `AGENTS.md`.
- `exclude`: globs that exclude paths from the scanned surface.
- `docMap`: path globs → docs to review (used by diff mode).

All keys are optional; zero configuration runs static checks with defaults.

Useful flags:

- `--strict`: promotes warnings to errors, for CI use.
- `--json` or `--plain`: structured output.

Examples:

```bash
ahm docs check
ahm docs check --strict
ahm --json docs check
ahm --plain docs check
```

The deprecated `--check project-docs` scope on `status` and `doctor` still
functions but prints a deprecation warning naming `ahm docs check`.

### `init`

Installs the managed `.ahm` workflow into the target root.

`init` creates missing workflow directories, metadata, and generated indexes.
On fresh installs (no prior workflow metadata), creates the committed `.ahm/`
layout directly: `.ahm/config.json`, workflow directories under `.ahm/`,
and generated indexes. Canonical agent instructions are exposed through
`ahm context`, not copied into consumer repositories as README files.
`init` does not create or overwrite `AGENTS.md`.

On repositories with existing `.agents/ahm.json` metadata, `init` preserves
the existing layout. After a repository has migrated (`ahm records migrate`),
`init` and `upgrade` create missing record directories and generated indexes
under `.ahm/` and read or write committed `.ahm/config.json`.

Writes in .ahm layout:

- `.ahm/config.json` metadata.
- Generated index files under `.ahm/tasks/`, `.ahm/research/`,
  `.ahm/exec-plans/`, and `docs/adr/index.md`.
- Workflow directories under `.ahm/` and `docs/adr/`.

Useful flags:

- `--dry-run`: prints files, directories, metadata, and indexes that would be
  written.
- `--force`: adopts supported existing managed files where applicable.
- `--json` or `--plain`: changes the emitted install summary format.

The JSON and plain summaries always include the array-valued keys `adopted`,
`created`, `updated`, `removed`, `skipped`, `conflicts`, `metadata`, and
`indexes`. Keys with no entries are emitted as empty arrays. Dry-run summaries
also include a `directories` array.

Example:

```bash
ahm init
ahm --dry-run init
```

### `upgrade`

Updates managed workflow state for the embedded template version.

`upgrade` compares the installed workflow metadata (`.ahm/config.json`
or legacy `.agents/ahm.json`) with managed files in the target root. Previously managed workflow instruction files that still match their
recorded managed hash are removed because canonical guidance and procedures now
come from scoped contexts, delegation commands, and the embedded task-work
review. Locally modified obsolete instruction files are preserved and reported
as conflicts unless `--force` is used. The former preflight, grooming-backlog,
and finding-improvements skill files are left in place and removed from
managed-file metadata; ahm no longer manages them. Generated indexes are
regenerated.

The metadata `version` field always advances to the embedded template version,
even when conflicts exist. This means a partial upgrade (some files conflicted,
others updated) records the new template version so subsequent upgrades can
correctly identify already-updated files.

`AGENTS.md` remains project-owned and is never overwritten or removed.

Useful flags:

- `--dry-run`: previews all supported writes.
- `--force`: removes locally modified former instruction files. This does not
  apply to `AGENTS.md` or the three project-owned procedure skill files.
- `--json` or `--plain`: changes the emitted upgrade summary format.

The JSON and plain summaries use the same stable array-valued key set as
`init`, including empty arrays for groups with no entries. Dry-run summaries
also include a `directories` array.

Example:

```bash
ahm upgrade
ahm --force upgrade
```

### `status`

Reports workflow health for the target root. This is the default command when
no command is provided.

The report includes:

- Target root.
- Current embedded template version.
- Whether workflow metadata is installed.
- Installed workflow version from `.ahm/config.json` when present, otherwise
  `.agents/ahm.json`.
- Task counts by status.
- Validation errors, warnings, and info findings for managed workflow files,
  task consistency, ADR records, generated indexes, ExecPlan references and
  lifecycle checks, and scoped Markdown links.

Validation checks include missing metadata, missing managed files, unreadable
managed files, untracked managed files, locally modified managed files,
malformed task front matter, task bucket mismatches, missing task dependencies,
active dependency cycles, ADR record issues, stale or missing generated indexes,
task ExecPlan reference issues, ExecPlan lifecycle coherence, and broken
relative Markdown links inside the managed workflow surface.

When the validation report contains any error (not warnings or info findings),
`status` exits with code 1. No `error:` prefix is printed to stderr; the JSON or
text output on stdout already describes the findings.

Useful flags:

- `--check <scope>`: limit validation to the specified scope. Repeatable or
  comma-separated. Valid scopes: `workflow`, `links`, `project-docs`.
  Without `--check`, the default validation runs `workflow` and `links` checks
  over the managed workflow surface. `project-docs` is opt-in: it runs only
  when requested explicitly with `--check project-docs` and never as part of
  the default. Unknown scope values produce a usage error.
- `--json`: prints indented JSON.
- `--plain`: prints compact JSON.

Example:

```bash
ahm status
ahm
ahm --check workflow status
ahm --check links --json status
ahm --check project-docs status
```

`ahm --check project-docs status` runs opt-in, read-only health checks over a
project's own documentation. It discovers common documentation surfaces rather
than requiring a fixed layout: root-level `README*`, `CONTRIBUTING*`,
`CHANGELOG*`, `ARCHITECTURE*`, and `DESIGN*` Markdown files, plus every Markdown
file under `docs/` (which covers `docs/adr/`). It reports broken relative
Markdown links in those files via `project_doc_link_missing`.

When the repository already uses the `docs/design-docs/` convention (a
`docs/design-docs/` directory containing an `index.md`), this scope also checks
that every design-doc Markdown file is represented in the index and emits
`design_doc_unindexed` for any that are not. Index entries that point at missing
files, and broken relative links inside design-doc files, are reported through
the shared `project_doc_link_missing` finding rather than a separate check.
`ahm` never creates, rewrites, or formats design-doc indexes. Repositories
without a `docs/design-docs/index.md` get no design-doc findings. The checks are
deterministic, read-only, never call models, and never edit source files.

### `doctor`

Reports environment and workflow checks.

The report includes:

- Target root.
- Whether `git` is available on `PATH`.
- Whether workflow metadata is installed.
- Installed and embedded workflow template versions.
- The same workflow validation report used by `status`.
- An informational `agents_prime_missing` finding when root `AGENTS.md`
  exists but does not reference `ahm prime`; `ahm onboard` prints the current
  bootstrap snippet. Missing `AGENTS.md` is not a finding.

Like `status`, `doctor` exits with code 1 when the validation report contains
any error, without printing `error:` to stderr.

Useful flags:

- `--check <scope>`: limit validation to the specified scope. See `status`
  for details.
- `--json`: prints indented JSON.
- `--plain`: prints compact JSON.

Example:

```bash
ahm doctor
ahm --check workflow doctor
```

### `index`

Regenerates generated task, research, ExecPlan, and ADR indexes.

Writes in legacy committed-record repositories:

- `.agents/.tasks/index.md`
- `.agents/.tasks/active/index.md`
- `.agents/.tasks/completed/index.md`
- `.agents/.tasks/cancelled/index.md`
- `.agents/.research/index.md`
- `.agents/exec-plans/active/index.md`
- `.agents/exec-plans/completed/index.md`
- `docs/adr/index.md`

In migrated repositories, the task, research, and ExecPlan indexes are
written to the same relative paths under `.ahm/`; `docs/adr/index.md` stays in
project documentation.

The root index includes status counts, the next ready queue, blocked/open tasks,
and an all-task table. Bucket indexes include task tables for their bucket. The
research and ExecPlan indexes link Markdown files from their source folders.
The ADR index lists readable ADR records from `docs/adr/` as a deterministic
table of ADR, title, status, and date; `README.md` and `index.md` are excluded.

Useful flags:

- `--dry-run`: prints only index paths that are missing or have stale content. A clean repository immediately after `ahm index` produces no output.

Behavior on errors:

- If a task directory cannot be read (I/O error), index generation is aborted
  with a non-zero exit code and existing generated indexes are left unchanged.
- If one or more task files fail to parse, a warning listing the affected files
  is printed to stderr and index generation continues with the remaining tasks.
- Generated indexes never silently omit tasks or produce empty output due to
  task file parse failures.
- Malformed ADR files are omitted from `docs/adr/index.md` and reported by
  `status` / `doctor`; legacy-format ADRs remain readable until migrated.
- After regenerating indexes, `ahm` runs workflow-scoped validation and
  prints any resulting warnings to stderr. This surfaces drift (e.g., a
  completed task referencing an active ExecPlan) immediately rather than
  requiring a separate `ahm status` pass. Use `--dry-run` to preview index
  changes without triggering post-mutation validation.

Example:

```bash
ahm index
ahm --dry-run index
```
