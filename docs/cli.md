# ahm CLI Reference

This document describes the supported `ahm` commands, flags, outputs, and
write behavior. The executable entrypoint is `cmd/ahm/main.go`; command wiring
lives in `internal/ahm/cli.go`, with focused implementation files under
`internal/ahm/`.

## Usage

```bash
ahm [global flags] <command> [command flags]
```

When no command is provided, `ahm` runs `status`.

Exit codes:

- `0`: success.
- `1`: runtime failure. `status` and `doctor` use exit code 1
  when the workflow validation report contains errors, without printing
  `error:` to stderr.
- `2`: invalid usage, such as an unknown flag or missing required argument.

## Root Selection

Most commands operate on a target repository root.

By default, `ahm` walks upward from the current working directory until it finds
a `.git` directory or `.agents/ahm.json`. If neither is found, the command
fails with an error message that explains how to use `--root` or `ahm init`.

Use `--root <path>` to bypass auto-detection and operate on a specific
directory.

`init`, `upgrade`, and `agents suggestions` are lenient: they can run in any
directory. `init` creates the `.agents` workflow scaffolding, `upgrade`
refreshes it, and `agents suggestions` only prints advisory text. All other
commands require a managed repository (`.git` or `.agents/ahm.json`).

## Global Flags

Global flags must appear before the command.

| Flag | Description |
| ---- | ----------- |
| `--root <path>` | Sets the target repository root. Defaults to the nearest git root or `.agents/ahm.json` parent. Outside a managed repository, strict commands fail with remediation instructions; use `--root` to bypass auto-detection. |
| `--json` | Emits structured JSON for commands that use the shared emitter. For task list/show commands, this returns parsed task structs. Takes precedence over `--plain` and `--text`. |
| `--plain` | Emits stable line-oriented output for shared-emitter responses by printing compact JSON on one line. Ignored by commands with custom text output. Takes precedence over `--text`. |
| `--text` | Emits human-friendly text output. This is the default mode. The flag exists for explicit clarity in scripts but does not override `--json` or `--plain`. |
| `--dry-run` | Previews supported write operations without writing files. Supported by `init`, `upgrade`, `index`, `task create`, `task work`, `task migrate`, task status transitions, and task dependency add/remove. |
| `--force` | Forces supported overwrites during `init` and `upgrade`, and overrides strict acceptance checks during `task complete`. It never forces overwriting an existing `AGENTS.md`. |
| `--help`, `-h` | Prints command help. |
| `--version` | Prints the ahm binary version. |

Examples:

```bash
ahm --root /path/to/repo status
ahm --json doctor
ahm --dry-run upgrade
```

## Output Modes

ahm supports three output modes: text (default), JSON (`--json`), and compact
JSON (`--plain`). Precedence: `--json` takes priority over `--plain`, and
`--plain` takes priority over the default text mode. The `--text` flag selects
the default explicitly and does not override `--json` or `--plain`.

In the default text mode, structured commands such as `status` and `doctor`
print human-friendly key-value output:

```
root: /path/to/repo
template_version: 1.0.0
installed: true
installed_version: 1.0.0
tasks:
  total: 5
  pending: 2
  in_progress: 1
  completed: 2
validation:
  ok: true
  errors: 0
  warnings: 0
```

When the workflow metadata is missing (not yet installed), `installed_version`
shows as `none` in text mode and `null` in JSON/plain mode, and the
validation report includes the metadata error:

```
root: /path/to/repo
template_version: 1.0.0
installed: false
installed_version: none
tasks:
  total: 0
  pending: 0
  in_progress: 0
  completed: 0
validation:
{
    "ok": false,
    "errors": [
      {
        "code": "metadata_missing",
        "path": ".agents/ahm.json",
        "message": "workflow metadata is missing or unreadable"
      }
    ],
    "warnings": [],
    "info": []
  }
```

Install and upgrade operations always print grouped text sections such as
`adopted:`, `created:`, `updated:`, `skipped:`, and `conflicts:`.

Some task commands use command-specific text output regardless of the output
mode:

- `agents suggestions` prints advisory Markdown snippets unless `--json` is
  used.
- `task create` prints the created task ID.
- `task list`, `task ready`, `task blocked`, and `task next` print task lines.
- `task show` prints the task Markdown file unless `--json` is used.
- `task migrate --dry-run` prints grouped task migration changes.
- Task status transitions print `<id> -> <status>`; if the task already has the target status, prints `<id> already <status>` instead and skips writing.
- Dependency updates print `<id> depends_on: <dependencies>`; if the dependency is already present (add) or absent (remove), prints `<id> already depends on <dep>` or `<id> does not depend on <dep>` instead and skips writing.
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

Prints the ahm binary version. This is the release tag version injected at
build time, distinct from the embedded workflow template version shown in
`status` and `doctor`.

Alias:

- `--version`

Example:

```bash
ahm version
```

### `agents suggestions`

Prints advisory snippets that a project may consider adding to an existing
project-owned `AGENTS.md`. The suggestions are intentionally limited to
AHM-owned workflow routing and ownership boundaries: when to read task,
research, ExecPlan, and documentation workflow files; how to treat generated
indexes; and which task state moves should use `ahm` commands.

This command never writes `AGENTS.md`. It exists for repositories where
`AGENTS.md` already exists and `ahm init` or `ahm upgrade` correctly skip that
file. The intended workflow is for an agent or maintainer to run the command,
review the suggestions, and adapt any useful snippets into the existing
instructions.

By default, the command reads `AGENTS.md` under the target root when present and
omits exact suggestion blocks that already appear in the file. The matching is
lightweight and advisory; projects should still review the output.

Useful flags:

- `--all`: prints all suggestions, including blocks that appear present.
- `--json`: prints structured suggestion objects with `id`, `title`, `body`,
  and `present` fields.

Examples:

```bash
ahm agents suggestions
ahm agents suggestions --all
ahm --json agents suggestions
```

### `init`

Installs the managed `.agents` workflow into the target root.

`init` creates missing managed workflow files, workflow directories, metadata,
and generated indexes. Existing managed files are skipped unless `--force` is
used. Files that exist on disk but are not yet tracked in metadata are
auto-adopted when their content matches the template. `AGENTS.md` is create-only:
it is created when missing, but an existing `AGENTS.md` is always skipped, even
with `--force`.

Writes:

- Managed templates listed by `internal/templates/templates.go`.
- Workflow guides under `.agents/`, including task, research, ExecPlan, and
  documentation guidance.
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
managed files are preserved and reported as conflicts. Files that exist on disk
but are not yet tracked in metadata are auto-adopted when their content matches
the template. Missing managed files are created. Generated indexes are
regenerated.

The metadata `version` field always advances to the embedded template version,
even when conflicts exist. This means a partial upgrade (some files conflicted,
others updated) records the new template version so subsequent upgrades can
correctly identify already-updated files.

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
- Validation errors, warnings, and info findings for managed workflow files,
  task consistency, generated indexes, ExecPlan references and lifecycle
  checks, and scoped Markdown links.

Validation checks include missing metadata, missing managed files, unreadable
managed files, untracked managed files, locally modified managed files,
malformed task front matter, task bucket mismatches, missing task dependencies,
active dependency cycles, stale or missing generated indexes, task ExecPlan
reference issues, ExecPlan lifecycle coherence, and broken relative Markdown
links inside the managed workflow surface.

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

- `--dry-run`: prints only index paths that are missing or have stale content. A clean repository immediately after `ahm index` produces no output.

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

All task front matter fields are preserved during status transitions,
dependency edits, and other task mutations. Known fields (`id`, `title`,
`status`, `priority`, `effort`, `labels`, `exec_plan`, `depends_on`,
`created`, `updated`, `parent`, `external_ref`) are written in a fixed
order. Unknown fields such as `assignee`, `due`, `tags`, or `ticket` are
preserved and written in sorted key order after the known fields.

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

### Malformed Task Resilience

List-like commands (`task list`, `task ls`, `task ready`, `task blocked`,
`task next`, `task dep cycles`, `task dep tree`) and `ahm index` tolerate
malformed task files. When one or more task files cannot be parsed, these
commands skip the malformed files, produce output from the remaining valid
tasks, and print a warning to stderr.

`task create` also tolerates malformed task files: it warns on stderr and
still assigns the next available ID, scanning both parsed tasks and task
files on disk to avoid ID collisions.

Task resolution commands (`task show`, `task work`, `task start`,
`task complete`, `task cancel`, `task accept`, `task reopen`, `task dep add`,
`task dep remove`) also
skip malformed files during ID resolution. A malformed task cannot be
resolved by ID and produces a `task not found` error.

Validation commands (`ahm status`, `ahm doctor`) are strict: they report
malformed task files as `task_malformed` validation errors and exit with
code 1.

To recover from a malformed task file, inspect the file, fix the front
matter (missing required fields, invalid enum values, or parse errors
such as unsupported block scalars), and run `ahm status` or `ahm doctor`
to confirm the repair.

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
| `--labels <value>` | Sets the raw labels front matter value. Default is `type:task, area:unknown`. |
| `--status <value>` | Sets initial task status. Default is `Open`. |
| `--description <text>`, `-d <text>` | Sets the initial summary body. Default is `TODO.` |
| `--body-file <path>` | Reads the task body from a file, or from stdin when the path is `-`. |

By default the created task has `exec_plan: -`, no dependencies, a `## Summary`
section, and a `## Acceptance Notes` checklist.

`--body-file` provides the full Markdown body that appears after the generated
H1 title. `ahm` still owns ID allocation, front matter, the `# <title>` heading,
the active task location, and index regeneration; only the body content below
the H1 is taken from the file. The file content is whitespace-trimmed and CRLF
line endings are normalized to LF.

```bash
ahm task create "Cache Immutable Tool Definitions For Agent Turns" \
  --priority P2 \
  --effort M \
  --labels "type:task, area:agent, area:tools" \
  --body-file -
```

`--body-file` and `--description` are mutually exclusive. The command reports an
explicit error when the body file cannot be read, when stdin is requested but
unavailable, or when the resolved body is empty.

Useful global flags:

- `--dry-run`: prints the target path and ID without creating the task.
- `--json` or `--plain`: affects only dry-run output. Successful non-dry-run
  creation prints the task ID.

Example:

```bash
ahm task create "Add release workflow" --priority P2 --effort M --labels type:task,area:ci
```

### `task list`

Lists parsed tasks.

Alias:

- `task ls`

Text output is sorted by priority rank and then task ID:

```text
001 [Pending] P2 S Add release workflow
```

Useful flags:

- `--status <status>`: filters tasks by status, such as `Pending`,
  `Completed`, or `Cancelled`. Status matching is case-insensitive and accepts
  `in-progress` for `In Progress`.
- `--json`: emits parsed task structs.

Example:

```bash
ahm task list
ahm task list --status pending
ahm task list --status completed
ahm task list --status cancelled
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

### `task next`

Shows the first ready task by the same ordering used by `task ready`: priority
rank first, then task ID. A task is ready when its status is `Pending` and all
dependencies are completed.

Useful flags:

- `--json`: emits the parsed task struct, or `null` when no task is ready.

Example:

```bash
ahm task next
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

### `task accept <id>`

Sets a task status to `Pending`, keeps it in the active bucket, stamps
`updated`, and regenerates indexes. This is the intentional transition from
`Open` (newly captured, untriaged) into the ready backlog.

Useful flags:

- `--dry-run`: previews the target path and status without writing.

Example:

```bash
ahm task accept 001
```

### `task work <id> [flags]`

Resolves a task, validates that it can be worked, and hands it to an external
coding-agent CLI from the repository root.

`task work` refuses completed and cancelled tasks. It also verifies every task
listed in `depends_on` is already `Completed` before invoking an agent. If the
task is `Pending`, the command marks it `In Progress` and regenerates indexes
after validation and executable lookup, but before invoking the external CLI.
Tasks already `In Progress`, `Open`, or `Blocked` are not rewritten.

Supported agents:

| Agent | Executable | Invocation |
| ----- | ---------- | ---------- |
| `cake` | `cake` | `cake --output-format text <prompt>` |
| `codex` | `codex` | `codex exec <prompt>` |
| `cursor` | `cursor-agent` | `cursor-agent -p --output-format text <prompt>` |

Agent selection precedence is:

1. `--agent <cake|codex|cursor>`
2. `.agents/ahm.json` `"default_work_agent": "<agent>"`
3. `cake`

The generated prompt includes the resolved task ID and task path, and instructs
the delegated agent to read `AGENTS.md`, `.agents/TASKS.md`, the generated task
index, and the task file before making changes. `ahm` does not pass provider
credentials, choose models, parse provider output, run review orchestration,
complete tasks, commit changes, push branches, or open pull requests.

Useful flags:

- `--agent <cake|codex|cursor>`: selects the external coding-agent CLI.
- `--dry-run`: previews the selected executable, arguments, task ID, agent, and
  resulting status without rewriting the task or invoking the external CLI.

Repository configuration:

```json
{
  "default_work_agent": "codex"
}
```

Examples:

```bash
ahm task work 001
ahm task work 001 --agent codex
ahm --dry-run task work 001 --agent cursor
```

### `task complete <id>`

Sets a task status to `Completed`, moves it to
`.agents/.tasks/completed/<id>.md`, removes the old file when the bucket changed,
and regenerates indexes.

Before completing, `ahm` verifies that all task dependencies (listed in
`depends_on`) are already in `Completed` status. If any dependency is not
completed, the command returns an error listing the incomplete dependencies
and does not modify the task file or indexes.

Before moving the task, `ahm` also checks for an acceptance section. It accepts
`##` or `###` headings named `Acceptance Notes`, `Acceptance Criteria`, or
`Acceptance`, case-insensitively. Completion prints stderr warnings when the
section is missing, still contains the seeded `- [ ] TODO` placeholder, or has
unchecked `- [ ]` or `* [ ]` checklist items.

By default, incomplete acceptance notes warn but do not block completion. Set
`"strict_acceptance": true` in `.agents/ahm.json` to make those findings return
a non-zero error. The global `--force` flag overrides strict acceptance and
completes the task while still printing the warnings.

Alias:

- `task close <id>`

Useful flags:

- `--dry-run`: previews the target path and status without writing.
- `--force`: overrides `"strict_acceptance": true` for this completion.

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

`ahm` parses a strict YAML-like front matter grammar between `---` delimiters.
The grammar supports `key: value` pairs where keys are alphanumeric with
underscores, and values can be plain text or double-quoted strings. Comment
lines (lines starting with `#`) and blank lines are silently skipped.
Unsupported shapes — keys with spaces or colons, and block scalar indicators
(`|`, `>`) — produce `task_malformed` validation errors.

Required task fields:

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

`status` and `doctor` can emit validation findings in three tiers:

- `errors`: hard validation failures; these set `validation.ok` to `false` and
  make the command exit with code 1.
- `warnings`: workflow inconsistencies that should be fixed but do not change
  `validation.ok`.
- `info`: low-noise advisory findings that do not change `validation.ok`.

The JSON shape includes `errors`, `warnings`, and `info` arrays even when a tier
is empty.

Finding codes:

| Code | Meaning |
| ---- | ------- |
| `metadata_missing` | `.agents/ahm.json` is missing or unreadable. |
| `managed_file_missing` | A managed workflow file is missing. |
| `managed_file_unreadable` | A managed workflow file could not be read. |
| `managed_file_untracked` | A managed workflow file exists but is not recorded in metadata; run `ahm init` to adopt. |
| `managed_file_modified` | A managed workflow file hash differs from metadata. |
| `task_dir_unreadable` | A task bucket directory could not be read. |
| `task_unreadable` | A task file could not be read. |
| `task_missing_field` | Task front matter is missing a required field. |
| `task_malformed` | A task could not be parsed or has unsupported enum values. |
| `task_bucket_mismatch` | A task status does not match its active, completed, or cancelled bucket. |
| `task_dependency_missing` | A task depends on an ID that does not exist. |
| `task_dependency_cycle` | Non-completed, non-cancelled tasks contain a dependency cycle. |
| `task_acceptance_missing` | A completed task is missing an acceptance section. |
| `task_acceptance_placeholder` | A completed task acceptance section still contains the seeded `- [ ] TODO` placeholder. |
| `task_acceptance_unchecked` | A completed task acceptance section contains unchecked `- [ ]` or `* [ ]` items. |
| `task_exec_plan_missing` | A task references an ExecPlan that could not be found. |
| `task_completed_exec_plan_active` | A completed task references an ExecPlan still under `.agents/exec-plans/active/`. |
| `task_completed_exec_plan_incomplete` | A completed task references a completed ExecPlan without a filled `Outcomes & Retrospective` section. |
| `exec_plan_active_with_outcomes` | An active ExecPlan has a filled `Outcomes & Retrospective` section. |
| `exec_plan_completed_without_outcomes` | A completed ExecPlan has an empty or missing `Outcomes & Retrospective` section. |
| `exec_plan_completed_with_open_progress` | A completed ExecPlan still has open `- [ ]` items in its `Progress` section. |
| `exec_plan_missing_section` | An ExecPlan is missing one of the mandatory lifecycle sections. `ahm` emits one finding per missing section. |
| `exec_plan_orphan` | An ExecPlan is not referenced by any task `exec_plan` field. This is an info-tier finding. |
| `generated_index_missing` | A generated workflow index is missing and should be regenerated with `ahm index`. |
| `generated_index_unreadable` | A generated workflow index could not be read. |
| `generated_index_stale` | A generated workflow index differs from the output `ahm index` would write. |
| `generated_index_check_failed` | `ahm` could not render expected generated indexes for validation. |
| `markdown_link_missing` | A relative Markdown link inside the managed workflow surface points at a missing file. |
| `markdown_link_check_failed` | A workflow Markdown link check could not be completed. |
| `project_doc_link_missing` | A relative Markdown link in a discovered project documentation file points at a missing file. Emitted only under the opt-in `--check project-docs` scope. |
| `project_doc_link_check_failed` | A project documentation Markdown link check could not be completed. Emitted only under the opt-in `--check project-docs` scope. |
| `design_doc_unindexed` | A design-doc Markdown file under `docs/design-docs/` is not represented in `docs/design-docs/index.md`. Emitted only under the opt-in `--check project-docs` scope, and only when the repository already uses the `docs/design-docs/` convention with an `index.md`. |
