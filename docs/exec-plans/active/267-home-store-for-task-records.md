# Store task records in a user-level home store

This ExecPlan is a living document. The sections `Progress`, `Surprises &
Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to
date as work proceeds. This document is maintained in accordance with the
[ExecPlan workflow](../../workflow/exec-plans.md); reopen that guide before
revising the plan.

Tracker: task 267; children 267a through 267h sequence the milestones. One
milestone maps to exactly one child task. The decision of record is
[ADR 023](../../adr/023-store-task-records-in-a-user-level-home-store.md);
read it before starting. This plan is the delivery sequence for that decision
and does not restate its reasoning.

## Purpose / Big Picture

Today a project's task list is a directory of Markdown files committed inside
the project, at `.ahm/tasks/`. That makes the list durable and reviewable, and
it also makes the list part of branch history: a task created on a branch does
not exist on another branch, every backlog edit appears in commits and pull
requests, and a fresh clone carries whatever the branch carried.

After this work, a project's task list lives on the machine instead, under
`~/.ahm/projects/<key>/tasks/`, where `<key>` identifies the project. The list
is the same from every branch, in every linked worktree, and in every clone of
the same repository, and it never appears in project history. ADRs are
unchanged: they stay committed under `docs/adr/` because they are durable
project documentation.

You can see it working by creating a task and looking outside the repository:

    $ cd ~/Projects/ahm
    $ ahm task create "Try the home store"
    267
    $ ahm store path
    root:    ~/.ahm
    key:     github.com/travisennis/ahm
    records: ~/.ahm/projects/ahm-3f9ac4d1/tasks
    $ git status --short
    $ ls ~/.ahm/projects/ahm-3f9ac4d1/tasks/active
    267.md

Before this work the same command writes `.ahm/tasks/active/267.md` inside the
repository and `git status --short` reports it as a new tracked file.

Two properties matter as much as the behavior. First, nothing changes for an
existing repository until a person opts in: a repository whose
`.ahm/config.json` has no `tasks_location` key keeps its records in the project
and produces byte-identical output, so this work can land in the v2.0.0 tree
without disturbing the tasks already there. Second, `ahm` stays a records CLI:
it gains no network access, no synchronization, no refs, and no Git writes.
It reads one Git value — the `origin` remote URL — to decide which store
directory belongs to the current project.

## Progress

- [x] (2026-09-21) Recorded the decision as
  `docs/adr/023-store-task-records-in-a-user-level-home-store.md`.
- [x] (2026-09-21) Wrote this plan and filed tracker 267 with children 267a
  through 267h.
- [x] (2026-09-21) 267a — Derive project identity and resolve the store root.
  Added `internal/ahm/identity.go`, `internal/ahm/store.go`, the Git remote
  read in `internal/ahm/git.go`, and the `ahm store path` command. Every test
  runs against a temporary `AHM_HOME`.
- [x] 267b — Split workflow paths into a project root and a records root, and
  contain every write to an owned root. No user-visible behavior change.
- [x] 267c — Read and write task records in the store, split index generation,
  report the store from `prime` and `status`, and report drift when records
  remain in the project. `project` mode stays the default.
- [ ] 267d — Persist a non-decrementing task ID counter in the store.
- [ ] 267e — Add `ahm store migrate --to home|project` and the `init` and
  validation behavior that supports it.
- [ ] 267f — Default new projects to the home store.
- [ ] 267g — Migrate this repository's own tasks to the home store as the
  first real use.
- [ ] 267h — Update the workflow spec, upgrade guide, CLI reference,
  architecture map, and agent instructions; hand the release notes to 264f.

## Surprises & Discoveries

- 2026-09-21 (267a): the registry records the remote spelling a key was derived
  from, and a URL remote can carry a token (`https://user:token@host/o/r.git`).
  Persisting the raw spelling would put a credential in `registry.json`, which
  contradicts ADR 023's rule that credentials are never hashed or persisted.
  The stored spelling therefore has userinfo removed. Evidence:
  `TestStorePathCommandRecordsRegistryWithoutCredentials` fails with
  `the registry persisted a credential` when the redaction is dropped.
- 2026-09-21 (267a): a review found that folding "Git could not read this
  repository" into "no remote applies" turns a transient Git failure into a
  different key. Evidence: with `git` off `PATH`, with a `.git` file pointing
  at a missing git directory, and with an unreadable `.git`, the resolver
  returned a path key and exit 0. `readGitRemote` now reports that case as an
  error, per the decision below.
- 2026-09-21 (267a): the registry is one file per machine, and
  `recordStoreProject` read-modify-writes it with no lock, so two concurrent
  `ahm store path` runs in different projects can drop each other's entry. The
  impact is bounded because a directory name is recomputable from the key, but
  a lost `remotes`/`paths` observation is not recoverable. No referenced
  milestone takes this on; the fix belongs with the store lock that 267b moves
  into the store, or in its own task.
- 2026-09-21 (267a): ADR 023 says symlink resolution "also normalizes platform
  aliases and on-disk casing". It normalizes aliases (a macOS `/var` root
  resolves through `/private/var`), but Go's `EvalSymlinks` does not
  case-normalize a path component, so a differently-cased `--root` derives a
  different key. Either the claim or the resolver has to change; the wording is
  in 267h's scope.
- 2026-09-21 (267a): ADR 023 says output never embeds absolute machine paths
  except in the store root that `prime` and `status` report. `store path`
  reports the store root and the records directory in absolute JSON, which is
  what this plan's Concrete Steps specify; the command exists to print where
  the records are. This is an exception to add to ADR 023's display rule in
  267h, or to revisit with a `store:`-relative `records` field.

- 2026-09-21 (267b): the plan's Interfaces sketch for `writeOwned` carried a
  `mode os.FileMode` parameter, but every workflow write is 0o644, and the
  repository's enabled `unparam` linter fails the gate on a parameter that
  always receives the same value. The signature dropped the parameter; the
  plan's Interfaces section and Decision Log now match. Evidence: `just lint`
  reported `writeOwned - mode always receives 0o644 (420) (unparam)` before the
  change and passes after it.
- 2026-09-21 (267b): a review found that the milestone's phrase "point
  `cleanupStaleTemps` at every owned root" cannot be implemented literally. An
  owned root is the project root, so a literal reading walks the whole
  repository for `*.tmp` files and reaps temp files the user owns. The cleanup
  roots are the workflow state directories instead, which is what the scan did
  before this milestone. Evidence:
  `TestCleanupStaleTempsCoversEveryStateRoot` fails if the scan is widened to
  the project root.
- 2026-09-21 (267b): the plan and ADR 023 disagree about where the store's
  managed `.gitignore` belongs. ADR 023 is in fact silent on a store
  `.gitignore`, while this plan's Idempotence section says "a managed
  `.gitignore` at the store root" and the Artifacts layout puts it at
  `<store>/projects/<slug>-<hash>/.gitignore`. 267b's containment decides the
  question by construction: `ownedRoots` is the project root plus the store's
  project directory, so a store-root write through `writeOwned` is refused.
  267c creates that file and must either place it per project or make the store
  root an owned root, and 267h owes the ADR wording for whichever it chooses.
  No code or doc was changed for this yet. Resolved in 267c: the file is per
  project, at `<store>/projects/<slug>-<hash>/.gitignore`, so the owned roots do
  not change and `registry.json` stays commit-visible.
- 2026-09-21 (267c): the display list 267b handed over names "the JSON path
  fields" and the dry-run previews as sites that relativize a record path, but
  they do not: `ahm --json task show <id>` and `ahm --dry-run task create` print
  the record's absolute path. Switching them to `displayPath` would change
  project-mode output, which ADR 023 and this milestone's acceptance forbid, so
  `workflowPaths.payloadPath` renders a store path as `store:<rel>` and returns
  a project path untouched. Evidence: the milestone's project-mode guard, and
  the payload assertions in `TestWorkflowPathsPayloadPathKeepsProjectPaths`.
- 2026-09-21 (267c): the `resolveTaskLocation(meta, configExists)` this
  milestone adds cannot use its no-configuration branch yet. Resolving a
  repository with no configuration as `home` splits its records: `init` would
  lay the store down and write a configuration without the key, and the next
  command would read that configuration as `project` and find an empty
  `.ahm/tasks/`. It also fails 87 tests in the existing suite, most of them the
  install and lifecycle tests that assert the in-project layout. Evidence: a
  mutation probe on a copy of the tree with the branch flipped to `locationHome`
  reports 87 `--- FAIL` lines. 267f flips the branch and writes the key in the
  same run; the milestone text already says the default for a new project is
  267f's change.
- 2026-09-21 (267c): resolving the layout in `detectRoot` means a home-mode
  project whose Git state cannot be read fails every command with the resolver's
  error, `doctor` and `status` included, where the previous behavior was to
  report findings. Project mode is unaffected, which is what the 267a decision
  requires, and failing is the intended alternative to deriving a different key
  from the path. Evidence: an empty `.git` directory plus a `home`
  configuration fails `ahm status` with `reading Git remotes in <root>: git
  remote: fatal: not a git repository`. 267h's display and upgrade wording
  should say that a home-mode project needs readable Git state, or that Git must
  be on `PATH`.
- 2026-09-21 (267c): operating-system error messages can still name an absolute
  path in either layout. `validateTaskEnums` prefixes the path it was given, so
  a malformed record reports `store:tasks/active/050.md: <store absolute
  path>/tasks/active/050.md: unsupported task status "-"`, and the same is true
  of raw `os` errors in `task_unreadable`, `generated_index_unreadable`, and
  `markdown_link_check_failed`. Project mode already printed its own absolute
  path this way, so fixing it would change project-mode messages; the display
  rule should be written so that findings and labels render through
  `displayPath` while an operating-system message keeps its own text.
  Evidence: the messages above, reproduced in both layouts.
- 2026-09-21 (267c): the Artifacts layout shows `.lock/workflow-records` and
  `project.json` in the store's project directory, but `install` creates
  neither: the lock directory appears with the first mutation and the state file
  with the first command that records the store observation. That matches
  project mode, where `init` does not create `.ahm/.lock` either.
- 2026-09-21 (267c): a `home` project whose store records directory is missing
  reports the missing generated task indexes alongside `store_dir_unreadable`.
  Suppressing them was considered and rejected: project mode reports exactly the
  same generated-index findings when `.ahm/tasks/` is deleted with the
  configuration still present, so the cascade is the existing layout's behavior
  rather than a new one. Evidence:
  `TestStatusReportsWorkflowArtifactConsistency` covers the project-mode
  state.

## Decision Log

- Decision: a Git read failure in a project root that holds `.git` is an error,
  not a silent fallback to the path key; and identity uses the project root's
  own `.git`, so a managed root nested inside another repository never inherits
  the outer remote.
  Rationale: 267c reads records through the key, so a fallback would point the
  backlog at a different, empty store directory and a write there would split
  it. Requiring `.git` at the root also answers the nested-root case that
  `git -C` would otherwise resolve to the outer repository. The consequence
  267c must handle: store resolution must stay lazy where it is not needed, so
  a `project`-mode repository never fails because Git is unreadable.
  Date/Author: 2026-09-21, Travis Ennis.
- Decision: resolving the store is a read, and `ahm store path` records the
  observation — the registry entry and the project state file — unless
  `--dry-run` is given, so that identity data starts accumulating before a
  project migrates.
  Rationale: ADR 023 wants the registry to hold "what has been observed", which
  only happens if an ordinary command writes it, and 267a is the only store
  command before 267c. Both files are written only when their bytes change, and
  `--dry-run` writes nothing, so the command stays repeatable and previewable.
  Date/Author: 2026-09-21, Travis Ennis.
- Decision: a store directory is `<slug>-<hash8>`, where the slug is the key's
  last path segment reduced to lowercase alphanumerics and the hash is the
  first eight hex characters of the key's SHA-256; a path-rule key, which is a
  digest with no repository name, uses `project` as its slug.
  Rationale: naming a directory from the key alone keeps it stable across
  checkouts, a readable slug is what makes the store inspectable, and the hash
  is what keeps two repositories with the same name apart.
  Date/Author: 2026-09-21, Travis Ennis.
- Decision: the store root is `~/.ahm`, overridable with the absolute-path
  environment variable `AHM_HOME`, and it holds both the machine-level
  `config.json` and `projects/`.
  Rationale: one environment variable makes the test suite hermetic, and a
  visible dot directory is easy to inspect and to place under the user's own
  version control. `$XDG_DATA_HOME` was rejected because it is unset on macOS,
  so both spellings would resolve to a dot directory anyway.
  Date/Author: 2026-09-21, Travis Ennis.
- Decision: workflow paths carry two roots — a project root for
  `.ahm/config.json` and `docs/adr/`, and a records root for task records and
  their indexes — with named accessors, an `ownedRoots` containment check, and
  a display helper.
  Rationale: the safety guardrail requires every write to be scoped to an
  owned root. Naming the roots in one place is what makes that checkable
  instead of a convention spread across call sites.
  Date/Author: 2026-09-21, Travis Ennis.
- Decision: an owned root is the project root, not the ahm-owned directories
  inside it, so `writeOwned` accepts any path under the repository that a
  workflow write builds. `cleanupStaleTemps` deliberately does not follow this
  rule to the letter: it scans the workflow state directories — `<project
  root>/.ahm` and the store's project directory — rather than the whole project
  root, because walking the repository for `*.tmp` files would reap temp files
  the user owns.
  Rationale: the plan defines an owned root as the project root or the store
  root, and `ahm` already confines its project writes to `.ahm/` and
  `docs/adr/` by construction. Cleanup is the one write-adjacent operation
  whose blast radius is a recursive walk, so it is scoped more tightly than
  containment requires.
  Date/Author: 2026-09-21, Travis Ennis.
- Decision: `writeOwned` takes no file mode and writes every workflow file with
  0o644, instead of the `mode os.FileMode` parameter the Interfaces section of
  this plan sketched.
  Rationale: every workflow write is 0o644, and the repository's `unparam` lint
  rejects the unused parameter, so the signature would have to carry a
  suppression. `writeFileAtomic` keeps its mode because it is also the store
  and test primitive.
  Date/Author: 2026-09-21, Travis Ennis.
- Decision: the record-mutation lock lives in `lockDir()`, the directory
  containing the records root, rather than in `storePaths` as originally
  sketched.
  Rationale: the lock must sit beside the records wherever they are —
  `<project>/.ahm/.lock` in project mode, the store's project directory in home
  mode — and that is a property of the resolved paths, not of the store alone.
  Date/Author: 2026-09-21, Travis Ennis.
- Decision: 267b migrated path *construction* to the accessors and left
  path *display* in validation, task lookup, and lock errors on `relPath`.
  Rationale: those sites are unreachable until a command resolves home mode,
  which is 267c's change, and every one of them is byte-identical while the
  records stay in the project. The sites are the validation findings,
  `validateTaskBuckets` messages, `collectTasksForPaths` parse errors,
  `checkDuplicateTaskID`/`checkTaskDepsNotDuplicated` messages, the four task
  paths in `task_status.go` (the raw `Task.Path` field, the dry-run `move`
  preview, and the unblock preview's `path`), and lock timeouts. 267c must
  switch them to `displayPath` — otherwise findings, previews, and lock
  timeouts would render a store path against the project root — and the task
  record for 267c carries the same list.
  Date/Author: 2026-09-21, Travis Ennis.
- Decision: store paths are displayed as `store:` plus a store-relative path,
  and `prime` and `status` expose the store root, key, and mode as structured
  fields. In `project` mode, output is byte-identical to the behavior before
  this work.
  Rationale: absolute machine paths in output would make JSON unstable across
  machines, and byte-identical project-mode output is what lets this land
  without touching existing repositories.
  Date/Author: 2026-09-21, Travis Ennis.
- Decision: identity is derived per command from the canonical `origin` remote
  URL, falling back to the SHA-256 of the symlink-resolved project root. The
  registry maps key to directory and records observed remotes and paths.
  Rationale: deriving identity needs no project-side state and makes clones
  and worktrees share a list automatically. Recording observations is what
  makes a future adoption command possible at all, because a changed remote or
  a moved directory cannot be recognized retroactively.
  Date/Author: 2026-09-21, Travis Ennis.
- Decision: a relative Markdown link in a task record resolves against the
  record's own directory first and, only if that target does not exist,
  against the record's logical in-project directory.
  Rationale: this keeps every link written under the committed-in-project
  model working after a move, needs no new authoring convention, and preserves
  intra-store links such as sibling and cross-bucket references.
  Date/Author: 2026-09-21, Travis Ennis.
- Decision: existing repositories keep `project` mode; a repository with no
  `.ahm/config.json` is a new project and defaults to `home` when initialized.
  Rationale: this is the requested behavior — opt-in for what exists, the new
  default for what does not — and it needs no record heuristics because the
  configuration file is already the observable marker of an initialized
  repository.
  Date/Author: 2026-09-21, Travis Ennis.
- Decision: the storage mode lives in the committed `.ahm/config.json` as
  `tasks_location`, rather than in machine-local state.
  Rationale: `ahm` is used in single-maintainer repositories, so a repository-
  wide committed answer is simpler than per-machine overrides and keeps
  `ahm init` the single place that reconciles workflow state.
  Date/Author: 2026-09-21, Travis Ennis.
- Decision: the task ID counter is persisted in the store and never
  decremented.
  Rationale: Git history no longer proves that a deleted ID was once used, so
  a counter derived from the highest record present would eventually reissue
  an ID.
  Date/Author: 2026-09-21, Travis Ennis.
- Decision: cross-project references and cross-project listing are out of
  scope. `depends_on` keeps bare IDs.
  Rationale: `depends_on` is a durable record format, and a qualified-ID
  syntax would need to be designed, migrated, and validated for a use case the
  tool does not have.
  Date/Author: 2026-09-21, Travis Ennis.
- Decision: this work lands in v2.0.0, before the release task 264f.
  Rationale: the release is already a breaking-change release, so the new
  default for new projects costs nothing extra to explain, and shipping the
  storage change in a minor release afterward would mean two migration notes.
  Date/Author: 2026-09-21, Travis Ennis.
- Decision: milestone 267c resolves a repository with no configuration as
  `project`, and 267f changes `resolveTaskLocation`'s no-configuration branch to
  `home` and writes the key. The storage mode otherwise comes from
  `tasks_location`: `home` uses the store, and a missing key or an unrecognized
  value keeps the records in the project.
  Rationale: the milestone's acceptance requires byte-identical project-mode
  output and layout, and a bare checkout with no configuration would otherwise
  resolve a store it cannot own yet. Keeping the branch in place and flipping
  one return value in 267f is a smaller change than a second resolution path.
  Date/Author: 2026-09-21, Travis Ennis.
- Decision: the store's managed `.gitignore` lives in the store's project
  directory, `<store>/projects/<slug>-<hash>/.gitignore`, not at the store root,
  and the store root stays outside the owned roots. It ignores the generated
  task indexes, the lock directory, temp files, and `project.json`, the store's
  own state file, which holds store-local state and changes with almost every
  task mutation. `registry.json` is left commit-visible, because the
  key-to-directory mapping is derived data that can be recomputed.
  Rationale: 267b's containment already accepts the store's project directory,
  so the per-project location needs no change to the ownership boundary, and the
  file only has to ignore the machine-local state that sits beside the records.
  Date/Author: 2026-09-21, Travis Ennis.
- Decision: record paths in findings, error messages, index listings, directory
  labels, and lock errors render through `displayPath`, while the JSON `path`
  field of a task and the dry-run create, move, and unblock previews render
  through `payloadPath`, which is `displayPath` for a store path and the
  record's own path otherwise.
  Rationale: the two payload families carry the absolute record path today, so
  routing them through `displayPath` would change project-mode output that ADR
  023 and this milestone's acceptance both promise stays byte-identical. Every
  other site already printed a project-relative path, so `displayPath` leaves it
  byte-identical and turns a store path into `store:<rel>`. An operating-system
  error keeps its own text, which can name an absolute path in either layout.
  Date/Author: 2026-09-21, Travis Ennis.
- Decision: the drift findings are error-tier and read-only. `task_records_in_project`
  aggregates every project bucket that still holds records into one finding on
  `.ahm/tasks` that names the count, and `store_dir_unreadable` reports a missing
  or unreadable store records directory. Both run in the disk-reading validation
  path — `status`, `doctor`, and `prime` — and in the fallback to that path that
  a mutation or `ahm index` takes when a task parse was partial; they never run
  in the post-mutation state validation that follows a clean write.
  Rationale: records left in the project are invisible to every command, and a
  missing store means the backlog cannot be read, so both are failures rather
  than advice; keeping them out of the clean post-mutation path keeps ordinary
  mutations quiet while a person is mid-migration.
  Date/Author: 2026-09-21, Travis Ennis.
- Decision: commands resolve the records layout once, in `detectRoot` /
  `detectRootOrCWD`, and `install` re-resolves it from the configuration it owns
  through `app.useWorkflowPaths`.
  Rationale: resolution can fail (an unreadable Git repository, a corrupt
  registry, a relative `AHM_HOME`) and a command must fail before it touches
  anything, while the lazy `workflowPaths()` accessor keeps helpers that cannot
  return an error working with the project layout. `install` resolves its own
  layout because the mode it writes is the mode it must lay directories down
  for, which is the seam 267f needs when it starts writing `tasks_location`.
  Date/Author: 2026-09-21, Travis Ennis.
- Decision: the delete side of a record move stays a direct `os.Remove` of the
  vacated record path, with no owned-root check of its own, and 267e owns the
  decision about whether deletion needs one.
  Rationale: 267c was asked to confirm the delete side, and the path it removes
  is the record path the task scan just produced under the resolved records
  root, so it is confined by construction; every other workflow write still goes
  through `writeOwned`, and 267e is the first command that deletes records on
  purpose rather than because one moved.
  Date/Author: 2026-09-21, Travis Ennis.

## Outcomes & Retrospective

### 267c — Read and write task records in the home store (2026-09-21)

Delivered: `metadata` gained `TasksLocation`, written and consumed by the
hand-written `MarshalJSON` and removed from the unknown-field map on read, plus
`resolveTaskLocation(meta, configExists)` and `resolveWorkflowPaths(root, meta,
configExists)`. `app` resolves the records layout once after root detection and
caches it; `workflowPaths()` is the lazy accessor for helpers that cannot return
an error, and `install` re-resolves the layout it is about to write. Task reads
and writes, the four task indexes, the lock, and the managed `.gitignore` all
follow the records root, while `.ahm/config.json` and `docs/adr/` stay in the
project. `prime` and `status` report the store root, key, kind, and location
only when the records live in the store, and the drill-down findings, error
messages, index listings, and lock timeouts render record paths through
`displayPath`. Validation gained `task_records_in_project` and
`store_dir_unreadable`, and Markdown link validation resolves a relative link
against the record's own directory first and the record's logical in-project
directory second.

Against the milestone's acceptance: with `tasks_location: home`, `ahm task
create`, `list`, `show`, `complete`, and `dep` operate entirely in the store and
write nothing under `.ahm/tasks/`, which
`TestHomeModeKeepsTaskRecordsInTheStore` drives through the whole lifecycle
(create, list, show, `--json` show, dependent create, dry-run complete, real
complete, indexes, and the store's `.gitignore`). `ahm doctor` and `ahm status`
report `task_records_in_project` for a record left in the project while the mode
is `home`, and `store_dir_unreadable` when the store records directory is
missing. A store record linking to `../../../docs/adr/001-probe.md` still
validates after the move, a sibling link resolves from the record's own
directory, and a broken link is still reported. With the key absent the entire
suite passes with project-mode records, output, and layout unchanged:
`TestProjectModeOutputHasNoStoreField` pins the absent store field for `status`,
`prime`, and `doctor`, and `TestWorkflowPathsPayloadPathKeepsProjectPaths` pins
the absolute project path in the structured payloads.

The judgment calls are recorded above. The no-configuration branch of
`resolveTaskLocation` stays `project` until 267f turns it on, because resolving
a repository with no configuration as `home` splits its records between the run
that creates the configuration and every run after it. Structured payloads use
`payloadPath` rather than `displayPath`, because they carry the record's
absolute path today and project-mode output is byte-identical by contract. The
store's managed `.gitignore` sits in the store's project directory and ignores
the generated indexes, the lock, temp files, and the store state file, so the
ownership boundary did not change; 267h owes the ADR and guide wording for that
choice, the location of the file, and the fact that `registry.json` stays
commit-visible.

A review round found one real defect: `ahm --json task list` emitted raw task
records, so a home-mode listing leaked the absolute store path even though
`task show` and `task search` did not. The list path now renders through
`tasksForOutput`, and `TestHomeModeKeepsTaskRecordsInTheStore` covers `task
list` and `task search`. The same round corrected the evidence cited for the
`resolveTaskLocation` decision, the drift findings' scope, and the plan's claim
that ADR 023 named the store's `.gitignore` location.

### 267b — Split the roots and contain writes (2026-09-21)

Delivered: `workflowPaths` is now a two-root type with the named accessors,
`ownedRoots`, `stateRoots`, `lockDir`, and `displayPath`, plus
`workflowPathsFor` (project mode) and `workflowPathsForStore` (store mode);
`writeOwned` in `internal/ahm/write.go` is the containment point, and every
workflow write routes through it; `cleanupStaleTemps` takes the resolved paths
and scans `<project root>/.ahm` plus the store's project directory;
`acquireWorkflowRecordLock` takes the resolved paths and puts the lock beside
the records root. Call sites that used to join `a.opts.root` were migrated to
the accessors, and path display in the index, install, task-create, comment,
and prime surfaces now goes through `displayPath`. `ARCHITECTURE.md` records the
two roots, the containment primitive, and the lock's new location, and the
safety guardrail names `writeOwned` as the containment point.

Against the milestone's acceptance: nothing user-visible changed. Records still
live in `.ahm/tasks/`, the lock is still `<project root>/.ahm/.lock`, and the
full suite passes with no assertion about paths, output, or on-disk layout
changed except the mechanical updates for the renamed `recordsRel` accessor and
the re-signatured `cleanupStaleTemps`, `writeOwned`, and
`acquireWorkflowRecordLock` helpers. New coverage:
`TestWriteOwnedContainsWritesToTheOwnedRoots` (a target under each owned root is
written, a target outside every root is refused and creates nothing),
`TestPathWithin` (prefix siblings, `..` traversal, relative against absolute),
`TestWorkflowPathsProjectModeAccessors` and
`TestWorkflowPathsHomeModeAccessors` (both layouts, including the lock and
display conventions), and `TestCleanupStaleTempsCoversEveryStateRoot`. A review
round supplied 13 mutants against the new tests; 12 were caught, and the
survivor is equivalent.

Deliberately left to 267c, and recorded in the Decision Log: path *display* in
validation findings, `checkDuplicateTaskID` messages, the `collectTasksForPaths`
parse-error message, the raw `Task.Path` field and the dry-run preview payloads
in `task_status.go`, and lock timeouts still relativize a record path against
the project root. All are unreachable until a command resolves home mode, and
all are byte-identical in project mode. Also left open: the delete side has no
containment counterpart (`task_status.go` removes a vacated record path
directly). 267e's migration is the first command to delete records on purpose
and should decide whether that needs its own owned-root check.

### 267a — Derive project identity and resolve the store root (2026-09-21)

Delivered: `canonicalRemoteKey`, `canonicalPathKey`, `projectKeyFor`, and
`storeDirName` in `internal/ahm/identity.go`; `storeRoot`, `resolveStore`, the
registry and project-state read/write helpers, and the `store path` command in
`internal/ahm/store.go`; a `readGitRemote`/`runGit` read in
`internal/ahm/git.go`; and `identity_test.go` plus `store_test.go`. The
golden table covers scp-like and URL spellings of one repository, uppercase,
credentials, default and non-default ports, `file://` and local-path remotes,
and a symlinked project root.

Against the Purpose section: nothing user-visible changed for an existing
repository. Records still live in `.ahm/tasks/`, every existing command
produces what it produced before, and the store is only observable through a
new command. The store's write surface so far is the registry entry and the
project state file that `ahm store path` records.

A review round hardened the identity rules: an unreadable Git repository is an
error rather than a silently different path key, identity uses the project
root's own `.git`, a remote that cannot key the project is still recorded as an
observation, and a registry-supplied directory is confined to one path segment.
The test helpers now install a scratch store root unless the test chose one
under the system temporary directory, so no test can reach a real store.

Not yet done, and deliberately: the store holds no records, the mode key is not
read or written, and `prime`/`status` do not report the store. Those are
267b through 267f.

## Context and Orientation

`ahm` is a Go CLI. The module is `github.com/travisennis/ahm`, the entrypoint
is `cmd/ahm/main.go`, essentially all behavior lives in the package
`internal/ahm/`, and `internal/version/version.go` holds the version string
that release builds inject. Tests live beside the code in `internal/ahm/`. The
task runner is `just`; `just test` runs `go test ./...`, `just quick` adds
`go vet`, and `just ci` is the full gate (`gofmt`, `go mod tidy -diff`, vet,
race tests, `golangci-lint`, `govulncheck`, markdownlint, and a GoReleaser
snapshot build).

### Terms

- **Project root** — the repository a command was pointed at, resolved by root
  detection from `.git` or `.ahm/config.json`.
- **Records root** — the directory that holds task records and their generated
  indexes. Today it is always `<project root>/.ahm/tasks`; after this work it
  is either that, in `project` mode, or `<store>/projects/<slug>-<hash>/tasks`
  in `home` mode.
- **Home store**, or **store** — the user-level directory tree rooted at
  `~/.ahm` (or `$AHM_HOME`) that holds the machine-level `config.json`,
  `registry.json`, and one directory per project.
- **Project key** — the canonical string identifying a project: either a
  canonical remote URL such as `github.com/travisennis/ahm`, or the hex digest
  of the symlink-resolved project root when no remote applies.
- **Registry** — `<store>/registry.json`, which maps each project key to its
  directory name and records the remotes and absolute paths seen for that key.
  It is derived data, and it can be rebuilt by scanning, but it is the
  authority for key-to-directory mapping.
- **Mode** — the value of the `tasks_location` key in `.ahm/config.json`,
  either `project` or `home`. A missing key means `project`.
- **Drift finding** — a read-only validation finding reporting that the mode
  and the records on disk disagree, for example `home` mode with task records
  still under `.ahm/tasks/`.
- **Owned root** — a directory a command is allowed to write under: the
  project root, or the store root. Every workflow write must land under one of
  them.
- **Store format version** — a version recorded in the store so that a future
  change to the store layout can refuse an unknown store instead of
  half-reading it, the same way root detection refuses the retired
  `.agents/ahm.json` layout today.

### What exists today

Root detection is `detectRoot`, `detectRootOrCWD`, `detectManagedRoot`, and
`rejectLegacyLayout` in `internal/ahm/root.go`. It walks up from the working
directory and accepts the first directory holding `.git` or `.ahm/config.json`.

Record paths come from `internal/ahm/workflow_paths.go`:

    type workflowPaths struct {
        root string
    }

    func (p workflowPaths) tasksRel() string                    // ".ahm/tasks"
    func (p workflowPaths) tasksBucketDir(bucket string) string  // <root>/.ahm/tasks/<bucket>
    func (p workflowPaths) taskFile(bucket, id string) string

Everything that touches records goes through those methods: `collectTasksForPaths`
and `taskFilePathsFor` in `internal/ahm/tasks.go`, `parseTask` and `renderTask`
in the same package, the lifecycle commands in `internal/ahm/task_*.go`, the
generated indexes in `internal/ahm/indexes.go`, link validation in
`internal/ahm/validation.go`, and the record lock in `internal/ahm/lock.go`.

Committed configuration is `metadata` in `internal/ahm/install.go`, which has a
hand-written `MarshalJSON` and an `UnmarshalJSON` that deletes known keys so
they do not survive in the `Extra` map. Adding a field means editing both.

`ahm init` runs `install()` in `internal/ahm/install.go`: it reads and
reconciles `metadata`, creates `.ahm/tasks/{active,completed,cancelled}` and
`docs/adr`, reconciles the managed `.ahm/.gitignore`, and regenerates indexes
only when their bytes differ.

Index generation is `indexWritesForPaths` in `internal/ahm/indexes.go`. It
returns five writes: four task indexes under the task directory and
`docs/adr/index.md` under the project root. Task index links are relative to
the task directory (`active/001.md`), and the ADR index links only to sibling
ADR files, which is why mirroring the in-project layout in the store keeps
every generated link working.

Link validation is `validateMarkdownLinks` and `validateMarkdownFileLinks` in
`internal/ahm/validation.go`. The candidate file list comes from
`workflowMarkdownFilesForPaths`, which enumerates the three task buckets, every
ADR file, and the five generated indexes. `validateMarkdownFileLinks` resolves
a relative target against the directory of the file being validated.

`ahm prime` builds its report in `internal/ahm/prime.go` and is the only place
that runs Git (`git status --short --branch`, whose output supplies the branch
and dirty-worktree fields). `internal/ahm/git.go` holds
`cleanGitEnvironment`, which strips inherited Git repository-location
variables from a Git subprocess; every new Git read must go through it, per
ADR 018.

Writes go through `writeFileAtomic` in `internal/ahm/write.go`; the safety
guardrail states that it is an atomicity primitive, not a containment check.
`cleanupStaleTemps` in the same file walks `<project root>/.ahm` for orphaned
`.tmp` files.

The generated index list and the retired-path list are `generatedIndexTargets`
in `internal/ahm/indexes.go`. The managed `.ahm/.gitignore` content is
`recordsGitignoreContent` and `recordsGitignoreEntries` in
`internal/ahm/install.go`.

Tests call the CLI in process through `runCLI` and `runCLIFromDir` in
`internal/ahm/test_helpers_test.go`, and also as a real child process through
`runBuiltCLI` in `internal/ahm/cli_integration_test.go`. No test in the
repository calls `t.Parallel()`, which is what makes `t.Setenv` usable for
hermetic store roots.

### What changes

Identity and the store are resolved once per command, after root detection, and
produce a `storePaths` value. `workflowPaths` gains a project root, a records
root, the resolved store, and the mode. Task commands read and write through
the records root. The lock, stale-temp cleanup, the ID counter, and the task
indexes all move with the records. `docs/adr/` and `.ahm/config.json` stay in
the project. Validation walks both roots and reports drift when they disagree.

## Plan of Work

The milestones are ordered so that the tree stays green and `project` mode
stays the effective default until milestone 267f. Every milestone updates the
tests and documentation it touches; 267h consolidates the documentation and
hands the release-note input to 264f.

### 267a — Derive project identity and resolve the store root

New file `internal/ahm/identity.go` with remote canonicalization, the path
fallback, and the directory-name derivation. New file `internal/ahm/store.go`
with `storeRoot` (reads `AHM_HOME`, falls back to `~/.ahm`, and rejects a
relative value), `resolveStore`, the registry load and write helpers, and the
store's `project.json` state file. New tests
`internal/ahm/identity_test.go` and `internal/ahm/store_test.go`, including a
golden table for canonicalization: scp-like and URL remotes agreeing,
uppercase lowercased, credentials stripped, default ports dropped,
non-default ports kept, `file://` and local-path remotes falling back to the
path rule, and a symlinked project root resolving to one key.

`internal/ahm/git.go` gains a helper that reads `origin` through
`cleanGitEnvironment`; `internal/ahm/cli.go` wires store resolution into
`app` after `detectRoot`; the `store` command group arrives with a single
`path` subcommand that prints the root, key, and records directory, and that
serializes the same fields under `--json`.

Acceptance: `ahm store path` in two clones of one repository prints identical
keys and directories; a repository with no remote prints a `path`-kind key;
`AHM_HOME` redirects the root and a relative value is a usage error.

### 267b — Split the roots and contain writes

`internal/ahm/workflow_paths.go` becomes the two-root type with accessors
(`configPath`, `adrDir`, `recordsRel`, `tasksBucketDir`, `taskFile`),
`ownedRoots`, and `displayPath`. `internal/ahm/write.go` gains
`writeOwned(paths workflowPaths, path string, data []byte)`, which refuses a
path outside the owned roots and then calls `writeFileAtomic`;
`cleanupStaleTemps` walks the workflow state directories of both roots.
`internal/ahm/lock.go` takes its lock directory from the records root. Call
sites migrate from string joins on `a.opts.root` to the accessors.

This milestone changes no user-visible behavior, which is its acceptance
criterion: the full test suite passes, `go test ./internal/ahm/ -run TestInit`
or the equivalent install tests still produce identical bytes, and a new test
asserts that a write outside the owned roots is refused.

### 267c — Read and write tasks in the store

`metadata` gains `TasksLocation`, edited in both `MarshalJSON` and the
`UnmarshalJSON` delete list, plus `resolveTaskLocation(meta metadata,
configExists bool) taskLocation`. `install()` creates the store project
directory and the store's managed `.gitignore` when the mode is `home`, and
never recreates `.ahm/tasks/`. Index writes split: task indexes under the
records root, the ADR index under the project root. `prime` and `status` gain
the structured store fields and display store paths with the `store:` prefix.
`validation.go` gains the drift finding for records left in the project when
the mode is `home`, and for a mode of `home` when the store project directory
cannot be read.

Acceptance: a configuration with `tasks_location: home` makes
`ahm task create`, `list`, `show`, and `complete` operate entirely in the
store, writing nothing under `.ahm/tasks/`; `ahm doctor` reports the drift
finding when a record is left behind; and, with the key absent, the whole
suite passes with byte-identical output.

### 267d — Persist the task ID counter

`nextTaskID` in `internal/ahm/task_create.go` currently derives the next ID
from the maximum parsed ID plus the filesystem entries. It reads and updates a
counter in the records root's `project.json` instead, taking the maximum of
the persisted value and the highest record present so that an existing store
self-heals upward, and never decreasing it. Child ID allocation under
`--parent` keeps its current rules against the same counter and the same
scan.

Acceptance: create a task, delete its file by hand, create another; the second
ID is one higher than the deleted one rather than a reuse. A test that fails
before the change demonstrates the reuse.

### 267e — Add the migration command

New file `internal/ahm/store_migrate.go` with
`ahm store migrate --to home|project`. The command requires `--to`, holds the
records lock, moves each record with a read, an atomic write into the
destination, and a removal of the source, never a cross-device rename, so it
is resumable and idempotent. It refuses when the destination holds records for
a different project key, and it refuses when records under `.ahm/tasks/` have
uncommitted changes unless `--force` is given, because Git history is the only
backup after the move. It updates `tasks_location`, rewrites the managed
`.ahm/.gitignore` for the new mode, regenerates indexes on both sides, records
the move in the registry as `migrated_from`, and prints the deletions the user
must commit. `--dry-run` previews every source, destination, deletion, and
index change without writing anything and without creating the store.

Acceptance: a dry run on a scratch repository lists the moves and changes
nothing on disk; a real run leaves the records in the store and the deletions
visible in `git status --short`; a second run reports no work; a reverse move
`--to project` restores the original layout; interrupting a move and re-running
it completes the move.

### 267f — Default new projects to the home store

`install()` writes `tasks_location: home` when it creates configuration for a
repository that has none, and preserves an existing value otherwise. Root
detection is unchanged: a directory with neither `.git` nor `.ahm/config.json`
still needs `ahm init`, and a no-Git directory works after that because its
key falls back to the path rule.

Acceptance: `git init` plus `ahm init` in a fresh directory puts tasks in the
store; a repository that already has configuration keeps its records in the
project; `ahm init` on a `home` repository never creates `.ahm/tasks/`.

### 267g — Migrate this repository

Run `ahm store migrate --to home` in this repository, then verify
`ahm prime`, `ahm doctor`, `ahm status`, and `ahm task list` against the store,
and confirm the records are absent from the tree and present in the store.
This is the first real use and the shakedown for the guide: any friction
becomes a fix in this milestone or a new task. The ExecPlan, the ADR, and
`docs/` stay in the repository, so the link-resolution rule is exercised by
real records.

Acceptance: this repository's backlog is served from the store, `git status`
shows only the intended deletions, and `ahm doctor` reports no findings.

### 267h — Documentation and release notes

Update `docs/references/workflow-spec.md` (store layout, identity, mode key,
link resolution, display convention, and the ownership boundary),
`docs/guides/workflow-upgrades.md` (how to opt in, what to commit, what to do
about leftover ignored indexes), `docs/cli.md` and
`docs/references/cli/commands.md`, `task-commands.md`, and
`global-contract.md` (the `store` group, the `init` flag, and the store fields
in output), `ARCHITECTURE.md` (module map, invariants, and the second owned
root), `docs/references/glossary.md` (the new terms), `docs/workflow/tasks.md`
(the storage section), and the `AGENTS.md` sentence about branch-scoped
records. Then report to 264f the release-note items this work owes: the new
default for new projects, the `tasks_location` key, the `ahm store` group, and
the fact that existing repositories are unaffected until migrated.

Acceptance: `just docs-md-lint` passes; no document still claims that task
records are always committed under `.ahm/tasks/` or branch-scoped; the CLI
reference documents every new command and flag.

## Concrete Steps

All commands run from the repository root unless stated otherwise. The examples
use `~/Projects/ahm` as the project path.

Start each milestone by reading the tracker and the child task. Run
`just quick` while working (it is `go test ./...` followed by `go vet ./...`),
and run the full gate `just ci` before handing the milestone off.

Every test that exercises a command must set a temporary store root. In-process
tests get it in the helpers:

    func runCLI(t *testing.T, args ...string) (string, string, int) {
        t.Helper()
        t.Setenv("AHM_HOME", t.TempDir())
        ...
    }

The child-process harness must set the environment explicitly, because it
inherits the developer's:

    cmd.Env = append(os.Environ(), "AHM_HOME="+t.TempDir())

No test in this repository calls `t.Parallel()`. If a later test does, it must
not use `t.Setenv`; pass the store root through the `app` value instead.

To watch 267a work, run it in two clones of one repository and compare:

    $ ahm store path
    root:    ~/.ahm
    key:     github.com/travisennis/ahm
    records: ~/.ahm/projects/ahm-3f9ac4d1/tasks

    $ ahm --json store path
    {"root":"/Users/you/.ahm","key":"github.com/travisennis/ahm","kind":"remote","records":"/Users/you/.ahm/projects/ahm-3f9ac4d1/tasks"}

To watch 267e work, use a scratch repository so nothing important is at risk:

    $ mkdir -p /tmp/ahm-migrate && cd /tmp/ahm-migrate
    $ git init -q && ahm init && ahm task create "First"
    $ AHM_HOME=/tmp/ahm-home ahm --dry-run store migrate --to home
    would move .ahm/tasks/active/001.md -> store:tasks/active/001.md
    would update .ahm/config.json, .ahm/.gitignore, and 1 index
    $ AHM_HOME=/tmp/ahm-home ahm store migrate --to home
    moved 1 record to store:tasks/active/001.md
    $ git status --short
     D .ahm/tasks/active/001.md

## Validation and Acceptance

The behavior to observe, milestone by milestone, is the acceptance text in
`Plan of Work`. Beyond those, the following must hold at the end.

Running `just ci` from the repository root passes, including the race tests,
`golangci-lint`, `govulncheck`, markdownlint, and the GoReleaser snapshot
build. `ahm doctor` reports no findings in this repository after 267g.
`ahm --dry-run init` writes nothing, and neither does `ahm --dry-run store
migrate`, including creating no store directory.

Existing repositories are protected by a test that runs the install and task
lifecycle against a configuration with no `tasks_location` key and compares
its output and on-disk layout to the layout produced before this work. That
test is the guard for the "opt-in for existing repositories" requirement and
must not be weakened to accommodate a later change.

Store behavior is covered by tests that exercise: identity resolution across
two clones of one repository, a repository with no remote, a repository whose
symlinked path resolves to one key, the containment refusal in `writeOwned`,
the drift finding for records left in the project, the counter's
non-decrementing property, a full `--to home` and `--to project` round trip,
and an interrupted move that is completed by re-running the command.

## Idempotence and Recovery

`ahm init` and `ahm index` remain idempotent: they write only when the bytes
on disk differ from the bytes they own.

`ahm store migrate` is idempotent and resumable. Each record is read, written
atomically into the destination, and then removed from the source, so a crash
or an interrupt leaves records on both sides, and re-running the command
finishes the move. The command never uses a rename across filesystems, because
the store and the project may be on different volumes.

Before the user commits, `git restore` returns the working tree to its
pre-migration state. After the user commits, `git revert` of the migration
commit restores the in-project layout. `ahm` never stages, commits, or moves
`HEAD`, so both recovery paths are ordinary Git operations the user runs.

If the store is lost, the task list is lost. That is the accepted tradeoff of
ADR 023. A user who wants a copy may place the store under their own version
control; `ahm` runs no Git commands against it and writes a managed
`.gitignore` in the store's directory for the project that ignores the generated
task indexes, the lock directory, temporary files, and the store state file. The
store root stays outside the owned roots, so `registry.json` is left
commit-visible; it is derived data whose mapping can be recomputed from the
project keys.

## Artifacts and Notes

The store layout to create, with the in-project layout for comparison:

    ~/.ahm/                                   <project>/
      config.json                               .ahm/config.json
      registry.json                             .ahm/.gitignore
      projects/                                 .ahm/tasks/            (home mode: absent)
        ahm-3f9ac4d1/                           docs/adr/index.md
          .gitignore
          project.json          { "next_id": 268 }
          .lock/workflow-records
          tasks/index.md
          tasks/active/index.md
          tasks/active/267.md

The registry shape:

    {
      "version": 1,
      "projects": {
        "github.com/travisennis/ahm": {
          "kind": "remote",
          "dir": "ahm-3f9ac4d1",
          "remotes": ["git@github.com:travisennis/ahm.git"],
          "paths": ["/Users/you/Projects/ahm"],
          "created": "2026-09-21T10:00:00-04:00",
          "migrated_from": "project"
        }
      }
    }

## Interfaces and Dependencies

New file `internal/ahm/identity.go` defines the identity rules:

    func canonicalRemoteKey(rawRemote string) (key string, ok bool)
    func canonicalPathKey(projectRoot string) (key string, err error)
    func projectKeyFor(projectRoot string) (key string, kind string, rawRemote string, err error)
    func storeDirName(key string) string

`canonicalRemoteKey` returns `ok == false` for a `file://` URL or a local-path
remote, which makes the caller use `canonicalPathKey`.

New file `internal/ahm/store.go` defines the store:

    type storePaths struct {
        Root       string // ~/.ahm or $AHM_HOME
        Key        string
        Kind       string // "remote" or "path"
        ProjectDir string // <Root>/projects/<slug>-<hash>
    }
    func storeRoot() (string, error)
    func resolveStore(projectRoot string) (storePaths, error)
    func (s storePaths) statePath() string
    func (s storePaths) recordsDir() string
    func (s storePaths) dirName() string

The registry path is derived where the registry is read and written, and the
record-mutation lock directory is `workflowPaths.lockDir()`, because the lock
belongs beside the records wherever they are rather than to the store alone.

    type projectEntry struct {
        Key          string   `json:"key"`
        Kind         string   `json:"kind"`
        Dir          string   `json:"dir"`
        Remotes      []string `json:"remotes,omitempty"`
        Paths        []string `json:"paths,omitempty"`
        Created      string   `json:"created"`
        MigratedFrom string   `json:"migrated_from,omitempty"`
    }
    type registry struct {
        Version  int                     `json:"version"`
        Projects map[string]projectEntry `json:"projects"`
    }
    func loadRegistry(root string) (registry, error)

`internal/ahm/workflow_paths.go` becomes:

    type taskLocation string
    const (
        locationProject taskLocation = "project"
        locationHome    taskLocation = "home"
    )

    type workflowPaths struct {
        projectRoot string
        recordsRoot string
        store       storePaths  // zero value when the records live in the project
        mode        taskLocation
    }
    func workflowPathsFor(root string) workflowPaths            // project mode, used by tests and existing call sites
    func workflowPathsForStore(root string, store storePaths) workflowPaths
    func resolveWorkflowPaths(root string, meta metadata, configExists bool) (workflowPaths, error)
    func (p workflowPaths) configPath() string
    func (p workflowPaths) adrDir() string
    func (p workflowPaths) adrIndexPath() string
    func (p workflowPaths) recordsRel() string                  // ".ahm/tasks" or "tasks"
    func (p workflowPaths) inStore() bool
    func (p workflowPaths) tasksBucketDir(bucket string) string
    func (p workflowPaths) taskFile(bucket string, id string) string
    func (p workflowPaths) ownedRoots() []string
    func (p workflowPaths) stateRoots() []string               // where cleanupStaleTemps scans
    func (p workflowPaths) lockDir() string
    func (p workflowPaths) displayPath(path string) string

`internal/ahm/install.go` gains the mode in committed configuration:

    type metadata struct {
        Version       string            `json:"version,omitempty"`
        StrictAcceptance bool           `json:"strict_acceptance"`
        TasksLocation string            `json:"tasks_location,omitempty"`
        Files         map[string]string `json:"files"`
        Extra         map[string]json.RawMessage `json:"-"`
    }
    func resolveTaskLocation(meta metadata, configExists bool) taskLocation

`internal/ahm/write.go` gains the containment check:

    func writeOwned(paths workflowPaths, path string, data []byte) error

`internal/ahm/store_migrate.go` gains the migration:

    func (a *app) storeMigrate(to taskLocation, force bool) error
    func moveTaskRecords(from workflowPaths, to workflowPaths) (moved []string, err error)

No new dependencies. `ahm` continues to run exactly one class of subprocess,
Git, and adds only a read of the `origin` remote URL through the existing
`cleanGitEnvironment` filter from ADR 018.

## Change Notes

- 2026-09-21: Initial version, written before implementation. Records the
  decision set agreed with the maintainer: the store root and `AHM_HOME`, the
  two-root split, the `store:` display convention, derived identity with a
  registry, the link-resolution order, opt-in for existing repositories with a
  `home` default for new ones, the committed `tasks_location` key, and the
  persisted ID counter.
