# ahm Commands

This reference covers non-task `ahm` commands. For global flags and output
modes, see [the global CLI contract](global-contract.md). For task lifecycle
commands, see [task commands](task-commands.md).

Exhaustive flag details live in `ahm <command> --help`. This page documents
only compatibility guarantees that generated help cannot express.

## Compatibility Guarantees

All non-task commands share these guarantees unless stated otherwise:

- **`--dry-run`**: previews the operation without writing files. Supported by
  `init`, `upgrade`, `index`, `adr create`, ADR lifecycle commands, `records migrate`.
- **`--json` / `--plain`**: structured output mode. Unsupported commands print
  text regardless of the flag.

### `audit [flags]`

Delegates a read-only codebase-improvement survey to a supported coding-agent
CLI. Creates one `Open` task per finding with the `source:audit` label.

**Guarantees:**

- Agent selection, `--agent`, `--model`, and `--timeout` match `task groom`.
- `--dry-run` prints the prompt and schema without delegation or writes.
- Invalid output creates no tasks and exits nonzero.
- Text, `--plain`, and `--json` summaries share one structured result.

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

### `adr accept|reject|deprecate <id>`

Sets the ADR status, updates `date:` to today, regenerates indexes.

**Guarantees:**

- Idempotent on already-matching status (reports `<id> already <status>`).
- Refuses transitions that violate MADR lifecycle rules (e.g., accepting an
  already-rejected ADR).
- `--dry-run` prints the target and new status without writing.

### `onboard [flags]`

Prints a paste-ready AGENTS.md bootstrap snippet for new repositories.

**Guarantees:**

- Text mode: framed Markdown. `--plain`: bare snippet. `--json`: structured
  `snippet` field.
- Runs in any directory; does not detect or require a repository root.

### `prime`

Regenerates all generated indexes, runs workflow validation, and prints a
live repository briefing. The entry point for agent sessions.

**Guarantees:**

- Fast, offline-tolerant, idempotent.
- Prints warnings, backlog summary, and managed-work routing.

### `context [scope]`

Prints full managed-work instructions for one scope: `task`, `plan`, `adr`, or
`research`. General project documentation is not an ahm scope; see
[ADR 021](../../adr/021-limit-ahm-to-structured-workflow-records.md).

**Guarantees:**

- Scoped context prints binary-emitted procedures, not installed files.
- Unsupported or missing scope exits with a usage error listing valid scopes;
  the unscoped form routes to `ahm prime`.

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

Installs the managed `.ahm` workflow state in the target repository.

**Guarantees:**

- On fresh installs (no prior metadata): creates the `.ahm/` layout.
- On repositories with existing `.agents/ahm.json`: preserves the existing
  layout.
- Never creates, overwrites, or removes project-owned `AGENTS.md`.
- `--dry-run` previews all write operations.

### `upgrade`

Updates managed workflow state to match the current binary.

**Guarantees:**

- Missing directories, metadata, and indexes are created.
- Legacy instruction files matching the previous managed hash are removed.
- Locally modified files are preserved and reported as conflicts.
- Project-owned `AGENTS.md` is never created, overwritten, or removed, even
  with `--force`.
- `--dry-run` previews all write operations.

### `index`

Regenerates all generated indexes from source records.

**Guarantees:**

- Deterministic sort order.
- Never edits source records.

### `records migrate`

Migrates workflow records from the legacy `.agents/` layout to `.ahm/`.

**Guarantees:**

- Preview mode prints changes without writing.
- Preserves project-owned `.agents/` content.
- Does not stage files or move `HEAD`.

### `records doctor`

Diagnoses migration state.
