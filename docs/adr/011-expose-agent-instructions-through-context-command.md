---
status: accepted
date: 2026-06-19
---
# Expose agent instructions through context command

## Context and Problem Statement

`ahm init` historically installed agent-facing instruction templates into every
consumer repository: `.agents/TASKS.md`, `.agents/PLANS.md`,
`.agents/RESEARCH.md`, `.agents/DOCS.md`, `.agents/.tasks/README.md`,
`.agents/.research/README.md`, `docs/adr/README.md`, and a create-only
starter `AGENTS.md`. The same template registry also installed agent skills
under `.agents/skills/`.

Those files made the workflow discoverable through normal file reading, but
they also copied canonical `ahm` behavior into target repositories. Every
template change created upgrade conflict handling, stale local guidance, and
extra files that were not project data.

Agents need current workflow instructions plus live repository state. Static
installed Markdown can provide the instructions, but it cannot summarize the
current task queue, validation state, or dirty worktree.

## Decision Drivers

- Keep target repositories focused on workflow state and project records, not
  copied `ahm` instruction prose.
- Give coding agents a single session-start command that returns canonical
  guidance and dynamic repository context.
- Remove upgrade conflicts caused by locally edited instruction templates.
- Keep agent skills installed and managed until there is an explicit decision
  to replace that mechanism.
- Preserve `AGENTS.md` as project-owned and never overwrite or delete it.
- Keep generated indexes and workflow source records repo-local and auditable.

## Considered Options

- Continue installing instruction templates into consumer repositories.
- Stop installing workflow guide templates and expose canonical guidance
  through `ahm context`, while continuing to install managed skill templates.
- Keep installing only a minimal starter `AGENTS.md` bridge.

## Decision Outcome

Chosen option: stop installing workflow guide templates and expose canonical
guidance through `ahm context`, while continuing to install managed skill
templates, because the binary already owns workflow behavior and can combine
that guidance with live repository state.

`ahm init` creates workflow directories, managed skill templates, metadata, and
generated indexes, but does not create starter `AGENTS.md` or copied workflow
guide files.

`ahm upgrade` removes previously managed workflow guide templates when metadata
shows their on-disk content still matches the recorded managed hash. Locally
modified workflow guide templates are preserved and reported as conflicts
unless `--force` is used. Managed skill templates continue to be updated from
embedded templates. Existing `AGENTS.md` remains project-owned and is never
overwritten or removed by `init`, `upgrade`, or `--force`.

### Consequences

- Good, because consumer repositories contain fewer ahm-owned files and fewer
  upgrade conflicts.
- Good, because agents can run `ahm context` to receive current instructions,
  validation state, task queue summaries, and git worktree state in one place.
- Good, because canonical agent guidance updates with the installed `ahm`
  binary instead of depending on copied Markdown being upgraded cleanly.
- Good, because existing skill-based agent workflows continue to work.
- Bad, because first-contact discoverability depends more on project
  `AGENTS.md`, user instruction, or prior agent knowledge of `ahm context`.
- Bad, because reviewing canonical workflow instructions now means reading
  `ahm` source/docs or command output rather than a copied file in every
  consumer repository.

## More Information

- Supersedes the `ahm init` starter-file portion of ADR 002. ADR 014 later
  supersedes the remaining `ahm agents suggestions` mechanism with
  `ahm onboard`.
- ADR 014 resolves this ADR's explicit deferral of the managed-skills
  decision by replacing installed skill templates with command-based
  procedure scopes.
- Task 115: Add session context command.

## Refinement (2026-06-20): Context Role Split

Task 117 narrowed the role of `ahm context` to align with the primitives model:

- Unscoped `ahm context` is a live repository briefing. It no longer prints
  an `## Instructions` section or claims workflow authority. Project
  `AGENTS.md` owns workflow routing and implementation decisions.
- Scoped `ahm context task|plan|adr|research|docs` provides managed-work
  references for ahm-managed artifacts. Scoped JSON (`ahm --json context task`)
  now returns only `scope`, `instructions`, and `commands`, without the live
  briefing fields (`root`, `workflow`, `git`, `tasks`).
- The `ahm owned-files` suggestion block now describes scoped `ahm context` as
  the managed-work reference and `AGENTS.md` as the owner of routing.
- `ahm task show <id>` is the normal task inspection primitive; opening the
  task file directly is only for fallback or manual edits.

This refinement does not reverse ADR 011's core decision to stop installing
workflow guide templates into consumer repositories. Scoped context commands
continue to expose the full embedded reference documents on demand.
