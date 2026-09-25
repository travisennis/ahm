# Vision

Where `ahm` is going, and the tests future work should pass to keep it
moving there. Direction agreed 2026-07-02; this document frames individual
decisions recorded in ADRs, it does not replace them.

## What ahm is becoming

`ahm` started as an installer: it dropped workflow files, templates, and
skills into a consumer repository and kept them upgraded. Its direction is
to become the **records CLI for structured agent work in a repository** — the
system of record for tasks and ADRs, and the mechanical enforcer of their
integrity. It no longer stores research notes or ExecPlans, and it no longer
carries opinions about how a project should work.

The reasoning: agent context is scarce and static files rot, so the records a
project needs mechanical guarantees about — identity, lifecycle, indexes,
integrity — belong in a tool, while the prose that tells a project how to work
belongs to the project, which can change it without waiting for a release.
General project documentation has different structures and policies in every
repository, so its content and enforcement remain project-owned.

## The channels

1. **Bootstrap** — the one durable line in project-owned `AGENTS.md`:
   run `ahm prime` before work. Everything else is discoverable from there.
2. **State** — `ahm prime`: regenerate indexes, validate workflow state, and
   print the live briefing (warnings, record counts, and the backlog). Pure
   state, no instructions.
3. **Enforcement** — `ahm status` and `ahm doctor`: mechanical validation of
   workflow-record integrity and the environment, designed to run
   unconditionally from hooks and CI. `status` answers "is the workflow
   state healthy," and `doctor` answers "is the environment sane."

Procedure is not a channel. Task, ADR, and planning practice lives in
project-owned prose under `docs/` and `AGENTS.md`, because the judgment it
encodes is the project's; ahm accepts that prose can drift rather than
shipping opinions about working. ADR 022 records this reversal.

## What lives where

| Content | Home | Why |
| --- | --- | --- |
| ADRs | committed `docs/adr/` records | durable decisions with an ahm-managed lifecycle and index |
| General project docs and accepted designs | project-chosen committed paths | project-owned knowledge that managed work may reference or update |
| Tasks | project mode: committed files under tool-owned `.ahm/`; home mode: machine-local files under the user-level store | the selected `tasks_location` mode determines whether records are branch-scoped or shared across branches; clones share them when they resolve to the same project key; ahm manages the lifecycle and integrity semantics |
| Task indexes | local-only under the resolved task records root, regenerated from records | derived data is never a source of truth |
| ADR index | committed `docs/adr/index.md`, regenerated from ADR records | derived data that remains durable project documentation |
| ahm config | committed under `.ahm/` | settings must be identical on every clone and in CI |
| Structured-work procedures and checks | project-owned `docs/workflow/`, `docs/exec-plans/`, and `AGENTS.md` | per-project judgment ahm no longer ships, and may drift |
| Routing, operating loop, project rules | project-owned `AGENTS.md` and `docs/` | per-project judgment ahm must never overwrite |
| Agent-facing project content (skills, standing instructions) | committed `.agents/` | the ecosystem-standard directory agents read; ahm may read it, never manages it |

The namespace rule behind the table: `.agents/` is for agents to read
and the project to own; `.ahm/` is for ahm to manage. In project mode `.ahm/`
carries committed source records, configuration, and a managed internal
`.gitignore` that ignores generated indexes and machine-local state. In home
mode the committed `.ahm/` holds configuration and the project gitignore, while
records, generated indexes, the lock, and store state live in the store's
per-project directory. The consumer's root `.gitignore` is never touched.
Decided 2026-07-02; recorded for project mode in ADR 015 (task 172) and
extended to the user-level home store by ADR 023.

Working records whose outcomes matter may produce or update project docs or
ADRs. Ahm manages the structured records, while each project owns the form and
policy of its general documentation.

## The git-safety boundary

Stated once, canonically. `ahm` may:

- read git state freely (status, diffs, refs);
- write workflow state under the resolved records root and the project-owned
  paths ahm owns.

`ahm` never commits, stages, writes the index, moves `HEAD`, mutates
branches, creates pull requests, or patches project source. It prints any
git command a person must run rather than executing it.

Commands intended for hooks, including `ahm prime`, `ahm status`, and
`ahm doctor`, must be fast, offline-tolerant, and idempotent.

## Design tests for new work

A change fits this vision when:

- it keeps structured workflow source records under ahm ownership and derived
  indexes out of branch history;
- it renders text, `--plain`, and `--json` from one structure;
- it stays inside the git-safety boundary above;
- its enforcement protects task or ADR integrity without expanding into
  general project-documentation governance;
- project-specific documentation structure, content, and validation remain in
  project-owned instructions and tooling.

## Non-goals

- `ahm` does not implement code changes; it manages the work around them.
- `ahm` does not own `AGENTS.md` or project documentation content.
- `ahm` does not prescribe or validate the general project-documentation
  surface.
- No per-repo customization of binary-emitted procedures, because the binary
  emits none.

## Current work embodying this

This section should reference the active task arc. Update it when the focus
shifts. For current active work, run `ahm task list` or read
[the task workflow](workflow/tasks.md).
