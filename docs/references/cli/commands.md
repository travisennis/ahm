# ahm Commands

This reference covers non-task `ahm` commands. For global flags and output
modes, see [the global CLI contract](global-contract.md). For task lifecycle
commands, see [task commands](task-commands.md).

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

### `agents suggestions`

Prints advisory integration instructions that a project may use to update an
existing project-owned `AGENTS.md`. The suggestions are intentionally limited
to ahm-owned workflow intake and ownership boundaries: how to adapt an existing
Operating Loop, when to use scoped `ahm context` commands for managed tasks,
ExecPlans, ADRs, or research before returning to the project's normal workflow
routing, how to treat generated indexes, and which task or ADR state moves
should use `ahm` commands.

This command never writes `AGENTS.md`. It exists for repositories where
`AGENTS.md` already exists or where a maintainer wants a small bridge to
`ahm context`. The intended workflow is for an agent or maintainer to run the
command, review the suggestions, and adapt any useful instructions into the
existing instructions without replacing project-specific guidance.

When a target `AGENTS.md` already has an Operating Loop, the output recommends
patching that loop so managed-work intake happens before normal workflow
routing. When a target has workflow routing but no Operating Loop, it
recommends adding a short loop before the routing section. When a target has
neither, it recommends adding only the ahm-specific intake and ownership
sections rather than inventing a full project workflow.

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

### `context`

Prints read-only context for humans and coding agents.

With no scope, `context` combines canonical agent workflow instructions with
live repository state:

- Repository root, installed workflow version, and embedded template version.
- Validation status and the first few validation findings, without failing the
  command when findings exist.
- Git branch and dirty worktree count when `git` is available.
- Task counts, in-progress tasks, and the next ready task.
- Useful follow-up commands for the selected scope.

With a scope, `context` prints the full embedded instruction document for that
workflow. For example, `ahm context task` prints the task workflow rules to use
before creating, choosing, updating, or working on tasks.

Supported optional scopes:

- `task`
- `adr`
- `research`
- `plan`
- `docs`

The command is read-only. It does not run `ahm index`, start tasks, mutate
workflow files, invoke external agents, or run mutating git commands.

Useful flags:

- `--json`: prints structured context sections.
- `--plain`: prints compact JSON.

Examples:

```bash
ahm context
ahm context task
ahm --json context adr
```

### `init`

Installs the managed `.agents` workflow into the target root.

`init` creates missing workflow directories, metadata, and generated indexes.
Canonical agent instructions are exposed through `ahm context`, not copied into
consumer repositories. `init` does not create or overwrite `AGENTS.md`.

Writes:

- Managed skill templates under `.agents/skills/`.
- `.agents/ahm.json` metadata.
- Generated index files under `.agents/.tasks/`, `.agents/.research/`,
  `.agents/exec-plans/`, and `docs/adr/index.md`.
- Workflow directories under `.agents/` and `docs/adr/`.

Useful flags:

- `--dry-run`: prints files, directories, metadata, and indexes that would be
  written.
- `--force`: overwrites existing managed skill templates.
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
recorded managed hash are removed because canonical guidance now comes from
`ahm context`. Managed skill templates are still updated from embedded
templates. Locally modified files are preserved and reported as conflicts
unless `--force` is used. Generated indexes are regenerated.

The metadata `version` field always advances to the embedded template version,
even when conflicts exist. This means a partial upgrade (some files conflicted,
others updated) records the new template version so subsequent upgrades can
correctly identify already-updated files.

`AGENTS.md` remains project-owned and is never overwritten or removed.

Useful flags:

- `--dry-run`: previews all supported writes.
- `--force`: removes locally modified former instruction templates and
  overwrites locally modified managed skill templates. This does not apply to
  `AGENTS.md`.
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

Writes:

- `.agents/.tasks/index.md`
- `.agents/.tasks/active/index.md`
- `.agents/.tasks/completed/index.md`
- `.agents/.tasks/cancelled/index.md`
- `.agents/.research/index.md`
- `.agents/exec-plans/active/index.md`
- `.agents/exec-plans/completed/index.md`
- `docs/adr/index.md`

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

Example:

```bash
ahm index
ahm --dry-run index
```
