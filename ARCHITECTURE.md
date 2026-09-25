# Architecture

`ahm` is a single-binary Go CLI. It manages a repository's project workflow
state and its optional user-level home store. The project root owns committed
configuration at `.ahm/config.json` and ADRs under `docs/adr/`; the resolved
records root owns task records and their generated indexes. In project mode
those records are ordinary tracked files under `.ahm/`; in home mode they are
machine-local files in the store. `ahm` reports live repository state,
validates workflow health, and regenerates deterministic indexes. A repository
still on the retired `.agents/ahm.json` layout is refused by root detection
rather than half-adopted.

## System Boundaries

- `ahm` owns workflow installation and reconciliation, validation, task/ADR
  lifecycle commands, and generated indexes.
- Target repositories own their source code and project-specific `AGENTS.md`.
  `ahm` does not patch source files, commit, create PRs, or run implicit git
  operations. Workflow commands read and write task records and generated
  indexes under the resolved records root, and ADRs under `docs/adr/`, without
  moving `HEAD`, staging files, writing the project index, or modifying
  project-owned files. `store migrate` is the one command that moves task
  records between the project and the store — as files, never as a Git
  operation: it reads Git only to derive identity and to check for uncommitted
  record changes.

## Compatibility Surfaces

- CLI command names, flags, aliases, exit codes, help text, and output modes.
- Text, JSON, and plain output shapes, including validation finding codes.
- `.ahm/config.json` metadata fields and compatibility.
- Task and ADR record formats and generated index formats.
- Install reconciliation behavior, including project-owned `AGENTS.md`
  behavior and the retired-file ownership boundary.
- Atomic write guarantees and stale temp-file cleanup.
- Home-store resolution: the `tasks_location` mode, the derived project key and
  registry mapping, the `store path` and `store migrate` command surface with
  its exit codes and refusals, and the
  `store:<path-relative-to-store-project-directory>` display convention (for
  example, `store:tasks/active/001.md`).
- Go module version, local tool versions, CI, and release packaging.

## Module Map

Files are grouped by responsibility. The source tree is the authoritative
location map; this section describes what each group does.

| Group | Location | Responsibility |
| --- | --- | --- |
| Entrypoint | `cmd/ahm/main.go` | Binary entrypoint. |
| CLI wiring | `internal/ahm/cli.go` | Cobra root command, global flags, command registration. |
| Root detection | `internal/ahm/root.go` | Repository root discovery from `.git` or `.ahm/config.json`, and refusal of the retired `.agents/ahm.json` layout. |
| Infrastructure | `internal/ahm/lock.go`, `write.go`, `fsync_unix.go`, `fsync_windows.go`, `git.go`, `identity.go`, `store.go`, `path.go`, `output.go`, `workflow_paths.go`, `recordcache.go`, `markdown_sections.go` | Atomic writes, write containment, and their directory sync, repo-local locks, Git environment isolation and remote reads, project identity derivation and home-store resolution, path helpers, shared output emitters, resolution of the project and records roots, per-command record read reuse, and Markdown heading-section lookup. |
| Store migration | `internal/ahm/store_migrate.go` | `store migrate`, the one command that moves task records between the project and the store: the resumable read-write-remove move, the precondition reads that precede it, the destination-key, divergent-record, and uncommitted-change refusals, and the configuration, `.gitignore`, index, counter, and registry writes the move owes. |
| Install | `internal/ahm/install.go` | `init` create-or-reconcile, metadata (including the `tasks_location` mode, which a new project writes as `home`), the managed `.gitignore` of every layout the mode owns, the store observation a new project records, and generated index writes. |
| Status, prime & validation | `internal/ahm/status.go`, `prime.go`, `validation.go` | `status`, `doctor`, the `prime` state report, and workflow/link/ADR/task validation. |
| Tasks | `internal/ahm/tasks.go`, `task_commands.go`, `task_create.go`, `task_id_counter.go`, `task_list.go`, `task_status.go`, `task_find.go`, `task_enum.go`, `task_comment.go`, `task_deps.go`, `task_acceptance.go` | Task model, parsing, rendering, all lifecycle commands, dependency management, acceptance checking, and the store's task ID counter. |
| ADRs | `internal/ahm/adrs.go`, `adr_commands.go` | ADR model, parsing, lifecycle commands. |
| Indexes | `internal/ahm/indexes.go` | Task and ADR generated index rendering. |
| Version | `internal/version/version.go` | Binary version injected by release builds. |

## Architectural Invariants

- Writes are explicit and use the atomic temp-file-then-rename path in
  `internal/ahm/write.go`, which syncs the temp file and then its parent
  directory.
- Every workflow record, index, and configuration write goes through
  `writeOwned`, which refuses a target outside the owned roots (the project
  root, and the store's project directory when records live in the store).
  `writeFileAtomic` guarantees atomicity only; containment lives in
  `writeOwned`. Two writers stay outside it by design: the lock protocol writes
  its owner token inside the lock it just created, and `store path`,
  `store migrate`, and a home-mode `ahm init` write the store's registry — and
  their own observation of `project.json` — directly, because the registry
  always sits at the store root, outside every owned root, and a resolved
  `storePaths` is what names it. The state file is inside an owned root only
  when the records live in the store. The
  task ID counter in that same state file is written by `task create`,
  `ahm init`, and `store migrate`, which do hold the resolved paths, so it goes
  through `writeOwned`.
- Records move between the two layouts only through `store migrate`, and only in
  the direction `--to` names: the layout the configuration currently names is
  never the evidence for a direction. Each record is read, written atomically
  into the destination through `writeOwned`, and removed from the source only
  afterwards, because the store and the project may not share a filesystem, so
  the move is resumable and idempotent by construction. Every read whose failure
  would leave the move owing a file it cannot write — the committed
  configuration, and the store state and registry it records — happens before
  the first record moves, so a precondition that cannot be met leaves the
  records where they are; a failure after that point (an unreadable ADR tree,
  say) leaves the move finishable by running the same command again. The
  committed `tasks_location` key is written last of all, after the records and
  the source's derived files are gone: it is the move's one commit point, so
  until it lands the configuration still names the source layout, a repeated
  command takes the same path again, and the lock this run holds stays the lock
  every other command takes. The source deletions are direct `os.Remove` calls
  on paths built from the source layout's own accessors — the task scan's record
  paths, its generated index paths, and the records directories the move emptied
  — which confines them by construction. Every write the move makes still goes
  through `writeOwned`, including the managed `.gitignore` of either layout and
  the task ID counter in the store's state file; the migration's own observation
  of that state file and of the registry stays the direct `writeFileAtomic` the
  bullet above describes. The move refuses a destination store directory the
  registry registers to another project key, refuses a destination that already
  holds the arriving record's identity — different bytes under the same name, or
  the same ID in another bucket — and refuses to delete records whose content
  exists only in the working tree, asking Git through one read-only
  `git status --porcelain` on the records root. The store-directory refusal is
  a misregistration to repair rather than to force past; the other two apply to
  both directions and to a move out of the project respectively, and `--force`
  overrides them.
- A repository has two roots: a project root that owns `.ahm/config.json` and
  `docs/adr/`, and a records root that owns task records and their generated
  indexes. `workflow_paths.go` is the single definition of both, and every task
  record path is derived from it rather than joined onto a root at the call
  site.
- The committed `tasks_location` key selects the layout: `project` keeps the
  records in the project, `home` resolves the store and puts the records, their
  generated indexes, the managed `.gitignore`, and the lock in the store's
  directory for the project. A missing key means `project`, so every repository
  that predates the store keeps its records in the project. A repository with no
  configuration at all is a new project: it resolves as `home`, and `init`
  writes that key. The store is resolved only when the configuration resolves
  to `home`, so a project-mode repository never reads Git and never fails
  because the store is unavailable. A home-mode project whose root holds `.git`
  needs Git to read that repository, because its key comes from the remote Git
  selects (`origin` when present, otherwise the only remote); a root with no
  `.git` uses the path rule and reads no Git.
- Record paths in findings, error messages, index listings, directory labels,
  and lock errors render through `displayPath`
  (`store:<path-relative-to-store-project-directory>` in home mode, such as
  `store:tasks/active/001.md`, and repository-relative in project mode). The
  JSON record path and the dry-run previews render through `payloadPath`: the
  same store-relative display in home mode and the record's own path in project
  mode, so those payloads stay byte-identical for existing repositories. An
  operating-system message keeps its own text.
- Home mode never reissues a top-level task ID while the store's state file
  survives: the store persists a `next_id` high-water mark in its project state
  file, beside the records, and allocation is the higher of one past the highest
  record present and that counter. The counter only ever moves up: both writers,
  `task create` under the record lock and the `store path` observation, re-read
  the file and keep the higher value, so a stale observation cannot lower it by
  itself. The residual holes are the two writers interleaving inside one
  read-to-rename window, and the state file being lost, which leaves the records
  present as the only evidence. `ahm init` records the mark the records present
  imply. Project mode keeps the number scan and does return a hand-deleted
  record's number to the pool: Git history is the evidence that the ID was
  spent, not something ahm reads before allocating. Child IDs keep their own
  letter scan in both layouts, so a deleted child's letter can be reissued.
- Cross-process workflow mutations that require read-compute-write consistency
  use repository-local locks beside the records root (`.ahm/.lock/` in project
  mode), so two clones that share a store serialize on one lock.
- Generated indexes are deterministic; sort output consistently and keep index
  generation centralized.
- Post-mutation index generation and workflow validation may reuse a complete
  freshly parsed task set. Partial task parses and standalone `status`/`doctor`
  validation retain independent disk reads so validation findings stay intact.
- Record reuse across steps goes through an explicitly passed `recordCache`
  scoped to one command, never a process-lifetime cache. Every command still
  reads each record from disk, so standalone `status`/`doctor` keep detecting
  out-of-band edits and stale indexes. Any write made while a cache is live must
  be reported to it, or a later read in the same command sees pre-write bytes.
- `AGENTS.md` is project-owned. Never treat a project `AGENTS.md` as a managed
  file that `init` or `--force` can create, replace, or remove.
- Validation is read-only. It reports workflow drift and structured-record
  link-integrity findings without mutating files.
- Ahm-owned Git subprocesses use an explicit repository root and ignore
  inherited Git repository-location variables that could redirect metadata,
  the worktree, or the index. ADR 018 defines this boundary.
- Command handlers should stay thin: parse args, validate boundaries, delegate
  to focused helpers, then emit output.
- File-format parsers should validate at the boundary and return explicit
  errors; renderers should preserve unknown fields where the format promises it.
- Dry-run behavior must not mutate disk or in-memory state in ways that affect
  later operations.

## Reference Docs

- Documentation index: `docs/README.md`.
- CLI contract: `docs/cli.md` and `docs/references/cli/`.
- Workflow state, file ownership, formats, and atomic writes:
  `docs/references/workflow-spec.md`.
- Upgrade and version behavior: `docs/guides/workflow-upgrades.md`.
- Task, ADR, and ExecPlan procedures: `docs/workflow/`; decision history:
  `docs/adr/`.
- Contributor commands and handoff expectations: `CONTRIBUTING.md`.
