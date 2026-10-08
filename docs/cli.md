# ahm CLI Reference

This is the stable entrypoint for the `ahm` command contract. The detailed
reference is split by surface so agents and contributors can load only the part
they need.

## Start Here

- [Global contract](references/cli/global-contract.md): usage, root selection,
  global flags, output modes, and exit codes.
- [Commands](references/cli/commands.md): non-task commands including ADR,
  init, status, doctor, index, and home-store commands.
- [Task commands](references/cli/task-commands.md): bulk JSON import, task lifecycle,
  dependencies, completion, blocking, cancellation, and reopening.
- [Task file and validation formats](references/cli/task-file-format.md): task
  Markdown format and validation finding codes.

## Compatibility Contract

CLI command names, flags, aliases, exit codes, help text, text output, JSON
output, plain output, dry-run behavior, and validation finding codes are
compatibility surfaces. Preserve them unless a task explicitly changes the CLI
contract.

Structured `init` summaries have a stable set of array-valued keys:
`created`, `updated`, `directories`, and `indexes`. Every key remains present as
an empty array when a reconcile pass changed nothing. `status` and `prime`
add a `store` object with `root`, `key`, `kind`, and `location` when an
installed project keeps task records in the home store; the field is absent in
project mode. `status` and `doctor` also report the resolved `records_mode`
(`project` or `home`) and the effective `strict_acceptance` boolean; both are
`none`/`null` when the project is not installed, and `strict_acceptance` is
also unknown when the committed configuration was not read.

The global `--project <selector>` flag targets another project's task records
through the home store, resolving the selector from the store registry without
reading a checkout or running Git. It overrides task record resolution only and
is mutually exclusive with `--root`; see the
[global contract](references/cli/global-contract.md) for the accepted commands,
the selector matching rules, and the exit codes.

The `store` family manages the store's location, registry, and records layout.
`ahm store list` is its read-only inventory: it reports every registered project
with its store directory, recorded paths, and record count, works from any
directory without writing, and refuses `--project`. See the
[commands reference](references/cli/commands.md) for its output and guarantees.

`task list`, `task ready`, and `task blocked` share configurable deterministic
ordering through `--sort` and `--reverse`; the supported fields and rank rules
are documented in the task command reference.

`task edit <id>` replaces a task field when its flag is supplied and leaves the
field alone when the flag is omitted. It refuses to write the `## Comments` and
`## Cancellation Reason` body sections, which `task comment` and `task cancel`
own, and it never writes `status` or `depends_on`. `--add-label` accepts only
labels some record already carries (see `task labels`); a new label enters
through `task create --labels`.

`task create` warns on stderr, but still creates the task, when the new title
matches an existing active task case-insensitively. The warning names the
colliding task's ID, status, and title; `Completed` and `Cancelled` records are
not compared. The match is exact, not fuzzy.

For implementation boundaries and invariants, see
[`ARCHITECTURE.md`](../ARCHITECTURE.md). For workflow state, store identity,
and file-format semantics, see
[the workflow specification](references/workflow-spec.md).

`task import --from-file tasks.json` creates a batch from an offline JSON array.
Use `--dry-run` to preview IDs, paths, parents, dependencies, and all refusals.
The [bulk import contract](references/cli/task-commands.md#bulk-import) defines
fields, `@ref` batch references, output, and failure recovery.
