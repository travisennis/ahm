# Glossary

This glossary maps `ahm` concepts to their definitions, implementing types, and
authoritative docs. Use it to resolve "what is the thing that does X?" without
reading the full specification or architecture doc first.

## Core Concepts

| Term | Definition | Implements | See Also |
| ---- | ---------- | ---------- | -------- |
| **Agent harness** | The `ahm` CLI itself: manages repo-local workflow records for tasks and ADRs, their generated indexes, and workflow metadata under `.ahm/`. | `cmd/ahm/main.go` | [README](../../README.md), [spec](workflow-spec.md) |
| **Workflow** | The set of directories, source records, generated indexes, and metadata that `ahm` manages: task queues and ADRs. Workflow instructions are project-owned prose, not part of the managed workflow. | `.ahm/` record paths, `ahm prime` | [spec §Workflow State](workflow-spec.md), [ARCHITECTURE.md](../../ARCHITECTURE.md) |
| **Task** | A unit of work with front matter (id, title, status, priority, effort, labels, dependencies), a Markdown body, and acceptance notes. Tasks move through statuses: `Open` → `Pending` → `In Progress` → `Completed` (or `Cancelled`, `Blocked`, `Tracking`). | `internal/ahm/tasks.go` (model), `internal/ahm/task_commands.go` (wiring) | [CLI task commands](../references/cli/task-commands.md), [task file format](../references/cli/task-file-format.md) |
| **ExecPlan** | A project-owned design plan for large or risky tasks, stored under `docs/exec-plans/active/` while in progress and moved to `docs/exec-plans/completed/` when done. `ahm` neither reads nor validates it. | — (project-authored) | [ExecPlan workflow](../workflow/exec-plans.md) |
| **ADR** (Architecture Decision Record) | A durable technical decision captured as a MADR-profile Markdown file under `docs/adr/`. Has scalar front matter (status, date, decision-makers) and standard sections (Context, Decision, Consequences). | `internal/ahm/adrs.go` (model), `internal/ahm/adr_commands.go` (lifecycle) | [ADR workflow](../workflow/adrs.md), [ADR 009](../adr/009-madr-adr-management.md) |
| **Generated index** | An ahm-owned Markdown file that aggregates records from source files. Never edit by hand; update source records and run `ahm index`. Includes the task indexes and the ADR index. | `internal/ahm/indexes.go` | [spec §File Ownership Boundary](workflow-spec.md), [CLI `index` command](../references/cli/commands.md) |

## File Ownership

| Term | Definition | Implements | See Also |
| ---- | ---------- | ---------- | -------- |
| **Retired managed file** | A file an older `ahm` version installed and tracked ownership for, and which this version neither creates, inspects, overwrites, nor removes. `ahm init` discards only the stale ownership hash. | `internal/ahm/install.go` | [spec §File Ownership Boundary](workflow-spec.md) |
| **Project-owned file** | A file owned by the repository, not `ahm`. `ahm` may read these but never overwrites them. Examples: project `AGENTS.md`, standing prompt files, ADR bodies, and project documentation. | — (project-authored) | [spec §File Ownership Boundary](workflow-spec.md) |
| **Project-owned AGENTS.md** | A repository's own agent instruction file. `ahm init` and `--force` never create, overwrite, or remove it, and `ahm` never prints a snippet to paste into it. | — (project-authored) | [spec §File Ownership Boundary](workflow-spec.md), [ADR 014](../adr/014-replace-managed-skills-with-command-based-procedures.md) |
| **Workflow source record** | A source file that `ahm` reads to generate indexes or validate state: task files and ADR bodies. Task records live under `.ahm/tasks/` and ADRs under `docs/adr/`. Updated through `ahm` lifecycle commands or documented manual edits. | `internal/ahm/workflow_paths.go` (record roots), record parsers by type | [spec §File Ownership Boundary](workflow-spec.md) |

## State and Validation

| Term | Definition | Implements | See Also |
| ---- | ---------- | ---------- | -------- |
| **Workflow metadata** | Workflow configuration committed at `.ahm/config.json`. Stores managed file hashes and repository-scoped settings. Unknown fields are preserved through reads and writes, and obsolete ahm-owned keys are dropped when `ahm init` reconciles it. | `internal/ahm/install.go` | [spec §Workflow State](workflow-spec.md) |
| **Binary version** | The `ahm` release version (`internal/version.Binary`, a `var` set by GoReleaser ldflags). Shown by `ahm --version`. Dev builds show `dev`. | `internal/version/version.go` | [release docs](../release.md) |
| **Validation scope** | A named set of checks run by `ahm status` and `ahm doctor`. `workflow` checks managed file consistency and task/ADR state. `links` checks relative Markdown links within tasks, ADRs, and their generated indexes. | `internal/ahm/validation.go` | [spec §Validation Scopes](workflow-spec.md), [CLI commands](../references/cli/commands.md) |
| **Validation finding** | A structured validation result with a code (e.g., `task_front_matter_malformed`), severity (error/warning/info), path, and message. Errors cause `status`/`doctor` to exit with code 1. | `internal/ahm/validation.go`, `internal/ahm/output.go` | [CLI global contract](cli/global-contract.md) |
| **Strict acceptance** | When `strict_acceptance: true` in workflow metadata, `ahm task complete` fails if the acceptance section is missing, still contains `- [ ] TODO`, or has unchecked items. Overridable with `--force`. | `internal/ahm/task_acceptance.go` | [spec §Workflow State](workflow-spec.md) |
| **Acceptance notes** | The `## Acceptance Notes` section in a task Markdown file. Contains a checklist of completion criteria; seeded with `- [ ] TODO` by `ahm task create`. | `internal/ahm/task_acceptance.go` | [spec §Workflow State](workflow-spec.md), [ADR 005](../adr/005-task-acceptance-completion-checks.md) |
| **Task bucket** | A status-based subdirectory under the current task root: `active/`, `completed/`, `cancelled/`. Task files live in the bucket matching their `status` front matter. Mismatches are reported by validation. | `internal/ahm/task_status.go`, `internal/ahm/validation.go` | [spec §Validation Scopes](workflow-spec.md) |
| **Dash sentinel** | The value `-` in task front matter meaning "not set" or "empty." Used for `labels` and `depends_on`. Normalized during parsing; round-trips identically to an absent field. | `internal/ahm/tasks.go` | [spec §Dash Sentinel Semantics](workflow-spec.md) |
| **Canonical front matter order** | The fixed field order `ahm` uses when writing task front matter: id, title, status, priority, effort, labels, depends_on, then optional fields, then extra fields sorted alphabetically. Ensures deterministic diffs. | `internal/ahm/tasks.go` (`renderTask`) | [spec §Canonical Front Matter Order](workflow-spec.md) |
| **MADR profile** | The constrained subset of MADR 4.x used by `ahm` ADRs. Scalar front matter only (`key: value`), no block scalars/lists. Comma-separated values for list-like fields. Unknown front matter fields preserved on rewrite. | `internal/ahm/adrs.go` | [ADR workflow](../workflow/adrs.md), [ADR 009](../adr/009-madr-adr-management.md) |

## Safety and I/O

| Term | Definition | Implements | See Also |
| ---- | ---------- | ---------- | -------- |
| **Atomic write** | The crash-safe write strategy: content is written to a unique sibling `.tmp` file, synced, atomically renamed to the target path, and the parent directory synced. A crash before rename leaves the original intact. All managed writes use this path. | `internal/ahm/write.go` (`writeFileAtomic`) | [ADR 001](../adr/001-atomic-writes-and-concurrency.md), [spec §Atomic Write Guarantee](workflow-spec.md) |
| **Repository-local lock** | A filesystem lock under `.ahm/.lock/` used to serialize `ahm task create` ID allocation across concurrent invocations. Narrower than broad advisory locking; adopted in ADR 010. | `internal/ahm/lock.go` | [ADR 010](../adr/010-task-create-id-allocation-lock.md), [spec §Workflow State](workflow-spec.md) |
| **Dry-run** | A global flag (`--dry-run`) that previews supported write operations without mutating disk, metadata, refs, or in-memory state that affects later operations. Supported by `init`, `index`, task create/status/dependency commands, and ADR lifecycle commands. | `internal/ahm/cli.go` (flag), per-command handlers | [CLI global contract](cli/global-contract.md), [spec §Architectural Invariants](workflow-spec.md) |
| **Root detection** | The process of finding the target repository root. Walks upward from CWD looking for `.git` or `.ahm/config.json`. A repository on the retired `.agents/ahm.json` layout is refused with the final v1 release named. Overridable with `--root`. `init` is lenient and can run in any directory. | `internal/ahm/root.go` | [CLI global contract](cli/global-contract.md) |
| **Compatibility surface** | A stable contract `ahm` guarantees not to break without an explicit version change. Includes CLI commands/flags/exit codes, output formats, the `.ahm/config.json` schema, workflow file formats, generated index structure, atomic writes, root detection, validation codes, and release semantics. | — (contract, not a type) | [`AGENTS.md`](../../AGENTS.md) (listed at top), [ARCHITECTURE.md](../../ARCHITECTURE.md) |
