---
status: accepted
date: 2026-10-04
decision-makers: Travis Ennis
---
# Record task block reasons in dedicated front matter fields

## Context and Problem Statement

A task's lifecycle status is stored in its front matter. `Blocked` is one of
the seven statuses, but no command could set it. The five status verbs are
`accept`, `start`, `complete`, `cancel`, and `reopen`; the only writers of
`Blocked` were hand edits and two internal paths that *clear* it — completing
a dependency moves a dependent from `Blocked` back to `Pending` (tasks 096 and
151).

Some tasks are blocked on something that is not another task's completion: an
unrecorded architecture decision, an upstream issue in another project, a
person, or a decision awaiting an ADR. `depends_on` cannot express that, and
forcing it into a fake dependency misrepresents the graph. Today such a task
reaches `Blocked` only by editing front matter, and its reason lives only in
prose in the body. `ahm prime` then reports a bare `Blocked: N` count, so an
agent briefing has to open every blocked record to learn why it is stuck.

Task 273 asked for an explicit block/unblock surface and a home for the
reason. The schema already carries an `external_ref` field, and the body can
carry a `## Comments` section, so three storage candidates existed.

## Decision Drivers

- A reason is prose; a URL is not a reason. A task blocked on an unrecorded
  decision has no URL at all.
- The reason must be queryable by the tool — `ahm task blocked` and `ahm
  prime` should surface it without opening the record — because discoverability
  is the gap being closed.
- Front matter is a compatibility surface. The change must be additive and
  round-trip safe, and it must not give an existing field a second meaning.
- Completing a dependency must keep auto-unblocking dependents, unchanged.
- The failure mode for a `Blocked` task with no reason should be a validation
  finding and a required flag, not a prose convention.

## Considered Options

- **New dedicated fields `blocked_reason` and `blocked_ref`.** An optional
  prose reason plus an optional external reference, omitted when empty like
  `created`, `updated`, `parent`, and `external_ref`.
- **Reuse `external_ref`.** Store the reason (or a URL) in the schema's
  existing, currently unused field.
- **A `Comments` entry.** Record the reason as a body comment.

## Decision Outcome

Chosen option: **new optional front-matter fields `blocked_reason` and
`blocked_ref`**, because the reason is structured state the tool must query,
and a dedicated field is the only option that gives it that without redefining
a field other records already use.

`blocked_reason` is the prose reason and is required while `status` is
`Blocked`. `blocked_ref` is an optional external reference — an issue URL, a
person, a decision awaiting an ADR — and is not a substitute for the reason.
Both are emitted only when non-empty, matching the existing optional fields.

Reusing `external_ref` is rejected because a URL is not a reason, a blocked
task may have no URL, and overloading the field would make `external_ref` mean
something different on a `Blocked` record than on every other record that
carries it (`ahm task create --external-ref`, `ahm task edit
--external-ref`). A `Comments` entry is rejected because comments are not
queryable by `ahm task blocked` or `ahm prime`, which is precisely the gap
this decision closes; a comment would preserve the status quo while looking
like a fix.

### Command surface

- `ahm task block <id> --reason <text> [--ref <text>]` sets `Blocked` from any
  non-terminal status (`Open`, `Pending`, `In Progress`, `Blocked`). `--reason`
  is required and must be non-empty after trimming; `--force` does not bypass
  it, matching `task cancel`. Blocking an already-blocked task rewrites the
  record so the reason can be corrected. A terminal task (`Completed`,
  `Cancelled`) is refused as a usage error. `--dry-run` reports the pending
  block, reason, and ref without writing.
- `ahm task unblock <id>` is the explicit release. It requires the task to be
  `Blocked` and returns it to `Pending`, clearing both fields. This is
  deliberately distinct from dependency auto-unblock: a task blocked on
  something other than a task can be released, and a task blocked *and*
  dependency-blocked is released only by this command (or by its dependencies
  completing).
- Returning to `Pending` is the same target dependency auto-unblock and
  `task reopen` already use, so there is one "ready again" status. A
  consequence is that unblocking an `Open` task accepts it; the alternative —
  remembering the prior status — would add hidden state to the record for a
  case the tool does not need to serve.

### Invariants

- `blocked_reason` and `blocked_ref` are present only while a task is
  `Blocked`. Every status transition away from `Blocked` clears both, so
  `complete`, `cancel`, `reopen`, `accept`, and `start` cannot leave a stale
  reason behind.
- Dependency auto-unblock clears both fields and writes no reason of its own.
- A `Blocked` task whose `blocked_reason` is empty produces a warning-tier
  validation finding `task_blocked_missing_reason`. This is what turns "a
  `Blocked` task with no reason" from a prose convention into a checked state;
  it flags records that reach `Blocked` by any route other than `task block` —
  a hand edit, an older release, `task create --status Blocked`, or
  `task import`.
- `ahm prime` names blocked tasks with their reasons instead of printing a
  bare count, in both text and structured output, and `ahm task blocked` prints
  a `reason:` line per task. A dependency-blocked `Pending` task, which has no
  stored reason, is shown as waiting on its incomplete dependencies.

### Consequences

- Good, because a reason is first-class state: `task blocked`, `prime`, and
  `--json` consumers read it structurally instead of parsing a body.
- Good, because the format change is additive. Records without the fields
  parse and round-trip unchanged, and unknown-field preservation is untouched.
- Good, because `depends_on` keeps its meaning: a non-task blocker no longer
  needs a fake dependency.
- Bad, because front matter is a compatibility surface, so this adds fields,
  a validation finding code, and two commands to the documented contract.
- Bad, because a repository can hold `Blocked` records written before this
  change that carry no reason; they warn until an operator records one.
- Neutral, because the two fields are not settable through `task edit`, which
  continues to leave `status` alone; `task block` and `task unblock` are the
  only writers, alongside dependency auto-unblock.

## More Information

- `docs/references/workflow-spec.md` owns the canonical front-matter order and
  the field semantics; `docs/references/cli/task-file-format.md` owns the
  validation finding codes; `docs/references/cli/task-commands.md` owns the
  command contract.
- Tasks 096 and 151 established dependency auto-unblock; this decision
  preserves it and additionally clears the reason fields.
- `task cancel --reason` (ADR 007) is the precedent for a required reason, but
  its reason lives in the body because it is provenance, not queryable state.
