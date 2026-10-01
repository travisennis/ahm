# Add a `--project` selector for cross-project record access

This ExecPlan is a living document. The sections `Progress`, `Surprises &
Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to
date as work proceeds. It is maintained in accordance with
`docs/workflow/exec-plans.md`.

## Purpose / Big Picture

Today an `ahm` command always resolves its task records from a *checkout*: it
walks up from the current working directory to a `.git` directory or
`.ahm/config.json`, or takes `--root <path>`. In `home` mode the records
themselves live under the user-level store (`~/.ahm/projects/<slug>-<hash8>/`)
keyed by the project's identity, but the *only* way to name a project is still
a checkout path. An agent that is sandboxed inside one project cannot reach
another project's records at all: `ahm --root /other/project` must read the
target checkout's Git remote to derive the project key, which the sandbox
denies.

After this change a person or agent can name a project by its store key or its
store directory name and reach that project's task records from anywhere,
without a checkout and without reading the target directory:

    ahm --project ahm task create "Follow up on the release notes"
    ahm --project ahm task list --status pending
    ahm --project ahm status

The first command writes the new record into
`~/.ahm/projects/ahm-0242d8b9/tasks/active/` and touches no file in the
directory it ran from. `--project` selects *records* only: it overrides where
task records are found and written, while `--root` keeps its meaning for
checkout-relative work (initializing a repository, regenerating project-owned
indexes, and ADRs). The two flags are mutually exclusive.

## Progress

- [x] (2026-10-01) Write the ExecPlan (this document).
- [x] (2026-10-01) Add the `--project` global flag and the selector resolver
      (`resolveProjectSelection`).
- [x] (2026-10-01) Resolve a records-only workflow layout from `--project` in
      root detection (`selectProject`, `workflowPathsForSelection`).
- [x] (2026-10-01) Gate project-scoped reads, writes, index generation, and
      validation on the records-only layout.
- [x] (2026-10-01) Refuse `--project` for checkout-relative commands (`init`,
      `index`, `adr`, `store migrate`) and reject `--project` together with
      `--root`.
- [x] (2026-10-01) Add tests: selector resolution, exit codes, records-only
      create, status, mutual exclusion (`internal/ahm/project_selector_test.go`).
- [x] (2026-10-01) Update `docs/references/cli/global-contract.md`,
      `docs/cli.md`, and the glossary; `just cli-parity` passes.
- [x] (2026-10-01) Run review and preflight; address findings.

## Surprises & Discoveries

- Observation: the store registry is populated by `ahm store path` even for a
  project whose records still live in the project (`storePath` calls
  `recordStoreProject` unconditionally), so a registry entry does **not** imply
  the project is in `home` mode.
  Evidence: `internal/ahm/store.go` `storePath` → `recordStoreProject`, and
  `storePath` runs for any detected root.

  Consequence: `--project` always resolves records into the store directory for
  the selected key. A registered project whose records are in the project will
  therefore show an empty/absent store records directory. This is documented as
  the meaning of the flag rather than guessed around, and `validateStoreReadable`
  reports the missing directory.

- Observation: `strict_acceptance` and the generated ADR index live in the
  committed project, so a records-only command cannot read or write them.
  Evidence: `task_status.go` reads `.ahm/config.json` for strict acceptance;
  `indexes.go` renders `docs/adr/index.md`. Both are gated on
  `isRecordsOnly()`.

  Consequence: `--project` deliberately does not apply strict acceptance, and
  `ahm index` (which owns the ADR index) is refused under `--project`. The store
  task indexes are still generated and validated, and a records-only task
  mutation regenerates them.

## Decision Log

- Decision: `--project` is a global (root persistent) flag, mutually exclusive
  with `--root`; combining them is a usage error (exit 2).
  Rationale: the task asks for a global selector and `gh --repo`-style
  ergonomics; mutual exclusion removes the question of which one wins.
  Date/Author: 2026-10-01 / agent for Trav.

- Decision: the selector matches the registry key exactly first, then a unique
  case-insensitive substring of the key or of the registry directory name
  (`<slug>-<hash8>`). Zero matches and more than one match are usage errors
  that list the candidates.
  Rationale: exact key is unambiguous; the directory slug is the friendly
  human handle the store already prints; an ambiguous or unknown selector must
  fail loudly with the choices rather than guess.
  Date/Author: 2026-10-01 / agent for Trav.

- Decision: a `--project` selection produces a *records-only* workflow layout:
  `workflowPaths.recordsOnly == true`, `recordsRoot` is the store's records
  directory, `mode` is `home`, and `projectRoot` is the selected project's
  recorded path (fallback: the current working directory) used only for display.
  Rationale: the layout needs to name the project's records without a checkout;
  the recorded path gives `status`/`prime` an informative `root`, while every
  project-scoped read and write is suppressed by the `recordsOnly` flag so the
  target directory is never read and never written.
  Date/Author: 2026-10-01 / agent for Trav.

- Decision: `ownedRoots` and `stateRoots` exclude `projectRoot` when
  `recordsOnly` is set, so a records-only command can only ever write inside the
  store's project directory.
  Rationale: containment is the safety boundary (`writeOwned`); including a
  target checkout as an owned root would let a records-only write reach it.
  Date/Author: 2026-10-01 / agent for Trav.

- Decision: generated-index generation and validation skip every
  project-scoped step in records-only mode: the ADR list and ADR index, the
  committed metadata, the records-in-project drift check, ADR validation, and
  ADR/project Markdown links. Task records, task buckets, and the four generated
  task indexes are still produced and validated.
  Rationale: those steps read or write under `projectRoot`, which a records-only
  command must not touch; task records are the whole point of the flag.
  Date/Author: 2026-10-01 / agent for Trav.

- Decision: `status`, `doctor`, `prime`, `store path`, and every `task`
  command accept `--project`; `init`, `index`, every `adr` command, and
  `store migrate` refuse it with a usage error naming `--root`.
  Rationale: the first group is record- or store-scoped; the second group reads
  or writes project-owned files and has no meaning without a checkout.
  Date/Author: 2026-10-01 / agent for Trav.

- Decision: in records-only mode `status`/`doctor`/`prime` report
  `installed: true`, because selecting a project from the registry asserts the
  project is known; the store block identifies it.
  Rationale: there is no project checkout to read metadata from, and the
  registry entry is the evidence the project exists.
  Date/Author: 2026-10-01 / agent for Trav.

- Decision: `ahm index` stays refused under `--project`, and the store's task
  indexes are regenerated by a records-only task mutation or by `ahm index` in
  the project. `store path`, `status`, `doctor`, and `prime` accept `--project`
  in addition to the task commands.
  Rationale: the task names `init`, indexes, and project docs as the
  checkout-relative work that keeps `--root`; `index` writes the project-owned
  ADR index. A records-only validation still checks the store task indexes, so
  the remedy is documented rather than adding an `index --project` mode the task
  did not ask for.
  Date/Author: 2026-10-01 / agent for Trav.

- Decision: a records-only `task complete` does not apply `strict_acceptance`,
  because that flag is read from the committed configuration. The weakening is
  documented in `docs/references/cli/global-contract.md` and pinned by a test.
  Rationale: reading the target's configuration would defeat the records-only
  boundary the flag exists to provide.
  Date/Author: 2026-10-01 / agent for Trav.

- Decision: the selected project's recorded path is display-only: it is the most
  recently observed value in `registry.json` and is never read or written. Every
  project-scoped read and write is suppressed by `isRecordsOnly()`.
  Rationale: `status` needs an informative `root`, but the sandboxed case must
  not touch the target checkout.
  Date/Author: 2026-10-01 / agent for Trav.

## Outcomes & Retrospective

The change delivers `--project <selector>` end to end. From any directory, a
person or agent can now do `ahm --project ahm task create "..."`,
`ahm --project ahm status`, `ahm --project ahm prime`, and
`ahm --project ahm store path`, all resolving the project from
`~/.ahm/registry.json` without reading a checkout or running Git. The selector
matches an exact key first and otherwise a unique case-insensitive substring of
the key or of the registry directory name; unknown and ambiguous selectors and a
`--root`/`--project` combination exit 2 with the candidates listed. Commands
that own project files (`init`, `index`, `adr`, `store migrate`) refuse the flag.

Verification: `just ci` passes (fmt-check, tidy-check, vet, race tests at 89%
coverage, golangci-lint, govulncheck, markdownlint, build, goreleaser check and
snapshot build), and the new tests in `internal/ahm/project_selector_test.go`
cover resolution, exit codes, a records-only create that never reads or writes
the selected checkout, status/prime/doctor/store path, the missing-store
report, strict-acceptance behavior, and the no-Git guarantee.

Deliberate limits, all documented: `strict_acceptance` is not applied under
`--project`; `index` is refused; and a registered project whose records still
live in the project reports a missing store directory (`store_dir_unreadable`)
and a records-only write creates the store directory, which can diverge from the
project's records.

Lesson: a flag that selects by identity rather than by path has to name the
line between record-scoped and project-scoped work explicitly, because the
project root is still needed for display and for ADRs even when it must never be
read. The `recordsOnly` flag on `workflowPaths` is what makes that line
checkable in one place.

## Context and Orientation

`ahm` is a Go CLI (`cmd/ahm/main.go`, implementation under `internal/ahm/`).
It manages two record families: tasks and ADRs. ADRs are always committed
project files under `docs/adr/`. Tasks have two storage *locations*, recorded in
the committed `.ahm/config.json` under `tasks_location`: `project` (default,
records under `.ahm/tasks/`) and `home` (records under the user-level store).
This plan concerns the `home` location.

Key types and files:

- `internal/ahm/cli.go` — `options` holds the parsed global flags; `app` holds
  per-command state, including a cached resolved layout. `app.detectRoot` (in
  `root.go`) resolves the target root and caches the layout.
- `internal/ahm/root.go` — `detectRoot` (strict), `detectRootOrCWD` (lenient,
  used by `init`), `detectManagedRoot` (walk up to `.git`/`.ahm/config.json`).
- `internal/ahm/store.go` — the user-level store. `storeRoot()` resolves
  `~/.ahm` or `AHM_HOME`. `loadRegistry(root)` reads `<store>/registry.json`,
  a `registry` with `Projects map[string]projectEntry`. A `projectEntry` has
  `Key`, `Kind`, `Dir` (the `projects/<dir>` segment), `Remotes`, `Paths`
  (absolute project roots seen), and `Created`. `storePaths` is a resolved
  store location: `Root`, `Key`, `Kind`, `ProjectDir`; `recordsDir()` is
  `<ProjectDir>/tasks`. `resolveStore(projectRoot)` derives a key from Git and
  the path rule, which reads the checkout.
- `internal/ahm/workflow_paths.go` — `workflowPaths` is the resolved records
  layout for one project: `projectRoot`, `recordsRoot`, `store`, `mode`.
  `inStore()` reports `mode == home` with a resolved store. `ownedRoots()` and
  `stateRoots()` bound every write and the stale-temp scan. `resolveWorkflowPaths`
  builds the layout from `.ahm/config.json`.
- `internal/ahm/indexes.go` — `indexWritesForPaths(root, tasks, paths, cache)`
  renders the four task indexes plus `docs/adr/index.md` from the project's ADRs.
- `internal/ahm/validation.go` — `validateWorkflowScopedForPathsWithCache` and
  `validateWorkflowStateForPaths` run every diagnostic.
- `internal/ahm/status.go`, `doctor.go` (in `status.go`), `prime.go` — read
  metadata and Git at `a.opts.root` and emit the report.

Plain-language definitions used below: a *checkout* is a project directory a
person has on disk. A *records-only* command is one that reads or writes task
records but never reads or writes project-owned files (`.ahm/config.json`, the
ADRs, `docs/adr/index.md`, the managed `.gitignore`). The *registry directory
name* is the `Dir` field of a registry entry, e.g. `ahm-0242d8b9`.

## Plan of Work

### 1. Selector resolution (`internal/ahm/store.go`)

Add a resolver that names a project from the registry alone, reading only the
store:

    // resolveProjectSelection resolves a --project selector against the store
    // registry. It reads no checkout and runs no Git. An exact key match wins;
    // otherwise a unique case-insensitive substring of the key or of the
    // registry directory name selects the project. An unknown or ambiguous
    // selector is a usage error that lists the candidates.
    func resolveProjectSelection(selector string) (storePaths, error)

Implementation notes: call `storeRoot()` for the root; `loadRegistry(root)`; the
candidate list is every entry whose key equals the selector first, else every
entry whose key or `Dir` contains the lowercased selector. Validate the entry's
`Dir` with `validStoreDirName`. Build
`storePaths{Root: root, Key: entry.Key, Kind: entry.Kind, ProjectDir: filepath.Join(root, storeProjectsDirName, entry.Dir)}`.
Errors are `usageError` so `Main` exits 2: unknown →
`no project matches "x" in the store; known projects: a, b` (all keys, sorted);
ambiguous → `project selector "x" matches multiple projects: a, b; use an exact key`.

### 2. Flag and app wiring (`internal/ahm/cli.go`, `internal/ahm/root.go`)

- Add `project string` to `options`.
- Register `root.PersistentFlags().StringVar(&a.opts.project, "project", "", "Select a project by home-store key or directory name")`.
- In `detectRoot` and `detectRootOrCWD`, handle the two early cases first:
  - `a.opts.project != "" && a.opts.root != ""` →
    `usageError("--project and --root are mutually exclusive; use --project to select task records or --root to select a checkout")`.
  - `a.opts.project != ""` → resolve the selection, set `a.opts.root` to the
    entry's recorded path (`Paths[0]`, else `os.Getwd()`), and cache a
    records-only layout with `a.useWorkflowPaths(...)`; return.
- Add `detectRootForCheckout` and `detectRootOrCWDForCheckout` that first refuse
  `--project` for a checkout-relative command:

      usageError("--project selects task records only; this command needs a checkout (use --root or run it in the project)")

  Use `detectRootForCheckout` in `simpleCommand` (index), in the `adr`
  subcommands, and in `store migrate`; use `detectRootOrCWDForCheckout` in
  `lenientCommand` (init).

### 3. Records-only layout (`internal/ahm/workflow_paths.go`)

- Add field `recordsOnly bool` to `workflowPaths`, and
  `func (p workflowPaths) recordsOnlyLayout() bool { return p.recordsOnly }` —
  name it `recordsOnly()` on the type only if it does not collide; the field and
  method need different names, so use field `recordsOnly` and method
  `isRecordsOnly()`.
- Add `workflowPathsForSelection(projectRoot string, store storePaths) workflowPaths`
  returning `mode: locationHome`, `recordsRoot: store.recordsDir()`, `store`,
  and `recordsOnly: true`.
- `ownedRoots()` and `stateRoots()` omit `projectRoot` when `recordsOnly` is set.

### 4. Index generation (`internal/ahm/indexes.go`)

In `indexWritesForPaths`, when `paths.isRecordsOnly()`: do not call
`cache.adrList(root)` and do not add `paths.adrIndexPath()` to the writes map;
return only the four task-index writes with no ADR error. Every other caller
(task create, transitions, `index`) keeps its current behavior for checkouts.

### 5. Validation (`internal/ahm/validation.go`)

Gate the project-scoped steps on `!paths.isRecordsOnly()`:

- `validateManagedFiles`: skip `validateMetadata`, `validateRecordLocation`;
  keep `validateTaskFiles` and `validateTaskDuplicateIDs`.
- `validateWorkflowScopedForPathsWithCache`: skip `validateADRs` and
  `validateGeneratedIndexes`' metadata gate; skip `validateMarkdownLinks`
  entirely (it is checkout-scoped).
- `validateWorkflowStateForPaths`: skip `validateMetadata` and `validateADRs`;
  `validateGeneratedIndexMetadata` returns true when records-only so the
  generated task indexes are still compared.
- Change `validateGeneratedIndexMetadata(root, report)` to
  `validateGeneratedIndexMetadata(paths, report)` and have it return true when
  records-only.

### 6. Status, doctor, prime (`internal/ahm/status.go`, `internal/ahm/prime.go`)

- `status`: when `a.workflowPaths().isRecordsOnly()`, treat the project as
  installed (skip `readMetadata`) and emit the store block.
- `doctor`: when records-only, report `workflow_installed: true` and skip the
  metadata read; `git_available` still reflects `exec.LookPath("git")`.
- `prime`: when records-only, skip the metadata read and the workflow
  preparation block (there is no project to prepare), skip `readGitContext`
  (report `primeGit{}`), and set `Installed: true`.

### 7. Command refusals (`internal/ahm/adr_commands.go`, `cli.go`, `store_migrate.go`)

Switch the `a.detectRoot()` calls in the `adr` subcommands, in `simpleCommand`,
and in the `store migrate` command to `a.detectRootForCheckout()`, and the
`init` command's `detectRootOrCWD` to `detectRootOrCWDForCheckout()`.

## Concrete Steps

Working directory: the repository root `/Users/travisennis/Projects/ahm`.

1. Make the code edits above.
2. Format and build:

       just fmt
       just build

3. Focused tests:

       go test ./internal/ahm -run 'Project|Selector|RecordsOnly|CLIDocumentationParity'

4. Full verification, as far as the environment allows:

       just ci

   Record any recipe that cannot run (for example `release-check` when
   `goreleaser` is absent) and the narrower checks run instead.

## Validation and Acceptance

- `ahm --project ahm task create "selector smoke test"` run from a directory
  outside any project writes the record under the store's `ahm` project and adds
  no file to the current directory's project. Remove the record afterward with
  `ahm --project ahm task cancel <id>` (or delete the file) so the shared store
  is left as found.
- `ahm --project ahm status` prints `root`, the task counts for the `ahm`
  project, and a `store:` block with `key: github.com/travisennis/ahm`.
- `ahm --project nomatch status` exits 2 and lists the known projects;
  `ahm --project github.com status` (a substring shared by several keys) exits 2
  and lists the matches; `ahm --project github.com/travisennis/ahm status` uses
  the exact key.
- `ahm --project ahm --root /tmp status` exits 2 with the mutual-exclusion
  message.
- `ahm --project ahm index` and `ahm --project ahm adr list` exit 2 with the
  checkout-required message.
- New Go tests in `internal/ahm`:
  - `TestProjectSelectorResolution` — exact key, unique substring of key,
    unique substring of directory name, ambiguous, unknown.
  - `TestProjectSelectorMutuallyExclusiveWithRoot` — exit 2.
  - `TestProjectSelectorRecordsOnlyCreate` — a task create with `--project`
    lands in the selected store project and writes nothing else; the selected
    project's recorded path is a path that does not exist, proving the target is
    never read.
  - `TestProjectSelectorStatus` — reports records and the store block.
  - `TestProjectSelectorRefusedForCheckoutCommands` — index/adr refuse it.
- `just cli-parity` passes after `docs/references/cli/global-contract.md`
  gains `--project` in its `global-flags` block.

## Idempotence and Recovery

Every step is additive and idempotent. Tests use `t.TempDir` store roots and the
test binary's resolved-root guard, so no developer store is reached. The manual
smoke test in Validation must clean up the record it creates in the shared
store.

## Artifacts and Notes

Not yet produced.

## Interfaces and Dependencies

New and changed signatures that must exist at the end:

    // internal/ahm/store.go
    func resolveProjectSelection(selector string) (storePaths, error)

    // internal/ahm/workflow_paths.go
    type workflowPaths struct {
        projectRoot string
        recordsRoot string
        store       storePaths
        mode        taskLocation
        recordsOnly bool
    }
    func (p workflowPaths) isRecordsOnly() bool
    func workflowPathsForSelection(projectRoot string, store storePaths) workflowPaths

    // internal/ahm/root.go
    func (a *app) detectRootForCheckout() error
    func (a *app) detectRootOrCWDForCheckout() error

    // internal/ahm/validation.go (signature change)
    func validateGeneratedIndexMetadata(paths workflowPaths, report *validationReport) bool

No new dependencies. The change is confined to `internal/ahm/` and the CLI
reference docs.

---

Note on revision: after the first review, this plan moved to
`docs/exec-plans/completed/` and the task was completed. The review added the
`store_dir_unreadable` check for a records-only layout, split the
checkout-required refusal into `refuseProjectFlag`, made the display path the
most recently recorded registry path, widened the docs to state the deliberate
limits of `--project` (no strict acceptance, no `index`, divergence risk for a
not-yet-migrated project), and added tests for prime/doctor/store path, strict
acceptance, a missing store directory, and the no-Git guarantee. The reason for
each is recorded in the Decision Log.
