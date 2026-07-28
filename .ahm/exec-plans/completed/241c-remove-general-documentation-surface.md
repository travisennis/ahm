# Remove the general documentation command and configuration surface

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document is maintained in accordance with the output of `ahm context plan`.

## Purpose / Big Picture

After this change, ahm presents one validation boundary: workflow integrity for tasks, research, ExecPlans, and ADRs. Running `ahm docs` or `ahm docs check` produces the normal unknown-command usage failure, and `status` or `doctor` accepts only the independently composable `workflow` and `links` scopes. Existing repositories may still contain the obsolete `projectDocs` JSON key; ordinary reads tolerate it, while `ahm upgrade` removes it without discarding unrelated unknown metadata.

The observable retained behavior is that default `ahm status` and `ahm doctor` still validate the four managed record families and their relative Markdown links, including ADRs, as established by task 241b.

## Progress

- [x] (2026-07-28T00:47Z) Primed the repository; inspected tasks 241, 241a, 241b, and 241c; confirmed dependencies are complete; and started task 241c.
- [x] (2026-07-28T00:47Z) Loaded the CLI, workflow-state, safety, implementation-quality, documentation, and verification routes plus ADR 021 and the current task, ADR, plan, and documentation context procedures.
- [x] (2026-07-28T00:47Z) Located the command wiring, validation scopes and general-documentation helpers, metadata model and upgrade rewrite path, focused tests, and compatibility documentation.
- [x] (2026-07-28T01:06Z) Removed the CLI command, scope alias/deprecation path, typed configuration, general-documentation validators, finding codes, and tests that existed only for those surfaces.
- [x] (2026-07-28T01:06Z) Added focused removal and upgrade-preservation coverage while retaining workflow/link validation tests.
- [x] (2026-07-28T01:06Z) Updated the CLI, workflow, upgrade, architecture, glossary, changelog, guardrail, embedded docs procedure, and repository completion-check references that described the removed surfaces.
- [x] (2026-07-28T01:06Z) Ran focused tests, formatting, package tests, Markdown checks, direct binary probes, `just ci`, independent subagent review, and L-scale preflight; applied all review findings and received a clean second review.
- [x] (2026-07-28T01:06Z) Completed task and ExecPlan records with exact verification evidence and prepared them for lifecycle completion.

## Surprises & Discoveries

- Observation: The custom metadata decoder already separates recognized keys from unknown top-level JSON, while the custom encoder deterministically re-emits unknown keys.
  Evidence: `internal/ahm/install.go` deletes recognized names from a raw JSON map and stores the remainder in `metadata.Extra`.
- Observation: The retained managed-record link validator is already structurally separate from the project-documentation validator.
  Evidence: `validateWorkflowScopedForPaths` dispatches `CheckScopeLinks` to `validateMarkdownLinks` and the opt-in `CheckScopeProjectDocs` to `validateProjectDocs`.
- Observation: A non-empty placeholder under an active ExecPlan's `Outcomes & Retrospective` section is itself validation drift.
  Evidence: The direct retained-scope probe emitted `exec_plan_active_with_outcomes`; clearing the section removed the task-created warning.
- Observation: The sandbox blocks the default Go, golangci-lint, npm, and vulnerability-database cache/network paths.
  Evidence: the first local checks failed on `~/Library/Caches` permissions or `vuln.go.dev` DNS; a task-specific `GOCACHE` and the approved unsandboxed final `just ci`/Markdown checks passed.

## Decision Log

- Decision: Treat `projectDocs` as an ignored obsolete key during metadata decoding rather than retaining a typed runtime field.
  Rationale: JSON decoding remains tolerant, subsequent metadata writes omit the obsolete field, and unrelated unknown top-level fields continue to round-trip through `metadata.Extra`.
  Date/Author: 2026-07-28 / Codex
- Decision: Remove the entire Cobra `docs` command group with no compatibility alias.
  Rationale: ADR 021 classifies the removal as intentional and says an alias would preserve the rejected product boundary.
  Date/Author: 2026-07-28 / Codex
- Decision: Keep only `workflow` and `links` in the shared scope parser and leave their default composition unchanged.
  Rationale: Task 241b made those validators the retained structured-record integrity surface.
  Date/Author: 2026-07-28 / Codex
- Decision: Collapse the configurable Markdown finding-code helper into the retained managed-record link validator.
  Rationale: Independent review identified that code variability existed only for the removed general-documentation path; the retained path has one stable pair of finding codes.
  Date/Author: 2026-07-28 / Codex

## Outcomes & Retrospective

Task 241c's acceptance scope is complete. The root Cobra command no longer
registers `docs`; `ahm docs` and `ahm docs check` now exit 2 through the normal
unknown-command path. `status` and `doctor` accept only `workflow` and `links`,
which remain independently selectable and composable. Default validation still
checks managed workflow state and relative links in tasks, research, ExecPlans,
ADRs, and their generated indexes.

The metadata model has no general-documentation configuration type. Its decoder
consumes the obsolete `projectDocs` key without treating it as runtime
configuration or preserving it in the unknown-field map. Ordinary reads
therefore tolerate transition files, real upgrade atomically omits the key, and
unrelated unknown metadata remains intact. Focused tests cover current and
legacy metadata paths plus dry-run immutability.

General project-documentation discovery, portability, entry-point budget,
index-coverage validators, finding codes, command output, and exclusive tests
were deleted. CLI references, workflow semantics, upgrade guidance, changelog,
architecture, glossary, project guardrails, embedded documentation guidance,
and the repository standing prompt now describe the narrowed boundary.
Historical ADRs, research, tasks, and changelog entries remain intact.

Focused tests, `go test ./internal/templates ./internal/ahm`, Markdown lint,
direct development-binary probes, `git diff --check`, and two full `just ci`
runs passed. Independent review found and prompted fixes for the default status
description, one stale architecture phrase, and an unnecessary configurable
finding-code helper; its second round reported no findings. L-scale preflight
then completed rules/documentation, correctness/source-of-truth, and
simplification passes with no additional changes. Task 241d still owns removal
of `ahm context docs` and its binary-owned procedure; task 235 owns the final
repository-wide product-documentation reconciliation.

## Context and Orientation

Ahm is a Go CLI. `internal/ahm/cli.go` constructs the Cobra command tree and owns `status`, `doctor`, and the soon-to-be-removed `docs check` flags and help. `internal/ahm/status.go` runs shared validation, emits the deprecated `project-docs` warning, and implements the dedicated docs report and strict-warning promotion. `internal/ahm/validation.go` owns the accepted validation scopes and has two separate link paths: `validateMarkdownLinks` discovers only tasks, research, ExecPlans, ADRs, and their generated indexes; `validateProjectDocs` discovers general project Markdown and runs the validators being removed.

`internal/ahm/install.go` owns metadata read/write behavior for current `.ahm/config.json` and legacy `.agents/ahm.json`. Its `metadata` struct represents supported settings, while `metadata.Extra` preserves unknown top-level JSON values. `ahm upgrade` reads that model and atomically writes it back, so deleting `projectDocs` from the raw unknown-key map without adding a typed field makes the obsolete key tolerated but not preserved. The same rewrite retains unrelated entries in `Extra`.

ADR 021, `docs/adr/021-limit-ahm-to-structured-workflow-records.md`, is the accepted product decision. It removes general project-documentation management while retaining relative-link validation for research, ADRs, ExecPlans, and tasks. Task 241d separately removes `ahm context docs` and its embedded procedure; this plan must not take over that child task.

## Plan of Work

First, narrow the public CLI in `internal/ahm/cli.go` and `internal/ahm/status.go`. Delete the `docs` command tree, its `strict` option, its execution helper, the project-doc scope deprecation warning, and all help examples that advertise the removed scope. Change the status and doctor `--check` help to list only `workflow` and `links`. Add CLI tests proving both `ahm docs` and `ahm docs check` fail with exit code 2 as unknown-command usage, and that `--check project-docs` is rejected while `workflow` and `links` remain accepted alone and together.

Second, narrow the metadata and validator internals. Remove `projectDocsConfig`, `defaultEntryPointBudget`, and `metadata.ProjectDocs`. Keep `projectDocs` in the decoder's list of consumed legacy keys so it is ignored instead of entering `metadata.Extra`. Remove `CheckScopeProjectDocs`, `validateProjectDocs`, discovery, portability, entry-point budget, index-coverage helpers, and their exclusive tests. Preserve the shared Markdown parser and `validateMarkdownFileLinksWithCodes` only if the retained managed-record path still needs them; otherwise simplify the retained call without changing its finding codes. Add current- and legacy-layout upgrade tests that begin with `projectDocs` plus an unrelated unknown key and prove the obsolete key disappears while the unrelated key survives.

Third, update current compatibility documentation. Remove the command from `docs/cli.md` and `docs/references/cli/commands.md`; document only `workflow` and `links` in `docs/references/workflow-spec.md`; remove general-doc findings from `docs/references/cli/task-file-format.md`; revise `docs/references/glossary.md`, `ARCHITECTURE.md`, and `docs/guardrails/documentation.md`; add a breaking-removal entry to `CHANGELOG.md` and a dated upgrade note to `docs/guides/workflow-upgrades.md`. Update live repository completion checks and project instructions that name `ahm docs check` or `--check project-docs` when they are current operational guidance. Preserve historical ADRs, completed tasks, research records, and changelog history.

Finally, run the focused CLI, metadata, validation, and retained managed-link tests before the full internal package. Run formatting, Markdown lint, and `just ci`. Then use an independent subagent to review the implementation, address findings until none remain, and perform the repository preflight skill. Update the plan and task acceptance notes, move this plan to the completed bucket, update the task link, and complete task 241c.

## Concrete Steps

Run all commands from `/Users/travisennis/Projects/ahm`.

After code and focused test edits, run:

    go test ./internal/ahm -run 'Test.*(DocsCommand|CheckScope|Metadata|Upgrade|ManagedMarkdown|MarkdownLinks|Status|Doctor)'

Format Go code and run the package suite:

    just fmt
    go test ./internal/ahm

Check Markdown structure and the full repository:

    just docs-md-lint
    just ci

The focused and full commands must exit zero. Direct CLI tests must observe exit code 2 for the removed command and scope, and retained scope tests must observe their existing success or validation exit behavior.

## Validation and Acceptance

Build the development binary with `just build`. In an initialized temporary repository, `bin/ahm --root <repo> docs` and `bin/ahm --root <repo> docs check` must exit 2 and identify `docs` as an unknown command. `bin/ahm --root <repo> status --check project-docs` and the equivalent doctor invocation must exit 2 and identify the unsupported scope. `--check workflow`, `--check links`, and `--check workflow,links` must remain valid.

Place a broken relative link in each retained record family in focused test fixtures and confirm the retained `markdown_link_missing` behavior still covers tasks, research, ExecPlans, and ADRs without scanning unrelated project Markdown. Start current and legacy config fixtures with `projectDocs`, an unrelated raw object, and normal supported settings; after a real upgrade, the obsolete key must be absent, the unrelated object and supported settings must be unchanged, and dry-run must leave the original file byte-for-byte unchanged.

Search current source, tests, and live compatibility documentation for the removed command, scope, config, validators, and finding codes. Remaining matches are permitted only in historical ADRs, research, completed/cancelled task records, the active tracker/follow-up records, and explicit breaking-removal or migration history.

## Idempotence and Recovery

The validator and command removals introduce no new writes. Metadata continues through the existing atomic writer. Upgrade is safely repeatable: once `projectDocs` is absent, another upgrade produces no additional semantic change. Dry-run must never rewrite metadata. If an upgrade test fails, inspect the temp repository's metadata and confirm the failure is not caused by generated-index output; do not hand-edit generated indexes.

All task and ExecPlan lifecycle changes use `ahm` commands except the plan file and task `exec_plan` linkage, whose source records may be edited directly before `ahm task complete` regenerates indexes. No commit, staging, branch, ref, or network operation is part of this plan.

## Artifacts and Notes

The removed command should follow Cobra's root unknown-command path, for example:

    unknown command "docs" for "ahm"

The unsupported scope should use the shared usage error and list the retained values:

    unknown validation scope "project-docs" (valid: workflow, links)

A transition config such as:

    {"strict_acceptance":false,"projectDocs":{"entryPointBudget":120},"vendorExtension":{"enabled":true},"files":{}}

must rewrite without `projectDocs` while preserving `vendorExtension`.

## Interfaces and Dependencies

Use only the Go standard library and the existing Cobra dependency. At the end, `validCheckScopes()` returns exactly `workflow` and `links`; the metadata model has no project-documentation config field or type; and no general-documentation discovery or validation helper remains. The retained managed-record Markdown walker, finding codes, output modes, atomic write path, and current/legacy record-layout behavior remain compatible.

Revision note (2026-07-28): Created after repository intake, routed-document review, ADR 021 inspection, and source discovery so the breaking removal and migration semantics survive context compaction. Updated throughout implementation with the ignored-key migration design, test and direct-probe evidence, independent-review fixes, preflight results, and final outcomes.
