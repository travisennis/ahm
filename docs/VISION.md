# Vision

Where `ahm` is going, and the tests future work should pass to keep it
moving there. Direction agreed 2026-07-02; this document frames individual
decisions recorded in ADRs, it does not replace them.

## What ahm is becoming

`ahm` started as an installer: it dropped workflow files, templates, and
skills into a consumer repository and kept them upgraded. Its direction is
to become the **runtime for structured agent work in a repository** — the
system of record for research, ADRs, ExecPlans, and tasks; the live channel
for their procedures; and the mechanical enforcer of their integrity.

The reasoning: agent context is scarce and static files rot. Instruction
files installed into a repo drift from the binary that understands them;
workflow records need lifecycle and integrity checks that prose alone cannot
provide. General project documentation has different structures and policies
in every repository, so its content and enforcement remain project-owned.
Every major feature underway replaces a static workflow artifact an agent
might read with a command an agent runs — computed from live state and
versioned with the binary.

## The four channels

1. **Bootstrap** — the one durable line in project-owned `AGENTS.md`:
   run `ahm prime` before work. Everything else is discoverable from
   there. (`ahm onboard` prints the snippet.)
2. **State** — `ahm prime`: regenerate indexes, validate workflow state,
   and print the live briefing (warnings, backlog, managed-work routing).
   State-rich, instruction-light.
3. **Procedure** — `ahm context <scope>`: full instructions for one of the
   four kinds of managed work (`task`, `plan`, `adr`, or `research`). Emitted
   by the binary, never installed as files, and not customizable per repo;
   project-specific guidance belongs in project-owned docs.
4. **Enforcement** — `ahm status` and `ahm doctor`: mechanical validation of
   workflow-record integrity and the environment, designed to run
   unconditionally from hooks and CI. `status` answers "is the workflow
   state healthy," and `doctor` answers "is the environment sane."

The pairing rule: `ahm context <record-family>` says *how* to manage that
structured work, and its commands create, change, or verify the records.

## What lives where

| Content | Home | Why |
| --- | --- | --- |
| ADRs | committed `docs/adr/` records | durable decisions with an ahm-managed lifecycle and index |
| General project docs and accepted designs | project-chosen committed paths | project-owned knowledge that managed work may reference or update |
| Tasks, scratch research, draft ExecPlans | committed files under tool-owned `.ahm/` | branch-scoped working records with ahm-managed lifecycle and integrity semantics |
| Generated indexes | local-only under `.ahm/`, regenerated from records | derived data is never a source of truth |
| ahm config | committed under `.ahm/` | settings must be identical on every clone and in CI |
| Structured-work procedures, templates, checks | the `ahm` binary | versioned with the tool that interprets them |
| Routing, operating loop, project rules | project-owned `AGENTS.md` and `docs/` | per-project judgment ahm must never overwrite |
| Agent-facing project content (skills, standing instructions) | committed `.agents/` | the ecosystem-standard directory agents read; ahm may read it, never manages it |

The namespace rule behind the table: `.agents/` is for agents to read
and the project to own; `.ahm/` is for ahm to manage. `.ahm/` carries a
managed internal `.gitignore` (generated indexes ignored, source records
and config not), so the consumer's root `.gitignore` is never touched.
Decided 2026-07-02; recorded formally in ADR 015 (task 172).

Working records whose outcomes matter may produce or update project docs or
ADRs. Ahm manages the structured records, while each project owns the form and
policy of its general documentation.

## The git-safety boundary

Stated once, canonically. `ahm` may:

- read git state freely (status, diffs, refs);
- write workflow files under its own `.ahm/` directory (and, during
  explicit opt-in migration only, move files out of `.agents/`).

`ahm` never commits, stages, writes the index, moves `HEAD`, mutates
branches, creates pull requests, or patches project source. Delegation
(`ahm task work`) hands the repository to an external agent CLI that owns
its own git operations. Migration commands preview effects and print any
required user-run git commands rather than executing them.

Commands intended for hooks, including `ahm prime`, `ahm status`, and
`ahm doctor`, must be fast, offline-tolerant, and idempotent.

## Design tests for new work

A change fits this vision when:

- it prefers a command over an installed file;
- it keeps structured workflow source records under ahm ownership and derived
  indexes out of branch history;
- it renders text, `--plain`, and `--json` from one structure;
- it stays inside the git-safety boundary above;
- its enforcement protects research, ADR, ExecPlan, or task integrity without
  expanding into general project-documentation governance;
- project-specific documentation structure, content, and validation remain in
  project-owned instructions and tooling.

## Non-goals

- `ahm` does not implement code changes; it manages the work around them.
- `ahm` does not own `AGENTS.md` or project documentation content.
- `ahm` does not prescribe or validate the general project-documentation
  surface.
- No per-repo customization of binary-emitted procedures.

## Current work embodying this

This section should reference the active task arc. Update it when the focus
shifts. For current active work, run `ahm task list --status active` or
`ahm context task`.
