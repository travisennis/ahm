# Glossary

This glossary maps `ahm` concepts to their definitions, implementing types, and
authoritative docs. Use it to resolve "what is the thing that does X?" without
reading the full specification or architecture doc first.

## Core Concepts

| Term | Definition | Implements | See Also |
| ---- | ---------- | ---------- | -------- |
| **Agent harness** | The `ahm` CLI itself: installs and manages repo-local `.agents` workflow files for tasks, research, ExecPlans, ADRs, indexes, and delegated coding-agent work. | `cmd/ahm/main.go` | [README](../../README.md), [spec](workflow-spec.md) |
| **Workflow** | The set of directories, source records, generated indexes, metadata, and command-exposed agent guidance that `ahm` manages. Includes task queues, research notes, ExecPlans, ADRs, and context instructions. | `.agents/` (on disk), `ahm context` | [spec §Workflow State](workflow-spec.md), [ARCHITECTURE.md](../../ARCHITECTURE.md) |
| **Task** | A unit of work with front matter (id, title, status, priority, effort, labels, dependencies), a Markdown body, and acceptance notes. Tasks move through statuses: `Open` → `Pending` → `In Progress` → `Completed` (or `Cancelled`, `Blocked`, `Tracking`). | `internal/ahm/tasks.go` (model), `internal/ahm/task_commands.go` (wiring) | [CLI task commands](../references/cli/task-commands.md), [task file format](../references/cli/task-file-format.md) |
| **ExecPlan** | An implementation plan for large or risky tasks. Lives in `.agents/exec-plans/active/` while in progress, moves to `completed/` when done. Must maintain Progress, Surprises & Discoveries, Decision Log, and Outcomes & Retrospective sections. | `internal/ahm/indexes.go` (indexes only; ExecPlan files are project-owned) | `ahm context plan`, [spec §Validation Scopes](workflow-spec.md) |
| **ADR** (Architecture Decision Record) | A durable technical decision captured as a MADR-profile Markdown file under `docs/adr/`. Has scalar front matter (status, date, decision-makers) and standard sections (Context, Decision, Consequences). | `internal/ahm/adrs.go` (model), `internal/ahm/adr_commands.go` (lifecycle) | `ahm context adr`, [ADR 009](../adr/009-madr-adr-management.md) |
| **Research note** | An investigation, evidence synthesis, or source summary under `.agents/.research/`. Organized into investigations, sources, and topics. | `internal/ahm/indexes.go` (index only; research files are project-owned) | `ahm context research` |
| **Generated index** | An ahm-owned Markdown file that aggregates records from source files. Never edit by hand; update source records and run `ahm index`. Includes task indexes, research indexes, ExecPlan indexes, and the ADR index. | `internal/ahm/indexes.go` | [spec §File Ownership Boundary](workflow-spec.md), [CLI `index` command](../references/cli/commands.md) |

## File Ownership

| Term | Definition | Implements | See Also |
| ---- | ---------- | ---------- | -------- |
| **Canonical workflow instructions** | Workflow guidance exposed by `ahm context` instead of copied into consumer repositories. Replaces previously installed workflow guide templates such as `.agents/TASKS.md` and `docs/adr/README.md`; agent skills remain installed templates. | `internal/ahm/context.go`, `internal/templates/templates.go` | [spec §File Ownership Boundary](workflow-spec.md), [ADR 011](../adr/011-expose-agent-instructions-through-context-command.md) |
| **Legacy managed template file** | An instruction file installed by older `ahm` versions and still tracked in `.agents/ahm.json`. `ahm upgrade` removes it when metadata proves ownership; locally modified copies are preserved as conflicts unless `--force`. | `internal/ahm/install.go` | [spec §File Ownership Boundary](workflow-spec.md), [upgrade guide](../guides/workflow-upgrades.md) |
| **Managed skill template** | An agent skill file under `.agents/skills/*/SKILL.md` installed by `ahm init` and updated by `ahm upgrade`. | `internal/templates/templates.go`, `internal/ahm/install.go` | [spec §File Ownership Boundary](workflow-spec.md) |
| **Project-owned file** | A file owned by the repository, not `ahm`. `ahm` reads these for validation and indexing but never overwrites them. Examples: task Markdown files, research notes, ExecPlans, ADR bodies. | — (project-authored) | [spec §File Ownership Boundary](workflow-spec.md) |
| **Project-owned AGENTS.md** | A repository's own agent instruction file. `ahm init`, `ahm upgrade`, and `--force` never create, overwrite, or remove it. `ahm agents suggestions` can print advisory agent instructions for maintainers to adapt manually. | `internal/ahm/agents.go` | [spec §File Ownership Boundary](workflow-spec.md), [ADR 011](../adr/011-expose-agent-instructions-through-context-command.md) |
| **Workflow source record** | Any project-owned file that `ahm` reads to generate indexes or validate state: task files, research notes, ExecPlans, and ADR bodies. Updated through `ahm` lifecycle commands or documented manual edits. | — (project-authored) | [spec §File Ownership Boundary](workflow-spec.md) |

## State and Validation

| Term | Definition | Implements | See Also |
| ---- | ---------- | ---------- | -------- |
| **`ahm.json`** | Workflow metadata file at `.agents/ahm.json`. Stores the installed template version, managed file hashes, and repository-scoped settings (`strict_acceptance`, `default_work_agent`). | `internal/ahm/install.go` | [spec §Workflow State](workflow-spec.md) |
| **Template version** | The embedded workflow template schema version (`internal/templates.Version`, a `const`). Advances only when templates under `internal/templates/workflow/` change. Distinct from the binary release version. | `internal/templates/templates.go` | [upgrade guide §Version Separation](../guides/workflow-upgrades.md) |
| **Binary version** | The `ahm` release version (`internal/version.Binary`, a `var` set by GoReleaser ldflags). Shown by `ahm --version`. Dev builds show `dev`. Distinct from the template version. | `internal/version/version.go` | [upgrade guide §Version Separation](../guides/workflow-upgrades.md) |
| **Validation scope** | A named set of checks run by `ahm status` and `ahm doctor`. `workflow` checks managed file consistency and task/ADR/ExecPlan state. `links` checks relative Markdown links within the managed workflow surface. `project-docs` is opt-in and checks project-level doc links. | `internal/ahm/validation.go` | [spec §Validation Scopes](workflow-spec.md), [CLI `status` command](../references/cli/commands.md) |
| **Validation finding** | A structured validation result with a code (e.g., `task_front_matter_malformed`), severity (error/warning/info), path, and message. Errors cause `status`/`doctor` to exit with code 1. | `internal/ahm/validation.go`, `internal/ahm/output.go` | [CLI global contract](cli/global-contract.md) |
| **Strict acceptance** | When `strict_acceptance: true` in `.agents/ahm.json`, `ahm task complete` fails if the acceptance section is missing, still contains `- [ ] TODO`, or has unchecked items. Overridable with `--force`. | `internal/ahm/task_acceptance.go` | [spec §Workflow State](workflow-spec.md) |
| **Acceptance notes** | The `## Acceptance Notes` section in a task Markdown file. Contains a checklist of completion criteria; seeded with `- [ ] TODO` by `ahm task create`. | `internal/ahm/task_acceptance.go` | [spec §Workflow State](workflow-spec.md), [ADR 005](../adr/005-task-acceptance-completion-checks.md) |
| **Task bucket** | A status-based subdirectory under `.agents/.tasks/`: `active/`, `completed/`, `cancelled/`. Task files live in the bucket matching their `status` front matter. Mismatches are reported by validation. | `internal/ahm/task_status.go`, `internal/ahm/validation.go` | [spec §Validation Scopes](workflow-spec.md) |
| **Dash sentinel** | The value `-` in task front matter meaning "not set" or "empty." Used for `labels`, `exec_plan`, and `depends_on`. Normalized during parsing; round-trips identically to an absent field. | `internal/ahm/tasks.go` | [spec §Dash Sentinel Semantics](workflow-spec.md) |
| **Canonical front matter order** | The fixed field order `ahm` uses when writing task front matter: id, title, status, priority, effort, labels, exec_plan, depends_on, then optional fields, then extra fields sorted alphabetically. Ensures deterministic diffs. | `internal/ahm/tasks.go` (`renderTask`) | [spec §Canonical Front Matter Order](workflow-spec.md) |
| **MADR profile** | The constrained subset of MADR 4.x used by `ahm` ADRs. Scalar front matter only (`key: value`), no block scalars/lists. Comma-separated values for list-like fields. Unknown front matter fields preserved on rewrite. | `internal/ahm/adrs.go` | `ahm context adr`, [ADR 009](../adr/009-madr-adr-management.md) |

## Safety and I/O

| Term | Definition | Implements | See Also |
| ---- | ---------- | ---------- | -------- |
| **Atomic write** | The crash-safe write strategy: content is written to a unique sibling `.tmp` file, synced, atomically renamed to the target path, and the parent directory synced. A crash before rename leaves the original intact. All managed writes use this path. | `internal/ahm/write.go` (`writeFileAtomic`) | [ADR 001](../adr/001-atomic-writes-and-concurrency.md), [spec §Atomic Write Guarantee](workflow-spec.md) |
| **Repository-local lock** | A filesystem lock under `.agents/.lock/` used to serialize `ahm task create` ID allocation across concurrent invocations. Narrower than broad advisory locking; adopted in ADR 010. | `internal/ahm/lock.go` | [ADR 010](../adr/010-task-create-id-allocation-lock.md), [spec §Workflow State](workflow-spec.md) |
| **Dry-run** | A global flag (`--dry-run`) that previews write operations without mutating disk or in-memory state. Supported by `init`, `upgrade`, `index`, `task create`, task status transitions, ADR lifecycle commands, and dependency commands. | `internal/ahm/cli.go` (flag), per-command handlers | [CLI global contract](cli/global-contract.md), [spec §Architectural Invariants](workflow-spec.md) |
| **Root detection** | The process of finding the target repository root. Walks upward from CWD looking for `.git` or `.agents/ahm.json`. Overridable with `--root`. `init`, `upgrade`, and `agents suggestions` are lenient and can run in any directory. | `internal/ahm/root.go` | [CLI global contract](cli/global-contract.md) |
| **Compatibility surface** | A stable contract `ahm` guarantees not to break without an explicit version change. Includes CLI commands/flags/exit codes, output formats, `.agents/ahm.json` schema, workflow file formats, generated index structure, embedded templates, atomic writes, root detection, validation codes, external agent orchestration, and release semantics. | — (contract, not a type) | [`AGENTS.md`](../../AGENTS.md) (listed at top), [ARCHITECTURE.md](../../ARCHITECTURE.md) |

## Agent Delegation

| Term | Definition | Implements | See Also |
| ---- | ---------- | ---------- | -------- |
| **Agent delegation** | The `ahm task work <id>` command: validates task state, builds an invocation for the selected external coding-agent CLI, and invokes it from the repository root. Review and commit run by default; use `--no-review` / `--no-commit` to opt out. | `internal/ahm/task_work.go`, `internal/ahm/task_agents.go` | [CLI task commands](../references/cli/task-commands.md), [ADR 006](../adr/006-task-work-agent-delegation.md) |
| **Session** | An agent invocation session identified by a session ID captured from the agent's stderr output. Used to resume the agent for review, completion, or commit handoff. | `internal/ahm/task_session.go` | [ADR 006](../adr/006-task-work-agent-delegation.md), [ADR 008](../adr/008-delegated-task-work-commit-handoff.md) |
| **Session ID** | A unique identifier emitted by the external agent CLI on stderr (e.g., `session started: <id>`). Parsed by ahm to enable session resume. | `internal/ahm/task_parsers.go` | [testing guide](../guides/testing.md) |
| **Golden transcript** | A committed fixture under `internal/ahm/testdata/agents/` capturing real agent CLI output (JSONL or text). Used by parser unit tests to validate parsing against real schemas. Refreshed with `just capture-agent-fixtures`. | `internal/ahm/testdata/agents/` | [testing guide](../guides/testing.md), `internal/ahm/testdata/agents/README.md` |
| **Smoke test** | A live end-to-end test (`just smoke-agents`) that drives each installed agent through `ahm task work` in a throwaway repo. Required after changes to agent arg builders, parsers, or orchestration. | `internal/ahm/task_work_smoke_test.go` | [testing guide](../guides/testing.md) |
