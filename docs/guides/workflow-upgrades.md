# Workflow Upgrades

`ahm` owns workflow state and managed-work references. To update the
workflow, edit the relevant implementation or instruction source in this
repository, rebuild `ahm`, and run:

```bash
ahm upgrade
```

The upgrade process compares the installed metadata in `.agents/ahm.json` with
the target repository files. Repositories that have already migrated to the
ADR 013 `.ahm/` namespace use `.ahm/config.json` as the committed config file;
when that file exists, `ahm upgrade` reads and writes it instead of the legacy
`.agents/ahm.json`.

- Missing workflow directories, metadata, and generated indexes are created.
- Legacy instruction files that still match the previous managed hash are
  removed because managed-work references now come from scoped `ahm context`.
- Managed skill templates under `.agents/skills/` are still installed and
  upgraded from embedded templates.
- Files with local modifications are preserved and reported as conflicts.
- `AGENTS.md` is project-owned. `ahm` never creates, overwrites, or removes it,
  even with `--force`.
- Generated indexes are regenerated.
- User-owned task files, research notes, and ExecPlans are not overwritten.
- Locally customized legacy instruction files (`.agents/TASKS.md`,
  `docs/adr/README.md`, etc.) are preserved and reported as conflicts unless
  `--force` is used.

See [the workflow specification](../references/workflow-spec.md) for the
complete file ownership boundary.

**Version advancement:** The metadata `version` field always advances to the
embedded template version, even when conflicts exist. This ensures that
subsequent upgrades correctly identify files that have already been updated.
Conflicted files retain their old expected hashes in metadata and remain in
conflict until resolved by deleting the local copy, restoring the recorded
content, or running with `--force`.

Use `--dry-run` to preview changes. Use `--force` only when old local
instruction files should be removed even though they no longer match their
recorded managed hash, or when local edits to managed skill templates should
be replaced.

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
