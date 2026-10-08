# Preflight

Use this procedure after a change is functionally correct and before commit or
handoff. The branch handoff, commit text, task notes, and final response should
describe already-preflighted code.

## Goals

Leave the smallest clear diff that still solves the issue. Run focused
review passes instead of one subjective read. Preserve behavior while
improving readability, correctness, and alignment with repo rules.

## Scale the review to the change size

Pick an effort level from the diff before reading anything else:

```bash
git diff --stat
git status --short
```

Include untracked new files from `git status --short` (or
`git ls-files --others --exclude-standard`) when choosing the scale. A split
into new files can look deceptively small in `git diff --stat` until those
files are staged.

- **XS** (docs/config only, ≤2 files): Root `AGENTS.md` if relevant;
  one combined pass; one-line compliance note.
- **S** (single module, ≤~50 LOC, no public API): Root and nearest nested
  `AGENTS.md`; one combined pass; one-line compliance note.
- **M** (multi-file, ≤~200 LOC, no cross-module): Add the task record and
  ExecPlan if one exists; run Pass 1 and Pass 2; use a short compliance block.
- **L/XL** (cross-module, public API, agent loop, persistence, concurrency,
  external integrations, or security boundaries): Add relevant design docs
  and ADRs; run all three passes; use a full compliance block.

Only read context items that are relevant to the changed surface. Discover
them with targeted commands, e.g. `rg --files -g AGENTS.md`,
`rg --files docs/adr docs/exec-plans`, `git diff -- <paths>`.

Required context items, in priority order:

- repo root `AGENTS.md`
- nested `AGENTS.md` files for the changed areas
- the [task workflow](tasks.md) and `ahm task show <id>` output when
  the work came from a task; open the source task record under the records
  root only when `ahm` is unavailable or when reviewing manual edits to the
  task file itself
- the relevant active plan under `docs/exec-plans/active/` when one exists for
  the current work
- the [ExecPlan workflow](exec-plans.md) for L/XL changes
- any design doc or ADR directly relevant to the changed area
- the changed files and enough nearby context to review them

## Review passes

Treat each pass as a clean read with its own focus. Do not blur findings
across passes.

### Pass 1: Rules and documentation conformance

- Are we following `AGENTS.md`, nested `AGENTS.md`, and design docs?
- Did we drift from documented repo patterns or ownership boundaries?
- If the changed surface is user-visible CLI/API/config/file-format/workflow
  behavior, did we update the affected docs in the same change or record why
  the behavior is intentionally undocumented?
- If the work came from a task or ExecPlan, does the implementation match
  its acceptance notes and recorded decisions?
- Did we update task, ExecPlan, design doc, or ADR notes when the change
  discovered something durable?

### Pass 2: Correctness and source of truth

This pass is about project-native correctness at the changed surface.
Read the changed files with nearby context, then prefer explicit repo
instructions in `AGENTS.md`, [`CONTRIBUTING.md`](../../CONTRIBUTING.md),
`justfile`, `.github/workflows/`, `go.mod`, and existing tests over generic
advice.

Focus questions:

- Are we preserving canonical domain models, schemas, identifiers, and
  state machines, or did we stringify, parse, duplicate, or reshape data
  instead of carrying the project-owned representation?
- Did we introduce stringly typed sentinels, unvalidated dictionaries/maps,
  loosely shaped JSON, global state, or duplicated constants where the
  project normally uses a schema, class, struct, enum, type alias, database
  constraint, or shared config?
- Are fallible boundaries explicit about failure, with useful context and
  without swallowing parse, validation, network, filesystem, process,
  persistence, auth, or external-service errors?
- Are concurrency, async, transaction, lifecycle, and resource boundaries
  consistent with nearby code and the runtime in use?
- Are CLI/API/UI/database/config/external-integration boundaries validated
  at the edge and then represented with project-owned shapes downstream?
- Could an existing compiler, type checker, linter, schema validator,
  migration check, test helper, or narrower data model catch a mistake
  earlier than this implementation currently does?

### Pass 3: Overengineering and simplification

- Did we write more code than needed?
- Did we create helpers, abstractions, factories, wrappers, or indirection
  without enough payoff?
- Could the same result be expressed more directly?
- Are new modules, traits, builders, or generic helpers justified by real
  reuse or by an existing design boundary?

## Between-pass hygiene

Ground each pass in narrow local evidence; [`CONTRIBUTING.md`](../../CONTRIBUTING.md)
is the canonical command catalog. Use the smallest check that fits the change:

- `git diff --stat` and `git diff -- <paths>` to keep review anchored
- `just fmt` when formatting is affected
- focused tests in the changed area with `go test` or `just test`
- `go vet`, `golangci-lint`, or `just cli-parity` when public types, shared
  code, config, or command wiring changed
- `just ci`, the repository's final validation command, after code, config, or
  dependency changes are complete

For docs-only edits, run `just docs-md-lint` and verify links by inspection or
`rg --files`; the full CI suite is not required.

## Synthesis

After running the passes for the chosen scale, synthesize into one balanced
report with these headings:

- "How did we do?"
- "Feedback to keep"
- "Feedback to ignore"
- "Plan of attack"
- "Preflight compliance" (skip for XS; one line for S; short block for M;
  full block for L/XL — see template below)

## What to fix automatically

In an unattended implementation flow, apply worthwhile feedback before
commit. Prioritize:

- type drift, unnecessary cloning/string conversion, duplicated type defs
- violations of documented repo boundaries or design documents
- dead helpers, dead code, debug leftovers, placeholder text
- new panic/abort paths, placeholder exceptions, debug prints, commented-out
  code, broad lint suppressions, or ignored errors in production paths
- errors lacking actionable context at CLI/API/UI/database/config/process/
  network/external-service boundaries
- unnecessary wrappers or indirection removable locally without widening
  scope

Leave out feedback that is speculative, conflicts across passes, or would
widen scope materially. Mention it briefly in the synthesis.

## Compliance note

Make the chosen context auditable. Length scales with change size.

**XS / S example:**

```markdown
### Preflight compliance
- XS docs-only change to one workflow doc. Root AGENTS.md skim only; no
  nested AGENTS.md under the changed path; no CI required.
```

**M / L / XL template:**

```markdown
### Preflight compliance

- Root AGENTS.md: read
- Nested AGENTS.md: <paths or "none under changed paths">
- Task context: <task id> / not applicable because <reason>
- ExecPlan: <plan id under `docs/exec-plans/`> / not applicable because <reason>
- Design docs: <docs> / not applicable because <reason>
- ADRs: <adrs> / not applicable because <reason>
- Documentation impact: <docs checked/updated, or intentionally none because ...>
- Changed files and diff: reviewed via `git diff --stat` and targeted diffs
- Validation: <commands run>
```

Do not write blanket "no design docs to check" claims unless you actually
looked for a relevant one and can explain why the changed area has no
design-doc surface.

## Steps

1. Run `git diff --stat` and `git status --short`. Pick a scale from the
   categories above, counting untracked new files.
2. Read only the required-context items for that scale.
3. Run the review passes for that scale, with a narrow evidence check
   between them.
4. Synthesize findings into the balanced report.
5. Apply worthwhile feedback that is clearly in scope.
6. Rerun the narrowest affected validation, then the repo's documented
   final validation command when the finished work changed code, config, or
   dependencies.
7. Update task notes, ExecPlan notes, commit text, and the branch handoff to
   describe the post-preflight state.

## Stop rules

- Do not turn this into a refactor unrelated to the ticket.
- Do not churn stable code outside the changed area just to make it
  prettier.
- If a cleanup is subjective and not clearly better, leave it alone.
- Do not blindly apply every finding from every pass.
- Do not run broad or slow checks repeatedly when a focused test already
  covers the current pass; save the repo's broad validation command for
  final validation.
- Do not escalate the scale beyond what the diff justifies just to feel
  thorough.
