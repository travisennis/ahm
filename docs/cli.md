# ahm CLI Reference

This document describes the supported `ahm` commands, flags, outputs, and
write behavior. The executable entrypoint is `cmd/ahm/main.go`; the command
implementation lives in `internal/ahm/cli.go`.

## Usage

```bash
ahm [global flags] <command> [command flags]
```

When no command is provided, `ahm` runs `status`.

Exit codes:

- `0`: success.
- `1`: runtime failure.
- `2`: invalid usage, such as an unknown flag or missing required argument.

## Root Selection

Most commands operate on a target repository root.

By default, `ahm` walks upward from the current working directory until it finds
a `.git` directory. If no `.git` directory is found, it uses the current working
directory.

Use `--root <path>` to bypass auto-detection and operate on a specific
directory.

## Global Flags

Global flags must appear before the command.

| Flag | Description |
| ---- | ----------- |
| `--root <path>` | Sets the target repository root. Defaults to the nearest git root or the current directory. |
| `--json` | Emits structured JSON for commands that use the shared emitter. For task list/show commands, this returns parsed task structs. |
| `--plain` | Emits stable line-oriented output for shared-emitter responses by printing compact JSON on one line. Ignored by commands with custom text output. |
| `--quiet` | Parsed and reserved for quieter output; no current command changes behavior based on it. |
| `--verbose` | Parsed and reserved for verbose output; no current command changes behavior based on it. |
| `--dry-run` | Previews supported write operations without writing files. Supported by `init`, `upgrade`, `index`, `task create`, `task migrate`, task status transitions, and task dependency add/remove. |
| `--force` | Forces supported overwrites during `init` and `upgrade`. It never forces overwriting an existing `AGENTS.md`. |
| `--help`, `-h` | Prints command help. |
| `--version` | Prints the embedded workflow template version. |

Examples:

```bash
ahm --root /path/to/repo status
ahm --json doctor
ahm --dry-run upgrade
```

## Output Modes

`--json` takes precedence over `--plain`.

Without either flag, most structured commands currently print indented JSON.
Install and upgrade operations print grouped text sections such as `created:`,
`updated:`, `skipped:`, and `conflicts:`.

Some task commands use command-specific text output:

- `task create` prints the created task ID.
- `task list`, `task ready`, and `task blocked` print one task per line.
- `task show` prints the task Markdown file unless `--json` is used.
- `task migrate --dry-run` prints grouped task migration changes.
- Task status transitions print `<id> -> <status>`.
- Dependency updates print `<id> depends_on: <dependencies>`.
- Dependency tree and cycle commands print tree/path text.

## Commands

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

Prints the embedded workflow template version.

Alias:

- `--version`

Example:

```bash
ahm version
```

### `init`

Installs the managed `.agents` workflow into the target root.

`init` creates missing managed workflow files, workflow directories, metadata,
and generated indexes. Existing managed files are skipped unless `--force` is
used. `AGENTS.md` is create-only: it is created when missing, but an existing
`AGENTS.md` is always skipped, even with `--force`.

Writes:

- Managed templates listed by `internal/templates/templates.go`.
- `.agents/ahm.json` metadata.
- Generated index files under `.agents/.tasks/`, `.agents/.research/`, and
  `.agents/exec-plans/`.
- Workflow directories under `.agents/` and `docs/adr/`.

Useful flags:

- `--dry-run`: prints files, directories, metadata, and indexes that would be
  written.
- `--force`: overwrites existing managed files, except create-only files.
- `--json` or `--plain`: changes the emitted install summary format.

Example:

```bash
ahm init
ahm --dry-run init
```

### `upgrade`

Updates managed workflow files from the embedded templates.

`upgrade` compares `.agents/ahm.json` hashes with files in the target root.
Files that still match their recorded managed hash are updated. Locally modified
managed files are preserved and reported as conflicts. Missing managed files are
created. Generated indexes are regenerated.

`AGENTS.md` remains create-only and is never overwritten.

Useful flags:

- `--dry-run`: previews all supported writes.
- `--force`: replaces locally modified managed workflow files with embedded
  templates. This does not apply to `AGENTS.md`.
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
- Installed workflow version from `.agents/ahm.json`.
- Task counts by status.
- Validation errors and warnings.

Validation checks include missing metadata, missing managed files, unreadable
managed files, untracked managed files, locally modified managed files, malformed
task front matter, missing task dependencies, and active dependency cycles.

Useful flags:

- `--json`: prints indented JSON.
- `--plain`: prints compact JSON.

Example:

```bash
ahm status
ahm
```

### `doctor`

Reports environment and workflow checks.

The report includes:

- Target root.
- Whether `go` is available on `PATH`.
- Whether `git` is available on `PATH`.
- Whether workflow metadata is installed.
- Installed and embedded workflow template versions.
- The same workflow validation report used by `status`.

Useful flags:

- `--json`: prints indented JSON.
- `--plain`: prints compact JSON.

Example:

```bash
ahm doctor
```

### `index`

Regenerates generated task, research, and ExecPlan indexes.

Writes:

- `.agents/.tasks/index.md`
- `.agents/.tasks/active/index.md`
- `.agents/.tasks/completed/index.md`
- `.agents/.tasks/cancelled/index.md`
- `.agents/.research/index.md`
- `.agents/exec-plans/active/index.md`
- `.agents/exec-plans/completed/index.md`

The root index includes status counts, the next ready queue, blocked/open tasks,
and an all-task table. Bucket indexes include task tables for their bucket. The
research and ExecPlan indexes link Markdown files from their source folders.

Useful flags:

- `--dry-run`: prints the index paths that would be written.

Example:

```bash
ahm index
ahm --dry-run index
```

## Task Commands

Task commands operate on Markdown task files under:

- `.agents/.tasks/active/`
- `.agents/.tasks/completed/`
- `.agents/.tasks/cancelled/`

Task IDs are resolved by exact match first. If no exact match is found, numeric prefix matching is used: `1` matches `001`, `01` matches `001`, and `10` matches `010`. Suffixed IDs like `1a` match `001a`. If a prefix matches more than one task, the command lists the matching IDs and fails as ambiguous.

Task statuses must be one of:

- `Open`
- `Pending`
- `In Progress`
- `Blocked`
- `Tracking`
- `Completed`
- `Cancelled`

Task priorities must be one of:

- `P0`
- `P1`
- `P2`
- `P3`
- `P4`

Task efforts must be one of:

- `XS`
- `S`
- `M`
- `L`
- `XL`

### `task create <title> [flags]`

Creates a new active task and regenerates indexes.

The next ID is the next zero-padded numeric ID after the highest existing
numeric task ID, such as `001`, `002`, and `003`. Non-numeric suffix IDs are
ignored for this calculation.

The title is built from all non-flag arguments, so both of these are valid:

```bash
ahm task create "Add release workflow" --priority P1
ahm task create Add release workflow --priority P1
```

Command flags:

| Flag | Description |
| ---- | ----------- |
| `--priority <value>`, `-p <value>` | Sets task priority. Default is `P2`. |
| `--effort <value>` | Sets task effort. Default is `S`. |
| `--labels <value>` | Sets the raw labels front matter value. Default is `type:task, area:cli`. |
| `--status <value>` | Sets initial task status. Default is `Pending`. |
| `--description <text>`, `-d <text>` | Sets the initial summary body. Default is `TODO.` |

The created task has `exec_plan: -`, no dependencies, a `## Summary` section,
and a `## Acceptance Notes` checklist.

Useful global flags:

- `--dry-run`: prints the target path and ID without creating the task.
- `--json` or `--plain`: affects only dry-run output. Successful non-dry-run
  creation prints the task ID.

Example:

```bash
ahm task create "Add release workflow" --priority P2 --effort M --labels type:task,area:ci
```

### `task list`

Lists all parsed tasks.

Alias:

- `task ls`

Text output is sorted by priority rank and then task ID:

```text
001 [Pending] P2 S Add release workflow
```

Useful flags:

- `--json`: emits parsed task structs.

Example:

```bash
ahm task list
```

### `task ready`

Lists pending tasks whose dependencies are all completed.

Completed dependencies are determined by dependency task status, not by file
bucket alone.

Useful flags:

- `--json`: emits parsed task structs.

Example:

```bash
ahm task ready
```

### `task blocked`

Lists blocked tasks.

A task is considered blocked when either:

- Its status is `Blocked`.
- Its status is `Pending` and at least one dependency is not completed.

Useful flags:

- `--json`: emits parsed task structs.

Example:

```bash
ahm task blocked
```

### `task migrate`

Normalizes legacy task front matter for projects that used an ahm-like workflow
before adopting the current ahm schema.

The migration is intentionally mechanical. It can:

- Add missing `labels` as `type:task, area:unknown`.
- Convert placeholder `priority: -` to `priority: P3`.
- Convert placeholder `effort: -` to `effort: M`.
- Trim annotated effort values such as `XL (split into subtasks)` to `XL`.
- Trim annotated dependency entries that start with task IDs, such as
  `050 (Backend abstraction), 051 (Tool abstraction)`, to `050, 051`.
- Convert legacy dependency notes such as `Follows 061` or `Completed by 061`
  to their referenced task IDs.

Source-only dependency notes such as `From code review...`, `Resolved in same
PR...`, `Research: ...`, and `Closed as obsolete...` are cleared to `-`.

Useful global flags:

- `--dry-run`: prints the task files and field changes without writing.
- `--json` or `--plain`: emits the migration report in machine-readable form.

Example:

```bash
ahm --dry-run task migrate
ahm task migrate
```

### `task show <id>`

Shows a task.

By default, this prints the raw task Markdown file. With `--json`, it prints the
parsed task struct.

Example:

```bash
ahm task show 001
ahm --json task show 001
```

### `task start <id>`

Sets a task status to `In Progress`, keeps it in the active bucket, and
regenerates indexes.

Useful flags:

- `--dry-run`: previews the target path and status without writing.

Example:

```bash
ahm task start 001
```

### `task complete <id>`

Sets a task status to `Completed`, moves it to
`.agents/.tasks/completed/<id>.md`, removes the old file when the bucket changed,
and regenerates indexes.

Alias:

- `task close <id>`

Useful flags:

- `--dry-run`: previews the target path and status without writing.

Example:

```bash
ahm task complete 001
```

### `task cancel <id>`

Sets a task status to `Cancelled`, moves it to
`.agents/.tasks/cancelled/<id>.md`, removes the old file when the bucket changed,
and regenerates indexes.

Useful flags:

- `--dry-run`: previews the target path and status without writing.

Example:

```bash
ahm task cancel 001
```

### `task reopen <id>`

Sets a task status to `Pending`, moves it to
`.agents/.tasks/active/<id>.md`, removes the old file when the bucket changed,
and regenerates indexes.

Useful flags:

- `--dry-run`: previews the target path and status without writing.

Example:

```bash
ahm task reopen 001
```

### `task dep add <id> <dependency-id>`

Adds a dependency to a task, rewrites the task file, and regenerates indexes.

Both IDs use normal task resolution. Dependencies are stored by canonical task
ID, deduplicated, and sorted by task ID.

Useful flags:

- `--dry-run`: previews the resulting dependency list without writing.

Example:

```bash
ahm task dep add 002 001
```

### `task dep remove <id> <dependency-id>`

Removes a dependency from a task, rewrites the task file, and regenerates
indexes.

Alias:

- `task dep rm <id> <dependency-id>`

Useful flags:

- `--dry-run`: previews the resulting dependency list without writing.

Example:

```bash
ahm task dep remove 002 001
```

### `task dep tree <id>`

Prints a dependency tree rooted at a task.

Missing dependencies are printed as `[missing]`. Cycles are detected during tree
walking and printed as `cycle to <id>`.

Example:

```bash
ahm task dep tree 002
```

### `task dep cycles`

Prints active dependency cycles.

Tasks with status `Completed` or `Cancelled` are excluded from cycle detection.
When no cycles are found, the command prints `No dependency cycles found`.

Example:

```bash
ahm task dep cycles
```

## Task File Format

`ahm` parses simple YAML-like front matter. Required task fields for validation
are:

- `id`
- `title`
- `status`
- `priority`
- `effort`
- `labels`
- `exec_plan`
- `depends_on`

Optional front matter preserved by task rewrites:

- `created`
- `updated`
- `parent`
- `external_ref`

`depends_on` accepts `-`, `[]`, or a comma-separated list. Rewrites use `-` for
an empty dependency list and comma-separated IDs for non-empty lists.

Task rewrites preserve the parsed body after the top-level task heading. They
rewrite front matter in `ahm`'s canonical order.

## Validation Findings

`status` and `doctor` can emit these validation codes:

| Code | Meaning |
| ---- | ------- |
| `metadata_missing` | `.agents/ahm.json` is missing or unreadable. |
| `managed_file_missing` | A managed workflow file is missing. |
| `managed_file_unreadable` | A managed workflow file could not be read. |
| `managed_file_untracked` | A managed workflow file exists but is not recorded in metadata. |
| `managed_file_modified` | A managed workflow file hash differs from metadata. |
| `task_dir_unreadable` | A task bucket directory could not be read. |
| `task_unreadable` | A task file could not be read. |
| `task_missing_field` | Task front matter is missing a required field. |
| `task_malformed` | A task could not be parsed or has unsupported enum values. |
| `task_dependency_missing` | A task depends on an ID that does not exist. |
| `task_dependency_cycle` | Non-completed, non-cancelled tasks contain a dependency cycle. |
