# ahm CLI Reference

This is the stable entrypoint for the `ahm` command contract. The detailed
reference is split by surface so agents and contributors can load only the part
they need.

## Start Here

- [Global contract](references/cli/global-contract.md): usage, root selection,
  global flags, output modes, and exit codes.
- [Commands](references/cli/commands.md): non-task commands including onboard,
  audit, ADR, install, upgrade, status, doctor, and index behavior.
- [Task commands](references/cli/task-commands.md): task lifecycle,
  backlog grooming, dependencies, delegation, completion, cancellation, and
  reopening.
- [Task file and validation formats](references/cli/task-file-format.md): task
  Markdown format and validation finding codes.

## Compatibility Contract

CLI command names, flags, aliases, exit codes, help text, text output, JSON
output, plain output, dry-run behavior, and validation finding codes are
compatibility surfaces. Preserve them unless a task explicitly changes the CLI
contract.

For implementation boundaries and invariants, see
[`ARCHITECTURE.md`](../ARCHITECTURE.md). For workflow state and file-format
semantics, see [the workflow specification](references/workflow-spec.md).
