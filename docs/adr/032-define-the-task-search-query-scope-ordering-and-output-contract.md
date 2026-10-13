---
status: accepted
date: 2026-10-13
decision-makers: Travis Ennis
informed: task 295
---
# Define the task search query, scope, ordering, and output contract

## Context and Problem Statement

`ahm task search <query>` is a compatibility surface (`docs/cli.md`,
`ARCHITECTURE.md`), but four of its behaviors are unstated contract. Measured on
this store, as a snapshot while this decision was written (315 Completed, 42
Cancelled, 3 Open, 1 Pending, 1 In Progress, 1 Blocked):

- **Query grammar.** `taskSearch` (`internal/ahm/task_list.go:184`) joins its
  arguments with a space and matches the result as one literal
  case-insensitive substring. A multi-word query therefore matches only records
  where the words are adjacent: `ahm task search "task search"` returns 10 rows
  while `ahm task search search` returns 89. A caller who does not already know
  the exact phrasing must issue single-term queries and intersect them by hand.
- **Default scope.** Search scans every record (`filterTasks(tasks, "all")` at
  `internal/ahm/task_list.go:193`), so terminal history dominates: 5 of the 89
  rows above are active. Reaching the active matches requires spelling out
  `--status Open,Pending,"In Progress",Blocked,Tracking` on every query.
- **Ordering.** Title matches are listed before body-only matches, then in the
  priority-then-ID order that `filterTasks` returns. That grouping is a ranking
  rather than a documented sort, and `task search` has no `--sort` at all
  (`internal/ahm/task_commands.go:155`).
- **Output.** The text line is the `task list` line,
  `ID [Status] Priority Effort Title`, and marks nothing about which field
  matched; there is no count mode. `--plain` is documented to emit compact JSON
  for shared-emitter responses, but `task search` has custom text output and
  ignores it, so `--plain` and `--text` are currently byte-identical.

Tasks 296 (AND terms), 297 (priority/effort filters and `--sort`), 298 (match
context), and 299 (active default and `--all`) are separate `type:feature`
changes to that surface, two of them labeled `risk:breaking-change`. Each needs
the same decisions, so this ADR settles them once and those tasks implement it.

## Decision Drivers

- A search must be usable without knowing a record's exact wording.
- The default result set should be what a lookup usually wants; terminal history
  is rarely the answer to a search.
- Query grammar, default scope, ordering, and output are compatibility surfaces.
  Whatever ships must be stateable in one place and implemented by one rule.
- Prefer the smallest change that settles the contract; OR, exclusion, field
  targeting, escaping, and count output can be added later without contradicting
  this decision.
- Existing scripted consumers of `--plain` and `--json` should keep working.

## Considered Options

Query grammar:

- Keep one literal substring over the space-joined arguments (status quo).
- AND of whitespace-separated terms, with double-quoted spans matching
  literally (chosen).
- A full query language now, including OR, `-term` exclusion, and
  `title:`/`body:` field targeting.

Default scope:

- Keep all records and rely on `--status` to narrow (status quo).
- Default to active records with an explicit `--all` opt-in (chosen), leaving
  `task list` all-records.
- Default both `task search` and `task list` to active records.

Ordering:

- Keep the title/body grouping fixed and let `--sort` reorder within each group.
- Let `--sort` replace the grouping (chosen); without `--sort` the grouping and
  priority-then-ID order stand.

Output:

- Keep the unannotated line and add nothing.
- Annotate body-only matches in `--text`, keep `--plain` marker-free, and drop a
  count mode (chosen).
- Add a `--count` mode and a matched-snippet line now.

## Decision Outcome

Chosen option: AND terms with quoted phrases, an active-records default with
`--all`, a replaceable title/body ranking, and an annotated `--text` line over a
stable unannotated `--plain` line. Together these make the common query
(`ahm task search "release workflow"`) return the handful of active records that
matter instead of the full history, and they pin one rule for the four
follow-through tasks to implement.

### Query grammar

The query splits into terms: whitespace separates terms, except that whitespace
inside a double-quoted span belongs to that term. Every term must appear,
case-insensitively, in the record's title or body; a term is a substring match,
not a word match. A double-quoted span is one term that must appear as a literal
adjacent substring, so phrase matching is preserved rather than silently
dropped. Terms combine with AND only.

`"` is a delimiter with no escape sequence, and it must delimit a whole term: a
quote may appear only at the start of the query or after whitespace, and its
closing partner must be followed by whitespace or the end of the query. A query
that breaks that rule — an odd quote count, or a quote glued to other text — is
a usage error (exit 2) that names the problem, as is a whitespace-only query or
an empty span (`""`). Text containing a literal double quote is therefore
unsearchable until escaping is added. OR, exclusion (`-term`), and
`title:`/`body:` field targeting are deferred; each is additive and none changes
the results this decision produces.

### Default scope

`task search` defaults to the active statuses (Open, Pending, In Progress,
Blocked, Tracking). `--all` widens the default to every status, including
Completed and Cancelled. `--status` names an explicit set and replaces the
default, so `--status Completed` is enough on its own; `--all` and `--status`
together are a usage error instead of a precedence rule, because a hidden
precedence would be the kind of unstated contract this ADR exists to remove.

`task list` keeps its all-records default. It is the inventory view, its
`--status`, `--priority`, and `--effort` filters exist to slice a full listing,
and its default is its own contract. Search is a lookup, where a terminal record
is almost never the answer. The two commands therefore disagree by design, and
both help texts and `docs/cli.md` state the divergence. `task ready` and
`task blocked` are already scoped by mode and are unaffected.

### Ordering and ranking

Without `--sort`, results keep the title-before-body grouping: a record is a
**title match** when every term matches its title, and a **body match** when
every term matches somewhere but not all of them in the title. Title matches are
listed first; within each group the order is priority then task ID, as
`filterTasks` already produces. For a single-term query this is exactly today's
rule.

With `--sort <field>`, the title/body grouping is dropped and the requested
field orders the whole result set, with the existing ID tie-breaker and
`--reverse` behavior from `sortTaskList` (`internal/ahm/task_list.go:92`). The
grouping is a heuristic ranking; an explicit sort must be able to put the newest
or oldest match first across the entire result, which a fixed outer grouping
would prevent. The default output is unchanged by this clause, and the rule is
stated in the `--sort` help text and the command reference.

### Text output

`--text` (the default) keeps the `task list` line and marks body-only matches
with a trailing `[body]` marker; title matches stay unmarked because they are the
common case and keep the `task list` line intact. `--plain` emits the same lines
**without** the marker, byte-identical to today's `task search` output, which
makes it the stable, parseable form for scripts. `--json` is unchanged: it still
emits the parsed task structs, and a consumer that needs the matched field reads
`body`.

Task 298 narrows to this annotation: it adds the trailing `[body]` marker and
drops its matched-snippet and count lines, and it does not add a marker to
`--plain`. Its "plain output distinguishes title from body-only matches"
acceptance is met by `--plain` staying marker-free and parseable, not by adding
a marker there.

No count mode and no matched-snippet line ship now. `ahm --plain task search <q>
| wc -l` and `ahm --json task search <q> | jq length` already answer "how many",
and a fourth output shape is not worth a new compatibility surface.

### Consequences

- Good, because a query no longer depends on word adjacency, so an agent that
  does not know the exact phrasing finds the record.
- Good, because the default result set drops from the full history to the active
  matches, and `--all` is one typed flag rather than a spelled-out status list.
- Good, because grammar, scope, ranking, and output live in one record, so tasks
  296-299 implement a fixed rule instead of inventing one per task.
- Good, because `--plain` output is unchanged and `--json` is untouched, so
  existing scripted consumers keep working.
- Bad, because a multi-word query returns more records and no longer returns
  terminal records by default; both are breaking changes to the documented
  contract.
- Bad, because `task search` and `task list` now disagree on default scope, which
  every help text and the reference must repeat, and a caller who wants history
  types `--all`.
- Bad, because `--sort` yields a different ranking than the default grouping, and
  a query containing a bare `"` becomes a usage error.
- Bad, because the matched field is visible only in `--text`; structured
  consumers still infer it from `body`.

## More Information

- Task 295 owns this decision; tasks 296, 297, 298, and 299 implement it and
  cite this ADR.
- No existing ADR covers search or list ordering, and this ADR supersedes none.
  `sortTaskList` and its `--sort`/`--reverse` contract are referenced, not
  changed.
- Implementation sites: `internal/ahm/task_list.go` (`taskSearch` 184,
  `sortTaskList` 92, `filterTasks` 390), `internal/ahm/task_commands.go` (search
  flags 155-184), `docs/references/cli/task-commands.md` (search contract),
  `docs/cli.md` (compatibility contract), and
  `docs/references/cli/global-contract.md`, whose `--plain` description currently
  calls the flag ignored by custom-text commands and must say what it means for
  search.
- Breaking-change bookkeeping: no command, flag, record format, or storage
  location is removed, so no file under `docs/releases/` is owed. The breaking
  follow-through changes (296, 298, 299; 297 only adds flags and is additive)
  carry the `!` marker in their Conventional Commit subjects so `git-cliff`
  records them in `CHANGELOG.md`.
