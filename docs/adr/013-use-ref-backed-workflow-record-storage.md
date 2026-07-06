---
status: accepted
date: 2026-07-06
decision-makers: Travis Ennis
consulted: -
informed: task 138
---
# Use Ref-Backed Workflow Record Storage

## Context and Problem Statement

`ahm` currently stores tasks, research notes, ExecPlans, generated workflow
indexes, and repository configuration as ordinary files under `.agents/` in the
consumer repository. That makes active workflow state easy to inspect and
recover, but it also leaves short-lived agent workflow artifacts in normal
branch commits, pull requests, and project history. Completed tasks,
backlog-grooming churn, scratch research, and draft plans are working records;
their durable outcomes should be promoted into project documentation, design
docs, or ADRs rather than preserved as branch-history ceremony.

There is a second ownership problem. `.agents/` is becoming the
ecosystem-standard directory for agent-facing project content such as skills
and standing prompt material. `ahm` should be able to read project-owned
`.agents/` content, but it should not mix mutable tool-owned workflow state into
that same namespace. A tool-owned `.ahm/` directory gives `ahm` one place for
records, generated indexes, sync state, and configuration, with an internal
`.gitignore` that keeps working records out of branch history without editing a
consumer repository's root `.gitignore`.

The project still needs durability. A pure `.gitignore` approach would keep
workflow records out of branch history, but it would also make a solo
developer's backlog vulnerable to laptop loss, disk failure, or accidental
cleanup. `ahm` is primarily designed for coding-agent workflows, so requiring
agents and humans to use `ahm` to acquire and sync workflow records is
acceptable.

## Decision Drivers

- Keep task, research, and ExecPlan churn out of normal project commits and
  pull requests.
- Preserve active backlog state across machine loss and machine-to-machine
  work.
- Keep the editable working copy as plain Markdown under a tool-owned `.ahm/`
  namespace.
- Avoid branch, index, staging-area, `HEAD`, and normal history mutations during
  routine workflow-record operations.
- Make the agent workflow simple: a coding agent can run one command before a
  session to sync records and inspect the current backlog.
- Keep ADRs, accepted design docs, and other durable project documentation in
  normal committed project history.
- Keep project-owned agent content under `.agents/` committed and out of
  ref-backed workflow-record storage.
- Support GitHub first. Other Git remotes can be probed later when they become
  real requirements.

## Considered Options

- **Keep records committed in the project branch.** This is the current model.
  It is durable and simple, but it pollutes branch history and pull requests
  with working artifacts.
- **Use gitignored `.agents` records only.** This removes branch-history
  pollution, but active backlog state can be lost if the local machine is lost
  or cleaned, and it keeps `ahm`-managed state mixed into a project-owned
  agent-content namespace.
- **Use gitignored `.ahm` records only.** This gives `ahm` a clear tool-owned
  namespace and avoids root `.gitignore` edits, but active backlog state can
  still be lost if the local machine is lost or cleaned.
- **Move records to an external per-repo store.** This keeps records out of the
  project tree, but it makes discovery harder for agents, creates a repo-key
  problem, and still needs a separate backup/sync story.
- **Use an orphan branch or nested worktree.** This gives Git-backed sync but
  exposes branch/worktree complexity to users and creates accidental checkout
  and IDE confusion.
- **Use gitignored `.ahm` working files plus a private Git ref.** Records
  remain plain files under `.ahm`, but `ahm` snapshots and explicitly syncs
  them through a custom ref such as `refs/ahm/records`.

## Decision Outcome

Chosen option: **tool-owned `.ahm` workflow state with gitignored records and
private-ref durability/sync**, initially targeting GitHub.

Tasks, scratch research notes, and draft ExecPlans should move from `.agents/`
to local working files under `.ahm/` as part of an explicit opt-in migration.
Those records should be gitignored and untracked from the project branch.
`ahm` should provide a records command surface that snapshots those files into
`refs/ahm/records` and explicitly syncs that ref to the configured GitHub
remote. Generated indexes should remain local-only and should be regenerated
from source records rather than snapshotted into the ref.

Routine ref-backed record operations may write local workflow files under
`.ahm/`, local refs under `refs/ahm/*`, and the minimal repo configuration
needed to fetch and push that namespace. Opt-in migration may move workflow
records out of `.agents/` and install a managed `.ahm/.gitignore`, but routine
operation must not write project-owned `.agents/` content. `ahm` must not
commit, create pull requests, mutate user branches, stage files, write the
index, move `HEAD`, or patch project source code.

`ahm` configuration should move from `.agents/ahm.json` to committed
`.ahm/config.json` during the same migration. Absence of the new configuration
must preserve the current committed-record behavior for existing repositories
until they opt in. Root detection must continue to work before and after the
namespace migration, and documentation, templates, validation, and command
output should treat `.agents/` as project-owned content that `ahm` can read but
does not manage.

The agent-facing entry point should be `ahm prime`. The command is the
session-start briefing hook; task 156 introduces the initial read-only briefing
form, and ref-backed records later add a fast, idempotent, offline-tolerant sync
step. When sync is available, `ahm prime` should fetch/sync the records ref,
materialize local records, regenerate indexes, validate workflow state, and
print the backlog state a coding agent needs before starting work. Failed
network sync should degrade to a warning rather than block a session, and a
`--no-sync` escape hatch should remain available for hooks or offline work.

Existing repositories should keep the current committed-record behavior until a
user explicitly opts into migration. Migration must preview effects and print
any required `git rm --cached` command for the user to run rather than silently
untracking project-owned records. Migration must also preserve project-owned
`.agents/` content such as `AGENTS.md` guidance, `.agents/prompt.md`, and other
agent-facing files; those files are committed project content and are not
snapshotted to `refs/ahm/records`.

Sequential task IDs remain acceptable for the first implementation. A future
task-ID redesign can revisit random or hash-like IDs if active multi-clone or
team task creation becomes a supported target.

### Consequences

- Good, because workflow ceremony no longer has to appear in normal branch
  history while active backlog state can still be recovered from GitHub.
- Good, because `.ahm/` gives `ahm` a clear managed-state boundary while
  leaving `.agents/` for committed project-owned agent content.
- Good, because agents get a clear startup command and humans get explicit
  stale/unsynced-state reporting.
- Good, because ADRs and durable project docs remain committed where they
  belong.
- Good, because generated indexes stay derived and local-only instead of
  becoming a merge surface inside the records ref.
- Bad, because this replaces the simple "no implicit git operations" guarantee
  with a narrower and more nuanced Git-safety boundary.
- Bad, because ref-backed storage introduces sync, staleness, conflict,
  migration, and remote-diagnostics behavior that must be designed and tested.
- Bad, because moving from `.agents/ahm.json` and `.agents/` records to
  `.ahm/config.json` and `.ahm/` records creates path churn across root
  detection, validation, templates, docs, and user guidance.
- Bad, because non-GitHub remotes are not part of the initial supported target.

## More Information

- Research note:
  `.agents/.research/topics/records-storage-via-git-refs.md`
- Implementation tracker: task 138.
- Execution plan:
  `.agents/exec-plans/active/138-ref-backed-workflow-records.md`
- GitHub smoke test on 2026-06-30 confirmed that GitHub accepts
  `refs/ahm/records`, normal clone does not fetch that namespace, explicit
  fetch works, GitHub's refs API lists the ref, and deleting a probe custom ref
  works.
