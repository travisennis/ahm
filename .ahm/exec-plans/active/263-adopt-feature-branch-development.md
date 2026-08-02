# Adopt feature-branch development with worktree support

This ExecPlan is a living document. The sections `Progress`, `Surprises &
Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to
date as work proceeds. This document is maintained in accordance with the
`ahm context plan` guidance; rerun that command before revising the plan.

Tracker: task 263; children 263a through 263g sequence the milestones. One
milestone may be completed by more than one child only where the milestone
paragraph says so; otherwise each milestone maps to exactly one child task.

## Purpose / Big Picture

Today this repository is developed by committing directly on `master` (see the
git history and `AGENTS.md`, which says only "do not commit or push unless
explicitly asked"). After this plan, a human or an agent works on a feature
branch named `feat/<slug>`, CI verifies every push to that branch, direct
commits to `master` fail locally and are blocked on GitHub, and merges happen
only through a pull request. Parallel work becomes possible: each branch gets
its own git worktree, so two agents (or a human and an agent) can implement
different changes at the same time without stepping on each other's files.

You can see it working by doing this after implementation:

```text
git worktree add -b feat/demo ../ahm-demo master
cd ../ahm-demo
ahm prime
# make a small change, then:
git add -A && git commit -m "docs: demo feature branch"
# the pre-commit hook refuses nothing (you are on feat/demo) and CI runs on push
git push -u origin feat/demo
# open a PR; GitHub shows the ci check; master rejects direct pushes
```

The change is a workflow change, not a feature change: it touches setup
(hooks, the documented worktree flow), build/CI (CI triggers, a release
guard), and
instructions (`AGENTS.md`, `CONTRIBUTING.md`, `docs/guardrails/`, and
`.agents/prompt.md`). Nothing in the `ahm` binary itself changes, because ahm
is already branch-scoped and worktree-compatible (see Surprises).

## Repository orientation

A novice executor needs this map before touching anything.

The repo is a Go CLI called `ahm` that manages repo-local workflow records:
tasks under `.ahm/tasks/`, research notes under `.ahm/research/`, and
ExecPlans like this one under `.ahm/exec-plans/`. Source records and
`.ahm/config.json` are ordinary committed files and are branch-scoped: each
git branch carries its own copies, which is exactly what makes feature-branch
work natural. Generated indexes (`index.md`) and the workflow lock (`.lock/`)
are gitignored machine-local state; `ahm prime` and `ahm index` regenerate
them for whatever branch is checked out. `ahm` itself performs no git commits,
pushes, branch operations, or network operations.

The files this plan touches and why:

| File | Role |
| --- | --- |
| `.github/workflows/ci.yml` | CI trigger; currently runs on PRs and pushes to `master` only. |
| `scripts/prepare-release.sh` | Release prep; refuses feature branches, and its printed steps move the changelog commit to a `release/vX.Y.Z` branch + PR. |
| `scripts/hooks/require-feature-branch.sh` | New local enforcement: refuse ALL commits while on `master` (no bypass). |
| `.pre-commit-config.yaml` | Hooks config; gains the new guard hook. |
| `AGENTS.md` | Project agent instructions; gains the feature-branch rule. |
| `CONTRIBUTING.md` | Human contributor docs; gains the branch and worktree workflow. |
| `docs/release.md` | Release checklist; rewritten for the release-branch + PR flow. |
| `docs/guardrails/safety-and-permissions.md` | Clarifies that ahm's "no implicit git ops" boundary is about the binary, not the project agent. |
| `.agents/prompt.md` | Agent prompt; reconciled with the committed-record model and the new workflow. |
| GitHub repository settings (not a file) | Branch protection on `master`; the only real enforcement. |

Hooks in this repo are installed per checkout with `prek install` and
`prek install --hook-type commit-msg` (see `CONTRIBUTING.md` Local Setup).
A git worktree is a second working directory that shares one repository;
`git worktree add -b feat/demo ../ahm-demo master` creates a directory
`../ahm-demo` with a new branch `feat/demo` based on `master`. Each worktree
has its own checkout and its own `.ahm/` files for its branch, but its hooks
directory resolves to the main checkout's `.git/hooks` (the shared common
git dir), so `prek install` runs once at the main checkout and covers every
linked worktree. Verified empirically on 2026-08-01: from a linked worktree,
`git rev-parse --git-path hooks` prints `<main>/.git/hooks`.

The completion order for each child task, from `ahm context plan`: fill the
task's Acceptance Notes checkboxes, update this ExecPlan's `Outcomes &
Retrospective` (and `Progress`) for the milestone, move this ExecPlan to
`.ahm/exec-plans/completed/` only when the whole plan is done, update the
task's `exec_plan` front-matter field, then run `ahm task complete <id>`.
`ahm task complete` regenerates indexes itself.

## Milestones

Each milestone is independently verifiable, and the milestones are ordered so
the cheap, mechanical changes land before the instruction changes that depend
on them.

### Milestone 1 — CI on all pushes and a release master guard (task 263e)

Scope: make CI verify every branch and keep releases master-anchored.

Edit `.github/workflows/ci.yml` so CI runs on every push, not only pushes to
`master`. Replace the `on:` block:

```text
on:
  pull_request:
  push:
    branches:
      - master
```

with:

```text
on:
  pull_request:
  push:
```

The `ci` job and its matrix stay unchanged. Running on all pushes (including
feature branches and tag pushes) costs one extra run per push and makes branch
protection's "require the ci check" meaningful.

Edit `scripts/prepare-release.sh`. Today it computes `current_branch` after
the clean-worktree check and prints `git push origin $current_branch` as the
final step, so a release prepared on a feature branch would be pushed to that
branch. Add a guard at the top of the script, before the svu/git-cliff tool
checks, so a feature branch fails fast with the master-only message without
requiring those tools:

```text
current_branch="$(git branch --show-current)"
if [[ -z "$current_branch" ]]; then
  echo "prepare-release: cannot determine current branch" >&2
  exit 1
fi
if [[ "$current_branch" != "master" ]]; then
  echo "prepare-release: releases are cut from master; current branch is $current_branch" >&2
  exit 1
fi
```

Keep the existing later `current_branch` computation or reuse the guarded one;
the script must not reference an undefined variable after the edit.

With the guard hook (Milestone 3) and branch protection (Milestone 5) in
place, the changelog commit can no longer land on master directly: the hook
blocks the commit and GitHub blocks the push. Replace the release flow
instead of fighting those guards. After `just prepare-release` on master
(clean worktree, guard passes), the generated `CHANGELOG.md` change moves to a
short-lived release branch, is committed there, and merges via PR; the tag is
created on master and pushed directly (tag pushes are not branch-protected):

```text
git checkout -b release/vX.Y.Z        # carries the uncommitted CHANGELOG.md change
git add CHANGELOG.md
git commit -m "chore(release): prepare vX.Y.Z"
git push -u origin release/vX.Y.Z     # open a PR; ci runs; branch protection applies
# after the PR merges:
git checkout master && git pull
git tag -a vX.Y.Z -m "vX.Y.Z"
git push origin vX.Y.Z
```

Update the instructions `scripts/prepare-release.sh` prints (replace
`git push origin $current_branch` with the release-branch and PR steps above)
and rewrite the "Weekly Release Checklist" steps in `docs/release.md`
(commit-and-push steps) to match. The script must keep refusing feature
branches: releases are prepped on master and then moved to the release branch
for the commit and PR.

Acceptance for 263e:

- [ ] Pushing a feature branch to GitHub triggers the `ci` workflow.
- [ ] `scripts/prepare-release.sh` on a feature branch exits non-zero with the
      master-only message; on `master` it generates the changelog and prints
      the release-branch + PR instructions.
- [ ] `docs/release.md` describes the release-branch + PR flow and no longer
      instructs committing the changelog on master or `git push origin master`.
- [ ] `just ci` passes locally.

### Milestone 2 — Document the worktree workflow (task 263a)

Scope: give humans and agents the exact worktree commands without adding a
helper script.

Decision (see Decision Log): no `scripts/new-worktree.sh` and no `justfile`
recipe. Agents manage git worktrees directly (`git worktree add`), and the
operating loop already mandates `ahm prime` before any work, which regenerates
the branch-scoped indexes a fresh worktree needs. A script would duplicate
git's own command surface for little gain.

Add a "Worktree Setup" subsection to `CONTRIBUTING.md` under Local Setup with
exactly this content:

- Worktrees are the parallel-work mechanism: each worktree is a separate
  working directory sharing one repository, so two agents can implement
different branches at once.
- Create one for a task with `git worktree add -b feat/<slug> ../ahm-<slug>
  master` from the repository root, then `cd ../ahm-<slug>` and run `ahm
  prime` before any work (prime regenerates the branch's indexes and prints
the briefing).
- Linked worktrees share the main checkout's hooks directory (verify with
  `git rev-parse --git-path hooks` from the worktree), so one `prek install`
at the main checkout covers all worktrees and no per-worktree hook install is
needed.
- Cleanup after the PR merges: `git worktree remove ../ahm-<slug>` and
  `git branch -d feat/<slug>`.

Acceptance for 263a:

- [ ] `CONTRIBUTING.md` documents the worktree create, prime, and cleanup
      commands and the shared-hooks behavior.
- [ ] Following the documented commands creates a working worktree whose
      `ahm prime` prints its branch.
- [ ] No new script or justfile recipe was added (the workflow is documented,
      not scripted).
- [ ] `just ci` passes on `master`.

### Milestone 3 — Local master-commit guard (task 263d)

Scope: refuse ALL commits on `master` locally, with no bypass and no
exceptions. Everything lands on a branch; after Milestone 1 there is no
legitimate master commit, including release prep, so the hook can be
unconditional.

Create `scripts/hooks/require-feature-branch.sh` and wire it into
`.pre-commit-config.yaml` as a `pre-commit`-stage hook alongside the existing
`go-*` hooks. The hook fails when `git branch --show-current` reports
`master`. Reference implementation:

```text
#!/usr/bin/env bash
set -euo pipefail

branch="$(git branch --show-current 2>/dev/null || true)"
if [[ "$branch" == "master" ]]; then
  echo "error: direct commits to master are disabled; work on a feat/<slug> branch and merge via PR" >&2
  exit 1
fi
exit 0
```

The `.pre-commit-config.yaml` entry follows the existing local-hook pattern:

```text
- id: require-feature-branch
  name: require feature branch
  entry: scripts/hooks/require-feature-branch.sh
  language: system
  pass_filenames: false
  stages: [pre-commit]
```

There is deliberately no override: no environment variable, no message
prefix, no exception in the hook itself. Any escape hatch is a loophole an
agent would find and exploit, and the release flow in Milestone 1 removes the
only previously legitimate master commit. State plainly in the task notes and
commit message that this hook is a local nudge: it cannot stop a push to
`master` (GitHub branch protection in Milestone 5 is the real gate), and it
does not apply in checkouts that never ran `prek install`.

Acceptance for 263d:

- [ ] On `master`, `git commit` fails with the guard message.
- [ ] Reading the hook confirms there is no conditional on any environment
      variable and no bypass.
- [ ] On `feat/<slug>`, `git commit` is unaffected.
- [ ] `prek run --all-files` (or the equivalent invocation) passes.
- [ ] `just ci` passes.

### Milestone 4 — Feature-branch rules in AGENTS.md and CONTRIBUTING.md (task 263c)

Scope: make the feature-branch workflow the documented default for agents and
humans, and clarify the ahm git boundary.

These are behavior-shaping instruction edits. Per
`docs/guardrails/agent-instructions.md`, the commit message for this task must
name the motivating failure, state the observable behavior the edit is
expected to change, and either run the fresh-session probe below or record
that verification is deferred and which probe would establish it.

Edit `AGENTS.md`. In the Repository Rules section (the numbered list near the
bottom), add a feature-branch rule that says: nothing is ever committed
directly to `master` — not development work, not planning or intake records,
not release prep (release commits live on a `release/vX.Y.Z` branch); work
happens on `feat/<slug>` branches and merges to `master` only through a pull
request with CI green; "commit or push unless explicitly asked" now means a
task or instruction that names branch work authorizes commits on the feature
branch, and pushing and opening a PR require an explicit instruction or a
Milestone-6-style proof step; after finishing, hand off with the branch name
and whether a PR was opened. Keep the rule short; details belong in
`CONTRIBUTING.md`.

Edit `CONTRIBUTING.md`. Replace or expand the "Commit And PR Workflow" section
so it describes: branch naming (`feat/<slug>`), the standard sequence (create
the branch or worktree, implement, `ahm prime` before work and after
checkouts, commit on the branch with Conventional Commits, push, open a PR,
wait for CI, merge, then `ahm prime` on `master` to regenerate indexes), a
sync strategy (rebase the branch onto `master` before opening or updating the
PR), the master-commit guard hook from Milestone 3 (no bypass), and the
release-branch flow from Milestone 1.

Edit `docs/guardrails/safety-and-permissions.md`. Its compatibility surface
"No implicit git commits, pushes, PRs, or branch operations" describes the
`ahm` binary's boundary. Add one clarifying sentence: this is a property of
the `ahm` tool, not a prohibition on the human or agent working in the
repository, whose commit and branch behavior is governed by `AGENTS.md` and
`CONTRIBUTING.md`.

The fresh-session probe (run it, or record deferral with this exact probe): in
a fresh agent session, give a task whose instructions say "implement on a
feature branch and open a PR", then check that the agent states the
feature-branch rule from `AGENTS.md`, creates `feat/<slug>`, commits only on
that branch, and opens a PR rather than committing to `master`.

Acceptance for 263c:

- [ ] `AGENTS.md` Repository Rules contains the feature-branch rule.
- [ ] `CONTRIBUTING.md` describes branch naming, the commit-push-PR-merge
      sequence, the sync strategy, and post-merge `ahm prime`.
- [ ] `docs/guardrails/safety-and-permissions.md` distinguishes the ahm binary
      boundary from project-agent behavior.
- [ ] The commit message names the motivating failure, the expected behavior
      change, and the probe result or deferral.
- [ ] `just docs-md-lint` and `just ci` pass.

### Milestone 4b — Reconcile .agents/prompt.md (task 263f)

Scope: remove the stale ref-backed vision from the agent prompt without
weakening ahm's git-safety boundary.

`.agents/prompt.md` was written for the superseded ref-backed records vision:
it tells the agent to write records "under refs/`ahm`/*", and its
Git-safety-boundary bullet says "never commit, stage, touch the index or
HEAD, mutate branches, open PRs". ADR 015 accepted committed `.ahm/` records
and ahm performs no ref operations, so those sentences describe a design that
no longer exists. The commit- and PR-prohibition also now conflicts with the
feature-branch workflow from Milestone 4, where the agent working on a task
commits on its own branch.

Rewrite the Git-safety boundary bullet so it states the binary boundary
accurately: `ahm` must never commit, stage, touch the index or HEAD, mutate
branches, open PRs, or patch project source on its own; the agent executing a
task in the repository follows `AGENTS.md` and commits on its feature branch.
Remove the `refs/ahm/*` mention or replace it with the committed-record
description (`.ahm/` source records, gitignored indexes). Preserve the
"coordinate parallel trees" section unchanged; it stays valid for parallel
work. This is a behavior-shaping edit: same evidence-and-probe requirement as
Milestone 4, and the probe can be the same fresh-session run.

Acceptance for 263f:

- [ ] `.agents/prompt.md` contains no reference to ref-backed records or
      `refs/ahm/*`.
- [ ] The Git-safety bullet describes the ahm binary's boundary and defers
      the agent's own commit behavior to `AGENTS.md`.
- [ ] The "coordinate parallel trees" guidance is intact.
- [ ] The commit message carries the evidence and probe record.
- [ ] `just docs-md-lint` passes (or the file's linter, if prompt.md is
      outside the lint glob, state that).

### Milestone 5 — GitHub branch protection (task 263b, manual)

Scope: the only real enforcement, done by a human in GitHub settings, not in
repository files.

This task has no code. The executor (or the repository owner) must open the
repository settings page, Branch protection rules for `master`, and enable:
require a pull request before merging; require the `ci` status check to pass;
require branches to be up to date before merging; and block direct pushes
(which is implied by requiring a PR). Self-merge after a green `ci` check is
allowed: the owner is the only committer and the point of protection is
blocking direct pushes, not adding review overhead. No actor or rule is
exempted from the required-PR rule.

Acceptance for 263b:

- [ ] Settings show `master` protected with required PRs, required `ci`
      check, and up-to-date branches.
- [ ] Self-merge of a green PR is allowed; no reviewer is required.
- [ ] A direct `git push origin master` is rejected by GitHub.
- [ ] A PR with a green `ci` check can be merged.
- [ ] The task body records which settings were changed and the date.

### Milestone 6 — End-to-end proof (task 263g)

Scope: demonstrate the whole loop, including two parallel worktrees.

This milestone runs after 263a through 263f are complete. It is a proof, not
new code. It requires explicit authorization to push branches and open pull
requests on GitHub (granted by the repository owner on 2026-08-01); the `gh`
CLI is available (version 2.97.0, verified 2026-08-01). Execute and record a
transcript:

First, two parallel worktrees. From the repository root, run
`git worktree add -b feat/alpha ../ahm-alpha master` and
`git worktree add -b feat/beta ../ahm-beta master`, then `ahm prime` in each
worktree, then in `../ahm-alpha` commit
a small docs-only change on `feat/alpha` (e.g., a comment improvement in
`justfile` or a `CONTRIBUTING.md` sentence) and in `../ahm-beta` commit a
different docs-only change on `feat/beta`. Show that both branches exist
(`git branch --all`), that each worktree's `ahm prime` prints its own branch,
and that neither worktree sees the other's unmerged commit in its working
tree.

Then the PR loop for one branch: push `feat/alpha`, open a pull request,
confirm the `ci` check runs and passes, confirm a direct push to `master`
fails locally (guard hook) and on GitHub (branch protection), merge the PR,
then return to the main checkout and run `ahm prime` to confirm master's
indexes regenerate cleanly and `git status --short` is clean. Repeat for
`feat/beta` to show sequential merging of parallel work.

Finally, record the answer to the known caveat: if both branches had created
an ahm task record with the same numeric ID, the merge would conflict; this
proof should demonstrate the mitigation (pre-allocate task IDs on master
before branching, or resolve the duplicate by renumbering via ahm records
commands) and note the outcome in Surprises.

Acceptance for 263g:

- [ ] The transcript shows two worktrees on two branches with independent
      commits.
- [ ] The transcript shows CI on the branch push, the local master-commit
      refusal, GitHub's direct-push rejection, and a green PR merge.
- [ ] After both merges, `ahm prime` on master shows a clean index and
      `git status --short` is clean.
- [ ] The caveat outcome (task-ID collision or its avoidance) is recorded in
      this ExecPlan's Surprises section.

## Progress

- [x] (2026-08-01) Analysis of the current state: CI triggers, release
      script, hooks, ahm branch/worktree mechanics, and instruction files.
      Findings recorded in Surprises & Discoveries.
- [x] (2026-08-01) Tracker 263 created; children 263a-263g created, mapped to
      milestones, and accepted to Pending. Note: suffix letters were allocated
      out of call order by ahm's ID-allocation lock (263a=worktree workflow
      docs, 263b=branch protection, 263c=instructions, 263d=guard hook,
      263e=CI/release, 263f=prompt.md, 263g=proof); the ExecPlan uses the
      verified mapping above.
- [x] (2026-08-01) This ExecPlan written and linked from tracker 263's
      `exec_plan` field.
- [x] (2026-08-01) Tracker 263 set to Tracking; real Acceptance Notes added
      to all eight tasks (create's TODO placeholders replaced with the
      milestone criteria); execution order made explicit in tracker 263;
      ExecPlan corrected with the verified finding that linked worktrees
      share the main checkout's hooks directory.
- [x] (2026-08-01) Review round: owner decisions folded in — guard hook has
      no bypass (263d); worktree helper dropped in favor of documented
      commands (263a); release flow moved to a release branch + PR (263e);
      self-merge allowed after green CI (263b); Milestone 6 assigned to the
      agent with push/PR authorization, `gh` 2.97.0 verified.
- [x] (2026-08-01) Milestone 1 (263e) implemented on `feat/263e-ci-releases`:
      ci.yml runs on all pushes; prepare-release.sh guards master at the top
      of the script and prints release-branch + PR instructions;
      docs/release.md rewritten for the release-branch + PR flow. Local
      checks passed (`just ci`; guard exits 1 on the feature branch); push
      and PR CI green on GitHub; master-path clone test verified the guard,
      changelog regeneration, and printed release-branch instructions.
- [x] (2026-08-01) Milestone 2 (263a) implemented on
      `feat/263a-worktree-docs`: CONTRIBUTING.md gained a Worktree Setup
      subsection under Local Setup (create, prime, shared hooks, cleanup);
      no script or justfile recipe. Verified: docs-md-lint clean; scratch
      linked worktree ran `ahm prime` with `git.branch=feat/<slug>` in
      `--json` output and hooks resolving to the main checkout's `.git/hooks`;
      `just ci` green on the branch.
- [ ] Milestone 3 (263d): master-commit guard hook.
- [ ] Milestone 4 (263c): AGENTS.md, CONTRIBUTING.md, and
      safety-and-permissions instructions with evidence and probe.
- [ ] Milestone 4b (263f): reconcile .agents/prompt.md.
- [ ] Milestone 5 (263b): GitHub branch protection enabled (manual).
- [ ] Milestone 6 (263g): end-to-end proof with two parallel worktrees.

## Surprises & Discoveries

- Observation: ahm is already worktree-compatible by design, so this plan
  needs no binary changes. `TestDetectManagedRootSucceedsWithDotGitFile`
  covers worktree `.git` pointer files; `TestBranchCheckoutRegeneratesIndexes`
  proves `git checkout` plus `ahm prime` rebuilds branch-scoped indexes;
  `ahm context` and `ahm prime` already report the current branch.
  Evidence: `internal/ahm/root_test.go:88`, `internal/ahm/prime_test.go:530`.

- Observation: the workflow lock lives under `.ahm/.lock/`, which is
  gitignored and therefore per-worktree. Two worktrees creating tasks in
  parallel can allocate the same numeric task ID on different branches; the
  collision surfaces only at merge time as a duplicate record path.
  Evidence: `internal/ahm/lock.go:55`; `.ahm/.gitignore` lists `.lock/`.
  Mitigation chosen in this plan: pre-allocate task IDs on master before
  branching, and record the outcome in Milestone 6.

- Observation: ahm records are branch-scoped committed files, so two parallel
  worktrees cannot see each other's task or research state; the coordination
  channel for parallel agent work is the PR and master between merges.
  Evidence: `.ahm/.gitignore` (source records committed, indexes ignored);
  `docs/VISION.md` "branch-scoped working records".

- Observation: `.agents/prompt.md` still describes the superseded ref-backed
  records vision ("refs/ahm/*", "never commit ... open PRs"), which conflicts
  with ADR 015's committed-record model and with the feature-branch workflow.
  Evidence: `.agents/prompt.md` Git-safety bullet; ADR 015.

- Observation: `scripts/prepare-release.sh` prints
  `git push origin $current_branch`, so on a feature branch it would push the
  release there; `docs/release.md` already anchors releases to master. This is
  the release-side gap Milestone 1 closes.

- Observation: parallel `ahm task create` invocations in one worktree
  demonstrated the ADR 010 ID-allocation lock working: child IDs 263a-263g
  were allocated uniquely but out of call order. This is a live example of the
  mechanism that Milestone 6 must probe across worktrees.
  Evidence: task creation transcript on 2026-08-01.

- Observation: a feature branch has been used at least once already:
  `origin/ci/task-260-node24-actions` was merged as PR #1. The tooling works;
  only the documented workflow and enforcement are missing.
  Evidence: `git branch -a`; commit 8dc8765.

- Observation: the documented release flow conflicts with the new guards.
  `docs/release.md` instructs committing the changelog and running
  `git push origin master`, which the guard hook (Milestone 3) and branch
  protection (Milestone 5) both block. The release commit moves to a
  `release/vX.Y.Z` branch + PR; tag pushes are not branch-protected, so
  GoReleaser still triggers.
  Evidence: docs/release.md:85-99.

- Observation: linked worktrees share the main checkout's hooks directory via
  the common git dir, so one `prek install` covers all worktrees; the plan's
  original per-worktree install assumption was wrong and was corrected.
  Evidence: `git rev-parse --git-path hooks` from a linked worktree in a
  scratch repo, 2026-08-01.

## Decision Log

- Decision: CI runs on every push, not only pushes to master.
  Rationale: branch protection requires a status check, and verifying feature
  branches before PR review is the point of the workflow; the cost is one
  extra run per push.
  Date/Author: 2026-08-01, Travis + agent.

- Decision: releases stay master-anchored; `prepare-release.sh` refuses any
  branch other than master.
  Rationale: releases are tag-driven from master (`docs/release.md`), and the
  script's current-branch push would otherwise publish a release from a
  feature branch.
  Date/Author: 2026-08-01, Travis + agent.

- Decision: enforcement is GitHub branch protection; the local guard hook is a
  nudge with an explicit bypass.
  Rationale: hooks are per-checkout and trivially bypassable, while branch
  protection is the only mechanism that actually blocks pushes to master.
  Date/Author: 2026-08-01, Travis + agent.

- Decision: the worktree helper runs `prek install` even though linked
  worktrees share the main checkout's hooks directory via the common git
  dir. (Superseded 2026-08-01 by the decision below: no helper script
  exists; the hooks-sharing finding itself still stands.)
  Rationale: the install is idempotent (it writes to the same shared hooks
  dir) and acts as a safety net if the main checkout never installed hooks;
  switching to `core.hooksPath` is unnecessary because sharing already
  happens by default.
  Date/Author: 2026-08-01, Travis + agent.

- Decision: task-ID collisions across parallel worktrees are accepted for now,
  mitigated by pre-allocating task IDs on master before branching. No new
  shared ID-allocation design is built in this plan.
  Rationale: a shared allocator would require ahm to perform ref or network
  operations, crossing its git-safety boundary (ADR 015); the collision is
  rare, visible at merge time, and resolvable with ahm records commands.
  Date/Author: 2026-08-01, Travis + agent.

- Decision: instruction changes (263c, 263f) are behavior-shaping edits and
  must carry the evidence and fresh-session probe required by
  `docs/guardrails/agent-instructions.md`, or an explicit deferral.
  Rationale: the guardrail exists precisely because instruction edits without
  evidence of effect accumulate; this plan follows it rather than bypassing it.
  Date/Author: 2026-08-01, Travis + agent.

- Decision: the master-commit guard hook has no bypass and no exceptions.
  Rationale: any escape hatch (env var, message prefix) is a loophole agents
  will find and exploit; Milestone 1 removes the only previously legitimate
  master commit, so the hook can be unconditional.
  Date/Author: 2026-08-01, Travis + agent.

- Decision: no worktree helper script or justfile recipe; the workflow is
  documented in CONTRIBUTING.md.
  Rationale: agents manage git worktrees directly, `ahm prime` is already
  mandated by the operating loop, and a script would duplicate git's own
  surface for little gain.
  Date/Author: 2026-08-01, Travis + agent.

- Decision: releases move the changelog commit to a `release/vX.Y.Z` branch
  merged via PR; tags still push directly.
  Rationale: branch protection blocks the documented `git push origin master`
  and the no-bypass hook blocks the master changelog commit; tags are not
  branch-protected, so GoReleaser still triggers on the tag push.
  Date/Author: 2026-08-01, Travis + agent.

- Decision: self-merge after a green `ci` check is allowed on master; no
  review requirement and no actor exemptions.
  Rationale: the owner is the only committer; protection exists to block
  direct pushes, not to add review overhead.
  Date/Author: 2026-08-01, Travis + agent.

- Decision: 263e-263f implementation proceeds on feature branches in the
  main checkout rather than per-task worktrees.
  Rationale: the agent sandbox cannot write outside the project directory,
  so sibling worktrees are unavailable for this session; sequential
  milestones need no parallelism. The documented worktree flow still stands
  for parallel work, and Milestone 6's parallel proof will be run by the
  repository owner.
  Date/Author: 2026-08-01, Travis + agent.

## Outcomes & Retrospective

- (2026-08-01) Milestone 2 (263a) delivered: CONTRIBUTING.md documents the
  worktree create (`git worktree add -b feat/<slug> ../ahm-<slug> master`),
  prime, shared-hooks, and cleanup flow with no helper script, per the
  Decision Log. The worktree claim was verified empirically: a scratch linked
  worktree's `ahm prime --json` reports `git.branch` and `git rev-parse
  --git-path hooks` resolves to the main checkout's `.git/hooks`. One
  sandbox caveat: the agent shell cannot write outside the project directory,
  so the verification worktree used an in-repo path; the documented
  `../ahm-<slug>` path is equivalent and Milestone 6 will exercise it.

