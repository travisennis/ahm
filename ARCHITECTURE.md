# Architecture

`ahm` is a single-binary Go CLI. It manages repository-local workflow state
under `.agents` for legacy committed-record repositories and under tool-owned
`.ahm` after the opt-in ADR 013 ref-backed records migration, which also moves
committed configuration to `.ahm/config.json`. Workflow commands resolve
record paths from the configured storage mode, and record mutations in
ref-backed repositories refresh the local `refs/ahm/records` snapshot. It
exposes managed-work references and live repository briefings, validates that
workflow, regenerates deterministic indexes, and can delegate a resolved task
to an external coding-agent CLI.

## System Boundaries

- `ahm` owns workflow installation, upgrades, managed-work references,
  validation, task/ADR lifecycle commands, generated indexes, and external
  agent orchestration.
- Target repositories own their source code and project-specific `AGENTS.md`.
  `ahm` does not patch source files, commit, create PRs, or run implicit git
  operations. Explicit records commands may fetch, push, and update private
  `refs/ahm/*` refs without moving `HEAD`, staging files, writing the project
  index, or modifying project-owned `.agents/` content.
- External coding agents own the actual work performed through
  `ahm task work`. `ahm` validates state, builds the invocation, captures
  session IDs, and resumes sessions for review, completion, or commit handoff.

## Compatibility Surfaces

- CLI command names, flags, aliases, exit codes, help text, and output modes.
- Text, JSON, and plain output shapes, including validation finding codes.
- `.agents/ahm.json` and `.ahm/config.json` metadata fields and version
  semantics.
- Task, research, ExecPlan, ADR, and generated index formats.
- Embedded templates under `internal/templates/workflow/`.
- Install and upgrade conflict behavior, including project-owned `AGENTS.md`
  behavior and legacy instruction-template removal.
- Atomic write guarantees and stale temp-file cleanup.
- External agent argument shapes, JSONL/session parsing, and resume behavior.
- Go module version, local tool versions, CI, and release packaging.

## Module Map

- `cmd/ahm/main.go`: CLI entrypoint.
- `internal/ahm/cli.go`: Cobra root command, global flags, command wiring.
- `internal/ahm/root.go`: repository root discovery.
- `internal/ahm/context.go`: `context` command briefing and managed-work
  references.
- `internal/ahm/agents.go`: `agents suggestions` command that reports missing
  AGENTS.md integration suggestions.
- `internal/ahm/install.go`: `init`, `upgrade`, metadata, legacy instruction
  removal, and generated index writes.
- `internal/ahm/status.go`: `status` and `doctor`.
- `internal/ahm/validation.go`: workflow, link, ADR, task, and project-doc
  validation.
- `internal/ahm/output.go`: shared text, JSON, and plain emitters.
- `internal/ahm/path.go`: `relPath` helper for converting absolute paths to
  slash-separated relative paths.
- `internal/ahm/workflow_paths.go`: storage-mode-aware resolution of workflow
  record paths (`.agents` for legacy committed records, `.ahm` after the
  ref-backed migration).
- `internal/ahm/records.go`: internal ref-backed workflow record plumbing for
  selecting `.ahm/` source records, snapshotting them to `refs/ahm/*`, syncing
  private refs, comparing ref state, and materializing records back to `.ahm/`.
- `internal/ahm/records_commands.go`: `records status`, `pull`, `push`,
  `sync`, and `doctor` command surface for explicit ref-backed records sync and
  diagnostics.
- `internal/ahm/tasks.go`: task model, parsing, rendering, and ID helpers.
- `internal/ahm/task_commands.go`: `task` command wiring (`taskCommand` and
  `taskListCommand`).
- `internal/ahm/task_comment.go`: `task comment` command for appending
  timestamped comments to task bodies.
- `internal/ahm/task_create.go`: `task create` parsing, body resolution, and ID
  allocation.
- `internal/ahm/task_list.go`: task list, next, labels, show, and
  filter/sort helpers.
- `internal/ahm/task_status.go`: status transitions, dependent unblocking, and
  cancellation reasons.
- `internal/ahm/task_find.go`: task ID resolution and prefix matching.
- `internal/ahm/task_enum.go`: task status, priority, and effort enum
  validation.
- `internal/ahm/task_work.go`: `task work` delegation to external coding-agent
  CLIs.
- `internal/ahm/task_agents.go`: external agent registry and selection.
- `internal/ahm/task_session.go`: agent session orchestration, review, and
  completion/commit handoff.
- `internal/ahm/task_parsers.go`: agent stream-JSON session/feedback parsers and
  resume-arg builders.
- `internal/ahm/task_deps.go`: task dependency commands and cycle handling.
- `internal/ahm/task_migrate.go`: task metadata migration.
- `internal/ahm/task_acceptance.go`: acceptance-note completion checks.
- `internal/ahm/adrs.go`: ADR model, parsing, rendering, and validation.
- `internal/ahm/adr_commands.go`: ADR lifecycle commands.
- `internal/ahm/adr_migrate.go`: legacy ADR migration.
- `internal/ahm/indexes.go`: task, research, ExecPlan, and ADR index rendering.
- `internal/ahm/lock.go`: repository-local workflow locks for serialized
  cross-process mutations.
- `internal/ahm/write.go`: atomic writes and stale temp cleanup.
- `internal/templates/templates.go`: embedded template registry and template
  version.
- `internal/templates/workflow/`: canonical workflow templates installed into
  target repositories.
- `internal/version/version.go`: binary version injected by release builds.

## Architectural Invariants

- Writes are explicit and use the atomic temp-file-then-rename path in
  `internal/ahm/write.go`.
- Cross-process workflow mutations that require read-compute-write consistency
  use repository-local locks under `.agents/.lock/` in legacy mode and
  `.ahm/.lock/` after ref-backed migration.
- Generated indexes are deterministic; sort output consistently and keep index
  generation centralized.
- Managed-work references are exposed by scoped `ahm context task|plan|adr|research|docs`;
  managed agent skills remain installed templates under `.agents/skills/`.
- Legacy managed instruction templates are removed by `upgrade` only when
  metadata proves ownership, unless `--force` is used.
- `AGENTS.md` is project-owned. Never treat a project `AGENTS.md` as a managed
  file that `init`, `upgrade`, or `--force` can create, replace, or remove.
- Validation is read-only. It reports workflow and documentation drift without
  mutating files.
- Command handlers should stay thin: parse args, validate boundaries, delegate
  to focused helpers, then emit output.
- File-format parsers should validate at the boundary and return explicit
  errors; renderers should preserve unknown fields where the format promises it.
- Dry-run behavior must not mutate disk or in-memory state in ways that affect
  later operations.

## Reference Docs

- Documentation index: `docs/README.md`.
- CLI contract: `docs/cli.md` and `docs/references/cli/`.
- Workflow state, file ownership, formats, and atomic writes:
  `docs/references/workflow-spec.md`.
- Upgrade and version behavior: `docs/guides/workflow-upgrades.md`.
- Agent integration smoke checks: `docs/guides/testing.md`.
- ADR workflow and decision history: `docs/adr/README.md` and `docs/adr/`.
- Contributor commands and handoff expectations: `CONTRIBUTING.md`.
