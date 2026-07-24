# ahm Task Commands

This reference covers task lifecycle, dependency, delegation, completion,
cancellation, and reopening commands. For task file grammar and validation
finding codes, see [task file and validation formats](task-file-format.md).

Exhaustive flag details live in `ahm <command> --help`. This page documents
only compatibility guarantees that generated help cannot express.

## Task Record Locations

Task files live under the current record layout:

- `.agents/` in legacy committed-record repositories.
- `.ahm/` after migration.

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

Task resolution commands (`task show`, `task groom`, `task work`, `task start`,
`task complete`, `task cancel`, `task accept`, `task reopen`, `task comment`,
`task dep add`, `task dep remove`) skip malformed files during ID resolution.
A malformed task cannot be resolved and produces a `task not found` error.

Validation commands (`ahm status`, `ahm doctor`) are strict: they report
malformed task files as `task_malformed` validation errors and exit code 1.

## Per-Command Guarantees

All task commands regenerate indexes after mutations unless stated otherwise.
All support `--dry-run` for previewing write operations.

### `task create <title> [flags]`

Creates a new task and regenerates indexes.

**Top-level ID allocation:** next zero-padded numeric ID after the highest
existing numeric task ID (`001`, `002`, ...). Non-numeric suffix IDs are
ignored for this calculation.

**Subtask (child) ID allocation:** When `--parent <id>` is provided, next
available lettered child ID under that parent (`137a`, `137b`, ...). At most
26 children per parent. Scans across `active/`, `completed/`, `cancelled/`
buckets to avoid collisions.

Concurrent creates are serialized with a repo-local workflow lock.

**Guarantees:**

- `--body-file` provides full body content below the H1; ahm owns ID, front
  matter, heading, location, and index regeneration.
- `--body-file` and `--description` are mutually exclusive.
- Title and `--labels` must not contain leading/trailing whitespace, newlines,
  or carriage returns. Empty labels canonicalize to `-`.
- `--dry-run` prints the target path and ID without creating.

### `task list` / `task ls`

Lists parsed tasks.

**Guarantees:**

- `--status <status>`: filters by one or more statuses. Comma-separated list
  or repeated flags. Case-insensitive. Accepts `in-progress` for `In Progress`.
  Default: all statuses.
- `--label <label>`: filters by label. Comma-separated or repeated. AND logic
  across labels.
- `--priority`, `--effort`: filter by enum value.
- `--search <text>`: case-insensitive substring match against title, body,
  labels, and comments.
- `--by-id <id>`: exact ID match.
- `--sort <field>`, `--reverse`: see shared sorting above.
- `--json`: emits parsed task structs with lowercase snake_case keys.
- `--plain`: compact JSON.

### `task ready`

Lists Open or Pending tasks with all dependencies satisfied, sorted by
priority.

**Guarantees:**

- Same `--sort`, `--reverse`, `--json`, `--plain`, `--search` flags as
  `task list`.

### `task blocked`

Lists Blocked tasks sorted by priority.

**Guarantees:**

- Same presentation flags as `task list`.

### `task next`

Prints the single highest-priority ready task (or nothing).

### `task show <id>`

Shows one task. Default: raw Markdown file. `--json` / `--plain`: parsed task
record.

### `task search <text>`

Searches tasks by text. Equivalent to `task list --search <text>`.

### `task labels`

Lists all unique labels across all tasks with per-label counts.

### `task start <id>`

Sets task status to `In Progress`.

**Guarantees:**

- Refuses transition from `Completed` or `Cancelled` (use `task reopen`).
- Prints `<id> already <status>` if already in progress.

### `task complete <id>`

Sets task status to `Completed`.

**Guarantees:**

- Strict acceptance (when enabled): fails if acceptance section missing,
  contains `- [ ] TODO`, or has unchecked items. Override with `--force`.
- Refuses transition from `Cancelled`.
- Prints `<id> -> Completed` or `<id> already Completed`.

### `task cancel <id> [reason]`

Sets task status to `Cancelled`.

**Guarantees:**

- Optional reason appended to task body.
- Refuses transition from `Completed`.
- Unblocks dependents when they have no remaining dependencies.

### `task reopen <id>`

Returns a `Completed` or `Cancelled` task to `Pending`.

### `task accept <id>`

Sets task status to `Completed` from `In Progress` or `Pending`. Equivalent to
`task complete` for that transition subset.

### `task comment <id> <text>`

Appends a timestamped comment under `## Comments` in the task body. Creates
the section if missing.

### `task groom <id> [flags]`

Delegates structured backlog grooming of a task to a supported coding-agent
CLI.

**Guarantees:**

- Agent selection, `--agent`, `--model`, `--timeout` match `task work`.
- `--no-<step>` flags skip individual grooming steps.
- `--dry-run` prints the prompt without delegation.
- Requires `Open` status.

### `task dep add|remove <id> <dependency-id>`

Adds or removes a task dependency.

**Guarantees:**

- `add` refuses cycles (detected before writing).
- Prints `<id> depends_on: <deps>` on change, or `<id> already depends on <dep>`
  / `<id> does not depend on <dep>` when no change needed.
- `--dry-run` prints the new dependency set without writing.

### `task dep cycles`

Prints dependency cycles for non-completed, non-cancelled tasks.

### `task dep tree`

Prints dependency trees for non-completed, non-cancelled tasks.

### `task work <id> [flags]`

Delegates a resolved task to an external coding-agent CLI.

**Guarantees:**

- Validates task state before delegation.
- Review and commit run by default. `--no-review` / `--no-commit` to opt out.
- Session ID captured from agent stderr for resume.
- Supported agents: `cake`, `claude`, `codex`, `cursor`.
- `--agent <name>` selects agent; `--model <name>` overrides the model.
- `--timeout <duration>` sets the agent timeout.
- `--dry-run` prints the invocation without executing.

### `task migrate [flags]`

Migrates task metadata to the current format.

**Guarantees:**

- `--dry-run` prints grouped migration changes.
- Idempotent: re-running after migration reports no changes.
