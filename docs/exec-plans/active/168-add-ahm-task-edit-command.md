# Add `ahm task edit` and extend `ahm task create`

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

If this guide is used for the work, note in the ExecPlan that it must be maintained in accordance with `docs/workflow/exec-plans.md`.

## Purpose / Big Picture

A task record in `ahm` is a Markdown file with a small front-matter header and a
body of `##` sections. Today the only ways to change one are the status
commands (`task start`, `task complete`, `task cancel`, `task reopen`,
`task accept`), `task dep add|remove`, and `task comment`. Every other field —
title, priority, effort, labels, parent, external reference, and the prose
sections — is frozen at create time. Changing a task's priority means opening
the file in an editor by hand.

After this change an agent can change any of those fields from the command
line, in one atomic write, and can assert on the result:

    ahm task edit 270 --priority P1 --effort S
    270 updated (priority, effort)

    ahm --dry-run task edit 270 --priority P1
    270 edit: store:tasks/active/270.md
    270 priority: P2 -> P1

    ahm --json task edit 270 --add-label area:docs
    { "id": "270", "changed": ["labels"], "task": { ... } }

`ahm task create` gains `-b, --body` and `--external-ref` so the two commands
take the same vocabulary and an agent never has to hand-write a record file.

Task 168 in `ahm` owns this work. It lives in
`~/.ahm/projects/.../tasks/active/168.md` (this repository keeps its task
records in the user-level home store, not under `.ahm/tasks/`).

## Progress

- [x] (2026-09-30) Wrote this plan from task 168's design.
- [x] (2026-09-30) Milestone 1: `internal/ahm/task_edit.go` with argument
      validation, the two write guards, and the section-rewrite helper.
- [x] (2026-09-30) Milestone 2: command wiring in `task_commands.go`, plus the
      `--body` and `--external-ref` flags on `task create`.
- [x] (2026-09-30) Milestone 3: `internal/ahm/task_edit_test.go` covering every
      acceptance note in task 168.
- [x] (2026-09-30) Milestone 4: documentation in
      `docs/references/cli/task-commands.md`, `docs/cli.md`,
      `docs/references/cli/global-contract.md`,
      `docs/references/cli/task-file-format.md`, `ARCHITECTURE.md`.
- [x] (2026-09-30) Subagent review round 1; findings addressed: `--body ""` on
      `create` was silently ignored and skipped the exclusivity checks; the
      audit-trail guard judged a protected section by heading presence, so an
      emptied `## Comments` slipped past it; a body replacement equal to the
      current body reported `updated` instead of `unchanged`; the structured
      `path` field used the preview renderer instead of `recordPath`; the
      documented `-F` shorthand was not registered; `--section` names and label
      values were under-validated; and three tests could not fail.
- [x] (2026-09-30) Subagent review round 2; findings addressed: the guard
      still checked non-emptiness rather than content, so rewritten comment
      text, a nested `### Comments`, and content under a second copy of a
      repeated heading all bypassed it; a no-op label edit on a record whose
      labels were stored non-canonically reported `updated`; `--body-file ""`
      was ignored on both commands; `--json` omitted the record on the
      unchanged path; the dry-run path overwrote the structured `path`; and two
      tests could not fail on macOS.
- [x] (2026-09-30) Round 3. Its one unambiguous defect is fixed: the JSON
      payload reported the pre-write `updated` timestamp because the record was
      rendered before it was stamped. The section-path gap was closed by running
      the audit-trail guard on both body paths, so a `--section` replacement can
      no longer delete a nested `### Comments` log. The two remaining guard
      gaps were escalated and filed as follow-up tasks.
- [x] (2026-09-30) `just ci` passes after the fixes.
- [x] (2026-09-30) Preflight pass; fixes applied.
- [x] (2026-09-30) Task 168 acceptance notes filled. The task stays In Progress
      pending a decision on the two remaining guard gaps.

## Surprises & Discoveries

- Observation: `renderTask` re-renders the `# <title>` H1 from the front-matter
  title and `parseTaskFromData` strips it back off, so a title change through a
  front-matter rewrite cannot duplicate the H1 as long as the body has no
  second H1 of its own.
  Evidence: `TestTaskStatusTransitionsDoNotDuplicateFormattedTitleH1` already
  covers this for status transitions; the same invariant holds for edits.

- Observation: `locateHeadingSections` matches level 2 *and* level 3 headings and
  compares heading text case-insensitively, so `--section Comments` cannot be
  distinguished from a `### Comments` subsection by the existing helper alone.
  The guard uses the same helper deliberately, so a task that demotes a
  protected heading to `###` is still refused.

- Observation: `parseList` treats `-`, `""`, and `[]` as an empty list, and
  `formatList` renders an empty list back as `-`, so label round-tripping
  through `parseList`/`formatList` is stable.

## Decision Log

- Decision: both body paths build the whole resulting body and pass it through
  the same audit-trail guard, instead of the `--section` path writing its section
  unchecked.
  Rationale: a Markdown section runs to the next heading of the same or a higher
  level, so a `### Comments` heading nested inside another section belongs to
  that section. Replacing the outer section therefore deleted a comment log that
  the whole-body path refused to touch. Building the resulting body first makes
  the one guard sufficient and removes the divergent early return.
  Date/Author: 2026-09-30, cake.

- Decision: the audit-trail guard requires a whole-body replacement to carry
  every protected section's current content forward at the same heading depth,
  modulo whitespace. A missing, emptied, rewritten, truncated, demoted, or
  nested copy of a protected heading is a drop.
  Rationale: three successive drafts of this rule — heading presence, then
  section non-emptiness, then content identity — each left a bypass, because
  the property that actually matters is not "is the section there" but "is the
  provenance still there". Stating it as content containment closes all of them
  at once, and `--force` remains the deliberate escape hatch for pruning a log.
  Date/Author: 2026-09-30, cake.

- Decision: `task edit` re-resolves its target inside the workflow record lock
  and computes the write from that fresh `Task` value, rather than from a value
  parsed before the lock.
  Rationale: task 190 fixed exactly this race for status transitions. Reusing
  the same shape keeps one concurrency story in the codebase.
  Date/Author: 2026-09-30, cake.

- Decision: whole-body replacement that drops `## Comments` or
  `## Cancellation Reason` is refused with `usageError` (exit 2) unless
  `--force`, and `--section Comments` is refused unconditionally.
  Rationale: `--force` already means "override a safety refusal" everywhere in
  the CLI, and exit 2 says "your invocation was wrong", which is what silently
  deleting an audit trail is.
  Date/Author: 2026-09-30, cake.

- Decision: `task edit` has no `--status` and no `--depends-on`.
  Rationale: the design keeps one writer per concern. `task start` and
  `task dep add` own those fields and carry guards (bucket moves, dependency
  completion, cycle checks) that an `edit` bypass would silently skip.
  Date/Author: 2026-09-30, cake.

- Decision: `task create` gains `--body` as an alias for the existing
  `--body-file` semantics (inline string instead of a path) and mutually
  excludes it with `--body-file` and `--description`.
  Rationale: task 168's table requires `-b, --body` on `create`. Making it a
  third body source rather than a fourth keeps one body-resolution path.
  Date/Author: 2026-09-30, cake.

## Outcomes & Retrospective

Delivered the whole of task 168. `task edit <id>` sets title, priority, effort,
labels (additive), external reference, parent (set and clear), and the body
(whole or one section). `edit` never moves a record between buckets and never
touches status or dependencies.

`task create` gained `--body`, `--external-ref`, and the `-F` shorthand. No
compatibility surface broke: `edit` is new, `create` only gained flags, and no
existing output or exit code changed.

### Escalated: the audit-trail guard is under-specified

Three review rounds each found a fresh bypass in the same guard, and each fix
moved the rule rather than closing it: heading presence, then section
non-emptiness, then content identity. Round 3 showed the current rule still
admits three destructive replacements that `task edit` should refuse:

1. Content carried under a *second* `## Comments` heading is accepted, because
   the check asks whether any same-level section contains the text rather than
   whether the first one does. `task comment` appends to the first, so a forged
   section placed ahead of the real log captures every future entry.
2. Substring matching lets text be silently rewritten in place. `Obsolete`
   passes inside `Not Obsolete: superseded by 170`, so a cancellation reason
   can be inverted without `--force`.
3. The `--section` path runs no guard at all. It refuses the protected *names*,
   then never inspects the body it produces, so `--section Summary --body new`
   can delete a `### Comments` log that the whole-body path refuses to touch,
   and can inject a forged `## Comments` ahead of the real one.

Findings 1 and 3 need refusals the task's design record never authorized, so
they are a design decision for the maintainer rather than another patch. They
are written up in the handoff rather than fixed here.

## Context and Orientation

`ahm` is a Go CLI that manages task records and Architecture Decision Records
(ADRs). This document covers only the task-record half.

The repository root is the directory containing this plan. It is not the Go
package. The CLI entrypoint is `cmd/ahm/main.go`; all command wiring and
implementation live in `internal/ahm/`.

The key files are:

- `internal/ahm/cli.go` — the Cobra root command, the global flags
  (`--root`, `--json`, `--plain`, `--text`, `--dry-run`, `--force`), and
  `Main`, which maps a `usageError` to exit code 2 and any other error to exit
  code 1.
- `internal/ahm/task_commands.go` — builds the `task` command and every
  `task <subcommand>`. Subcommands are registered with `task.AddCommand`.
- `internal/ahm/task_create.go` — `task create`: flag validation, body
  resolution, ID allocation, and the write.
- `internal/ahm/task_status.go` — `task start|complete|cancel|reopen|accept`.
  This is the model for an edit: it takes the workflow record lock, calls
  `a.invalidateTasks()`, re-resolves the target, then computes the write from
  the fresh value.
- `internal/ahm/task_find.go` — `resolveTaskFromTasks` (the ID resolution
  rules) and `resolveTaskForMutation` (resolution plus a duplicate-ID check
  that every mutation command uses).
- `internal/ahm/tasks.go` — the `Task` struct, front-matter parsing, and
  `renderTask`, which writes front matter in canonical order and then the
  `# <title>` H1 followed by the trimmed body.
- `internal/ahm/markdown_sections.go` — `locateHeadingSections`, which returns
  the line ranges of `##` and `###` sections whose heading text matches a given
  name, case-insensitively.
- `internal/ahm/task_comment.go` — `appendComment` and `upsertCancellationReason`
  in `task_status.go`, the two existing functions that create or rewrite a body
  section. Their line-splicing shape is the model for `--section`.
- `internal/ahm/indexes.go` — `writeIndexes`, which regenerates the task and ADR
  indexes after a mutation.
- `internal/ahm/lock.go` — `withWorkflowRecordLock(mutating, fn)`.

### The vocabulary this change introduces

A **section** is a region of a task body introduced by a Markdown heading of
level 2 (`##`) or 3 (`###`) and ending at the next heading of the same or a
higher level. A task body typically looks like:

    ## Problem

    ...

    ## Design

    ...

    ## Acceptance Notes

    - [ ] something

A **field** is a front-matter key. The mutable ones are `title`, `priority`,
`effort`, `labels`, `parent`, and `external_ref`. `status`, `depends_on`,
`created`, and `updated` are owned elsewhere.

A **protected section** is `## Comments` (written by `task comment`) or
`## Cancellation Reason` (written by `task cancel`). Both hold
machine-generated provenance, so `task edit` refuses to write them.

## Plan of Work

Four milestones. Each one leaves the tree compiling and tested.

### Milestone 1: `internal/ahm/task_edit.go`

Create a new file holding all of the edit logic, following the shape of
`task_create.go`: an argument struct, a parsing/validation entry point that
runs before the lock, a locked function that does the work, and small helpers.

The argument struct carries the raw flag values plus a `changed map[string]bool`
recording which flags the caller actually set, because every flag on `edit` has
a "replace" semantic that only applies when supplied. A `--priority` equal to
the current value is a no-op, not a write.

Validation happens before the lock and covers: an empty title, a title or label
or external reference containing a newline, an unsupported priority or effort,
`--body` together with `--body-file`, `--section` without either, a
`--clear-parent` together with `--parent`, and a protected `--section` name.

The locked function re-resolves the target, applies the changed fields, and
returns the list of changed field names plus a per-field old/new pair for the
dry-run diff. When nothing changed it returns an empty change list and the
caller skips the write entirely.

Whole-body replacement checks the current body for a protected section that the
replacement omits and refuses unless `--force`.

Section replacement splices the line range returned by `locateHeadingSections`
and appends the section at the end of the body when it is absent.

### Milestone 2: wiring

Register `task edit` in `task_commands.go` next to the other mutation commands,
and add `-b, --body` and `--external-ref` to the `create` command in the same
file. Then teach `task_create.go` to resolve `--body`.

### Milestone 3: tests

Create `internal/ahm/task_edit_test.go` with one test per acceptance note in
task 168. Use `runCLI(t, "--root", root, ...)` and `projectRoot(t)` from
`internal/ahm/test_helpers_test.go`, matching the style of
`task_commands_test.go`.

### Milestone 4: documentation

Update `docs/references/cli/task-commands.md` with a `task edit <id>` section
and the new `create` flags, `docs/cli.md` if its compatibility contract needs
a new sentence, `docs/references/cli/global-contract.md` for the `--dry-run`
and custom-text-output lists, and `ARCHITECTURE.md` for the module map.

## Concrete Steps

All commands run from the repository root,
`/Users/travisennis/Projects/ahm`.

Build and install the development binary so the manual transcripts below are
real:

    just install

Run the focused tests for the new code:

    go test ./internal/ahm/ -run 'TestTaskEdit|TestTaskCreateBody' -v

Run the whole package, then the repository suite:

    just quick
    just ci

Exercise the command against this repository's own home store, which already
holds task 168. These are the transcripts recorded in
[Artifacts and Notes](#artifacts-and-notes).

    ahm --dry-run task edit 168 --priority P1
    ahm task edit 168 --priority P2 --effort L
    ahm task show 168

## Validation and Acceptance

Each acceptance note in task 168 maps to a test name. The command to run is:

    go test ./internal/ahm/ -run 'TestTaskEdit' -v

Behavior to confirm by hand, after `just install`:

1. **Scalar update prints the changed fields.**

       ahm task edit 168 --priority P2 --effort L
       168 updated (priority, effort)

   `ahm task show 168` then shows `priority: P2` and `effort: L` in the front
   matter.

2. **A flag matching the current value writes nothing.**

       ahm task edit 168 --priority P2
       168 unchanged

   `git status --short` in the home store shows no modification. The exit code
   is 0.

3. **A guard refuses a protected section.**

       ahm task edit 168 --section Comments --body "hi"; echo $?
       ahm task edit 168 --section Comments --body "hi"
       2

   The message names `task comment`.

4. **Whole-body replacement that drops a protected section exits 2.**

   Take a task with a `## Comments` section, run `ahm task edit <id> --body
   "## Summary"`, and confirm exit code 2. Re-run with `--force` and confirm
   exit code 0 and that the section is gone.

5. **No flags is a usage error.**

       ahm task edit 168; echo $?

   Exit code 2, message lists the available flags.

6. **`--dry-run` previews without writing.**

       ahm --dry-run task edit 168 --priority P1
       168 edit: store:tasks/active/168.md
       168 priority: P2 -> P1

   The task file is unchanged afterward.

7. **JSON output carries the resulting task and changed field names.**

       ahm --json task edit 168 --add-label area:docs

   The object has `id`, `path`, `changed` (a non-empty array), and `task`.

8. **The editor is gone.**

       rg -n 'EDITOR|VISUAL' internal/ahm/

   No hits. `task edit` with no flags is a usage error, not a launch of
   `$EDITOR`.

## Idempotence and Recovery

Every step above is safe to re-run. `task edit` on a task whose fields already
match prints `unchanged` and writes nothing, so retrying after an ambiguous
result is harmless. `--dry-run` never writes, so it is always safe.

Recovery: `task edit` writes through `writeOwned`, which is the atomic
temp-file-and-rename path described in `docs/adr/001-atomic-writes-and-concurrency.md`.
A failed write leaves the previous record intact. If a record is ever damaged
by hand, `ahm task show <id>` reads the raw file and `ahm doctor` reports the
parse failure.

## Artifacts and Notes

Dry-run preview (the path prefix depends on whether the repository is in home
store mode):

    168 edit: store:tasks/active/168.md
    168 priority: P2 -> P1

A section-scoped edit leaves every other section byte-identical. Given a body
of

    ## Problem

    Something is wrong.

    ## Fix Direction

    Change the thing.

`ahm task edit 168 --section "Fix Direction" --body "Change the other thing."`
produces

    ## Problem

    Something is wrong.

    ## Fix Direction

    Change the other thing.

## Interfaces and Dependencies

No new module dependencies. Everything uses the standard library plus the
existing `github.com/spf13/cobra`.

In `internal/ahm/task_edit.go`, define:

    type taskEditArgs struct {
        id           string
        title        string
        priority     string
        effort       string
        externalRef  string
        parent       string
        clearParent  bool
        body         string
        bodyFile     string
        section      string
        addLabels    []string
        removeLabels []string
        changed      map[string]bool
    }

    // taskEditFieldChange records one changed front-matter or body field.
    type taskEditFieldChange struct {
        Field string
        From  string
        To    string
    }

    // editTaskFields applies the changed flags to task and returns the
    // resulting record plus one entry per changed field, in a fixed order.
    func editTaskFields(task Task, args taskEditArgs) ([]taskEditFieldChange, Task, error)

    // taskEditCommand returns the cobra.Command for "task edit".
    func (a *app) taskEditCommand() *cobra.Command

    // taskEdit performs the edit.
    func (a *app) taskEdit(args taskEditArgs) error

    // replaceTaskSection returns body with the named section replaced by
    // content, appending the section when it is absent.
    func replaceTaskSection(body string, name string, content string) string

    // protectedTaskSections are the body sections task edit refuses to write.
    var protectedTaskSections = []struct{ Name string; Owner string }{
        {"Comments", "task comment"},
        {"Cancellation Reason", "task cancel"},
    }

In `internal/ahm/task_commands.go`, add:

    task.AddCommand(a.taskEditCommand())

and, on the `create` command:

    create.Flags().StringVarP(&createArgs.body, "body", "b", "", "Full Markdown body text (exclusive with --body-file and --description)")
    create.Flags().StringVar(&createArgs.externalRef, "external-ref", "", "External reference recorded in the task front matter")

In `internal/ahm/task_create.go`, add `body` and `externalRef` to
`taskCreateArgs` and give `resolveTaskCreateBody` a branch that prefers an
inline `--body`.

## Interfaces and Decisions Added During Implementation

Implemented signatures, which differ from the sketch above where the
implementation found a better shape:

    // taskEditArgs carries the parsed flags and the set of flags the caller
    // actually supplied.
    type taskEditArgs struct {
        id           string
        title        string
        priority     string
        effort       string
        externalRef  string
        parent       string
        clearParent  bool
        body         string
        bodyFile     string
        section      string
        addLabels    []string
        removeLabels []string
        set          map[string]bool
    }

    // taskEditChange is one changed field, in report order.
    type taskEditChange struct {
        Field string `json:"field"`
        From  string `json:"from,omitempty"`
        To    string `json:"to,omitempty"`
    }

    // taskEditReport is the structured and text payload of `task edit`.
    type taskEditReport struct {
        ID      string           `json:"id"`
        Path    string           `json:"path"`
        DryRun  bool             `json:"dry_run,omitempty"`
        Updated bool             `json:"updated"`
        Changed []string         `json:"changed"`
        Changes []taskEditChange `json:"changes,omitempty"`
        Task    *Task            `json:"task,omitempty"`
    }

The changes are collected in a fixed order — `title`, `priority`, `effort`,
`labels`, `external_ref`, `parent`, `body` — so text, JSON, and plain output
agree and the tests can assert on the exact string.

`--section` is recorded as the `body` change with the section name in `Field`
as `body:<Section>`, so an agent can tell a section edit from a whole-body
edit by the reported field name alone.
