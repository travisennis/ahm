# ahm

`ahm` is an agent harness manager. It installs and manages a repo-local
`.agents` workflow for agent tasks, research notes, ExecPlans, and generated
indexes.

`ahm` replaces the earlier `agent-workflow-scaffold` skill as the owner of this
workflow. The canonical workflow templates live in this repository and are
embedded into the CLI at build time.

## Status

Initial implementation. The CLI supports workflow install/upgrade/status,
native task index generation, and basic task management.

## Quickstart

```bash
ahm init
ahm status
ahm task create "Add release workflow" --priority P2 --effort M --labels type:task,area:ci
ahm task ready
ahm task show 001
ahm task complete 001
```

Useful global flags:

- `--root <path>`: target repository root. Defaults to the nearest git root or
  current directory.
- `--json`: print structured JSON.
- `--plain`: print stable line-oriented output.
- `--dry-run`: preview write operations for commands that support it.
- `--force`: overwrite managed workflow files when supported.

## Safety

`ahm` does not commit, push, open PRs, or modify source code. Write commands are
explicit and operate on the `.agents` workflow files unless a future command
states otherwise.

## Development

This machine did not have `go` on PATH when the project was bootstrapped. After
installing Go 1.22 or newer, run:

```bash
go test ./...
go vet ./...
gofmt -w .
```
