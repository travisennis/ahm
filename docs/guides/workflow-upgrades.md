# Workflow Upgrades

`ahm` owns the workflow templates. To update the workflow, edit files under
`internal/templates/workflow/`, rebuild `ahm`, and run:

```bash
ahm upgrade
```

The upgrade process compares the installed metadata in `.agents/ahm.json` with
the target repository files.

- Missing managed files are created.
- Files that still match the previous managed hash are updated.
- Files with local modifications are preserved and reported as conflicts.
- `AGENTS.md` is create-only. `ahm` may add the starter entrypoint when it is
  missing, but it never overwrites an existing `AGENTS.md`, even with
  `--force`.
- Generated indexes are regenerated.
- User-owned task files, research notes, and ExecPlans are not overwritten.
- Managed template files (`.agents/TASKS.md`, `docs/adr/README.md`, etc.) are
  overwritten only when their content matches the previous managed hash or
  `--force` is used; locally customized managed templates are preserved and
  reported as conflicts.

See [the workflow specification](../references/workflow-spec.md) for the
complete file ownership boundary.

**Version advancement:** The metadata `version` field always advances to the
embedded template version, even when conflicts exist. This ensures that
subsequent upgrades correctly identify files that have already been updated.
Conflicted files retain their old expected hashes in metadata and remain in
conflict until resolved (either by the user reverting the local edit or by
running with `--force`).

Use `--dry-run` to preview changes. Use `--force` only when the embedded
template should replace local edits to managed workflow files.

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
- Dev builds (`go build`, `just build`) without ldflags will show the default
  value from `internal/version.Binary` (currently `"0.2.0"`), which is
  acceptable for local development.

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

- `ahm upgrade` will update `docs/adr/README.md` in consumer repositories
  that have not locally modified it. Locally customized ADR guides will be
  preserved and reported as conflicts.
- `ahm init` in new repositories will install the MADR-only guidance.
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

- `ahm upgrade` will update `.agents/TASKS.md` in consumer repositories that
  have not locally modified it.
- The generated task workflow semantics are unchanged; only the documentation
  target for verification policy changed.
