# ahm Task Commands

This reference covers task lifecycle, dependency, completion, blocking,
cancellation, and reopening commands. For task file grammar and validation
finding codes, see [task file and validation formats](task-file-format.md).

Exhaustive flag details live in `ahm <command> --help`. This page documents
only compatibility guarantees that generated help cannot express.

## Task Command Inventory

The blocks below are machine-checked against the Cobra command tree by
`just cli-parity`: every visible `task` and `task dep` subcommand appears in
one of them. Add or remove a subcommand in `internal/ahm/task_commands.go` or
`task_deps.go` and update the matching block in the same change. Command
aliases are one inventory for the whole tree, so they are documented on the
[commands page](commands.md#command-inventory) rather than here.

```text ahm-inventory task-subcommands
accept
block
blocked
cancel
comment
complete
create
dep
edit
import
labels
list
next
ready
reopen
search
show
start
unblock
```

```text ahm-inventory dep-subcommands
add
cycles
remove
tree
```

## Task Record Locations

Task records live under the records root selected by `tasks_location`. In
project mode they are in `.ahm/tasks/active/`, `.ahm/tasks/completed/`, and
`.ahm/tasks/cancelled/`. In home mode the same buckets live under
`~/.ahm/projects/<dir>/tasks/`, or under the absolute `AHM_HOME` override.
ADRs always live under `docs/adr/`. Run `ahm store path` to print the resolved
home-store records directory; findings, index listings, and task JSON paths
render home-mode records relative to the store project directory as
`store:<path-relative-to-store-project-directory>`, for example
`store:tasks/active/001.md`.

Task statuses: `Open`, `Pending`, `In Progress`, `Blocked`, `Tracking`,
`Completed`, `Cancelled`.

Task priorities: `P0` – `P4`.

Task efforts: `XS`, `S`, `M`, `L`, `XL`.

## ID Resolution

Task IDs are resolved by exact string match first. If no exact match is found,
an exact numeric match is attempted (the pattern and task ID are parsed by
numeric value and optional letter suffix, so `1` matches `001` and `1a` matches
`001a`). If no exact numeric match exists, numeric prefix matching is used,
which can match multiple tasks. If a prefix matches more than one task, the
command lists the matching IDs and fails as ambiguous.

## Shared List Sorting

`task list`, `task ready`, and `task blocked` accept `--sort <field>` with
values `priority`, `id`, `created`, `updated`, `effort`, `status`, `title`.
Default is `priority`. `--reverse` reverses the complete selected ordering.

Priority sort order: `P0` → `P4`. Effort: `XS` → `XL`. Status: `Open` →
`Cancelled`. IDs use numeric-aware ordering. Titles use case-insensitive
alphabetical order. Missing or invalid timestamps sort before valid ones.
Every field except `id` uses the task ID as its deterministic tie-breaker.

The selected order is the same in text, plain, and JSON output.

`task next` always selects the highest-priority ready task without sort flags.

## Malformed Task Resilience

List-like commands (`task list`, `task ls`, `task ready`, `task blocked`,
`task search`, `task labels`, `task next`, `task dep cycles`, `task dep tree`)
and `ahm index` tolerate malformed task files: they skip unparseable files,
produce output from remaining valid tasks, and print a warning to stderr.

`task create` also tolerates malformed files: it warns but still assigns the
next available ID, scanning both parsed tasks and task files on disk to avoid
collisions.

Task resolution commands (`task show`, `task edit`, `task start`,
`task complete`, `task cancel`, `task accept`, `task reopen`, `task block`,
`task unblock`, `task comment`,
`task dep add`, `task dep remove`) skip malformed files during ID resolution.
A malformed task cannot be resolved and produces a `task not found` error.

Validation commands (`ahm status`, `ahm doctor`) are strict: they report
malformed task files as `task_malformed` validation errors and exit code 1.

## Per-Command Guarantees

All task commands regenerate indexes after mutations unless stated otherwise.
All support `--dry-run` for previewing write operations.

### `task create <title> [flags]`

Creates a new task and regenerates indexes.

**Top-level ID allocation:** the next zero-padded numeric ID after the highest
existing numeric task ID (`001`, `002`, ...), which is the higher of one past
the highest record present and the counter the home store persists.
Non-numeric suffix IDs are ignored for this calculation. In the store the
counter never decreases, so a record deleted by hand does not return its ID to
the pool, and `ahm init` records the counter the records present imply; in a
project, Git history is the evidence that a deleted ID was used.

**Subtask (child) ID allocation:** When `--parent <id>` is provided, next
available lettered child ID under that parent (`137a`, `137b`, ...). At most
26 children per parent. Scans across `active/`, `completed/`, `cancelled/`
buckets to avoid collisions. In the store the parent's child suffix mark never
decreases, so a child record deleted by hand does not return its letter to the
pool, and `ahm init` records the marks the children present imply; in a project,
allocation keeps the letter scan and Git history is the evidence that a letter
was spent.

Concurrent creates are serialized with the workflow record lock beside the
records root, so clones that share a home store serialize on the same lock.

**Guarantees:**

- `--body-file` provides full body content below the H1; ahm owns ID, front
  matter, heading, location, and index regeneration.
- `--body-file` and `--description` are mutually exclusive.
- `--body` takes the same full-body content as `--body-file` but inline. It is
  mutually exclusive with both `--body-file` and `--description`. An inline
  `--body` of only whitespace is a usage error.
- `--external-ref` writes the `external_ref` front-matter field, which no other
  flag sets. A value containing a newline or carriage return is a usage error.
- Title and `--labels` must not contain leading/trailing whitespace, newlines,
  or carriage returns. Empty labels canonicalize to `-`.
- `--labels` sets the full label set and accepts labels no record has used yet,
  so it is how a new label enters the vocabulary that `task edit --add-label`
  then accepts.
- `--depends-on <ids>` accepts a comma-separated list of task IDs. Each ID is
  resolved with the normal ID resolution rules, canonicalized to the
  zero-padded form, and written to the `depends_on` field sorted by ID. A
  missing, ambiguous, Completed, or Cancelled dependency is rejected; a
  dependency on the ID that `task create` is about to allocate is rejected as
  a self-cycle. `--depends-on` combines with `--parent`.
- `--dry-run` prints the target path and ID without creating, plus the planned
  `depends_on` when `--depends-on` is set. Dependency validation still runs in
  dry-run mode.

### `task edit <id> [flags]`

Edits an existing task's front matter and body. Added after v2.0.0.

Every flag replaces its field when supplied and leaves the field alone when
omitted, so a caller can change one field without restating the rest. The
`<id>` resolves with the same rules as `task show`, so a record in any bucket
is reachable.

| Flag | Semantics |
| ---- | --------- |
| `-t, --title` | Replace the title. |
| `-p, --priority` | Replace the priority. |
| `--effort` | Replace the effort. |
| `--add-label` | Add a label the records already use; comma-separated or repeatable. |
| `--remove-label` | Remove a label; comma-separated or repeatable. |
| `--external-ref` | Replace the external reference; an empty value clears it. |
| `--parent` | Replace the parent task ID. The parent must exist and be top-level. |
| `--clear-parent` | Remove the parent task ID. |
| `-b, --body` | Replace the body, or one section when `--section` is set. |
| `-F, --body-file` | Read the replacement body from a file, or `-` for stdin. |
| `--section <name>` | Scope `--body`/`--body-file` to one `##` section instead of the whole body. |

**Guarantees:**

- Labels are additive: `--add-label` and `--remove-label` adjust the set and
  never clobber labels the caller did not name. `--labels` remains create-only.
  Each accepts a comma-separated list and may be repeated. `-` and `[]` are the
  empty-list sentinels of the front-matter format, not labels, and are refused.
- `--add-label` accepts only labels already in use by some record, which is the
  vocabulary `ahm task labels` reports; an unknown label is a usage error that
  names the label. A caller introducing a genuinely new label does so through
  `task create --labels`, the only path that extends the vocabulary.
  `--remove-label` is not checked: removing a label the record does not carry is
  a no-op, so idempotent cleanup does not fail. A corpus that carries no labels
  at all has an empty vocabulary, so an addition is accepted unchecked.
- `--section <name>` replaces exactly that heading section, creating it at the
  end of the body when absent, and leaves every other section byte-identical.
  The heading is matched case-insensitively at `##` or `###`. The name must be
  non-empty, must not contain newlines, and must not contain `#`, since the
  name is spliced into a `## <name>` heading.
  A `--section` replacement builds the whole resulting body and passes it
  through the same audit-trail guard as a whole-body replacement, so replacing a
  section that contains a nested protected heading cannot delete it. Supplying
  text that introduces a `## Comments` or `## Cancellation Reason` heading is
  not refused; the guard protects existing provenance, it does not police the
  headings a caller writes.
- `edit` never moves a record between buckets and never rewrites `status` or
  `depends_on`; `task accept|start|complete|cancel|reopen|block|unblock` and
  `task dep add|remove` remain the only writers of those fields and keep their
  guards.
- `edit` has no `--status` and no `--depends-on` flag.
- A mutation flag whose value already matches the record prints
  `<id> unchanged`, writes nothing, and exits 0.
- A write regenerates the task indexes. `edit` re-resolves its target inside the
  workflow record lock, so a concurrent update that lands before the lock is
  acquired is preserved.
- `--dry-run` prints the record path and a per-field diff (`<id> priority: P2 ->
  P1`) and writes nothing. `--json` returns the record plus a `changed` array of
  the field names that changed, empty on the unchanged path.

**Refusals (exit code 2, `--force` where noted):**

- `task edit <id>` with no mutation flag. The message lists the available
  flags. There is no editor fallback: no code path reads `EDITOR` or `VISUAL`.
- `--body` with `--body-file`, `--section` with neither, `--parent` with
  `--clear-parent`, an empty `--title`, a newline in `--title` or
  `--external-ref`, and an unsupported `--priority` or `--effort`.
- `--add-label <label>` where no record carries `<label>`. The message names the
  label and points at `ahm task labels` and `task create --labels`. `--force`
  does not override this; it is a usage error, not a write guard, and it is
  checked in `--dry-run` too.
- `--section Comments` and `--section "Cancellation Reason"`. Those sections
  are owned by `task comment` and `task cancel` respectively, and the message
  names the owning command. `--force` does not override this.
- A `--body`, `--body-file`, or `--section` replacement that does not carry over
  the content of a `## Comments` or `## Cancellation Reason` section the record
  already has is refused unless `--force` is passed. The rule is: some section of
  the same name at the same heading depth in the resulting body must contain the
  current section's text, compared with whitespace runs collapsed. A missing
  heading, an emptied heading, rewritten or truncated text, and a `###`
  demotion all count as a drop. Re-wrapping, re-indenting, or moving a preserved
  section within the body is allowed. A record that has no content in a
  protected section is not constrained at all.
  Known limit of this check, which needs a whole-body rewrite to reach: it does
  not require the carrying section to be the *first* one of that name, so a
  forged section placed ahead of the real log is accepted; and it compares by
  substring, so text that merely contains the original passes.
- A `--body` whose value equals the current body, and a `--section` whose
  content already matches, are no-ops: they report `unchanged` and write
  nothing.

**Text output:**

```text
$ ahm task edit 270 --priority P1 --effort S
270 updated (priority, effort)

$ ahm task edit 270 --priority P1
270 unchanged
```

**Dry-run output:**

```text
$ ahm --dry-run task edit 270 --priority P1
270 edit: store:tasks/active/270.md
270 priority: P2 -> P1
```

### `task list` / `task ls`

Lists parsed tasks.

**Guarantees:**

- `--status <status>`: filters by one or more statuses. Comma-separated list
  or repeated flags. Case-insensitive. Accepts `in-progress` for `In Progress`.
  Default: all statuses.
- `--label <label>`: filters by label. Comma-separated or repeated. AND logic
  across labels.
- `--priority`, `--effort`: filter by enum value.
- `--sort <field>`, `--reverse`: see shared sorting above.
- `--json`: emits parsed task structs with lowercase snake_case keys.
- `--plain`: compact JSON.

### `task ready`

Lists Pending tasks with all dependencies satisfied, plus Tracking tasks with
at least one child whose child tasks are all Completed or Cancelled and whose
own dependencies are satisfied (the tracker itself is the only remaining
work), sorted by priority.

**Guarantees:**

- Same `--label`, `--sort`, `--reverse`, `--json`, and `--plain` flags as
  `task list`.

### `task blocked`

Lists the blocked queue sorted by priority: tasks whose status is `Blocked`,
plus `Pending` tasks with an incomplete dependency.

**Guarantees:**

- Same presentation flags as `task list`.
- In text output each task is followed by a `reason:` line. A `Blocked` task
  reports its `blocked_reason` (with `blocked_ref` when present, or `no reason
  recorded` when the field is empty); a `Pending` task reports the dependency it
  is waiting on.
- `--json` emits the parsed task structs, which carry `blocked_reason` and
  `blocked_ref` directly.

### `task next`

Prints the single highest-priority ready task (or nothing).

### `task show <id> [<id>...]`

Shows one or more tasks. With a single ID, default output is the raw Markdown
file; with several IDs, each file follows the previous one, separated by `---`.
`--json` emits one object for a single ID and an array for several.

### `task search <query>`

Searches tasks by case-insensitive substring match on the title or body.
Title matches are listed before body-only matches; within each group the
result order is unchanged. Supports the `--status` and `--label` filters to
scope results. Line format matches `task list`, and the text line does not mark
which field matched, so `--json` consumers should read `body` to tell.

### `task labels`

Lists all unique labels across all tasks with per-label counts.

### `task start <id>`

Sets task status to `In Progress` and moves the file to the `active/` bucket.

**Guarantees:**

- Prints `<id> already In Progress` and writes nothing when status and bucket
  already match.
- No transition guard: starting a `Completed` or `Cancelled` task moves it
  back to `active/`. Use `task reopen` to return it to `Pending` instead.

### `task complete <id>`

Sets task status to `Completed`.

**Guarantees:**

- Strict acceptance (when enabled): fails if acceptance section missing,
  contains `- [ ] TODO`, or has unchecked items. Override with `--force`.
- Requires every `depends_on` entry to be `Completed`; an incomplete
  dependency fails with `cannot complete task <id>: incomplete dependencies:
  ...`.
- Moves active `Blocked` tasks that depend on the completed ID to `Pending`
  when that completion satisfies their whole `depends_on` list, clearing their
  `blocked_reason` and `blocked_ref`; the auto-unblock writes no reason of its
  own.
- Prints `<id> -> Completed` or `<id> already Completed`.

### `task cancel <id> --reason <text>`

Sets task status to `Cancelled`.

**Guarantees:**

- `--reason` is required and must be non-empty; `--force` does not bypass it.
  The reason is stored in the body under `## Cancellation Reason`.
- Moves the file to the `cancelled/` bucket. There is no transition guard, so
  a `Completed` task can be cancelled this way.
- Leaves dependents untouched; only `task complete` unblocks them.

### `task reopen <id>`

Returns a `Completed` or `Cancelled` task to `Pending`.

### `task block <id> --reason <text> [--ref <text>]`

Sets task status to `Blocked` and records why in front matter: the required
`blocked_reason` and the optional `blocked_ref`.

**Guarantees:**

- `--reason` is required and must be non-empty after trimming; `--force` does
  not bypass it, matching `task cancel`.
- The task may be in any non-terminal status (`Open`, `Pending`, `In Progress`,
  or already `Blocked`); a `Completed` or `Cancelled` task is refused as a
  usage error (exit 2). Blocking an already-blocked task rewrites the record so
  the reason can be corrected.
- `--ref` records an external reference, such as an issue URL or a person,
  beside the reason. It is not a substitute for `--reason`. Blocking a task
  replaces both fields, so re-blocking to correct the reason clears a
  previously recorded `--ref` unless it is supplied again.
- `--reason` and `--ref` must not contain a newline or carriage return; both
  are single-line front-matter scalars.
- `blocked_reason` and `blocked_ref` are present only while a task is
  `Blocked`; every status transition away from `Blocked` clears them.
- `--dry-run` prints the pending move, `status: Blocked`, and the reason (and
  the ref when given) without writing.

### `task unblock <id>`

Returns a `Blocked` task to `Pending` and clears its recorded block reason.

**Guarantees:**

- The task must currently be `Blocked`; any other status is a usage error
  (exit 2) naming the current status.
- This is distinct from the automatic unblock that `task complete` performs
  when a task's dependencies are satisfied. Either path clears `blocked_reason`
  and `blocked_ref`.

### `task accept <id>`

Accepts an `Open` task into the ready queue by setting its status to
`Pending`.

### `task comment <id> <text>`

Appends a timestamped comment under `## Comments` in the task body. Creates
the section if missing.

### `task dep add|remove <id> <dependency-id>`

Adds or removes a task dependency.

**Guarantees:**

- `add` refuses cycles (detected before writing).
- Prints `<id> depends_on: <deps>` on change, or `<id> already depends on <dep>`
  / `<id> does not depend on <dep>` when no change needed.
- `--dry-run` prints the new dependency set without writing.

### `task dep cycles`

Prints dependency cycles for non-completed, non-cancelled tasks.

### `task dep tree <id>`

Prints the dependency tree rooted at `<id>` for non-completed, non-cancelled
tasks. The tree is rendered as a DAG: each node is fully expanded the first
time it is encountered; subsequent references render as back-references to
avoid exponential re-expansion of shared subtrees.

**Text output:**

```text
001 [Pending] Root task
  002 [Pending] Dependency
    003 [Pending] Sub-dependency
      002 [already expanded]
  999 [missing]
```

- Missing tasks show the ID followed by `[missing]`.
- Cycles show `cycle to <id>` (unchanged from previous behavior).
- Tasks that were already expanded elsewhere in the tree show `[already expanded]`.

**JSON / plain output:**

- The first occurrence of each node is a full `depTreeNode` with `id`, `title`,
  `status`, and `dependencies`.
- Subsequent non-cycle occurrences render as a stub node containing only `id`,
  `title`, and `status` (no `dependencies`), signalling that the full subtree
  appears earlier in the output.
- Missing tasks render as `{"id": "<id>"}` with no other fields.
- Cycle occurrences render as a stub node (same shape as a back-reference).

JSON uses `--json` (pretty-printed with 2-space indent); `--plain` produces
compact JSON. The structural shape is the same in both modes.

**Guarantees:**

- Output size grows linearly in node count (not exponentially) for DAG-shaped
  dependency graphs. Each node is visited once for full expansion; each
  additional edge adds at most one stub back-reference.
- Cycle detection is unchanged: a cycle is detected when a node appears twice
  in the current expansion path and renders as `cycle to <id>` in text or a
  stub node in JSON/plain.

**Examples:**

```shell
  ahm task dep tree 002
  ahm --json task dep tree 002
  ahm --plain task dep tree 002
```

## Bulk Import

`ahm task import --from-file <path>` reads a JSON array of task objects. Use
`--from-file -` for stdin. This command creates records offline and never
fetches external issues or changes the store registry. An empty array succeeds
without changing records, indexes, or task ID mark state.

```json
[
  {
    "ref": "child",
    "title": "Implement parser",
    "status": "Pending",
    "priority": "P1",
    "effort": "M",
    "labels": "type:feature, area:cli",
    "parent": "@tracker",
    "depends_on": ["@foundation", "012"],
    "created": "2024-01-02T03:04:05Z",
    "external_ref": "https://github.com/example/project/issues/42",
    "body": "## Summary\n\nImplement the parser.\n\n## Comments\n\n**2024-01-03T03:04:05Z** — _Trav_: Reviewed."
  },
  {"ref": "tracker", "title": "Parser migration", "status": "Tracking"},
  {"ref": "foundation", "title": "Build foundation", "status": "Pending"}
]
```

`title` is required. `status`, `priority`, `effort`, and `labels` default to
`Open`, `P2`, `S`, and `type:task, area:unknown` when omitted or empty. Enum
values use the canonical spelling listed above. `body` is Markdown below the
H1; a matching H1 is removed, CRLF is normalized, and outer whitespace is
trimmed by the record renderer. An omitted body is empty. Include comments and
cancellation reasons as Markdown body sections; they have no separate JSON
fields. `created` accepts an RFC3339 timestamp and defaults to import time;
`updated` is always import time. `external_ref` is optional and is never
invented. `labels` is a comma-separated string, and `depends_on` is an array of
strings. Objects and fields must not be JSON `null`. Titles, labels, and external
references must have no newlines or outer whitespace. Unknown fields and
duplicate field names are rejected. Duplicate detection is case-insensitive,
matching the JSON decoder: `title` and `TITLE` in the same object are duplicates,
including escaped spellings of the same name. `id`, `updated`, `path`, `bucket`, and
unknown front matter (`extra`, including retired `exec_plan`) are generated or
not importable. `blocked_reason` and `blocked_ref` are not importable either:
an imported `Blocked` record carries no reason and warns under `task_blocked_missing_reason`
until one is recorded with `ahm task block`.

`ref` is an optional unique, case-sensitive name scoped to this document; it
cannot contain whitespace or `@` and is not stored in the task. `@name` in
`parent` or `depends_on` denotes a record in this batch, including a forward
reference. Unprefixed numeric task IDs resolve against existing records using
the normal ID resolution rules; they never denote IDs being allocated in this
batch. Missing, ambiguous, and duplicate-existing-ID references are refused.
A parent must be top-level. Top-level IDs are allocated in document order using
the existing allocator; child IDs are then allocated in document order under
their resolved parents, including forward parents. The 26-child limit applies.
Dependencies are deduplicated and sorted. Completed dependencies are accepted
for historical imports; active tasks cannot depend on cancelled tasks. Self
references and cycles among active tasks are refused.

The whole batch is validated before writing, and every semantic refusal is
reported in one response (exit 1). Malformed JSON, unknown or duplicate fields,
invalid field types, and an invalid document shape exit 2. An unreadable file or
store exits 1.
Unparseable existing task records prevent importing until repaired. Refusal
creates no records or index/state writes. A real import acquires the record
lock once; home mode also holds the store-state lock across writes and recovery.
Records go to the bucket matching their status, the task ID marks advance past
the allocated top-level IDs and child letters, and indexes are generated once.
The command does not apply lifecycle transitions to existing dependents or
tracker parents.

Ordinary write failures restore affected records, indexes, and state bytes.
If restoration fails, the error names recovery paths. Process termination or
power loss can leave a partial batch: individual file writes remain atomic,
but there is no crash-atomic batch transaction. Inspect the planned paths,
remove only records from the failed batch, and run `ahm index` before retrying;
retain any raised mark after an interrupted run. A successful import is
additive: repeating it creates another batch, even with the same external refs.

`--dry-run` writes nothing and takes neither lock. It reports each allocated
ID, record path, parent, dependency edge, and refusal. The preview is an
observation; a concurrent command can change the IDs a later real run allocates.
Relative Markdown links are preserved and remain the importer's responsibility;
`ahm --check links status` reports missing targets after import. Completed
records should include checked Acceptance Notes to avoid normal validation
warnings. Import does not refuse body link or acceptance findings.

Text, JSON, and plain output use the shared emitter. JSON is an object with
`dry_run` and `records`; plain is the same object as one compact JSON line.
Each record outcome has `ref`, `id`, `path`, `parent`, `depends_on`, `outcome`,
and `errors`, in input order. Empty strings denote absent ref, parent, or an
unallocatable ID/path; arrays remain present when empty. Outcomes are `planned`
for a successful preview, `imported` for a successful write, `refused` for a
record with semantic errors, and `not_imported` for another record in a refused
batch. Runtime write failures print an error to stderr and no success report.
