# Workflow Upgrades

`ahm` owns workflow state and managed-work references. To update the
workflow, edit the relevant implementation or instruction source in this
repository, rebuild `ahm`, and run:

```bash
ahm upgrade
```

The upgrade process compares the installed metadata (in `.ahm/config.json`
or legacy `.agents/ahm.json`) with the target repository files.

- Missing workflow directories, metadata, and generated indexes are created.
- Legacy instruction files that still match the previous managed hash are
  removed because managed-work references now come from scoped `ahm context`.
- Former preflight, grooming-backlog, and finding-improvements procedure skills
  are left in place as project-owned files. Any old ownership hashes are
  discarded; ahm no longer inspects, reports, overwrites, or removes them.
- Files with local modifications are preserved and reported as conflicts.
- `AGENTS.md` is project-owned. `ahm` never creates, overwrites, or removes it,
  even with `--force`.
- Generated indexes are regenerated.
- User-owned task files, research notes, and ExecPlans are not overwritten.
- Locally customized legacy instruction files such as `.agents/TASKS.md` are
  preserved and reported as conflicts unless `--force` is used.
- Existing task, research, and ADR README scaffold files are preserved and
  relinquished from metadata ownership; even `--force` does not remove them.

See [the workflow specification](../references/workflow-spec.md) for the
complete file ownership boundary.

## Current-Layout Onboarding Guidance (2026-07-19)

`ahm onboard` now describes `.ahm/` as the workflow-record directory and
distinguishes ADRs under `docs/adr/` as project-owned durable documentation.
It no longer advertises project-owned `.agents/` as ahm record storage.

This is binary-owned CLI guidance in `internal/ahm/onboard.go`, not an embedded
workflow template under `internal/templates/workflow/`, so
`internal/templates.Version` remains `0.6.3`.

**Version advancement:** The metadata `version` field always advances to the
embedded template version, even when conflicts exist. This ensures that
subsequent upgrades correctly identify files that have already been updated.
Conflicted files retain their old expected hashes in metadata and remain in
conflict until resolved by deleting the local copy, restoring the recorded
content, or running with `--force`.

Use `--dry-run` to preview changes. Use `--force` only when old local
instruction files should be removed even though they no longer match their
recorded managed hash.

## Record Layout Terminology (2026-07-18)

`internal/templates.Version` advanced from `0.6.2` to `0.6.3` because the
embedded research workflow reference now describes the metadata-selected
record layout rather than a storage mode. Legacy `.agents/` and post-migration
`.ahm/` layouts both keep source records as ordinary committed files.

Structured `prime` output no longer includes the obsolete `records.mode`
field, and `records doctor` no longer includes `checks.mode`. Record-path
selection and legacy-layout compatibility are unchanged.

## Context-Only Workflow References (2026-07-15)

`internal/templates.Version` advanced from `0.6.1` to `0.6.2`. Fresh installs
no longer create `.ahm/tasks/README.md`, `.ahm/research/README.md`, or
`docs/adr/README.md`. Task, research, and ADR guidance remains available from
`ahm context task`, `ahm context research`, and `ahm context adr` respectively.
The embedded ADR context source was renamed from `workflow/adr-README.md` to
`workflow/ADR.md` to reflect that it is command output rather than a copied
README template.

Existing README files are preserved. The `0.6.0` scaffold files were
create-only and had no ownership hashes, so `ahm upgrade` cannot safely remove
or overwrite consumer copies.

## Concise Task Workflow Reference (2026-07-15)

`internal/templates.Version` advanced from `0.6.0` to `0.6.1` because the
embedded task workflow reference changed. `ahm context task` now focuses on
task decisions and a single end-to-end working procedure while leaving task
front matter, body scaffolding, exact flags, and lifecycle mechanics to the
`ahm task ...` commands that own them.

The procedure applies to tasks with and without ExecPlans. ExecPlan completion
is an explicit conditional step instead of the only fully ordered completion
path. Task storage, file formats, lifecycle semantics, and CLI behavior are
unchanged.

## Command-Based Procedures (2026-07-11)

`internal/templates.Version` advanced from `0.4.6` to `0.5.0` because the
embedded managed-file set changed. Fresh installs no longer create
`.agents/skills/`. Grooming is delegated through `ahm task groom`, improvement
audits through `ahm audit`, and task-work review uses a binary-embedded
preflight procedure. `ahm onboard` replaces the removed `ahm agents`
suggestions group.

## .ahm-first init (2026-07-12)

`internal/templates.Version` advanced from `0.5.0` to `0.6.0` because
`managedFiles` in `templates.go` was populated with `.ahm/` scaffold targets.
Fresh `ahm init` (no prior workflow metadata) now creates the committed
`.ahm/` layout directly: `.ahm/config.json`, scaffold READMEs under
`.ahm/tasks/`, `.ahm/research/`, and `docs/adr/`, and workflow directories
under `.ahm/`. Legacy `.agents/ahm.json` is no longer created for new
installs. Repositories with existing `.agents/ahm.json` are unaffected;
`upgrade` continues to preserve the existing layout.

The former preflight, grooming-backlog, and finding-improvements procedure
files remain in place as project-owned content. Init, upgrade, and records
migration discard their old managed hashes, and even a forced upgrade does not
inspect or remove them. `AGENTS.md` remains project-owned and is never modified.

The dated entries below are release history and describe behavior at those
versions. References there to installed skills or `ahm agents suggestions` are
superseded by the `0.5.0` migration above.

## Records Migration (2026-07-06)

Workflow migration moves `.agents/.tasks/`, `.agents/.research/`, and
`.agents/exec-plans/` (including generated indexes) to non-dot names
under `.ahm/` (`.ahm/tasks/`, `.ahm/research/`, `.ahm/exec-plans/`),
installs internal `.ahm/.gitignore` entries, converts
`.agents/ahm.json` into committed `.ahm/config.json`, and prints the
`git rm -r --cached` command for the user to run instead of untracking
project-owned records itself. It never touches project-owned `.agents/`
content such as `.agents/prompt.md`, `.agents/skills/`, or `AGENTS.md`.
Migration also discards any old ownership hashes for the former preflight,
grooming-backlog, and finding-improvements skills so later ahm commands leave
them entirely project-owned. The migration is a
separate command, `ahm records migrate`; routine `ahm upgrade` never
performs it, and repositories keep the current committed-record behavior
until they opt in.

Use `ahm --dry-run records migrate` to preview every effect first. The command
is resumable after interruption, and `ahm records doctor` diagnoses partially
migrated states. Rollback steps are documented in the
[`records migrate` reference](../references/cli/commands.md#records-migrate).

### Impact

- `.ahm/config.json` becomes the committed config file after migration;
  `ahm upgrade` and other commands read and write it instead of legacy
  `.agents/ahm.json` (see the note above about metadata paths).
- After migration, workflow commands (`ahm task ...`, `ahm index`,
  `ahm status`, `ahm doctor`, `ahm context`, `ahm init`/`ahm upgrade`
  directory creation) read and write records and generated indexes under
  `.ahm/` instead of `.agents/`.
- Record mutations in a migrated repository write source records as
  normal committed project files; generated indexes are regenerated
  locally and remain gitignored.
- `ahm prime` is the session-start command for agents. It regenerates
  indexes, validates workflow state, and prints the backlog briefing without
  network or custom-ref operations.
- Migrated record files leave normal branch history once the printed
  `git rm -r --cached` command is run and committed.
- Repositories that do not run `ahm records migrate` are unaffected.

## Role-Specific Agent/Model Configuration (2026-07-09)

`ahm task work` now supports role-specific agent and model defaults under the
`taskWork` block in both `.ahm/config.json` and legacy `.agents/ahm.json`.

The `implementation` and `review` objects each accept `agent` and `model`
fields. Review falls back to the implementation agent when no review-specific
config is set. Feedback-resume and commit handoff always use the implementation
agent because they resume the implementation session.

See the [workflow specification](../references/workflow-spec.md) and
[task commands reference](../references/cli/task-commands.md) for the full
precedence rules and examples.

### Impact

- Projects can now configure different agents and models for implementation vs.
  review without relying on CLI flags.
- Existing `default_work_agent` continues to work and serves as the fallback
  when no role-specific config is present.
- `ahm --dry-run task work <id>` includes `review_agent` and `review_model`
  fields when review will run.
- No template version change; existing metadata formats are fully backward
  compatible.

## Layout-Aware Agent Guidance Rendering (2026-07-09)

`internal/templates.Version` advanced from `0.4.5` to `0.4.6`.

Live agent instruction output now renders workflow record, generated index, and
metadata paths for the repository's record layout. Legacy repositories
see direct `.agents/...` paths; migrated repositories see direct `.ahm/...`
paths. Generic project documentation still describes both layouts where
that distinction is durable user-facing behavior.

### Impact

- `ahm context task`, `ahm context research`, `ahm context plan`,
  `ahm context docs`, and `ahm prime` avoid unnecessary paired-path wording in
  live output when the repository layout is known.
- Managed skill templates and `ahm agents suggestions` render the generated
  task or research index paths for the active record layout.
- `ahm upgrade` records template version `0.4.6` in `.ahm/config.json` after
  migration, or legacy `.agents/ahm.json` before migration.

## Migrated Agent Guidance (2026-07-07)

`internal/templates.Version` advanced from `0.4.4` to `0.4.5`.

The embedded task, research, and ExecPlan workflow references now describe the
current record layout instead of hard-coding only legacy `.agents/` paths.
Legacy repositories still use `.agents/`; repositories that run
`ahm records migrate` use `.ahm/` for task, research, ExecPlan, and generated
index paths. The `ahm agents suggestions` advisory output now includes
`ahm prime` as the session-start step before managed-work intake.

### Impact

- `ahm context task`, `ahm context research`, and `ahm context plan` output
  describes both layouts for fallback paths and generated indexes.
- `ahm agents suggestions` tells maintainers to put `ahm prime` before normal
  managed-work intake in project-owned `AGENTS.md`.
- `ahm upgrade` records template version `0.4.5` in `.ahm/config.json` after
  migration, or legacy `.agents/ahm.json` before migration.

## Context Role Split (2026-06-20)

`internal/templates.Version` advanced from `0.4.2` to `0.4.3`.

This release splits the `ahm context` command into two distinct modes:

- **Unscoped `ahm context`** is a live repository briefing. It prints root,
  workflow version, validation, git state, task summary, and useful commands.
  It no longer prints an `## Instructions` section or claims workflow
  authority.
- **Scoped `ahm context task|plan|adr|research|docs`** prints a pure managed-work
  reference document without live briefing wrapper fields.
- **Scoped JSON** (`ahm --json context task`) now returns `scope`,
  `instructions`, and `commands`. It no longer includes `root`, `workflow`,
  `git`, or `tasks` fields.
- **Unscoped JSON** (`ahm --json context`) no longer includes `instructions`.
- **Validation display** now distinguishes warnings-only state: `validation: ok`
  means zero errors and zero warnings; warnings alone show the warning count
  and sample findings.
- `ahm agents suggestions`, managed skills, and embedded workflow references
  now describe `ahm task show <id>` as the normal task inspection primitive
  and `AGENTS.md` as the owner of workflow routing.

### Impact

- `ahm agents suggestions` output changes for the `ahm-owned-files` advisory
  block to describe the primitives model.
- `ahm context` text and JSON output shapes change as described above.
  Consumers relying on the old scoped JSON shape (with `root`, `workflow`,
  `git`, `tasks`) need to adapt.
- `ahm upgrade` records template version `0.4.3` in `.agents/ahm.json`.
- Existing project-owned `AGENTS.md` files are still never modified by `ahm`.

## AGENTS.md Integration Suggestions (2026-06-20)

`internal/templates.Version` advanced from `0.4.1` to `0.4.2`.

The `ahm agents suggestions` output now frames its Markdown as AGENTS.md
integration guidance rather than simple additions. It tells maintainers or
agents how to preserve project-specific instructions while connecting an
existing Operating Loop and Workflow Routing section to `ahm` managed-work
intake.

The guidance now distinguishes three target shapes:

- Existing Operating Loop: patch it so managed-work intake happens before
  normal workflow routing.
- Workflow Routing but no Operating Loop: add a short Operating Loop before
  the routing section.
- Neither Operating Loop nor Workflow Routing: add only the ahm-specific
  managed-work intake and ownership sections, without inventing a full project
  workflow.

### Impact

- `ahm agents suggestions` output changes for the advisory suggestion blocks.
- `ahm upgrade` records template version `0.4.2` in `.agents/ahm.json`.
- Existing project-owned `AGENTS.md` files are still never modified by `ahm`.

## Managed Work Intake Suggestions (2026-06-20)

`internal/templates.Version` advanced from `0.4.0` to `0.4.1`.

The `ahm-workflow-routing` advisory block printed by
`ahm agents suggestions` now frames `ahm` as managed-work intake for tasks,
ExecPlans, ADRs, and research rather than as a broad default first step for
every session. The suggestion tells agents to use scoped `ahm context`
commands for higher-order workflow records, then return to the project
`AGENTS.md` workflow routing and load the routed docs for the actual code,
docs, CLI, safety, or release change.

### Impact

- `ahm agents suggestions` output changes for the `ahm-workflow-routing`
  advisory block.
- `ahm upgrade` records template version `0.4.1` in `.agents/ahm.json`.
- Existing project-owned `AGENTS.md` files are still never modified by `ahm`.

## Context-Based Agent Instructions (2026-06-19)

`internal/templates.Version` advanced from `0.3.1` to `0.4.0`.

Canonical agent workflow guidance moved from installed repository workflow
guide files to the new `ahm context` command. Fresh installs no longer create
starter `AGENTS.md` or instruction templates such as `.agents/TASKS.md`,
`.agents/PLANS.md`, `.agents/RESEARCH.md`, `.agents/DOCS.md`, and
`docs/adr/README.md`. Agent skills under `.agents/skills/` remain managed
template files.

### Impact

- `ahm init` now creates workflow directories, managed skill templates,
  `.agents/ahm.json`, and generated indexes.
- The next `ahm upgrade` run with this version or newer removes previously
  managed instruction files when their content still matches metadata,
  including `.agents/TASKS.md`, `.agents/PLANS.md`, `.agents/RESEARCH.md`,
  `.agents/DOCS.md`, `.agents/.tasks/README.md`,
  `.agents/.research/README.md`, and `docs/adr/README.md`.
- `ahm upgrade` still updates managed skill templates.
- Locally modified instruction files are preserved as conflicts unless
  `--force` is used.
- Existing `AGENTS.md` files are never modified or removed.
- Agents should run `ahm context` for a session briefing or a scoped form such
  as `ahm context task` for the full scoped instruction document.

## Version Separation (2026-06-10)

The binary version and the workflow template version are now separate.

- `internal/version.Binary` (var, set by goreleaser ldflags) is the release
  version shown by `ahm --version` and `ahm version`. It advances with every
  tagged release.
- `internal/templates.Version` (const) is the embedded workflow template schema
  version. It advances only when the content of the embedded workflow templates
  under `internal/templates/workflow/` changes.
- `.agents/ahm.json`'s `version` field continues to track the template version
  (`templates.Version`), not the binary version. This ensures that `ahm upgrade`
  correctly detects template changes regardless of the release tag.

This separation avoids the bug where `ahm --version` silently reported the
wrong version because `templates.Version` was a `const` and the linker `-X`
flag only sets `var` symbols. Task 023 had made `Version` a `const` on the
assumption there was no separate release pipeline — that assumption was wrong.

### Impact

- `ahm init` and `ahm upgrade` still stamp `templates.Version` into metadata.
- `ahm status` and `ahm doctor` still report `template_version` from
  `templates.Version`.
- `ahm --version` and `ahm version` now return the injected binary version,
  which matches the release tag in goreleaser builds.
- Dev builds (`go build`, `just build`) without ldflags show `dev` so they are
  not confused with tagged release builds.

## ADR Template Rewrite for MADR (2026-06-14)

`internal/templates.Version` advanced from `0.2.0` to `0.3.0`.

`docs/adr/README.md` was rewritten to document only the constrained MADR
profile instead of the legacy Nygard-style format. The new template covers:

- Constrained MADR front matter and section guidance.
- The `ahm adr` command family (create/list/show/accept/reject/deprecate/
  supersede/migrate).
- The generated `docs/adr/index.md` ownership rule.
- Updated supersession rules from ADR 009 (no "Superseded in part" status;
  partial supersession via body notes).

The starter `AGENTS.md` suggestions were updated to route ADR work through
`ahm adr` commands and list `docs/adr/index.md` as an ahm-owned generated
index.

### Impact

- In `0.3.0`, `ahm upgrade` updated `docs/adr/README.md` in consumer
  repositories that had not locally modified it. Locally customized ADR guides
  were preserved and reported as conflicts.
- In `0.3.0`, `ahm init` in new repositories installed the MADR-only guidance.
- The `ahm-workflow-routing` and `ahm-owned-files` agent suggestions now
  cover ADR commands and the generated ADR index.

### Rejected Alternative

Reverting `templates.Version` to `var` would have fixed the injection but
would conflate the binary release version with the template schema version,
causing every release to bump the template version even when templates hadn't
changed.

## Task Workflow Verification Link Update (2026-06-14)

`internal/templates.Version` advanced from `0.3.0` to `0.3.1`.

The task workflow template now points contributors to `CONTRIBUTING.md` for the
project's full CI check instead of the root `AGENTS.md`. This supports the
progressive-disclosure split where `AGENTS.md` routes work and
`CONTRIBUTING.md` owns setup, commands, verification, and commit workflow.

### Impact

- In `0.3.1`, `ahm upgrade` updated `.agents/TASKS.md` in consumer
  repositories that had not locally modified it.
- The generated task workflow semantics are unchanged; only the documentation
  target for verification policy changed.
