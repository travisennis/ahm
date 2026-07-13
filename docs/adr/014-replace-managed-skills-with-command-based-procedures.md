---
status: accepted
date: 2026-07-07
decision-makers: Travis Ennis
---
# Replace managed skills with command-based procedures

## Revision Note

Revised on 2026-07-13 to clarify the ownership handoff for existing copies of
the three former managed skills. Ahm leaves those files in place, discards
their old managed-file hashes during init, upgrade, or records migration, and
never inspects, reports, overwrites, or removes them. This replaces the
initial implementation's upgrade-time cleanup behavior without changing the
binary-owned command procedures described below.

Revised on 2026-07-07, before implementation of the procedure surfaces began.
As first accepted, this ADR exposed the three skill procedures as live scoped
contexts (`ahm context groom`, `ahm context improve`, `ahm context preflight`).
A follow-up design discussion the same day replaced that channel with
delegation commands (`ahm task groom`, `ahm audit`), folded the preflight
procedure into the `ahm task work` review handoff, and recorded the autonomy
model that motivates the change. The sections below describe the revised
decision; the scoped-context form is retained under Considered Options.

## Context and Problem Statement

ADR 011 moved managed-work reference documents out of consumer repositories and
into scoped `ahm context` output, but it deliberately kept managed agent skills
installed until a separate decision replaced that mechanism. That deferral now
creates the same drift problem ADR 011 removed for copied workflow guides:
procedure content is versioned by the binary that understands it, but consumer
repositories still receive static `.agents/skills/*/SKILL.md` files that can go
stale, conflict during upgrade, and blur the boundary between project-owned
agent content and ahm-owned workflow machinery.

ADR 013 and the task 137 revision also settled the namespace direction:
`.agents/` is project-owned agent-facing content that `ahm` may read but should
not manage, while `.ahm/` is the tool-owned home for ahm-managed workflow state
after migration. Continuing to install ahm-owned skills under `.agents/` works
against that boundary.

At the same time, the command model is changing. Agents need one session-start
command that reports live state and points them at the right managed-work
entry point, not a long static `AGENTS.md` paste or installed skill files.
ADR 013 reserves `ahm prime` as that session-start hook for future ref-backed
record sync. This decision defines the initial read-only `ahm prime` form and
the procedure surfaces that later work can build on without conflicting with
ADR 013.

Finally, the three skill procedures are imperatives over ahm-managed records,
not reference material. ADR 011's scoped contexts are a reference channel: the
agent reads instructions and then performs the record writes itself, which
leaves ahm's invariants (label vocabulary, ID allocation, atomic writes, index
regeneration) to the discipline of whichever agent read the prompt. `ahm task
work` (ADR 006) already established the alternative boundary: `ahm` composes
the prompt, delegates judgment to an external agent CLI, and keeps
deterministic workflow behavior on its own side. The grooming and audit
procedures, whose entire output is changes to ahm-managed records, fit that
boundary better than the reference channel.

## Decision Drivers

- Keep `.agents/` project-owned after the ADR 013 `.ahm/` namespace decision.
- Remove stale copied procedure files and their upgrade conflict surface.
- Keep `AGENTS.md` project-owned and minimal, with only durable bootstrap and
  safety invariants.
- Give agents a canonical startup command that is state-rich and
  instruction-light.
- Keep full procedures versioned with the `ahm` binary.
- Keep scoped `ahm context` a reference channel; procedures are imperatives
  and belong on the command surface as verbs.
- Treat agent output as data: apply results through ahm's own record
  machinery so workflow invariants hold regardless of which agent ran.
- Assume delegated agents grow more capable over time; prefer better
  procedure instructions, observability, and reversibility over interactive
  gates and per-repo procedure customization.
- Preserve explicit JSON parity for new command outputs.
- Coordinate the read-only `ahm prime` briefing with the future ref-backed
  records sync step reserved by ADR 013.

## Considered Options

- **Keep installing managed skills as-is.** This preserves existing agent
  behavior, but continues copying canonical procedure text into consumer
  repositories, keeps `.agents/skills/` as an ahm-managed surface under a
  project-owned namespace, and leaves upgrade conflicts for files whose content
  should be binary-owned.
- **Install thin shim skills that point at ahm commands.** This gives existing
  skill-aware agents a transition path, but still leaves ahm-managed files under
  `.agents/`, adds another copied artifact that can drift, and makes the real
  source of truth indirect.
- **Expose procedures as live scoped contexts.** `ahm context groom`,
  `ahm context improve`, and `ahm context preflight` would emit the procedure
  text interleaved with live repository state for whatever agent is already in
  session. This was the option originally accepted by this ADR. It removes the
  installed files and keeps content binary-versioned, but it stretches the
  ADR 011 reference channel into an imperative channel, and it leaves applying
  the procedure's results to the reading agent's judgment.
- **Delegate procedures to external agent CLIs and apply structured results
  mechanically.** `ahm` composes the prompt (procedure plus live state), runs
  the configured coding-agent CLI, requires a schema-constrained JSON result,
  and applies that result through its own task machinery.

## Decision Outcome

Chosen option: **delegate procedures to external agent CLIs and apply
structured results mechanically**, because the binary owns the procedures, can
compute live repository state, keeps the `.agents/` namespace project-owned,
and — unlike the scoped-context form — keeps every resulting record write
inside ahm's own appliers.

`ahm` will stop installing and hash-managing these files:

- `.agents/skills/preflight/SKILL.md`
- `.agents/skills/grooming-backlog/SKILL.md`
- `.agents/skills/finding-improvements/SKILL.md`

Their procedure content will live only in the binary. There will be no per-repo
procedure override mechanism; project-specific agent instructions belong in
project-owned `AGENTS.md` and documentation, not in ahm-managed procedure
copies.

### Delegation commands

Two delegation commands replace the grooming and audit procedures, modeled on
the `ahm task work` orchestration (ADR 006): agent selection via
`default_work_agent` and `--agent`, execution from the repository root, and
session capture. The global `--dry-run` flag prints the composed prompt and
performs no delegation and no writes.

- **`ahm task groom [<id>]`** delegates backlog triage. Without an argument it
  covers all `Open` tasks plus a staleness review of `Blocked` tasks; with an
  id it grooms that task only. The delegated agent returns a per-task verdict
  and `ahm` applies it. Write authority granted to groom results: accept a
  task (`Open` to `Pending`) when the verdict states nothing prevents it from
  being worked; append a timestamped comment stating what the task still
  needs; correct dependencies; normalize labels. Cancellation is never
  performed autonomously — the agent may only recommend it in a comment.
- **`ahm audit`** delegates the codebase-improvement audit (the
  finding-improvements procedure). The agent surveys read-only and returns
  findings; `ahm` creates one task per finding through the normal task
  creation path with status `Open` and a `source:audit` provenance label.
  There is no interactive acceptance gate: `Open` is the acceptance gate the
  task lifecycle already defines, and grooming later accepts or cancels the
  proposals (cancellation requires a reason, preserving the audit trail).

Prompts are composed in Go from the embedded procedure text plus live state
(backlog lines, label vocabulary, validation findings) using the same internal
listing functions the task commands use, never by shelling out to `ahm`.

### Structured results

Delegated procedure runs must produce a schema-constrained JSON final answer.
The schema is supplied per agent through the CLI's native mechanism where one
exists (`--json-schema` for `claude`, `--output-schema` for `codex exec` and
`cake`), and embedded in the prompt for agents without one. `ahm` validates
the result against the schema before applying it regardless of transport. On
invalid or missing output, `ahm` writes nothing, preserves the raw output for
inspection, and exits nonzero.

### Preflight

Preflight stops being a public procedure surface entirely — no installed
skill, no scoped context, no standalone command. Its content becomes the
binary-embedded review procedure used by the `ahm task work` review handoff,
which today invokes the installed skill by name and would break when the
skills are removed. The review prompt also gains the managed-work completion
checklist (acceptance notes checked off, ExecPlan updated, docs touched, no
hand-edited indexes). A pre-commit review methodology is not a
workflow-record procedure; it is internal review machinery for the handoff
`ahm` already owns (ADR 006, ADR 008).

### Autonomy model

This revision makes a deliberate autonomy decision. With audit and grooming
both delegated, the pipeline can run end-to-end without a human decision:
`ahm audit` proposes tasks, `ahm task groom` accepts them, and `ahm task work`
implements them. The remaining human touchpoints are priority setting, choice
of which task to work, the task-work review and commit handoff, and
cancellation, which is never autonomous.

The safety posture is observability and reversibility rather than interactive
gates: every delegated write flows through ahm's appliers into workflow
records only — never project source, git branches, or `HEAD`, per ahm's
standing guarantee; audit-created tasks carry a provenance label so they can
be queried and mass-cancelled; sessions are captured; cancellations require
reasons. When delegated output is poor, the intended response is improving the
procedure instructions, for which cancelled-with-reason audit tasks and groom
comments are the feedback data.

### Retained decisions

`ahm prime` becomes the canonical agent session-start command. In this
decision, `ahm prime` is strictly read-only: it reports warnings, live
repository and backlog state, and the managed-work routing table, which points
at the delegation commands above. ADR 013 continues to own the later
ref-backed records sync behavior. Task 143 will add that sync/materialization
step to the same command after the read-only command exists; the sync step
must keep ADR 013's fast, idempotent, offline-tolerant hook contract.

Unscoped `ahm context` will become a usage error that points users to
`ahm prime` and lists valid scoped contexts. This is a breaking CLI change.
Scoped reference contexts (task, plan, adr, research, docs) remain valid; no
procedure scopes are added.

`ahm agents suggestions` and the `ahm agents` command group will be removed,
including `--all`. They will be replaced by `ahm onboard`, which prints a
minimal paste-ready `AGENTS.md` snippet. This supersedes ADR 002's advisory
suggestions mechanism. The new snippet carries only:

- the bootstrap invariant: always run `ahm prime` before starting work and
  re-run it after context compaction;
- safety invariants: never hand-edit generated indexes, use `ahm task` for
  task state changes, and use `ahm adr` for ADR lifecycle changes.

Long-form routing and workflow intake guidance moves out of the pasted snippet.
The content architecture is:

- **Pasted snippet:** static, project-owned bootstrap and safety invariants
  only.
- **`ahm prime`:** dynamic, binary-versioned live state plus the managed-work
  intake routing table.
- **Scoped contexts:** managed-work reference instructions.
- **Delegation commands:** procedures over ahm-managed records, executed by a
  delegated agent and applied mechanically.
- **Project-owned guidance:** operating-loop and workflow-routing policy stays
  in `AGENTS.md` and project docs; `ahm` does not emit it.

`ahm doctor` will add a low-severity check for drift in the static bootstrap
layer: when root `AGENTS.md` exists but does not reference `ahm prime`, it
should suggest running `ahm onboard`. Absence of `AGENTS.md` is not a finding.

All new outputs introduced by this decision must support text, `--plain`, and
`--json` from the same structured Go data. Text rendering must not scrape or
template-execute another subcommand's human output.

Migration will use the existing obsolete-managed-files mechanism. `ahm upgrade`
will remove previously managed skill files whose content still matches their
recorded managed hashes, and report locally edited skill files as conflicts
unless `--force` is used. Fresh `ahm init` will not create `.agents/skills/`.

Tracker 156 does not need a separate ExecPlan before implementation because
this ADR is the durable design record and the tracker is split into scoped
child tasks with explicit dependencies and acceptance notes. If a child task
expands to L/XL effort or crosses a new architectural boundary, that child
should add an ExecPlan before implementation.

### Consequences

- Good, because procedure content updates with the installed `ahm` binary
  instead of copied files in every consumer repository.
- Good, because `.agents/` becomes cleaner project-owned agent content after
  the ADR 013 namespace decision.
- Good, because agents get one startup command for live state and routing.
- Good, because static `AGENTS.md` integration becomes small enough to review
  and maintain manually.
- Good, because workflow invariants are enforced mechanically by ahm's
  appliers no matter which agent executed the procedure.
- Good, because procedures become runnable from a human shell or automation
  without an agent already in session.
- Good, because JSON parity is required for the new command surfaces from the
  start.
- Bad, because this removes the existing skill files without a compatibility
  shim, so consumers that rely on those exact installed skill paths must update
  their agent instructions.
- Bad, because unscoped `ahm context` and `ahm agents suggestions` are breaking
  CLI removals and need clear release notes and documentation updates.
- Bad, because the commands depend on external agent CLIs honoring
  schema-constrained output; contract changes there surface as orchestration
  failures and grow the golden-transcript and smoke-test surface.
- Bad, because delegated runs cost real agent time and tokens; grooming a
  large backlog is a long batch run.
- Bad, because an agent already in session cannot execute the procedure
  inline; it must either invoke the delegation command (a nested agent run) or
  use `--dry-run` to obtain the prompt and apply results through ordinary
  `ahm task` commands.
- Bad, because command-based procedures are less inspectable from a plain file
  tree than installed Markdown skills.

## More Information

- ADR 017 partially supersedes this decision's grooming write-authority
  boundary by allowing ahm to apply validated, section-level task revisions;
  the delegation, schema, cancellation, and workflow-record safety decisions
  here remain accepted.
- Resolves ADR 011's explicit deferral of the managed-skills decision.
- Coordinates with ADR 013 by defining the initial read-only `ahm prime`
  command; ADR 013 still owns the later records sync behavior layered onto the
  same command.
- Reuses the delegation boundary of ADR 006 and ADR 008; the change to the
  task-work review prompt (embedded preflight procedure replacing the
  installed-skill invocation) is recorded here.
- Task 156 tracks implementation through child tasks 156b-156g; tasks
  156c-156e were re-specified on 2026-07-07 to match this revision.
- Task 137 recorded the `.ahm/` namespace decision that keeps `.agents/`
  project-owned and ahm-managed workflow state under `.ahm/`.

- Supersedes [ADR-002](002-advisory-agents-suggestions.md).
