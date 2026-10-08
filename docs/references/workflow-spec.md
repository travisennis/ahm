# ahm Specification

## Goals

`ahm` manages task records in either a project or a user-level home store,
and ADRs under `docs/adr/`. In project mode task records live under
`.ahm/tasks/`; in home mode they live under the store's project directory. A
user can initialize a repository, create and advance tasks, manage ADR
lifecycle, regenerate indexes, and reconcile ahm-owned workflow state.

## Non-goals

- No model or coding-agent calls, and no delegation to another program. Git is
  the only subprocess `ahm` runs.
- No source-code patching.
- No implicit git commits, pushes, PRs, or branch operations. Workflow
  commands may read and write under the resolved records root and the
  ahm-owned project state, but they must not move `HEAD`, create branch
  commits, stage files, write the project index, or modify project-owned
  files.
- No database.

## CLI Contract

Usage:

```bash
ahm [global flags] <command> [command flags]
```

The CLI contract is owned by the CLI reference, starting at
[`docs/cli.md`](../cli.md): [the global
contract](cli/global-contract.md) owns the global flag set,
[commands](cli/commands.md) owns the command inventory and aliases,
and [task commands](cli/task-commands.md) owns the task family. Each
page carries an inventory block that a test checks against the executable
command tree. This specification does not repeat them, so a change to the
command surface updates those pages instead.

Exit codes:

- `0`: success.
- `1`: runtime failure.
- `2`: invalid usage.

## Workflow State

Workflow state has a project root that owns committed configuration at
`.ahm/config.json`, the managed `.ahm/.gitignore`, and ADRs under `docs/adr/`.
Task records and their generated indexes live in a records root selected by
`tasks_location`: `project` keeps them under `.ahm/tasks/`, and `home` keeps
them under `<store>/projects/<dir>/tasks/`. Project-owned agent content stays
outside ahm-managed state, including `AGENTS.md` and files under `.agents/`.

A repository has two managed roots. The project root is always the directory
containing `.ahm/` and `docs/adr/`. The records root is the project root's
`.ahm/tasks/` in project mode, or the store project's `tasks/` directory in
home mode. Every task record path is derived from the records root. Record
writes never touch branches, `HEAD`, or the project index. A repository whose
metadata is still the retired `.agents/ahm.json` layout, without
`.ahm/config.json`, is refused by root detection, which names the final v1
release (`v1.1.0`) that can migrate it.

When `ahm` invokes Git, it scopes the command to the detected repository root
and removes inherited `GIT_DIR`, `GIT_WORK_TREE`, `GIT_INDEX_FILE`,
`GIT_OBJECT_DIRECTORY`, and `GIT_COMMON_DIR` values from the subprocess
environment. This prevents Git hooks or parent processes from redirecting
ahm-owned Git operations to another repository's metadata, worktree, or index.
See ADR 018.

Supported record mutations (`ahm task` lifecycle and metadata commands, and
`ahm index` after hand edits to records) write task records and their generated
indexes to the resolved records root. ADR records and `docs/adr/index.md`
remain in the project root. `store migrate` is the only command that moves task
records between the two records roots.

`ahm` writes `.ahm/config.json` with repository-scoped workflow settings. The
file is hand-edited: `ahm` deliberately has no `config` command, and
[ADR 026](../adr/026-keep-workflow-configuration-hand-edited-rather-than-adding-a-config-command.md)
keeps hand-editing as the supported write path. The recognized keys split into
one user-mutable setting and three `ahm`-owned keys:

- `strict_acceptance` — the only user-mutable, behavior-bearing key. It is an
  optional boolean that defaults to `false`. When it is `true`,
  `ahm task complete <id>` fails if the task acceptance section is missing,
  still contains the seeded `- [ ] TODO` placeholder, or contains unchecked
  checklist items. The global `--force` flag overrides this strict completion
  gate for a single command while still printing warnings.
- `version` — the obsolete template-version field, preserved only so existing
  files round-trip. `ahm` re-emits it verbatim but never interprets or sets it.
- `tasks_location` — the records layout. It changes only through
  `ahm store migrate --to home|project`.
- `files` — the managed-file ownership hash map. `ahm init` reconciles it,
  deleting the entries for the retired managed files and generated-index paths
  it knows about while preserving every other entry; this version records no
  new hashes and validates none.

`version`, `tasks_location`, and `files` are `ahm`-owned; hand-editing them is
unsupported. Unknown top-level keys are project-owned and preserved on every
`ahm` write. A missing `.ahm/config.json` means uninstalled, and `ahm init`
creates it; a corrupt file fails loudly and is never silently reset.

Example:

```json
{
  "strict_acceptance": true,
  "tasks_location": "home",
  "files": {}
}
```

`ahm task cancel <id>` requires `--reason <text>`. The reason is trimmed and
must be non-empty; `--force` does not bypass this requirement. Cancellation
stores the reason in the task Markdown body under `## Cancellation Reason`,
updating that section when it already exists and appending it otherwise.
`--dry-run` validates and previews the reason without writing. Cancellation
warns, but does not fail, when acceptance notes still contain the seeded
`- [ ] TODO` placeholder.

`ahm task block <id> --reason <text> [--ref <text>]` records the reason, and an
optional external reference, in the task front-matter fields `blocked_reason`
and `blocked_ref`, and sets the status to `Blocked`. Blocking replaces any
previously recorded reason and reference. `--reason` is required and
must be non-empty, and neither `--reason` nor `--ref` may contain a newline or
carriage return, since both are single-line front-matter scalars. Any
non-terminal status (`Open`, `Pending`, `In Progress`,
or `Blocked`) can be blocked; a `Completed` or `Cancelled` task is refused as a
usage error. `ahm task unblock <id>` returns a `Blocked` task to `Pending` and
clears both fields; a task that is not `Blocked` is refused. The fields are
present only while the status is `Blocked`: every other status transition
clears them, and a `Blocked` task whose `blocked_reason` is empty is reported as
the warning-tier finding `task_blocked_missing_reason`.

When `ahm task complete <id>` completes a task, it also scans active `Blocked`
tasks that directly depend on that completed ID. Dependents whose full
`depends_on` list is now satisfied are moved to `Pending` with an `updated`
timestamp, and their `blocked_reason` and `blocked_ref` are cleared, before
indexes are regenerated; the automatic path writes no reason of its own.
Dependents with remaining incomplete dependencies, and blocked tasks that do
not depend on the completed task, are left unchanged. `--dry-run` reports the
completion move and dependent unblock changes without writing task files or
indexes.

`ahm` reads workflow metadata from committed `.ahm/config.json`. `ahm init`
creates it when it is missing and reconciles it when it is present.

Metadata reads tolerate the obsolete top-level `taskWork`,
`default_work_agent`, `projectDocs`, and `research` keys without applying
runtime behavior. `ahm init` omits those keys when it rewrites ahm-owned
metadata, while preserving unrelated unknown top-level fields. A dry-run
reports the rewrite without changing the file, and an up-to-date file is left
untouched.

All workflow record mutations (`ahm task` lifecycle and metadata commands and
`ahm adr` lifecycle commands) serialize on a single repository-local workflow
record lock. The lock lives beside the records root: `.ahm/.lock/` in project
mode, or the store project's `.lock/` in home mode. It is held across the full
read-compute-write sequence for each command, including ID allocation, file
writes, and index regeneration. `--dry-run` and read-only preview paths do not
take the lock and do not write workflow state.

The store's registry and project state writes serialize on a second lock, the
store-state lock at `<store>/.lock/store-state`, because the registry is
machine-wide and a per-project lock cannot serialize it. It is held across the
whole read-modify-write of `<store>/registry.json` and of a project's
`project.json`, so two projects that record an observation concurrently both
keep theirs and no interleaving of `store path` and `task create` can lower the
persisted `next_id`. The record lock is always acquired first, and no command
takes the record lock while holding the store-state lock. The lock is taken
only where a store write happens, so `--dry-run` still creates no store at all
and read-only commands take no lock.

When the `--parent <id>` flag is provided, `ahm task create` allocates the next
available lettered child ID under that parent (`137a`, `137b`, ..., `137z`) and
writes `parent: <id>` in the child task front matter. The parent must be a
top-level task (no letter suffix); child tasks cannot be parents. The allocation
scans parsed tasks and filesystem entries across all three task buckets to avoid
collisions. At most 26 children are allowed per parent. The workflow lock
serializes both top-level and child ID allocation. In the store, allocation
follows the parent's persisted child suffix mark, self-healed from the highest
child letter present, so a deleted child's letter is never reissued; in a
project it takes the first free letter, because Git history there is the
evidence that a letter was spent.

### Task Records And The Home Store

The store root is `~/.ahm` by default. `AHM_HOME` overrides it and must name
an absolute path; a relative value is a usage error. The root holds
`registry.json`, a `projects/` directory, and the `.lock/` directory that holds
the store-state lock. Each project directory is named
`<slug>-<hash8>`, where the slug is the key's repository name for a remote key
and `project` for a path key, and the hash keeps same-named projects apart. It
contains the task records under `tasks/`,
the store-managed `.gitignore`, the project state file `project.json`, and the
`.lock/` directory that holds the record lock.

A project key is derived per command from the remote Git selects: `origin`
when it exists, otherwise the only remote when the repository has exactly one.
The key is the lowercased `host/owner/repo` form, with the scheme, userinfo,
default port, trailing `.git`, and trailing slash removed, while non-default
ports remain. A repository with no single remote candidate, a remote that names
no URL, a `file://` or local-path remote, or no remote at all uses the SHA-256
of the symlink-resolved project root instead. Credentials are never included in
the key or persisted in the registry. A home-mode project whose root has
`.git` needs Git to read that repository; a Git read failure is reported rather
than silently deriving a different key. A root with no `.git` uses the path
rule and reads no Git.

`registry.json` is the machine-level mapping from project key to store
directory. It is derived data and is never the authority for record contents.
Each project directory's `project.json` records store format version `1` and
the non-decrementing task ID marks that prevent a deleted task ID from being
reissued: `next_id` for the top-level numbers, and `child_suffix_marks`, one
highest allocated letter per parent, for the child letters. Every
read-modify-write of the registry and of a project's state file holds the
store-state lock, and each writer keeps the higher mark it sees, so a mark only
moves up even against a writer that does not take the lock. A store or project
file with a format version newer than this version is refused rather than
partially read. The registry may also record observed remote spellings with
credentials removed and `migrated_from`. `store unregister` removes one recorded
path from an entry, or the entry itself; the store directory and the project's
state file, including the task ID marks it holds, are left in place, so
unregistering loses no record and a later `store path` re-registers the entry.

The committed `.ahm/config.json` selects the layout with `tasks_location`:
`project` keeps records in the project, `home` resolves the store, and a
missing key means `project`. A repository with no configuration at all is a new
project and resolves as `home`; `ahm init` writes `tasks_location: home` when
it creates that configuration. A home-mode project keeps its records, generated
indexes, managed store `.gitignore`, and workflow lock in the store's project
directory, while the committed configuration and ADRs stay in the repository.

Paths in validation findings, error messages, index listings, and lock errors
render as `store:<path-relative-to-the-store-project-directory>` in home mode
(for example, `store:tasks/active/001.md`) and repository-relative paths in
project mode. A lock at the store root, where no project directory contains it,
renders relative to the store root instead (`store:.lock/store-state`). The JSON
`path` field of a task and the dry-run create, move, and
unblock previews use the same store display while leaving project-mode payloads
byte-identical. An operating-system error keeps its own text. The store's
reporting commands, `ahm store path` and `ahm store list`, are the exceptions:
they report absolute store paths, with `~` abbreviation in text output.

A relative Markdown link in a task record resolves first against the record's
own directory. If the target does not exist there, home mode retries against
the record's logical in-project path (`<project>/.ahm/tasks/<bucket>/`), so
links written before a migration keep working. Project mode uses the project
layout directly.

An `ahm:` reference resolves by identity instead of by path:

- `ahm:task/<id>` names a task record in any bucket, resolved the way
  `ahm task show` resolves its argument.
- `ahm:adr/<ref>` names an ADR by the reference form `ahm adr show` accepts
  (`9`, `009`, `009-slug`).
- `ahm:doc/<path>` names a project file by its repository-relative path, with
  forward slashes.

The scheme and its kinds are matched case-insensitively; the canonical
spelling is lower case. Both forms are supported: existing relative links are
not migrated and are not reported as drift, and `ahm` keeps generating
relative links for its indexes and for the supersession note `ahm adr
supersede` writes. A reference that follows the scheme but names nothing is
reported as `markdown_link_missing`; a malformed or ambiguous reference — a
missing or unknown kind, an empty target, a target that resolves to more than
one record, or an `ahm:doc/` path that escapes the project root — is reported
as `markdown_link_invalid`. Link validation covers task records, ADR records,
and their generated indexes; it does not scan general project documentation.
See
[ADR 027](../adr/027-use-ahm-identity-references-for-workflow-record-cross-links.md).

Workflow record mutations take the lock beside the records root:
`.ahm/.lock/` in project mode, or `<store>/projects/<dir>/.lock/` in home mode.
Store state writes take the store-state lock at `<store>/.lock/store-state`
instead, because the registry is machine-wide. Stale temporary-file cleanup
scans the project's `.ahm/` state directory and,
in home mode, the store project's state directory; it never walks the whole
repository or store root.

## File Ownership Boundary

`ahm` owns the workflow files it installs, maintains, generates, and upgrades.
Consumer projects must not hand-edit ahm-owned generated files as a substitute
for using `ahm` commands.

The ownership categories are:

1. **Generated indexes** (the task index and its bucket indexes under the
   resolved records root, plus `docs/adr/index.md`) — owned by
   `ahm`. Do not edit by hand. Update source records and run `ahm index`.

2. **Workflow procedures** — project-owned. `ahm` emits no procedure text:
   task, ADR, and planning practice lives in the project's own prose under
   `docs/` and `AGENTS.md`. Fresh `ahm init` copies no reference document
   such as `.agents/TASKS.md`, `.agents/DOCS.md`, or `docs/adr/README.md`
   into consumer repositories. Existing `.ahm/tasks/README.md`,
   `.ahm/research/README.md`, and `docs/adr/README.md` scaffold copies from
   older releases are preserved and relinquished from metadata ownership; no
   command removes them.

3. **Retired managed files** — older releases copied workflow guides into
   repositories and tracked ownership hashes for them. `ahm init` discards
   those hashes and never creates, inspects, overwrites, or removes the
   files. The former preflight, grooming-backlog, and finding-improvements
   skill files are project-owned: ahm leaves them in place and never inspects,
   reports, overwrites, or removes them. Fresh installs create none.

4. **Workflow source records** — task files live under the resolved records
   root. Update them through their documented workflows (e.g., `ahm task
   create`, `ahm task complete <id>`, or `ahm index` after manual edits). In
   project mode these records are committed project files under `.ahm/`. In
   home mode they are machine-local files in the store's project directory.
   ADRs under `docs/adr/` remain project-owned durable documentation and use
   `ahm adr` lifecycle commands.

5. **`AGENTS.md`** — project-owned. `ahm init` and `--force`
   never create, overwrite, or remove `AGENTS.md`. Bootstrap text is README
   prose the project writes for itself; `ahm` prints no snippet and inspects
   no project instruction file.

In home mode, the store's per-project directory is ahm-owned working state:
its `.gitignore`, generated indexes, `project.json`, and `.lock/` are managed
there, while `<store>/registry.json` is machine-level derived data. The
project root remains the only place `ahm` writes committed workflow state
(`.ahm/config.json`, the committed `.ahm/.gitignore`, and ADRs).

Workflow validation is read-only. `status` and `doctor` report missing or stale
generated indexes, duplicate task IDs across task files, task status and bucket
mismatches, broken task dependencies, tracking tasks with at least one child
whose child tasks are all Completed or Cancelled, completed task
acceptance-note drift, ADR record issues, and broken or malformed Markdown
cross-references within tasks, ADRs, and their generated indexes. They also
report the home store's error-tier `task_records_in_project` finding when a
task record remains under `.ahm/tasks/` while `tasks_location` is `home`, and
`store_dir_unreadable` when the resolved store records directory is missing or
unreadable. Link discovery uses the current record root for tasks plus ADR
source files and the generated ADR index under `docs/adr/`; it does not scan
general project documentation or project-owned agent instructions.
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

- `workflow` — workflow metadata, task front matter, dependency cycles, task
  bucket placement, ADR records, generated index freshness. This is the core
  workflow validation set.
- `links` — Markdown cross-reference resolution within task and ADR records
  and their generated indexes: relative link existence and the `ahm:`
  reference scheme. Link validation is independent
  of workflow state and can be run separately to focus on record-integrity
  drift. It does not scan README, CONTRIBUTING, ARCHITECTURE, general `docs/`,
  `AGENTS.md`, `CLAUDE.md`, project-owned skills, or records outside the
  resolved task records root and `docs/adr/`.

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
findings that say to convert them to MADR front matter by hand; they do not
make `status` or `doctor` fail.

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
7. `exec_plan` (retired, emitted only when an older `ahm` wrote a value)
8. `depends_on`
9. `created` (optional, omitted when empty)
10. `updated` (optional, omitted when empty)
11. `parent` (optional, omitted when empty)
12. `external_ref` (optional, omitted when empty)
13. `blocked_reason` (optional, omitted when empty)
14. `blocked_ref` (optional, omitted when empty)
15. Extra/unknown fields (sorted by key)

Optional fields (`created`, `updated`, `parent`, `external_ref`,
`blocked_reason`, `blocked_ref`) are emitted only when non-empty. Extra fields
not recognized as standard task fields are emitted in alphabetical order after
all standard fields.

`exec_plan` is retired: `ahm` neither reads nor validates it, and no command
writes one. A value an older release wrote survives as an unknown field and is
re-emitted in the slot it occupied while it was schema, so such a task file
round-trips byte-identically. Deleting the field is the project's choice.

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
to the `-` sentinel so that every accepted value round-trips. `ahm task
create --labels` sets the full label set and is the only path that introduces a
label no record carries yet; `ahm task edit --add-label` accepts only labels the
records already use, the vocabulary `ahm task labels` reports.

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

All managed writes (metadata, generated indexes, task files, and installed
workflow files) use a temporary-file-then-atomic-rename strategy that
guarantees crash safety:

1. Content is written to a unique sibling temp file in the same directory.
2. The temp file is synced to disk (`fsync`).
3. The temp file is atomically renamed to the target path (`os.Rename`, which
   is atomic on Unix when source and destination are on the same filesystem).
4. The parent directory is synced so the rename survives a power loss.

A crash before the rename leaves the original file intact. A crash after the
rename is indistinguishable from a successful write. Stale `.tmp` files left
by a crash are cleaned up opportunistically at the start of the `index`
command, except under `--dry-run`, which removes nothing.

All workflow record mutations share a single repository-local lock beside the
records root: `.ahm/.lock/workflow-records` in project mode, or
`<store>/projects/<dir>/.lock/workflow-records` in home mode. It serializes
read-compute-write sequences across concurrent `ahm` invocations, including
clones that share one store. Dry-run and read-only preview paths do not take
the lock. While the lock is held, a background heartbeat periodically refreshes
the lock directory's modification time so that the stale-lock reclamation can
distinguish a live lock from an abandoned one. Stale lock reclamation uses a
two-check pattern: it observes the modification time, waits a short delay, and
observes again. It only
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

Store state writes share a second lock, the store-state lock at
`<store>/.lock/store-state`. It serializes every read-modify-write of
`registry.json` and of a project's `project.json`, across projects and
processes, and uses the same protocol: owner token, heartbeat, and stale
reclamation. It is acquired only where a store write happens — `store path`, a
home-mode `init`, `store migrate`, `store unregister`, and the `next_id` write of
`task create` — and only after the record lock when a command holds both, so a
store write from `task create` cannot deadlock with another project's
`store path`.

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

### Bulk Task Import Writes

`task import` uses the existing task record format and ID allocator. It resolves
batch-local `@ref` names before rendering; names are not persisted. All record
refusals are reported before writing. A successful batch holds the record lock
once and regenerates indexes once. In home mode it also holds the store-state
lock while writing and, if an ordinary write fails, restoring affected files.
Rollback restores pre-import record/index/state bytes; it never modifies the
registry. A mark restoration can undo this transaction's unpublished IDs
while the store-state lock excludes observations by other cooperating writers.
The marks remain monotonic for successfully published records.

Import adds restoration for ordinary write failures; the usual sequential
index semantics still apply to other commands. Atomicity after power loss or
process termination is per file, with no durable batch journal. Preserve raised
marks when recovering an interrupted import and regenerate indexes after
removing only records from the failed batch. See the
[bulk import contract](cli/task-commands.md#bulk-import) and
[ADR 025](../../docs/adr/025-import-task-batches-with-prevalidation-and-rollback.md).
No workflow format migration is required.
