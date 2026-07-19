---
status: accepted
date: 2026-07-12
decision-makers: Travis Ennis
informed: tasks 178, 179, 180
---
# Apply structured task revisions during grooming

## Context and Problem Statement

ADR 014 delegates `ahm task groom` to an external agent but grants the result
applier authority only to accept, comment, correct dependencies, and normalize
labels. That boundary works for already-ready tasks and tasks requiring human
judgment. It fails for objectively repairable tasks: the agent can identify a
missing compatibility constraint, incomplete relevant-file list, placeholder
acceptance notes, or inaccurate effort, but it can only append a comment and
leave the task ungroomed.

Giving the external process direct file-editing authority would bypass ahm's
task parser, validation, atomic writes, workflow lock, index regeneration, and
storage-mode boundary. Giving it unrestricted replacement authority would
also risk losing comments, unknown front-matter fields, provenance, or
unrelated task content. Grooming needs a constrained representation that lets
the agent propose useful revisions while keeping ahm as the only writer and
preserving the task lifecycle's human-owned decisions.

This decision partially supersedes ADR 014 only where that ADR limits groom
result write authority. ADR 014's delegation, schema-constrained output,
session capture, cancellation, and workflow-record-only safety decisions
remain accepted.

## Decision Drivers

- Let one grooming run resolve objective, repository-verifiable gaps instead
  of merely describing them in a comment.
- Keep delegated output as untrusted data and keep ahm as the sole writer.
- Preserve task identity, provenance, comments, unknown metadata, and content
  outside the explicitly revised surface.
- Make revisions deterministic to validate, preview, apply, and render in
  text, plain, and JSON output.
- Preserve the no-write guarantee for missing, malformed, incomplete,
  duplicated, or out-of-scope delegated results.
- Allow a repaired Open task to become Pending in the same validated
  operation when no human decision remains.
- Keep cancellation, architectural decisions, priority tradeoffs that cannot
  be established from repository evidence, and other subjective choices
  human-owned.

## Considered Options

- **Replace the full task body.** This is simple to represent, but makes the
  agent responsible for copying every comment and unrelated section exactly.
  A small omission becomes destructive, and meaningful diffs are difficult to
  summarize.
- **Replace named sections.** The result identifies a bounded set of Markdown
  sections and supplies their complete new contents. Ahm can validate the
  names, replace or insert only those sections, and preserve everything else.
- **Apply a textual or JSON patch.** Patches can express small edits, but they
  are brittle against whitespace and heading variation, are difficult to
  constrain semantically, and either expose arbitrary ranges or require a
  second task-specific patch language.
- **Let the delegated agent edit task files directly.** This offers maximum
  flexibility but crosses the ADR 014 applier boundary and bypasses ahm's
  workflow invariants and no-write validation guarantee.

## Decision Outcome

Chosen option: **schema-constrained, section-level task revisions applied by
ahm**, because this is expressive enough to repair a task while retaining a
small semantic write surface and deterministic preservation rules.

Each groom verdict may carry a revision object in addition to the existing
comment, dependency, and label fields. The revision object has optional
`priority` and `effort` values plus a list of section replacements. A section
replacement contains a heading and the complete Markdown content beneath that
heading; it does not contain front matter or an H1.

The permitted revision surface is:

- `priority`, when repository evidence makes the current value objectively
  inconsistent with the documented priority scale;
- `effort`, including the corresponding ExecPlan readiness check for L/XL;
- labels and dependencies through the existing groom verdict fields;
- the task's `Problem`, `Relevant Files`, `Fix Direction`, and `Acceptance
  Notes` sections, including recognized existing aliases for those section
  roles; and
- insertion of a missing canonical section using exactly one of those four
  canonical headings.

The result schema uses a closed set of section roles rather than accepting an
arbitrary heading string. Ahm maps each role to the single matching existing
section, preserving that section's original heading spelling. If no matching
section exists, ahm appends the canonical heading. A task with duplicate or
ambiguous matching sections cannot be revised automatically and requires a
comment verdict for human cleanup.

The following remain human-owned and cannot appear in a groom revision:

- task `id`, title, creation timestamp, parent, external reference, and
  `exec_plan` linkage;
- unknown front-matter fields and provenance fields;
- arbitrary body sections, including grooming comments, cancellation reasons,
  historical notes, and any content outside the four permitted section roles;
- cancellation and task deletion;
- Completed or Cancelled records; and
- product, design, architectural, priority, or dependency decisions that
  cannot be established from repository evidence.

Status is not a general revision field. It changes only through the verdict
outcome described below. `updated` is always generated by ahm, never supplied
by the delegated agent.

### Preservation and rendering

Ahm parses the current task, applies replacements to a copy, and renders it
through the normal task renderer. Unknown front-matter key/value pairs are
preserved. Identity and human-owned known fields are copied from the current
record. Sections not named by the revision retain their bytes and order except
for the task renderer's existing LF and surrounding-whitespace normalization.
Existing comments are never replaced or relocated by a revision. New comments
continue to use the existing timestamped grooming-comment mechanism.

A replacement supplies the entire content of its permitted section. Ahm does
not merge checklist items or infer partial prose edits. This makes the proposed
change visible and prevents an underspecified patch from applying differently
across task shapes.

### Validation and write behavior

Ahm treats the complete delegated result as one validation unit. Before taking
the workflow mutation lock or writing any file, it must:

1. parse exactly one verdict for every requested task and no other task;
2. reject unknown fields, duplicate verdicts, duplicate section roles, empty
   replacement content, and revisions outside the permitted surface;
3. validate priority, effort, labels, dependencies, task status, and section
   targeting against the current repository state;
4. apply every proposed verdict in memory, reparse every rendered task, run
   task enum and dependency-cycle validation, and run readiness validation on
   every revise-and-accept result; and
5. precompute the complete task and index output set.

Any delegated-output or semantic validation failure causes no task, timestamp,
or index write. Multi-task grooming is therefore all-or-nothing with respect
to verdict validity. After validation, ahm acquires the normal workflow
mutation lock, verifies that the targeted records have not changed since they
were read, and writes each task with the existing atomic-file primitive before
regenerating indexes. Filesystem failures remain reported as operational
errors; atomic replacement prevents a partially written individual record.

### Bounded semantic correction

When the first delegated result is schema-valid but fails semantic validation,
ahm collects every independently detectable validation error and permits one
correction request. The correction request includes the original target scope,
the original structured result, and the machine-generated errors. It requests
a complete replacement batch, not a patch or permission to partially retain
verdicts. The corrected batch is parsed and validated from scratch before ahm
takes the mutation lock, preserving the all-or-nothing contract.

The correction is strictly bounded to one attempt. Provider execution errors,
timeouts, output that cannot be parsed into the result schema, and target-state
changes found during locked revalidation are terminal and do not trigger it.
Ahm never coerces an invalid action. On final semantic failure, diagnostics
retain the original and corrected structured results and precise validation
errors without dumping the full provider transcript by default.

Readiness validation for acceptance is the same contract used by manual task
acceptance: required problem and acceptance content, type and area labels,
valid priority and effort, resolved and non-cyclic dependency declarations,
and any required ExecPlan or ADR. Grooming must not invent an architectural or
product decision merely to pass this gate.

### Verdict outcomes

The groom result supports three semantic outcomes:

- **comment:** no section or metadata revision is applied. Ahm may apply the
  existing safe dependency/label corrections and appends a precise comment
  describing the human input, decision, or cleanup still required. Blocked
  tasks and cancellation recommendations use this outcome.
- **revise:** ahm applies a valid revision but does not change status. This is
  used when the task is improved but still needs human input, an ADR/ExecPlan,
  or another prerequisite. A comment is required to state what remains.
- **revise-and-accept:** ahm applies the revision and, only after the revised
  task passes readiness validation, changes an Open task to Pending in the
  same locked operation. An already-ready task may use this outcome with an
  empty revision. Blocked tasks cannot use it; grooming does not implicitly
  remove a blocked state.

These are semantic names; implementation may retain `accept` as the wire value
for compatibility if it has the same revise-and-accept behavior. It must not
silently reinterpret a formerly valid comment verdict as revision authority.

### Dry-run and output observability

Global `--dry-run` retains its current orchestration contract: it emits the
complete prompt, result schema, agent selection, and target IDs without
running an agent or writing files. The schema and prompt must describe the
revision surface and preservation rules. It also reports that a single
correction is available only for semantic validation failures.

For an executed grooming run, text and plain output summarize each task's
outcome, changed metadata, replaced or inserted section roles, comment status,
and status transition. JSON exposes the same data as structured fields and
also includes before/after values for mutable metadata plus section-level
before/after content so automation can audit the exact applied revision. The
summary is produced from the validated in-memory change set, not by scraping
rendered text. Initial provider and parse failures preserve raw agent output
for inspection and exit nonzero without emitting an applied-change summary.
When correction succeeds, text output names the retry and original validation
errors, while JSON and plain output expose the attempted/succeeded state and
validation-error list as structured correction metadata.

### Consequences

- Good, because grooming can turn an objectively repairable Open task into a
  ready task in one delegated run.
- Good, because comments, unknown metadata, identity, provenance, and unrelated
  body content remain outside delegated revision authority.
- Good, because section-level before/after data makes autonomous changes
  inspectable in JSON and concise in human output.
- Good, because validation occurs against the fully rendered batch before any
  mutation, preserving ADR 014's invalid-output no-write guarantee.
- Good, because one malformed verdict no longer discards the practical value
  of an otherwise useful audit without first offering a bounded repair.
- Bad, because closed section roles cannot repair arbitrary custom task
  sections; those require a comment and human edit.
- Bad, because full-section replacement can still rewrite more prose than a
  small patch, even though its scope and resulting diff are explicit.
- Bad, because readiness validation and stale-record detection add complexity
  to the groom applier and its tests.
- Bad, because a semantic validation failure may make one additional paid
  external-agent call and increase command latency.
- Bad, because an operational failure during a multi-file batch can leave
  some atomically written records updated even though invalid delegated output
  never can; recovery uses ordinary Git/worktree behavior and a rerun.

## More Information

- Partially supersedes the grooming write-authority paragraph in
  [ADR-014](014-replace-managed-skills-with-command-based-procedures.md).
- Implementation is tracked by task 179.
- Live supported-agent verification is tracked by task 180.
- The task file format and unknown-field preservation rules are documented in
  [`docs/references/workflow-spec.md`](../references/workflow-spec.md).
