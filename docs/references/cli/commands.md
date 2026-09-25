# ahm Commands

This reference covers non-task `ahm` commands. For global flags and output
modes, see [the global CLI contract](global-contract.md). For task lifecycle
commands, see [task commands](task-commands.md).

Exhaustive flag details live in `ahm <command> --help`. This page documents
only compatibility guarantees that generated help cannot express.

## Compatibility Guarantees

All non-task commands share these guarantees unless stated otherwise:

- **`--dry-run`**: previews the operation without writing files. Supported by
  `init`, `index`, `adr create`, ADR lifecycle commands, `store path`, and
  `store migrate`.
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
- For an installed home-mode project, text and structured output include
  `store.root`, `store.key`, `store.kind` (`remote` or `path`), and
  `store.location` (`home`). The block is absent in project mode and for an
  uninstalled repository.
- `--dry-run` previews index regeneration and does not write files or create
  store state.

### `status`

Reports workflow health. No scope flag: runs the `workflow` and `links`
validation scopes by default.

**Guarantees:**

- Exit code 1 when validation contains errors.
- For an installed home-mode project, text and structured output include the
  same `store` object as `prime`; the block is absent in project mode and for
  an uninstalled repository.
- See `docs/references/workflow-spec.md` for validation scopes and finding codes.
- See `task-file-format.md` for the full validation finding code catalog.

### `doctor`

Reports environment health: workflow metadata, installed version, repository
state.

**Guarantees:**

- Exit code 1 when validation contains errors.
- Shares validation infrastructure with `status`; it does not add a separate
  `store` block. A home-mode store failure appears in its validation findings
  as `store_dir_unreadable`.

### `init`

Creates ahm-owned workflow state when it is absent and reconciles it when it
is present.

**Guarantees:**

- A repository with no `.ahm/config.json` is a new project, and a new project
  keeps its task records in the user-level store. The run writes
  `tasks_location: home` into the configuration it creates and prepares the
  store: the record directories, the generated task indexes, and the store's
  managed `.gitignore` under the store's directory for the project, plus the
  store's `project.json` state file, which records the next top-level task ID
  the records present imply, and its identity in `<store>/registry.json`.
  Neither store file is listed in the result, because they are store state
  rather than reconciled workflow files.
- A new project whose root holds `.git` needs Git to read that repository: an
  unreadable one fails (exit code 1) rather than deriving a different store key,
  because identity comes from the selected Git remote (`origin`, or the only
  remote when there is one). A root with no `.git` uses
  the path rule and reads no Git.
- A repository that already has a configuration keeps the mode it names: a
  configuration without a `tasks_location` key — every repository that predates
  the store — keeps its records in the project, and `init` leaves the file
  byte-identical. In `project` mode it creates the `.ahm/tasks/` record
  directories and the generated indexes under the project root instead of the
  store's.
- In both modes it writes the committed `.ahm/config.json` and the managed
  `.ahm/.gitignore` when they are missing. The committed `.gitignore` lists the
  generated task indexes and the records lock in `project` mode, and only the
  temp-file pattern in `home` mode, where the task indexes and the lock live in
  the store.
- There is no `--tasks-project` flag. A new repository that wants its records in
  the project runs `ahm store migrate --to project`, which writes the mode and
  needs no preceding `ahm init`.
- The store must be writable, because the run records the project in the store
  registry: a store that cannot be written exits 1 and names the path, at the
  first write that fails. A store root that cannot be created fails before
  anything is written; a store whose state cannot be written fails after the
  project's own files are installed.
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
- `--dry-run` previews every write without touching the filesystem, the store
  directory included.

### `store path`

Prints the resolved home-store location for this project: the store root, the
project key, and the records directory
(`<root>/projects/<dir>/tasks`). The command resolves this location even when
the repository currently keeps records in the project.

**Guarantees:**

- The store root is `~/.ahm`, or `AHM_HOME` when it names an absolute path. A
  relative `AHM_HOME` is a usage error (exit code 2), and an `AHM_HOME` that
  exists and is not a directory exits 1.
- The key is derived per command and never stored in the project. With a Git
  remote it is the canonical form of the selected remote — the lowercased
  `host/owner/repo` form, with scheme, userinfo, default port, trailing `.git`,
  and trailing slash removed, and non-default ports kept. A repository whose
  only remote is not `origin` uses that remote; several remotes without an
  `origin`, a remote that names no URL, a `file://` remote, a local-path remote,
  and no remote at all fall back to the SHA-256 of the symlink-resolved project
  root. Credentials are never included in the key or persisted.
- Identity uses the project root's own `.git`. A root whose own directory
  holds no `.git` — a directory managed by `.ahm/config.json` alone, a `--root`
  pointing into a repository subdirectory, or a bare repository — always uses
  the path rule, so it never inherits the remote of an enclosing repository.
- A root that holds `.git` but that Git cannot read (git is missing, or the
  repository is broken or unreadable) exits 1 instead of falling back to the
  path rule, because a silent fallback would resolve a different key.
- Two clones, and a linked path, of one project report the same key and
  directory when they resolve to the same project key. Path-keyed projects at
  different roots resolve to different keys and directories.
- The command records the project in `<store>/registry.json`, and its state in
  `<store>/projects/<dir>/project.json`, unless `--dry-run` is given; both are
  written only when their bytes change, so a repeated run writes nothing. An
  unknown store format version is refused with exit code 1. No other path is
  written.
- The project state file holds the store format version, and `next_id` once a
  counter has been recorded. The command records the value it read and never
  lowers it.
- `--json` and `--plain` emit `root`, `key`, `kind` (`remote` or `path`), and
  `records`; text output abbreviates the user's home directory to `~`.

### `store migrate --to home|project`

Moves this project's task records between the project and the user-level home
store. It is the only command that moves a record between the two layouts;
`task status` moves a record between buckets inside one layout.

**Guarantees:**

- `--to` is required and names the destination. The direction is never inferred
  from the current configuration, so repeating the command after an interrupt
  finishes the move it started. A missing or unknown `--to` is a usage error
  (exit code 2).
- A repository with no configuration names no layout, so it takes the full move
  in either direction and writes the mode `--to` asks for: `--to home` sets a new
  project up in the store, and `--to project` keeps a new project's records in
  the project. The configuration is written even though no record moved, because
  the run is what commits the layout.
- Each record is read, written atomically into the destination, and removed from
  the source only afterwards. Nothing is renamed, so the move works when the
  store and the project are on different volumes, and a crash or an interrupt
  leaves the record on both sides; re-running the command completes the move and
  leaves a record that already arrived byte-identical, modification time
  included.
- Every read whose failure would leave the move owing a file it cannot write —
  the committed configuration, and the store state and registry it records —
  happens before the first record moves, so a precondition that cannot be met
  (an unreadable configuration, a store file a newer ahm wrote) fails with the
  records still in their source layout. A failure after that point leaves the
  move finishable by running the same command again.
- The whole move holds the record-mutation lock of the layout the configuration
  currently names, which is the lock every other command serializes on. The
  configuration flips only at the end of the move, so the lock stays the one
  every other command takes for as long as the move writes anything.
- The committed `tasks_location` key names the destination, and is written in
  both directions, so a repository moved back to the project keeps the project
  layout even once a configuration without the key defaults to the store. It is
  written last of all, after the source's derived files are gone: it is the
  move's commit point, and until it lands a repeated command continues the same
  move instead of treating it as finished. A move that stops at that point
  leaves the records in the destination while the configuration still names the
  source, so `status`, `doctor`, and the record commands read a layout the
  records have left and report an empty backlog; `git status` shows the
  deletions or additions, and re-running the same command finishes the move.
- The managed `.gitignore` of the destination layout is rewritten for the new
  mode, and a move into the store writes the store's `.gitignore` beside the
  records. The store's file covers the generated indexes, the lock, the state
  file, and temp files; the committed `.ahm/.gitignore` keeps only the `*.tmp`
  pattern in home mode, because the atomic rewrite of `config.json` is the one
  ahm write that stays in the project.
- Generated indexes are regenerated on the destination side, the committed ADR
  index stays under the project root, and the source's generated task indexes
  and the empty records tree they leave are removed with the records. Those
  removals happen before the committed `.gitignore` stops ignoring them; a run
  that finds nothing to move but such leftovers still removes them.
- The store's task ID counter is initialized from the records present, so a
  record deleted after the move cannot have its number reissued.
- The move is recorded in the store registry entry as `migrated_from`, naming
  the layout the records came from. A move out of a store this machine has no
  state file for — a fresh clone, say — records nothing: it creates no registry
  entry and no store state file, because it has nothing observed to record.
- Records moving out of the project are refused (exit code 1) when Git reports
  record content only the working tree has: a modified, staged, or untracked task
  record is named, because the move deletes it and only committed content is
  recoverable afterwards. Generated indexes and other files in the records tree
  are not named. `--force` moves such records anyway. A deletion relative to
  `HEAD` is not such a change: the deleted content is in Git's history or already
  in the destination, and a half-finished move leaves exactly that behind.
- A destination that already holds the arriving record's identity is refused
  (exit code 1) in both directions, with both paths named: a record with
  different bytes under the same name, or a record wearing the same ID in
  another bucket, which would survive the move as a duplicate. The resume case
  is always byte-identical, so neither is one. `--force` moves the source record
  anyway, overwriting the first and duplicating the second.
- A destination store directory that `registry.json` registers to a different
  project key is refused (exit code 1) with both paths named, so two projects'
  records never share one directory. This refusal has no override: a store
  directory registered to another key is a misregistration to repair, not a risk
  to accept.
- A root whose own directory holds no `.git` is moved without the Git check:
  there is no project history to protect and no Git to ask.
- `ahm` never stages, commits, or moves `HEAD`. The project-side deletions or
  additions the move leaves are printed as the commit list for the user to
  review.
- `--dry-run` previews the record moves, the committed configuration and
  `.gitignore` writes, the destination indexes, the source index removals, and
  the commit list. It writes nothing, takes no lock, creates no store
  directory, and does not list the store's own `project.json` and
  `registry.json` bookkeeping. It reports the refusals a real run would hit.
- `--json` and `--plain` emit `to`, `moved` (source and destination pairs),
  `written`, `removed`, `dry_run` (dry runs only), and the `deletions` or
  `additions` commit list. Text output prints the same actions, one per line,
  and abbreviates no path: a store path renders relative to the store project
  directory, such as `store:tasks/active/001.md`.
- A repeated run whose records already live in the destination — a repository
  whose configuration names it — reports no work
  and writes nothing, unless it finds the source's generated indexes or empty
  records directories still there, which it removes and reports.

### `index`

Regenerates all generated indexes from source records. Also removes stale
`.tmp` files older than five minutes from the project's `.ahm/` state
directory and, in home mode, the store project's state directory, including
leftovers from an interrupted write; a cleanup failure warns instead of
failing the command. It never scans the whole repository or store root.

**Guarantees:**

- Deterministic sort order.
- Never edits source records.
- `--dry-run` previews index writes and removes nothing, including stale
  `.tmp` files.
