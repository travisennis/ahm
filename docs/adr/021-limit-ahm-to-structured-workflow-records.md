---
status: accepted
date: 2026-07-27
decision-makers: Travis Ennis
---
# Limit ahm to structured workflow records

## Context and Problem Statement

Ahm currently treats general project documentation as a workflow concern. ADR
016 established `ahm docs check`, the `project-docs` validation scope,
`projectDocs` configuration, and `ahm context docs` so ahm could prescribe and
mechanically validate a repository's documentation surface.

That boundary is broader than ahm's durable product role. Ahm's first-class
domain is structured agent work with machine-readable lifecycle or integrity
semantics: research, architecture decision records (ADRs), ExecPlans, and
tasks. General project documentation may be an output of that work, but its
structure, content, validation policy, and maintenance procedure vary by
project and belong to the project.

Ahm still needs deterministic relative-link validation within its four record
families. Broken links in those records undermine the workflow graph and its
evidence, decisions, plans, and handoffs. That is record-integrity validation,
not general documentation governance.

How should ahm narrow its product boundary while treating the existing command,
configuration, validation, and managed-file surfaces honestly?

## Decision Drivers

- Give ahm one coherent product boundary centered on structured agent work.
- Assign general documentation policy and judgment to the project that owns
  the documentation.
- Preserve deterministic integrity checks for ahm-managed records.
- Avoid retaining aliases or procedures that continue to present general
  documentation as an ahm workflow type.
- Make the intentional CLI and configuration break visible and migratable.
- Preserve historical records and remove obsolete managed files only when
  ownership is proven.

## Considered Options

- **Retain ADR 016 and the general-documentation product surface.** Keep `ahm
  docs check`, `project-docs`, `projectDocs`, and `ahm context docs`.
- **Remove only the dedicated command and context procedure.** Keep general
  project-documentation validators and configuration behind `status` and
  `doctor`.
- **Keep general relative-link validation, but remove opinionated checks.**
  Continue scanning project documentation for broken links while removing the
  other documentation checks, configuration, and procedure.
- **Limit ahm to four structured record families.** Remove all
  general-documentation surfaces and validate relative links only within
  research, ADRs, ExecPlans, and tasks.

## Decision Outcome

Chosen option: **limit ahm to four structured record families**, because
research, ADRs, ExecPlans, and tasks have stable workflow semantics that ahm
can own, while a repository's general documentation taxonomy and quality
policy require project-specific judgment.

Ahm manages these four record families:

- research preserves evidence, exploration, and uncertainty;
- ADRs preserve durable architectural decisions;
- ExecPlans preserve substantial execution intent and progress;
- tasks preserve actionable work, dependencies, lifecycle state, and
  completion evidence.

General project documentation is project-owned output. Managed work may
require, reference, create, or update it, but ahm does not treat it as a fifth
work type. Each project owns its documentation structure, content, procedures,
and validation policy through its own instructions and tooling.

ADR 016 is superseded in full. The following surfaces will be removed:

- the `ahm docs check` command group;
- the `project-docs` validation scope and its compatibility alias;
- `projectDocs` configuration and every general-documentation validator and
  finding code derived from it;
- `ahm context docs` and the binary-owned documentation procedure.

Default `ahm doctor` and `ahm status` validation will continue to check
relative Markdown links in the four structured record families, including
ADRs. Discovery will be limited to the current or supported legacy roots for
those records and their managed indexes; it will not scan general project
documentation or project-owned agent instructions. Existing managed-record
link finding codes and output contracts remain compatible.

### Compatibility and Upgrade Treatment

These removals are an intentional breaking CLI and configuration change.
Keeping deprecated command or scope aliases would preserve the product
boundary this decision rejects, so no continuing alias is required.

The release that implements this decision must:

- identify the removals as breaking changes in release notes and the
  changelog;
- tell users to remove hooks and CI invocations of `ahm docs check` or
  `--check project-docs`, and to adopt project-owned documentation tooling
  where they still want those checks;
- document that `projectDocs` no longer affects runtime behavior;
- make ordinary metadata reads tolerate the obsolete `projectDocs` field
  during transition; and
- make explicit `ahm upgrade` remove the obsolete field from ahm-owned
  configuration without discarding unrelated unknown metadata.

The removed documentation procedure has one cleanup exception. Upgrade may
remove an obsolete `.agents/DOCS.md` only when existing metadata proves that
ahm owns the file and the normal managed-file safety checks permit removal.
Locally modified or unproven files remain project-owned and are preserved.
Ahm will neither expose nor install a replacement general-documentation
procedure.

### Consequences

- Good, because the product boundary and context scopes match the four
  structured record families ahm can manage coherently.
- Good, because broken links in workflow evidence, decisions, plans, tasks,
  and their managed indexes remain detectable by default.
- Good, because projects can choose documentation structures and enforcement
  suited to their own repositories.
- Good, because upgrade removes obsolete ahm-owned configuration and files
  without claiming or overwriting project-owned content.
- Bad, because existing command, scope, configuration, hook, and CI consumers
  must migrate in a breaking release.
- Bad, because users who valued ahm's general-documentation checks must replace
  them with project-owned tooling.
- Neutral, because historical research and ADRs continue to describe the
  former direction; supersession metadata, rather than rewriting history,
  records the change.

## More Information

- Product direction and implementation tracker: task 241 and child tasks
  241b-241d.
- Historical design research:
  [ahm docs check](../../.ahm/research/topics/ahm-docs-check.md).
- Workflow upgrade contract:
  [Workflow Upgrades](../guides/workflow-upgrades.md).

- Supersedes [ADR-016](016-ahm-docs-check-command-and-project-docs-deprecation.md).
