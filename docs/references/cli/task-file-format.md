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
- `depends_on`

Optional front matter preserved by task rewrites:

- `created`
- `updated`
- `parent`
- `external_ref`

`parent` and `external_ref` are written by `ahm task create` (`--parent`,
`--external-ref`), `ahm task import`, and `ahm task edit <id>`. `task create --external-ref` sets
it at creation; `task edit --external-ref ""` clears it.

Retired front matter preserved as an unknown field:

- `exec_plan` — the link to an ExecPlan that older releases managed. `ahm`
  neither reads nor validates it and never writes a new one; an existing value
  is re-emitted in its original slot, so the file round-trips unchanged.

`depends_on` accepts `-`, `[]`, or a comma-separated list. Rewrites use `-` for
an empty dependency list and comma-separated IDs for non-empty lists.

Task rewrites preserve the parsed body after the top-level task heading. They
rewrite front matter in `ahm`'s canonical order.

## Task Body Sections

### `## Comments`

`task import` can preserve existing comments and cancellation reasons through
the imported Markdown body.

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

### Sections Owned By Another Command

`## Comments` (written by `task comment`) and `## Cancellation Reason` (written
by `task cancel`) hold machine-generated provenance, so `task edit` treats them
as one-writer sections. `ahm task edit <id> --section Comments` and
`--section "Cancellation Reason"` are usage errors naming the owning command,
and a whole-body `task edit` that would drop either section is refused unless
`--force` is passed. Every other `##` section is editable, and
`--section <name>` rewrites one section without touching the rest of the body.

## Cross-References

A record names another record, or a project file, with either a relative
Markdown link or an `ahm:` reference, for example `[ADR 023](ahm:adr/023)`,
`[task 258](ahm:task/258)`, or `[the CLI reference](ahm:doc/docs/cli.md)`. An
`ahm:` reference resolves by identity, so it survives a task record moving
between lifecycle buckets and a home-store task that has no repository path;
relative links keep their existing resolution and are not migrated. The
[workflow specification](../workflow-spec.md) owns the scheme, its resolution
rules, and the compatibility decision. Examples inside fenced code blocks and
inline code spans are not treated as navigation.

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
| `metadata_missing` | Workflow metadata `.ahm/config.json` is missing. |
| `metadata_corrupt` | Workflow metadata exists but cannot be read or parsed. |
| `task_records_in_project` | A task record is still in `.ahm/tasks/` while `tasks_location` is `home`, so no command reads it. This is an error-tier finding. |
| `store_dir_unreadable` | The configured home store's task records directory is missing or unreadable, so the task list cannot be read. This is an error-tier finding. |
| `task_dir_unreadable` | A task bucket directory could not be read. |
| `task_unreadable` | A task file could not be read. |
| `task_missing_field` | Task front matter is missing a required field. |
| `task_malformed` | A task could not be parsed or has unsupported enum values. |
| `task_duplicate_id` | The same task ID appears in more than one bucket file. This error names both files and the manual recovery action. |
| `task_bucket_mismatch` | A task status does not match its active, completed, or cancelled bucket. |
| `task_dependency_missing` | A task depends on an ID that does not exist. |
| `task_dependency_cycle` | Non-completed, non-cancelled tasks contain a dependency cycle. |
| `task_dependency_cancelled` | A non-completed task depends on a cancelled task, which can never be satisfied. |
| `task_blocked_deps_complete` | A Blocked task has all its dependencies Completed, so it can be unblocked. This is a warning-tier finding. |
| `task_tracking_children_complete` | A Tracking task has at least one child and every child task is Completed or Cancelled, so only the tracker remains to be closed. This is a warning-tier finding. |
| `task_acceptance_missing` | A completed task is missing an acceptance section. |
| `task_acceptance_placeholder` | A completed task acceptance section still contains the seeded `- [ ] TODO` placeholder. |
| `task_acceptance_unchecked` | A completed task acceptance section contains unchecked `- [ ]` or `* [ ]` items. |
| `adr_malformed` | An ADR file could not be parsed. |
| `adr_id_mismatch` | An ADR metadata `id` value does not match the numeric filename prefix. |
| `adr_duplicate_id` | Multiple ADR files use the same numeric ADR ID. |
| `adr_invalid_status` | A MADR-profile ADR has a status outside `proposed`, `accepted`, `rejected`, `deprecated`, or `superseded by ADR-NNN`. |
| `adr_supersede_missing` | A MADR-profile ADR status references a missing superseding ADR. |
| `adr_legacy_format` | An ADR uses the legacy bold-metadata format; convert it to MADR front matter manually. This is a warning-tier finding. |
| `generated_index_missing` | A generated workflow index is missing and should be regenerated with `ahm index`. |
| `generated_index_unreadable` | A generated workflow index could not be read. |
| `generated_index_stale` | A generated workflow index differs from the output `ahm index` would write. |
| `generated_index_check_failed` | `ahm` could not render expected generated indexes for validation. |
| `markdown_link_missing` | A Markdown cross-reference inside a task, ADR, or their generated indexes names nothing: a relative link target that does not exist, or an `ahm:` reference that does not resolve. |
| `markdown_link_invalid` | A Markdown link uses the `ahm:` reference scheme but is malformed or ambiguous: a missing or unknown kind, an empty target, a target that resolves to more than one record, or an `ahm:doc/` path that escapes the project root. |
| `markdown_link_check_failed` | A structured-record Markdown link check could not be completed. |
