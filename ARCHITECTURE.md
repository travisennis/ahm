# Architecture

`ahm` is a single-binary Go CLI. It manages a repository-local `.agents`
workflow, validates that workflow, regenerates deterministic indexes, and can
delegate a resolved task to an external coding-agent CLI.

## System Boundaries

- `ahm` owns workflow installation, upgrades, validation, task/ADR lifecycle
  commands, generated indexes, and external agent orchestration.
- Target repositories own their source code and project-specific `AGENTS.md`.
  `ahm` does not patch source files, commit, push, create PRs, or run implicit
  git operations.
- External coding agents own the actual work performed through
  `ahm task work`. `ahm` validates state, builds the invocation, captures
  session IDs, and resumes sessions for review, completion, or commit handoff.

## Compatibility Surfaces

- CLI command names, flags, aliases, exit codes, help text, and output modes.
- Text, JSON, and plain output shapes, including validation finding codes.
- `.agents/ahm.json` metadata fields and version semantics.
- Task, research, ExecPlan, ADR, and generated index formats.
- Embedded templates under `internal/templates/workflow/`.
- Install and upgrade conflict behavior, including `AGENTS.md` create-only
  behavior.
- Atomic write guarantees and stale temp-file cleanup.
- External agent argument shapes, JSONL/session parsing, and resume behavior.
- Go module version, local tool versions, CI, and release packaging.

## Module Map

- `cmd/ahm/main.go`: CLI entrypoint.
- `internal/ahm/cli.go`: Cobra root command, global flags, command wiring.
- `internal/ahm/root.go`: repository root discovery.
- `internal/ahm/install.go`: `init`, `upgrade`, metadata, and template writes.
- `internal/ahm/status.go`: `status` and `doctor`.
- `internal/ahm/validation.go`: workflow, link, ADR, task, and project-doc
  validation.
- `internal/ahm/output.go`: shared text, JSON, and plain emitters.
- `internal/ahm/tasks.go`: task model, parsing, rendering, and ID helpers.
- `internal/ahm/task_commands.go`: `task` command wiring (`taskCommand` and
  `taskListCommand`).
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
  use repository-local locks under `.agents/.lock/`.
- Generated indexes are deterministic; sort output consistently and keep index
  generation centralized.
- Managed templates are updated through `init` and `upgrade`; project-owned
  records are changed through their lifecycle commands or documented manual
  source edits.
- `AGENTS.md` is create-only. Never treat an existing project `AGENTS.md` as
  a managed file that `upgrade` or `--force` can replace.
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
