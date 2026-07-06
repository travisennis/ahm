# Implement Ref-Backed Workflow Record Storage

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

If `ahm context plan` guidance is used for the work, this document must be maintained in accordance with that command output.

## Purpose / Big Picture

After this change, a solo developer or coding agent can use `ahm` without committing task, research, and ExecPlan churn to the project branch. Workflow records remain plain Markdown under tool-owned `.ahm/` paths while active, but `ahm` can back them up and sync them through a GitHub-hosted private ref named `refs/ahm/records`. A coding agent can begin a session by running `ahm prime`, which synchronizes records when possible, regenerates indexes, validates workflow state, and prints the current backlog.

This matters because tasks, scratch research, and draft ExecPlans are working artifacts. Their durable outcomes belong in ADRs, design docs, and project docs; the ceremony of creating and completing them should not clutter normal project history. At the same time, an active backlog must survive laptop loss or machine changes.

## Progress

- [x] (2026-06-30 15:10Z) Created research note `.agents/.research/topics/records-storage-via-git-refs.md` and revised it around gitignored working records plus private-ref durability.
- [x] (2026-06-30 15:10Z) Smoke-tested GitHub custom refs with a disposable private repository.
- [x] (2026-06-30 15:15Z) Created proposed ADR `docs/adr/013-use-ref-backed-workflow-record-storage.md`.
- [x] (2026-06-30 15:15Z) Created tracking task 138 and implementation tasks 137, 139, 140, 141, 142, 143, 144, and 145.
- [x] (2026-07-06 00:00Z) Revised ADR 013 around the accepted `.ahm/` namespace, GitHub-only initial support, generated-index exclusion, opt-in migration, `ahm prime`, and deferred task-ID redesign.
- [x] (2026-07-06 00:00Z) Accepted ADR 013 via `ahm adr accept 013`.
- [x] (2026-07-06 00:00Z) Implemented records metadata and storage-mode model for `.ahm/config.json` read compatibility, legacy `.agents/ahm.json` fallback, unknown-field preservation, storage defaults, and dynamic metadata validation paths.
- [ ] Implement private-ref snapshot and materialization plumbing.
- [ ] Add `ahm records` command surface.
- [ ] Add migration workflow for existing committed records.
- [ ] Add `ahm prime` and stale-state reporting.
- [ ] Integrate ref-backed records with task, research, and ExecPlan write paths.
- [ ] Update docs, tests, and agent guidance.

## Surprises & Discoveries

- Observation: GitHub accepts custom refs under `refs/ahm/*`, but normal clone does not fetch them.
  Evidence: The 2026-06-30 smoke test pushed `refs/ahm/records`, cloned the repo normally, observed no local `refs/ahm/*`, then explicitly fetched `refs/ahm/records` and verified the fetched commit matched the pushed commit.
- Observation: A local private ref alone does not protect against laptop loss.
  Evidence: The design requires an explicit remote push/sync path through `ahm records sync` or `ahm prime`; otherwise the only copy may remain local.
- Observation: The initial remote support target can be GitHub-only.
  Evidence: Bitbucket Data Center is not a blocking requirement for the first ADR or implementation.
- Observation: Current `init` and unmigrated `upgrade` should keep writing legacy `.agents/ahm.json` until migration creates `.ahm/config.json`.
  Evidence: Task 140 added read/write preference tests showing `writeMetadata` writes `.agents/ahm.json` by default and switches to `.ahm/config.json` only when that file already exists.

## Decision Log

- Decision: Use gitignored working files plus `refs/ahm/records` as the backup/sync layer.
  Rationale: This keeps workflow records out of normal branch history while preserving recoverability through the existing GitHub remote. The original draft used `.agents/` as the working namespace; the 2026-07-02 namespace decision below moves the working files to `.ahm/`.
  Date/Author: 2026-06-30, Travis Ennis and Codex.
- Decision: Move ahm-managed state from `.agents/` to `.ahm/` as part of the ref-backed records migration.
  Rationale: `.agents/` is project-owned agent-facing content that should remain committed. `.ahm/` gives `ahm` a tool-owned namespace for workflow records, generated indexes, sync state, and committed config, and lets `ahm` manage an internal `.ahm/.gitignore` without touching the consumer repository's root `.gitignore`.
  Date/Author: 2026-07-02, Travis Ennis. Recorded in ADR 013 revision on 2026-07-06 by Codex.
- Decision: Make `ahm prime` the agent-facing startup command.
  Rationale: `ahm` is primarily used with coding agents, so the safest sync model is an explicit command agents are instructed to run before each session.
  Date/Author: 2026-06-30, Travis Ennis and Codex.
- Decision: Layer ref-backed sync onto `ahm prime` after task 156b introduces the read-only briefing command.
  Rationale: The `prime` command is the single session-start hook. It must remain fast, idempotent, and offline-tolerant; failed records sync should warn instead of blocking the live briefing, with a `--no-sync` escape hatch for offline or hook-constrained environments.
  Date/Author: 2026-07-02, Travis Ennis. Recorded in ADR 013 revision on 2026-07-06 by Codex.
- Decision: Defer task ID redesign.
  Rationale: Solo development is the primary target. Sequential IDs remain acceptable for the first implementation; random or hash-like stable IDs can be revisited if active multi-clone or team task creation becomes a supported target.
  Date/Author: 2026-06-30, Travis Ennis and Codex.
- Decision: Treat GitHub as the initial supported remote and defer Bitbucket/Data Center probes.
  Rationale: GitHub has been smoke-tested successfully and Bitbucket is not currently a blocked requirement.
  Date/Author: 2026-06-30, Travis Ennis and Codex.
- Decision: For task 140, prefer `.ahm/config.json` only when it already exists; otherwise preserve legacy `.agents/ahm.json` writes.
  Rationale: This lets migration introduce the new committed config path explicitly without changing fresh install or unmigrated upgrade behavior in the metadata-model task.
  Date/Author: 2026-07-06, Codex.

## Outcomes & Retrospective

## Context and Orientation

`ahm` is a Go CLI that manages repository-local workflow state. Today, task files live under `.agents/.tasks/`, research notes under `.agents/.research/`, and ExecPlans under `.agents/exec-plans/`. ADRs live under `docs/adr/`. Generated indexes are produced by `internal/ahm/indexes.go` and must not be edited by hand.

Current behavior treats workflow source records as project-owned files that can be committed to the consumer repository. That behavior is documented in `docs/references/workflow-spec.md` and summarized in `ARCHITECTURE.md`. ADR 013 changes the future boundary: ahm-managed workflow records, generated indexes, sync state, and configuration move to tool-owned `.ahm/`; project-owned agent-facing content remains committed under `.agents/` and is read but not managed by `ahm`. The current architecture also says `ahm` does not run implicit git operations. Ref-backed workflow records intentionally narrow that boundary, so ADR 013 must be accepted before implementation.

Important files and modules:

- `docs/adr/013-use-ref-backed-workflow-record-storage.md` records the architectural decision.
- `.agents/.research/topics/records-storage-via-git-refs.md` records the design research and GitHub smoke-test evidence.
- `internal/ahm/cli.go` wires top-level commands.
- `internal/ahm/install.go` reads and writes current `.agents/ahm.json` metadata and handles init/upgrade behavior. This plan moves committed configuration to `.ahm/config.json` while preserving legacy behavior when the new config is absent.
- `internal/ahm/tasks.go`, `internal/ahm/task_create.go`, `internal/ahm/task_status.go`, and related task files write task records.
- `internal/ahm/indexes.go` renders generated indexes.
- `internal/ahm/context.go` produces the existing session briefing.
- `internal/ahm/status.go` and `internal/ahm/validation.go` report workflow health.
- `internal/ahm/write.go` provides atomic writes.
- `docs/cli.md`, `docs/references/workflow-spec.md`, `docs/guides/workflow-upgrades.md`, and `ARCHITECTURE.md` document compatibility surfaces.

Definitions:

- A private ref is a Git reference that is not a branch or tag. For this work, the namespace is `refs/ahm/*`, with record snapshots stored at `refs/ahm/records`.
- Materialization means taking the tree stored in `refs/ahm/records` and writing its files back to the normal `.ahm/` working paths.
- Staleness means the working records, local records ref, or remote records ref differ in a way the user or agent should know before making decisions.

## Plan of Work

First, finish the decision layer. Review ADR 013 and this ExecPlan. Accept ADR 013 only when the storage boundary, GitHub-only initial support, migration policy, generated-index exclusion, and `ahm prime` behavior are settled. Task 137 covers this step.

Next, add the metadata model. Introduce `.ahm/config.json` for committed ahm configuration and keep parsing legacy `.agents/ahm.json` so existing repositories preserve current behavior until migration. The model must represent legacy committed records, gitignored local records, and ref-backed records. The initial fields should be conservative, such as `store_mode`, `records_ref`, `records_remote`, and last-sync metadata. Absence of new configuration must preserve current committed-record behavior. This is task 140.

Then build private-ref plumbing behind internal helpers before exposing CLI commands. The helpers should create a Git tree from the selected workflow record files under `.ahm/`, create a commit object, update `refs/ahm/records`, fetch the remote ref, compare local and remote refs, and materialize a ref tree back to `.ahm/`. These helpers must be covered with tests that prove they do not move `HEAD`, stage files, write the index, or mutate branches. This is task 139.

After the plumbing exists, add the `ahm records` command surface. The first surface should include `status`, `pull`, `push`, `sync`, and `doctor` or equivalent diagnostics. `sync` should be explicit and network-capable. It should fail with precise diagnostics when no GitHub remote exists, credentials fail, or the remote rejects the ref. This is task 141.

Add migration after the command surface can report status. `ahm records migrate` should be opt-in and dry-run-previewed. It should move ahm-managed records, generated indexes, sync state, and configuration into `.ahm/`, install or update `.ahm/.gitignore` so records and indexes are ignored while `.ahm/config.json` remains committed, seed `refs/ahm/records`, set metadata, and print the user-run `git rm -r --cached .agents/.tasks .agents/.research .agents/exec-plans .agents/ahm.json` command instead of running it silently. It must not gitignore or snapshot project-owned `.agents/` content such as `.agents/prompt.md`. This is task 142.

Add the records layer to `ahm prime` after records sync exists and after task 156b has introduced the read-only briefing command. It should sync or fetch records when allowed, materialize local records, regenerate local indexes, run workflow validation, and print a compact backlog briefing. The output should include ready/open/blocked task counts, high-priority ready tasks, stale or unsynced warnings, active ExecPlans, recent research notes, and suggested next commands. Because `prime` is intended for session-start hooks, sync failures must degrade to warnings, and a `--no-sync` escape hatch should be available. This is task 143.

Integrate ref-backed storage with task, research, and ExecPlan write paths. Existing commands must keep working in legacy mode. In ref-backed mode, source record writes should update local `.ahm/` files and local snapshot state without including generated indexes in the records ref. External-agent commit handoff must not sweep gitignored task files into project commits. This is task 144.

Finally, update documentation, tests, and agent guidance. Document the new CLI commands, metadata, safety boundary, migration flow, GitHub-only initial remote support, and `ahm prime` instruction. Add focused tests for metadata parsing, dry-run no-write behavior, git boundary behavior, generated-index exclusion, migration previews, and command output. This is task 145.

## Concrete Steps

Work from the repository root:

    cd /Users/travisennis/Projects/ahm

Inspect the current decision and plan:

    ahm adr show 013
    ahm task show 138
    sed -n '1,260p' .agents/exec-plans/active/138-ref-backed-workflow-records.md

When ADR 013 is ready, accept it:

    ahm adr accept 013

Start the first implementation task only after accepting the ADR:

    ahm task start 140

During implementation, keep each task focused. Use the task dependencies to sequence work. Do not mark the tracker task 138 complete until all child tasks are complete and this ExecPlan has moved to `.agents/exec-plans/completed/`.

## Validation and Acceptance

The full feature is accepted when a repository can opt into ref-backed records and demonstrate this behavior:

1. In a GitHub-backed repository, `ahm records migrate --dry-run` previews the records that would move, the ref that would be seeded, metadata changes, `.gitignore` changes, and the exact `git rm --cached` command the user must run.
2. After migration, task/research/ExecPlan records live under `.ahm/`, are gitignored, and do not appear in normal branch commits or `git status --short` as tracked changes.
3. `ahm records sync` pushes `refs/ahm/records` to GitHub and reports the synced commit.
4. A normal fresh clone does not fetch `refs/ahm/records` by default.
5. Running `ahm prime` in that clone fetches/materializes the records when network sync succeeds, regenerates indexes, validates workflow state, and prints backlog status; when sync fails because the network or remote is unavailable, it prints a warning and still returns the local briefing.
6. Generated indexes are regenerated locally and are not stored in the records ref.
7. Tests prove that routine records commands do not move `HEAD`, create commits on project branches, stage files, write the index, or patch project source code.

Run focused tests after each milestone, then the project verification command from `CONTRIBUTING.md` before completing implementation tasks. If a task changes external agent orchestration or commit handoff, also follow `docs/guides/testing.md`.

## Idempotence and Recovery

All migration and sync commands must be safe to retry. If a local snapshot succeeds but a remote push fails, `ahm records status` should show that the local records ref is ahead of the remote. If materialization finds local unsnapshotted edits, it should stop or produce an explicit conflict report rather than overwriting silently. If migration has been started but the user has not run `git rm --cached`, the command should report that state clearly.

Rollback from opt-in migration should be possible by removing storage metadata, restoring `.agents/ahm.json` if needed, re-adding `.agents/.tasks`, `.agents/.research`, and `.agents/exec-plans` to the project branch, removing or ignoring the `.ahm/` state directory, and optionally deleting `refs/ahm/records`.

## Artifacts and Notes

GitHub smoke-test evidence from 2026-06-30, using a disposable private repository:

    pushed refs/ahm/records successfully
    normal clone: no refs/ahm/* refs
    normal clone: no .agents records from main
    explicit fetch: refs/ahm/records fetched successfully
    explicit fetch: fetched commit matched pushed commit
    GitHub API: listed refs/ahm/records
    delete probe: refs/ahm/delete-probe create/delete succeeded

The disposable repository does not need to remain available. The important evidence is the observed GitHub behavior.

## Interfaces and Dependencies

Prefer internal helpers over shelling out directly from command handlers. Command handlers should parse flags, validate options, call focused helpers, and emit text/JSON/plain output through existing output conventions.

Expected internal concepts include:

- A records storage configuration derived from `.ahm/config.json`, with legacy `.agents/ahm.json` compatibility before migration.
- A record-file selector that includes task, research, and ExecPlan source records under `.ahm/` but excludes generated indexes and project-owned `.agents/` content.
- Git plumbing helpers for creating record snapshots and fetching/pushing `refs/ahm/records`.
- A records status model that can express missing local records, local working edits, local-ref ahead/behind, remote-ref ahead/behind, last successful sync time, and unsupported remote.
- A migration planner that can produce dry-run and real plans without silently touching the Git index.

Do not introduce a database. Do not change task ID format in the first implementation. Do not support non-GitHub remotes as an initial requirement, but make remote-rejection diagnostics clear enough that future support can be added without redesigning the command surface.

Revision note, 2026-07-06: Updated the plan to match ADR 013's accepted `.ahm/` namespace direction. The previous draft used `.agents/` as the ref-backed record working directory; this revision treats `.agents/` as committed project-owned agent content and moves ahm-managed records, indexes, sync state, and config under `.ahm/`.
