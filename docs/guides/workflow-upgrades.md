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
release (ahm v1.0.0) before using this version
```

A repository on that layout must move with a v1 build that ships the record
migration before it can adopt v2. Run `ahm upgrade` and then `ahm records
migrate`: `ahm upgrade` refreshes the managed files, and `ahm records migrate`
moves the records to `.ahm/`, writes `.ahm/config.json`, and removes
`.agents/ahm.json`. Then run `ahm init` with this version.

The `v1.0.0` tag predates `ahm records migrate`, so a repository on that
release needs the last v1 build before this reduction — the one whose `ahm
records migrate` resolves — to make the move. Check with `ahm records migrate
--help` before starting.

## Release History

This guide used to carry a dated entry for every workflow-state change through
v1: added and removed commands, the `context` scope split, the research inbox,
the record-layout moves, and the template-version separation. v2 removed most
of the surfaces those entries describe, so they were dropped rather than left
here offering commands that no longer exist. Recover them with `git log -p --
docs/guides/workflow-upgrades.md`; [`CHANGELOG.md`](../../CHANGELOG.md) holds
the released versions (v0.1.0 and v1.0.0), and [the ADRs](../adr/index.md) hold
the decisions.
