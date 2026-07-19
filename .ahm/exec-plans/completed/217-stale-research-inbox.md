# Surface stale research inbox notes

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document is maintained in accordance with the output of `ahm context plan`.

## Purpose / Big Picture

After this change, a repository owner can run `ahm doctor` and see a warning for each research inbox note that has waited at least the configured number of days without disposition. The same note age and stale state appear in `ahm prime`, where an agent can act at session start. A repository may set `research.inboxStaleDays` in its workflow metadata, with 21 days as the default and zero disabling the check. Ahm never moves, converts, or deletes a note.

## Progress

- [x] (2026-07-19T18:48Z) Inspected task 217, current workflow metadata, validation, prime rendering, research guidance, upgrade behavior, tests, and compatibility documentation.
- [x] (2026-07-19T18:48Z) Recorded the durable behavior and tradeoffs in ADR 020.
- [x] (2026-07-19T19:01Z) Added layout-aware research age calculation and configurable metadata round trips, including explicit rejection of negative thresholds.
- [x] (2026-07-19T19:01Z) Added the workflow warning and prime text/JSON surfacing with focused tests for fresh, stale, disabled, dated, undated, custom-threshold, empty, and legacy-layout notes.
- [x] (2026-07-19T19:01Z) Updated the embedded research reference, template version 0.6.4, CLI finding/output docs, workflow specification, upgrade guide, architecture map, and ADR 020.
- [x] (2026-07-19T19:01Z) Completed L-scale preflight and full `just ci`; external Codex review was attempted but sandbox escalation was rejected because it would export uncommitted repository contents without explicit user authorization.

## Surprises & Discoveries

- Observation: The documented research header is a body-level `Created:` / `Updated:` block rather than YAML front matter, while raw inbox notes may contain neither.
  Evidence: `internal/templates/workflow/RESEARCH.md` documents those scalar header lines and explicitly allows shorter inbox notes.
- Observation: The motivating 0.4.x repositories use the legacy `.agents/` record layout, so limiting the check to `.ahm/` would not solve the reported consumer problem.
  Evidence: `workflowPaths.researchRel()` already selects `.agents/.research` or `.ahm/research` for all callers.

## Decision Log

- Decision: Apply the check to both current and legacy record layouts.
  Rationale: The path abstraction makes parity narrow, and legacy consumers are part of the motivating case.
  Date/Author: 2026-07-19 / Codex
- Decision: Store the optional setting as `research.inboxStaleDays`, default it to 21 when absent, and treat zero as disabled.
  Rationale: A pointer-backed integer distinguishes omission from an explicit zero while preserving additive JSON compatibility; negative values are rejected at metadata decode rather than silently changing behavior.
  Date/Author: 2026-07-19 / Codex
- Decision: Prefer valid `updated`, `date`, then `created` metadata and fall back to file modification time; do not call Git.
  Rationale: Updates should reset staleness, both documented research headers and YAML notes should work, and mtime is a portable no-subprocess fallback.
  Date/Author: 2026-07-19 / Codex
- Decision: Emit `research_inbox_stale` as a warning at age greater than or equal to the threshold.
  Rationale: Stale untriaged research merits action but must not make validation fail.
  Date/Author: 2026-07-19 / Codex
- Decision: Add disposition guidance to `ahm context research`.
  Rationale: The warning can stay concise while the scoped reference owns the durable lifecycle explanation.
  Date/Author: 2026-07-19 / Codex

## Outcomes & Retrospective

Task 217's acceptance scope is complete. Workflow metadata now carries the
optional `research.inboxStaleDays` setting without changing fresh-install
output when it is absent. The default is 21 days, zero disables the check,
positive values customize it, and negative values fail metadata parsing with
an actionable error. Upgrade round-trip coverage confirms the setting and
unknown top-level fields survive metadata rewrites.

The shared research helper recognizes flat YAML and the documented research
header, prefers `updated`, then `date`, then `created`, and falls back to file
modification time. Both current and legacy layouts emit warning-tier
`research_inbox_stale` findings with explicit dispositions. Prime preserves its
existing sort/cap behavior while adding inbox-only age and stale state to text,
JSON, and plain output. The scoped research reference now explains that ahm
never applies a disposition automatically.

Focused research/config/doctor/prime tests passed, followed by
`go test ./internal/templates ./internal/ahm` and the complete `just ci` suite.
The development binary's dry-run and real `upgrade` paths both succeeded and
advanced this repository to workflow template 0.6.4. L-scale preflight found
and fixed the negative-threshold ambiguity, then found no further actionable
correctness or simplification issues. The repository-specific external Codex
review could not be completed because its sandbox escalation was rejected as
an unauthorized export of uncommitted code; no workaround was attempted.

## Context and Orientation

Ahm is a Go CLI. `internal/ahm/install.go` owns the JSON metadata model read from `.ahm/config.json` or legacy `.agents/ahm.json`; its custom marshal logic preserves unknown top-level fields and `ahm upgrade` rewrites the parsed model with a new template version. `internal/ahm/workflow_paths.go` selects the record layout. `internal/ahm/validation.go` builds the report shared by `status`, `doctor`, and the validation summary in `prime`. `internal/ahm/prime.go` collects up to five research notes and renders both structured and human output. `internal/templates/workflow/RESEARCH.md` is the embedded source for `ahm context research`, and changing it requires advancing `internal/templates.Version` in `internal/templates/templates.go`.

A research inbox note is any direct Markdown child of the selected research `inbox/` directory, excluding `index.md`. A disposition means promoting useful synthesis to `topics/`, converting actionable work into an ahm task, or deleting material with no continuing value. Staleness is advisory and never authorizes ahm to perform one of those actions.

ADR 020 in `docs/adr/020-report-stale-research-inbox-notes.md` defines the durable contract implemented by this plan.

## Plan of Work

First, extend the concrete metadata structs and explicit JSON marshal/unmarshal key handling in `internal/ahm/install.go` with an optional research configuration. Add a helper that resolves the effective default, explicit disable, and positive threshold. Keep the object absent on fresh installs so existing config rendering changes only when a user configured it. Exercise custom values and zero through read/write and upgrade tests in `internal/ahm/install_test.go`.

Second, add a focused research-note age helper in `internal/ahm/validation.go` or a small adjacent source file in the same package. It will read normalized Markdown, examine flat YAML front matter and documented body header lines case-insensitively for ISO `YYYY-MM-DD` values in the precedence `updated`, `date`, `created`, and otherwise use `os.Stat` modification time. Convert the selected instant to a clamped non-negative whole-day age using an injected `now` value in helpers so tests do not depend on the wall clock.

Use that helper in workflow-scope validation to scan the selected inbox deterministically. For every note whose age reaches the enabled threshold, add the warning code `research_inbox_stale` at its layout-relative path. The message must include the age, threshold, and the three dispositions. Ensure empty, missing, fresh, and disabled inboxes produce no stale finding. Add focused tests in `internal/ahm/validation_test.go`, including a legacy-layout case.

Third, reuse the age helper in `internal/ahm/prime.go`. Extend `primeResearchNote` additively so inbox notes include age and stale state in JSON/plain output. In text output, append a concise age label to inbox entries and a clear stale marker only when the effective threshold is enabled and reached. Leave non-inbox note rendering unchanged. Preserve the existing global filename sort and five-note cap. Add collection, text, JSON, disabled, and fallback tests in `internal/ahm/prime_test.go`.

Fourth, add the disposition rule to `internal/templates/workflow/RESEARCH.md` and advance the embedded template version. Document the config, layout parity, warning code, age basis, `prime` fields and rendering, and status/doctor behavior in `docs/references/workflow-spec.md`, `docs/references/cli/commands.md`, `docs/references/cli/task-file-format.md`, and `docs/guides/workflow-upgrades.md`. Regenerate only generated indexes through `ahm index` or lifecycle commands.

Finally, update this plan and task 217 acceptance notes with exact evidence. Run the repository-mandated codex review and fix findings until clean. Then run the preflight skill, apply its worthwhile fixes, complete the plan lifecycle, and use `ahm task complete 217`.

## Concrete Steps

Run all commands from `/Users/travisennis/Projects/ahm`.

After the metadata and age helper edits, run:

    go test ./internal/ahm -run 'Test.*(Metadata|ResearchInbox|PrimeRecentResearch|PrimeJSON)'

After the embedded research template and documentation edits, run:

    go test ./internal/templates ./internal/ahm

Format Go changes and run the complete repository verification:

    just fmt
    just ci

The focused tests must exit zero and demonstrate fresh, stale, disabled, missing-date fallback, explicit research dates, and legacy layout behavior. The full CI command must exit zero.

## Validation and Acceptance

In a temporary `.ahm/` repository, place a Markdown file in `.ahm/research/inbox/` with a date or modification time at least 21 whole days old. `ahm doctor` must return success with a warning-tier `research_inbox_stale` finding naming the note and the promote-to-topic, convert-to-task, and delete dispositions. `ahm prime` must continue returning success, show the inbox entry's age and stale marker in text, and expose additive age and stale data in JSON. A fresh note, an empty inbox, and a repository with `{ "research": { "inboxStaleDays": 0 } }` must not produce the finding or stale marker. A custom positive threshold must take effect, survive `ahm upgrade`, and round-trip byte-semantically through the metadata model. Equivalent notes in the legacy `.agents/.research/inbox/` layout must behave the same while reporting legacy-relative paths.

## Idempotence and Recovery

All new runtime behavior is read-only. Re-running `status`, `doctor`, or `prime` does not change research notes; `prime` retains its existing explicit index-regeneration behavior. Metadata writes continue through the repository's atomic writer. If a metadata test fails, inspect the rendered JSON before retrying; unknown top-level keys and unrelated configuration must remain intact. Generated indexes must never be edited manually and can be recovered with `ahm index`.

## Artifacts and Notes

The expected warning shape is concise and disposition-oriented, for example:

    research_inbox_stale: research inbox note is 24 days old (threshold 21); promote it to research/topics, convert it to a task, or delete it if it has no continuing value (.ahm/research/inbox/example.md)

The expected Recent Research text preserves the existing link and title while adding inbox-only state, for example:

    - inbox .ahm/research/inbox/example.md Example (24 days old, STALE)

## Interfaces and Dependencies

Use only the Go standard library and existing package helpers. The metadata boundary will add a concrete optional research configuration carrying `inboxStaleDays`. The shared age calculation must accept a caller-provided `time.Time` so validation and prime tests can be deterministic, return whole non-negative days, and expose whether a usable timestamp was obtained. No new command, flag, external dependency, Git invocation, write path, or background process is introduced.

Revision note (2026-07-19): Created this plan after repository inspection and ADR 020 so the cross-cutting implementation, compatibility decisions, and verification survive context compaction. Updated during preflight to record explicit rejection of negative stale-day thresholds at the metadata boundary. Completed the progress, outcomes, verification, and review notes after implementation and full CI.
