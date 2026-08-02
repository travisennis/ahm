You are working one task in the ahm repository, a Go CLI that manages
repo-local agent workflow state. Tasks, research notes, ExecPlans, and
config live under `.ahm/`; project guidance lives in `AGENTS.md` and under
`.agents/`. Workflow records are ordinary committed, branch-scoped files;
generated indexes are gitignored machine-local state that `ahm prime` and
`ahm index` regenerate. `ahm` performs no git ref or network operations.

## Git-safety boundary (the ahm binary)

`ahm` reads git freely but never commits, stages, touches the index or
HEAD, mutates branches, opens PRs, or patches project source on its own.
This boundary describes the tool. Your own commit and branch behavior is
governed by `AGENTS.md`: work on a `feat/<slug>` branch, never commit
directly to `master`, and merge only through a pull request with CI green.

## Working the task

- Read the task record and its acceptance notes via `ahm task show <id>`
  before choosing implementation work.
- Classify the implementation under `AGENTS.md` Workflow Routing and load
  only the routed documents; run `ahm prime` before any work.
- Write task and ADR records through `ahm task` and `ahm adr`; keep
  research notes as files under `.ahm/research/`; never hand-edit generated
  indexes (`ahm index` regenerates them).
- Keep changes scoped to the task; record decisions, surprises, and
  discovered constraints as task comments (`ahm task comment <id>`).

## Verification and handoff

- Update the docs your change makes stale in the same change and run the
  verifications `CONTRIBUTING.md` prescribes (`just ci` before handoff).
- Hand off per `AGENTS.md`: route and docs loaded, what changed, exact
  checks run, remaining risk, and the branch name with PR status.
