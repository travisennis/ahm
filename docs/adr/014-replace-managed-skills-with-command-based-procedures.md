---
status: accepted
date: 2026-07-07
decision-makers: Travis Ennis
---
# Replace managed skills with command-based procedures

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
command that reports live state and points them at the right scoped procedure,
not a long static `AGENTS.md` paste or installed skill files. ADR 013 reserves
`ahm prime` as that session-start hook for future ref-backed record sync. This
decision defines the initial read-only `ahm prime` form and the procedure
scopes that later work can build on without conflicting with ADR 013.

## Decision Drivers

- Keep `.agents/` project-owned after the ADR 013 `.ahm/` namespace decision.
- Remove stale copied procedure files and their upgrade conflict surface.
- Keep `AGENTS.md` project-owned and minimal, with only durable bootstrap and
  safety invariants.
- Give agents a canonical startup command that is state-rich and
  instruction-light.
- Keep full procedures versioned with the `ahm` binary and available on demand.
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
- **Expose procedures and session bootstrap through commands.** This removes
  installed procedure files, keeps the command output live and versioned with
  the binary, and makes `AGENTS.md` carry only the minimal instructions needed
  to run the right command.

## Decision Outcome

Chosen option: **expose procedures and session bootstrap through commands**,
because the binary owns the procedures, can compute live repository state, and
can keep the `.agents/` namespace project-owned.

`ahm` will stop installing and hash-managing these files:

- `.agents/skills/preflight/SKILL.md`
- `.agents/skills/grooming-backlog/SKILL.md`
- `.agents/skills/finding-improvements/SKILL.md`

Their procedure content will live only in the binary. There will be no per-repo
procedure override mechanism; project-specific agent instructions belong in
project-owned `AGENTS.md` and documentation, not in ahm-managed procedure
copies.

The procedures will be exposed as live, state-aware scoped contexts:

- `ahm context groom`
- `ahm context improve`
- `ahm context preflight`

The short scope names are deliberate. Scoped `ahm context <scope>` remains the
instruction and procedure channel for managed work.

`ahm prime` becomes the canonical agent session-start command. In this
decision, `ahm prime` is strictly read-only: it reports warnings, live
repository and backlog state, and the managed-work routing table. ADR 013
continues to own the later ref-backed records sync behavior. Task 143 will add
that sync/materialization step to the same command after the read-only command
exists; the sync step must keep ADR 013's fast, idempotent, offline-tolerant
hook contract.

Unscoped `ahm context` will become a usage error that points users to
`ahm prime` and lists valid scoped contexts. This is a breaking CLI change.
Scoped contexts remain valid.

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
- **Scoped contexts:** full instructions and procedures.
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
- Good, because JSON parity is required for the new command surfaces from the
  start.
- Bad, because this removes the existing skill files without a compatibility
  shim, so consumers that rely on those exact installed skill paths must update
  their agent instructions.
- Bad, because unscoped `ahm context` and `ahm agents suggestions` are breaking
  CLI removals and need clear release notes and documentation updates.
- Bad, because command-based procedures are less inspectable from a plain file
  tree than installed Markdown skills.

## More Information

- Resolves ADR 011's explicit deferral of the managed-skills decision.
- Coordinates with ADR 013 by defining the initial read-only `ahm prime`
  command; ADR 013 still owns the later records sync behavior layered onto the
  same command.
- Task 156 tracks implementation through child tasks 156b-156g.
- Task 137 recorded the `.ahm/` namespace decision that keeps `.agents/`
  project-owned and ahm-managed workflow state under `.ahm/`.

- Supersedes [ADR-002](002-advisory-agents-suggestions.md).
