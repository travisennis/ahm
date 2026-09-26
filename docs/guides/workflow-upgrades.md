# Workflow Upgrades

`ahm` owns workflow state, and `ahm init` reconciles it:

```bash
ahm init
```

`ahm init` creates `.ahm/` state when it is absent and reconciles it when it is
present. It rewrites only the files ahm owns — `.ahm/config.json`, the managed
`.ahm/.gitignore`, and the generated indexes — and only when their bytes differ
from what ahm owns, so an up-to-date repository is left completely untouched.

- Obsolete ahm-owned configuration keys (`taskWork`, `default_work_agent`,
  `projectDocs`, `research`) are dropped, and `files` ownership hashes for the
  retired managed files and generated-index paths this version knows about are
  discarded. Other top-level fields, and any `files` entry outside that list,
  are preserved.
- Retired managed files stay exactly as the project left them. Older releases
  installed instruction templates, procedure skills, and record scaffold
  READMEs and tracked ownership hashes for them; `ahm init` discards those
  stale hashes and never creates, inspects, overwrites, or removes the files.
- `AGENTS.md` is project-owned. `ahm` never creates, overwrites, or removes it,
  even with `--force`.
- Generated indexes are regenerated.
- `--dry-run` previews every write without touching the filesystem.

See [the workflow specification](../references/workflow-spec.md) for the
complete file ownership boundary.

## Moving Task Records To The Home Store

Existing repositories keep task records in the project until they opt in to
the home store. Install a build that includes the `store migrate` command;
v2.0.0 is the first release that provides it. Set `AHM_HOME` to an absolute
path to override the default `~/.ahm` store root. Record the current status
before moving, then preview and run the move from the repository root:

```bash
ahm status --json > /tmp/ahm-status-before.json
ahm --dry-run store migrate --to home
ahm store migrate --to home
```

The migration refuses to run when task records under `.ahm/tasks/` have
uncommitted changes, because the move deletes them and Git history is the only
recovery for content that exists only in the working tree. Commit or discard
those record changes first. Do not start a task immediately before the
migration: `ahm task start` changes the task record and therefore trips the
same guard; start it after the move. A dry run previews the record moves and
the configuration, ignore-file, and index writes without touching the project
or creating the store.

After the move, inspect the resolved store and exercise the workflow surface.
If the store already contained records for this project, run `ahm init` once
before deleting any remaining project records; `init` seeds or raises the
store's `next_id` counter from the records present.

```bash
ahm store path
ahm init
ahm prime
ahm doctor
ahm status --json > /tmp/ahm-status-after.json
ahm task list
TASK_ID=001  # replace with an existing task ID
ahm task show "$TASK_ID"
ahm task next
ahm index
```

Compare the `tasks` counts and `validation` sections in the before and after
status reports; they should match, while the after report also contains the
`store` block. `prime`, `status`, and the task commands read the store named by
`store path`; ADRs and `docs/adr/index.md` stay in the repository. The
migration writes `tasks_location: home` to `.ahm/config.json`, removes the
project's task records and generated task indexes, and leaves the deletions
unstaged. A leftover `.ahm/tasks/**/index.md` from an interrupted or older
migration is derived output, not a source record: rerun the same
`store migrate --to home` to remove it, or delete it after verifying the
store-side index. Do not commit a project task index in home mode; the store's
managed `.gitignore` excludes its generated indexes, lock, and state file.
Review `git status --short` and commit the deletions together with
`.ahm/config.json` and `.ahm/.gitignore`. The store is machine-local and is
not part of the repository. `ahm` never stages or commits the move.

To move the records back to the repository, run
`ahm store migrate --to project`. The uncommitted-record refusal applies only
when moving records out of the project, so it does not guard this direction;
review the project additions and the configuration change before committing
them.

## Migrating To v2

v2 reduces `ahm` to a records CLI for tasks and ADRs (ADR 022). Six commands
are gone, with no aliases and no replacements: `audit`, `context`, `onboard`,
`task groom`, `task work`, and `upgrade`. Delete every hook, CI step, script,
and instruction that invokes them. The `records` group and its commands
(`records migrate`, `records doctor`), `task migrate`, and `adr migrate` are
gone with the legacy layout support they served.

Research notes under `.ahm/research/` and ExecPlans under `.ahm/exec-plans/`
stop being ahm-managed record families: `ahm` no longer reads, indexes, or
validates them, and the task front-matter field `exec_plan` is retired.
Nothing is deleted. Those files stay where they are, the generated indexes
inside them stay stale, and keeping or removing them is the project's choice.

Move a repository onto v2 by running `ahm init` with the new binary. It
reconciles ahm-owned configuration — dropping `taskWork` — rewrites the
managed `.ahm/.gitignore` so it no longer ignores the retired families'
indexes, and regenerates the indexes. The retired families' index files were
ignored by the old pattern, so after `ahm init` they appear as untracked paths
in `git status` unless the project commits or deletes them. Everything not
listed above is compatible: the task and ADR lifecycles, record formats,
generated index formats, exit codes, and output modes behave as they did in
v1.

## Migrating A Legacy `.agents/ahm.json` Repository

This version reads only `.ahm/config.json`. Root detection refuses a repository
whose metadata is still `.agents/ahm.json`, so a legacy tree is never
half-adopted:

```text
error: legacy ahm workflow layout <root>/.agents/ahm.json: this version
reads only .ahm/config.json; upgrade the repository with the final v1
release (ahm v1.1.0) before using this version
```

A repository on that layout must move with `v1.1.0`, the last release that
reads it, before it can adopt v2. Install that build, then run `ahm upgrade`
and then `ahm records migrate`: `ahm upgrade` refreshes the managed files, and
`ahm records migrate` moves the records to `.ahm/`, writes `.ahm/config.json`,
and removes `.agents/ahm.json`. Then install v2 and run `ahm init`.

The `v1.0.0` tag predates `ahm records migrate`, so it cannot perform the move
and is not the release this error names. If you are on `v1.0.0`, install
`v1.1.0` rather than trying to migrate with what you have.

## Release History

This guide used to carry a dated entry for every workflow-state change through
v1: added and removed commands, the `context` scope split, the research inbox,
the record-layout moves, and the template-version separation. v2 removed most
of the surfaces those entries describe, so they were dropped rather than left
here offering commands that no longer exist. Recover them with `git log -p --
docs/guides/workflow-upgrades.md`; [`CHANGELOG.md`](../../CHANGELOG.md) holds
the released versions (v0.1.0, v1.0.0, and v1.1.0), and
[the ADRs](../adr/index.md) hold the decisions.
