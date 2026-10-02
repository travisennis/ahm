---
status: accepted
date: 2026-10-02
---
# Use ahm: identity references for workflow record cross-links

## Context and Problem Statement

Workflow records and committed project documents cross-reference each other by
filesystem-relative path. Link validation (`validateMarkdownLinks`) resolves
those paths inside task records, ADR records, and their generated indexes, and
warns `markdown_link_missing` when a target is absent; it deliberately does not
scan general project documentation.

A relative path is not a stable identity. A task record moves between the
active, completed, and cancelled buckets as its status changes. In the home
store a task record is not in the repository at all, so a committed document
cannot link one by path in any form. An ADR keeps its number but its slug can
change. Every new record adds another relative path that silently depends on a
layout nothing enforces. `markdown_link_missing` is a compatibility surface, so
changing how links resolve needs an explicit decision.

## Decision Drivers

- A cross-reference must survive a record changing lifecycle bucket, storage
  layout, or slug.
- One reference form should serve both directions: a record naming a project
  file, and a project document naming a record.
- Existing relative links, and ADR-023's byte-identical project-mode output,
  must keep working.
- Validation must keep resolving references meaningfully, without false
  positives on valid references.

## Considered Options

- Keep the status quo: filesystem-relative paths only.
- Add a typed identity reference scheme, `ahm:<kind>/<target>`.
- Use root-anchored absolute paths such as `/docs/adr/022-example.md`.

## Decision Outcome

Chosen option: "Add a typed identity reference scheme", because it is the only
option that resolves the same reference after a record moves, in the home
store, or across an ADR slug change, and because one typed form serves both
directions.

### Syntax

A reference is an ordinary Markdown link whose target uses the `ahm:` scheme.
One form covers both directions; the kind chooses what the target names.

| Reference | Names |
| --- | --- |
| `ahm:task/<id>` | A task record by ID (`258`, `263a`), in any bucket. |
| `ahm:adr/<ref>` | An ADR by the reference form `ahm adr show` accepts (`9`, `009`, `009-slug`). |
| `ahm:doc/<path>` | A project file by its repository-relative path, with forward slashes. |

A record names a project file with `ahm:doc/<path>` or with a relative path; a
project document names a record with `ahm:task/<id>` or `ahm:adr/<ref>`. The
scheme and its kinds are matched case-insensitively; the canonical spelling is
lower case.

### Resolution

`ahm` resolves an `ahm:` target against the records and the project tree it
already reads:

- `task/<id>` resolves through the same resolver `ahm task show` uses, across
  every bucket.
- `adr/<ref>` resolves the same way `ahm adr show` does.
- `doc/<path>` resolves under the project root. A target that is absolute or
  escapes the project root is malformed rather than missing.

A reference that follows the scheme but names no record keeps the existing
`markdown_link_missing` warning. A malformed or ambiguous reference — a
missing kind, an unknown kind, an empty target, a target that resolves to more
than one record, or an `ahm:doc/` path that escapes the project root — is
reported as a new `markdown_link_invalid` warning. Relative links keep their
current resolution unchanged.

### Compatibility

Existing relative links are supported indefinitely: they are not migrated, and
`ahm` does not report them as drift. Generated indexes and the supersession
note `ahm adr supersede` writes keep their directory-relative links, because
`ahm` generates them and each is stable where `ahm` writes it; only
hand-authored references adopt the scheme.

### Consequences

- Good, because a reference to a task survives a bucket move, a store
  migration, and a rename, and a committed document can reference a home-store
  record that has no repository path.
- Good, because relative links, the generated index format, and ADR-023's
  byte-identical project-mode payloads are unchanged.
- Bad, because `ahm:` targets do not render as working links in a generic
  Markdown viewer such as GitHub's; the reference is meaningful to `ahm` and to
  a reader, not to the viewer.
- Bad, because link checking still covers only ahm-owned records and their
  indexes, so a project document that uses the scheme is not validated. This is
  unchanged behavior, not a new gap.

## More Information

- Task 258 designs and implements the scheme.
- [ADR-023](023-store-task-records-in-a-user-level-home-store.md) defines the
  home store and the project-mode byte-identity promise this decision keeps.
