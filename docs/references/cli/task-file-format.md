# Task File And Validation Formats

This reference covers task Markdown file shape and validation finding codes.

## Task File Format

`ahm` parses a strict YAML-like front matter grammar between `---` delimiters.
The grammar supports `key: value` pairs where keys are alphanumeric with
underscores, and values can be plain text or double-quoted strings. Comment
lines (lines starting with `#`) and blank lines are silently skipped.
Unsupported shapes — keys with spaces or colons, and block scalar indicators
(`|`, `>`) — produce `task_malformed` validation errors.

Required task fields:

- `id`
- `title`
- `status`
- `priority`
- `effort`
- `labels`
- `exec_plan`
- `depends_on`

Optional front matter preserved by task rewrites:

- `created`
- `updated`
- `parent`
- `external_ref`

`depends_on` accepts `-`, `[]`, or a comma-separated list. Rewrites use `-` for
an empty dependency list and comma-separated IDs for non-empty lists.

Task rewrites preserve the parsed body after the top-level task heading. They
rewrite front matter in `ahm`'s canonical order.

## Task Body Sections

### `## Comments`

Comments may be appended to any task (active, completed, or cancelled) using
`ahm task comment <id> <text>`. Each comment is a timestamped Markdown line
under a `## Comments` heading in the task body:

```markdown
## Comments

**2026-06-24T18:30:00Z** — Discovered the root cause.

**2026-06-24T18:31:00Z** — _Author Name_: Follow-up observation.
```

The section is created if it does not exist. New comments are appended after
existing ones. The comment command preserves all front matter, body sections,
and unknown fields.

## Validation Findings

`status` and `doctor` can emit validation findings in three tiers:

- `errors`: hard validation failures; these set `validation.ok` to `false` and
  make the command exit with code 1.
- `warnings`: workflow inconsistencies that should be fixed but do not change
  `validation.ok`.
- `info`: low-noise advisory findings that do not change `validation.ok`.

The JSON shape includes `errors`, `warnings`, and `info` arrays even when a tier
is empty.

Finding codes:

| Code | Meaning |
| ---- | ------- |
| `metadata_missing` | Workflow metadata is missing (`.ahm/config.json` after migration, otherwise `.agents/ahm.json`). |
| `metadata_corrupt` | Workflow metadata exists but cannot be read or parsed. |
| `managed_file_missing` | A managed workflow file is missing. |
| `managed_file_unreadable` | A managed workflow file could not be read. |
| `managed_file_untracked` | A managed workflow file exists but is not recorded in metadata; run `ahm init` to adopt. |
| `managed_file_modified` | A managed workflow file hash differs from metadata. |
| `task_dir_unreadable` | A task bucket directory could not be read. |
| `task_unreadable` | A task file could not be read. |
| `task_missing_field` | Task front matter is missing a required field. |
| `task_malformed` | A task could not be parsed or has unsupported enum values. |
| `task_bucket_mismatch` | A task status does not match its active, completed, or cancelled bucket. |
| `task_dependency_missing` | A task depends on an ID that does not exist. |
| `task_dependency_cycle` | Non-completed, non-cancelled tasks contain a dependency cycle. |
| `task_dependency_cancelled` | A non-completed task depends on a cancelled task, which can never be satisfied. |
| `task_acceptance_missing` | A completed task is missing an acceptance section. |
| `task_acceptance_placeholder` | A completed task acceptance section still contains the seeded `- [ ] TODO` placeholder. |
| `task_acceptance_unchecked` | A completed task acceptance section contains unchecked `- [ ]` or `* [ ]` items. |
| `task_exec_plan_missing` | A task references an ExecPlan that could not be found. |
| `task_completed_exec_plan_active` | A completed task references an ExecPlan still in the active ExecPlan bucket. |
| `task_completed_exec_plan_incomplete` | A completed task references a completed ExecPlan without a filled `Outcomes & Retrospective` section. |
| `exec_plan_active_with_outcomes` | An active ExecPlan has a filled `Outcomes & Retrospective` section. |
| `exec_plan_completed_without_outcomes` | A completed ExecPlan has an empty or missing `Outcomes & Retrospective` section. |
| `exec_plan_completed_with_open_progress` | A completed ExecPlan still has open `- [ ]` items in its `Progress` section. |
| `exec_plan_missing_section` | An ExecPlan is missing one of the mandatory lifecycle sections. `ahm` emits one finding per missing section. |
| `exec_plan_orphan` | An ExecPlan is not referenced by any task `exec_plan` field. This is an info-tier finding. |
| `adr_malformed` | An ADR file could not be parsed. |
| `adr_id_mismatch` | An ADR metadata `id` value does not match the numeric filename prefix. |
| `adr_duplicate_id` | Multiple ADR files use the same numeric ADR ID. |
| `adr_invalid_status` | A MADR-profile ADR has a status outside `proposed`, `accepted`, `rejected`, `deprecated`, or `superseded by ADR-NNN`. |
| `adr_supersede_missing` | A MADR-profile ADR status references a missing superseding ADR. |
| `adr_legacy_format` | An ADR uses the legacy bold-metadata format; run `ahm adr migrate`. This is a warning-tier finding. |
| `generated_index_missing` | A generated workflow index is missing and should be regenerated with `ahm index`. |
| `generated_index_unreadable` | A generated workflow index could not be read. |
| `generated_index_stale` | A generated workflow index differs from the output `ahm index` would write. |
| `generated_index_check_failed` | `ahm` could not render expected generated indexes for validation. |
| `markdown_link_missing` | A relative Markdown link inside the managed workflow surface points at a missing file. |
| `markdown_link_check_failed` | A workflow Markdown link check could not be completed. |
| `project_doc_link_missing` | A relative Markdown link in a discovered project documentation file points at a missing file. Emitted only under the opt-in `--check project-docs` scope. |
| `project_doc_link_check_failed` | A project documentation Markdown link check could not be completed. Emitted only under the opt-in `--check project-docs` scope. |
| `design_doc_unindexed` | A design-doc Markdown file under `docs/design-docs/` is not represented in `docs/design-docs/index.md`. Emitted only under the opt-in `--check project-docs` scope, and only when the repository already uses the `docs/design-docs/` convention with an `index.md`. |
