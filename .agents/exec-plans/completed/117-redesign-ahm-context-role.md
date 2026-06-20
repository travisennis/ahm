# Redesign `ahm context` Around Briefing And Managed-Work References

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `ahm context plan` guidance.

## Purpose / Big Picture

After this change, users and agents can treat `ahm` as a set of workflow primitives instead of a competing source of project workflow authority. Project `AGENTS.md` files define the operating loop and implementation routing. Unscoped `ahm context` reports live repository state. Scoped `ahm context task|plan|adr|research|docs` prints reference material for ahm-managed artifacts. The behavior is observable by running `ahm context`, `ahm --json context`, `ahm context task`, and `ahm --json context task` and seeing that the outputs have distinct roles.

## Progress

- [x] (2026-06-20 18:38 -04:00) Interviewed Travis Ennis and resolved the product/design decisions captured in task 117.
- [x] Update implementation and focused tests for the new text and JSON contracts.
- [x] Update embedded suggestions, workflow references, managed skills, and this repository's `AGENTS.md`.
- [x] Update durable docs and ADR 011.
- [x] Run focused tests, then full project verification as required by `CONTRIBUTING.md`.
- [x] Fill task 117 Acceptance Notes and complete the task when the implementation and docs are verified.

## Surprises & Discoveries

- Observation: Current text output hides warnings-only validation state. In this repository, `ahm context` prints `validation: ok` while `ahm --json context` reports warnings.
  Evidence: On 2026-06-20, `ahm --json context` reported nine warnings, including completed tasks that reference an active ExecPlan.
- Observation: Scoped text and scoped JSON currently disagree. Scoped text prints only the embedded reference document, while scoped JSON includes live workflow, git, and task briefing fields.
  Evidence: `ahm context task` prints `# Task Workflow` without the briefing wrapper; `ahm --json context task` includes `workflow`, `git`, `tasks`, `instructions`, and `commands`.
- Observation: `ahm agents suggestions` already mostly frames `ahm` as managed-work intake, but the ownership block still says to treat `ahm context` output as canonical workflow guidance.
  Evidence: `internal/templates/templates.go` contains the sentence `Treat ahm context output as the canonical workflow guidance.`

## Decision Log

- Decision: Unscoped `ahm context` is a live briefing primitive, not workflow authority.
  Rationale: Project-owned `AGENTS.md` should define workflow routing from the primitives `ahm` provides.
  Date/Author: 2026-06-20 / Travis Ennis and Codex.
- Decision: Remove the unscoped `## Instructions` section.
  Rationale: Any prose instruction block in unscoped output competes with `AGENTS.md` and recreates the ambiguity this task fixes.
  Date/Author: 2026-06-20 / Travis Ennis and Codex.
- Decision: Keep scoped text output pure: it prints the relevant embedded reference document with no live briefing wrapper.
  Rationale: The scoped commands should be narrow references. Users can run unscoped `ahm context` separately when they need live state.
  Date/Author: 2026-06-20 / Travis Ennis and Codex.
- Decision: Make scoped JSON semantically match scoped text by returning scope, instructions, and optional scoped commands, but no workflow, git, or task briefing fields.
  Rationale: Text and JSON should expose the same command concept. The current scoped JSON shape is muddy.
  Date/Author: 2026-06-20 / Travis Ennis and Codex.
- Decision: Allow a deliberate JSON compatibility change and document it.
  Rationale: Removing unscoped `instructions` and changing scoped JSON are CLI compatibility changes; hiding that would be dishonest.
  Date/Author: 2026-06-20 / Travis Ennis and Codex.
- Decision: Replace broad "canonical workflow guidance" wording with "managed-work reference" or equivalent narrow language.
  Rationale: The scoped references are authoritative for ahm-managed artifacts, not for the project's implementation workflow.
  Date/Author: 2026-06-20 / Travis Ennis and Codex.
- Decision: Update ADR 011 with a refinement note instead of superseding it.
  Rationale: ADR 011 remains correct about removing copied workflow guide files. This task narrows the role of unscoped `ahm context` rather than reversing that decision.
  Date/Author: 2026-06-20 / Travis Ennis and Codex.
- Decision: Treat `ahm task show <id>` as the normal task inspection primitive.
  Rationale: `ahm task show <id>` prints the task file contents, so requiring agents to also open the task file is redundant and teaches the wrong primitive.
  Date/Author: 2026-06-20 / Travis Ennis and Codex.
- Decision: `validation: ok` in context text means zero errors and zero warnings.
  Rationale: A briefing that hides warnings is misleading. Full details still belong in `ahm doctor`, so context should cap findings at five.
  Date/Author: 2026-06-20 / Travis Ennis and Codex.
- Decision: Keep task 117 as one task, raise it to effort `L`, and require this ExecPlan.
  Rationale: The work crosses CLI output, JSON shape, docs, templates, managed skills, AGENTS.md, and ADR 011. It is cohesive but too broad for `M`.
  Date/Author: 2026-06-20 / Travis Ennis and Codex.

## Outcomes & Retrospective

Implementation complete. Summary of changes:

### Command behavior

- **Unscoped `ahm context` text**: Prints live briefing (root, workflow, validation, git, tasks, useful commands). No `## Instructions` section. Validation text distinguishes warnings-only from clean state.
- **Unscoped `ahm context` JSON**: Contains `root`, `workflow`, `git`, `tasks`, `commands`. No `instructions` field.
- **Scoped `ahm context task|...` text**: Pure managed-work reference document. No briefing wrapper.
- **Scoped `ahm context task|...` JSON**: Contains `scope`, `instructions`, `commands`. No `root`, `workflow`, `git`, `tasks`.
- **Warnings-only display**: Shows warning count with `run ahm doctor` instead of `validation: ok`. Findings still capped at 5.

### Files changed

- `internal/ahm/context.go` — Core logic: separate unscoped/scoped report shapes, removed instructions from unscoped, fixed validation display.
- `internal/ahm/context_test.go` — Updated tests for new contracts, added scoped JSON test.
- `internal/ahm/cli.go` — Updated context command short description.
- `internal/ahm/task_work.go` — Updated delegation prompt.
- `internal/templates/templates.go` — Bumped Version to 0.4.3, updated suggestions.
- `internal/templates/workflow/TASKS.md` — Made `ahm task show <id>` the normal inspection primitive.
- `internal/templates/workflow/preflight-SKILL.md` — Updated task context item.
- `.agents/skills/preflight/SKILL.md` — Same update as embedded template.
- `AGENTS.md` — Updated managed-work intake entry. `docs/guides/workflow-upgrades.md` — Added 0.4.3 section.
- `docs/references/cli/commands.md` — Updated context command docs.
- `docs/references/workflow-spec.md` — Updated terminology.
- `docs/adr/011-*.md` — Added refinement note.
- `ARCHITECTURE.md` — Updated terminology.

### Verification

- `go test -race -cover ./internal/...` — All pass.
- `just ci` — Full CI (fmt, tidy, vet, test -race -cover, lint, vuln, build, goreleaser check) — All pass.
- Manual smoke check: `ahm context`, `ahm --json context`, `ahm context task`, `ahm --json context task` produce expected shapes.

### Compatibility impact

- Scoped JSON output shape changed; consumers relying on `root`, `workflow`, `git`, `tasks` in `ahm --json context task` need to adapt to the new `scope`/`instructions`/`commands` shape.
- Unscoped JSON lost `instructions` field.
- These are deliberate breaking changes documented in the upgrade guide.

## Context and Orientation

`ahm` is a Go CLI that manages repository-local workflow state under `.agents/`. The relevant command is wired in `internal/ahm/cli.go` and implemented in `internal/ahm/context.go`. The command currently has one optional scope argument: `task`, `adr`, `research`, `plan`, or `docs`.

The current implementation builds a single `contextReport` containing root, workflow validation, git state, task summary, instructions, and commands. Text output behaves differently depending on scope. Unscoped text prints `# ahm context`, live state, an `## Instructions` section, and `## Useful Commands`. Scoped text prints only the embedded reference document by calling `emitScopedInstructionText`. JSON currently emits the full `contextReport` for both unscoped and scoped invocations.

Embedded reference documents live under `internal/templates/workflow/`. They are read by `scopedContextInstruction` in `internal/ahm/context.go`. `internal/templates/templates.go` contains `AgentSuggestions`, the source for `ahm agents suggestions`, and `Version`, the embedded workflow template version stamped into `.agents/ahm.json` by install and upgrade commands.

This repository's root `AGENTS.md` is project-owned. `ahm init`, `ahm upgrade`, and `--force` must never create, overwrite, or remove project `AGENTS.md`, but this task may edit this repository's own `AGENTS.md` because the requested work is explicitly to update the ahm repo's workflow contract.

`ahm task show <id>` prints the task Markdown content. It is the normal inspection primitive for tasks. Reading `.agents/.tasks/active/<id>.md` directly remains useful when `ahm` is unavailable or when manually editing the task record.

## Plan of Work

Start by changing `internal/ahm/context.go` so unscoped and scoped JSON are separate report shapes. Keep the unscoped report focused on root, workflow, git, tasks, and commands. Remove unscoped instructions from both text and JSON. Add a scoped report shape that contains `scope`, `instructions`, and `commands`. Do not include live workflow, git, or task fields in scoped JSON.

Adjust validation text in `emitContextText`. If errors and warnings are both zero, print `validation: ok`. Otherwise print the error and warning counts, include `run ahm doctor`, and render up to five findings, ordered errors first and then warnings. Reuse or preserve `contextFindings`'s total cap behavior.

Adjust `contextCommands`. For unscoped output, keep command discovery read-only/reference-oriented: `ahm status`, `ahm doctor`, `ahm task ready`, `ahm task blocked`, `ahm task show <id>`, and `ahm context <scope>`. Remove `ahm index --dry-run` from the default list. For scoped JSON, keep commands only if they are scoped discovery commands and do not imply mutation as a default next step.

Update `internal/ahm/cli.go` help so the command is described as two modes while keeping syntax stable. The short help should communicate "repository briefing or ahm-managed workflow reference" rather than only "agent session context."

Update `internal/templates/templates.go` to bump `Version` to `0.4.3` and revise `AgentSuggestions`. The suggestions should describe `ahm context` as live briefing, scoped `ahm context` as managed-work references, and project `AGENTS.md` as the owner of routing. Remove the sentence that says to treat `ahm context` output as canonical workflow guidance.

Update embedded workflow reference prose in `internal/templates/workflow/TASKS.md` and any other scoped reference that claims session-start control. The references should say they are used when project routing, managed-work intake, or the user request points at the artifact type. In `TASKS.md`, make `ahm task show <id>` the normal task inspection path and reserve opening the task file for fallback or manual edits.

Update `internal/templates/workflow/preflight-SKILL.md` and the installed `.agents/skills/preflight/SKILL.md` copy. The skill should require `ahm task show <id>` output when the work came from a task, and mention the active task file only as fallback when `ahm` is unavailable or when reviewing manual edits to the task file itself.

Update `internal/ahm/task_work.go` to avoid session-start language. The prompt should tell delegated agents to use `ahm context task` as the task workflow reference and inspect the task with `ahm task show <id>` before editing. Do not tell them to open the task file too.

Update durable docs: `docs/references/cli/commands.md`, `docs/references/cli/global-contract.md`, `docs/references/workflow-spec.md`, `docs/guides/workflow-upgrades.md`, `ARCHITECTURE.md`, and `docs/adr/011-expose-agent-instructions-through-context-command.md`. The docs must acknowledge the JSON compatibility change, the two command modes, the warning display behavior, and the `0.4.3` template update.

Update this repository's `AGENTS.md` to remove the instruction to open the task file after inspecting a task with `ahm task ...`. Use wording that treats `ahm task show <id>` or another task command that prints task contents as sufficient before editing.

Update tests in `internal/ahm/context_test.go`, `internal/ahm/agents_test.go`, and any template tests affected by the wording/version changes. Add or adjust tests for unscoped text without instructions, unscoped JSON without instructions, scoped JSON without live briefing fields, warnings-only validation display, command help or command lists if currently covered, and suggestions wording.

## Concrete Steps

Work from repository root `/Users/travisennis/Projects/ahm`.

Before editing implementation, inspect the current context and suggestions behavior:

    ahm context
    ahm --json context
    ahm context task
    ahm --json context task
    ahm agents suggestions --all

Implement the code and template changes described above. Prefer focused edits in the existing files; do not add new abstractions unless the split between unscoped and scoped JSON would otherwise be confusing.

Run focused tests while iterating:

    go test ./internal/ahm ./internal/templates

After docs and template edits, regenerate workflow indexes because task 117 and this ExecPlan changed:

    ahm index
    ahm --dry-run index

Before completion, run the project's full verification command from `CONTRIBUTING.md`. If `CONTRIBUTING.md` identifies `just ci` as the full check, run:

    just ci

At completion, update this ExecPlan's Progress and Outcomes & Retrospective sections, fill task 117 Acceptance Notes, and complete the task with:

    ahm task complete 117

## Validation and Acceptance

The observable CLI behavior after implementation should be:

Running `ahm context` prints a live briefing with root, workflow versions, validation summary, git state, task summary, next/in-progress tasks when present, and useful commands. It does not print `## Instructions`.

Running `ahm --json context` prints JSON containing `root`, `workflow`, `git`, `tasks`, and `commands`. It does not contain `instructions`.

Running `ahm context task` prints the task reference document only. It does not print `# ahm context`, git state, task summary, or useful command wrapper text.

Running `ahm --json context task` prints scoped JSON with `scope`, `instructions`, and optionally `commands`. It does not contain `workflow`, `git`, or `tasks`.

When validation has warnings and no errors, `ahm context` prints a warning count and sample findings instead of `validation: ok`. When validation has no errors and no warnings, it prints `validation: ok`.

The focused test command `go test ./internal/ahm ./internal/templates` should pass. Full project verification from `CONTRIBUTING.md` should pass before task completion.

## Idempotence and Recovery

All planned edits are normal source, docs, template, task, and ExecPlan edits. Re-running tests and `ahm index` is safe. If index generation reports changes after manual task or ExecPlan edits, rerun `ahm index` and inspect the generated index diffs rather than editing indexes directly. Do not use destructive git commands to recover; inspect `git status --short` and revert only intentional local edits if Travis explicitly asks.

## Artifacts and Notes

Key current artifacts before implementation:

    ahm context
    # ahm context
    ...
    validation: ok
    ...
    ## Instructions

    ahm --json context task
    {
      "root": "...",
      "workflow": {...},
      "git": {...},
      "tasks": {...},
      "instructions": [...],
      "commands": [...]
    }

Those shapes should change according to this plan.

## Interfaces and Dependencies

Keep the CLI syntax stable:

    ahm context
    ahm context task
    ahm context adr
    ahm context research
    ahm context plan
    ahm context docs

Keep the valid scope names stable. This task changes output contracts and documentation, not command names.

The implementation should continue using existing packages and helpers in `internal/ahm`, including `validateWorkflow`, `readMetadata`, `readGitContext`, `contextFindings`, `contextCommands`, `contextInstructions`, and the shared JSON emitter. If a new struct is needed for scoped JSON, define it in `internal/ahm/context.go` near the existing context report types.
