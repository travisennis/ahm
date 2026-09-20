# ahm Specification

## Goals

`ahm` manages repo-local workflow records: tasks under `.ahm/tasks/` and ADRs
under `docs/adr/`. A user can initialize a repository, create and advance
tasks, manage ADR lifecycle, regenerate indexes, and reconcile ahm-owned
workflow state.

## Non-goals For v1

- No model or coding-agent calls, and no delegation to another program. Git is
  the only subprocess `ahm` runs.
- No source-code patching.
- No implicit git commits, pushes, PRs, or branch operations. Explicit
  records commands may read and write under `.ahm/`, but they must not move
  `HEAD`, create branch commits, stage files, write the project index, or
  modify project-owned `.agents/` content.
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

- `init`: install the managed `.ahm` workflow state. On fresh installs
  (no prior workflow metadata), creates the committed `.ahm/` layout
  directly. On repositories with existing `.agents/ahm.json` metadata, the
  existing layout is preserved.
- `upgrade`: update managed workflow state.
- `status`: report workflow health.
- `doctor`: report environment and workflow checks.
- `index`: regenerate generated indexes.
- `records`: migrate records to `.ahm/` and diagnose migration state.
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
ahm-managed records under `.agents/`. The opt-in records migration
(`ahm records migrate`) moves ahm-managed state to
tool-owned `.ahm/` while leaving project-owned agent content under `.agents/`.

Workflow commands are record-layout aware. In legacy repositories (metadata
source `.agents/ahm.json`), task, index, validation, and install behavior is
unchanged and uses `.agents/` paths. After migration, the same commands read
and write task records under `.ahm/tasks/`, and generated indexes are
regenerated at the same relative paths under `.ahm/`.

After migration, supported record mutations (`ahm task` lifecycle
and metadata commands, and `ahm index` after hand edits to records) write
source records directly to `.ahm/`. Generated indexes remain local-only
under `.ahm/`. Record writes never touch branches, `HEAD`, or the project
index.

`ahm` writes `.agents/ahm.json` with the managed file hashes for any legacy managed templates, and repository-scoped workflow
settings. This metadata lets future versions remove or migrate files that have
not been locally changed while preserving user edits.

When `ahm` invokes Git, it scopes the command to the detected repository root
and removes inherited `GIT_DIR`, `GIT_WORK_TREE`, `GIT_INDEX_FILE`,
`GIT_OBJECT_DIRECTORY`, and `GIT_COMMON_DIR` values from the subprocess
environment. This prevents Git hooks or parent processes from redirecting
ahm-owned Git operations to another repository's metadata, worktree, or index.
See ADR 018.

`ahm` reads workflow metadata from committed `.ahm/config.json` when it
is present, falling back to legacy `.agents/ahm.json` otherwise. Fresh
`ahm init` (no prior metadata) creates `.ahm/config.json` and the
committed `.ahm/` layout directly. When `.agents/ahm.json` already
represents the repository, `init` respects the existing layout.

After an explicit migration creates `.ahm/config.json`, metadata reads
prefer it over the legacy `.agents/ahm.json`.

Metadata reads tolerate the obsolete top-level `projectDocs` key without
applying runtime behavior. A real `ahm upgrade` omits that key when it
atomically rewrites ahm-owned metadata, while preserving unrelated unknown
top-level fields. A dry-run reports the upgrade without changing the file.

Example:

```json
{
  "strict_acceptance": true,
  "files": {}
}
```

The optional `strict_acceptance` boolean defaults to `false`. When it is `true`,
`ahm task complete <id>` fails if the task acceptance section is missing, still
contains the seeded `- [ ] TODO` placeholder, or contains unchecked checklist
items. The global `--force` flag overrides this strict completion gate for a
single command while still printing warnings.

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

All workflow record mutations (`ahm task` lifecycle and metadata commands,
`ahm adr` lifecycle commands, `ahm records migrate`, and `ahm task|adr
migrate`) serialize on a single repository-local workflow record lock. The lock lives under `.agents/.lock/workflow-records` or
`.ahm/.lock/workflow-records` depending on the repository's record layout. It is
held across the full read-compute-write sequence for each command, including ID
allocation, file writes, and index regeneration. `--dry-run` and read-only
preview paths do not take the lock and do not write workflow state.

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

1. **Generated indexes** (the task index and its bucket indexes under
   `.agents/.tasks/` or `.ahm/tasks/`, plus `docs/adr/index.md`) — owned by
   `ahm`. Do not edit by hand. Update source records and run `ahm index`.

2. **Workflow procedures** — project-owned. `ahm` emits no procedure text:
   task, ADR, and planning practice lives in the project's own prose under
   `docs/` and `AGENTS.md`. Fresh `ahm init` copies no reference document
   such as `.agents/TASKS.md`, `.agents/DOCS.md`, or `docs/adr/README.md`
   into consumer repositories. Existing `.ahm/tasks/README.md`,
   `.ahm/research/README.md`, and `docs/adr/README.md` scaffold copies from
   older releases are preserved and relinquished from metadata ownership;
   `ahm upgrade` does not remove them.

3. **Obsolete managed instruction files** — older releases copied workflow
   guides into repositories. `upgrade` removes pristine hash-owned copies and
   reports locally edited copies as conflicts; `--force` removes those
   obsolete copies. The former preflight, grooming-backlog, and
   finding-improvements skill files are project-owned: ahm leaves them in
   place, discards any old ownership hashes during init, upgrade, or records
   migration, and never inspects, reports, overwrites, or removes them. Fresh
   installs create none.

4. **Workflow source records** — task files live under `.agents/` in legacy
   committed-record repositories and under tool-owned `.ahm/tasks/` after
   migration. Update them through their
   documented workflows (e.g., `ahm task create`, `ahm task complete <id>`, or
   `ahm index` after manual edits). In migrated repositories, these records
   are committed project files under `.ahm/`. ADRs under
   `docs/adr/` remain project-owned durable documentation and use `ahm adr`
   lifecycle commands.

5. **`AGENTS.md`** — project-owned. `ahm init`, `ahm upgrade`, and `--force`
   never create, overwrite, or remove `AGENTS.md`. Bootstrap text is README
   prose the project writes for itself; `ahm` prints no snippet and inspects
   no project instruction file.

Workflow validation is read-only. `status` and `doctor` report missing or stale
generated indexes, duplicate task IDs across task files, task status and bucket
mismatches, broken task dependencies, tracking tasks with at least one child
whose child tasks are all Completed or Cancelled, completed task
acceptance-note drift, ADR record issues, and broken relative Markdown links
within tasks, ADRs, and their generated indexes. Link discovery uses the
metadata-selected current or legacy record roots plus ADR source files and the
generated ADR index under `docs/adr/`; it does not scan general project
documentation or project-owned agent instructions.
Duplicate task IDs are error-tier findings that name every conflicting path and
require manual removal or renaming; task-record mutation commands refuse to
operate on an affected ID until the conflict is resolved. Read-only list and
validation commands remain available for diagnosis. Project-wide documentation
is not scanned by default; `ahm` validates the workflow files and artifacts it
manages or indexes.

### Validation Scopes

`status` and `doctor` accept a `--check` flag that limits validation to a
specific scope. The default (no `--check`) runs the `workflow` and `links`
validation groups over the managed workflow surface.

Supported scopes:

- `workflow` — managed file consistency, task front matter, dependency cycles,
  task bucket placement, ADR records, generated index freshness. This is the
  core workflow validation set.
- `links` — relative Markdown link existence within task and ADR records and
  their generated indexes. Link validation is independent
  of workflow state and can be run separately to focus on record-integrity
  drift. It does not scan README, CONTRIBUTING, ARCHITECTURE, general `docs/`,
  `AGENTS.md`, `CLAUDE.md`, project-owned skills, or records from the inactive
  current/legacy layout.

Scopes compose: `--check workflow --check links` or `--check workflow,links`
runs both the workflow and link validators. Passing an unknown scope value is a
usage error.

```bash
ahm --check workflow status
ahm --check links --json doctor
```

The output format and exit codes are the same regardless of which scopes are
active; only the reported findings change.

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
7. `depends_on`
8. `created` (optional, omitted when empty)
9. `updated` (optional, omitted when empty)
10. `parent` (optional, omitted when empty)
11. `external_ref` (optional, omitted when empty)
12. Extra/unknown fields (sorted by key)

Optional fields (`created`, `updated`, `parent`, `external_ref`) are emitted
only when non-empty. Extra fields not recognized as standard task fields are
emitted in alphabetical order after all standard fields.

### Front Matter Grammar

Task front matter uses a flat `key: value` format. Each line holds one field.
The value is everything after the first colon, trimmed of leading and trailing
whitespace. Double-quoted values have the wrapping quotes stripped and the
escape sequence `\"` (double quote) is decoded. All other backslash sequences,
including `\\`, are left literal.

Supported value forms:

- Simple: `key: value` → `"value"`
- Colon in value: `labels: type:bug, area:tasks` → `"type:bug, area:tasks"`
- Double-quoted: `title: "My Task: The Reckoning"` → `"My Task: The Reckoning"`
- Escaped double-quoted: `title: "say \"hello\""` → `"say \"hello\""`
- Inline list: `depends_on: 001, 002` or `depends_on: [001, 002]`
- Dash sentinel: `depends_on: -` (empty list, see Dash Sentinel Semantics)

When `ahm` writes a task file, it renders each scalar with the smallest safe
representation. Newlines and lone carriage returns are collapsed to spaces and
leading/trailing whitespace is trimmed. The renderer then quotes any value that
would otherwise be misinterpreted by the parser: empty values, values starting
with `#`, `|`, `>`, or `"`, and values that are a dash followed by a space.
Inside quoted values, double quotes are escaped as `\"`; backslashes are left
literal. Values that do not need quoting are left plain, so colons, internal
quotes, and backslashes are usually unescaped.

Examples of the rendered representation:

- Empty value: `title: ""`
- `# not a comment`: `title: "# not a comment"`
- `| block`: `title: "| block"`
- `- list item`: `title: "- list item"`
- `"quoted`: `title: "\"quoted"`
- `""`: `title: "\"\"\""`
- `say "hello"`: `title: say "hello"`
- `type:bug`: `labels: type:bug`

`ahm task create` rejects titles and labels with leading or trailing whitespace,
newlines, or carriage returns. It also canonicalizes an empty `--labels` value
to the `-` sentinel so that every accepted value round-trips.

Unsupported forms that produce a parse error:

- Unquoted block scalars (`|` and `>`): `description: |\n  multi\n  line`
- Block lists (`-` prefix): `depends_on:\n  - 001\n  - 002`
- Keys with spaces: `bad key: value`

Comments (`#` at line start) and blank lines within front matter are ignored.

A front matter block is opened by a line containing exactly `---` and closed by a
second line containing exactly `---`. The closing delimiter may appear either at
the end of a line followed by the body (`---\n# Body`) or at the end of the file
with no trailing newline (`---\n...\n---`). CRLF line endings are normalized to LF
before parsing. A file that opens with `---` but never closes it produces a parse
error rather than being treated as a file with no front matter.

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
and `labels` during parsing. However, `status`, `priority`, and
`effort` also undergo enum validation that rejects `-`; in valid task files
these fields always hold a recognized enum value. The fields where `-` is an
accepted value are:

- `labels` — default `-` indicates no labels have been assigned.

Note that `depends_on` uses `-` and `[]` interchangeably for an empty dependency
list; both produce `-` on write (see `docs/cli.md`).

The practical consequence is that a round-trip (parse, modify, write) cannot
distinguish between an absent field and an explicit `-`. This is an accepted
convention: the dash sentinel means "not set" and preserves symmetry with the
grammar used in task creation (where `ahm task create` seeds `depends_on: -`).

## Atomic Write Guarantee

All managed writes (metadata, generated indexes, task files, and installed or
upgraded workflow files) use a temporary-file-then-atomic-rename strategy that
guarantees crash safety:

1. Content is written to a unique sibling temp file in the same directory.
2. The temp file is synced to disk (`fsync`).
3. The temp file is atomically renamed to the target path (`os.Rename`, which
   is atomic on Unix when source and destination are on the same filesystem).
4. The parent directory is synced so the rename survives a power loss.

A crash before the rename leaves the original file intact. A crash after the
rename is indistinguishable from a successful write. Stale `.tmp` files left
by a crash are cleaned up opportunistically at the start of `init`, `upgrade`,
and `index` commands.

All workflow record mutations share a single repository-local lock under
`.agents/.lock/workflow-records` or `.ahm/.lock/workflow-records` to serialize
read-compute-write sequences across concurrent `ahm` invocations. Dry-run and
read-only preview paths do not take the lock. While the lock is held, a
background heartbeat periodically refreshes the lock directory's modification
time so that the stale-lock reclamation can distinguish a live lock from an
abandoned one. Stale lock reclamation uses a two-check pattern: it observes
the modification time, waits a short delay, and observes again. It only
reclaims when both observations show an unchanged modification time past the
stale threshold, preventing the reclamation from stealing a lock from a
heartbeating owner. Each acquire writes a unique owner token file named
`owner` (hex content, mode 0600) inside the lock directory; locks left by
pre-token versions of ahm have no such file and are reclaimed via a
filesystem-identity fallback. The reclamation then atomically moves the observed
directory into a unique quarantine and verifies its owner token before
deletion, so a replacement lock is not removed. Release performs the same
owner-token check and reports an error when the acquired directory is missing
or has been replaced.

### Generated Index Write Semantics

`ahm index` writes its 5 generated index files sequentially in sorted path
order. There is no cross-file atomicity: if a mid-batch write fails, earlier
files in the batch have already been updated, the failed file remains stale,
and later files are not written. This leaves a temporarily inconsistent index
state that self-heals on the next successful `ahm index` run. The individual
write of each file is still atomic (see Atomic Write Guarantee above); only
the batch as a whole has no rollback or transaction semantics.

Workflow instructions are project-owned prose under `docs/`; `ahm` prints
none, so there is nothing to render, customize, or keep in sync with the
binary. `ahm prime` is a pure state report: it regenerates indexes and prints
repository state, validation findings, and record counts, and `--json` and
`--plain` expose the same structured report for integrations. General project
documentation is not an ahm-managed scope; each project owns its own
documentation guidance.
