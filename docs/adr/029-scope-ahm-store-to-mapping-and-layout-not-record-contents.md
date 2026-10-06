---
status: accepted
date: 2026-10-06
decision-makers: Travis Ennis
---
# Scope ahm store to mapping and layout, not record contents

## Context and Problem Statement

`ahm store` grew by accretion. It holds two subcommands, `path` and `migrate`,
each added to answer one question as it arose, and it has no stated policy about
what belongs in it. Tasks 276 and 277 propose two more. That is the failure ADR
022 was written against: a command family whose scope nobody can predict, where
each new gap is answered by one more subcommand.

The demand behind the proposals is real. Observed across two projects in one
store session: there is no way to enumerate what the store holds; no way to
remove a stale registry entry or a dead path, so recovery is hand-editing a
machine-level `registry.json` that has no Git safety net; no store-scoped
diagnostic, because `ahm doctor` checks one selected project at a time; and no
way to see the store as a whole, which is what lets a phantom entry hide.
Pointing `--root` at a directory that is not a project — `ahm --root <dir>
store path` — registered a brand-new path-derived project for it.

The counterweight is ADR 022, which narrowed `ahm` to a tasks-and-ADRs records
CLI, and task 256, `store export` and `import`, which was cancelled precisely
because it was an unanswered product question: snapshotting or moving the
backlog "is a new product question the reduced boundary does not answer."
Adding a family of store commands by demand would reopen that question by
accumulation, which is the opposite of how ADR 022 asked for it to be settled.

This decision states the family's principle and settles the queued proposals
against it. It adds no subcommand.

## Decision Drivers

- One predictable rule about what `ahm store` may gain, so a future gap is
  answered by the rule rather than by another subcommand.
- Consistency with ADR 022's narrowing and with task 256's cancellation, with
  the tension stated rather than ignored.
- Keep the shipped surfaces (`path`, `migrate`) inside whatever rule is chosen,
  instead of writing a rule that has to read them out.
- A store a user can inspect and repair without hand-editing machine-level
  state, and without a project root where inspection has no use for one.
- No command that acts on record contents until that question is decided on its
  own.

## Considered Options

- **Strictly read-only.** Every `store` command reads; nothing mutates. Simple,
  but it excludes the shipped `migrate` and would also exclude the registry
  repair the store most needs.
- **The store's mapping and layout.** `ahm store` owns where records live: the
  store location, the registry mapping, and the records layout (`project` or
  `home`). It never owns record *contents*.
- **An open family.** Add `list`, `unregister`, a store diagnostic, and later
  `export`, `import`, and `purge` as demand appears.
- **Split the group.** Separate inspection (`path`, `list`, a store check) from
  migration (`migrate`) into two top-level commands.

## Decision Outcome

Chosen option: **the store's mapping and layout**, because it is the one rule
that keeps both shipped commands and the two queued ones inside it without an
exception, and it draws the boundary where task 256 actually stopped.

The principle: **`ahm store` manages the store's location, registry, and
records layout, never record contents.** A store command may observe or edit
the mapping from a project to its store directory, and may relocate records
between `ahm`'s two supported layouts; it may not export, import, snapshot,
purge, or otherwise add or remove record content. Record content is task 256's
question, and it stays answered the way 256 left it: not yet.

The "strictly read-only" option is rejected because it reads the wrong axis. It
would contradict the shipped `migrate`, whose whole job is to move records, and
it would exclude `unregister`, which mutates the registry but no record. The
defect the proposals expose is not that commands write; it is that nothing
distinguishes managing the store from managing the backlog. "An open family" is
rejected because it reopens 256 by accumulation. Splitting the group is
rejected for now: the in-scope commands all resolve the same store root, share
the same registry and lock, and serve one user in one sitting (inspect, then
repair), so a second top-level command would add surface without a distinct
audience. Reconsider the split only if a record-content command is ever
admitted, which this decision forbids.

### The command list falls out

In scope under the principle:

- `store path` (shipped) — observes and records this project's store location.
- `store migrate --to home|project` (shipped) — moves records between the two
  supported layouts.
- `store list` — task 277. Read-only inventory of the store. **Accepted.**
- `store unregister` — task 276. Edits the registry mapping, never records.
  **Accepted.**
- A store-scoped diagnostic (a store-wide `doctor` or `check`) is in scope in
  principle, because it reads the store rather than one project. It is not
  tasked here.

Out of scope under the principle:

- `store export` and `store import` — task 256, cancelled. **Rejected** until
  256 is reopened with a task-and-ADR-only scope and decided on its own.
- Any `purge`, `forget-records`, or `--purge-records` flag, and anything else
  whose primary effect is on record contents. Rejected for the same reason.

### Project root and the phantom entry

Inspection must not require a project root and must never create state. `store
list`, and any store-scoped diagnostic, run from any directory and register
nothing; a listing that had to run inside a project could not report the
projects the user is not in.

The recording commands are different: `store path` observes the current project
on purpose, so it needs a resolvable project. The defect is that it records one
for a directory that is not a project. `--root` bypasses root detection, so
`ahm --root <dir> store path` derives a path-based key for any directory and
registers a phantom entry and a store directory that nothing will ever clear.
The decision: **a `store` command that records refuses a root that is not a
managed project** — one holding neither `.git` nor `.ahm/config.json` — instead
of deriving a path key for it. Root detection's existing managed-root rule is
the test; a directory that is a managed project, or a Git checkout not yet
initialized, still records normally. This is recorded here and implemented as
its own change, because it is a behavior fix independent of the command list.

### What stays true

- The registry is derived data and never the authority for record contents;
  removing an entry is recoverable, because a later `store path` re-registers
  the project.
- `store migrate` remains the only command that moves a record between the two
  layouts. No store command creates, removes, or transforms a record's content
  outside that relocation.
- `ahm prime` stays pure state; store inventory is never added to the briefing.

### Consequences

- Good, because the family has one rule, stated in the `store` help text, that
  answers "does this belong?" without growing the command list by default.
- Good, because both shipped commands and the two queued ones fall inside the
  rule, so nothing needs a grandfather clause.
- Good, because the boundary lands where task 256 was cancelled: content
  operations stay a separate, still-open product question.
- Bad, because there is no command to move a backlog between machines or take a
  backup, and a user who wants one must reopen 256 rather than find a
  subcommand.
- Bad, because the phantom-entry fix is a behavior change to a shipped command
  and must be released as one.
- Neutral, because the family's placement is unaffected: `store` stays one
  group, documented as the store's location, registry, and layout surface.

## More Information

- Task 278 decides this; tasks 276 (`store unregister`) and 277 (`store list`)
  implement two accepted commands, and task 288 implements the managed-root
  refusal this decision requires.
- ADR 022 narrows `ahm` to a tasks-and-ADRs records CLI and requires
  `docs/VISION.md` to state the store family's place in that boundary.
- ADR 023 defines the home store and its registry; ADR 024 serializes registry
  writes.
- Task 256 was cancelled as an unanswered product question; this decision keeps
  it cancelled and names it as the place to reopen export and import.
