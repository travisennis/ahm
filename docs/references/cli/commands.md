# ahm Commands

This reference covers non-task `ahm` commands. For global flags and output
modes, see [the global CLI contract](global-contract.md). For task lifecycle
commands, see [task commands](task-commands.md).

Exhaustive flag details live in `ahm <command> --help`. This page documents
only compatibility guarantees that generated help cannot express.

## Compatibility Guarantees

All non-task commands share these guarantees unless stated otherwise:

- **`--dry-run`**: previews the operation without writing files. Supported by
  `init`, `index`, `adr create`, ADR lifecycle commands.
- **`--json` / `--plain`**: structured output mode. Unsupported commands print
  text regardless of the flag.

### `help`

Prints built-in help. Aliases: `--help`, `-h`.

### `version`

Prints the ahm binary version. Alias: `--version`.

The version is the release tag (e.g., `0.3.0`). Dev builds print `dev`.

### `adr create <title> [flags]`

Creates a new MADR-profile ADR under `docs/adr/` and regenerates indexes.

**Guarantees:**

- Next ID is the next zero-padded numeric ID after the highest existing ADR
  filename (`001`, `002`, ...).
- `--body-file` provides full body content below the H1; ahm owns ID
  allocation, front matter, heading, location, and index regeneration.
- `--body-file` and `--description` are mutually exclusive.
- `--dry-run` prints the target path and ID without creating.

### `adr list`

Lists ADRs parsed from `docs/adr/`.

**Guarantees:**

- Sorted by ADR ID.
- `--status <status>` filters by one or more statuses (comma-separated list
  or repeated flags). Case-insensitive. Prefix matching: `--status superseded`
  matches `superseded by ADR-009`.
- `--json` emits `id`, `title`, `status`, `date`. `--plain` emits compact JSON.

### `adr show <id>`

Shows one ADR. ID resolution: `9`, `009`, or `009-madr-adr-management`.

**Guarantees:**

- Default prints the raw Markdown file.
- `--json` / `--plain` prints the parsed ADR record.

### `adr accept|reject|deprecate|propose <id>`

Sets the ADR status, updates `date:` to today, regenerates indexes.

**Guarantees:**

- Idempotent on already-matching status (reports `<id> already <status>`).
- Refuses transitions that violate MADR lifecycle rules (e.g., accepting an
  already-rejected ADR).
- `--dry-run` prints the target and new status without writing.

### `adr supersede <old-id> --by <new-id>`

Marks `<old-id>` as `superseded by ADR-NNN`, adds a Supersession note to it,
and cross-references the replacement. Regenerates indexes.

**Guarantees:**

- `--by` is required and must resolve to an existing ADR.
- Both ADRs must be MADR-profile records, the replacement must be `accepted`,
  and the old ADR must be `accepted` or already superseded by that same
  replacement. An ADR cannot supersede itself.
- `--dry-run` prints the target and new status without writing.

### `prime`

Regenerates all generated indexes, runs workflow validation, and prints a
live repository briefing. The entry point for agent sessions.

**Guarantees:**

- Fast, offline-tolerant, idempotent.
- Prints validation findings, task counts, and the backlog.

### `status`

Reports workflow health. No scope flag: runs the `workflow` and `links`
validation scopes by default.

**Guarantees:**

- Exit code 1 when validation contains errors.
- See `docs/references/workflow-spec.md` for validation scopes and finding codes.
- See `task-file-format.md` for the full validation finding code catalog.

### `doctor`

Reports environment health: workflow metadata, installed version, repository
state.

**Guarantees:**

- Exit code 1 when validation contains errors.
- Shares validation infrastructure with `status`.

### `init`

Creates ahm-owned workflow state when it is absent and reconciles it when it
is present.

**Guarantees:**

- Creates `.ahm/config.json`, the managed `.ahm/.gitignore`, the record
  directories, and the generated indexes when they are missing.
- Rewrites an ahm-owned file only when its bytes differ from what ahm owns, so
  an up-to-date repository is left untouched and a repeated run writes nothing.
- Drops obsolete ahm-owned configuration keys (`taskWork`,
  `default_work_agent`, `projectDocs`, `research`) and discards `files`
  ownership hashes for the retired managed files and generated-index paths it
  knows about. Other unknown metadata, including any other `files` entry, is
  preserved.
- Never creates, overwrites, or removes project-owned `AGENTS.md`.
- Refuses a repository whose metadata is still `.agents/ahm.json` and names
  `v1.0.0`, the final v1 release, as the release to upgrade with first. A
  repository that also holds `.ahm/config.json` is managed: the v1 migration
  wrote the config after moving the records, so the legacy file is stale.
- `--dry-run` previews every write without touching the filesystem.

### `index`

Regenerates all generated indexes from source records. Also removes stale
`.tmp` files older than five minutes anywhere under `.ahm/`, including
leftovers from an interrupted write; a cleanup failure warns instead of failing
the command.

**Guarantees:**

- Deterministic sort order.
- Never edits source records.
- `--dry-run` previews index writes and removes nothing, including stale
  `.tmp` files.
