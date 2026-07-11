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

Example:

```bash
ahm adr accept 009
```

### `adr reject <id>`

Sets a MADR-profile ADR's `status:` to `rejected`, updates `date:` to today's
date, and regenerates indexes. The command rewrites only front matter.

Example:

```bash
ahm adr reject 009
```

### `adr deprecate <id>`

Sets a MADR-profile ADR's `status:` to `deprecated`, updates `date:` to today's
date, and regenerates indexes. The command rewrites only front matter.

Example:

```bash
ahm adr deprecate 009
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
attempts to point an already-superseded ADR at a different replacement.

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
- Writes committed `.ahm/config.json` preserving all metadata fields except
  ref-backed storage fields, and removes legacy `.agents/ahm.json`.
- Prints a `git add` / `git rm --cached` instruction covering the legacy
  record paths that are still tracked in the project git index. The user runs
  that command and commits the result together with the new `.ahm/` state;
  `ahm` never stages, commits, or moves `HEAD`.

Migration never touches project-owned `.agents/` content such as
`.agents/prompt.md`, `AGENTS.md`, or `.agents/skills/`, and it does not move
`HEAD`, create branch commits, stage files, write the project index, or
create or update any `refs/ahm/*` ref.

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
   delete the new `.ahm/` directory.
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

### `records status`

Reports ref-backed workflow record state without writing files, moving refs, or
materializing records. The command reads `.ahm/config.json` or legacy
`.agents/ahm.json`, inspects the configured local records ref, checks the
configured remote with `git ls-remote`, and compares local `.ahm/` source
records against the local records ref.

The text report includes:

- `mode`: configured records storage mode.
- `ref`: records ref, defaulting to `refs/ahm/records`.
- `remote`: records remote, defaulting to `origin`.
- `remote_url`: remote URL when configured.
- `remote_supported`: whether the remote URL is supported for the initial
  records sync surface. GitHub remotes are supported; local filesystem remotes
  are accepted for local testing and offline validation.
- `local_commit` and `remote_commit`: commit IDs, or `missing`.
- `relation`: `equal`, `local_only`, `remote_only`, `different`, `ahead`,
  `behind`, `diverged`, `missing`, or `unknown`.
- `working_clean`, `working_added`, `working_modified`, and `working_deleted`:
  local `.ahm/` source-record state relative to the local records ref.
- `diagnostic`: actionable setup or remote-access guidance when available.

Useful flags:

- `--json`: prints indented JSON with the same fields.
- `--plain`: prints compact JSON with the same fields.

Example:

```bash
ahm records status
ahm --json records status
```

### `records pull`

Fetches the configured remote records ref into a private tracking ref, updates
the local records ref, and materializes source records into `.ahm/`.

The command requires `store_mode: "ref"` metadata. It rejects unsupported
remotes before fetching. It also refuses to pull when local `.ahm/` source
records differ from the local records ref, because pulling would overwrite
unsnapshotted local work.

Writes:

- `refs/ahm/remotes/<remote>/...` through `git fetch`.
- `refs/ahm/records` through `git update-ref`.
- `.ahm/` source records through materialization.
- `records_last_sync` in workflow metadata after a successful pull.

It does not move `HEAD`, create branch commits, stage files, write the project
index, or modify project-owned `.agents/` content.

Useful flags:

- `--dry-run`: previews the pull plan without fetching, moving refs, writing
  files, or updating metadata.
- `--json`: prints indented JSON.
- `--plain`: prints compact JSON.

Example:

```bash
ahm records pull
ahm --dry-run records pull
```

### `records push`

Snapshots local `.ahm/` source records into the configured local records ref
and pushes that ref to the configured remote.

The command requires `store_mode: "ref"` metadata and a supported remote. Push
uses a normal non-forced ref update; when the remote records ref is not a
fast-forward, the command fails with a diagnostic that points to `ahm records
pull` or conflict resolution.

Writes:

- Git objects for the records tree and commit.
- `refs/ahm/records` through `git update-ref`.
- The remote `refs/ahm/records` through `git push`.
- `records_last_sync` in workflow metadata after a successful push.

It does not move `HEAD`, create branch commits, stage files, write the project
index, or modify project-owned `.agents/` content.

Useful flags:

- `--dry-run`: previews the push plan without snapshotting, moving refs,
  pushing, or updating metadata.
- `--json`: prints indented JSON.
- `--plain`: prints compact JSON.

Example:

```bash
ahm records push
ahm --dry-run records push
```

### `records sync`

Synchronizes local `.ahm/` source records and the configured remote
`refs/ahm/records` ref. The command fetches remote state, compares it with the
local records ref, then pulls, pushes, or reports divergence.

Behavior:

- If the remote ref is missing, local records are snapshotted and pushed.
- If the local ref is missing and local `.ahm/` records are clean, remote
  records are pulled and materialized.
- If local and remote refs are equal but `.ahm/` records changed locally, the
  local records are snapshotted and pushed.
- If the local ref is ahead, it is pushed.
- If the remote ref is ahead and local `.ahm/` records are clean, it is pulled
  and materialized.
- If local and remote refs diverged, or pulling would overwrite unsnapshotted
  local `.ahm/` changes, the command fails with an actionable diagnostic.

Writes are the union of `records pull` and `records push`, depending on the
chosen path. The command does not move `HEAD`, create branch commits, stage
files, write the project index, or modify project-owned `.agents/` content.

Useful flags:

- `--dry-run`: previews the sync plan without fetching, snapshotting, moving
  refs, writing files, pushing, or updating metadata.
- `--json`: prints indented JSON.
- `--plain`: prints compact JSON.

Example:

```bash
ahm records sync
ahm --dry-run records sync
```

### `records doctor`

Reports diagnostics for records setup and migration state. The migration check
reports leftover legacy record files or config under `.agents/` and legacy
record paths still tracked in the project git index, pointing at
`ahm records migrate` or the required Git cleanup commands. This command will
be removed in a future release; use `ahm status` for workflow state and
`ahm records migrate --dry-run` to preview migration.

Text output includes `ok: true|false` and a `checks:` section. With `--json` or
`--plain`, the same information is emitted as structured JSON.

Example:

```bash
ahm records doctor
ahm --json records doctor
```

### `prime`

Prints a session briefing with repository state, task backlog, and managed-work
routing. This is the canonical session-start command for coding agents.

In ref-backed record mode, `prime` synchronizes workflow records (fetch/pull
from remote when available), materializes them, regenerates indexes, and
validates workflow state before printing the briefing. Sync failures degrade
to warnings; the briefing is always shown.

The briefing includes (in order, omitting empty sections):

- Dirty-worktree warning when the working tree has uncommitted changes.
- Repository root, installed workflow version, and validation status.
- `## In Progress` — full task lines for all in-progress tasks.
- `## Ready` — ready task lines capped at 5, with an overflow pointer to
  `ahm task ready` for the remainder.
- Blocked and open task counts, with commands to expand each.
- Active ExecPlans and recent research notes.
- Stale/unsynced records state in ref mode.
- `## Managed Work Intake` — the routing table for managed-work types,
  active-mode workflow record paths, and notes on workflow and multi-step
  plans.
- `## Useful Commands` — common follow-up commands.

Supports `--json`, `--plain`, and `--text` output.

Useful flags:

- `--no-sync`: skip records sync in ref mode (offline or hook-constrained
  environments).
- `--json`: prints structured output with `root`, `workflow`, `git`,
  `tasks` (with `in_progress`, `ready`, `ready_total`, `blocked`, `open`),
  `records` (with `mode`, `synced`, `stale`, `last_sync`, `message`),
  `plans` (active ExecPlan summaries), `research` (recent research notes),
  and `commands`.
- `--plain`: prints compact JSON.

Examples:

```bash
ahm prime
ahm prime --no-sync
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
repository's active storage mode.

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

### `init`

Installs the managed `.agents` workflow into the target root.

`init` creates missing workflow directories, metadata, and generated indexes.
Canonical agent instructions are exposed through `ahm context`, not copied into
consumer repositories. `init` does not create or overwrite `AGENTS.md`.

Writes in legacy committed-record repositories:

- `.agents/ahm.json` metadata.
- Generated index files under `.agents/.tasks/`, `.agents/.research/`,
  `.agents/exec-plans/`, and `docs/adr/index.md`.
- Workflow directories under `.agents/` and `docs/adr/`.

After a repository has migrated to ref-backed records, `init` and `upgrade`
create missing record directories and generated indexes under `.ahm/` and read
or write committed `.ahm/config.json` instead of `.agents/ahm.json`.

Useful flags:

- `--dry-run`: prints files, directories, metadata, and indexes that would be
  written.
- `--force`: adopts supported existing managed files where applicable.
- `--json` or `--plain`: changes the emitted install summary format.

Example:

```bash
ahm init
ahm --dry-run init
```

### `upgrade`

Updates managed workflow state for the embedded template version.

`upgrade` compares `.agents/ahm.json` hashes with managed files in the target
root. Previously managed workflow instruction files that still match their
recorded managed hash are removed because canonical guidance and procedures now
come from scoped contexts, delegation commands, and the embedded task-work
review. This includes the three formerly managed procedure-skill files. Locally
modified obsolete files are preserved and reported as conflicts unless
`--force` is used. Generated indexes are regenerated.

The metadata `version` field always advances to the embedded template version,
even when conflicts exist. This means a partial upgrade (some files conflicted,
others updated) records the new template version so subsequent upgrades can
correctly identify already-updated files.

`AGENTS.md` remains project-owned and is never overwritten or removed.

Useful flags:

- `--dry-run`: previews all supported writes.
- `--force`: removes locally modified former instruction and procedure files.
  This does not apply to `AGENTS.md`.
- `--json` or `--plain`: changes the emitted upgrade summary format.

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

In ref-backed repositories, the task, research, and ExecPlan indexes are
written to the same relative paths under `.ahm/`; `docs/adr/index.md` stays in
project documentation. `ahm index` also snapshots hand-edited `.ahm/` source
records into the local records ref while excluding generated indexes.

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
