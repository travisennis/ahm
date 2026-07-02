# Vision

Where `ahm` is going, and the tests future work should pass to keep it
moving there. Direction agreed 2026-07-02; this document frames individual
decisions recorded in ADRs, it does not replace them.

## What ahm is becoming

`ahm` started as an installer: it dropped workflow files, templates, and
skills into a consumer repository and kept them upgraded. Its direction is
to become the **runtime for agent work in a repository** — the system of
record for workflow state, the live channel for procedures, and the
mechanical enforcer of workflow and documentation health.

The reasoning: agent context is scarce and static files rot. Instruction
files installed into a repo drift from the binary that understands them;
workflow records committed to branches pollute history and pull requests
with ceremony; documentation rules enforced by diligence fail exactly when
nobody is looking. Every major feature underway replaces a static artifact
an agent might read with a command an agent runs — computed from live
state, versioned with the binary, and cheap to trigger from hooks.

## The four channels

1. **Bootstrap** — the one durable line in project-owned `AGENTS.md`:
   run `ahm prime` before work. Everything else is discoverable from
   there. (`ahm onboard` prints the snippet.)
2. **State** — `ahm prime`: sync and materialize ref-backed records,
   regenerate indexes, validate, and print the live briefing (warnings,
   backlog, managed-work routing). State-rich, instruction-light.
3. **Procedure** — `ahm context <scope>`: full instructions for a kind of
   managed work (task, plan, adr, research, docs, groom, improve,
   preflight). Emitted by the binary, never installed as files, not
   customizable per repo — project-specific guidance belongs in
   project-owned docs.
4. **Enforcement** — `ahm status`, `ahm doctor`, `ahm docs check`:
   mechanical validation of workflow state, environment, and project
   documentation health, designed to run unconditionally from pre-commit
   hooks, agent-harness hooks, and CI. `status` answers "is the workflow
   state healthy," `doctor` answers "is the environment sane,"
   `docs check` answers "is the project documentation surface healthy."

The pairing rule: `ahm context <topic>` says *how* to do work;
`ahm <topic> ...` commands *do or verify* it.

## What lives where

| Content | Home | Why |
| --- | --- | --- |
| Durable project docs, ADRs, accepted designs | committed project history | knowledge the project must keep |
| Tasks, scratch research, draft ExecPlans | gitignored `.agents/` files, synced via `refs/ahm/records` | working records: durable, private, out of branch history |
| Generated indexes | local-only, regenerated from records | derived data is never a source of truth |
| Procedures, templates, checks | the `ahm` binary | versioned with the tool that interprets them |
| Routing, operating loop, project rules | project-owned `AGENTS.md` and `docs/` | per-project judgment ahm must never overwrite |

Working records whose outcomes matter get promoted into project docs or
ADRs; the records themselves are ceremony and stay out of history.

## The git-safety boundary

Stated once, canonically. `ahm` may:

- read git state freely (status, diffs, refs);
- write workflow files under `.agents/`;
- write refs under `refs/ahm/*` and fetch/push that namespace to the
  configured remote, plus the minimal repo config to do so.

`ahm` never commits, stages, writes the index, moves `HEAD`, mutates
branches, creates pull requests, or patches project source. Delegation
(`ahm task work`) hands the repository to an external agent CLI that owns
its own git operations. Migration commands preview effects and print any
required user-run git commands rather than executing them.

Commands intended for hooks (`ahm prime` on session start, `ahm docs
check` on commit) must be fast, offline-tolerant — a failed sync degrades
to a warning, never a blocked session — and idempotent.

## Design tests for new work

A change fits this vision when:

- it prefers a command over an installed file;
- it keeps workflow ceremony out of branch history and durable knowledge
  in it;
- it renders text, `--plain`, and `--json` from one structure;
- it stays inside the git-safety boundary above;
- its enforcement can run from a hook without judgment calls — anything
  requiring judgment stays in agent skills or project docs;
- advisory output stays advisory: checks that say "may need review" never
  block, or agents learn to game them.

## Non-goals

- `ahm` does not implement code changes; it manages the work around them.
- `ahm` does not own `AGENTS.md` or project documentation content.
- No per-repo customization of binary-emitted procedures.
- Sync targets GitHub first; other remotes wait for real demand.

## Current work embodying this

- ADR 013 + tracker 138: ref-backed record storage (state channel).
- Tracker 156 (+ task 143): `ahm prime`, scoped procedure contexts,
  `ahm onboard`, skills no longer installed (bootstrap, state, and
  procedure channels).
- Tracker 160: `ahm docs check`, docMap, hook recipes (enforcement
  channel).

These retire, in one coordinated arc: installed managed skills, unscoped
`ahm context`, `ahm agents suggestions`, `--check project-docs`, and
committed-by-default workflow records. Consumers should experience this as
one migration with one upgrade guide, not five surprises.
