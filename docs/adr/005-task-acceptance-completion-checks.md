---
status: accepted
date: 2026-06-05
---
# Task Acceptance Completion Checks

## Context

`ahm task create` seeds new tasks with an `## Acceptance Notes` checklist that
contains `- [ ] TODO`. The workflow asks agents to fill acceptance notes before
completing a task, but `ahm task complete` did not check that convention and
`ahm doctor` did not report completed tasks that still had incomplete
acceptance notes.

The project needs a default that catches drift without interrupting existing
task workflows, plus an opt-in stricter mode for repositories that want task
completion to fail until acceptance notes are filled.

## Decision

`ahm task complete <id>` checks the task body before moving a task to
`Completed`. It recognizes `##` and `###` headings named `Acceptance Notes`,
`Acceptance Criteria`, or `Acceptance`, case-insensitively.

Completion warns on stderr when the acceptance section is missing, still
contains the seeded `- [ ] TODO` placeholder, or contains unchecked `- [ ]` or
`* [ ]` items. This warning-only behavior is the default.

Repositories can set `"strict_acceptance": true` in `.agents/ahm.json` to make
the same findings block completion with a non-zero error. The existing global
`--force` flag overrides strict acceptance and completes the task while still
printing warnings.

`ahm status` and `ahm doctor` report warning-tier findings for already-completed
tasks with incomplete acceptance notes:

- `task_acceptance_missing`
- `task_acceptance_placeholder`
- `task_acceptance_unchecked`

## Rationale

- Acceptance notes are workflow Markdown, so the check should parse the task
  body directly instead of adding new task front matter.
- Warning by default preserves existing CLI behavior while making drift visible.
- A persisted strict setting lets teams enforce the convention without adding a
  new command flag or separate configuration file.
- Reusing `--force` keeps override semantics consistent with existing global
  behavior.

## Consequences

### Positive

- Completed tasks with placeholder or unchecked acceptance notes become visible
  at completion time and during validation.
- Strict repositories can enforce completion hygiene without changing task file
  format.
- Existing repositories are not forced into a blocking workflow.

### Negative

- Completed tasks that intentionally omit acceptance notes will now produce
  warning-tier validation findings.
- The parser depends on conventional Markdown headings and checklist syntax, so
  unusual acceptance formats are not detected as complete.

## Alternatives Considered

- **Always block completion**: Rejected because it would be a disruptive default
  for existing workflows.
- **Add a `--strict` CLI flag only**: Rejected because strictness is a
  repository policy that should not depend on each invocation remembering a
  flag.
- **Store acceptance state in front matter**: Rejected because the source of
  truth is the human-readable task body checklist.

## References

- Task 048: Add acceptance-notes completeness check on task complete
- `.agents/TASKS.md` - task completion workflow
- `internal/ahm/task_acceptance.go` - acceptance parser
- `internal/ahm/task_status.go` - task completion behavior
- `internal/ahm/validation.go` - workflow validation findings
