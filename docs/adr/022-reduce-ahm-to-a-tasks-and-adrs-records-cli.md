---
status: accepted
date: 2026-09-20
decision-makers: Travis Ennis
---
# Reduce ahm to a tasks-and-ADRs records CLI

## Context and Problem Statement

Ahm's stated direction is to be "the runtime for structured agent work in a
repository" (`docs/VISION.md`): four record families (research, ADRs,
ExecPlans, tasks), a binary-owned procedure channel (`ahm context <family>`),
advisory commands that delegate to external CLIs (`ahm audit`,
`ahm task groom`), a delegation runtime (`ahm task work`, with agent
definitions, session capture, resume, review, and commit handoff), an
installer with an upgrade path and four record migrations, and an onboarding
snippet that bootstraps the workflow.

That boundary has outgrown its value. The tool has exactly one consumer
repository, and the surfaces that cost the most are the ones that carry no
mechanical responsibility:

- **Delegation and advisory machinery.** `task_work.go`, `task_agents.go`,
  `task_session.go`, `task_parsers.go`, `audit.go`, and `task_groom.go` total
  roughly 2,300 non-test lines, plus golden transcripts and fixtures (112K),
  smoke recipes that make real provider calls, and a dedicated guardrail.
  Their job is to hand the work to an external CLI whose arguments, output
  shapes, and session semantics change on that CLI's schedule.
- **The procedure channel.** `ahm context task|plan|adr|research` emits
  workflow instructions. That is prompt content: it tells a project how to
  work, and it changes whenever this repository's opinion about working
  changes.
- **Record families ahm never needed to own.** Research notes and ExecPlans
  carry no integrity semantics that tasks and ADRs do not already cover:
  they are prose in a directory, indexed and validated by machinery whose
  only output is a list of files.

Measured on the current tree: 12,302 non-test lines of Go, of which about
4,400 (36%) belong to the surfaces above; 18,552 test lines, of which about
4,000 (21%) test them; 135 references to the five commands across 28
documentation and instruction files; and 9 accepted ADRs whose decisions
would no longer be implemented. Roughly 15 of the 26 queued tasks — including
every open security-shaped one — exist only because of delegation.

The owner's judgment, stated directly: ahm should get out of the business of
telling projects how to work and become a CLI for managing tasks and ADRs.
The question is how to narrow the product boundary without keeping
half-measures, and how to treat the existing command, configuration,
record-family, and instruction surfaces honestly.

## Decision Drivers

- Give ahm one boundary that matches what it does mechanically: parse, store,
  index, and validate structured records.
- Stop tracking external agent CLIs. Argument builders, output parsers,
  session capture, and resume logic inherit churn from tools ahm does not
  control.
- Leave prompt and procedure content with the project that owns the judgment
  it encodes, accepting that prose can drift, because drift in a project's own
  instructions is a project cost, not a tool contract.
- Keep the integrity guarantees that are genuinely ahm's: atomic writes,
  repository-local locks, deterministic indexes, record validation, stable
  record formats.
- Make the narrowing a visible, single, breaking change rather than a slow
  accumulation of deprecated aliases that preserve the rejected boundary.
- Preserve historical records and delete the directories a consumer may still
  read only under project control, never by inference.

## Considered Options

- **Keep the four record families and delete only the five commands.** The
  database and formats stay; research and ExecPlans keep their validators,
  indexes, and templates, with no command that explains how to manage them.
- **Keep research and ExecPlans as data; delete only prescription.** No
  `context`, no delegation, no advisory commands, but four families and their
  index and validation code remain.
- **Reduce ahm to a tasks-and-ADRs records CLI.** Two families, no procedure
  channel, no delegation, no advisory commands, one idempotent installer.
- **Retire ahm and use per-project scripts.** Treat record management as a
  project concern like the rest of its documentation.

## Decision Outcome

Chosen option: **reduce ahm to a tasks-and-ADRs records CLI**, because tasks
and ADRs are the two families whose lifecycle and integrity semantics are
mechanical and worth owning, and because every other surface ahm currently
carries is either prompt content, delegation to a foreign CLI, or a record
family whose only ahm-managed behavior is being listed.

### The decision in detail

1. **Two record families.** Tasks and ADRs only. Research and ExecPlans cease
   to be record families: their directories, templates, index generation,
   prime sections, and validators are removed. Existing `.ahm/research/` and
   `.ahm/exec-plans/` content is left on disk untouched and becomes ordinary
   project files that ahm neither reads, indexes, nor validates. The task
   front-matter field `exec_plan` is retired from the schema; because parsers
   preserve unknown fields, existing values survive round-trips as inert
   data.
2. **No procedure channel.** `ahm context` is removed, including
   `context task`, `context adr`, and the routing block prime currently
   prints. Procedure lives in project-owned `AGENTS.md` and `docs/`. This
   deliberately reverses the thesis in `docs/VISION.md` that every feature
   replaces a static artifact with a command; ahm accepts prose drift in
   exchange for not shipping opinions about working.
3. **No delegation.** `ahm task work` is removed together with its agent
   definitions, argument builders, JSONL/session parsers, session capture and
   resume, review and commit handoff, golden transcripts, fixture-capture
   script, and provider smoke tests. The External Agent Orchestration guardrail
   is deleted.
4. **No advisory commands.** `ahm audit` and `ahm task groom` are removed.
   Both were delegations and shared the machinery above; advice about a
   backlog is a prompt, not a record operation.
5. **No onboarding command.** `ahm onboard` is removed. Bootstrap is README
   prose in the consuming project.
6. **`ahm prime` becomes pure state.** It regenerates indexes, runs
   validation, and prints findings and record counts. The backlog routing,
   "Managed Work Intake", grooming, and audit hints are removed with the
   commands they advertised.
7. **`ahm status` and `ahm doctor` are retained as diagnostics**, with
   validation trimmed to task and ADR integrity: task front matter, buckets,
   duplicate IDs, dependencies, blocked and tracking consistency, acceptance
   checks, ADR format and status, supersession references, generated-index
   staleness, and relative links inside tasks and ADRs.
8. **Generated indexes are retained**, in the task buckets and
   `docs/adr/index.md`, because they are deterministic derived data that make
   records browsable without ahm. Their formats remain compatible.
9. **Install collapses to one idempotent command.** `ahm init` creates
   ahm-owned state when absent and reconciles it when present: configuration,
   the managed `.ahm/.gitignore`, indexes, and the metadata version. `ahm
   upgrade` is removed. The legacy `.agents/ahm.json` layout and the records,
   task, and ADR migrations are removed; root detection accepts only `.git`
   and `.ahm/config.json`. `AGENTS.md` remains project-owned and is never
   created, replaced, or removed by init, force flags, or any other path.
10. **Configuration shrinks.** `taskWork` is removed. Retained keys cover the
    metadata version, acceptance behavior, and managed-file hashes.
11. **Record mechanics are unchanged.** Atomic writes, lock files, ID
    allocation under a lock, cancellation reasons, acceptance checks,
    dependency and parent/child relations, comments, `--json`/`--text`/
    `--plain` output modes, exit codes, and the git-environment boundary
    (ADR 018) all continue to behave as they do today.

### Compatibility and Upgrade Treatment

This is an intentional breaking CLI, configuration, and record-family change
in a major release. No removed command keeps an alias and no removed
configuration key keeps a behavior: aliases would preserve exactly the
boundary this decision rejects.

The release implementing this decision must:

- identify the removals as breaking changes in release notes and the
  changelog;
- list the removed commands (`audit`, `context`, `onboard`, `task groom`,
  `task work`, `upgrade`) so consumers can delete hooks, CI steps, and
  instruction references that invoke them;
- instruct consumers to run `ahm init` with the new binary, which reconciles
  ahm-owned configuration and removes the obsolete `taskWork` field while
  preserving unrelated unknown metadata;
- leave `.ahm/research/`, `.ahm/exec-plans/`, and any generated index files
  inside them in place, neither validating nor deleting them, and document
  that removing them is the project's choice;
- state that a repository cannot move to this release from the legacy
  `.agents/ahm.json` layout without first running the final v1 release's
  upgrade; and
- regenerate the managed `.ahm/.gitignore` so it no longer ignores indexes for
  removed families.

### Consequences

- Good, because ahm's responsibilities match what it does mechanically and
  the boundary needs no prompt content to explain it.
- Good, because the highest-churn and highest-risk surface disappears: no
  external CLI arguments, output formats, sessions, provider smoke tests, or
  untrusted-input handling to keep current.
- Good, because ahm stops competing with the project's own instructions;
  procedure and routing become project-owned content that a project can
  change without waiting for a release.
- Good, because the test suite loses roughly 4,000 lines and its
  network-dependent cases, and CI no longer needs provider credentials.
- Bad, because projects that relied on `ahm context` for a working procedure
  must write and maintain that procedure themselves.
- Bad, because research notes and ExecPlans lose their managed index and
  integrity checks; broken links inside them go unreported.
- Bad, because consumers on the legacy layout must perform one upgrade step
  with the previous release before adopting this one.
- Neutral, because historical research, ExecPlans, and ADRs remain on disk and
  in git history; supersession metadata, rather than rewriting history,
  records the boundary change.

## More Information

- Product direction: `docs/VISION.md`, which this decision requires rewriting
  to the two-family boundary.
- Where the salvaged procedure lives after the change:
  `docs/workflow/tasks.md`, `docs/workflow/exec-plans.md`, and
  `docs/workflow/adrs.md`, with design plans under `docs/exec-plans/`.
  Delivery sequence: ExecPlan `.ahm/exec-plans/active/264-reduce-ahm-to-tasks-and-adrs.md`,
  tracker task 264, and children 264a-264g.
- Supersedes ADR-004, ADR-006, ADR-011, ADR-012, ADR-014, ADR-017, ADR-019,
  ADR-020, and ADR-021, each of which decided a surface removed here.
- Unaffected decisions that continue to guide the reduced tool: ADR-001
  (atomic writes), ADR-003 (task create body input), ADR-005 (acceptance
  checks), ADR-007 (cancellation reasons), ADR-009 (MADR ADRs), ADR-010 (ID
  allocation lock), ADR-015 (committed `.ahm` record storage), and ADR-018
  (git-environment scrubbing).
