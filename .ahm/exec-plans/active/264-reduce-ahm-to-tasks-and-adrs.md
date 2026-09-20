# Reduce ahm to a tasks-and-ADRs records CLI

This ExecPlan is a living document. The sections `Progress`, `Surprises &
Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to
date as work proceeds. This document is maintained in accordance with the
`ahm context plan` guidance; rerun that command before revising the plan.

Tracker: task 264; children 264a through 264g sequence the milestones. One
milestone maps to exactly one child task.

## Purpose / Big Picture

Today `ahm` is a single Go binary that manages six command groups over four
record families, and one of its command groups, `ahm task work`, runs an
entire external coding agent on your behalf. After this work, `ahm` does two
things: it manages tasks and it manages ADRs. It reads and writes those
records, keeps deterministic indexes of them, and validates their integrity.
It no longer explains how to work, no longer delegates work to another CLI,
and no longer stores research notes or execution plans.

You can see it working by running the binary after the change:

    $ ahm --help
    Available Commands:
      adr         Manage architecture decision records
      doctor      Check environment and workflow health
      index       Regenerate task and ADR indexes
      init        Create or reconcile ahm-owned workflow state
      prime       Regenerate indexes and report workflow state
      status      Report workflow record counts and validation findings
      task        Manage tasks
      version     Print version information

Compare that with `ahm --help` today, which also lists `audit`, `context`,
`onboard`, `records`, and `upgrade`, and whose `task` group lists `groom`,
`migrate`, and `work`. Nothing about the task and ADR lifecycle changes:
`ahm task create`, `ahm task accept`, `ahm task start`, `ahm task complete`,
`ahm task dep add`, `ahm adr create`, and `ahm adr supersede` behave exactly
as they do now, including their flags, output modes, and exit codes.

The decision of record is `docs/adr/022-reduce-ahm-to-a-tasks-and-adrs-records-cli.md`,
which is checked in. Read it before starting; it states what is removed, what
survives, and why. This plan is the delivery sequence for that decision.

## Context and Orientation

`ahm` is a Go module at the repository root (`go.mod`, module
`github.com/travisennis/ahm`) that builds one binary from
`cmd/ahm/main.go`. Essentially all behavior lives in one package,
`internal/ahm/`, with a second small package `internal/templates/` that embeds
the managed file templates under `internal/templates/workflow/`, and
`internal/version/version.go` that holds the version string the release build
injects. There are about 12,300 lines of non-test Go and about 18,600 lines of
test Go.

Workflow records are ordinary committed Markdown files in the consuming
repository. Tasks live under `.ahm/tasks/{active,completed,cancelled}/`, ADRs
under `docs/adr/`, research notes under `.ahm/research/`, and ExecPlans under
`.ahm/exec-plans/{active,completed}/`. Each family has a generated `index.md`
that is derived data: `ahm index` and `ahm prime` regenerate it, and it must
never be edited by hand. Configuration lives in `.ahm/config.json`. Settings
are committed so every clone and CI sees the same values. `.ahm/.gitignore`
ignores the generated indexes that are not part of the published
documentation, such as the task bucket indexes.

Two rules constrain every edit in this plan. First, `AGENTS.md` at the
repository root is project-owned: no `ahm` code path may create, replace, or
remove it. Second, `ahm` never performs implicit Git operations: it does not
commit, stage, push, move `HEAD`, or modify branches. Every branch, commit,
and pull request in this plan is a human or agent action performed with
ordinary `git`, not with `ahm`.

The commands used to check work are declared in `justfile`:

    just ci            # fmt-check, tidy-check, vet, test-race, lint, vuln, docs-md-lint, build, release-check
    just test          # go test ./...
    just lint          # golangci-lint run
    just docs-md-lint  # markdownlint over every Markdown file

`just ci` is the gate for every milestone. `ahm doctor` reports validation
findings and environment health for the working repository, and `ahm prime`
regenerates indexes and prints the current state.

A "record" is one Markdown file with YAML front matter (`---` delimited key
and value lines) followed by a body. A "record family" is a group of records
with the same schema and lifecycle: tasks, ADRs, research notes, and
ExecPlans are the four families today. A "managed file" is a file whose
content `ahm` owns and hashes so it can update it later without clobbering a
user's edits; the metadata field `files` in `.ahm/config.json` stores those
hashes.

The repository requires a feature branch for all work, never a commit on
`master`, and a pull request with green CI to merge. Git hooks installed by
`prek` (a Rust reimplementation of `pre-commit`) enforce this locally:
`scripts/hooks/require-feature-branch.sh` refuses a commit on `master`, and
`scripts/hooks/go-*.sh` run formatting, tidy, test, and lint checks when a
`*.go` file is staged. Note the exact condition: those Go hooks exit 0
immediately unless `git diff --cached --name-only` lists a `.go` file, so
`prek run --all-files` proves nothing about them; to exercise them you must
stage a `.go` change and run `prek run`.

## Progress

- [x] (2026-09-20) ADR 022 written as `proposed` on branch
      `feat/022-reduce-ahm-to-tasks-and-adrs`.
- [x] (2026-09-20) Tracker task 264 and children 264a-264g created with
      acceptance notes.
- [x] (2026-09-20) This ExecPlan written and linked from tracker 264's
      `exec_plan` front-matter field.
- [x] (2026-09-20) ADR 022 accepted. Owner decisions recorded in this plan:
      design plans live in `docs/exec-plans/`, and the `ahm context`
      procedures are salvaged to `docs/workflow/{tasks,exec-plans,adrs}.md`
      during milestone 4.
- [x] (2026-09-20) Unrelated fix found while writing the ADR: `renderADR`
      appended a trailing blank line, so every created ADR failed
      `just docs-md-lint` with MD012. Fixed in `internal/ahm/adrs.go` with a
      regression test; task 265 completed.
- [ ] The nine ADRs that ADR 022 replaces marked superseded (`ahm adr
      supersede`).
- [ ] Milestone 1 (264g) complete: moot backlog tasks cancelled, tracker 263
      closed.
- [ ] Milestone 2 (264a) complete: delegation surface deleted.
- [ ] Milestone 3 (264b) complete: research and ExecPlans retired.
- [ ] Milestone 4 (264c) complete: procedure channel removed.
- [ ] Milestone 5 (264d) complete: install collapsed to one idempotent
      `ahm init`.
- [ ] Milestone 6 (264e) complete: documentation and instructions rewritten.
- [ ] Milestone 7 (264f) complete: v2.0.0 released.

## Surprises & Discoveries

- Observation: `ahm` has exactly one consumer repository. A search of the
  home directory finds no `.agents/ahm.json` anywhere and no `.ahm/` directory
  outside this repository.
  Evidence: `find ~ -maxdepth 4 -name ahm.json` and
  `find ~ -maxdepth 4 -type d -name .ahm` return only this repository's paths.

- Observation: `prek run --all-files` cannot validate the Go hooks in this
  repository, because each of `scripts/hooks/go-fmt-check.sh`,
  `go-tidy-check.sh`, `go-test.sh`, and `go-lint.sh` returns success without
  running anything unless a `.go` file is staged. A reader who takes the
  ExecPlan requirement from an earlier plan at face value will believe the
  hooks passed when nothing ran.
  Evidence: `prek run --all-files --verbose` reports `duration: 0.01s` for
  every Go hook; staging an appended comment in
  `internal/version/version.go` and running `prek run` shows real durations
  and real `go test ./...` and `golangci-lint` output.

- Observation: three modules reported as outdated by `go list -m -u all` are
  graph-only. Their `go.sum` entries end in `/go.mod`, meaning their packages
  are never compiled, so there is nothing to upgrade in this module.
  Evidence: `grep -E "go-md2man|go.yaml.in|check.v1" go.sum` shows only
  `/go.mod` hashes.

## Decision Log

- Decision: implement ADR 022 exactly as accepted, without retaining aliases
  for removed commands.
  Rationale: an alias preserves the product boundary the decision rejects, and
  the only consumer is this repository.
  Date/Author: 2026-09-20, Travis Ennis.

- Decision: keep generated indexes for the two surviving families.
  Rationale: they are deterministic derived data that make records browsable
  in an editor or a diff without running `ahm`, and their formats are a
  declared compatibility surface.
  Date/Author: 2026-09-20, Travis Ennis.

- Decision: skip an ExecPlan for nothing; the repository requires one for
  large, cross-cutting work and this plan satisfies that requirement. Whether
  a design document is required is a repository rule, not something the tool
  enforces, and `ahm` will no longer know what an ExecPlan is after
  milestone 3.
  Rationale: the repository owns process; the tool owns records.
  Date/Author: 2026-09-20, Travis Ennis.

- Decision: after milestone 3, design plans for this repository live as
  project-owned files under `docs/exec-plans/active/` and
  `docs/exec-plans/completed/`, and this plan file moves there when the work
  completes.
  Rationale: the repository owns process and documentation, so plans belong in
  its published docs alongside the rest of its design history, where a reader
  finds them without knowing anything about `ahm`, and where `ahm` holds no
  opinion about their structure or lifecycle.
  Date/Author: 2026-09-20, Travis Ennis.

- Decision: the procedures that `ahm context` currently prints are salvaged
  into project-owned documents before the command is deleted: `ahm context
  task` becomes `docs/workflow/tasks.md`, `ahm context plan` becomes
  `docs/workflow/exec-plans.md`, and `ahm context adr` becomes
  `docs/workflow/adrs.md`. The research procedure is not salvaged because that
  record family is removed.
  Rationale: the prose is worth keeping; only the tool's authority over it was
  the problem. The salvaged documents are rewrites, not copies: template
  variables become concrete paths, instructions that describe removed
  behavior (regenerating plan indexes, setting a task's `exec_plan` field) are
  deleted, and the plan document points at `docs/exec-plans/` rather than the
  retired `.ahm/exec-plans/`.
  Date/Author: 2026-09-20, Travis Ennis.

- Decision: milestone order is 264g, 264a, 264b, 264c, 264d, 264e, 264f, with
  each milestone on its own `feat/<slug>` branch and its own pull request.
  Rationale: the deletion milestones are independent and each leaves the tree
  green, so a failure in one is isolated to one branch; the documentation
  rewrite must follow the code it describes; the release must come last.
  Date/Author: 2026-09-20, Travis Ennis.

## Outcomes & Retrospective

## Plan of Work

The work is a sequence of deletions followed by a documentation rewrite and a
release. Each milestone removes one coherent surface, updates every caller,
and leaves `just ci` green. No milestone adds a compatibility shim: the
decision is a breaking release, and a shim would keep the removed boundary
alive in code.

Milestone 1 (task 264g) cleans the backlog so the remaining milestones are not
read against stale requirements, and closes the previous tracker.

Milestone 2 (264a) deletes the delegation surface: `ahm task work`,
`ahm audit`, and `ahm task groom`, together with the shared agent plumbing they
all use. This is the largest single deletion and the one that removes the
binary's dependency on any external agent CLI.

Milestone 3 (264b) retires research and ExecPlans as record families: their
validators, index generation, templates, prime sections, managed ignore
entries, and the task `exec_plan` schema field.

Milestone 4 (264c) removes the procedure channel: `ahm context` and
`ahm onboard`, plus the routing and intake prose that `ahm prime` and
`ahm status` print. Before the command is deleted, its three surviving
procedures are rendered into `docs/workflow/tasks.md`,
`docs/workflow/exec-plans.md`, and `docs/workflow/adrs.md`, and edited to
stand on their own as project-owned documents.

Milestone 5 (264d) collapses install and upgrade into one idempotent
`ahm init`, and deletes the legacy `.agents/ahm.json` layout support and the
four record migrations.

Milestone 6 (264e) rewrites the prose: `docs/VISION.md`, `README.md`,
`docs/`, `AGENTS.md`, `CONTRIBUTING.md`, and the architecture map.

Milestone 7 (264f) releases v2.0.0 with release notes that name every removal.

## Milestones

### Milestone 1 — Dispose of the backlog made moot by ADR 022 (task 264g)

Scope: cancel the queued tasks that exist only for removed surfaces, confirm
the survivors do not depend on them, and close tracker 263. Nothing in the
binary changes.

Work: read each non-terminal task under `.ahm/tasks/active/`. Cancel, with
`ahm task cancel <id> --reason <text>`, every task whose acceptance criteria
require a removed command: 163, 171, 185, 187, 188, 189, 189b, 189c, 189d,
189e, 189f, 189g, 205, 218, 222, 223, 231, 245, 246, 247. Each cancellation
reason cites ADR 022. Then resolve 263: it is a Tracking task whose only
child, 263g, remains Pending, and its ExecPlan sits in `active/` with a filled
Outcomes section, which is the single warning `ahm doctor` currently reports.
Either perform 263g (the worktree proof, which needs a human terminal) or
cancel 263g and complete 263, then move
`.ahm/exec-plans/active/263-adopt-feature-branch-development.md` to
`.ahm/exec-plans/completed/` and run `ahm index`.

Result: `ahm task ready` and `ahm task blocked` list only work that the
reduced tool can satisfy, and `ahm doctor` reports no warnings.

Proof: run `ahm doctor` and observe `"ok": true` with an empty warnings list;
run `ahm task list --status Cancelled` and observe each new cancellation cites
ADR 022.

### Milestone 2 — Delete the delegation surface (task 264a)

Scope: remove `ahm task work`, `ahm audit`, `ahm task groom`, and the shared
machinery that builds external agent command lines, parses their output, and
records their sessions.

Work: delete `internal/ahm/task_work.go`, `task_agents.go`, `task_session.go`,
`task_parsers.go`, `audit.go`, and `task_groom.go`, and the tests that cover
them: `audit_test.go`, `task_groom_test.go`, `task_groom_smoke_test.go`,
`task_work_smoke_test.go`, `task_work_env_test.go`, and
`task_session_prompt_test.go`. Delete `internal/ahm/testdata/agents/` (the
recorded JSONL transcripts and their metadata), `scripts/capture-agent-fixtures.sh`,
and the `capture-agent-fixtures` and `smoke-agents` recipes from `justfile`.
Delete `docs/guardrails/external-agent-orchestration.md`. Remove the command
registrations from `internal/ahm/cli.go` and `internal/ahm/task_commands.go`,
and any audit hint that `internal/ahm/prime.go` prints.

Then remove the configuration the delegation surface owned: the `taskWork`
field in `.ahm/config.json`, the code in `internal/ahm/install.go` that seeds
it, and any validator or finding code that reads it. Before deleting
`task_commands_golden_test.go`, read it: if it also pins non-agent command
output, keep the parts that do and delete only the agent cases. Do the same
check for `internal/ahm/markdown_sections.go` and `task_acceptance.go`: use
`rg -n "<name>" internal/ahm` and keep anything the surviving task lifecycle
uses.

Result: the binary no longer knows how to run another program on your behalf,
and no test reaches the network.

Proof: `ahm task work 1`, `ahm audit`, and `ahm task groom` each fail with
`unknown command`; `ahm task --help` lists neither `groom` nor `work`;
`just ci` passes; `rg -n "smoke-agents|promptFile|taskWork" --glob '!.ahm/**' .`
returns nothing outside historical records and the ADR.

### Milestone 3 — Retire research and ExecPlans as record families (task 264b)

Scope: `.ahm/research/` and `.ahm/exec-plans/` stop being ahm-managed. Their
contents stay on disk as ordinary project files.

Work: delete `internal/ahm/research_inbox.go` and its test, the `RESEARCH.md`,
`PLANS.md`, `exec-plans-active-index.md`, and `exec-plans-completed-index.md`
templates under `internal/templates/workflow/`, the research and ExecPlan
index generation in `internal/ahm/indexes.go`, and the prime sections that
list them. Delete the validators `validateResearchInbox`, `validateExecPlans`,
and `validateTaskExecPlans`, and the finding codes that only those emit. In
`internal/ahm/tasks.go`, remove the `ExecPlan` field from the `Task` struct;
confirm by test that an `exec_plan:` front-matter value survives a round trip
in the preserved unknown-field map instead of being dropped. Update
`internal/ahm/workflow_paths.go` so the resolved layout no longer includes
research or ExecPlan directories, and update the `.ahm/.gitignore` content
that `init` writes so it no longer ignores their indexes.

Result: deleting `.ahm/research/` and `.ahm/exec-plans/` from a repository
changes no `ahm` behavior.

Proof: `rg -n "research|exec-plans|exec_plan" internal/ahm --glob '!*_test.go'`
returns nothing that reads or writes those directories; a test creates a task
with `exec_plan: 999-old-plan`, runs a read and a write cycle, and asserts the
line is still present; `ahm doctor` in this repository, which still contains
research and ExecPlan records, reports no finding for them.

### Milestone 4 — Remove the procedure channel (task 264c)

Scope: `ahm` stops printing instructions.

Work: salvage the procedure prose first, while the command that renders it
still exists. Run `ahm context task > docs/workflow/tasks.md`,
`ahm context plan > docs/workflow/exec-plans.md`, and
`ahm context adr > docs/workflow/adrs.md`, then edit all three: replace every
template variable with a concrete repository-relative path, delete instructions
that describe removed behavior (regenerating plan indexes with `ahm index`,
setting a task's `exec_plan` field, the `ahm upgrade` step), and rewrite the
plan document's directory references from `.ahm/exec-plans/active|completed/`
to `docs/exec-plans/active|completed/`. Create `docs/exec-plans/active/` and
`docs/exec-plans/completed/`; this plan file moves into the completed bucket
when the work finishes. Add the three new documents to `docs/README.md`.

Then delete `internal/ahm/context.go` and `context_test.go`, and
`internal/ahm/onboard.go` and `onboard_test.go`, with their registrations in
`internal/ahm/cli.go`. In `internal/ahm/prime.go`, delete the routing block,
the `Managed Work Intake` section, and the grooming and audit hints, leaving:
regenerated indexes, validation findings, and record counts. In
`internal/ahm/status.go`, delete the onboarding snippet. Prose in the salvaged
documents that describes contributor practice rather than records —
verification commands, commit workflow — belongs in `CONTRIBUTING.md`; prose
that routes a reader to a record family belongs in `AGENTS.md`. Both are hand
edits.

Result: no command emits workflow instructions, and no document tells a reader
to run a command that no longer exists.

Proof: `ahm context task`, `ahm context plan`, `ahm context adr`, and
`ahm onboard` fail with `unknown command`; `ahm prime` output contains no
command name; `docs/workflow/tasks.md`, `docs/workflow/exec-plans.md`, and
`docs/workflow/adrs.md` exist, contain no `{{` template variables, and mention
no removed command;
`rg -n "ahm (audit|context|onboard|upgrade)|task groom|task work" --glob '!.ahm/**' --glob '!docs/adr/**' .`
returns nothing outside historical records; `just docs-md-lint` passes.

### Milestone 5 — Collapse install and upgrade into one idempotent `ahm init` (task 264d)

Scope: one command creates ahm-owned state when absent and reconciles it when
present.

Work: delete the `upgrade` command and its install-path code, the
`records migrate` and `records doctor` commands (`records_commands.go`), and
the migrations `records_migrate.go`, `task_migrate.go`, and `adr_migrate.go`
with their tests. Make `ahm init` idempotent: when `.ahm/config.json` exists,
it rewrites only ahm-owned state that drifted (managed `.ahm/.gitignore`,
generated indexes, and the metadata version) and removes obsolete ahm-owned
configuration fields such as `taskWork` while preserving unrelated unknown
fields and `files` hashes. Delete legacy layout support: in
`internal/ahm/root.go`, accept only `.git` and `.ahm/config.json` as root
markers and fail with a message that names the final v1 release a
legacy-layout repository must upgrade with first; in
`internal/ahm/workflow_paths.go`, remove the `.agents/` layout branch. Keep
the rule that `AGENTS.md` is never created, replaced, or removed.

Result: a repository needs one command to adopt or maintain `ahm`, and an
unmodified `AGENTS.md` survives every install path.

Proof: on a scratch clone, `ahm init` twice in a row: the second run writes
nothing and exits 0. Add `"taskWork": {...}` to a copy of the config: `ahm init`
removes that key, keeps unknown keys, and reports success. A directory holding
only a fake `.agents/ahm.json` fails with the v1 message rather than being
treated as unmanaged.

### Milestone 6 — Rewrite documentation and instructions (task 264e)

Scope: every prose surface describes the reduced tool.

Work: rewrite `docs/VISION.md` around the two-family records boundary and the
accepted tradeoff that procedure prose belongs to the project and may drift.
Update `README.md`, `docs/README.md`, `docs/cli.md`,
`docs/references/cli/commands.md`, `docs/references/cli/global-contract.md`,
`docs/references/cli/task-commands.md`, `docs/references/cli/task-file-format.md`,
`docs/references/workflow-spec.md`, and `docs/references/glossary.md`. Delete
or rewrite the guardrails that describe removed surfaces and add the plan
location decided in the Decision Log to `AGENTS.md`, which is a hand edit:
`ahm` must never write that file. Register the new surfaces: add `docs/workflow/` and `docs/exec-plans/` to
`docs/README.md`, to the ownership table in
`docs/guardrails/documentation.md`, and to `AGENTS.md`, which must name
`docs/exec-plans/` as where design plans for large work live now that `ahm` no
longer manages them. Update `ARCHITECTURE.md`'s system boundaries,
compatibility surfaces, and module map so a reader can navigate the reduced
package.

Result: a newcomer can read `README.md`, `ARCHITECTURE.md`, and `AGENTS.md`
and start work without encountering a removed command or family.

Proof: `just docs-md-lint` and `just ci` pass;
`rg -n "research|exec ?plan|worktree|subagent delegation" docs/ README.md AGENTS.md`
returns only intentional references in historical ADRs and records.

### Milestone 7 — Release v2.0.0 (task 264f)

Scope: ship the breaking change with honest notes.

Work: follow `docs/release.md`: create `release/v2.0.0` from `master`, run the
release preparation script, regenerate the changelog, and write release notes
that name the removed commands (`audit`, `context`, `onboard`, `task groom`,
`task work`, `upgrade`), the removed record families, the removed
configuration key, and the required `ahm init` step. A repository on the
legacy `.agents/ahm.json` layout must upgrade with the final v1 release
before adopting v2.

Result: v2.0.0 is tagged and published with a migration note.

Proof: `just release-check` and `just ci` pass on the release branch; the
release branch merges through a pull request with CI green; artifacts exist
for the six target platforms.

## Concrete Steps

All commands run from the repository root unless stated otherwise.

To start any milestone:

    git switch master && git pull --ff-only
    git switch -c feat/<slug>
    ahm prime

Milestone 1 (264g):

    ahm task cancel 245 --reason "Removed surface per ADR 022: task work delegation"
    # … repeat for each task in the milestone's list …
    ahm task list --status Cancelled
    ahm doctor

Milestone 2 (264a):

    git rm internal/ahm/task_work.go internal/ahm/task_agents.go \
        internal/ahm/task_session.go internal/ahm/task_parsers.go \
        internal/ahm/audit.go internal/ahm/task_groom.go \
        internal/ahm/audit_test.go internal/ahm/task_groom_test.go \
        internal/ahm/task_groom_smoke_test.go internal/ahm/task_work_smoke_test.go \
        internal/ahm/task_work_env_test.go internal/ahm/task_session_prompt_test.go \
        scripts/capture-agent-fixtures.sh \
        docs/guardrails/external-agent-orchestration.md
    git rm -r internal/ahm/testdata/agents
    # edit cli.go, task_commands.go, prime.go, install.go, justfile
    go build ./... && just ci

Milestone 3 (264b):

    rg -n "research|exec-plans|execPlan|ExecPlan" internal/ahm internal/templates
    git rm internal/ahm/research_inbox.go internal/ahm/research_inbox_test.go
    git rm internal/templates/workflow/RESEARCH.md internal/templates/workflow/PLANS.md \
        internal/templates/workflow/exec-plans-active-index.md \
        internal/templates/workflow/exec-plans-completed-index.md
    go build ./... && just ci

Milestone 4 (264c):

    mkdir -p docs/workflow docs/exec-plans/active docs/exec-plans/completed
    ahm context task > docs/workflow/tasks.md
    ahm context plan > docs/workflow/exec-plans.md
    ahm context adr  > docs/workflow/adrs.md
    # then edit all three: concrete paths, no template variables, no removed commands
    git rm internal/ahm/context.go internal/ahm/context_test.go \
        internal/ahm/onboard.go internal/ahm/onboard_test.go
    go build ./... && just docs-md-lint && just ci

Milestone 5 (264d):

    git rm internal/ahm/records_commands.go internal/ahm/records_migrate.go \
        internal/ahm/records_migrate_test.go internal/ahm/task_migrate.go \
        internal/ahm/task_migrate_test.go internal/ahm/adr_migrate.go
    go build ./... && just ci

Milestone 6 (264e): edit prose, then

    just docs-md-lint && just ci

Milestone 7 (264f): follow `docs/release.md`.

Every milestone ends with a commit on its branch, a pull request, and a merge
with CI green. Commits use Conventional Commits, for example
`refactor(cli): remove task work, audit, and groom`.

## Validation and Acceptance

The change is accepted when a person can run the following and see these
results, in a clone of this repository at the merged state:

    $ ahm --help
    # lists adr, doctor, index, init, prime, status, task, version only
    $ ahm task --help
    # lists accept, blocked, cancel, comment, complete, create, dep, labels,
    # list, next, ready, reopen, search, show, start — no groom, work, migrate
    $ ahm context
    Error: unknown command "context" for "ahm"
    $ ahm task work 1
    Error: unknown command "work" for "ahm task"
    $ ahm doctor
    # "ok": true

Task and ADR lifecycle behavior must be unchanged. Prove it by running the
existing suites, which is what `just ci` does: `just ci` runs formatting,
`go mod tidy -diff`, `go vet`, `go test -race ./...`, `golangci-lint`,
`govulncheck`, markdown lint, a build, and `goreleaser check`. Expect exit
status 0 and no new findings. The suite must not make any network request
after milestone 2; the provider smoke tests, which required `AHM_AGENT_SMOKE=1`
and real API keys, no longer exist.

The reduction is measurable. Before the work, from the repository root:

    find cmd internal -name '*.go' -not -name '*_test.go' | xargs wc -l | tail -1
    # 12302 total

and after milestone 5 the expectation is roughly 7,000-8,000 lines, with the
exact number recorded in Artifacts and Notes. The test suite drops from about
18,600 lines to about 14,000.

A repository-level acceptance case, run on a scratch clone after milestone 5,
proves the surviving product end to end:

    ahm init
    ahm task create "Try the reduced tool" --priority P1 --effort S
    ahm task accept <id> && ahm task start <id>
    ahm task complete <id> --body "did the thing"
    ahm adr create "Records only" --status accepted
    ahm index && ahm prime

Expect: the task moves through the buckets with its acceptance notes
validated, the ADR appears in `docs/adr/index.md`, and `ahm prime` prints
counts and validation findings without any routing prose.

## Idempotence and Recovery

Every step in this plan is a file deletion or a prose edit, and every step is
committed on its own branch, so recovery is `git switch master` and abandon
the branch. No step mutates a database, a remote, or the user's worktree
outside the repository.

Two steps deserve care. Deleting a file that a surviving file still references
breaks the build; run `go build ./...` after each `git rm` batch and let the
compiler enumerate the callers, then fix or delete them in the same commit.
Editing `.ahm/config.json` and `.ahm/.gitignore` by hand is safe because both
are ordinary committed files, but the only supported way to change them is
through `ahm init`; after any hand edit, run `ahm index` and `ahm doctor` so
generated state matches.

The one irreversible action is deleting a record file. This plan never deletes
`.ahm/research/` or `.ahm/exec-plans/` content; those files stay in the working
tree and in git history, and their removal is the project's choice, made
outside `ahm`.

## Artifacts and Notes

Baseline measured on 2026-09-20 at commit `371fc62`, before any milestone:

    12302 total non-test Go lines in cmd/ and internal/
    18552 total test Go lines across the repository
    112K of agent transcript fixtures under internal/ahm/testdata/agents/
    135 references to the six removed commands across 28 documentation files
    ahm doctor: 1 warning (active ExecPlan with a filled Outcomes section)

The command surface before the work, from `ahm --help` and
`ahm task --help`: `adr`, `audit`, `context`, `doctor`, `index`, `init`,
`onboard`, `prime`, `records`, `status`, `task`, `upgrade`, `version`; and
under `task`: `accept`, `blocked`, `cancel`, `comment`, `complete`, `create`,
`dep`, `groom`, `labels`, `list`, `migrate`, `next`, `ready`, `reopen`,
`search`, `show`, `start`, `work`.

## Interfaces and Dependencies

No new dependency is added, and no dependency version changes. The binary
keeps `github.com/spf13/cobra` for command parsing and `internal/version`
for the injected version string. `justfile`, `.goreleaser.yaml`, and
`.github/workflows/ci.yml` keep their current tool versions and command
contract, except that `justfile` loses the `smoke-agents` and
`capture-agent-fixtures` recipes.

At the end of milestone 5 the CLI must expose exactly these commands:
`version`, `init`, `prime`, `status`, `doctor`, `index`, `task`, `adr`. The
`task` group must expose exactly `create`, `list`, `show`, `search`, `next`,
`ready`, `blocked`, `accept`, `start`, `complete`, `cancel`, `reopen`,
`comment`, `labels`, `dep`. The `adr` group must expose exactly `create`,
`list`, `show`, `accept`, `reject`, `deprecate`, `propose`, `supersede`. The
global flags `--root`, `--json`, `--plain`, `--text`, `--dry-run`, and
`--force` keep their current meanings and output contracts.

`.ahm/config.json` keeps the metadata version, the acceptance setting, and the
managed-file hash map, and loses `taskWork`. Task front matter keeps `id`,
`title`, `status`, `priority`, `effort`, `labels`, `depends_on`, `parent`,
`external_ref`, `created`, and `updated`; `exec_plan` becomes an unmanaged,
preserved field. ADR front matter is unchanged.

Internal package boundaries to preserve: `internal/ahm/write.go` remains the
only atomic write path (write a temporary file in the target directory, then
rename); `internal/ahm/lock.go` remains the only cross-process lock; generated
indexes remain deterministic in ordering; validation remains read-only; and
`internal/ahm/git.go` continues to scrub inherited Git repository-location
environment variables before running any Git subprocess.

Note on this plan's own lifecycle: milestone 3 removes the ExecPlan record
family from `ahm`, at which point this file becomes an ordinary project-owned
document. When the work completes, move it to `docs/exec-plans/completed/`,
the location recorded in the Decision Log, and update `AGENTS.md` accordingly
in milestone 6.

Documentation surfaces after the change: `docs/workflow/tasks.md`,
`docs/workflow/exec-plans.md`, and `docs/workflow/adrs.md` are project-owned and
hand-maintained; `docs/exec-plans/active/` and `docs/exec-plans/completed/`
hold design plans; and `docs/adr/` with its generated `index.md` remains the
ADR family `ahm` still manages.

## Revision Notes

- (2026-09-20) Owner decision recorded: design plans live in
  `docs/exec-plans/`, and the procedures that `ahm context` prints are salvaged
  to `docs/workflow/tasks.md`, `docs/workflow/exec-plans.md`, and
  `docs/workflow/adrs.md`. Milestone 4 and the Concrete Steps now salvage the
  prose before deleting `context.go`, and milestone 6 registers the new
  documentation surfaces.
