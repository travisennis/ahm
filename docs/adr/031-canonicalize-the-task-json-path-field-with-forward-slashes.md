---
status: accepted
date: 2026-10-09
decision-makers: Travis Ennis
---
# Canonicalize the task JSON path field with forward slashes

## Context and Problem Statement

ADR 023 promised that in `project` mode, command output is byte-identical to the
behavior before the home store. To keep that promise literally, the JSON `path`
field of a task (`task list --json`, `task show --json`, `task next --json`,
`task search --json`, and the `path` field of a `task edit` report) returned the
record's own absolute path in the platform's own separator. On Windows that
meant backslashes, while every other path in the same payload — the dry-run
create, move, and unblock previews, the generated index links, and the `store:`
rendering — used forward slashes. So one payload could carry both separators on
Windows (tracker task 280).

Forward slashes are valid on Windows and are what most tooling emits. The
inconsistency forced consumers to handle two separator styles for one field, and
a Windows consumer that needs a usable path already normalizes paths from every
other tool.

## Decision Drivers

- One payload should not mix path separators.
- The field should agree with the previews, the index links, and the `store:`
  rendering that already use forward slashes.
- The change should touch only Windows output and only this one field.

## Considered Options

- **Keep the native separator.** The byte-identity promise stays literally true,
  but the field disagrees with every other path in the same payload.
- **Canonicalize the field with forward slashes on every platform.** The field
  matches the rest of the payload and tooling conventions, at the cost of
  narrowing ADR 023's byte-identity promise for this field.

## Decision Outcome

Chosen option: **canonicalize the JSON `path` field with forward slashes on
every platform**, because one payload mixing two separators is the worse
inconsistency, forward slashes are valid everywhere, and only Windows output
changes.

The field is rendered by `workflowPaths.recordPath`, which now also renders the
record path in the dry-run create, move, and unblock previews, so a record's
path has one rendering for structured and preview output. This partially
supersedes ADR 023's statement that project-mode command output is
byte-identical to the behavior before the home store: that statement now holds
for every field except this one, which is canonicalized.

### Consequences

- Good, because one payload never mixes separators, so a consumer parses one
  path style.
- Good, because the field now agrees with the previews, the index links, and
  the `store:` rendering.
- Bad, because a Windows consumer that expected the native separator must
  normalize this field; the change is otherwise invisible outside Windows.
- Bad, because ADR 023's byte-identity promise is now qualified rather than
  literally true.

## More Information

- Tracker task 280.
- Partially supersedes ADR 023 (store task records in a user-level home store),
  whose byte-identity claim for project-mode command output now carries this
  field as an exception.
