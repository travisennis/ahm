# Reduce ahm to a tasks-and-ADRs records CLI

This ExecPlan is a living document. The sections `Progress`, `Surprises &
Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to
date as work proceeds. This document is maintained in accordance with the
[ExecPlan workflow](../../workflow/exec-plans.md); reopen that guide before
revising the plan.

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
`internal/ahm/`, and `internal/version/version.go` holds the version string the
release build injects. Milestone 2 deleted `internal/templates/`, the package
that embedded the managed instruction templates. There were about 12,300 lines
of non-test Go and about 18,600 lines of test Go when this plan was written.

Workflow records are ordinary committed Markdown files in the consuming
repository. Tasks live under `.ahm/tasks/{active,completed,cancelled}/` and ADRs
under `docs/adr/`; both families have a generated `index.md` that is derived
data: `ahm index` and `ahm prime` regenerate it, and it must never be edited by
hand. Research notes under `.ahm/research/` and ExecPlans under
`.ahm/exec-plans/{active,completed}/` were retired as ahm-managed families in
milestone 3; they are ordinary project files now, and design plans for this
repository live under `docs/exec-plans/{active,completed}/` (milestone 4 moved
this plan into `docs/exec-plans/active/`). Configuration lives in
`.ahm/config.json`. Settings are committed so every clone and CI sees the same
values. `.ahm/.gitignore`
ignores the generated indexes that are not part of the published
documentation, such as the task bucket indexes.

Two rules constrain every edit in this plan. First, `AGENTS.md` at the
repository root is project-owned: no `ahm` code path may create, replace, or
remove it. Second, `ahm` never performs implicit Git operations: it does not
commit, stage, push, move `HEAD`, or modify branches. Every commit and push in
this plan is a human or agent action performed with ordinary `git`, not with
`ahm`.

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

The repository commits directly to `master`; there is no pull request step. Git
hooks installed by `prek` (a Rust reimplementation of `pre-commit`) run
formatting, tidy, test, and lint checks when a `*.go` file is staged, and CI
runs on every push. Note the exact condition: those Go hooks exit 0
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
- [x] (2026-09-20) The nine ADRs that ADR 022 replaces are marked superseded
      (`ahm adr supersede` for 004, 006, 011, 012, 014, 017, 019, 020, 021;
      019 was `proposed` and was accepted first).
- [x] (2026-09-20) Second defect found while superseding: the replacement ADR
      lost its final newline when its More Information section was last, which
      failed `just docs-md-lint` with MD047. Fixed by normalizing rewritten ADR
      content in `rewriteADR`; task 266 completed.
- [x] (2026-09-20) Workflow reversal applied and published: `AGENTS.md` and
      `CONTRIBUTING.md` describe direct commits to `master`, the
      `require-feature-branch` hook and `semantic-pr.yml` are deleted, and
      `docs/release.md` and `scripts/prepare-release.sh` drop the
      release-branch flow. ExecPlan 263 and task 263g are cancelled.
- [x] (2026-09-20) The GitHub pull-request rule on `master` is gone: `gh api
      repos/:owner/:repo/branches/master/protection` reports
      `required_pull_request_reviews: null` and `required_status_checks:
      null`, and the reversal commit `bcb8ae0` is on `origin/master`.
- [x] (2026-09-20) Milestone 1 (264g) complete: 28 moot tasks cancelled, 7
      survivors re-scoped to the reduced tool, and tracker 263 closed with its
      ExecPlan already in `completed/`.
- [x] (2026-09-20) Milestone 2 (264a) complete: the delegation surface is
deleted, and the binary runs no program but Git.
- [x] Milestone 3 (264b) complete: research and ExecPlans retired. The two
      procedure templates are deferred to milestone 4, which is the only
      consumer left; see the Decision Log.
- [x] Milestone 4 (264c) complete: the procedure channel and the last
      prescription are gone. The three surviving procedures are project-owned
      documents under `docs/workflow/`, design plans live under
      `docs/exec-plans/`, and no command emits workflow instructions.
- [x] Milestone 5 (264d) complete: install collapsed to one idempotent
      `ahm init`, the legacy `.agents/ahm.json` layout refused, and the four
      record migrations and their commands deleted. The prose rule is applied
      to the live docs that named them, and `docs/guides/workflow-upgrades.md`
      now opens with the v2 migration note. Landed as `94790cf`, with the
      Windows test fix in `35ed4ec`; CI is green on both runners at `35ed4ec`.
- [x] Milestone 6 (264e) complete: documentation and instructions rewritten.
      README, ARCHITECTURE, CONTRIBUTING, `.agents/prompt.md`, and the two
      surviving skills describe the two-family records boundary; the live
      `docs/` surfaces lost their stale claims about removed commands,
      removed families, managed-file hashes, and task-command behavior;
      `docs/guides/workflow-upgrades.md` gained the v2 migration note and
      dropped its dated pre-v2 release history; `docs/VISION.md` needed no
      edit, because milestone 4 had already rewritten it. Two subagent review
      rounds ran; `just ci` and `just docs-md-lint` are green on the working
      tree. Not committed at handoff — see the Revision Notes.
- [x] (2026-09-26) Milestone 7 (264f) complete: v2.0.0 released, preceded by
      v1.1.0 as the legacy-layout migration waypoint. See the Decision Log
      and Outcomes for what the release forced that the plan did not foresee.

## Surprises & Discoveries

- Observation: the documentation milestone had to correct claims the reference
  pages made about current behavior, not only claims about removed surfaces.
  Four were wrong: `docs/references/cli/task-commands.md` said `task accept`
  completes a task (it sets `Pending`), documented a `--reason` positional for
  `task cancel` (it is a required flag), listed `--search` and `--by-id` flags
  that `task list` does not have, and claimed status-transition refusals that
  no code implements (`taskStatusWithArgsLocked` guards only dependency
  completion and strict acceptance, so `task start` on a completed task really
  does move it back to `active/`).
  Evidence: `internal/ahm/task_commands.go:186` (`accept` maps to `Pending`),
  `:233` (`--reason`), `internal/ahm/task_status.go:75-135`, and
  `/tmp/ahm-dev --dry-run task start 001` printing `move: .../active/001.md`.

- Observation: `managed_file_*` finding codes no longer exist. Milestone 5
  retired managed-file hash reconciliation, and `validateManagedFiles` now
  only checks metadata presence and task files, so the four rows in
  `docs/references/cli/task-file-format.md` and the "managed file consistency"
  scope description in `docs/references/workflow-spec.md` were stale.
  Evidence: `rg managed_file internal/` returns nothing; `validation.go:208`.

- Observation: the retired generated indexes under `.ahm/exec-plans/` and
  `.ahm/research/` were verified, not touched. ADR 022's release treatment
  says ahm leaves them in place, and `docs/exec-plans/README.md` already
  records that they are unmaintained, so milestone 6 owes no further action
  here.
  Evidence: `git status --short` shows no change under either directory.

- Observation: the installed dev binary on `PATH` (`~/go/bin/ahm`, built
  2026-09-20 09:54) is a stale v1 build: `ahm prime` still prints `## Recent
  Research` and `ahm --root <dir> --dry-run init` still plans
  `.ahm/research/*` directories. Every behavior probe in this milestone used a
  build of the current tree instead, and the stale binary was left alone.
  Evidence: `go build -o /tmp/ahm-dev ./cmd/ahm`; the two commands above.

- Observation: the legacy-layout migration path names a release that cannot
  perform it. `internal/ahm/root.go` sets `finalV1Release = "v1.0.0"`, but the
  `v1.0.0` tag predates `ahm records migrate` (the `records` group arrived
  after it), and v1.0.0's `upgrade` keeps `.agents/ahm.json` in place, so a
  repository that follows the message is refused again by v2. Milestone 7 owns
  the naming: either cut a v1.x tag from the pre-reduction tree and name that,
  or reword the refusal. The guide now describes the requirement without
  naming `v1.0.0` as the fix.
  Evidence: `git ls-tree -r --name-only v1.0.0 | rg records` returns no
  `internal/ahm/records*` file; `git log --diff-filter=A --
  internal/ahm/records_commands.go` dates the command after the tag.

- Observation: prime's `## Useful Commands` block, the ready-overflow line that
  pointed at `ahm task ready`, the blocked and open counts with their
  parenthetical commands, and the doctor pointer in the validation line were
  the last places the binary named a command to run. Criterion two forbids any
  of them, so prime now prints a bare count for the overflow and a bare
  `validation: N errors, M warnings` line, and the `commands` array left the
  JSON report with the routing prose. The dirty-worktree warning also lost its
  "resolve them before starting new work" advice, because that prescribes a
  step. This is a deliberate breaking change to prime's output shape and
  belongs in the v2 release notes.
  Evidence: `ahm prime` before the change ended with a `## Useful Commands`
  list containing `ahm status` and `ahm doctor`; after it, the report ends at
  `Open: N`, and `TestPrimePrintsSessionBriefing` asserts that `ahm context`,
  `ahm doctor`, `ahm task ready`, `ahm task blocked`, and `ahm task list` do
  not appear.

- Observation: deleting `internal/ahm/context.go` made
  `workflowPaths.researchRel` and `workflowPaths.execPlansRel` dead code, which
  milestone 3 had knowingly kept because the context renderer resolved those
  strings. `golangci-lint`'s `unused` check named both as soon as the renderer
  was gone, and both are deleted in this milestone.
  Evidence: `just lint` reported `func workflowPaths.researchRel is unused` and
  `func workflowPaths.execPlansRel is unused` before the deletion, and reports
  `0 issues` after it.

- Observation: milestone 4's acceptance criteria reach further into `docs/`
  than the milestone's Work paragraph does, and milestone 3's handoff list had
  assigned `docs/references/*` to milestone 6. The pass resolved the overlap by
  applying the instruction clause of the prose rule to every live document in
  `AGENTS.md`, `CONTRIBUTING.md`, and `docs/`, and leaving the positive
  rewrite to milestone 6: `README.md`, `ARCHITECTURE.md`'s remaining prose,
  `.agents/prompt.md` and `.agents/skills/*`, and the dated release-history
  sections of `docs/guides/workflow-upgrades.md` still name removed commands
  and removed families. `ARCHITECTURE.md` was corrected only where this
  milestone deleted the files its module map and compatibility list named
  (`context.go`, `research_inbox.go`, `onboard.go`, `internal/templates/`).
  Evidence: `rg -n "ahm (audit|context|onboard)|task groom|task work"
  AGENTS.md CONTRIBUTING.md docs/ --glob '!docs/adr/**' --glob
  '!docs/guides/workflow-upgrades.md' --glob '!docs/exec-plans/**'` returns
  nothing, while the same search without the exclusions still hits
  `README.md`, `ARCHITECTURE.md`, `.agents/`, and the upgrade guide.

- Observation: removing the doctor's onboarding finding left
  `validationReport.addInfo` with no caller and the `info` severity with no
  producer. The method is deleted; the `Info` field and its rendering stay,
  because the field is part of the `status`/`doctor` JSON shape and milestone
  3's deletion-invariance test deliberately compares all three finding
  severities.
  Evidence: `rg -n "addInfo" internal/` returned only the definition after the
  onboard finding was deleted; `validation_test.go` compares `report.Info`
  before and after the retired trees are removed.

- Observation: the independent review of this milestone ran once, through
  `codex exec --sandbox read-only`, which read the diff and the new files and
  returned four findings (the dirty-worktree advice prescribing a step, an
  ambiguous scaffold-preservation sentence in the workflow spec, the preflight
  skill's `ahm context` references, and "commit frequently" in the salvaged plan
  guide). All four are fixed in this commit. The follow-up round could not run:
  the second invocation hit the reviewer's usage limit after reading the tree
  (`You've hit your usage limit`), so no verdict exists for the post-fix state.
  Evidence: the first run's transcript named the four findings; the second run
  ended with `ERROR: You've hit your usage limit` and no report. Deferred
  probe: a fresh review pass over this commit, or the repository's own preflight
  skill in a session whose reviewer has budget.

- Observation: the retired generated indexes under `.ahm/exec-plans/` and
  `.ahm/research/` are now stale project files that still claim to be
  "generated by `ahm index`" and, in the active bucket, list a plan at a path
  it no longer occupies. ADR 022's release treatment says ahm leaves retired
  index files in place, neither validating nor deleting them, so this milestone
  left them and `docs/exec-plans/README.md` records that they are unmaintained.
  Evidence: `.ahm/exec-plans/active/index.md` links
  `264-reduce-ahm-to-tasks-and-adrs.md`, which now lives under
  `docs/exec-plans/active/`.

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

- Observation: `ahm adr supersede` deleted the replacement ADR's final newline
  whenever its `More Information` section was the last section in the file, so
  ADR 022 failed `just docs-md-lint` with MD047 right after the nine
  supersessions. The defect predates this work: `upsertADRMoreInformationLine`
  trims the section's trailing blank lines and re-joins the lines, which drops
  the end-of-file newline regardless of how the body was written.
  Evidence: a binary built from `master` reproduces it in a scratch repository:
  create ADR 001 as accepted, create ADR 002 whose last section is
  `## More Information`, run `ahm adr supersede 001 --by 002`, and the
  replacement file ends `...decision.md).` with no newline. Fixed by
  `ensureSingleTrailingNewline` in `rewriteADR` (task 266).

- Observation: the required-pull-request rule on `master` cannot be removed
  from an agent shell. Deleting the protection rule over the API was refused
  by the environment even with the owner's explicit confirmation, so the owner
  changes it, not the agent.
  Evidence: `gh api -X DELETE
  repos/:owner/:repo/branches/master/protection/required_pull_request_reviews`
  is blocked; the pre-change configuration is saved at
  `/tmp/ahm-master-protection-backup.json`, and the settings that matter are
  reproduced in the Revision Notes below.

- Observation: the milestone-1 backlog lists under-count the work ADR 022 makes
  moot. Neither the task record nor this plan named 229, 250, 251, 255, or 256,
  yet each requires a deleted surface: `applyGroomVerdicts`, `createAuditTasks`,
  the shared groom and audit result parsers, ADR 019's research and plan
  lifecycle commands, and an export format whose payload was research notes and
  ExecPlans. The plan's list also omitted 231 (groom and audit stream output)
  and 189c-189f, which the task record omitted.
  Evidence: `rg -n "groom|audit|task work" .ahm/tasks/active/` before the pass
  returns hits in every one of those records.

- Observation: cancelling a task leaves its id in every dependent's
  `depends_on`, and validation then reports `task_dependency_cancelled` for each
  non-terminal dependent, so `ahm doctor` stays noisy until the whole chain is
  disposed of. Task 236 was the only dependent worth keeping, so its dependency
  moved from the cancelled 235 to 264e.
  Evidence: each `ahm task cancel` printed the surviving dependents; `ahm task
  dep remove 236 235` followed by `ahm task dep add 236 264e` clears it.

- Observation: seven task records written before ADR 022 name surfaces that
  survive but describe them through removed neighbours, so they are wrong rather
  than moot: 155 (config keys), 168 (which indexes an edit regenerates), 236
  (context scopes and the `records` group), 249 (prompt-building reads, and
  `readMetadata` is in `install.go`, not the `metadata.go` the record named),
  254 (two of eight validators), 257 (external-store motivation), and 258
  (research-note references and `context` guidance). The three newest also cite
  `just check`, a recipe that does not exist; the gate is `just ci`.
  Evidence: `rg -n "research|exec ?plan|am context|task work|groom"
  .ahm/tasks/active/` after the cancellations.

- Observation: `internal/ahm/testdata/agents/` held only the parser goldens;
  the delegation tests themselves were not confined to the files the plan
  named. `internal/ahm/task_commands_test.go` was 5,726 lines and 2,317 of them
  (every `TestTaskWork*`, every `parse*SessionID` and `parse*ReviewFeedback`
  test, the `taskWorkCapture` stub, `stubTaskWorkLookPath`,
  `stubTaskWorkRunner`, and the `completeTaskOnDisk` helper) covered the deleted
  surface. The milestone's test list named six files and missed the largest,
  exactly as milestone 1's task list missed five records.
  Evidence: the pre-milestone file has 55 `TestTaskWork*` functions and the
  trimmed file keeps 103 functions.

- Observation: the plan's milestone-2 deletion list omitted
  `scripts/task-workflow.sh`, a tracked project-local script that runs four
  `cake` invocations, captures a session ID from JSON output with `jq`, and
  resumes the session for review and commit. It is delegation plumbing by any
  reading of the milestone's own acceptance criterion, which names "script"
  explicitly, so it was deleted with the rest.
  Evidence: `rg -n "session_id|--resume" scripts/task-workflow.sh`; ADR 006 and
  ADR 008 both cite the script as the reference workflow they replaced.

- Observation: `.ahm/config.json` carried a `taskWork` block and a
  `default_work_agent` key, and after their typed fields are gone the keys
  would otherwise survive forever in the preserved unknown-field map.
  Consuming both in `metadata.UnmarshalJSON` makes the next metadata write drop
  them, which is what this milestone needs; the review of this milestone raised
  the second key, since its only reader was `task_agents.go`.
  Evidence: `TestMetadataDropsObsoleteAgentKeys` and
  `TestMetadataRewritePreservesUnknownFieldsAndDropsAgentKeys` in
  `internal/ahm/install_test.go`.

- Observation: deleting the guardrail and the testing guide orphaned live
  links. `docs/references/glossary.md` had an entire "Agent Delegation" section
  and `ARCHITECTURE.md` a reference bullet pointing at the removed surfaces, so
  both lost those lines in this commit rather than waiting for milestone 6.
  `docs/testing.md` existed only to point at `docs/guides/testing.md` and went
  with it; `docs/guardrails/safety-and-permissions.md` named `task work` as its
  delegation boundary, and now names the one command runner that is left.
  Evidence: `rg -n "external-agent-orchestration|guides/testing" docs
  README.md AGENTS.md CONTRIBUTING.md ARCHITECTURE.md` returns only ADR 016,
  which is historical record.

- Observation: `internal/ahm/markdown_sections.go` looks groom-owned but is
  not. `task_status.go` uses `locateHeadingSections` for the Cancellation
  Reason section and `adrs.go` uses it for MADR sections, so the file and its
  `TestLocateHeadingSections` survive; only the groom-specific test in
  `markdown_sections_test.go` was removed.
  Evidence: `rg -n "locateHeadingSections" internal/ahm` shows the
  `task_status.go` and `adrs.go` callers.

- Observation: milestone 3's deletion list and milestone 4's salvage step
  cannot both run as written. Milestone 3 is told to delete
  `internal/templates/workflow/PLANS.md`, and milestone 4 is told to run
  `ahm context plan > docs/workflow/exec-plans.md` before deleting
  `context.go`. That command renders `workflow/PLANS.md` through the embedded
  template set, so deleting the template in milestone 3 leaves milestone 4
  with nothing to render and no way to recover the prose except from Git
  history. `RESEARCH.md` has the same shape but a smaller cost, since its
  procedure is deliberately not salvaged.
  Evidence: `internal/ahm/context.go` maps the `plan` scope to
  `workflow/PLANS.md` and `renderInstructionTemplate` refuses a missing key, so
  the scoped command fails outright once the file is gone; the ExecPlan's own
  Concrete Steps put the `ahm context plan` redirect in the milestone after
  this one.

- Observation: `exec_plan` cannot simply become a sorted preserved field and
  still satisfy the task's "round-trips byte-identically" criterion. A v1 task
  file carries the field between `labels` and `depends_on`, while
  `renderTask` writes preserved keys after every known field, so the first
  rewrite of every v1 task file would reorder it. The review of this milestone
  raised exactly that, and the fix is to keep the field's original slot while
  carrying its value in `Task.Extra`.
  Evidence: the first pass of `TestRenderTaskExecPlanFieldSurvivesWriteCycle`
  could only compare the second render against the first, which proves
  idempotence rather than the byte-identity the acceptance notes require.

- Observation: milestone 3's index change also touches the task index format
  and the managed `.ahm/.gitignore`, and both need a migration note.
  `writeTaskTable` derived its `ExecPlan` column from the retired field, so the
  column goes with the field, and `recordsGitignoreEntries` has to stop
  ignoring every `index.md` under `.ahm/` or the retired families' indexes stay
  hidden behind a pattern that no longer describes ahm-managed state.
  Evidence: `.ahm/research/index.md`, `.ahm/exec-plans/active/index.md`, and
  `.ahm/exec-plans/completed/index.md` appear as untracked files in
  `git status` after the narrowing, and `ahm index` no longer rewrites them.

- Observation: `planRecordsGitignore` only ever appends missing entries, so a
  repository that already ran `ahm records migrate` keeps its broad `index.md`
  line and still ignores the retired families' indexes. Nothing in milestone 3
  can prune it without turning `records migrate` into a reconciler, which is
  milestone 5's job for `ahm init`.
  Evidence: `planRecordsGitignore` compares `recordsGitignoreEntries` against
  the file only to collect `missing` entries, and `writeRecordsGitignore`
  appends them.

- Observation: the legacy layout was load-bearing for the test suite, not just
  for the binary. About 180 fixtures wrote task files under `.agents/.tasks/`
  and relied on the no-metadata default layout to read them back, two
  `validation_test.go` tables existed only to run each case twice (once per
  layout), and the install suite was mostly upgrade-removal and conflict cases
  that the new boundary deletes outright. The milestone's file list named none
  of that; the mechanical repoint is `".agents", ".tasks"` to `".ahm", "tasks"`.
  Evidence: `rg -c '"\.agents", "\.tasks"' *_test.go` counted 167 occurrences
  across seven files before the repoint, and `git diff --stat` shows those files.

- Observation: `records.go` held only `runGit` and `runGitBytes`, whose sole
  caller was the records migration's tracked-path check, so deleting the
  migration made the whole file dead. `hashBytes` moved to `tasks.go`, where its
  remaining caller is the task source hash, and `indexWriteTargetsFor`,
  `taskFilePaths`, and `frontMatterValue` were dead once their callers went.
  Evidence: `just lint` reported `func runGit is unused`, `func runGitBytes is
  unused`, `func taskFilePaths is unused`, and `func frontMatterValue is unused`
  before the cleanup, and reports `0 issues` after it.

- Observation: the legacy ADR validation finding was the one place a validator
  still named a removed command. `adr_legacy_format` read "run ahm adr
  migrate", so the milestone reworded it to "convert it to MADR front matter
  manually" and updated the two documents that quote it. The finding code and
  severity are unchanged.
  Evidence: `rg -n "adr migrate" internal/ docs/` before the change hits
  `validation.go`, `docs/references/cli/task-file-format.md`,
  `docs/references/workflow-spec.md`, `docs/workflow/adrs.md`, and
  `validation_test.go`; after it, only the dated release history and the ADRs
  name it.

- Observation: the milestone changed two shapes the v2 release notes owe.
  `init`'s diagnostic report is now the reconcile set — `created`, `updated`,
  `directories`, and the stale `indexes` — instead of
  `adopted`/`created`/`updated`/`removed`/`skipped`/`conflicts`/`metadata`/`indexes`,
  and an up-to-date repository prints nothing at all. `--force` also stops
  having any install-path effect: its only remaining reader is `task
  complete`'s strict-acceptance override.
  Evidence: `docs/cli.md` and `docs/references/cli/global-contract.md` were
  updated for both, and `TestInitJSONResultSchema` pins the four keys.

- Observation: `.ahm/.gitignore` is reconciled unconditionally rather than
  guarded by an ownership hash and `--force`. It is a tool-owned file whose own
  header says ahm manages it, the milestone's requirement is to prune drifted
  ahm-owned state (the broad `index.md` line a `records migrate` left behind),
  and recording hashes for it would reintroduce the conflict reporting this
  milestone deletes. A project that needs extra ignore lines has the
  repository-root `.gitignore`, which ahm never touches.
  Evidence: `reconcileFile` rewrites `.ahm/.gitignore` only when its bytes
  differ from `recordsGitignoreContent()`, and
  `TestInitRewritesDriftedManagedGitignore` covers both the drift and the
  no-op case.

- Observation: the milestone applied the prose rule to the live documents that
  named the removed commands and left the coherent rewrites to milestone 6. It
  touched `docs/references/cli/{commands,global-contract,task-commands,task-file-format}.md`,
  `docs/references/{workflow-spec,glossary}.md`, `docs/cli.md`,
  `docs/README.md`,
  `docs/guardrails/{agent-instructions,documentation,safety-and-permissions,workflow-state-and-file-formats}.md`,
  `docs/workflow/adrs.md`, `docs/VISION.md`, `ARCHITECTURE.md`,
  `CONTRIBUTING.md`, `AGENTS.md`, and `README.md`, and rewrote the head of
  `docs/guides/workflow-upgrades.md` as the v2 migration note the prose rule's
  first exception covers.
  Evidence: `rg -n "ahm upgrade|records migrate|records doctor|ahm records|task
  migrate|adr migrate" docs/ README.md AGENTS.md CONTRIBUTING.md ARCHITECTURE.md
  .agents/` now hits only `docs/adr/**` (historical record), this plan's own
  milestone text, and that note, whose two remaining references name the final
  v1 release deliberately.

- Observation: `README.md`, `.agents/prompt.md`, and its two skills still
  describe removed commands and the retired layout. This milestone left them to
  milestone 6 rather than patching them piecemeal, because both files need a
  coherent rewrite and the plan names them there. The spots are `README.md`
  lines 4, 36, and 59 (`.agents` intro, `ahm task work 001` in the quickstart,
  and the delegation paragraph in Safety), `.agents/prompt.md`'s opening
  paragraph, and
  `.agents/skills/{grooming-backlog,finding-improvements}/SKILL.md`.
  Evidence: `rg -n "ahm task work|\.agents/" README.md .agents/prompt.md`
  before handoff.

- Observation: the plan's milestone-5 acceptance case for the surviving product
  does not run as written: `ahm task complete <id> --body "did the thing"`
  fails with `unknown flag: --body`, because completion takes no body flag and
  the record's Acceptance Notes carry the outcome. The case is corrected in
  place, and the rest of the sequence — init, task create, accept, start,
  complete, `ahm adr create`, `ahm index`, `ahm prime` — runs end to end on a
  scratch repository.
  Evidence: the acceptance case in "Validation and Acceptance" is now
  `ahm task complete <id>`, and every step was run against a scratch clone.

- Observation: root detection refuses exactly one retired layout, the
  `.agents/ahm.json` metadata file the acceptance notes name. The other retired
  shape, the dot-prefixed `.ahm/.tasks/` tree that an early `.ahm` migration
  created, is not checked: a `.ahm/.tasks/` directory can also be an empty
  leftover, so refusing on its existence alone would block `init` in a
  repository with nothing to migrate, and the v1 `records migrate` path that
  normalized it is gone. A repository on that layout therefore reports an empty
  backlog instead of an error. This is a deliberate scoping decision and a
  release-note candidate.
  Evidence: `rejectLegacyLayout` checks only `.agents/ahm.json` and the
  `.ahm/config.json` exemption, and `legacyDotRecordMigrationRoots` was deleted
  with `records_migrate.go`.

- Observation: this milestone's independent review was self-performed, not run
  in a subagent. The session exposed no subagent tool, and the reviewer CLIs on
  the machine are unusable from here (codex is excluded by the owner's
  instruction, the `claude` shim points at a missing version directory, and
  `amp` fails at startup on its cache directory). The preflight skill's three
  L/XL passes therefore ran against the diff directly and produced seven fixes:
  the explicit `--root` path bypassed the legacy refusal, `.ahm/config.json`
  now wins over a leftover legacy file, `reconcileFile`'s parameter shadowed the
  package `relPath` helper, `directories` marshalled as `null`, `hashBytes`
  claimed a purpose its only caller does not have, a test variable still said
  `agentsDir`, and `records_test.go` named a deleted subject. Deferred probe: a
  subagent or independent review pass over this commit.
  Evidence: every fix is in the tree with a covering test or an updated comment;
  `just ci` is green afterwards.

- Observation: two structures were kept that a stricter reading could remove,
  and the milestone chose the smaller diff for both. `workflowPaths` survives as
  a root-carrying path value (`tasksRel`, `tasksBucketDir`, `taskFile`) rather
  than being deleted and its parameter threaded out of `collectTasksForPaths`,
  `indexWritesForPaths`, and the validation entry points; and `init` renders the
  index set twice, once to classify drift and once inside `writeIndexes`, which
  is the same double render the previous `init` did with
  `indexWriteTargetsFor` plus `writeIndexes`.
  Evidence: `rg -c "workflowPaths" internal/ahm/*.go` totals 68 mentions
  across the package and its tests, and `reconcileIndexes` calls `indexWrites`
  then `writeIndexes`.

- Observation: the milestone's first push failed CI on windows-latest while
  `just ci` was green locally. Three of the new assertions compared an error
  message against the literal `.agents/ahm.json`, but the message renders the
  path with `filepath.Join`, so Windows prints backslashes. Tests that assert
  a path built by `filepath.Join` must build the expectation with
  `filepath.FromSlash`, or assert a separator-free substring; the report's own
  entries are safe because `relPath` returns `filepath.ToSlash`.
  Evidence: run 35538827634 failed on windows-latest with "output missing
  \".agents/ahm.json\""; run 35539315535 is green on both runners after
  `35ed4ec`.

## Decision Log

- Decision: milestone 6 drops the dated pre-v2 release history from
  `docs/guides/workflow-upgrades.md` instead of rewriting each entry.
  Rationale: the prose rule's three exceptions do not cover a dated history,
  and most entries address the reader in the present tense about surfaces v2
  removes (`ahm upgrade` still removes a managed file, `ahm context docs` exits
  with a usage error, `research_inbox_stale` findings), so they offer removed
  commands and retired families as current. Rewriting them to past tense would
  be a 400-line diff that still presents the retired families as ahm-managed.
  The guide keeps its live job — reconcile behavior, the v2 migration note, and
  the legacy-layout path — and points at `git log -p`, `CHANGELOG.md`, and the
  ADRs for the record.
  Date/Author: 2026-09-20, Travis Ennis (executed under task 264e).

- Decision: the reference pages now describe the task transitions the binary
  implements, not the refusals they previously promised.
  Rationale: `docs/references/cli/task-commands.md` documented refusals for
  `task start`, `task complete`, and `task cancel` that no code path enforces,
  and a compatibility-surface reference that promises a guard the binary lacks
  misleads every reader who relies on it. Correcting the prose is in scope for
  this milestone; adding the guards would be a behavior change that needs its
  own task and, if the current behavior is the intent, no change at all.
  Date/Author: 2026-09-20, Travis Ennis (executed under task 264e).

- Decision: the legacy-layout migration release name is escalated to milestone
  7 rather than settled here.
  Rationale: `root.go` names `v1.0.0`, and the `v1.0.0` tag predates
  `ahm records migrate`, so the named release cannot perform the migration the
  message asks for. Choosing the release to name is a release decision — cut a
  v1.x tag from the pre-reduction tree, or reword the refusal — and the message
  text is a compatibility surface. The guide now states the requirement
  (a v1 build whose `ahm records migrate` resolves) without asserting a
  version that cannot satisfy it; milestone 7 owes the naming and the matching
  edit to `root.go`, the guide, and the release notes.
  Date/Author: 2026-09-20, Travis Ennis (executed under task 264e).

- Decision: root detection refuses `.agents/ahm.json` only when
  `.ahm/config.json` is absent, and every root-resolution entry point checks
  the refusal, including an explicit `--root`.
  Rationale: `.ahm/config.json` is the stronger signal. The v1 records
  migration moved the records first, wrote the config second, and removed the
  legacy file last, so a repository holding both has finished its move and only
  has stale metadata left; refusing it would block a manageable repository and
  offer the wrong remedy. Checking the explicit `--root` path too means
  `ahm --root <legacy> status` names the layout instead of reporting missing
  metadata, so no command reads a legacy tree as if it were managed.
  Date/Author: 2026-09-20, Travis Ennis (executed under task 264d).

- Decision: milestone 5 implements `ahm init` as create-or-reconcile and
  changes its diagnostic report to the reconcile set: `created`, `updated`,
  `directories`, and the stale `indexes`, with nothing printed for an up-to-date
  repository.
  Rationale: the acceptance criterion is that a second run writes nothing, and
  a report that always names the metadata file and every index target cannot
  show that. `created` and `updated` are computed by comparing the bytes ahm
  would write against the bytes on disk, so the report and the write set cannot
  disagree; the old `removed`, `skipped`, `conflicts`, and `adopted` keys
  described upgrade behavior that no longer exists. The shape change is
  breaking and belongs in the v2 release notes.
  Date/Author: 2026-09-20, Travis Ennis (executed under task 264d).

- Decision: `ahm init` reconciles the managed `.ahm/.gitignore` to ahm's own
  content whenever it differs, with no ownership hash and no `--force` gate; the
  retired managed files are handled by discarding their stale hashes only.
  Rationale: `.ahm/.gitignore` is a tool-owned file that declares ahm as its
  manager, the milestone exists partly to prune drifted ahm-owned state, and
  hash-gated rewriting would reintroduce the conflict reporting this milestone
  deletes. The retired instruction templates, skills, and scaffold READMEs are
  the opposite case — project-owned or no longer generated — so no command may
  remove them, which is what ADR 022's release treatment requires.
  Date/Author: 2026-09-20, Travis Ennis (executed under task 264d).

- Decision: `--force` keeps its place in the global flag set even though install
  no longer has anything to force, and the `init` help text stops advertising
  it.
  Rationale: `task complete` still reads it as the strict-acceptance override,
  so removing the flag would be a second breaking change the ADR does not ask
  for, and the acceptance notes themselves exercise `ahm init --force`.
  Date/Author: 2026-09-20, Travis Ennis (executed under task 264d).

- Decision: the legacy ADR validation finding is reworded rather than removed:
  `adr_legacy_format` now says to convert the record to MADR front matter by
  hand.
  Rationale: the finding code and severity are compatibility surfaces and
  legacy ADRs still exist in the wild, but its message was the last place a
  validator told a reader to run a command this release deletes. ADR 022 keeps
  record validation; it removes the migration that used to satisfy it.
  Date/Author: 2026-09-20, Travis Ennis (executed under task 264d).

- Decision: milestone 5 applied the prose rule to every live document that named
  `ahm upgrade`, `records migrate`, `records doctor`, `task migrate`, or
  `adr migrate`, and wrote the v2 migration note into
  `docs/guides/workflow-upgrades.md` now, while leaving that guide's dated
  sections and the milestone-6 files alone.
  Rationale: the prose rule's first exception is the migration documentation
  ADR 022 requires, so the note is the one place the removed commands may
  appear; the dated sections are release history, and milestone 6 owns the
  coherent rewrite of `README.md`, `.agents/`, and the architecture prose. The
  alternative — leaving a live document that offers a command the binary no
  longer has — is the defect this milestone would otherwise create.
  Date/Author: 2026-09-20, Travis Ennis (executed under task 264d).

- Decision: milestone 4 moves this plan into `docs/exec-plans/active/`,
  creates `docs/exec-plans/completed/`, and registers both buckets in
  `docs/README.md`, `docs/guardrails/documentation.md`, and `AGENTS.md`.
  Rationale: the Decision Log already places project-owned design plans under
  `docs/exec-plans/`; moving the file in the milestone that creates the
  directories keeps the active bucket real instead of an empty directory with a
  placeholder, and it makes the plan document point at the retired family's
  replacement, which is the milestone's acceptance criterion. `ADR 022`'s
  reference to the plan, task 264's `exec_plan` value, and the plan's own
  orientation were updated with the move. The retired `.ahm/exec-plans/`
  content, including its generated indexes, stays in place as ADR 022 requires.
  Date/Author: 2026-09-20, Travis Ennis (executed under task 264c).

- Decision: milestone 4 applies the instruction clause of the prose rule to
  every live document in `AGENTS.md`, `CONTRIBUTING.md`, and `docs/`, and
  leaves the positive rewrite to milestone 6. Concretely, this milestone
  removed or rewrote the procedure-channel and removed-family prose in
  `docs/VISION.md`, `docs/README.md`, `docs/cli.md`, `docs/references/cli/*`,
  `docs/references/workflow-spec.md`, `docs/references/glossary.md`,
  `docs/references/cli/task-file-format.md`, and both touched guardrails; it
  left `README.md`, `ARCHITECTURE.md`'s remaining prose, `.agents/prompt.md`,
  `.agents/skills/*`, and the dated sections of
  `docs/guides/workflow-upgrades.md` to milestone 6, whose criterion one covers
  every live document and whose Work paragraph names those files.
  Rationale: the acceptance notes are the contract, and a live document that
  offers a command the binary no longer has is a defect this milestone creates;
  the guide is a dated release-history document, which the prose rule's second
  exception covers. Milestone 6 still owns the coherent rewrite, and it lands
  after milestone 5 so it is written once against the final tree.
  Date/Author: 2026-09-20, Travis Ennis (executed under task 264c).

- Decision: `ahm prime` prints no command name at all, not even the
  run-`ahm doctor` pointer in its validation line or the ready-overflow
  pointer. The overflow became a bare count (`3 more ready`), the blocked and
  open counts lost their parentheticals, the `## Useful Commands` section is
  gone, and the JSON report lost its `commands` array.
  Rationale: the milestone's second acceptance criterion is that prime
  contains no text that names a command to run or a step in a workflow, and
  ADR 022 says prime becomes pure state: regenerated indexes, validation
  findings, and record counts. The output-shape change is breaking and must
  appear in the v2 release notes.
  Date/Author: 2026-09-20, Travis Ennis (executed under task 264c).

- Decision: the salvaged procedures are rewrites, not copies. `docs/workflow/
  tasks.md` loses research routing and the `exec_plan` lifecycle step;
  `docs/workflow/exec-plans.md` points at `docs/exec-plans/{active,completed}/`
  and drops the `ahm index` and `exec_plan` steps; `docs/workflow/adrs.md`
  points at the task workflow instead of a removed command; and all three state
  that they are project-owned prose the binary neither prints nor validates.
  Rationale: the procedure content is worth keeping, but only the tool's
  authority over it was the problem. A copy that still told a reader to run
  `ahm context` or to set `exec_plan` would be wrong on the day it landed.
  Date/Author: 2026-09-20, Travis Ennis (executed under task 264c).

- Decision: the `info` finding severity keeps its field and rendering but
  loses its only producer, `validationReport.addInfo`.
  Rationale: the field is part of the `status`/`doctor` JSON shape, which is a
  declared compatibility surface, and milestone 3's deletion-invariance test
  compares all three severities; only the method was dead once the onboarding
  finding left.
  Date/Author: 2026-09-20, Travis Ennis (executed under task 264c).

- Decision: milestone 2 deletes `scripts/task-workflow.sh` even though the
  plan's deletion list does not name it, because the milestone's acceptance
  criterion — no code, recipe, script, or guardrail references session capture
  or resume — is the contract and the script is exactly that plumbing.
  Rationale: milestone 1 established the precedent by resolving the same
  tension in favour of the criterion over the enumerated list; the script is a
  shell reimplementation of the command this milestone removes. It stays
  recoverable from Git history, and a project that wants a local script of its
  own can keep one outside `ahm`'s boundary.
  Date/Author: 2026-09-20, Travis Ennis (executed under task 264a).

- Decision: references whose target this milestone deletes or that name its
  removed plumbing are fixed here; the rest of the prose that names removed
  commands is left for milestones 4 and 6, as the plan intends, and the
  specific spots are recorded here so the next worker does not have to
  rediscover them: `README.md`, `docs/VISION.md`, `docs/cli.md`,
  `docs/references/cli/commands.md`, `docs/references/cli/task-commands.md`,
  `docs/references/cli/global-contract.md`, the `taskWork` config section of
  `docs/references/workflow-spec.md`, `docs/guides/workflow-upgrades.md`
  (line 214), the remaining delegation rows in `docs/references/glossary.md`,
  `ARCHITECTURE.md`'s task module map, `.agents/prompt.md`,
  `.agents/skills/*/SKILL.md`, and `internal/templates/workflow/TASKS.md`
  (line 81) and `ADR.md` (line 22).
  Rationale: this milestone changes code, and the plan sequences the prose
  rewrite after the deletions so that it is written once against the final
  tree.
  Date/Author: 2026-09-20, Travis Ennis (executed under task 264a).

- Decision: `default_work_agent` is retired in this milestone alongside
  `taskWork`, both consumed on read so the next metadata write drops them.
  Rationale: its only reader was `task_agents.go`, deleted here, so leaving the
  typed field would keep a configuration knob that controls nothing; the first
  review round of this milestone raised exactly that, and ADR 022's list of
  retained keys (version, acceptance, managed-file hashes) excludes it. Removal
  is what milestone 5 would otherwise have had to do, and milestone 5's
  remaining work — idempotent `ahm init`, legacy-layout removal, and dropping
  unknown obsolete keys — is unaffected.
  Date/Author: 2026-09-20, Travis Ennis (executed under task 264a).

- Decision: milestone 1 cancels every task whose subject is a removed surface,
  including five the milestone lists did not name (229, 250, 251, 255, 256),
  and re-scopes the tasks whose subject survives.
  Rationale: the milestone's own acceptance criterion — no non-terminal task
  may require a removed command, the research or ExecPlan families, or
  delegated task work — is the contract, and the enumerated ids were a starting
  point. Twenty-eight tasks were cancelled and seven were re-scoped: 155, 168,
  236, 249, 254, 257, and 258 keep their subject and lose only the requirements
  that named removed surfaces.
  Date/Author: 2026-09-20, Travis Ennis (executed under task 264g).

- Decision: cancel task 256 (store export and import) rather than re-scope it to
  tasks and ADRs.
  Rationale: as scoped, its payload was research notes and ExecPlans, and it
  existed to serve the external-store design that this plan rejects; both
  layouts it supported are gone too. The thinner case that survives —
  snapshotting or moving the task and ADR backlog without Git — is a new
  product question, not a cleanup, and the reduced boundary does not answer
  it. The record is cancelled with a reason that says so and can be reopened
  with a task-and-ADR-only scope.
  Date/Author: 2026-09-20, Travis Ennis (executed under task 264g).

- Decision: tasks 235, 237, and 238 are cancelled as superseded by task 264e,
  and task 236's dependency moves from 235 to 264e.
  Rationale: 264e rewrites the same prose surfaces and the same
  `ARCHITECTURE.md` module map after the code deletion, so the pre-ADR-022
  reconciliations have no satisfiable baseline, while 236's mechanical parity
  test still does.
  Date/Author: 2026-09-20, Travis Ennis (executed under task 264g).

- Decision: the prose criteria of the documentation milestones are stated once,
  in the `### The prose rule for removals` subsection of Validation and
  Acceptance, and tasks 264c, 264e, and 264f reference it rather than each
  restating an absolute prohibition.
  Rationale: three independent prohibitions on naming a removed command
  contradicted the prose ADR 022 requires in the same release — the migration
  note that names the final v1 `ahm upgrade`, the release notes that must name
  every removal, and the ADRs themselves — and a review of milestone 1 found a
  fresh instance each round. One rule with one exception list is checkable; the
  restatements were not.
  Date/Author: 2026-09-20, Travis Ennis.

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

- Decision: milestone 3 leaves `internal/templates/workflow/RESEARCH.md` and
  `PLANS.md` in place and deletes only the three templates nothing renders, so
  that milestone 4 can still salvage the ExecPlan procedure with
  `ahm context plan > docs/workflow/exec-plans.md` as this plan's Concrete
  Steps and Decision Log require. The task's own acceptance notes, not its
  Summary, are the contract, and none of them mention a template; the plan's
  step dependency is explicit.
  Rationale: the two instruction templates are the procedure channel's payload,
  and milestone 4 deletes that channel wholesale, so their removal belongs
  there with `context.go`; milestone 3 still removes everything that made
  research and ExecPlans record families. The index templates it does delete
  are dead weight: `research-index.md`, `exec-plans-active-index.md`, and
  `exec-plans-completed-index.md` were duplicated by the Go renderers that this
  milestone removes.
  Date/Author: 2026-09-20, Travis Ennis (executed under task 264b).

- Decision: the retired `exec_plan` field keeps its original front-matter slot.
  `metaExtra` still treats it as a preserved unknown field, but `renderTask`
  re-emits it between `labels` and `depends_on` and skips it in the sorted
  preserved block.
  Rationale: the acceptance notes promise a byte-identical round trip, and
  moving a v1 file's field would rewrite every task record in a consumer's
  repository on its next lifecycle command. Avoiding that churn is worth five
  lines and one named constant.
  Date/Author: 2026-09-20, Travis Ennis (executed under task 264b).

- Decision: the generated task index loses its `ExecPlan` column in this
  milestone rather than keeping a column fed from the preserved field.
  Rationale: the column's value came from the retired schema, and the plan
  retires the family rather than the field alone; a column of unresolvable plan
  references presents a removed family as current state. The index format is a
  declared compatibility surface, so `docs/references/workflow-spec.md` and
  `docs/guides/workflow-upgrades.md` must record the change in milestone 6.
  Date/Author: 2026-09-20, Travis Ennis (executed under task 264b).

- Decision: milestone 3 retires the `research` metadata block and the stale
  `files` hashes for the six retired index paths, in the same milestone that
  deletes their readers.
  Rationale: milestone 2 set the precedent for `taskWork` and
  `default_work_agent` — a typed field whose reader is gone is a configuration
  knob that controls nothing — and milestone 5's remaining work (idempotent
  `ahm init`, legacy-layout removal, unknown-key pruning) is unaffected.
  `retiredGeneratedIndexes` rather than `preservedScaffoldFiles` keeps the
  "no longer generated" case separate from the "scaffold README" case, because
  the two are relinquished for different reasons.
  Date/Author: 2026-09-20, Travis Ennis (executed under task 264b).

- Decision: milestone 3 corrects the help text and `.ahm/.gitignore` that
  claimed ahm manages research notes and their indexes, and leaves the rest of
  the prose naming the retired families to milestones 4 and 6.
  Rationale: this milestone's own claim about itself is wrong the moment it
  lands, which is a defect, while a `context` scope that still describes the
  families is milestone 4's stated subject. Milestone 6 inherits the list of
  documentation spots recorded in the task notes.
  Date/Author: 2026-09-20, Travis Ennis (executed under task 264b).

- Decision: milestone 3 leaves the two milestone-4 items (`RESEARCH.md`,
  `PLANS.md`) and the legacy `.ahm/.gitignore` pruning recorded rather than
  implemented, and hands them to the milestones that own them.
  Rationale: both are stated in the plan's own sequencing, and a third
  independent rewrite of the procedure templates would duplicate milestone 4.
  Date/Author: 2026-09-20, Travis Ennis (executed under task 264b).

- Decision: milestone order is 264g, 264a, 264b, 264c, 264d, 264e, 264f, with
  each milestone as its own commit on `master` (a `feat/<slug>` branch is
  optional and only for isolation).
  Rationale: the deletion milestones are independent and each leaves the tree
  green, so a failure is isolated to one commit that can be reverted; the
  documentation rewrite must follow the code it describes; the release must
  come last.
  Date/Author: 2026-09-20, Travis Ennis.

- Decision: this repository works directly on `master` with no pull requests,
  reversing the workflow that ExecPlan 263 adopted on 2026-08-01.
  Rationale: one maintainer and CI on every push meant a branch plus pull
  request added ceremony without a gate. The guard hook and its script, the
  required-pull-request rule on GitHub, and the release-branch flow are removed
  together, because the local hook alone would keep refusing commits on
  `master`. Only the owner can drop the GitHub rule; the agent environment
  refuses that API call.
  Date/Author: 2026-09-20, Travis Ennis.

- Decision: leave the blank-line-separated `- Supersedes …` items that
  `ahm adr supersede` writes into ADR 022's `More Information` section as the
  tool produced them, rather than hand-tidying them into one list.
  Rationale: ADR-015 and ADR-021 already carry that layout, so ADR 022 matches
  repository precedent and stays stable when the tool appends another item;
  hand-tidying would have to be repeated on every future supersession.
  Date/Author: 2026-09-20, Travis Ennis.

## Outcomes & Retrospective

- (2026-09-26) Milestone 7 (task 264f). Outcome: `v2.0.0` is tagged on
  `396bf3c` and published with all six target archives; the release body is
  `docs/releases/v2.0.0.md`, which names every removed command, configuration
  key, and record family and gives the `ahm init` and home-store steps. The
  release forced three things the plan did not foresee, each recorded in the
  Surprises and Decision Log: no published release could perform the legacy
  `.agents/ahm.json` migration the plan's prose rule depends on, so `v1.1.0`
  was cut from `bcb8ae0` as the waypoint; the GitHub Release body was a
  generated commit list that cannot state a removal, so per-tag release notes
  became a documented release step; and `just prepare-release` produced a
  changelog that failed `docs-md-lint`, so the documented release flow could
  not produce a green release commit. `master` was also red on
  `windows-latest` before this milestone - six test failures from a
  `payloadPath` separator regression introduced by the home store - which the
  milestone's CI-green acceptance caught. Lesson: a release milestone is the
  first time the whole documented pipeline actually runs, and every step in it
  that had only ever been run in pieces was broken.

- (2026-09-20) Milestone 4 (task 264c). Outcome: `ahm` no longer prints
  instructions. `context`, `onboard`, their templates, and the two remaining
  procedure templates are deleted; prime is a pure state report with no command
  names; the doctor no longer looks for an onboarding snippet; and the three
  surviving procedures are project-owned documents under `docs/workflow/` with
  design plans under `docs/exec-plans/`. Against the original purpose — a tool
  that manages records and no longer tells a project how to work — the milestone
  completes the removal half of the plan; the documentation rewrite (milestone
  6) and the release (milestone 7) remain. The gap it leaves is deliberate and
  recorded: `README.md`, `ARCHITECTURE.md`'s remaining prose, `.agents/*`, and
  the dated sections of `docs/guides/workflow-upgrades.md` still describe
  removed surfaces, and the retired generated indexes are stale. Lesson: the
  acceptance notes, not the Work paragraph, defined how far the prose sweep had
  to reach, and the boundary between this milestone and milestone 6 had to be
  decided explicitly rather than inferred from the handoff list.

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
reason cites ADR 022. Task 263 is the second half of the milestone and is
already resolved: the workflow reversal above cancelled it and 263g and moved
`263-adopt-feature-branch-development.md` to `.ahm/exec-plans/completed/`, so
that step is a check rather than an action.

Disposition (2026-09-20): 28 tasks cancelled, each with a reason that cites
ADR 022 — 163, 171, 185, 187, 188, 189, 189b, 189c, 189d, 189e, 189f, 189g,
205, 218, 222, 223, 229, 231, 235, 237, 238, 245, 246, 247, 250, 251, 255,
256. Seven survivors re-scoped to the reduced tool — 155, 168, 236, 249, 254,
257, 258 — with task 236's dependency moved from 235 to 264e. Tracker 263 was
already cancelled with 263g and its ExecPlan already in `completed/` before
this pass, so nothing was left to close.

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

Disposition (2026-09-20): every named file is gone, and two deletions the
plan's list missed are recorded in the Surprises and Decision Log sections:
`internal/ahm/task_commands_test.go` lost 2,317 of its 5,726 lines (the
delegation tests and their stubs), and `scripts/task-workflow.sh` was deleted
because it implements cake session capture and resume. `task_acceptance.go`
and `markdown_sections.go` both survive: the task lifecycle still uses
`parseAcceptanceNotes`, `locateHeadingSections`, and their helpers, so only the
groom-specific test in `markdown_sections_test.go` was removed. The typed
`taskWork` and `default_work_agent` config fields are gone and both keys are
consumed on read so the next metadata write drops them; `.ahm/config.json` in
this repository was hand-edited and then verified with `ahm index` and
`ahm doctor`.

Result: the binary no longer knows how to run another program on your behalf,
and no test reaches the network.

Proof: `ahm task work 1`, `ahm audit`, and `ahm task groom` each fail with
`unknown command`; `ahm task --help` lists neither `groom` nor `work`;
`just ci` passes; `rg -n "smoke-agents|promptFile|taskWork" --glob '!.ahm/**' .`
is a review aid, not the criterion: a worker reads every hit and applies the
prose rule for removals to it.

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

Disposition (2026-09-20): every validator, index renderer, prime section, path
resolver, directory creation, and config reader is gone, and the task's
`exec_plan` field is out of the schema and out of the required front-matter
list. Three items in the work list were resolved differently and the reasons
are in the Decision Log: `RESEARCH.md` and `PLANS.md` stay for milestone 4's
salvage step, `workflow_paths.go` keeps `researchRel` and `execPlansRel`
because the `ahm context` renderer still resolves those strings
(`execPlansDir`, the disk resolver, is gone), and prose that names the retired
families is left to milestones 4 and 6 except where help text and the managed
`.ahm/.gitignore` had become false claims. Two further changes were needed to
make the milestone's Result true rather than nearly true: `ensureWorkflowDirs`
no longer creates the retired directories, since recreating them on `ahm init`
would be observable behavior, and the metadata `research` block plus the stale
`files` hashes for the six retired index paths are relinquished so no
readerless state survives. Two independent review rounds shaped the pass: the
first found that a sorted preserved `exec_plan` field could not round-trip a v1
task file byte-identically, and the second that the coverage tests did not
prove deletion invariance, which `TestRetiredRecordFamiliesChangeNothing` now
does by reading every touched path through the existing instrumentation,
deleting both trees, and comparing all three finding severities.

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
`internal/ahm/cli.go`. Milestone 3 left all four instruction templates behind
because this milestone is their only remaining consumer, so delete
`internal/templates/workflow/RESEARCH.md`,
`internal/templates/workflow/PLANS.md`,
`internal/templates/workflow/TASKS.md`, and
`internal/templates/workflow/ADR.md` here as well, together with the template
cases in `internal/templates/templates_test.go` that pin the research, task,
and ExecPlan procedures. In `internal/ahm/prime.go`, delete the routing block,
the `Managed Work Intake` section, and the grooming and audit hints, leaving:
regenerated indexes, validation findings, and record counts. In
`internal/ahm/status.go`, delete the onboarding snippet. Prose in the salvaged
documents that describes contributor practice rather than records —
verification commands, commit workflow — belongs in `CONTRIBUTING.md`; prose
that routes a reader to a record family belongs in `AGENTS.md`. Both are hand
edits.

Result: no command emits workflow instructions, and no live document instructs
a reader to run a removed command, under the prose rule for removals in
Validation and Acceptance.

Proof: `ahm context task`, `ahm context plan`, `ahm context adr`, and
`ahm onboard` fail with `unknown command`; `ahm prime` output contains no
command name; `docs/workflow/tasks.md`, `docs/workflow/exec-plans.md`, and
`docs/workflow/adrs.md` exist, contain no `{{` template variables, and instruct
no reader to run a removed command;
`rg -n "ahm (audit|context|onboard|upgrade)|task groom|task work" --glob '!.ahm/**' .`
is a review aid, not the criterion: a worker reads every hit and applies the
prose rule for removals to it; `just docs-md-lint` passes.

Disposition (2026-09-20): every named file is gone, plus the two dead path
helpers the deletion exposed in `internal/ahm/workflow_paths.go` and the
`validationReport.addInfo` method whose only caller was the onboarding finding.
The salvage step ran against a binary built from the pre-milestone tree
(`go build -o /tmp/ahm-m4 ./cmd/ahm`, then `/tmp/ahm-m4 --root . context
{task,plan,adr}`), which is the same rendering the milestone's Concrete Steps
produce, and the three documents were edited to concrete paths, project-owned
framing, and no removed-command instructions. `docs/exec-plans/active/` and
`docs/exec-plans/completed/` exist, this plan moved into the active bucket, and
`docs/exec-plans/README.md` explains the buckets. Prime lost its routing block,
its `## Useful Commands` section, and every parenthetical command pointer,
including the one in its validation line; status and doctor lost the onboarding
finding. The prose sweep covered `AGENTS.md`, `CONTRIBUTING.md`, and every live
document under `docs/` except the ADRs, the dated sections of the upgrade guide,
and this plan's own history; the files left to milestone 6 and the reasoning are
in the Decision Log and Surprises sections. One independent review round ran
against the final tree (`codex exec --sandbox read-only`, which reported four
findings: the dirty-worktree advice prescribing a step, an ambiguous
scaffold-preservation sentence in the workflow spec, the preflight skill's
`ahm context` references, and "commit frequently" in the salvaged plan guide);
all four are fixed, and the follow-up round could not run because the reviewer
CLI hit its usage limit. That deferral is recorded in the Surprises section;
the probe that would establish it is a fresh review pass over this commit.
`just ci` and `just docs-md-lint` pass on the final tree.

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

Disposition (2026-09-20): done, and wider than the Work paragraph. Deleted:
`records_commands.go`, `records_migrate.go`, `task_migrate.go`,
`adr_migrate.go`, their tests, `records.go` (the `runGit`/`runGitBytes` helpers
whose only caller was the records migration), and the three
`dir_notempty*` files plus their test once the upgrade-time obsolete-file
removal was gone. `init` is now create-or-reconcile: it writes a file only when
the bytes differ from what ahm owns, which makes a second run silent and
byte-identical, and it drops the obsolete ahm-owned configuration keys while
preserving unknown metadata and `files` hashes. The retired-file ownership
boundary replaces upgrade's removal and conflict reporting: `ahm init` discards
stale hashes and never creates, inspects, overwrites, or removes those files.
Root detection accepts only `.git` and `.ahm/config.json` and refuses a
`.agents/ahm.json` repository with a message naming `v1.0.0`; the collapse also
dropped the layout resolver, the path cache, and the migration lock namespace,
leaving one `.ahm/.lock/workflow-records` lock. Test-suite changes went well
beyond the named files: `install_test.go` was rewritten around the idempotence,
obsolete-key, `AGENTS.md`, and legacy-refusal criteria; the `current/legacy`
layout tables in `validation_test.go` collapsed to one layout; and about 180
fixtures across `task_commands_test.go`, `task_deps_test.go`, `tasks_test.go`,
`indexes_test.go`, `lock_test.go`, `task_list_test.go`, `write_test.go`, and
`prime_test.go` were repointed from `.agents/` paths to `.ahm/`.

Result: a repository needs one command to adopt or maintain `ahm`, an
unmodified `AGENTS.md` survives every install path, and no command reads the
retired layout.

Proof: `ahm init` in a scratch repository prints the created files, its second
run prints nothing and leaves every mtime and byte identical, and `ahm init` in
a directory holding only `.agents/ahm.json` exits 1 with the v1 message and
creates nothing.

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
and start work without being told to run a command that no longer exists, and
without meeting a removed record family presented as current.

Proof: `just docs-md-lint` and `just ci` pass;
`rg -n "ahm (audit|context|onboard|upgrade)|task groom|task work|records migrate" docs/ README.md AGENTS.md CONTRIBUTING.md`
is a review aid, not the criterion: a worker reads every hit and applies the
prose rule for removals to it, so no live document offers a removed command to
run and none presents research notes or ExecPlans as an `ahm`-managed record
family.

### Milestone 7 — Release v2.0.0 (task 264f)

Scope: ship the breaking change with honest notes.

Work: follow `docs/release.md`: run the release preparation script on
`master`, regenerate the changelog, and write release notes that name the
removed commands (`audit`, `context`, `onboard`, `task groom`, `task work`,
`upgrade`), the removed record families, the removed configuration key, and the
required `ahm init` step. This repository releases directly from `master` — the
`release/*` branch flow was removed with the workflow reversal — so the tag is
created on the release commit there. A repository on the legacy
`.agents/ahm.json` layout must upgrade with the final v1 release before
adopting v2.

Result: v2.0.0 is tagged and published with a migration note.

Proof: `just release-check` and `just ci` pass on the release commit on
`master`; CI is green on that commit before the tag is pushed; artifacts exist
for the six target platforms.

## Concrete Steps

All commands run from the repository root unless stated otherwise.

To start any milestone:

    git switch master && git pull --ff-only
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
    # then edit all three: concrete paths, no template variables, and no live
    # instruction to run a removed command (the prose rule for removals)
    git rm internal/ahm/context.go internal/ahm/context_test.go \
        internal/ahm/onboard.go internal/ahm/onboard_test.go \
        internal/templates/workflow/RESEARCH.md \
        internal/templates/workflow/PLANS.md \
        internal/templates/workflow/TASKS.md \
        internal/templates/workflow/ADR.md
    go build ./... && just docs-md-lint && just ci

Milestone 5 (264d):

    git rm internal/ahm/records_commands.go internal/ahm/records_migrate.go \
        internal/ahm/records_migrate_test.go internal/ahm/task_migrate.go \
        internal/ahm/task_migrate_test.go internal/ahm/adr_migrate.go
    go build ./... && just ci

Milestone 6 (264e): edit prose, then

    just docs-md-lint && just ci

Milestone 7 (264f): follow `docs/release.md`.

Every milestone ends with a commit on `master`, a push, and CI green on that
commit. Commits use Conventional Commits, for example
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
    ahm task complete <id>
    ahm adr create "Records only" --status accepted
    ahm index && ahm prime

Expect: the task moves through the buckets with its acceptance notes
validated, the ADR appears in `docs/adr/index.md`, and `ahm prime` prints
counts and validation findings without any routing prose.

### The prose rule for removals

One rule governs the documentation criteria of milestones 4, 6, and 7, and
tasks 264c, 264e, and 264f reference it instead of each restating it: no live
document instructs a reader to run a removed command or manage a removed record
family, except for three kinds of text that this release must keep.

1. The migration documentation ADR 022 requires, which names the final v1
   release's `ahm upgrade` so a legacy-layout repository knows what to run
   before adopting v2.
2. The changelog and release notes, which must name every removed command,
   configuration key, and record family.
3. The historical and decision record: the ADRs, including this supersession,
   and the retired `.ahm/research/` and `.ahm/exec-plans/` files that stay on
   disk.

The rule is stated once, in this section, because three separate absolute
prohibitions produced three criteria that no release complying with ADR 022
could satisfy. A milestone worker checks the rule, not a private restatement of
it: naming a removed command in passing is allowed anywhere, and offering one
to run is not. Every name-based search in a milestone proof is a review aid
whose hits the worker reads and applies this rule to; no search is itself a
criterion.

## Idempotence and Recovery

Every step in this plan is a file deletion or a prose edit, and every step is
its own commit on `master`, so recovery is `git revert <hash>`. No step mutates
a database, a remote, or the user's worktree outside the repository.

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

After milestone 6, measured on 2026-09-20 (working tree, uncommitted):

    16 tracked files changed, +189/-541 lines (this plan file is the 17th,
    +127/-1), plus the task record's acceptance notes and its move to
    .ahm/tasks/completed/
    docs/guides/workflow-upgrades.md: 438 lines to 88; its dated pre-v2 release
    history is gone, and its live content is the reconcile behavior, the v2
    migration note, and the legacy-layout migration path
    claims corrected against the code: task-command flags and transitions,
    four managed_file_* finding codes, the workflow scope description, the
    init `files`-pruning wording, status/--root root-detection behavior, the
    module map, and the `adr supersede` preconditions
    no change to docs/VISION.md, docs/README.md's surface list, AGENTS.md's
    routes, or the ownership table in docs/guardrails/documentation.md: all
    four already satisfied their acceptance notes after milestone 4
    .ahm/exec-plans/ and .ahm/research/ are untouched; ADR 022 keeps the
    retired generated indexes in place, and docs/exec-plans/README.md records
    that they are unmaintained
    just ci green: gofmt, go mod tidy -diff, go vet, go test -race -cover
    (88.5% internal/ahm), golangci-lint 0 issues, govulncheck no
    vulnerabilities, markdownlint 62 files 0 issues, build, goreleaser check
    and snapshot
    milestone 7 owes: the release notes, and the legacy-layout release naming
    recorded in the Decision Log

Baseline measured on 2026-09-20 at commit `371fc62`, before any milestone:

    12302 total non-test Go lines in cmd/ and internal/
    18552 total test Go lines across the repository
    112K of agent transcript fixtures under internal/ahm/testdata/agents/
    135 references to the six removed commands across 28 documentation files
    ahm doctor: 1 warning (active ExecPlan with a filled Outcomes section)

After milestone 3, measured on 2026-09-20:

    9244 total non-test Go lines in cmd/ and internal/, down from 9914
    14099 total test Go lines across the repository, down from 14579
    ahm index no longer writes .ahm/research/index.md or .ahm/exec-plans/*/index.md
    ahm doctor and ahm status: "ok": true, no findings, on a repository that
    still holds research notes, ExecPlans, and a task with a dangling exec_plan
    retired in this milestone: research_inbox.go, the research and ExecPlan
    validators, collectMarkdownDocs and its two index renderers, the ExecPlan
    section parser, prime's two briefing sections, the task exec_plan field,
    three dead index templates, and the research metadata block
    untracked after the .ahm/.gitignore narrowing: .ahm/research/index.md,
    .ahm/exec-plans/active/index.md, .ahm/exec-plans/completed/index.md
    milestone 6 owes: docs/references/cli/task-file-format.md (exec_plan field,
    exec-plan and research finding codes), docs/references/workflow-spec.md
    (task front matter, research config block, index shape), docs/cli.md,
    docs/guides/workflow-upgrades.md (v2 migration note), and the glossary

After milestone 5, measured on 2026-09-20:

    7246 total non-test Go lines in cmd/ and internal/, down from 8767
    11900 total test Go lines across the repository, down from 13642
    deleted: records_commands.go, records_migrate.go, task_migrate.go,
    adr_migrate.go, records.go, the three dir_notempty files, and their tests
    ahm init twice in a row: the second run prints nothing, exits 0, and leaves
    every file's bytes and modification time unchanged
    ahm init in a repository holding only .agents/ahm.json: exit 1 with
    "legacy ahm workflow layout ... upgrade the repository with the final v1
    release (ahm v1.0.0) before using this version"
    ahm --help lists adr, doctor, index, init, prime, status, task, and version
    only; upgrade, records, context, onboard, audit, task migrate, and adr
    migrate are all unknown commands
    ahm doctor and ahm status on this repository: "ok": true, no findings
    milestone 6 owes: README.md (the .agents intro, the quickstart, and the
    Safety paragraph), .agents/prompt.md and
    .agents/skills/{grooming-backlog,finding-improvements}, ARCHITECTURE.md's
    remaining system-boundary prose, and the stale retired generated indexes
    under .ahm/exec-plans/ and .ahm/research/
    release notes owe: the removal of upgrade, records migrate, records doctor,
    task migrate, and adr migrate; the init report's new key set; the refusal of
    the .agents/ahm.json layout with v1.0.0 named; and the reworded
    adr_legacy_format finding

After milestone 4, measured on 2026-09-20:

    8767 total non-test Go lines in cmd/ and internal/, down from 9244
    13642 total test Go lines across the repository, down from 14099
    deleted: context.go, context_test.go, onboard.go, onboard_test.go, the
    whole internal/templates package (templates.go, templates_test.go, and the
    four workflow templates), workflowPaths.researchRel, and
    workflowPaths.execPlansRel
    ahm prime: no `commands` array, no routing block, no command pointer; the
    ready overflow reads `3 more ready`
    ahm doctor and ahm status: "ok": true, no findings
    new project-owned docs: docs/workflow/tasks.md, docs/workflow/exec-plans.md,
    docs/workflow/adrs.md, docs/exec-plans/README.md, and
    docs/exec-plans/active/264-reduce-ahm-to-tasks-and-adrs.md (moved)
    milestone 6 owes: README.md (quickstart and safety prose),
    ARCHITECTURE.md's remaining system-boundary and module prose,
    .agents/prompt.md and .agents/skills/{grooming-backlog,finding-improvements},
    docs/guides/workflow-upgrades.md (the v2 migration note, plus whatever the
    dated sections should say), and the stale retired generated indexes under
    .ahm/exec-plans/ and .ahm/research/

After milestone 1, measured on 2026-09-20:

    18 non-terminal task records, down from 46
    28 cancellations, each citing ADR 022
    7 re-scoped survivors, each carrying a dated re-scope comment
    ahm doctor: "ok": true, no findings

After milestone 2, measured on 2026-09-20:

    9914 total non-test Go lines in cmd/ and internal/, down from 12302
    14579 total test Go lines across the repository, down from 18552
    internal/ahm/testdata/ (the 112K of agent fixtures) is gone
    ahm doctor and ahm status: "ok": true, no findings
    milestone 5's deletions total about 1786 non-test lines (context.go,
    onboard.go, records_*.go, and the migration files), which puts the
    post-milestone-5 total near the 7000-8000 the plan projects

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

- (2026-09-20) Milestone 6 (264e) executed. The pass followed the acceptance
  notes rather than the Work paragraph: it swept every live prose surface, then
  corrected the reference pages against the code, which is why stale claims
  about task-command flags and transitions, `managed_file_*` finding codes,
  `files` pruning, and root detection changed in the same commit. Two subagent
  review rounds ran; round one raised five should-fix items and four nits, all
  of them fixed, and round two confirmed the fixes and escalated one release
  naming question to milestone 7. The guide's dated release history was deleted
  rather than rewritten, and the plan records why. `just ci` and
  `just docs-md-lint` pass on the final tree; the milestone is uncommitted at
  handoff.

- (2026-09-20) Milestone 4 (264c) executed. The pass followed the milestone's
  acceptance notes rather than its Work paragraph, which is why the prose sweep
  reached into `docs/VISION.md`, `docs/cli.md`, `docs/references/*`, and the
  guardrails, and why `ARCHITECTURE.md` was corrected where its module map named
  files this milestone deleted. The plan moved into
  `docs/exec-plans/active/` and `docs/exec-plans/` was registered in
  `docs/README.md`, `docs/guardrails/documentation.md`, and `AGENTS.md`. The
  residual items this milestone hands to milestone 6 are listed in the Decision
  Log, the Surprises section, and the Artifacts section. `just ci` and
  `just docs-md-lint` pass on the final tree.

- (2026-09-20) Milestone 3 (264b) executed. The pass followed the milestone's
  acceptance criteria rather than its enumerated deletion list, which is why
  `RESEARCH.md` and `PLANS.md` survive into milestone 4 and why
  `ensureWorkflowDirs`, the metadata `research` block, the task index's
  `ExecPlan` column, and the managed `.ahm/.gitignore` pattern were changed in
  the same commit. Two review rounds ran; the second round's findings are
  folded into the tests, and `just ci` passes on the final tree. The residual
  items this milestone hands to later steps are listed in the task notes and in
  the Surprises and Artifacts sections above.

- (2026-09-20) Milestone 2 (264a) review round. The review ran through
  `codex exec review --uncommitted`, whose nested sandbox could not execute
  commands in this environment (`sandbox-exec: sandbox_apply: Operation not
  permitted`), so the diff, the deleted-file list, and the removed test names
  were supplied inline and the reviewer answered from that text. It raised one
  finding: `default_work_agent` remained typed and readerless configuration.
  That is fixed in the same commit, and the decision entry above records why.
  The unrun probe for future milestones is the repository's own preflight skill
  in a session that can execute commands.

- (2026-09-20) Milestone 2 (264a) executed. The pass matched the milestone's
  acceptance criteria rather than only its deletion list, which is why
  `scripts/task-workflow.sh` went with the delegation surface, why
  `internal/ahm/task_commands_test.go` was trimmed by 2,317 lines, and why the
  glossary's "Agent Delegation" section, the ARCHITECTURE reference bullet,
  `docs/guides/testing.md`, `docs/testing.md`, and the AGENTS.md route into the
  deleted guardrail were edited in the same commit. Prose that names removed
  commands elsewhere is left for milestones 4 and 6, and the Decision Log lists
  every known spot so the next worker does not rediscover them.

- (2026-09-20) Owner decision on the milestone prose criteria: state the rule
  once and point the milestones at it. The `### The prose rule for removals`
  subsection in Validation and Acceptance is now the single statement, and
  tasks 264c, 264e, and 264f reference it instead of repeating it three ways.
  The review of milestone 1 raised the conflict — a prohibition on naming
  removed commands versus the migration note and release notes ADR 022
  requires — and 264e had already been patched twice before the class was
  escalated. Milestone 4 (264c) may start now.

- (2026-09-20) Milestone 1 corrections from an independent review of the pass.
  Three stale requirements that the milestone made visible are fixed here: the
  release milestone no longer creates a `release/v2.0.0` branch, task 264e's
  "no document references a removed command" criterion now excepts the
  historical record and the release notes that must name the removals, and
  milestone 1's Work paragraph no longer describes resolving 263 as pending
  work. Task 264g's notes carry the same list.

- (2026-09-20) Owner decision recorded: design plans live in
  `docs/exec-plans/`, and the procedures that `ahm context` prints are salvaged
  to `docs/workflow/tasks.md`, `docs/workflow/exec-plans.md`, and
  `docs/workflow/adrs.md`. Milestone 4 and the Concrete Steps now salvage the
  prose before deleting `context.go`, and milestone 6 registers the new
  documentation surfaces.
- (2026-09-20) The nine supersessions ADR 022 requires are done, and a second
  ADR defect surfaced while doing them: a replaced ADR lost its final newline.
  The fix normalizes every rewritten ADR to end with one newline, and the
  discovery is recorded above.
- (2026-09-20) Workflow reversal: the repository commits directly to `master`.
  The guard hook, `semantic-pr.yml`, and the release-branch flow are gone, and
  `AGENTS.md`, `CONTRIBUTING.md`, `docs/release.md`, and
  `scripts/prepare-release.sh` describe the new sequence. ExecPlan 263 and task
  263g are cancelled.

  To restore the previous GitHub protection on `master`, re-apply the settings
  captured before the change (also saved at
  `/tmp/ahm-master-protection-backup.json`): required pull request reviews with
  `required_approving_review_count: 0`, required status checks
  `ci (ubuntu-latest)` and `ci (windows-latest)` with `strict: true`,
  `enforce_admins: true`, and force pushes and deletions disabled.
